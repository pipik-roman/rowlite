package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "rowlite",
	Short: "Rowlite is a flat Markdown database CLI and MCP server",
	Long:  `A zero-config, non-SQL, lightweight command line interface and MCP server to query and edit Markdown GFM tables.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Welcome to Rowlite! Use --help to see available commands.")
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func main() {
	Execute()
}
