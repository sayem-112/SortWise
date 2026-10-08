package ai

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"sortwise/internal/credentials"
)

type Worker struct {
	db                 *sql.DB
	credentials        credentials.Manager
	factories          map[string]Factory
	pollInterval       time.Duration
	minRequestInterval time.Duration
	lastRequestAt      time.Time
	// cooldowns holds, per provider, the unix nanoseconds until which it is
	// resting after a rate limit, daily quota, outage, or bad key. The map is
	// fixed at creation; HTTP handlers clear the values through Unblock.
	cooldowns map[string]*atomic.Int64
	// transientStreak counts consecutive provider outages to grow the backoff.
	transientStreak int
	fetcher         PostFetcher
	// lastHydrationSweep throttles the background fill-in of unreadable posts.
	lastHydrationSweep time.Time
}

func NewWorker(db *sql.DB, manager credentials.Manager, factory Factory) *Worker {
	return NewWorkerWithProviders(db, manager, map[string]Factory{DefaultProvider: factory})
}

func NewWorkerWithProviders(db *sql.DB, manager credentials.Manager, factories map[string]Factory) *Worker {
	cooldowns := map[string]*atomic.Int64{}
	for name := range factories {
		cooldowns[name] = &atomic.Int64{}
	}
	return &Worker{db: db, credentials: manager, factories: factories, cooldowns: cooldowns, pollInterval: 2 * time.Second, minRequestInterval: 2 * time.Second}
}

// Configuration is the saved AI choice: the main provider, whether the other
// provider takes over while the main one is resting, and whether AI is paused.
type Configuration struct {
	Provider string
	Backup   bool
	Paused   bool
}

// ReadConfiguration reads the AI settings, defaulting to Gemini with backup on.
func ReadConfiguration(ctx context.Context, db *sql.DB, known func(string) bool) Configuration {
	config := Configuration{Provider: DefaultProvider, Backup: true}
	var value string
	if err := db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key='ai_provider'`).Scan(&value); err == nil && known(value) {
		config.Provider = value
	}
	if err := db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key='ai_backup'`).Scan(&value); err == nil {
		config.Backup = value != "false"
	}
	if err := db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key='ai_paused'`).Scan(&value); err == nil {
		config.Paused = value == "true"
	}
	return config
}

func (w *Worker) cooldown(provider string) *atomic.Int64 {
	if value := w.cooldowns[provider]; value != nil {
		return value
	}
	return &atomic.Int64{}
}

// candidates lists the providers to try in order: the main one, then the
// others when backup is on.
func (w *Worker) candidates(config Configuration) []string {
	names := []string{config.Provider}
	if config.Backup {
		for _, name := range []string{"gemini", "groq"} {
			if name != config.Provider && w.factories[name] != nil {
				names = append(names, name)
			}
		}
	}
	return names
}

type providerChoice struct {
	name, key string
	// resting is true when a configured provider exists but every one is
	// resting; unconfigured is true when no candidate has a key.
	resting, unconfigured bool
}

func (w *Worker) choose(ctx context.Context, config Configuration) providerChoice {
	now := time.Now().UnixNano()
	choice := providerChoice{unconfigured: true}
	for _, name := range w.candidates(config) {
		key, _, err := w.credentials.Get(name)
		if err != nil || key == "" || w.factories[name] == nil {
			continue
		}
		choice.unconfigured = false
		if now < w.cooldown(name).Load() {
			choice.resting = true
			continue
		}
		return providerChoice{name: name, key: key}
	}
	return choice
}

// ProviderStatus reports, for Settings, which provider is resting and until when.
type ProviderStatus struct {
	Provider     string    `json:"provider"`
	RestingUntil time.Time `json:"restingUntil"`
}

func (w *Worker) Resting() []ProviderStatus {
	now := time.Now()
	statuses := []ProviderStatus{}
	for _, name := range []string{"gemini", "groq"} {
		if until := time.Unix(0, w.cooldown(name).Load()); until.After(now) {
			statuses = append(statuses, ProviderStatus{Provider: name, RestingUntil: until})
		}
	}
	return statuses
}

func (w *Worker) Run(ctx context.Context) {
	for {
		w.hydratePending(ctx)
		processed, _ := w.ProcessOne(ctx)
		wait := w.pollInterval
		if processed {
			wait = 50 * time.Millisecond
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
}

type claimedJob struct {
	ID, BookmarkID        int64
	InputHash             string
	Attempts, MaxAttempts int
}

func (w *Worker) claim(ctx context.Context) (*claimedJob, error) {
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	now := time.Now().UTC()
	_, _ = tx.ExecContext(ctx, `UPDATE jobs SET status='pending',lease_until=NULL WHERE status='processing' AND lease_until < ?`, now.Format(time.RFC3339))
	var job claimedJob
	err = tx.QueryRowContext(ctx, `SELECT id,bookmark_id,input_hash,attempts,max_attempts FROM jobs WHERE status IN ('pending','blocked') AND available_at <= ? ORDER BY (input_hash LIKE '%:manual:%') DESC,bookmark_id DESC LIMIT 1`, now.Format(time.RFC3339)).Scan(&job.ID, &job.BookmarkID, &job.InputHash, &job.Attempts, &job.MaxAttempts)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE jobs SET status='processing',attempts=attempts+1,lease_until=?,last_error='',updated_at=CURRENT_TIMESTAMP WHERE id=? AND status IN ('pending','blocked')`, now.Add(2*time.Minute).Format(time.RFC3339), job.ID)
	if err != nil {
		return nil, err
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		return nil, nil
	}
	job.Attempts++
	if _, err = tx.ExecContext(ctx, `UPDATE bookmarks SET processing_status='processing' WHERE id=?`, job.BookmarkID); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return &job, nil
}

