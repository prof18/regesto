// A knowledge base that stops maintaining itself has to say so. These pin the
// part that is easy to get wrong twice over: alerting so often that the channel
// gets muted, and alerting so rarely that an 11-day outage passes unnoticed.
package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/prof18/regesto/internal/config"
	"github.com/prof18/regesto/internal/notify"
)

// notifyInstance returns an instance whose notifier appends one line per alert
// to a file, so a test can assert on what the human would have been told. A
// real command rather than a seam in the code: dispatch is where the escaping
// and argument order live, and a mock would test neither.
func notifyInstance(t *testing.T) (*config.Config, string) {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "sent.log")
	script := filepath.Join(dir, "notifier")
	body := "#!/bin/sh\nprintf '%s|%s|%s\\n' \"$REGESTO_NOTIFY_STATE\" \"$1\" \"$2\" >> " + log + "\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := writeInstance(t, "machine = \"testbox\"\n\n[notify]\ncommand = \""+script+"\"\n", "")
	return cfg, log
}

func sent(t *testing.T, log string) []string {
	t.Helper()
	data, err := os.ReadFile(log)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

func failing(msg string) notify.Health {
	return notify.Health{Key: "cycle", Failing: true, Title: "regesto: the cycle is failing", Message: msg}
}

func healthy() notify.Health {
	return notify.Health{Key: "cycle", Title: "regesto: the cycle is working again", Message: "all clean"}
}

// The first failure is the one that matters: everything after it is knowledge
// written but never committed.
func TestFirstFailureNotifies(t *testing.T) {
	cfg, log := notifyInstance(t)
	now := time.Now().UTC()

	did, err := notify.Report(cfg, failing("2 validation error(s)"), now)
	if err != nil {
		t.Fatalf("Report: %v", err)
	}
	if !did {
		t.Fatal("the first failure sent no notification")
	}
	lines := sent(t, log)
	if len(lines) != 1 || !strings.HasPrefix(lines[0], "failing|") {
		t.Fatalf("notifier got %q", lines)
	}
	if !strings.Contains(lines[0], "2 validation error(s)") {
		t.Errorf("the message dropped the reason: %q", lines[0])
	}
}

// An hourly job that alerts every hour gets muted, and a muted channel loses the
// next real failure too.
func TestRepeatedFailureStaysQuiet(t *testing.T) {
	cfg, log := notifyInstance(t)
	start := time.Now().UTC()

	for i := 0; i < 12; i++ {
		if _, err := notify.Report(cfg, failing("still broken"), start.Add(time.Duration(i)*time.Hour)); err != nil {
			t.Fatalf("Report: %v", err)
		}
	}
	if lines := sent(t, log); len(lines) != 1 {
		t.Fatalf("12 hourly failures sent %d notifications, want 1: %q", len(lines), lines)
	}
}

// Quiet must not mean forgotten: a failure nobody fixed is still costing facts a
// day later.
func TestPersistentFailureRenagsDaily(t *testing.T) {
	cfg, log := notifyInstance(t)
	start := time.Now().UTC()

	if _, err := notify.Report(cfg, failing("day one"), start); err != nil {
		t.Fatalf("Report: %v", err)
	}
	if _, err := notify.Report(cfg, failing("day two"), start.Add(notify.DefaultRenagHours*time.Hour)); err != nil {
		t.Fatalf("Report: %v", err)
	}
	if lines := sent(t, log); len(lines) != 2 {
		t.Fatalf("got %d notifications across two days, want 2: %q", len(lines), lines)
	}
}

// The recovery notice is what makes the failure notice worth trusting. Without
// it you never learn whether the thing you were told about is still true.
func TestRecoveryNotifies(t *testing.T) {
	cfg, log := notifyInstance(t)
	start := time.Now().UTC()

	if _, err := notify.Report(cfg, failing("broken"), start); err != nil {
		t.Fatalf("Report: %v", err)
	}
	did, err := notify.Report(cfg, healthy(), start.Add(time.Hour))
	if err != nil {
		t.Fatalf("Report: %v", err)
	}
	if !did {
		t.Fatal("recovery sent no notification")
	}
	lines := sent(t, log)
	if len(lines) != 2 || !strings.HasPrefix(lines[1], "ok|") {
		t.Fatalf("notifier got %q, want a recovery as the second line", lines)
	}
}

// A working knowledge base has nothing to say. An instance that announces every
// successful pass teaches you to dismiss the one that matters.
func TestHealthyPassesAreSilent(t *testing.T) {
	cfg, log := notifyInstance(t)
	start := time.Now().UTC()

	for i := 0; i < 5; i++ {
		if _, err := notify.Report(cfg, healthy(), start.Add(time.Duration(i)*time.Hour)); err != nil {
			t.Fatalf("Report: %v", err)
		}
	}
	if lines := sent(t, log); len(lines) != 0 {
		t.Fatalf("healthy passes sent %q, want silence", lines)
	}
}

// `schedule status` answers the question the cycle cannot answer about itself:
// a job that never fires never reports a failure, so the last clean pass is the
// only evidence that the schedule is real.
func TestLastCleanPassIsRecorded(t *testing.T) {
	cfg, _ := notifyInstance(t)
	when := time.Now().UTC().Add(-3 * time.Hour).Truncate(time.Second)

	if _, err := notify.Report(cfg, healthy(), when); err != nil {
		t.Fatalf("Report: %v", err)
	}
	state, ok := notify.Load(cfg, "cycle")
	if !ok {
		t.Fatal("no state recorded for the cycle")
	}
	if state.Failing {
		t.Error("a clean pass was recorded as failing")
	}
	if !state.LastOK.Equal(when) {
		t.Errorf("last clean pass = %s, want %s", state.LastOK, when)
	}
}

// Per-machine, like the harvest baselines: this record says whether *this*
// machine has already alerted. Syncing it would let one machine's alert
// suppress another's.
func TestStateIsPerMachineAndUnsynced(t *testing.T) {
	cfg, _ := notifyInstance(t)
	if _, err := notify.Report(cfg, failing("broken"), time.Now().UTC()); err != nil {
		t.Fatalf("Report: %v", err)
	}
	path := filepath.Join(cfg.KBRoot, ".state", cfg.Machine, "notify", "cycle.state")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected per-machine state at %s: %v", path, err)
	}
}

