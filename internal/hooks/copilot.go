package hooks

import (
	"fmt"

	"github.com/prof18/regesto/internal/config"
)

// copilotPayload is the SessionStart input Copilot CLI and Copilot in VS Code
// send: VS Code-compatible snake_case common fields (hook_event_name,
// session_id, timestamp) plus the session working directory.
type copilotPayload struct {
	CWD string `json:"cwd"`
}

func runCopilot(cfg *config.Config, body []byte) (string, error) {
	var payload copilotPayload
	if err := decodePayload(body, &payload); err != nil {
		return "", err
	}
	dir := usableDirectory(payload.CWD)
	if dir == "" {
		return "", fmt.Errorf("hook payload contains no usable cwd")
	}
	return buildContext(cfg, dir)
}