func (w *Worker) loadInput(ctx context.Context, job *claimedJob) (AnalysisInput, error) {
	var input AnalysisInput
	input.BookmarkID = job.BookmarkID
	input.InputHash = job.InputHash
	input.Language = ReadLanguage(ctx, w.db)
	var visible string
	err := w.db.QueryRowContext(ctx, `SELECT text,author,username,visible_context_json FROM bookmarks WHERE id=?`, job.BookmarkID).Scan(&input.Text, &input.Author, &input.Username, &visible)
	if err != nil {
		return input, err
	}
	var contextData struct {
		QuotedPost *struct {
			Text string `json:"text"`
		} `json:"quotedPost"`
		Card *struct {
			Title       string `json:"title"`
			Description string `json:"description"`
		} `json:"card"`
	}
	_ = json.Unmarshal([]byte(visible), &contextData)
	if contextData.QuotedPost != nil {
		input.QuotedText = contextData.QuotedPost.Text
	}
	if contextData.Card != nil {
		input.CardTitle = contextData.Card.Title
		input.CardDescription = contextData.Card.Description
	}
	rows, err := w.db.QueryContext(ctx, `SELECT id,name FROM categories WHERE active=1 ORDER BY name`)
	if err != nil {
		return input, err
	}
	for rows.Next() {
		var category CategoryOption
		if err := rows.Scan(&category.ID, &category.Name); err != nil {
			rows.Close()
			return input, err
		}
		input.Categories = append(input.Categories, category)
	}
	rows.Close()
	rows, err = w.db.QueryContext(ctx, `SELECT t.name FROM tags t JOIN bookmark_tags bt ON bt.tag_id=t.id WHERE bt.manual_state <> 'removed' AND (bt.manual_state='added' OR bt.ai_confidence IS NOT NULL) GROUP BY t.id ORDER BY COUNT(*) DESC,t.name LIMIT ?`, maxVocabularyTags)
	if err != nil {
		return input, err
	}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return input, err
		}
		input.ExistingTags = append(input.ExistingTags, name)
	}
	rows.Close()
	rows, err = w.db.QueryContext(ctx, `SELECT kind,url,alt_text FROM media WHERE bookmark_id=? ORDER BY position LIMIT 4`, job.BookmarkID)
	if err != nil {
		return input, err
	}
	for rows.Next() {
		var media MediaInput
		if err := rows.Scan(&media.Kind, &media.URL, &media.AltText); err != nil {
			rows.Close()
			return input, err
		}
		input.Media = append(input.Media, media)
	}
	rows.Close()
	return input, nil
}

