// Package server embeds the built web UI.
package server

import "embed"

//go:embed all:dist
var distFS embed.FS
