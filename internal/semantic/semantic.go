// Package semantic finds posts by what they are about. A background worker
// asks the AI service for one vector per post and keeps them in the library;
// a search turns its words into a vector the same way and ranks posts by how
// close theirs are. Comparing vectors happens on this computer, so showing a
// post's related posts sends nothing anywhere.
package semantic

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"sortwise/internal/ai"
	"sortwise/internal/credentials"
	"sortwise/internal/store"
)

const (
	// provider is the AI service whose key vectors are made with.
	provider = "gemini"
	// scan is how many posts one round compares with their vectors.
	scan = 200
	// searchLimit is the most posts a search adds for being close in meaning.
	searchLimit = 50
	// searchMargin and relatedMargin drop posts much further away than the
	// closest one, so a specific search or post does not pad itself out.
	searchMargin  = 0.1
	relatedMargin = 0.08
	// queryTimeout bounds the wait for a search's vector.
	queryTimeout = 8 * time.Second
	// maxRefusals is how many posts in a row may be refused alone before the
	// refusals are taken to be about the key or account instead.
	maxRefusals = 3
	// maxQueryRunes bounds the words of a search that are sent.
	maxQueryRunes = 200
	// timestamp is the form SQLite's CURRENT_TIMESTAMP writes.
	timestamp = "2006-01-02 15:04:05"
)

var (
	// ErrOff means search by meaning is turned off in Settings.
	ErrOff = errors.New("search by meaning is turned off")
	// ErrNoKey means there is no Gemini API key to make vectors with.
	ErrNoKey = errors.New("search by meaning needs a Gemini API key")
	// ErrNotReady means no post has been indexed yet.
	ErrNotReady = errors.New("no posts are indexed for search by meaning yet")
	// ErrResting means the AI service is limiting requests for now.
	ErrResting = errors.New("the AI service is limiting requests")
	// ErrPaused means AI is paused in Settings, so a search not made before
	// cannot be sent.
	ErrPaused = errors.New("AI is paused")
	// ErrUnknownQuery means the search has no saved vector and asking for
	// one was not allowed.
	ErrUnknownQuery = errors.New("this search has no saved vector")
)

// Service indexes posts in the background and answers searches.
type Service struct {
	store       *store.Store
	credentials credentials.Manager
	factory     ai.EmbedderFactory
	model       string

	// batch is how many posts one request carries; pause is the gap between
	// requests, which keeps a first index of a large library under the AI
	// service's per-minute limits; idle is the wait with nothing to do.
	batch       int
	pause, idle time.Duration

	mu          sync.Mutex
	embedder    ai.Embedder
	embedderKey string
	current     atomic.Pointer[snapshot]
	// restUntil is the unix nanoseconds until which no request is made.
	restUntil atomic.Int64
	// streak counts failed requests in a row.
	streak atomic.Int32
	// After a batch is refused, the next narrow posts go one at a time, to
	// find the one the AI service objects to. refusals counts posts refused
	// alone in a row: several means the trouble is not the posts.
	narrow, refusals int
}

func New(s *store.Store, manager credentials.Manager, factory ai.EmbedderFactory) *Service {
	return &Service{store: s, credentials: manager, factory: factory, model: ai.EmbeddingModel(), batch: 16, pause: 8 * time.Second, idle: 10 * time.Second}
}

// Status describes search by meaning for Settings.
type Status struct {
	// Enabled is the user's choice; Available is whether there is a key.
	Enabled   bool `json:"enabled"`
	Available bool `json:"available"`
	// Indexed posts out of Total that can be.
	Indexed int `json:"indexed"`
	Total   int `json:"total"`
	// RestingUntil is set while the AI service is limiting requests.
	RestingUntil *time.Time `json:"restingUntil,omitempty"`
}

func (s *Service) Status(ctx context.Context) Status {
	status := Status{Enabled: s.store.MeaningSearchEnabled(ctx), Available: s.key() != ""}
	status.Indexed, status.Total, _ = s.store.EmbeddingProgress(ctx, s.model)
	if until := time.Unix(0, s.restUntil.Load()); until.After(time.Now()) {
		status.RestingUntil = &until
	}
	return status
}

func (s *Service) key() string {
	key, _, err := s.credentials.Get(provider)
	if err != nil {
		return ""
	}
	return key
}

// Wake ends any rest and gives refused posts another try, for when the key
// or the settings change.
func (s *Service) Wake(ctx context.Context) {
	s.restUntil.Store(0)
	s.streak.Store(0)
	if err := s.store.ForgetRefusedEmbeddings(ctx); err != nil {
		slog.Debug("Refused posts could not be queued again", "error", err)
	}
}

