// Package web holds the review UI. The binary embeds it; serve --web <dir>
// serves a checkout instead, so UI edits show on reload without a rebuild.
package web

import "embed"

//go:embed index.html icon.svg favicon.ico apple-touch-icon.png kit styles vendor
var FS embed.FS
