package main

import (
	"os"

	"github.com/roman-pipik/rowlite/internal/cmd"
)

func main() {
	if err := cmd.NewRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}
