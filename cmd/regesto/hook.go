package main

import (
	"fmt"
	"io"
	"os"

	"github.com/prof18/regesto/internal/config"
	regestohooks "github.com/prof18/regesto/internal/hooks"
)

func runHook(cfg *config.Config, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: regesto hook <protocol>")
	}
	return regestohooks.Run(cfg, args[0], os.Stdin, os.Stdout, os.Stderr)
}

func failOpenHook(args []string, output, diagnostic io.Writer, cause error) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: regesto hook <protocol>")
	}
	fmt.Fprintf(diagnostic, "regesto hook %s: %v\n", args[0], cause)
	switch args[0] {
	case "claude-session-start-v1":
		return nil
	case "hermes-pre-llm-v1":
		_, _ = io.WriteString(output, "{}")
		return nil
	default:
		return fmt.Errorf("unknown hook protocol %q", args[0])
	}
}
