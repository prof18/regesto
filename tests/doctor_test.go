package tests

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type doctorJSON struct {
	SchemaVersion int    `json:"schema_version"`
	Status        string `json:"status"`
	Machine       struct {
		Name   string `json:"name"`
		Source string `json:"source"`
	} `json:"machine"`
	Integrations []struct {
		ID           string `json:"id"`
		ProfileID    string `json:"profile_id"`
		Configured   bool   `json:"configured"`
		Detected     bool   `json:"detected"`
		Status       string `json:"status"`
		Capabilities struct {
			Detection struct {
				Status string `json:"status"`
			} `json:"detection"`
			Skills struct {
				Status string `json:"status"`
			} `json:"skills"`
			Instructions struct {
				Status string `json:"status"`
			} `json:"instructions"`
			Hooks []struct {
				Protocol  string `json:"protocol"`
				Registrar string `json:"registrar"`
				Status    string `json:"status"`
			} `json:"hooks"`
			Memory []struct {
				Kind    string `json:"kind"`
				Status  string `json:"status"`
				Matches int    `json:"matches"`
			} `json:"memory"`
			Trust struct {
				Status string `json:"status"`
				Detail string `json:"detail"`
			} `json:"trust"`
		} `json:"capabilities"`
		Artifacts    []map[string]any `json:"artifacts"`
		Remediations []string         `json:"remediations"`
	} `json:"integrations"`
	Trust struct {
		Precedence     []string `json:"precedence"`
		SourcePolicies []struct {
			Source  string `json:"source"`
			Trust   string `json:"trust"`
			Pattern bool   `json:"pattern"`
		} `json:"source_policies"`
		LegacySources []struct {
			Source string `json:"source"`
			Reason string `json:"reason"`
		} `json:"legacy_trusted_sources"`
	} `json:"trust"`
	Checks       []map[string]any `json:"checks"`
	Remediations []string         `json:"remediations"`
}

func runDoctorJSON(t *testing.T, cfgPath string, extra ...string) (doctorJSON, []byte) {
	t.Helper()
	args := []string{"run", "./cmd/regesto", "--config", cfgPath, "doctor", "--json"}
	args = append(args, extra...)
	cmd := exec.Command("go", args...)
	cmd.Dir = repoRoot(t)
	cmd.Env = append(os.Environ(), "PATH=/usr/bin:/bin", "GOTELEMETRY=off", "GOCACHE="+t.TempDir(), "GOPATH="+t.TempDir())
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("doctor command: %v\nstderr: %s\nstdout: %s", err, stderr.String(), stdout.String())
	}
	var got doctorJSON
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("invalid doctor JSON: %v\n%s", err, stdout.String())
	}
	return got, stdout.Bytes()
}