func (w *Worker) ProcessOne(ctx context.Context) (bool, error) {
	config := ReadConfiguration(ctx, w.db, func(name string) bool { return w.factories[name] != nil })
	if config.Paused {
		return false, nil
	}
	choice := w.choose(ctx, config)
	if choice.resting {
		return false, nil
	}
	job, err := w.claim(ctx)
	if err != nil || job == nil {
		return false, err
	}
	if choice.unconfigured {
		w.block(ctx, job, "Add a "+ProviderName(config.Provider)+" API key in Settings to organize bookmarks")
		return true, nil
	}
	providerName, key := choice.name, choice.key
	input, err := w.loadInput(ctx, job)
	if err != nil {
		w.fail(ctx, job, err)
		return true, err
	}
	if needsHydration(input) && w.hydrate(ctx, job.BookmarkID) {
		if input, err = w.loadInput(ctx, job); err != nil {
			w.fail(ctx, job, err)
			return true, err
		}
	}
	if nothingToAnalyze(input) {
		// The extension captured no text or media for this post; asking a model would
		// only produce an empty analysis. Finish it so it lands in Needs review.
		w.completeEmpty(ctx, job)
		return true, nil
	}
	factory := w.factories[providerName]
	if factory == nil {
		w.block(ctx, job, "Selected AI provider is unavailable")
		return true, nil
	}
	provider, err := factory(ctx, key)
	if err == nil {
		if err = w.waitForRequestSlot(ctx, providerName); err != nil {
			w.block(ctx, job, "AI processing stopped")
			return true, err
		}
		var result AnalysisResult
		result, err = provider.Analyze(ctx, input)
		if err == nil {
			w.transientStreak = 0
			model := result.Model
			if model == "" {
				model = ModelFor(providerName)
			}
			err = w.complete(ctx, job, input, result, providerName, model)
		}
	}
	if err != nil {
		if errors.Is(err, ErrDailyQuota) {
			w.rest(ctx, job, config, providerName, NextQuotaReset(time.Now()), err)
			return true, err
		}
		if isRateLimitError(err) {
			w.rest(ctx, job, config, providerName, time.Now().Add(retryDelay(err)), err)
			return true, err
		}
		if isTransientError(err) {
			w.rest(ctx, job, config, providerName, time.Now().Add(w.outageDelay()), err)
			return true, err
		}
		if isConfigurationError(err) {
			// A bad key or retired model fails every job the same way: rest this
			// provider instead of spending each bookmark's attempts.
			w.cooldown(providerName).Store(time.Now().Add(configurationCooldown).UnixNano())
			if w.choose(ctx, config).name != "" {
				w.requeue(ctx, job, time.Now(), err)
			} else {
				w.block(ctx, job, truncateError(err))
			}
			return true, err
		}
		w.fail(ctx, job, err)
		return true, err
	}
	return true, nil
}

// geminiRequestInterval keeps Gemini under its free-tier 15 requests per minute,
// so the queue runs steadily instead of bursting into rate limits.
const geminiRequestInterval = 4200 * time.Millisecond

func (w *Worker) waitForRequestSlot(ctx context.Context, provider string) error {
	interval := w.minRequestInterval
	if provider == "gemini" && interval > 0 && interval < geminiRequestInterval {
		interval = geminiRequestInterval
	}
	readyAt := w.lastRequestAt.Add(interval)
	if cooldown := time.Unix(0, w.cooldown(provider).Load()); cooldown.After(readyAt) {
		readyAt = cooldown
	}
	if wait := time.Until(readyAt); wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}
	w.lastRequestAt = time.Now()
	return nil
}

func isRateLimitError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "429") || strings.Contains(message, "resource_exhausted") || strings.Contains(message, "rate limit") || strings.Contains(message, "quota exceeded")
}

const configurationCooldown = 10 * time.Minute

// isOverloadError matches a provider saying it is temporarily over capacity.
func isOverloadError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	for _, marker := range []string{"error 503", "503 service unavailable", "unavailable", "overloaded", "high demand"} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

