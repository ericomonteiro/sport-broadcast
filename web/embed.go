// Package web embeds the HTML pages into the binary.
package web

import "embed"

//go:embed index.html overlay.html control.html
var Files embed.FS
