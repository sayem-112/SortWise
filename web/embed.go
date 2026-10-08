package webassets

import "embed"

// Assets contains the production web application.
//
//go:embed dist
var Assets embed.FS