// Turning it off has to actually turn it off — a notification channel someone
// silenced and that keeps firing is worse than one that never worked.
func TestNotificationsCanBeTurnedOff(t *testing.T) {
	cfg, log := notifyInstance(t)
	cfg.Sections["notify"]["on"] = "off"

	if _, err := notify.Report(cfg, failing("broken"), time.Now().UTC()); err != nil {
		t.Fatalf("Report: %v", err)
	}
	if lines := sent(t, log); len(lines) != 0 {
		t.Fatalf("notifications were off but %q was sent", lines)
	}
	if notify.Enabled(cfg) {
		t.Error("Enabled reported true with [notify].on = off")
	}
}

// Lint messages carry file paths, quoted field values, and backslashes. They
// must survive direct argv and environment dispatch unchanged.
func TestQuotesInTheMessageSurviveDispatch(t *testing.T) {
	cfg, log := notifyInstance(t)
	nasty := `status "draft" is not one of active/proposed \ superseded`

	if _, err := notify.Report(cfg, failing(nasty), time.Now().UTC()); err != nil {
		t.Fatalf("Report: %v", err)
	}
	lines := sent(t, log)
	if len(lines) != 1 || !strings.Contains(lines[0], `status "draft" is not one of`) {
		t.Fatalf("the message did not survive dispatch: %q", lines)
	}
}

func TestFailureMessageIsPersistedAndClearedOnRecovery(t *testing.T) {
	cfg, _ := notifyInstance(t)
	message := "validation failed: \"draft\"\nfile=notes/today.md\nline 4"
	when := time.Now().UTC().Truncate(time.Second)
	if _, err := notify.Report(cfg, failing(message), when); err != nil {
		t.Fatalf("Report failure: %v", err)
	}
	state, ok := notify.Load(cfg, "cycle")
	if !ok || state.Message != message {
		t.Fatalf("persisted message = %q, want %q (ok=%v)", state.Message, message, ok)
	}
	if _, err := notify.Report(cfg, healthy(), when.Add(time.Hour)); err != nil {
		t.Fatalf("Report recovery: %v", err)
	}
	state, ok = notify.Load(cfg, "cycle")
	if !ok || state.Message != "" {
		t.Fatalf("recovery message = %q, want empty (ok=%v)", state.Message, ok)
	}
}

func TestNoCommandDisablesNotifications(t *testing.T) {
	cfg := writeInstance(t, "machine = \"testbox\"\n", "")
	if notify.Enabled(cfg) {
		t.Fatal("notifications enabled without [notify].command")
	}
	if did, err := notify.Report(cfg, failing("broken"), time.Now().UTC()); err != nil || did {
		t.Fatalf("Report = (%v, %v), want no dispatch without command", did, err)
	}
	state, ok := notify.Load(cfg, "cycle")
	if !ok || !state.Failing || state.Message != "broken" {
		t.Fatalf("disabled notification did not persist failure state: %+v (ok=%v)", state, ok)
	}
	if !state.Notified.IsZero() {
		t.Fatalf("disabled notification consumed the alert transition at %s", state.Notified)
	}

	script := filepath.Join(cfg.KBRoot, "notifier")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg.Sections["notify"] = map[string]string{"command": script}
	if did, err := notify.Report(cfg, failing("still broken"), time.Now().UTC().Add(time.Minute)); err != nil || !did {
		t.Fatalf("Report after configuring command = (%v, %v), want immediate dispatch", did, err)
	}
}

func TestOldStateWithoutMessageRemainsReadable(t *testing.T) {
	cfg := writeInstance(t, "machine = \"testbox\"\n", "")
	path := filepath.Join(cfg.KBRoot, ".state", cfg.Machine, "notify", "cycle.state")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	old := "state=failing\nsince=2026-09-03T08:00:00Z\nnotified=2026-09-03T08:00:00Z\n"
	if err := os.WriteFile(path, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	state, ok := notify.Load(cfg, "cycle")
	if !ok || !state.Failing || state.Message != "" {
		t.Fatalf("old state = %+v (ok=%v), want readable failing state with empty message", state, ok)
	}
}