func TestDoctorReportsCapabilitiesTrustAndRemediationWithoutWriting(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, relative := range []string{".claude", ".codex/memories"} {
		if err := os.MkdirAll(filepath.Join(home, relative), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	root := t.TempDir()
	materializeInstallHook(t, root)
	configBody := strings.Join([]string{
		`machine = "doctorbox"`,
		`integrations = ["claude", "codex", "other"]`,
		`[integrations.other]`,
		`skills_dir = "~/.other/skills"`,
		`instructions_file = "~/.other/INSTRUCTIONS.md"`,
		`trust = "quarantine"`,
		`[source_policies]`,
		`"codex@doctorbox" = "quarantine"`,
		`"other@*" = "supervised"`,
		`[trusted_sources]`,
		`"claude@doctorbox" = "private host"`,
	}, "\n") + "\n"
	cfgPath := filepath.Join(root, "config.toml")
	if err := os.WriteFile(cfgPath, []byte(configBody), 0o644); err != nil {
		t.Fatal(err)
	}
	beforeHome, beforeRoot := doctorHostSnapshot(t, home), treeSnapshot(t, root)
	got, first := runDoctorJSON(t, cfgPath)
	_, second := runDoctorJSON(t, cfgPath)
	if !bytes.Equal(first, second) {
		t.Fatalf("doctor JSON is not deterministic:\nfirst: %s\nsecond: %s", first, second)
	}
	if got.SchemaVersion != 1 || got.Machine.Name != "doctorbox" || got.Machine.Source != "config.toml" {
		t.Fatalf("unexpected doctor envelope: %+v", got)
	}
	if got.Status != "warning" || len(got.Integrations) != 3 {
		t.Fatalf("status/integrations = %q/%d, want warning/3; checks=%+v; json=%s", got.Status, len(got.Integrations), got.Checks, first)
	}
	byID := map[string]struct {
		Detected    bool
		ProfileID   string
		HookStatus  string
		MemoryKind  string
		MemoryState string
		Trust       string
		Artifacts   int
		Remedies    int
	}{}
	for _, integration := range got.Integrations {
		hookStatus := ""
		if len(integration.Capabilities.Hooks) > 0 {
			hookStatus = integration.Capabilities.Hooks[0].Status
		}
		memoryKind, memoryStatus := "", ""
		if len(integration.Capabilities.Memory) > 0 {
			memoryKind = integration.Capabilities.Memory[0].Kind
			memoryStatus = integration.Capabilities.Memory[0].Status
		}
		byID[integration.ID] = struct {
			Detected    bool
			ProfileID   string
			HookStatus  string
			MemoryKind  string
			MemoryState string
			Trust       string
			Artifacts   int
			Remedies    int
		}{integration.Detected, integration.ProfileID, hookStatus, memoryKind, memoryStatus, integration.Capabilities.Trust.Detail, len(integration.Artifacts), len(integration.Remediations)}
	}
	if claude := byID["claude"]; !claude.Detected || claude.ProfileID != "claude" || claude.HookStatus != "warning" || claude.MemoryState != "warning" || claude.Artifacts == 0 || claude.Remedies == 0 {
		t.Errorf("claude diagnostic incomplete: %+v", claude)
	}
	if codex := byID["codex"]; !codex.Detected || codex.ProfileID != "codex" || codex.HookStatus != "unsupported" || codex.MemoryKind != "markdown-glob-v1" || codex.MemoryState != "ok" {
		t.Errorf("codex diagnostic incomplete: %+v", codex)
	}
	if other := byID["other"]; other.Detected || other.ProfileID != "generic" || other.HookStatus != "unsupported" || other.MemoryKind != "none" || other.MemoryState != "unsupported" || !strings.Contains(other.Trust, "quarantine") {
		t.Errorf("generic diagnostic incomplete: %+v", other)
	}
	if len(got.Trust.SourcePolicies) != 2 || got.Trust.SourcePolicies[0].Source != "codex@doctorbox" || got.Trust.SourcePolicies[1].Source != "other@" || !got.Trust.SourcePolicies[1].Pattern {
		t.Errorf("source policy diagnostics are incomplete or unsorted: %+v", got.Trust.SourcePolicies)
	}
	if len(got.Trust.Precedence) != 6 || got.Trust.Precedence[0] != "exact source policy" || got.Trust.Precedence[5] != "quarantine fallback" {
		t.Errorf("trust precedence is incomplete: %v", got.Trust.Precedence)
	}
	if len(got.Trust.LegacySources) != 1 || got.Trust.LegacySources[0].Source != "claude@doctorbox" {
		t.Errorf("legacy trust diagnostics are incomplete: %+v", got.Trust.LegacySources)
	}
	if after := doctorHostSnapshot(t, home); !sameSnapshot(beforeHome, after) {
		t.Fatalf("doctor changed HOME: before=%v after=%v", beforeHome, after)
	}
	if after := treeSnapshot(t, root); !sameSnapshot(beforeRoot, after) {
		t.Fatalf("doctor changed instance: before=%v after=%v", beforeRoot, after)
	}
}

func TestDoctorIntegrationFilter(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := t.TempDir()
	cfgPath := filepath.Join(root, "config.toml")
	if err := os.WriteFile(cfgPath, []byte("machine = \"filterbox\"\nintegrations = [\"claude\", \"codex\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, _ := runDoctorJSON(t, cfgPath, "--integration", "codex")
	if len(got.Integrations) != 1 || got.Integrations[0].ID != "codex" {
		t.Fatalf("filtered integrations = %+v", got.Integrations)
	}
}

func TestDoctorReportsDetectedButUnconfiguredProfile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, relative := range []string{".claude", ".codex"} {
		if err := os.Mkdir(filepath.Join(home, relative), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	root := t.TempDir()
	cfgPath := filepath.Join(root, "config.toml")
	if err := os.WriteFile(cfgPath, []byte("machine = \"detectbox\"\nintegrations = [\"codex\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, _ := runDoctorJSON(t, cfgPath)
	byID := map[string]bool{}
	for _, integration := range got.Integrations {
		byID[integration.ID] = integration.Configured
	}
	if configured, exists := byID["claude"]; !exists || configured {
		t.Fatalf("detected unconfigured Claude missing or misclassified: %+v", got.Integrations)
	}
	if configured, exists := byID["codex"]; !exists || !configured {
		t.Fatalf("configured Codex missing or misclassified: %+v", got.Integrations)
	}
}

func TestDoctorIsolatesPlanFailuresAndFilteredRuns(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, relative := range []string{".claude", ".codex"} {
		if err := os.Mkdir(filepath.Join(home, relative), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(home, ".claude", "settings.json"), []byte("{broken"), 0o644); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	materializeInstallHook(t, root)
	cfgPath := filepath.Join(root, "config.toml")
	if err := os.WriteFile(cfgPath, []byte("machine = \"brokenbox\"\nintegrations = [\"claude\", \"codex\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, _ := runDoctorJSON(t, cfgPath)
	if got.Status != "error" {
		t.Fatalf("global malformed-host status = %q, want error", got.Status)
	}
	for _, integration := range got.Integrations {
		if integration.ID == "codex" {
			if integration.Status == "error" || len(integration.Artifacts) == 0 {
				t.Fatalf("Claude failure poisoned Codex diagnostics: %+v", integration)
			}
		}
	}
	filtered, _ := runDoctorJSON(t, cfgPath, "--integration", "codex")
	if filtered.Status == "error" || len(filtered.Integrations) != 1 || filtered.Integrations[0].ID != "codex" || len(filtered.Integrations[0].Artifacts) == 0 {
		t.Fatalf("filtered Codex diagnostic was not isolated: %+v", filtered)
	}
}

func TestDoctorUnsupportedAndLegacyTrustAreExplicit(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, tc := range []struct {
		name string
		body string
	}{
		{name: "generic", body: "integrations = [\"generic\"]\n"},
		{name: "legacy-unknown", body: "agents = [\"mystery\"]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			cfgPath := filepath.Join(root, "config.toml")
			if err := os.WriteFile(cfgPath, []byte("machine = \"trustbox\"\n"+tc.body), 0o644); err != nil {
				t.Fatal(err)
			}
			got, _ := runDoctorJSON(t, cfgPath)
			if got.Status != "warning" || len(got.Integrations) != 1 || got.Integrations[0].Status != "warning" {
				t.Fatalf("unsupported-only diagnostic looked successful: %+v", got)
			}
			if !strings.Contains(got.Integrations[0].Capabilities.Trust.Detail, "quarantine") {
				t.Fatalf("fallback trust is not explicit: %+v", got.Integrations[0].Capabilities.Trust)
			}
		})
	}
}

func TestDoctorUnsupportedAndMalformedMemoryKinds(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, tc := range []struct {
		name, kind, location, want string
	}{
		{name: "unsupported", kind: "sqlite-v1", want: "unsupported"},
		{name: "malformed-glob", kind: "markdown-glob-v1", location: "/tmp/[", want: "error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			body := "machine = \"memorybox\"\nintegrations = [\"custom\"]\n[integrations.custom]\nprofile = \"generic\"\nmemory_kind = \"" + tc.kind + "\"\n"
			if tc.location != "" {
				body += "memory_location = \"" + tc.location + "\"\n"
			}
			cfgPath := filepath.Join(root, "config.toml")
			if err := os.WriteFile(cfgPath, []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
			got, _ := runDoctorJSON(t, cfgPath)
			if len(got.Integrations) != 1 || got.Integrations[0].Capabilities.Memory[0].Status != tc.want {
				t.Fatalf("memory status = %+v, want %s", got.Integrations, tc.want)
			}
		})
	}
}

func TestDoctorRejectsMemoryThatOverlapsKnowledgeBase(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, throughLink := range []bool{false, true} {
		name := "direct"
		if throughLink {
			name = "symlink"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			memoryLocation := root
			if throughLink {
				memoryLocation = filepath.Join(t.TempDir(), "memory-link")
				if err := os.Symlink(root, memoryLocation); err != nil {
					t.Fatal(err)
				}
			}
			body := "machine = \"overlapbox\"\nintegrations = [\"custom\"]\n[integrations.custom]\nprofile = \"generic\"\nmemory_kind = \"markdown-glob-v1\"\nmemory_location = \"" + memoryLocation + "\"\n"
			cfgPath := filepath.Join(root, "config.toml")
			if err := os.WriteFile(cfgPath, []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
			got, _ := runDoctorJSON(t, cfgPath)
			if got.Status != "error" || len(got.Integrations) != 1 || got.Integrations[0].Capabilities.Memory[0].Status != "error" {
				t.Fatalf("overlapping memory source was not rejected: %+v", got)
			}
		})
	}
}

func TestDoctorTextPrintsTrustAndGlobalRemediation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := t.TempDir()
	cfgPath := filepath.Join(root, "config.toml")
	body := "machine = \"textbox\"\nintegrations = [\"generic\"]\n[source_policies]\n\"generic@*\" = \"quarantine\"\n"
	if err := os.WriteFile(cfgPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "run", "./cmd/regesto", "--config", cfgPath, "doctor")
	cmd.Dir = repoRoot(t)
	cmd.Env = append(os.Environ(), "PATH=/usr/bin:/bin", "GOTELEMETRY=off", "GOCACHE="+t.TempDir(), "GOPATH="+t.TempDir())
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("doctor text: %v\nstderr: %s", err, stderr.String())
	}
	if !strings.Contains(stdout.String(), "trust source-policy generic@*: quarantine") || !strings.Contains(stdout.String(), "remedy:") {
		t.Fatalf("text diagnostics omit trust/remediation:\n%s", stdout.String())
	}
}

func doctorHostSnapshot(t *testing.T, home string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, relative := range []string{".claude", ".codex", ".hermes", ".other"} {
		path := filepath.Join(home, relative)
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			out[relative] = "missing"
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if !info.IsDir() {
			out[relative] = info.Mode().String()
			continue
		}
		out[relative] = "dir"
		for child, value := range treeSnapshot(t, path) {
			out[filepath.Join(relative, child)] = value
		}
	}
	return out
}

func sameSnapshot(left, right map[string]string) bool {
	if len(left) != len(right) {
		return false
	}
	for path, value := range left {
		if right[path] != value {
			return false
		}
	}
	return true
}
