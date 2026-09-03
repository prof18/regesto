package notify

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/prof18/regesto/internal/config"
)

func TestReportTimeoutWithDescendantHoldingPipeReturnsPromptly(t *testing.T) {
	oldTimeout := dispatchTimeout
	oldWaitDelay := dispatchWaitDelay
	dispatchTimeout = 10 * time.Millisecond
	dispatchWaitDelay = 25 * time.Millisecond
	t.Cleanup(func() {
		dispatchTimeout = oldTimeout
		dispatchWaitDelay = oldWaitDelay
	})

	script := filepath.Join(t.TempDir(), "notifier")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nsleep 1 & wait\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		KBRoot:   t.TempDir(),
		Machine:  "testbox",
		Sections: map[string]map[string]string{"notify": {"command": script}},
	}

	started := time.Now()
	did, err := Report(cfg, Health{Key: "cycle", Failing: true, Title: "failure", Message: "broken"}, started)
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("Report took %s; descendant-held pipe was not bounded", elapsed)
	}
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("Report error = %v, want timeout error", err)
	}
	if did {
		t.Fatal("Report reported dispatch despite timeout")
	}
	state, ok := Load(cfg, "cycle")
	if !ok || state.Notified.IsZero() {
		t.Fatalf("timeout did not retain persisted Notified state: %+v (ok=%v)", state, ok)
	}
}

func TestReportTimeoutKillsNotifierDescendants(t *testing.T) {
	oldTimeout := dispatchTimeout
	oldWaitDelay := dispatchWaitDelay
	dispatchTimeout = time.Second
	dispatchWaitDelay = 25 * time.Millisecond
	t.Cleanup(func() { dispatchTimeout, dispatchWaitDelay = oldTimeout, oldWaitDelay })

	dir := t.TempDir()
	ready := filepath.Join(dir, "descendant-started")
	sentinel := filepath.Join(dir, "descendant-wrote")
	script := filepath.Join(t.TempDir(), "notifier")
	if err := os.WriteFile(script, []byte("#!/bin/sh\n(sleep 2; printf wrote > \"$REGESTO_NOTIFY_SENTINEL\") &\nprintf started > \"$REGESTO_NOTIFY_READY\"\nwait\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{KBRoot: t.TempDir(), Machine: "testbox", Sections: map[string]map[string]string{"notify": {"command": script}}}

	// The command environment is inherited through the test process. The ready
	// marker proves the shell actually launched its descendant before timing out.
	t.Setenv("REGESTO_NOTIFY_READY", ready)
	t.Setenv("REGESTO_NOTIFY_SENTINEL", sentinel)
	_, err := Report(cfg, Health{Key: "descendant", Failing: true}, time.Now())
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("Report error = %v, want timeout", err)
	}
	if _, err := os.Stat(ready); err != nil {
		t.Fatalf("notifier never launched its descendant: %v", err)
	}
	time.Sleep(2200 * time.Millisecond)
	if _, err := os.Stat(sentinel); err == nil {
		t.Fatal("timed-out notifier descendant wrote sentinel")
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

func TestReportTimeoutRecordsNotificationAndReturnsError(t *testing.T) {
	oldTimeout := dispatchTimeout
	dispatchTimeout = 10 * time.Millisecond
	t.Cleanup(func() { dispatchTimeout = oldTimeout })

	script := filepath.Join(t.TempDir(), "notifier")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexec sleep 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		KBRoot:   t.TempDir(),
		Machine:  "testbox",
		Sections: map[string]map[string]string{"notify": {"command": script}},
	}

	did, err := Report(cfg, Health{Key: "cycle", Failing: true, Title: "failure", Message: "broken"}, time.Now())
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("Report error = %v, want timeout error", err)
	}
	if did {
		t.Fatal("Report reported dispatch despite timeout")
	}
	state, ok := Load(cfg, "cycle")
	if !ok || state.Notified.IsZero() {
		t.Fatalf("timeout did not retain persisted Notified state: %+v (ok=%v)", state, ok)
	}
}

func TestReportNonzeroNotifierRecordsNotificationAndReturnsError(t *testing.T) {
	script := filepath.Join(t.TempDir(), "notifier")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf 'broken'\nexit 7\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		KBRoot:   t.TempDir(),
		Machine:  "testbox",
		Sections: map[string]map[string]string{"notify": {"command": script}},
	}

	did, err := Report(cfg, Health{Key: "cycle", Failing: true, Title: "failure", Message: "broken"}, time.Now())
	if err == nil || !strings.Contains(err.Error(), "exit status 7") || !strings.Contains(err.Error(), "broken") {
		t.Fatalf("Report error = %v, want notifier exit and output", err)
	}
	if did {
		t.Fatal("Report reported dispatch despite notifier failure")
	}
	state, ok := Load(cfg, "cycle")
	if !ok || state.Notified.IsZero() {
		t.Fatalf("notifier failure did not retain persisted Notified state: %+v (ok=%v)", state, ok)
	}
}

func TestWriteStateUsesIntendedMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "cycle.state")
	if err := writeState(path, State{Failing: true, Since: time.Now()}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o644 {
		t.Fatalf("state mode = %04o, want 0644", got)
	}
}

func TestContextMessageTruncationHonorsByteLimit(t *testing.T) {
	for _, input := range []string{
		strings.Repeat("a", maxContextMessageBytes+100),
		strings.Repeat("🙂", maxContextMessageBytes),
	} {
		got := truncateContextMessage(input)
		if len(got) > maxContextMessageBytes {
			t.Fatalf("truncated message is %d bytes, want at most %d", len(got), maxContextMessageBytes)
		}
		if !utf8.ValidString(got) {
			t.Fatalf("truncated message is not valid UTF-8: %q", got)
		}
		if !strings.HasSuffix(got, "…") {
			t.Fatalf("truncated message has no marker: %q", got)
		}
	}
}
