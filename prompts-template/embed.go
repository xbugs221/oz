// Package promptstemplate embeds the default wo prompt templates.
package promptstemplate

import "embed"

// FS contains the markdown templates installed by `wo install`.
//
//go:embed *.md
var FS embed.FS