// isTransientError matches outages and network failures that say nothing about
// the bookmark itself, so they must not spend its attempts.
func isTransientError(err error) bool {
	if err == nil {
		return false
	}
	if isOverloadError(err) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	message := strings.ToLower(err.Error())
	for _, marker := range []string{"error 500", "error 502", "error 504", "500 internal server error", "502 bad gateway", "504 gateway timeout", "internal error", "deadline exceeded", "timeout", "connection reset", "connection refused", "eof", "no such host"} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

// rest pauses one provider until the given time without spending the job's
// attempt. When another provider can take over, the job goes straight back to
// the queue for it; otherwise it waits until this provider is ready again.
func (w *Worker) rest(ctx context.Context, job *claimedJob, config Configuration, provider string, until time.Time, cause error) {
	w.cooldown(provider).Store(until.UnixNano())
	available := until
	if w.choose(ctx, config).name != "" {
		available = time.Now()
	}
	w.requeue(ctx, job, available, cause)
}

// outageDelay grows while outages continue: 30s, doubling, capped at 10 minutes.
func (w *Worker) outageDelay() time.Duration {
	delay := 30 * time.Second << min(w.transientStreak, 5)
	if delay > 10*time.Minute {
		delay = 10 * time.Minute
	}
	w.transientStreak++
	return delay
}

func (w *Worker) requeue(ctx context.Context, job *claimedJob, available time.Time, cause error) {
	_, _ = w.db.ExecContext(ctx, `UPDATE jobs SET status='pending',attempts=MAX(attempts-1,0),available_at=?,lease_until=NULL,last_error=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, available.UTC().Format(time.RFC3339), truncateError(cause), job.ID)
	_, _ = w.db.ExecContext(ctx, `UPDATE bookmarks SET processing_status='pending' WHERE id=?`, job.BookmarkID)
}

func isConfigurationError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	for _, marker := range []string{"401 unauthorized", "403 forbidden", "404 not found", "model_not_found", "invalid_api_key", "api key not valid", "permission_denied", "unauthenticated", "error 401", "error 403", "error 404", "not_found", "decommissioned", "does not exist"} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

var retryInPattern = regexp.MustCompile(`(?i)retry in\s+([0-9]+(?:\.[0-9]+)?)\s*(ms|s|m|h)`) // Gemini includes this hint in SDK errors.

func retryDelay(err error) time.Duration {
	if err != nil {
		if match := retryInPattern.FindStringSubmatch(err.Error()); len(match) == 3 {
			value, parseErr := strconv.ParseFloat(match[1], 64)
			if parseErr == nil {
				unit := time.Second
				switch strings.ToLower(match[2]) {
				case "ms":
					unit = time.Millisecond
				case "m":
					unit = time.Minute
				case "h":
					unit = time.Hour
				}
				delay := time.Duration(value * float64(unit))
				if delay > 0 {
					return delay + 2*time.Second
				}
			}
		}
	}
	return 15 * time.Minute
}

func (w *Worker) block(ctx context.Context, job *claimedJob, message string) {
	// Blocked jobs wait instead of being re-claimed in a tight loop; saving a key,
	// switching provider, or resuming calls Unblock to release them immediately.
	_, _ = w.db.ExecContext(ctx, `UPDATE jobs SET status='blocked',attempts=MAX(attempts-1,0),available_at=?,lease_until=NULL,last_error=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, time.Now().UTC().Add(configurationCooldown).Format(time.RFC3339), message, job.ID)
	_, _ = w.db.ExecContext(ctx, `UPDATE bookmarks SET processing_status='blocked' WHERE id=?`, job.BookmarkID)
}

func (w *Worker) fail(ctx context.Context, job *claimedJob, cause error) {
	status := "pending"
	if job.Attempts >= job.MaxAttempts || errors.Is(cause, ErrInvalidResult) {
		status = "failed"
	}
	delay := time.Duration(math.Min(math.Pow(2, float64(job.Attempts))*5, 900))*time.Second + time.Duration(rand.IntN(1000))*time.Millisecond
	_, _ = w.db.ExecContext(ctx, `UPDATE jobs SET status=?,available_at=?,lease_until=NULL,last_error=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, status, time.Now().UTC().Add(delay).Format(time.RFC3339), truncateError(cause), job.ID)
	bookmarkStatus := status
	if status == "pending" {
		bookmarkStatus = "pending"
	}
	_, _ = w.db.ExecContext(ctx, `UPDATE bookmarks SET processing_status=? WHERE id=?`, bookmarkStatus, job.BookmarkID)
}

func truncateError(err error) string {
	if err == nil {
		return ""
	}
	value := strings.TrimSpace(err.Error())
	if len(value) > 500 {
		value = value[:500]
	}
	return value
}

func (w *Worker) complete(ctx context.Context, job *claimedJob, input AnalysisInput, result AnalysisResult, provider, model string) error {
	if err := ValidateResult(input, &result); err != nil {
		return err
	}
	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO enrichments(bookmark_id,ai_summary,media_description,provider,model,prompt_version,input_hash,analyzed_at) VALUES (?,?,?,?,?,?,?,CURRENT_TIMESTAMP) ON CONFLICT(bookmark_id) DO UPDATE SET ai_summary=excluded.ai_summary,media_description=excluded.media_description,provider=excluded.provider,model=excluded.model,prompt_version=excluded.prompt_version,input_hash=excluded.input_hash,analyzed_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP`, job.BookmarkID, result.Summary, result.MediaDescription, provider, model, PromptVersion, input.InputHash)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE bookmark_categories SET ai_confidence=NULL WHERE bookmark_id=? AND manual_state='automatic'`, job.BookmarkID)
	if err != nil {
		return err
	}
	for _, classification := range result.Categories {
		var categoryID int64
		err = tx.QueryRowContext(ctx, `SELECT id FROM categories WHERE active=1 AND lower(name)=lower(?)`, classification.Name).Scan(&categoryID)
		if err != nil {
			return fmt.Errorf("category changed during analysis: %w", err)
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO bookmark_categories(bookmark_id,category_id,ai_confidence) VALUES (?,?,?) ON CONFLICT(bookmark_id,category_id) DO UPDATE SET ai_confidence=excluded.ai_confidence,updated_at=CURRENT_TIMESTAMP`, job.BookmarkID, categoryID, classification.Confidence)
		if err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, `UPDATE bookmark_tags SET ai_confidence=NULL WHERE bookmark_id=? AND manual_state='automatic'`, job.BookmarkID)
	if err != nil {
		return err
	}
	for _, classification := range result.Tags {
		normalized := NormalizeTag(classification.Name)
		tagID, err := findTag(ctx, tx, normalized)
		if errors.Is(err, sql.ErrNoRows) {
			created, createErr := tx.ExecContext(ctx, `INSERT INTO tags(name,normalized_name,kind) VALUES (?,?,?)`, classification.Name, normalized, normalizeKind(classification.Kind))
			if createErr != nil {
				return createErr
			}
			tagID, _ = created.LastInsertId()
		} else if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO bookmark_tags(bookmark_id,tag_id,ai_confidence) VALUES (?,?,?) ON CONFLICT(bookmark_id,tag_id) DO UPDATE SET ai_confidence=excluded.ai_confidence,updated_at=CURRENT_TIMESTAMP`, job.BookmarkID, tagID, classification.Confidence)
		if err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, `UPDATE jobs SET status='completed',lease_until=NULL,last_error='',updated_at=CURRENT_TIMESTAMP WHERE id=?`, job.ID)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE bookmarks SET processing_status='completed',updated_at=CURRENT_TIMESTAMP WHERE id=?`, job.BookmarkID)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// findTag resolves a tag by name or alias, treating singular and plural
