package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/prof18/regesto/internal/config"
	"github.com/prof18/regesto/internal/mcp"
)

func runMCP(cfg *config.Config, args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("mcp: unexpected arguments: %s", strings.Join(args, " "))
	}
	return mcp.Serve(cfg, os.Stdin, os.Stdout, os.Stderr)
}
