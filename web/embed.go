package web

import "embed"

// Dist holds the built upstream frontend (see web/README.md). An empty dist
// simply disables the built-in UI.
//
//go:embed all:dist
var Dist embed.FS
