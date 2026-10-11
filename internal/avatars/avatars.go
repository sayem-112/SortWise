// Package avatars keeps authors' profile pictures on this computer. A worker
// downloads each picture from X's image server, with no login or cookie, and
// saves it in the data folder. The app then shows the saved copy, so a
// picture survives its author changing or removing theirs, and showing
// authors' pictures asks X for nothing.
package avatars

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"sortwise/internal/store"
	"sortwise/internal/xgraphql"
)

const (
	// maxBytes bounds one picture; real ones are a few kilobytes.
	maxBytes = 2 << 20
	// batch is how many pictures one round fetches.
	batch = 8
	// idle is the wait when there is nothing to do.
	idle = 5 * time.Second
	// With nothing new, older posts are re-checked, and the folder is brought
	// in line with the records, this often.
	upkeep = 10 * time.Minute
	// When the image server can't be reached or refuses, everything waits:
	// this long at first, doubling to the limit.
	firstBackoff = 30 * time.Second
	maxBackoff   = 30 * time.Minute
	maxRedirects = 3
)

// ValidFile reports whether name is a saved picture's file name.
func ValidFile(name string) bool { return store.ValidAvatarFile(name) }

// picturePath is the one shape a profile picture's address has on X's image
// server: the account's number, then a file name.
var picturePath = regexp.MustCompile(`^/profile_images/\d+/[A-Za-z0-9_-]+\.(jpg|jpeg|png|gif|webp)$`)

// thumbnail is the 48px size X hands out; sharper asks for the 200px one.
var thumbnail = regexp.MustCompile(`_normal(\.[a-z]+)$`)

// extensions maps the image kinds that are saved to file extensions.
var extensions = map[string]string{"image/jpeg": "jpg", "image/png": "png", "image/webp": "webp", "image/gif": "gif"}

var (
	// errNothing means there is no picture to save from an address, now or
	// later: X's default picture, a dead address, or not a picture at all.
	errNothing = errors.New("no picture to save")
	// errMissing means the image server has nothing at that address.
	errMissing = errors.New("no picture at that address")
	// errRedirected means the image server pointed somewhere pictures are
	// not fetched from.
	errRedirected = errors.New("redirected away from the picture")
)

// Worker downloads pictures in the background.
type Worker struct {
	store *store.Store
	dir   string
	// scheme and host are the only place pictures are fetched from.
	scheme, host string
	client       *http.Client
	// pause is the gap between two downloads.
	pause time.Duration
	salt  string
	// backfilled is set once every saved author has been looked at;
	// nextUpkeep is when to look again and tidy the folder.
	backfilled bool
	nextUpkeep time.Time
}

// New returns a worker that saves pictures into dir.
func New(s *store.Store, dir string) *Worker {
	w := &Worker{store: s, dir: dir, scheme: "https", host: "pbs.twimg.com", pause: 250 * time.Millisecond}
	w.client = &http.Client{
		Timeout: 20 * time.Second,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects || !w.allowed(request.URL) {
				return errRedirected
			}
			return nil
		},
	}
	return w
}

// allowed accepts only profile pictures on X's image server: the exact host
// on its usual port, over HTTPS, with no login or query in the address, and
// a path that is a picture's and nothing else.
func (w *Worker) allowed(address *url.URL) bool {
	return address.Scheme == w.scheme && address.Host == w.host && address.User == nil &&
		address.RawQuery == "" && address.Fragment == "" && picturePath.MatchString(address.EscapedPath())
}

