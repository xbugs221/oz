// Package main provides the root oz command for `go install github.com/xbugs221/oz@latest`.
package main

import (
	"os"

	"github.com/xbugs221/oz/internal/ozcli"
)

// main delegates root-package installs to the oz CLI implementation.
func main() {
	os.Exit(ozcli.Main(os.Args[1:], os.Stdout, os.Stderr))
}
