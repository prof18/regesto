// Package notify records scheduled-pass health and optionally tells the human
// through an explicitly configured command, once per change of state rather
// than once per run.
//
// The cycle aborts on the first validation error and commits nothing — correct,
// since applying half a reconciliation is worse than applying none. But it runs
// under a scheduler with no terminal attached, so the only trace is an exit code
// and a line in a log nobody opens. DESIGN §12 lists the consequence as a known
// trade-off ("generated pages and the index can drift if lint fails silently");
// in practice the drift is unbounded, because every hour after the first failure
// adds facts that are written but never committed, reconciliations that never
// apply, and an INDEX.md that agents keep reading as current.
//
// This closes that gap from inside the pass rather than from a watchdog beside
// it. The pass already knows its own outcome, so there is nothing to poll and
// nothing to infer: a separate job would have to read the scheduler's record of
// the last exit, which is macOS-specific, reports a stale value between runs,
// and cannot tell "failed" from "has not run since load".
package notify

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/prof18/regesto/internal/config"
)

// DefaultRenagHours is how long a persisting failure stays quiet after it has
// been reported once. Long enough that an hourly job cannot turn into 24 alerts
// a day — which is how a notification channel gets muted, taking the first
// useful alert with it — and short enough that a failure cannot be forgotten.
const DefaultRenagHours = 24

const defaultDispatchTimeout = 10 * time.Second

const defaultDispatchWaitDelay = 100 * time.Millisecond

// dispatchTimeout is a variable so the timeout behavior can be tested without
// making the test suite wait for a real hung notifier.
var dispatchTimeout = defaultDispatchTimeout

// dispatchWaitDelay bounds how long exec waits for pipes to close after a
// timed-out notifier is killed. It is a variable so tests can keep cleanup
// checks fast without waiting for the production grace period.
var dispatchWaitDelay = defaultDispatchWaitDelay

// Health is the outcome of one pass, as reported to the human.
type Health struct {
	// Key names the pass. One state file per key, so an alert about the cycle
	// is never suppressed by an alert about something else.
	Key     string
	Failing bool
	Title   string
	Message string
}

// State is what the last run of a pass recorded about its health.
type State struct {
	Failing  bool
	Message  string    // current failure message; empty after recovery
	Since    time.Time // when the current state began
	Notified time.Time // when the human was last told about it
	LastOK   time.Time // when the pass last completed cleanly
}

// Report records this pass's health and dispatches through the configured
// command if the human needs to know: on entering failure, on recovering from
// it, and once per renag interval while a failure persists. It returns whether
// a notification was dispatched; with no command configured, health is still
// recorded but the return value is false.
//
// Nothing here is fatal to the caller. The markdown files are the knowledge and
// this is a courtesy on top (DESIGN §9.1) — a missing notifier, a locked state
// file or a machine with no notification system at all must not fail the pass,
// or the alerting becomes the thing that stops the knowledge base working.
func Report(cfg *config.Config, h Health, now time.Time) (bool, error) {
	path := statePath(cfg, h.Key)
	prev, known := readState(path)

	next := State{Failing: h.Failing, Message: prev.Message, Since: prev.Since, Notified: prev.Notified, LastOK: prev.LastOK}
	if h.Failing {
		next.Message = h.Message
	} else {
		next.Message = ""
	}
	if !h.Failing {
		next.LastOK = now
	}
	if !known || prev.Failing != h.Failing {
		next.Since = now
	}

	send := false
	switch {
	case !known:
		// First run ever. A healthy pass says nothing — an instance that
		// announces itself on install teaches you to dismiss it.
		send = h.Failing
	case prev.Failing != h.Failing:
		// Both directions are worth an alert. A recovery notice is what makes
		// the failure notice trustworthy: an alert you cannot see the end of is
		// one you learn to ignore.
		send = true
	case h.Failing:
		send = renagDue(cfg, prev.Notified, now)
	}

	// A state transition is still recorded when notifications are disabled, but
	// it must not consume the transition: enabling a command later should alert
	// immediately about the still-active failure.
	if send && Enabled(cfg) {
		next.Notified = now
	}
	// Written before dispatching: an enabled notifier that hangs or crashes must not
	// cause the same alert to fire again on the next pass.
	if err := writeState(path, next); err != nil {
		return false, err
	}
	if !send || !Enabled(cfg) {
		return false, nil
	}
	if err := dispatch(cfg, h); err != nil {
		// Notified was intentionally persisted above. A broken notifier should
		// not turn into an alert storm on every scheduled pass.
		return false, err
	}
	return true, nil
}

