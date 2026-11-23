// Package advisor holds the repository's changelog, embedded at build time so the app can show
// "What's new" with no network.
package advisor

import _ "embed"

// Changelog is CHANGELOG.md as it was when the binary was built.
//
//go:embed CHANGELOG.md
var Changelog string
