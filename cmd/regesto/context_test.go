package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/prof18/regesto/internal/notify"
)

func captureContextStderr(t *testing.T, run func() error) (string, error) {
	t.Helper()
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stderr
	os.Stderr = write
	err = run()
	if closeErr := write.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	os.Stderr = original
	data, readErr := io.ReadAll(read)
	if closeErr := read.Close(); closeErr != nil && readErr == nil {
		readErr = closeErr
	}
	if readErr != nil {
		t.Fatal(readErr)
	}
	return string(data), err
}

func TestContextReportsRecordedWarningWithoutMaskingLoadError(t *testing.T) {
	cfg := jsonCommandConfig(t)
	_, err := notify.Report(cfg, notify.Health{
		Key:     "cycle",
		Failing: true,
		Message: "knowledge/facts/global/dec-a-json.md: no frontmatter",
	}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfg.KBRoot, "knowledge", "facts", "global", "dec-a-json.md"), []byte("not a fact\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout := ""
	stderr, err := captureContextStderr(t, func() error {
		var runErr error
		stdout, runErr = captureNormalizeStdout(t, func() error {
			return runContext(cfg, []string{"--json", "--project", "aurora"})
		})
		return runErr
	})
	if err == nil || !strings.Contains(err.Error(), "no frontmatter") {
		t.Fatalf("context masked fact load error: %v", err)
	}
	if stdout != "" {
		t.Fatalf("failed JSON context contaminated stdout: %q", stdout)
	}
	if !strings.Contains(stderr, "## Maintenance warning") || !strings.Contains(stderr, "no frontmatter") {
		t.Fatalf("failed context omitted maintenance warning from stderr: %q", stderr)
	}
}
