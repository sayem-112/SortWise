package ai

import (
	"context"
	"database/sql"
)

// DefaultLanguage is used for summaries and topic tags until the user picks
// another in Settings.
const DefaultLanguage = "English"

// Languages lists the choices offered for summaries and topic tags. Posts in
// any language are understood; this only sets the language the library is
// written in, so tags stay consistent and searchable.
var Languages = []string{
	"English", "Arabic", "Bengali", "Chinese (Simplified)", "Chinese (Traditional)", "Dutch", "French", "German",
	"Hindi", "Indonesian", "Italian", "Japanese", "Korean", "Polish", "Portuguese", "Russian", "Spanish",
	"Thai", "Turkish", "Ukrainian", "Vietnamese",
}

func SupportedLanguage(value string) bool {
	for _, language := range Languages {
		if language == value {
			return true
		}
	}
	return false
}

// ReadLanguage returns the saved AI language, or English.
func ReadLanguage(ctx context.Context, db *sql.DB) string {
	var value string
	if err := db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key='ai_language'`).Scan(&value); err == nil && SupportedLanguage(value) {
		return value
	}
	return DefaultLanguage
}
