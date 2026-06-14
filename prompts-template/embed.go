// Package promptstemplate embeds the default oz flow prompt templates.
package promptstemplate

import "embed"

// FS contains the markdown templates installed by `oz flow install`.
//
//go:embed *.md
var FS embed.FS
