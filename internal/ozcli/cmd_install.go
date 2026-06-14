// Package ozcli implements the standalone oz skill installation command.
package ozcli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/xbugs221/oz/skills"
)

func (c *cli) installCmd(args []string) error {
	// installCmd installs built-in agent skills into a project or user skill directory.
	if hasHelp(args) {
		c.printInstallHelp()
		return nil
	}
	global := hasArg(args, "--global") || hasArg(args, "-g")
	if len(args) > 1 || (len(args) == 1 && !global) {
		return errors.New("用法：oz install [--global | -g]")
	}
	targetRoot := filepath.Join(".agents", "skills")
	if global {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		targetRoot = filepath.Join(home, ".agents", "skills")
	}
	builtIn, err := skills.BuiltIn()
	if err != nil {
		return err
	}
	for _, skill := range builtIn {
		target := filepath.Join(targetRoot, skill.Name, "SKILL.md")
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, []byte(skill.Content), 0o644); err != nil {
			return err
		}
	}
	fmt.Fprintf(c.out, "已安装 %d 个技能到 %s\n", len(builtIn), targetRoot)
	return nil
}

func (c *cli) printInstallHelp() {
	// printInstallHelp documents local and global skill installation forms.
	fmt.Fprintln(c.out, "用法：")
	fmt.Fprintln(c.out, "  oz install [--global | -g]")
	fmt.Fprintln(c.out, "  oz i [--global | -g]")
}
