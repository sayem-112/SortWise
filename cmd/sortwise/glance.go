package main

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strconv"
	"sync"
	"time"

	"sortwise/internal/ai"
	"sortwise/internal/credentials"
	"sortwise/internal/flyout"
	"sortwise/internal/store"
)

// library is what the tray panel reads while Sortwise runs.
type library struct {
	db        *sql.DB
	store     *store.Store
	keys      credentials.Manager
	providers map[string]ai.Factory
	worker    *ai.Worker

	mu sync.Mutex
	// Reading keys from Windows Credential Manager is the slow part of a
	// status, so which providers have one is kept for a while.
	keysRead time.Time
	keyed    map[string]bool
	// lastGood is shown again when the library is too busy to read.
	lastGood *flyout.Status
}

// keysFreshFor is how long a reading of the keys is used again.
const keysFreshFor = 30 * time.Second

// status says what Sortwise is doing, as the app's sidebar does, with the
// library's size and what was saved today.
func (l *library) status() flyout.Status {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	glance, err := l.store.Glance(ctx, time.Now())
	config := ai.ReadConfiguration(ctx, l.db, func(name string) bool { return l.providers[name] != nil })
	l.mu.Lock()
	defer l.mu.Unlock()
	// ReadConfiguration falls back to defaults when it can't read, so a
	// timeout means its answer can't be trusted either.
	if err != nil || ctx.Err() != nil {
		if l.lastGood != nil {
			return *l.lastGood
		}
		return flyout.Status{Headline: "Can't read your library", Tone: flyout.Warning, Detail: "Open Sortwise to see what's wrong"}
	}

	detail := count(glance.Bookmarks, "bookmark") + " on this PC"
	if glance.SavedToday > 0 {
		detail += " · " + thousands(glance.SavedToday) + " saved today"
	}
	status := flyout.Status{Headline: "Up to date", Tone: flyout.Good, Detail: detail}
	ready := l.withKeys(config.Order())
	switch {
	case len(ready) == 0:
		status.Headline, status.Tone = "Needs an AI key to organize", flyout.Warning
	case config.Paused && glance.Waiting > 0:
		status.Headline, status.Tone = fmt.Sprintf("Paused, %s waiting", thousands(glance.Waiting)), flyout.Quiet
	case config.Paused:
		status.Headline, status.Tone = "Organizing paused", flyout.Quiet
	case glance.Waiting > 0 && l.allResting(ready):
		status.Headline, status.Tone = "Waiting for AI limits to reset", flyout.Warning
	case glance.Waiting > 0:
		status.Headline, status.Tone = "Organizing "+count(glance.Waiting, "new post"), flyout.Busy
	}
	l.lastGood = &status
	return status
}

// withKeys returns the providers among names that have a key, reading the
// keys again once the last reading is keysFreshFor old. l.mu is held.
func (l *library) withKeys(names []string) []string {
	if time.Since(l.keysRead) > keysFreshFor {
		l.keyed = map[string]bool{}
		for name := range l.providers {
			key, _, err := l.keys.Get(name)
			l.keyed[name] = err == nil && key != ""
		}
		l.keysRead = time.Now()
	}
	var ready []string
	for _, name := range names {
		if l.keyed[name] && l.providers[name] != nil {
			ready = append(ready, name)
		}
	}
	return ready
}

// allResting reports whether every one of providers is waiting for its
// limit to reset, so nothing can organize; while one rests, the backup
// carries on.
func (l *library) allResting(providers []string) bool {
	resting := []string{}
	for _, item := range l.worker.Resting() {
		resting = append(resting, item.Provider)
	}
	for _, name := range providers {
		if !slices.Contains(resting, name) {
			return false
		}
	}
	return true
}

func count(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return thousands(n) + " " + noun + "s"
}

// thousands writes n with commas, as the app does: 2,547.
func thousands(n int) string {
	digits := strconv.Itoa(n)
	for at := len(digits) - 3; at > 0 && digits[at-1] != '-'; at -= 3 {
		digits = digits[:at] + "," + digits[at:]
	}
	return digits
}