func (s *Service) client(ctx context.Context) (ai.Embedder, error) {
	key := s.key()
	if key == "" {
		return nil, ErrNoKey
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.embedder == nil || s.embedderKey != key {
		embedder, err := s.factory(ctx, key)
		if err != nil {
			return nil, err
		}
		s.embedder, s.embedderKey = embedder, key
	}
	return s.embedder, nil
}

// Run indexes posts until ctx ends.
func (s *Service) Run(ctx context.Context) {
	for {
		busy, err := s.step(ctx)
		if ctx.Err() != nil {
			return
		}
		wait := s.idle
		if err != nil {
			slog.Debug("Indexing for search by meaning is waiting", "error", err)
		} else if busy {
			wait = s.pause
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
}

// step does one round: it compares a batch of posts with their vectors and
// asks for new vectors for some of those that changed. It reports whether
// more work is waiting.
func (s *Service) step(ctx context.Context) (bool, error) {
	if !s.store.MeaningSearchEnabled(ctx) || s.store.AIPaused(ctx) || time.Now().UnixNano() < s.restUntil.Load() {
		return false, nil
	}
	if s.key() == "" {
		return false, nil
	}
	// Taken before the posts are read, so a change made while their vectors
	// are being computed is noticed on the next round.
	checkedAt := time.Now().UTC().Format(timestamp)
	candidates, err := s.store.EmbeddingCandidates(ctx, s.model, scan)
	if err != nil {
		return false, err
	}
	unchanged := []int64{}
	saved := []store.SavedEmbedding{}
	changed := []store.EmbeddingSource{}
	for _, candidate := range candidates {
		switch {
		case candidate.Current:
			unchanged = append(unchanged, candidate.ID)
		case candidate.Text == "":
			// Nothing to describe: recorded, so it is not looked at every round.
			saved = append(saved, store.SavedEmbedding{ID: candidate.ID, Hash: candidate.Hash})
		default:
			changed = append(changed, candidate)
		}
	}
	if err := s.store.TouchEmbeddings(ctx, unchanged, checkedAt); err != nil {
		return false, err
	}
	size := s.batch
	if s.narrow > 0 {
		size = 1
	}
	take := changed[:min(size, len(changed))]
	if len(take) > 0 {
		vectors, err := s.embed(ctx, take)
		switch {
		case err == nil:
			s.streak.Store(0)
			s.refusals = 0
			s.narrow = max(s.narrow-1, 0)
			for index, vector := range vectors {
				saved = append(saved, store.SavedEmbedding{ID: take[index].ID, Hash: take[index].Hash, Vector: Quantize(vector)})
			}
		case !ai.EmbeddingRefused(err) || s.refusals >= maxRefusals:
			// Busy, unreachable, or a key problem. Post after post refused
			// alone is treated the same: it is not about the posts.
			s.refusals = 0
			return false, s.rest(err)
		case len(take) > 1:
			// The AI service will not take one of these posts: the next
			// rounds go one post at a time to find it.
			s.narrow = len(take)
			return true, nil
		default:
			// Found it. Recorded as refused, so it is not sent again until
			// its text changes or the key or settings do.
			slog.Warn("A post could not be indexed for search by meaning", "bookmark", take[0].ID, "error", err)
			saved = append(saved, store.SavedEmbedding{ID: take[0].ID, Hash: store.RefusedPrefix + take[0].Hash})
			s.refusals++
			s.narrow = 0
		}
	}
	if err := s.store.SaveEmbeddings(ctx, s.model, checkedAt, saved); err != nil {
		return false, err
	}
	return len(candidates) == scan || len(changed) > len(take), nil
}

func (s *Service) embed(ctx context.Context, posts []store.EmbeddingSource) ([][]float32, error) {
	embedder, err := s.client(ctx)
	if err != nil {
		return nil, err
	}
	texts := make([]string, len(posts))
	for index, post := range posts {
		texts[index] = post.Text
	}
	requestCtx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	return embedder.Embed(requestCtx, texts, false)
}

// rest stops requests for a while after a failure and returns the failure.
func (s *Service) rest(err error) error {
	streak := s.streak.Add(1) - 1
	s.restUntil.Store(time.Now().Add(ai.EmbeddingRest(err, int(streak))).UnixNano())
	return err
}

// fresh returns the vectors as they are now, loading them again when posts
// were indexed or deleted since the last look.
func (s *Service) fresh(ctx context.Context) (*snapshot, error) {
	count, version, err := s.store.EmbeddingSignature(ctx, s.model)
	if err != nil {
		return nil, err
	}
	if loaded := s.current.Load(); loaded != nil && loaded.count == count && loaded.version == version {
		return loaded, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if loaded := s.current.Load(); loaded != nil && loaded.count == count && loaded.version == version {
		return loaded, nil
	}
	next := &snapshot{count: count, version: version, size: ai.EmbeddingDimensions, at: map[int64]int{}}
	err = s.store.LoadEmbeddings(ctx, s.model, func(id int64, packed []byte) {
		if len(packed) != next.size {
			return
		}
		vector := signed(packed)
		next.at[id] = len(next.ids)
		next.ids = append(next.ids, id)
		next.vectors = append(next.vectors, vector...)
		next.lengths = append(next.lengths, length(vector))
	})
	if err != nil {
		return nil, err
	}
	s.current.Store(next)
	return next, nil
}

func (s *Service) queryHash(text string) string {
	sum := sha256.Sum256([]byte(s.model + "\nquery\n" + text))
	return hex.EncodeToString(sum[:16])
}

// queryVector returns the vector for the words searched for. A search made
// before is answered from the library; a new one asks the AI service, when
// remote allows it.
func (s *Service) queryVector(ctx context.Context, text string, remote bool) ([]int8, error) {
	hash := s.queryHash(text)
	if packed, ok := s.store.QueryVector(ctx, hash, remote); ok && len(packed) == ai.EmbeddingDimensions {
		return signed(packed), nil
	}
	if !remote {
		return nil, ErrUnknownQuery
	}
	if s.store.AIPaused(ctx) {
		return nil, ErrPaused
	}
	if time.Now().UnixNano() < s.restUntil.Load() {
		return nil, ErrResting
	}
	embedder, err := s.client(ctx)
	if err != nil {
		return nil, err
	}
	requestCtx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()
	vectors, err := embedder.Embed(requestCtx, []string{text}, true)
	if err != nil {
		if ctx.Err() != nil {
			// The person moved on before the answer came; nothing is wrong.
			return nil, ctx.Err()
		}
		// One slow answer is not a reason to stop everything.
		if !ai.EmbeddingRefused(err) && !errors.Is(err, context.DeadlineExceeded) {
			_ = s.rest(err)
		}
		return nil, err
	}
	s.streak.Store(0)
	packed := Quantize(vectors[0])
	if len(packed) != ai.EmbeddingDimensions {
		return nil, errors.New("the AI service returned a vector of the wrong size")
	}
	if err := s.store.SaveQueryVector(ctx, hash, packed); err != nil {
		slog.Debug("A search vector could not be saved", "error", err)
	}
	return signed(packed), nil
}

// normalize makes searches that differ only in case or spacing the same.
func normalize(text string) string {
	return strings.ToLower(strings.Join(strings.Fields(text), " "))
}

// Search returns the posts closest in meaning to the words, closest first.
// It asks the AI service for the words' vector unless that search was made
// before.
func (s *Service) Search(ctx context.Context, text string) ([]int64, error) {
	return s.search(ctx, text, true)
}

// Recall is Search for a search made before: it never asks the AI service,
// and returns ErrUnknownQuery when the words have no saved vector.
func (s *Service) Recall(ctx context.Context, text string) ([]int64, error) {
	return s.search(ctx, text, false)
}

func (s *Service) search(ctx context.Context, text string, remote bool) ([]int64, error) {
	text = normalize(text)
	if text == "" {
		return nil, nil
	}
	if runes := []rune(text); len(runes) > maxQueryRunes {
		text = string(runes[:maxQueryRunes])
	}
	if !s.store.MeaningSearchEnabled(ctx) {
		return nil, ErrOff
	}
	vectors, err := s.fresh(ctx)
	if err != nil {
		return nil, err
	}
	if len(vectors.ids) == 0 {
		if s.key() == "" {
			return nil, ErrNoKey
		}
		return nil, ErrNotReady
	}
	query, err := s.queryVector(ctx, text, remote)
	if err != nil {
		return nil, err
	}
	hits := vectors.nearest(query, searchLimit, 0, searchMargin)
	ids := make([]int64, len(hits))
	for position, hit := range hits {
		ids[position] = hit.ID
	}
	return ids, nil
}

// Related returns up to limit posts about the same thing as the given one,
// closest first. It is empty when the post is not indexed, or nothing in
// the library is about the same thing.
func (s *Service) Related(ctx context.Context, id int64, limit int) ([]int64, error) {
	if !s.store.MeaningSearchEnabled(ctx) {
		return nil, nil
	}
	vectors, err := s.fresh(ctx)
	if err != nil {
		return nil, err
	}
	index, ok := vectors.at[id]
	if !ok {
		return nil, nil
	}
	hits := vectors.nearest(vectors.vector(index), limit, id, relatedMargin)
	ids := make([]int64, len(hits))
	for position, hit := range hits {
		ids[position] = hit.ID
	}
	return ids, nil
}
