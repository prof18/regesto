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
	if err != nil || !strings.Contains(out, "Find and record knowledge") {
		t.Fatalf("help: %s, %v", out, err)
	}
}

func TestDoctorSummaryKeepsProblemsAndVerboseKeepsHealthyArtifacts(t *testing.T) {
	report := doctorReport{Status: "warning", Remediations: []string{"Repair the missing hook."}, Integrations: []doctorIntegration{{
		ID: "aurora", DisplayName: "Aurora", Status: "warning",
		Capabilities: doctorCapabilities{Skills: doctorCapability{Status: "warning", Detail: "Skills directory is missing."}},
		Artifacts:    []doctorArtifact{{Kind: "hook", Action: "create", CanonicalTarget: "/missing/settings.json"}, {Kind: "skill-link", Action: "current", CanonicalTarget: "/healthy/skill"}},
	}}}
	summary, _ := captureNormalizeStdout(t, func() error { printDoctorReport(report, false); return nil })
	for _, want := range []string{"Next steps", "Skills directory is missing.", "/missing/settings.json", "1 current", "--verbose"} {
		if !strings.Contains(summary, want) {
			t.Errorf("missing %q in %s", want, summary)
		}
	}
	if strings.Contains(summary, "/healthy/skill") || strings.Index(summary, "Next steps") > strings.Index(summary, "Integrations") {
		t.Fatalf("summary buries problems: %s", summary)
	}
	verbose, _ := captureNormalizeStdout(t, func() error { printDoctorReport(report, true); return nil })
	if !strings.Contains(verbose, "/healthy/skill") {
		t.Fatalf("verbose lost healthy path: %s", verbose)
	}
}

func TestInstallSummaryKeepsPlannedChanges(t *testing.T) {
	plan := &regestoinstall.Plan{Items: []regestoinstall.Item{
		{Action: "current", CanonicalTarget: "/healthy/skill"},
		{Action: "create", Kind: "hook", CanonicalTarget: "/missing/settings.json", CurrentState: "missing", IntendedState: "registered", BackupAction: "none", DryRun: "would register"},
	}}
	out, _ := captureNormalizeStdout(t, func() error { printInstallPlan(plan, true); return nil })
	for _, want := range []string{"would create", "/missing/settings.json", "Current: missing", "Planned: registered", "Already current: 1"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in %s", want, out)
		}
	}
	if strings.Contains(out, "/healthy/skill") {
		t.Fatalf("healthy detail not condensed: %s", out)
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
