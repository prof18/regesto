package main

import (
	"errors"
	"flag"
	"strings"
	"testing"

	regestoinstall "github.com/prof18/regesto/internal/install"
)

func TestHelpDoesNotRequireInstance(t *testing.T) {
	for _, command := range []string{"search", "index", "context", "config", "write", "mcp", "install", "doctor", "hook", "harvest", "init", "upgrade", "version", "promote", "cycle", "schedule", "normalize", "lint", "project"} {
		t.Run(command, func(t *testing.T) {
			err := run([]string{"--config", "/nonexistent/regesto/config.toml", command, "--help"})
			if err != nil && !errors.Is(err, flag.ErrHelp) {
				t.Fatal(err)
			}
		})
	}
	out, err := captureNormalizeStdout(t, func() error { return run([]string{"--config", "/nonexistent/regesto/config.toml", "help"}) })
	if err != nil || !strings.Contains(out, "Get started") {
		t.Fatalf("help: %s, %v", out, err)
	}
}

func TestDoctorSummaryKeepsProblemsAndVerboseKeepsHealthyArtifacts(t *testing.T) {
	report := doctorReport{Status: "warning", Remediations: []string{"Repair the missing hook."}, Integrations: []doctorIntegration{{
		ID: "aurora", DisplayName: "Aurora", Status: "warning", Configured: true,
		Capabilities: doctorCapabilities{Skills: doctorCapability{Status: "warning", Detail: "Skills directory is missing."}},
		Artifacts:    []doctorArtifact{{Kind: "hook", Action: "create", CanonicalTarget: "/missing/settings.json"}, {Kind: "skill-link", Action: "current", CanonicalTarget: "/healthy/skill"}},
	}}}
	summary, _ := captureNormalizeStdout(t, func() error { printDoctorReport(report, false); return nil })
	for _, want := range []string{"What to do", "Repair the missing hook.", "Skills directory is missing.", "1 change to make, 1 file up to date", "--verbose"} {
		if !strings.Contains(summary, want) {
			t.Errorf("missing %q in %s", want, summary)
		}
	}
	if strings.Contains(summary, "/healthy/skill") || strings.Index(summary, "What to do") > strings.Index(summary, "Agents") {
		t.Fatalf("summary buries problems: %s", summary)
	}
	verbose, _ := captureNormalizeStdout(t, func() error { printDoctorReport(report, true); return nil })
	for _, want := range []string{"/healthy/skill", "/missing/settings.json"} {
		if !strings.Contains(verbose, want) {
			t.Fatalf("verbose lost %s: %s", want, verbose)
		}
	}
}

func TestDoctorSummaryCollapsesUnconnectedAgents(t *testing.T) {
	report := doctorReport{Status: "warning", Integrations: []doctorIntegration{{
		ID: "aurora", DisplayName: "Aurora", Status: "warning",
		Capabilities: doctorCapabilities{Skills: doctorCapability{Status: "warning", Detail: "not connected"}},
	}}}
	out, _ := captureNormalizeStdout(t, func() error { printDoctorReport(report, false); return nil })
	if !strings.Contains(out, "Aurora (aurora): not connected") || strings.Contains(out, "Skills") {
		t.Fatalf("unconnected agent not summarised in one line: %s", out)
	}
}

func TestInstallSummaryGroupsChangesByAgent(t *testing.T) {
	plan := &regestoinstall.Plan{Items: []regestoinstall.Item{
		{Action: "current", CanonicalTarget: "/healthy/skill"},
		{Action: "create", Kind: "hook", Owners: []string{"claude"}, CanonicalTarget: "/missing/settings.json", CurrentState: "settings file not found", BackupAction: "none", DryRun: "create it"},
		{Action: "skip", Kind: "instructions", Owners: []string{"claude"}, CanonicalTarget: "/missing/CLAUDE.md", CurrentState: "file not found", BackupAction: "none", DryRun: "create this file yourself"},
		{Action: "create", Kind: "skill-render", Owners: []string{"claude"}, CanonicalTarget: "/kb/.state/skill/SKILL.md"},
		{Action: "create", Kind: "skill-render", Owners: []string{"claude"}, CanonicalTarget: "/kb/.state/skill/.regesto-owned"},
	}}
	out, _ := captureNormalizeStdout(t, func() error { printInstallPlan(plan); return nil })
	for _, want := range []string{"Claude Code\n", "Create   Hook", "/missing/settings.json", "Skip     Instructions", "File not found.", "To fix: create this file yourself", "Skill copies   2 files", "1 file already up to date"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in %s", want, out)
		}
	}
	for _, hidden := range []string{"/healthy/skill", ".regesto-owned", "Backup"} {
		if strings.Contains(out, hidden) {
			t.Errorf("summary shows %q: %s", hidden, out)
		}
	}
}

