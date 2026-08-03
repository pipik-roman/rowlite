package main

import (
	"os"

	"github.com/pipik-roman/rowlite/internal/cmd"
)

func main() {
	if err := cmd.NewRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}
