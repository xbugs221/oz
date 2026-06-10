// Package profilestemplate embeds built-in wo workflow profile templates.
package profilestemplate

import "embed"

// FS contains YAML profile templates used by `wo config`.
//
//go:embed *.yaml
var FS embed.FS
