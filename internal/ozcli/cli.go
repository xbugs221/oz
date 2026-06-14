// Package ozcli implements the standalone oz CLI entrypoint and command dispatch.
package ozcli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"github.com/xbugs221/oz/internal/app"
)

func Main(args []string, stdout, stderr io.Writer) int {
	// Main maps CLI arguments to process exit codes for shell and agent use.
	return (&cli{out: stdout, err: stderr, now: time.Now}).run(args)
}

func (c *cli) run(args []string) int {
	// run dispatches the small non-interactive command surface.
	if c.now == nil {
		c.now = time.Now
	}
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		c.printHelp()
		return 0
	}
	if args[0] == "--version" || args[0] == "-v" {
		fmt.Fprintln(c.out, resolvedVersion())
		return 0
	}
	var err error
	switch args[0] {
	case "list", "l":
		err = c.listCmd(args[1:])
	case "install", "i":
		err = c.installCmd(args[1:])
	case "create":
		err = c.createCmd(args[1:])
	case "status":
		err = c.statusCmd(args[1:])
	case "validate":
		err = c.validateCmd(args[1:])
	case "archive":
		err = c.archiveCmd(args[1:])
	case "flow":
		err = app.Run(args[1:], os.Stdin, c.out, c.err)
	default:
		err = fmt.Errorf("未知命令：%s", args[0])
	}
	if err != nil {
		fmt.Fprintln(c.err, err)
		return 1
	}
	return 0
}

func (c *cli) printHelp() {
	// printHelp separates user commands from automation interfaces.
	fmt.Fprintln(c.out, "oz "+resolvedVersion())
	fmt.Fprintln(c.out, "")
	fmt.Fprintln(c.out, "日常命令：")
	fmt.Fprintln(c.out, "  list | l [--json]")
	fmt.Fprintln(c.out, "  install | i [--global | -g]")
	fmt.Fprintln(c.out, "  flow <status|watch|run|config|clean|...>")
	fmt.Fprintln(c.out, "")
	fmt.Fprintln(c.out, "自动化接口：")
	fmt.Fprintln(c.out, "  create")
	fmt.Fprintln(c.out, "  status <change> [--json]")
	fmt.Fprintln(c.out, "  validate <change> [--json]")
	fmt.Fprintln(c.out, "  archive <change> --yes")
}

func resolvedVersion() string {
	// resolvedVersion reports the release tag when oz was installed or run from the source repository.
	if version != "" && version != "dev" {
		return version
	}
	if tag, err := sourceGitTag(); err == nil && tag != "" {
		return tag
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return version
}

func sourceGitTag() (string, error) {
	// sourceGitTag reads git describe only when the current repository is oz itself.
	root, err := sourceRoot()
	if err != nil {
		return "", err
	}
	describeCmd := exec.Command("git", "-C", root, "describe", "--tags", "--always", "--dirty")
	describeOut, err := describeCmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(describeOut)), nil
}

func sourceRoot() (string, error) {
	// sourceRoot locates the oz checkout from compiled source paths before consulting the current directory.
	if _, file, _, ok := runtime.Caller(0); ok {
		if root, err := ozModuleRoot(filepath.Dir(file)); err == nil {
			return root, nil
		}
	}
	rootCmd := exec.Command("git", "rev-parse", "--show-toplevel")
	rootOut, err := rootCmd.Output()
	if err != nil {
		return "", err
	}
	return ozModuleRoot(strings.TrimSpace(string(rootOut)))
}

func ozModuleRoot(start string) (string, error) {
	// ozModuleRoot walks upward until it finds the oz module root.
	for dir := start; ; dir = filepath.Dir(dir) {
		data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
		if err == nil && strings.Contains(string(data), "module github.com/xbugs221/oz\n") {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("oz module root not found")
		}
	}
}
