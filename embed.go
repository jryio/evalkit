// Package evalkit embeds the files the CLI hands out, so a `go install`ed binary
// needs no checkout: the agent skill, the suite reference and the example suites.
package evalkit

import "embed"

//go:embed skill/SKILL.md docs/schema.md examples/starter examples/kitchen-sink
var Files embed.FS
