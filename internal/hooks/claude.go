package hooks

import (
	"fmt"
	"os"

	"github.com/prof18/regesto/internal/config"
	"github.com/prof18/regesto/internal/facts"
	"github.com/prof18/regesto/internal/index"
	"github.com/prof18/regesto/internal/project"
)

type claudePayload struct {
	Workspace *struct {
		CurrentDir string `json:"current_dir"`
	} `json:"workspace"`
	CWD string `json:"cwd"`
}

func runClaude(cfg *config.Config, body []byte) (string, error) {
	var payload claudePayload
	if err := decodePayload(body, &payload); err != nil {
		return "", err
	}
	dir := ""
	if payload.Workspace != nil {
		dir = usableDirectory(payload.Workspace.CurrentDir)
	}
	if dir == "" {
		dir = usableDirectory(payload.CWD)
	}
	if dir == "" {
		return "", fmt.Errorf("hook payload contains no usable workspace.current_dir or cwd")
	}
	return buildContext(cfg, dir)
}

func usableDirectory(path string) string {
	if path == "" {
		return ""
	}
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return ""
	}
	return path
}

func buildContext(cfg *config.Config, dir string) (string, error) {
	resolution := project.Resolve(cfg, dir)
	all, err := facts.LoadAll(cfg.KBRoot)
	if err != nil {
		return "", err
	}
	return index.BuildContext(all, index.ContextOptions{Project: resolution.Name, MaxBytes: 4096}), nil
}
