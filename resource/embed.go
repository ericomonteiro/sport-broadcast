// Package resource embeds the static assets shipped with the binary.
package resource

import "embed"

//go:embed icons/*.svg
var Icons embed.FS