// Load reports what the last pass recorded, so `schedule status` can answer the
// question this package cannot: whether the pass is running at all. A cycle that
// never fires never reports failure, and a stale LastOK is the only evidence.
func Load(cfg *config.Config, key string) (State, bool) {
	return readState(statePath(cfg, key))
}

// Enabled reports whether notifications will actually go anywhere, so callers
// can say so rather than silently doing nothing.
func Enabled(cfg *config.Config) bool {
	if strings.EqualFold(strings.TrimSpace(cfg.Section("notify")["on"]), "off") {
		return false
	}
	return len(command(cfg, Health{})) > 0
}

// statePath keeps the record under .state/<machine>/, which is gitignored and
// excluded from sync: whether *this* machine has already alerted is per-machine
// state, exactly like the harvest baselines, and syncing it would let one
// machine's alert suppress another's.
func statePath(cfg *config.Config, key string) string {
	return filepath.Join(cfg.KBRoot, ".state", cfg.Machine, "notify", key+".state")
}

func renagDue(cfg *config.Config, last, now time.Time) bool {
	hours := DefaultRenagHours
	if raw := strings.TrimSpace(cfg.Section("notify")["renag_hours"]); raw != "" {
		if n, err := fmt.Sscanf(raw, "%d", &hours); n != 1 || err != nil {
			hours = DefaultRenagHours
		}
	}
	if hours <= 0 {
		return false // configured off: report the transition and never nag again
	}
	if last.IsZero() {
		return true
	}
	return now.Sub(last) >= time.Duration(hours)*time.Hour
}

func readState(path string) (State, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return State{}, false
	}
	var s State
	for _, line := range strings.Split(string(data), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		switch key {
		case "state":
			s.Failing = value == "failing"
		case "message": // legacy/plain-text form
			s.Message = value
		case "message_b64":
			if decoded, err := base64.StdEncoding.DecodeString(value); err == nil {
				s.Message = string(decoded)
			}
		case "since":
			s.Since, _ = time.Parse(time.RFC3339, value)
		case "notified":
			s.Notified, _ = time.Parse(time.RFC3339, value)
		case "ok_at":
			s.LastOK, _ = time.Parse(time.RFC3339, value)
		}
	}
	return s, true
}

// writeState uses key=value lines and RFC3339 stamps because the first thing
// anyone does with a state file is cat it.
func writeState(path string, s State) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "state=%s\n", map[bool]string{true: "failing", false: "ok"}[s.Failing])
	writeStamp(&b, "since", s.Since)
	writeStamp(&b, "notified", s.Notified)
	writeStamp(&b, "ok_at", s.LastOK)
	if s.Message != "" {
		fmt.Fprintf(&b, "message_b64=%s\n", base64.StdEncoding.EncodeToString([]byte(s.Message)))
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".notify-state-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	removeTemp := true
	defer func() {
		if removeTemp {
			_ = os.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.WriteString(b.String()); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	removeTemp = false
	return nil
}

func writeStamp(b *strings.Builder, key string, t time.Time) {
	if t.IsZero() {
		return
	}
	fmt.Fprintf(b, "%s=%s\n", key, t.UTC().Format(time.RFC3339))
}

// command builds the argv to run. Notifications are opt-in and portable: the
// configured command is the only dispatch mechanism.
func command(cfg *config.Config, h Health) []string {
	if custom := strings.Fields(cfg.Section("notify")["command"]); len(custom) > 0 {
		// Title and message go on the end for simple wrapper scripts. Anything
		// needing them elsewhere reads the REGESTO_NOTIFY_* environment instead.
		return append(custom, h.Title, h.Message)
	}
	return nil
}

func dispatch(cfg *config.Config, h Health) error {
	if strings.EqualFold(strings.TrimSpace(cfg.Section("notify")["on"]), "off") {
		return nil
	}
	argv := command(cfg, h)
	if len(argv) == 0 {
		return nil // no external notifier configured
	}
	ctx, cancel := context.WithTimeout(context.Background(), dispatchTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	configureProcessGroup(cmd)
	cmd.WaitDelay = dispatchWaitDelay
	cmd.Env = append(os.Environ(),
		"REGESTO_NOTIFY_KEY="+h.Key,
		"REGESTO_NOTIFY_STATE="+map[bool]string{true: "failing", false: "ok"}[h.Failing],
		"REGESTO_NOTIFY_TITLE="+h.Title,
		"REGESTO_NOTIFY_MESSAGE="+h.Message,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("%s: notifier timed out after %s: %s", argv[0], dispatchTimeout, strings.TrimSpace(string(out)))
		}
		return fmt.Errorf("%s: %v: %s", argv[0], err, strings.TrimSpace(string(out)))
	}
	return nil
}
