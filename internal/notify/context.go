package notify

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/prof18/regesto/internal/config"
)

const maxContextMessageBytes = 1024

// ContextAlert renders the current failure for agent context. Notification
// delivery is optional, but an agent must not consume generated knowledge as if
// it were current while the cycle that maintains it is failing.
func ContextAlert(cfg *config.Config, key string, now time.Time) string {
	state, ok := Load(cfg, key)
	if !ok || !state.Failing {
		return ""
	}

	status := "Regesto's scheduled cycle is failing; the start time is unknown."
	if !state.Since.IsZero() {
		status = fmt.Sprintf("Regesto's scheduled cycle has been failing since %s (%s ago).",
			state.Since.UTC().Format(time.RFC3339), compactAge(now.Sub(state.Since)))
	}
	message := truncateContextMessage(strings.TrimSpace(state.Message))

	var b strings.Builder
	b.WriteString(status + "\n")
	if message != "" {
		fmt.Fprintf(&b, "Failure: %s\n", message)
	}
	b.WriteString("Treat `INDEX.md` and generated topic pages as stale. Fix the failure, then run `regesto cycle`.")
	return b.String()
}

func compactAge(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return "under a minute"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d/time.Minute))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d/time.Hour))
	default:
		return fmt.Sprintf("%dd", int(d/(24*time.Hour)))
	}
}

func truncateContextMessage(message string) string {
	if len(message) <= maxContextMessageBytes {
		return message
	}
	const marker = "…"
	message = message[:maxContextMessageBytes-len(marker)]
	for !utf8.ValidString(message) {
		message = message[:len(message)-1]
	}
	return strings.TrimSpace(message) + marker
}
