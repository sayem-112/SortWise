// Package updates checks, once a day, whether a newer Sortwise release is
// out, so the app can say so. It only asks GitHub for the latest release's
// version: nothing is downloaded or installed, and nothing about you is
// sent. Development builds never check.
package updates

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"sortwise/internal/store"
)

const (
	// releases is where Sortwise is published.
	releases  = "https://github.com/sayem-112/SortWise/releases"
	latestAPI = "https://api.github.com/repos/sayem-112/SortWise/releases/latest"
	// every is how often to check; failed checks are tried again hourly.
	every = 24 * time.Hour
	retry = time.Hour
	// settle is how long after starting the first check waits, out of the
	// way of opening the library.
	settle = time.Minute
)

// Release is a newer version and the page to get it from.
type Release struct {
	Version string `json:"version"`
	URL     string `json:"url"`
}

// State is what Settings shows about updates.
type State struct {
	// Version is the running version, or "dev" for a development build.
	Version string `json:"version"`
	// Released is false for a development build, which never checks.
	Released bool `json:"released"`
	// Check is whether the daily check is on.
	Check bool `json:"check"`
	// Available is the newer release, when there is one.
	Available *Release `json:"available,omitempty"`
	// CheckedAt is when the last check finished; nil before the first.
	CheckedAt *time.Time `json:"checkedAt,omitempty"`
}

// Checker runs the daily check for the running version.
type Checker struct {
	current  string
	store    *store.Store
	client   *http.Client
	endpoint string
	wake     chan struct{}
}

func New(current string, st *store.Store) *Checker {
	return &Checker{current: current, store: st, client: &http.Client{Timeout: 20 * time.Second}, endpoint: latestAPI, wake: make(chan struct{}, 1)}
}

// Run checks whenever a day has passed since the last check, until ctx
// ends. A development build returns at once.
func (c *Checker) Run(ctx context.Context) {
	if _, ok := parse(c.current); !ok {
		return
	}
	timer := time.NewTimer(settle)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		case <-c.wake:
		}
		wait := c.checkIfDue(ctx)
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(wait)
	}
}

// Wake asks for a check now, if one is due: after the check is turned on.
func (c *Checker) Wake() {
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

// checkIfDue checks when the check is on and a day has passed, and returns
// how long to wait before looking again.
func (c *Checker) checkIfDue(ctx context.Context) time.Duration {
	record := c.store.UpdateRecord(ctx)
	if !record.On {
		return retry
	}
	// A check stamped in the future came from a clock that was wrong; it
	// doesn't hold the next one back.
	now := time.Now()
	if due := record.CheckedAt.Add(every); now.Before(due) && !now.Before(record.CheckedAt) {
		return min(retry, due.Sub(now))
	}
	latest, err := c.latest(ctx)
	if err != nil {
		slog.Info("Could not check for a newer Sortwise", "error", err)
		return retry
	}
	if err := c.store.SaveUpdateCheck(ctx, latest, time.Now()); err != nil {
		return retry
	}
	return every
}

// latest asks GitHub for the newest release's version: "" when there is
// none, or when its name isn't a plain version.
func (c *Checker) latest(ctx context.Context) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint, nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "Sortwise/"+c.current)
	response, err := c.client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return "", nil // nothing published yet
	}
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub answered %s", response.Status)
	}
	var release struct {
		Tag string `json:"tag_name"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&release); err != nil {
		return "", errors.New("GitHub's answer could not be read")
	}
	version := strings.TrimPrefix(release.Tag, "v")
	if !plainVersion.MatchString(version) {
		return "", nil
	}
	return version, nil
}

var plainVersion = regexp.MustCompile(`^\d{1,4}\.\d{1,4}\.\d{1,4}$`)

// State reports the running version and any newer release found.
func (c *Checker) State(ctx context.Context) State {
	record := c.store.UpdateRecord(ctx)
	_, released := parse(c.current)
	state := State{Version: c.current, Released: released, Check: record.On}
	if !record.CheckedAt.IsZero() {
		state.CheckedAt = &record.CheckedAt
	}
	if released && record.On && Newer(c.current, record.Latest) {
		state.Available = &Release{Version: record.Latest, URL: releases + "/tag/v" + record.Latest}
	}
	return state
}

// SetCheck turns the daily check on or off; turning it on checks soon.
func (c *Checker) SetCheck(ctx context.Context, on bool) error {
	if err := c.store.SetUpdateCheck(ctx, on); err != nil {
		return err
	}
	if on {
		c.Wake()
	}
	return nil
}

// Newer reports whether latest is a later version than current. Either
// being something other than a plain version, such as "dev", makes it
// false.
func Newer(current, latest string) bool {
	have, ok := parse(current)
	want, ok2 := parse(latest)
	if !ok || !ok2 {
		return false
	}
	for index := range have {
		if want[index] != have[index] {
			return want[index] > have[index]
		}
	}
	return false
}

func parse(version string) ([3]int, bool) {
	var parts [3]int
	version = strings.TrimPrefix(version, "v")
	if !plainVersion.MatchString(version) {
		return parts, false
	}
	for index, part := range strings.Split(version, ".") {
		parts[index], _ = strconv.Atoi(part)
	}
	return parts, true
}
