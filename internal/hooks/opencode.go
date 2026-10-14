package hooks

import (
	"fmt"

	"github.com/prof18/regesto/internal/config"
)

// opencodePayload is what Regesto's OpenCode plugin forwards from either
// host generation — the prompt-admission hook on OpenCode 2, chat.message on
// OpenCode 1: the session id plus the directory and git worktree the session
// runs in. OpenCode 2 resolves one directory from the session record and
// sends it as both fields.
type opencodePayload struct {
	SessionID string `json:"sessionID"`
	Directory string `json:"directory"`
	Worktree  string `json:"worktree"`
}

func runOpencode(cfg *config.Config, body []byte) (string, error) {
	var payload opencodePayload
	if err := decodePayload(body, &payload); err != nil {
		return "", err
	}
	dir := usableDirectory(payload.Worktree)
	if dir == "" {
		dir = usableDirectory(payload.Directory)
	}
	if dir == "" {
		return "", fmt.Errorf("hook payload contains no usable worktree or directory")
	}
	return buildContext(cfg, dir)
}
