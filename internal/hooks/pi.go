package hooks

import (
	"fmt"

	"github.com/prof18/regesto/internal/config"
)

// piPayload is the before_agent_start input Regesto's pi extension forwards:
// the session id used to gate one injection per session, plus the working
// directory pi captured for the run.
type piPayload struct {
	SessionID string `json:"sessionId"`
	CWD       string `json:"cwd"`
}

func runPi(cfg *config.Config, body []byte) (string, error) {
	var payload piPayload
	if err := decodePayload(body, &payload); err != nil {
		return "", err
	}
	dir := usableDirectory(payload.CWD)
	if dir == "" {
		return "", fmt.Errorf("hook payload contains no usable cwd")
	}
	return buildContext(cfg, dir)
}