// spellings ("llm" and "llms") as the same tag.
func findTag(ctx context.Context, tx *sql.Tx, normalized string) (int64, error) {
	for _, variant := range TagVariants(normalized) {
		var id int64
		err := tx.QueryRowContext(ctx, `SELECT t.id FROM tags t LEFT JOIN tag_aliases a ON a.tag_id=t.id WHERE t.normalized_name=? OR a.normalized_alias=? LIMIT 1`, variant, variant).Scan(&id)
		if err == nil {
			return id, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return 0, err
		}
	}
	return 0, sql.ErrNoRows
}

func nothingToAnalyze(input AnalysisInput) bool {
	return strings.TrimSpace(input.Text) == "" && strings.TrimSpace(input.QuotedText) == "" &&
		strings.TrimSpace(input.CardTitle) == "" && strings.TrimSpace(input.CardDescription) == "" && len(input.Media) == 0
}

func (w *Worker) completeEmpty(ctx context.Context, job *claimedJob) {
	_, _ = w.db.ExecContext(ctx, `UPDATE jobs SET status='completed',lease_until=NULL,last_error='No text or media was captured for this post, so it was not sent to AI.',updated_at=CURRENT_TIMESTAMP WHERE id=?`, job.ID)
	_, _ = w.db.ExecContext(ctx, `UPDATE bookmarks SET processing_status='completed',updated_at=CURRENT_TIMESTAMP WHERE id=?`, job.BookmarkID)
}

func (w *Worker) RetryFailed(ctx context.Context, jobID int64) error {
	result, err := w.db.ExecContext(ctx, `UPDATE jobs SET status='pending',attempts=0,available_at=CURRENT_TIMESTAMP,last_error='',lease_until=NULL,updated_at=CURRENT_TIMESTAMP WHERE id=? AND status='failed'`, jobID)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		return sql.ErrNoRows
	}
	_, _ = w.db.ExecContext(ctx, `UPDATE bookmarks SET processing_status='pending' WHERE id=(SELECT bookmark_id FROM jobs WHERE id=?)`, jobID)
	return nil
}

func (w *Worker) Unblock(ctx context.Context) {
	for _, cooldown := range w.cooldowns {
		cooldown.Store(0)
	}
	_, _ = w.db.ExecContext(ctx, `UPDATE jobs SET status='pending',available_at=CURRENT_TIMESTAMP,last_error='',updated_at=CURRENT_TIMESTAMP WHERE status='blocked'`)
	_, _ = w.db.ExecContext(ctx, `UPDATE bookmarks SET processing_status='pending' WHERE processing_status='blocked'`)
}
