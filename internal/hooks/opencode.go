package hooks

import (
	"fmt"

	"github.com/prof18/regesto/internal/config"
)

// opencodePayload is the chat.message input Regesto's OpenCode plugin
// forwards: the session id plus the directory and git worktree captured when
// the plugin was initialized.
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
