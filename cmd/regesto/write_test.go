package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func withWriteStdin(t *testing.T, body string, run func() error) error {
	t.Helper()
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := write.WriteString(body); err != nil {
		t.Fatal(err)
	}
	if err := write.Close(); err != nil {
		t.Fatal(err)
	}
	original := os.Stdin
	os.Stdin = read
	defer func() {
		os.Stdin = original
		read.Close()
	}()
	return run()
}

func validWriteJSON(id, scope string) string {
	return `{"id":"` + id + `","title":"Validated write","type":"decision","scope":"` + scope + `",` +
		`"subject":"writer","relation":"cli-contract","topics":[],"body":"Use the shared writer.","why":"It preserves authority."}`
}

func TestWriteJSONInputAndOutputContract(t *testing.T) {
	cfg := normalizeTestConfig(t)
	out, err := captureNormalizeStdout(t, func() error {
		return withWriteStdin(t, validWriteJSON("dec-cli-write", "global"), func() error {
			return runWrite(cfg, []string{"--source", "codex@testbox", "--json-input", "--json"})
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		SchemaVersion         int      `json:"schema_version"`
		Path                  string   `json:"path"`
		PendingReconciliation bool     `json:"pending_reconciliation"`
		Actions               []any    `json:"actions"`
		Reviews               []string `json:"reviews"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("write output is not JSON: %v\n%s", err, out)
	}
	if got.SchemaVersion != 1 || got.Path != "knowledge/facts/global/dec-cli-write.md" || got.PendingReconciliation {
		t.Fatalf("unexpected write result: %+v", got)
	}
	if got.Actions == nil || got.Reviews == nil {
		t.Fatalf("empty collections must be []: %s", out)
	}
}

func TestWriteTitleLengthHardLimit(t *testing.T) {
	cfg := normalizeTestConfig(t)

	atLimit := strings.Replace(validWriteJSON("dec-title-at-limit", "global"), `"title":"Validated write"`, `"title":"`+strings.Repeat("é", 100)+`"`, 1)
	_, err := captureNormalizeStdout(t, func() error {
		return withWriteStdin(t, atLimit, func() error {
			return runWrite(cfg, []string{"--source", "codex@testbox", "--json-input", "--json"})
		})
	})
	if err != nil {
		t.Fatalf("title at hard limit was rejected: %v", err)
	}

	overLimit := strings.Replace(validWriteJSON("dec-title-over-limit", "global"), `"title":"Validated write"`, `"title":"`+strings.Repeat("é", 101)+`"`, 1)
	out, err := captureNormalizeStdout(t, func() error {
		return withWriteStdin(t, overLimit, func() error {
			return runWrite(cfg, []string{"--source", "codex@testbox", "--json-input", "--json"})
		})
	})
	if err == nil || !strings.Contains(err.Error(), "title is 101 characters; maximum is 100 (aim for 80)") {
		t.Fatalf("title above hard limit was not rejected clearly: %v", err)
	}
	if out != "" {
		t.Fatalf("rejected title contaminated stdout: %q", out)
	}
	if _, err := os.Stat(filepath.Join(cfg.KBRoot, "knowledge", "facts", "global", "dec-title-over-limit.md")); !os.IsNotExist(err) {
		t.Fatalf("rejected title reached disk: %v", err)
	}
}

func TestWriteJSONInputRejectsForgedAuthorityWithoutWriting(t *testing.T) {
	cfg := normalizeTestConfig(t)
	body := strings.TrimSuffix(validWriteJSON("dec-forged-write", "global"), "}") +
		`,"source":"human","schema_version":99,"created":"2000-01-01T00:00:00Z"}`
	out, err := captureNormalizeStdout(t, func() error {
		return withWriteStdin(t, body, func() error {
			return runWrite(cfg, []string{"--source", "codex@testbox", "--json-input", "--json"})
		})
	})
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("forged authority was not rejected: %v", err)
	}
	if out != "" {
		t.Fatalf("failed JSON write contaminated stdout: %q", out)
	}
	if _, err := os.Stat(filepath.Join(cfg.KBRoot, "knowledge", "facts", "global", "dec-forged-write.md")); !os.IsNotExist(err) {
		t.Fatalf("forged write reached disk: %v", err)
	}
}

func TestWriteJSONInputRejectsDuplicateFields(t *testing.T) {
	cfg := normalizeTestConfig(t)
	body := strings.Replace(validWriteJSON("dec-duplicate-json", "global"), `"id":"dec-duplicate-json"`, `"id":"dec-first-json","id":"dec-duplicate-json"`, 1)
	out, err := captureNormalizeStdout(t, func() error {
		return withWriteStdin(t, body, func() error {
			return runWrite(cfg, []string{"--source", "codex@testbox", "--json-input", "--json"})
		})
	})
	if err == nil || !strings.Contains(err.Error(), `duplicate field "id"`) {
		t.Fatalf("duplicate JSON field was not rejected: %v", err)
	}
	if out != "" {
		t.Fatalf("failed duplicate-key write contaminated stdout: %q", out)
	}
	for _, id := range []string{"dec-first-json", "dec-duplicate-json"} {
		if _, err := os.Stat(filepath.Join(cfg.KBRoot, "knowledge", "facts", "global", id+".md")); !os.IsNotExist(err) {
			t.Fatalf("duplicate-key write reached disk as %s: %v", id, err)
		}
	}
}

func TestWriteDirResolvesExplicitProjectScope(t *testing.T) {
	cfg := normalizeTestConfig(t)
	dir := filepath.Join(t.TempDir(), "aurora")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	out, err := captureNormalizeStdout(t, func() error {
		return withWriteStdin(t, validWriteJSON("dec-project-cli-write", "project"), func() error {
			return runWrite(cfg, []string{"--source", "codex@testbox", "--dir", dir, "--json-input"})
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	if out != "knowledge/facts/projects/aurora/dec-project-cli-write.md\n" {
		t.Fatalf("text write output changed or project resolution failed: %q", out)
	}
}
