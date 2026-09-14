package hooks

import (
	"fmt"

	"github.com/prof18/regesto/internal/config"
)

// antigravityPayload is the PreInvocation input the Antigravity harness
// sends before every model call: camelCase system fields (conversationId,
// workspacePaths, transcriptPath) plus the 0-indexed invocationNum that
// identifies the first call of a conversation.
type antigravityPayload struct {
	InvocationNum  *int     `json:"invocationNum"`
	WorkspacePaths []string `json:"workspacePaths"`
}

func runAntigravity(cfg *config.Config, body []byte) (string, error) {
	var payload antigravityPayload
	if err := decodePayload(body, &payload); err != nil {
		return "", err
	}
	// PreInvocation fires before every model call; only the first one is
	// the injection point. A missing invocationNum is not provably the
	// first call, so it injects nothing rather than repeat context on
	// every turn.
	if payload.InvocationNum == nil || *payload.InvocationNum != 0 {
		return "", nil
	}
	var dir string
	for _, candidate := range payload.WorkspacePaths {
		if dir = usableDirectory(candidate); dir != "" {
			break
		}
	}
	if dir == "" {
		return "", fmt.Errorf("hook payload contains no usable workspace path")
	}
	return buildContext(cfg, dir)
}
