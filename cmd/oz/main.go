// Package main provides the cmd/oz entrypoint used by local builds and release automation.
package main

import (
	"os"

	"github.com/xbugs221/oz/internal/ozcli"
)

// main delegates command behavior to the shared oz CLI implementation.
func main() {
	os.Exit(ozcli.Main(os.Args[1:], os.Stdout, os.Stderr))
}