func TestWrapKeepsBackquotedCommandsWhole(t *testing.T) {
	out, _ := captureNormalizeStdout(t, func() error {
		printWrapped("  ", "  ", strings.Repeat("word ", 12)+"then run `regesto integrations add antigravity copilot hermes` now")
		return nil
	})
	if !strings.Contains(out, "`regesto integrations add antigravity copilot hermes`") {
		t.Fatalf("command split across lines: %q", out)
	}
}

func TestPlural(t *testing.T) {
	for n, want := range map[int]string{0: "0 facts", 1: "1 fact", 2: "2 facts"} {
		if got := plural(n, "fact"); got != want {
			t.Errorf("plural(%d) = %q, want %q", n, got, want)
		}
	}
	if got := plural(2, "entry"); got != "2 entries" {
		t.Errorf("plural entry = %q", got)
	}
}

func TestProseWrapKeepsWordsAndPaths(t *testing.T) {
	path := "/" + strings.Repeat("a", 100)
	out, _ := captureNormalizeStdout(t, func() error { printWrapped("  ", "    ", strings.Repeat("word ", 30)+path); return nil })
	if strings.Count(out, "\n") < 3 || !strings.Contains(out, path) {
		t.Fatalf("bad wrapping: %q", out)
	}
	for _, line := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
		if len(line) > 80 && !strings.Contains(line, path) {
			t.Errorf("long prose: %q", line)
		}
	}
}

func TestWrappedOutputPreservesStructuredInstructions(t *testing.T) {
	snippet := "Registration:\nhooks:\n  pre_llm_call:\n    - command: hook"
	out, _ := captureNormalizeStdout(t, func() error { printWrapped("Planned: ", "  ", snippet); return nil })
	want := "Planned: Registration:\n  hooks:\n    pre_llm_call:\n      - command: hook\n"
	if out != want {
		t.Fatalf("structured instructions changed: %q", out)
	}
}

func TestInstallSummaryMergesSkillsOwnedByAnotherKnowledgeBase(t *testing.T) {
	skip := func(name string) regestoinstall.Item {
		return regestoinstall.Item{Action: "skip", Kind: "skill-link", Owners: []string{"codex"}, CanonicalTarget: "/home/.codex/skills/" + name,
			CurrentState: "used by another knowledge base (/other-kb); left alone", DryRun: "no change"}
	}
	plan := &regestoinstall.Plan{Items: []regestoinstall.Item{skip("regesto-search"), skip("regesto-write"), skip("regesto-promote")}}
	out, _ := captureNormalizeStdout(t, func() error { printInstallPlan(plan); return nil })
	if strings.Count(out, "another knowledge base") != 1 || !strings.Contains(out, "3 skills in /home/.codex/skills") {
		t.Fatalf("repeated skips not merged: %s", out)
	}
}

func TestOtherKnowledgeBaseReadsSkillCopyOwner(t *testing.T) {
	if got := regestoinstall.OtherKnowledgeBase("/Users/me/regesto-kb/.state/integrations/claude/skills/regesto-write"); got != "/Users/me/regesto-kb" {
		t.Fatalf("owner = %q", got)
	}
	if got := regestoinstall.OtherKnowledgeBase("/Users/me/dotfiles/skills/regesto-write"); got != "" {
		t.Fatalf("non-Regesto link reported an owner: %q", got)
	}
}
