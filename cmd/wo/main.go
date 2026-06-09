// Package main provides the wo command entrypoint.
package main

import (
	"fmt"
	"os"

	"github.com/xbugs221/oz/internal/app"
)

// main delegates all workflow behavior to the application package.
func main() {
	if err := app.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