// Run works until ctx ends: it finds picture addresses for authors saved
// before pictures were kept, downloads whatever is pending, and keeps the
// folder in line with the records.
func (w *Worker) Run(ctx context.Context) {
	if err := os.MkdirAll(w.dir, 0o700); err != nil {
		slog.Warn("Profile pictures are off: the folder could not be created", "error", err)
		return
	}
	backoff := time.Duration(0)
	for {
		busy, err := w.step(ctx)
		wait := idle
		switch {
		case ctx.Err() != nil:
			return
		case err != nil:
			// The image server is unreachable or refusing, or the disk or
			// database failed. Nothing is marked against any author; all
			// of it simply waits, longer each time.
			backoff = min(max(backoff*2, firstBackoff), maxBackoff)
			wait = backoff
			slog.Debug("Profile pictures are waiting", "error", err, "retry_in", wait.String())
		case busy:
			backoff, wait = 0, w.pause
		default:
			backoff = 0
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
}

// step does one round of work and reports whether there may be more. An
// error means "stop and try again later", and leaves every author as it was.
func (w *Worker) step(ctx context.Context) (bool, error) {
	if w.salt == "" {
		salt, err := w.store.AvatarSalt(ctx)
		if err != nil {
			return false, err
		}
		w.salt = salt
	}
	due := time.Now().After(w.nextUpkeep)
	if due {
		// Before anything is downloaded, so a picture being saved right now
		// is never mistaken for one nobody uses.
		if err := w.reconcile(ctx); err != nil {
			return false, err
		}
		w.nextUpkeep = time.Now().Add(upkeep)
	}
	looked := 0
	if !w.backfilled || due {
		var err error
		if looked, err = w.store.BackfillAvatars(ctx, xgraphql.AvatarFromRaw, 200); err != nil {
			return false, err
		}
		w.backfilled = looked == 0
	}
	pending, err := w.store.PendingAvatars(ctx, batch)
	if err != nil {
		return false, err
	}
	for index, item := range pending {
		if index > 0 {
			select {
			case <-ctx.Done():
				return false, ctx.Err()
			case <-time.After(w.pause):
			}
		}
		if err := w.save(ctx, item); err != nil {
			return false, err
		}
	}
	return looked > 0 || len(pending) == batch, nil
}

// reconcile drops authors who have no posts left, makes pictures whose file
// has gone missing due for download again, and deletes files nothing uses:
// replaced pictures that could not be deleted at the time, and anything
// half-written when the app last stopped.
func (w *Worker) reconcile(ctx context.Context) error {
	if err := w.store.DropOrphanAvatars(ctx); err != nil {
		return err
	}
	entries, err := os.ReadDir(w.dir)
	if err != nil {
		return err
	}
	present := []string{}
	for _, entry := range entries {
		switch name := entry.Name(); {
		case ValidFile(name):
			present = append(present, name)
		case strings.HasPrefix(name, "incoming-"):
			os.Remove(filepath.Join(w.dir, name))
		}
	}
	unused, err := w.store.ReconcileAvatars(ctx, present)
	if err != nil {
		return err
	}
	for _, name := range unused {
		w.remove(name)
	}
	return nil
}

// save downloads one picture and records the outcome. It returns an error
// only for trouble that is not about this picture, which stops the round.
func (w *Worker) save(ctx context.Context, item store.PendingAvatar) error {
	file, err := w.download(ctx, item.Username, item.URL)
	if errors.Is(err, errNothing) {
		// Nothing to fetch from this address. The copy saved earlier stays.
		return w.store.AvatarSettled(ctx, item.Username, item.URL)
	}
	if err != nil {
		// Trouble reaching the server, or saving the file. This author goes to
		// the back of the line, so one stubborn picture cannot hold up the rest.
		if deferErr := w.store.AvatarDeferred(ctx, item.Username); deferErr != nil {
			return deferErr
		}
		return err
	}
	recorded, err := w.store.AvatarSaved(ctx, item.Username, item.URL, file)
	if err != nil || !recorded {
		// The author's picture changed again, or they are gone: this copy is not wanted.
		w.remove(file)
		return err
	}
	if item.OldFile != file {
		w.remove(item.OldFile)
	}
	return nil
}

// download fetches the picture at address and returns the saved file's
// name. errNothing means the address will never give a picture; any other
// error means try again later.
func (w *Worker) download(ctx context.Context, username, address string) (string, error) {
	parsed, err := url.Parse(address)
	if err != nil || !w.allowed(parsed) {
		// X's default picture lives elsewhere, and anything else that is not
		// a profile picture's address is never requested.
		return "", errNothing
	}
	// The sharper size first; a picture without one falls back to what X gave.
	sharp := *parsed
	sharp.Path, sharp.RawPath = thumbnail.ReplaceAllString(parsed.Path, "_200x200$1"), ""
	body, err := w.fetch(ctx, &sharp)
	if errors.Is(err, errMissing) && sharp.Path != parsed.Path {
		body, err = w.fetch(ctx, parsed)
	}
	if errors.Is(err, errMissing) {
		return "", errNothing
	}
	if err != nil {
		return "", err
	}
	// Trust what the bytes are, not what the server says they are.
	extension, ok := extensions[http.DetectContentType(body)]
	if !ok {
		return "", errNothing
	}
	// The name is private to this library (the salt) and changes with the
	// picture, so it cannot be guessed and can be cached for good.
	sum := sha256.Sum256([]byte(w.salt + "\n" + strings.ToLower(username) + "\n" + address))
	name := hex.EncodeToString(sum[:10]) + "." + extension
	// Written beside its final place, then moved in whole, so a half-written
	// picture is never served.
	temp, err := os.CreateTemp(w.dir, "incoming-*")
	if err != nil {
		return "", err
	}
	_, err = temp.Write(body)
	if closeErr := temp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(temp.Name(), filepath.Join(w.dir, name))
	}
	if err != nil {
		os.Remove(temp.Name())
		return "", err
	}
	return name, nil
}

// fetch reads one picture. errMissing and errNothing say the address has no
// usable picture; anything else is trouble reaching the server.
func (w *Worker) fetch(ctx context.Context, address *url.URL) ([]byte, error) {
	if !w.allowed(address) {
		return nil, errNothing
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address.String(), nil)
	if err != nil {
		return nil, errNothing
	}
	response, err := w.client.Do(request)
	if errors.Is(err, errRedirected) {
		return nil, errNothing
	}
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	switch {
	case response.StatusCode == http.StatusNotFound || response.StatusCode == http.StatusGone:
		return nil, errMissing
	case response.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("image server answered %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) == 0 || len(body) > maxBytes {
		return nil, errNothing
	}
	return body, nil
}

// remove deletes a saved picture, if name is one. A file that cannot be
// deleted right now (another program has it open) goes at the next reconcile.
func (w *Worker) remove(name string) {
	if ValidFile(name) {
		os.Remove(filepath.Join(w.dir, name))
	}
}
