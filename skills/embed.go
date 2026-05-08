// Package skills embeds oz agent skill templates from the source tree.
package skills

import (
	"embed"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

// FS contains the source skill templates injected into the oz binary at build time.
//
//go:embed oz-*/SKILL.md
var FS embed.FS

type Skill struct {
	Name    string
	Content string
}

func BuiltIn() ([]Skill, error) {
	// BuiltIn returns embedded skills in deterministic name order.
	var out []Skill
	err := fs.WalkDir(FS, ".", func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || filepath.Base(path) != "SKILL.md" {
			return nil
		}
		data, err := FS.ReadFile(path)
		if err != nil {
			return err
		}
		out = append(out, Skill{
			Name:    strings.TrimSuffix(path, "/SKILL.md"),
			Content: string(data),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out, nil
}
