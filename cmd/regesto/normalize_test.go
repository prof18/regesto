package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/prof18/regesto/internal/config"
)

func normalizeTestConfig(t *testing.T) *config.Config {
	t.Helper()
	t.Setenv("REGESTO_MACHINE", "testbox")
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "knowledge", "facts", "global"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "config.toml")
	if err := os.WriteFile(path, []byte("integrations = [\"claude\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func putNormalizeTestCapture(t *testing.T, cfg *config.Config, source, body string) {
	t.Helper()
	dir := filepath.Join(cfg.KBRoot, "inbox", source, "20260829T120000Z")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "note.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func captureNormalizeStdout(t *testing.T, run func() error) (string, error) {
	t.Helper()
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stdout
	os.Stdout = write
	err = run()
	if closeErr := write.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	os.Stdout = original
	data, readErr := io.ReadAll(read)
	if closeErr := read.Close(); closeErr != nil && readErr == nil {
		readErr = closeErr
	}
	if readErr != nil {
		t.Fatal(readErr)
	}
	return string(data), err
}

func TestTrustShowPromptOnlyRevealsEligibleCaptures(t *testing.T) {
	cfg := normalizeTestConfig(t)
	// This source sorts before claude@testbox, so the regression catches the
	// old implementation that selected the first raw capture without checking.
	putNormalizeTestCapture(t, cfg, "aaa@testbox", "QUARANTINED SECRET")
	putNormalizeTestCapture(t, cfg, "claude@testbox", "TRUSTED CAPTURE")

	out, err := captureNormalizeStdout(t, func() error {
		return runNormalize(cfg, []string{"--show-prompt"})
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "TRUSTED CAPTURE") {
		t.Fatalf("show-prompt did not show trusted capture:\n%s", out)
	}
	for _, secret := range []string{"QUARANTINED SECRET", "aaa@testbox"} {
		if strings.Contains(out, secret) {
			t.Errorf("show-prompt revealed quarantined capture %q:\n%s", secret, out)
		}
	}
}

func TestTrustShowPromptWithOnlyQuarantinedCapturesIsSafe(t *testing.T) {
	cfg := normalizeTestConfig(t)
	putNormalizeTestCapture(t, cfg, "aaa@testbox", "QUARANTINED SECRET")

	out, err := captureNormalizeStdout(t, func() error {
		return runNormalize(cfg, []string{"--show-prompt"})
	})
	if err != nil {
		t.Fatal(err)
	}
	if out != "no eligible captures in inbox/\n" {
		t.Errorf("show-prompt output = %q", out)
	}
	if strings.Contains(out, "QUARANTINED SECRET") || strings.Contains(out, "aaa@testbox") {
		t.Errorf("show-prompt revealed quarantined capture:\n%s", out)
	}
}
