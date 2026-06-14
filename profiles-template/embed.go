// Package profilestemplate embeds built-in oz flow workflow profile templates.
package profilestemplate

import "embed"

// FS contains YAML profile templates used by `oz flow config`.
//
//go:embed *.yaml
var FS embed.FS
