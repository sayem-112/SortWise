package migrations

import "embed"

// Files contains the application's versioned SQLite migrations.
//
//go:embed *.sql
var Files embed.FS
