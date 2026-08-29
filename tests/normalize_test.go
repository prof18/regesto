// Tests for PLAN 2.b. The property that matters most is that the model proposes
// but the binary disposes: a candidate that breaks a schema rule must be
// rejected rather than written, and the capture left for a retry.
package tests

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/prof18/regesto/internal/config"
	"github.com/prof18/regesto/internal/facts"
	"github.com/prof18/regesto/internal/normalize"
)

func normalizeInstance(t *testing.T) *config.Config {
	t.Helper()
	t.Setenv("REGESTO_MACHINE", "testbox")
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "knowledge", "facts", "global"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "config.toml")
	if err := os.WriteFile(path, []byte("agents = [\"claude\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func putCapture(t *testing.T, cfg *config.Config, source, name, body string) {
	t.Helper()
	dir := filepath.Join(cfg.KBRoot, "inbox", source, "20260728T120000Z")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A fake agent: a shell command that echoes a canned response, so the pipeline
// is testable without a model.
func fakeAgent(t *testing.T, response string) string {
	t.Helper()
	script := filepath.Join(t.TempDir(), "agent.sh")
	body := "#!/bin/sh\ncat >/dev/null\ncat <<'REGESTO_EOF'\n" + response + "\nREGESTO_EOF\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return script
}

// A model that cannot see the project names in use invents a spelling, and the
// project's knowledge splits in two — which is how a store ends up holding both
// aurora-2 and aurora.
func TestPromptOffersKnownProjectNames(t *testing.T) {
	c := normalize.Capture{Path: "p", Source: "claude@testbox", Body: "note"}
	got := normalize.Prompt(c, []string{"(a, b)"}, []string{"dec-x"}, "aurora", "beacon")
	for _, want := range []string{"project:aurora", "project:beacon", "splits"} {
		if !strings.Contains(got, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
	// With none known, the section is omitted rather than left empty.
	if strings.Contains(normalize.Prompt(c, nil, nil), "Project names in use") {
		t.Error("empty project list should omit the section")
	}
}

func TestParseFactsIgnoresCommentary(t *testing.T) {
	got := normalize.ParseFacts("Here is what I found.\n\n" +
		"```regesto-fact\n---\nid: dec-a\n---\n\nA claim.\n```\n\n" +
		"Some chatter in between.\n\n" +
		"```regesto-fact\n---\nid: dec-b\n---\n\nAnother.\n```\n\nThat's all.\n")
	if len(got) != 2 {
		t.Fatalf("got %d blocks, want 2: %v", len(got), got)
	}
	if !strings.Contains(got[0], "dec-a") || !strings.Contains(got[1], "dec-b") {
		t.Errorf("blocks not extracted in order: %v", got)
	}
	if strings.Contains(strings.Join(got, ""), "chatter") {
		t.Error("commentary leaked into a fact")
	}
}

func TestNormalizeWritesValidFactAndStampsTimes(t *testing.T) {
	cfg := normalizeInstance(t)
	putCapture(t, cfg, "claude@testbox", "note.md", "Gradle should always run quiet.")

	agent := fakeAgent(t, "```regesto-fact\n---\nid: pref-gradle-console-flags\n"+
		"title: Gradle runs with -q --console=plain\ntype: preference\nscope: global\n"+
		"subject: gradle\nrelation: console-flags\nstatus: active\n---\n\n"+
		"Gradle runs with `-q --console=plain`.\n\n**Why:** quieter logs.\n```")

	outcomes, err := normalize.Run(cfg, nil, normalize.Options{Commands: []string{agent}})
	if err != nil {
		t.Fatal(err)
	}
	if len(outcomes) != 1 || len(outcomes[0].Written) != 1 {
		t.Fatalf("expected one written fact, got %+v", outcomes)
	}

	written := filepath.Join(cfg.KBRoot, "knowledge", "facts", "global", "pref-gradle-console-flags.md")
	f, err := facts.ParseFile(written)
	if err != nil {
		t.Fatalf("written fact does not parse: %v", err)
	}
	// Times and source are stamped by the binary, never taken from the model —
	// a guessed date is the most common error in hand-written facts.
	if f.Created == "" || !strings.HasSuffix(f.Created, "Z") {
		t.Errorf("created not stamped as UTC: %q", f.Created)
	}
	if f.Modified != f.Created {
		t.Errorf("new fact should have modified == created, got %q/%q", f.Modified, f.Created)
	}
	if f.Source != "claude@testbox" {
		t.Errorf("source = %q, want the capture's source", f.Source)
	}
	if f.SchemaVersion != "1" {
		t.Errorf("schema_version = %q, want 1", f.SchemaVersion)
	}
}

// The model proposes; the binary disposes.
func TestInvalidCandidatesAreRejectedNotWritten(t *testing.T) {
	cases := []struct {
		name, frontmatter, wantErr string
	}{
		{
			"type and id prefix disagree",
			"id: dec-thing\ntitle: T\ntype: preference\nscope: global\nsubject: s\nrelation: r\nstatus: active",
			"prefix",
		},
		{
			"born superseded",
			"id: dec-thing\ntitle: T\ntype: decision\nscope: global\nsubject: s\nrelation: r\nstatus: superseded",
			"status",
		},
		{
			"scope is neither global nor project",
			"id: dec-thing\ntitle: T\ntype: decision\nscope: everywhere\nsubject: s\nrelation: r\nstatus: active",
			"scope",
		},
		{
			"missing relation",
			"id: dec-thing\ntitle: T\ntype: decision\nscope: global\nsubject: s\nstatus: active",
			"",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := normalizeInstance(t)
			putCapture(t, cfg, "claude@testbox", "note.md", "something")
			agent := fakeAgent(t, "```regesto-fact\n---\n"+tc.frontmatter+"\n---\n\nA claim.\n```")

			outcomes, err := normalize.Run(cfg, nil, normalize.Options{Commands: []string{agent}})
			if err != nil {
				t.Fatal(err)
			}
			if n := len(outcomes[0].Written); n != 0 {
				t.Fatalf("wrote %d fact(s) for an invalid candidate", n)
			}
			if len(outcomes[0].Rejected) == 0 {
				t.Fatal("invalid candidate was neither written nor reported as rejected")
			}
			// Nothing may have landed in the store.
			all, err := facts.LoadAll(cfg.KBRoot)
			if err != nil {
				t.Fatal(err)
			}
			if len(all) != 0 {
				t.Errorf("store gained %d fact(s) from an invalid candidate", len(all))
			}
		})
	}
}

func TestDuplicateIdIsRejected(t *testing.T) {
	cfg := normalizeInstance(t)
	existing := filepath.Join(cfg.KBRoot, "knowledge", "facts", "global", "pref-a.md")
	if err := os.WriteFile(existing, []byte("---\nschema_version: 1\nid: pref-a\ntitle: T\n"+
		"type: preference\nscope: global\nsubject: s\nrelation: r\nstatus: active\nsource: human\n"+
		"created: 2026-07-01T09:00:00Z\nmodified: 2026-07-01T09:00:00Z\n---\n\nOriginal.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	all, err := facts.LoadAll(cfg.KBRoot)
	if err != nil {
		t.Fatal(err)
	}
	putCapture(t, cfg, "claude@testbox", "note.md", "something")
	agent := fakeAgent(t, "```regesto-fact\n---\nid: pref-a\ntitle: Different\ntype: preference\n"+
		"scope: global\nsubject: s2\nrelation: r2\nstatus: active\n---\n\nA colliding claim.\n```")

	outcomes, err := normalize.Run(cfg, all, normalize.Options{Commands: []string{agent}})
	if err != nil {
		t.Fatal(err)
	}
	if len(outcomes[0].Written) != 0 {
		t.Fatal("a colliding id was written, overwriting existing knowledge")
	}
	body, err := os.ReadFile(existing)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "Original.") {
		t.Error("the existing fact was overwritten")
	}
}

// SCHEMA.md, Trust: an unknown or unconfigured source is never normalised — a
// planted claim that reaches a session's context before review has already done
// its damage.
func TestTrustQuarantinedCaptureRemainsInvisibleAndUnconsumed(t *testing.T) {
	cfg := normalizeInstance(t)
	putCapture(t, cfg, "unattended@testbox", "note.md", "Someone messaged this in.")
	agent := fakeAgent(t, "```regesto-fact\n---\nid: fact-planted\ntitle: Planted\ntype: fact\n"+
		"scope: global\nsubject: s\nrelation: r\nstatus: active\n---\n\nPlanted claim.\n```")

	outcomes, err := normalize.Run(cfg, nil, normalize.Options{Commands: []string{agent}})
	if err != nil {
		t.Fatal(err)
	}
	if len(outcomes) != 1 {
		t.Fatalf("expected one outcome, got %d", len(outcomes))
	}
	if len(outcomes[0].Written) != 0 {
		t.Fatal("a quarantined capture was normalised into the store")
	}
	if !strings.Contains(outcomes[0].Note, "quarantined") {
		t.Errorf("quarantine not reported: %q", outcomes[0].Note)
	}
	remaining, err := normalize.Find(cfg.KBRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 1 || remaining[0].Source != "unattended@testbox" {
		t.Errorf("quarantined raw capture was consumed: %+v", remaining)
	}
	all, err := facts.LoadAll(cfg.KBRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 0 {
		t.Error("quarantined content reached knowledge/, where search and hooks would see it")
	}
}

func TestTrustExactSourceOverrideAllowsNormalization(t *testing.T) {
	cfg := normalizeInstance(t)
	if err := os.WriteFile(cfg.Path, []byte("agents = [\"claude\"]\n[trusted_sources]\n\"unattended@testbox\" = \"human-approved\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var err error
	cfg, err = config.Load(cfg.Path)
	if err != nil {
		t.Fatal(err)
	}
	putCapture(t, cfg, "unattended@testbox", "note.md", "A human approved this exact source.")
	agent := fakeAgent(t, "```regesto-fact\n---\nid: fact-approved-source\ntitle: Approved source\ntype: fact\n"+
		"scope: global\nsubject: source\nrelation: approved\nstatus: active\n---\n\nApproved claim.\n```")

	outcomes, err := normalize.Run(cfg, nil, normalize.Options{Commands: []string{agent}})
	if err != nil {
		t.Fatal(err)
	}
	if len(outcomes) != 1 || len(outcomes[0].Written) != 1 || outcomes[0].Archived == "" {
		t.Fatalf("exact trusted source was not normalized and archived: %+v", outcomes)
	}
}

func TestTrustSourcePolicyPatternAllowsNormalization(t *testing.T) {
	cfg := normalizeInstance(t)
	if err := os.WriteFile(cfg.Path, []byte("agents = [\"claude\"]\n[source_policies]\n\"unattended@*\" = \"supervised\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var err error
	cfg, err = config.Load(cfg.Path)
	if err != nil {
		t.Fatal(err)
	}
	putCapture(t, cfg, "unattended@testbox", "note.md", "A source policy approved this surface.")
	agent := fakeAgent(t, "```regesto-fact\n---\nid: fact-policy-approved-source\ntitle: Policy approved source\ntype: fact\n"+
		"scope: global\nsubject: source\nrelation: policy-approved\nstatus: active\n---\n\nApproved claim.\n```")

	outcomes, err := normalize.Run(cfg, nil, normalize.Options{Commands: []string{agent}})
	if err != nil {
		t.Fatal(err)
	}
	if len(outcomes) != 1 || len(outcomes[0].Written) != 1 || outcomes[0].Archived == "" {
		t.Fatalf("source policy capture was not normalized and archived: %+v", outcomes)
	}
}

func TestTrustHumanInboxCaptureNormalizesAsHuman(t *testing.T) {
	cfg := normalizeInstance(t)
	putCapture(t, cfg, "human@testbox", "note.md", "A human recorded this directly.")
	agent := fakeAgent(t, "```regesto-fact\n---\nid: fact-human-inbox\ntitle: Human inbox\ntype: fact\n"+
		"scope: global\nsubject: human\nrelation: inbox\nstatus: active\n---\n\nHuman claim.\n```")

	outcomes, err := normalize.Run(cfg, nil, normalize.Options{Commands: []string{agent}})
	if err != nil {
		t.Fatal(err)
	}
	if len(outcomes) != 1 || len(outcomes[0].Written) != 1 {
		t.Fatalf("human capture was not normalized: %+v", outcomes)
	}
	f, err := facts.ParseFile(filepath.Join(cfg.KBRoot, outcomes[0].Written[0]))
	if err != nil {
		t.Fatal(err)
	}
	if f.Source != "human" {
		t.Errorf("human inbox source = %q, want human", f.Source)
	}
	if got := normalize.Prompt(normalize.Capture{Agent: "human", Machine: "testbox", Source: "human@testbox"}, nil, nil); !strings.Contains(got, "source: human") || strings.Contains(got, "source: human@testbox") {
		t.Errorf("human prompt source was not canonical: %q", got)
	}
}

func TestTrustExactPolicyCanQuarantineHumanInboxCapture(t *testing.T) {
	cfg := normalizeInstance(t)
	if err := os.WriteFile(cfg.Path, []byte("agents = [\"claude\"]\n[source_policies]\n\"human@testbox\" = \"quarantine\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var err error
	cfg, err = config.Load(cfg.Path)
	if err != nil {
		t.Fatal(err)
	}
	putCapture(t, cfg, "human@testbox", "note.md", "QUARANTINED HUMAN CAPTURE")

	outcomes, err := normalize.Run(cfg, nil, normalize.Options{Commands: []string{"/nonexistent/agent"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(outcomes) != 1 || !strings.Contains(outcomes[0].Note, "quarantined") {
		t.Fatalf("human quarantine policy was ignored: %+v", outcomes)
	}
	remaining, err := normalize.Find(cfg.KBRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 1 || remaining[0].Source != "human@testbox" {
		t.Errorf("quarantined human capture was consumed: %+v", remaining)
	}
	all, err := facts.LoadAll(cfg.KBRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 0 {
		t.Errorf("quarantined human capture wrote facts: %+v", all)
	}
}

func TestEmptyResponseIsNotAnError(t *testing.T) {
	cfg := normalizeInstance(t)
	putCapture(t, cfg, "claude@testbox", "note.md", "today I ran the tests")
	agent := fakeAgent(t, "Nothing in this capture is durable knowledge.")

	outcomes, err := normalize.Run(cfg, nil, normalize.Options{Commands: []string{agent}})
	if err != nil {
		t.Fatal(err)
	}
	if len(outcomes[0].Written) != 0 || len(outcomes[0].Rejected) != 0 {
		t.Errorf("expected a clean no-op, got %+v", outcomes[0])
	}
	if !strings.Contains(outcomes[0].Note, "nothing durable") {
		t.Errorf("note = %q, want it to say nothing was recorded", outcomes[0].Note)
	}
}

func TestDryRunInvokesNothing(t *testing.T) {
	cfg := normalizeInstance(t)
	putCapture(t, cfg, "claude@testbox", "note.md", "something")
	// A command that would fail loudly if it were ever run.
	outcomes, err := normalize.Run(cfg, nil, normalize.Options{
		Commands: []string{"/nonexistent/agent"}, DryRun: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(outcomes[0].Written) != 0 {
		t.Error("dry run wrote facts")
	}
	if !strings.Contains(outcomes[0].Note, "would normalise") {
		t.Errorf("dry run should report intent, got %q", outcomes[0].Note)
	}
}

// This runs unattended on a schedule, so an unexpectedly large capture must not
// become an unexpectedly large bill. Deferral leaves the capture in the inbox.
func TestOversizePromptIsDeferredNotSpent(t *testing.T) {
	cfg := normalizeInstance(t)
	putCapture(t, cfg, "claude@testbox", "huge.md", strings.Repeat("a line of captured text\n", 5000))
	// A command that would fail loudly if it were ever invoked.
	outcomes, err := normalize.Run(cfg, nil, normalize.Options{Commands: []string{"/nonexistent/agent"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(outcomes[0].Written) != 0 || len(outcomes[0].Rejected) != 0 {
		t.Fatalf("oversize capture was processed: %+v", outcomes[0])
	}
	if !strings.Contains(outcomes[0].Note, "deferred") {
		t.Errorf("note = %q, want a deferral", outcomes[0].Note)
	}
	// Still in the inbox, so raising the limit later still picks it up.
	found, err := normalize.Find(cfg.KBRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 {
		t.Error("deferred capture was consumed or lost")
	}
}

func TestCaptureCountIsBounded(t *testing.T) {
	cfg := normalizeInstance(t)
	for i := 0; i < 5; i++ {
		putCapture(t, cfg, "claude@testbox", fmt.Sprintf("n%d.md", i), "a small note")
	}
	agent := fakeAgent(t, "nothing durable here")
	outcomes, err := normalize.Run(cfg, nil, normalize.Options{Commands: []string{agent}, MaxCaptures: 2})
	if err != nil {
		t.Fatal(err)
	}
	deferred := 0
	for _, o := range outcomes {
		if strings.Contains(o.Note, "already processed this run") {
			deferred++
		}
	}
	if deferred != 3 {
		t.Errorf("expected 3 of 5 captures deferred by the per-run cap, got %d", deferred)
	}
}

// Codex and Claude bill against separate quotas, so exhausting one must fall
// through to the other — and say so, since a silent fallback would hide that
// the first choice is out.
func TestFallsBackWhenFirstProviderIsExhausted(t *testing.T) {
	cfg := normalizeInstance(t)
	putCapture(t, cfg, "claude@testbox", "note.md", "something durable")

	exhausted := filepath.Join(t.TempDir(), "exhausted.sh")
	if err := os.WriteFile(exhausted, []byte(
		"#!/bin/sh\ncat >/dev/null\necho 'Error: usage limit reached' >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	good := fakeAgent(t, "```regesto-fact\n---\nid: dec-a\ntitle: A claim\ntype: decision\n"+
		"scope: global\nsubject: s\nrelation: r\nstatus: active\n---\n\nA claim.\n```")

	outcomes, err := normalize.Run(cfg, nil, normalize.Options{Commands: []string{exhausted, good}})
	if err != nil {
		t.Fatal(err)
	}
	if len(outcomes[0].Written) != 1 {
		t.Fatalf("fallback did not produce the fact: %+v", outcomes[0])
	}
	if len(outcomes[0].Attempts) != 1 {
		t.Fatalf("the exhausted provider was not reported: %v", outcomes[0].Attempts)
	}
	if !strings.Contains(outcomes[0].Attempts[0], "quota or rate limit") {
		t.Errorf("attempt should name the reason, got %q", outcomes[0].Attempts[0])
	}
	if outcomes[0].UsedCommand != good {
		t.Errorf("used %q, want the fallback", outcomes[0].UsedCommand)
	}
}

// A provider that exits cleanly but says it is out must not have its apology
// parsed as a fact.
func TestCleanExitReportingALimitCountsAsExhausted(t *testing.T) {
	cfg := normalizeInstance(t)
	putCapture(t, cfg, "claude@testbox", "note.md", "something")
	polite := fakeAgent(t, "You have reached your usage limit for this month.")
	good := fakeAgent(t, "```regesto-fact\n---\nid: dec-b\ntitle: A claim\ntype: decision\n"+
		"scope: global\nsubject: s\nrelation: r\nstatus: active\n---\n\nA claim.\n```")

	outcomes, err := normalize.Run(cfg, nil, normalize.Options{Commands: []string{polite, good}})
	if err != nil {
		t.Fatal(err)
	}
	if len(outcomes[0].Written) != 1 || outcomes[0].UsedCommand != good {
		t.Fatalf("did not fall back past a clean-exit limit message: %+v", outcomes[0])
	}
}

func TestChainOrderAndOverride(t *testing.T) {
	// Cheapest first, then the other quota as a fallback for continuity.
	if got := normalize.Commands("", ""); len(got) != 2 ||
		!strings.HasPrefix(got[0], "claude") || !strings.HasPrefix(got[1], "codex") {
		t.Errorf("default chain should be claude then codex, got %v", got)
	}
	if got := normalize.Commands("my-agent --flag", "a ;; b"); len(got) != 1 || got[0] != "my-agent --flag" {
		t.Errorf("explicit override should win, got %v", got)
	}
	if got := normalize.Commands("", "first --x ;; second --y"); len(got) != 2 ||
		got[0] != "first --x" || got[1] != "second --y" {
		t.Errorf("configured chain not split on the separator: %v", got)
	}
}

// PLAN 2.e. A transcript is the raw source: it is archived whole and never
// edited, while only the durable claims enter knowledge/.
func TestPromoteExtractsFactsAndArchivesTranscript(t *testing.T) {
	cfg := normalizeInstance(t)
	agent := fakeAgent(t, "```regesto-fact\n---\nid: pref-a\ntitle: A preference\n"+
		"type: preference\nscope: global\nsubject: s\nrelation: r\nstatus: active\n---\n\n"+
		"The claim.\n\n**Why:** stated twice.\n```")

	transcript := "Me: do X not Y.\nAssistant: noted.\nMe: also I had lunch.\n"
	res, err := normalize.Promote(cfg, nil, transcript, normalize.PromoteOptions{
		Options: normalize.Options{Commands: []string{agent}},
		Name:    "chat.md",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Written) != 1 {
		t.Fatalf("expected one fact, got %+v", res)
	}
	if res.Archived == "" {
		t.Fatal("the transcript was not archived — the raw source must survive")
	}
	body, err := os.ReadFile(filepath.Join(cfg.KBRoot, res.Archived))
	if err != nil {
		t.Fatal(err)
	}
	// Archived verbatim, including what was not worth extracting.
	if string(body) != transcript {
		t.Errorf("the archived transcript was altered:\n%s", body)
	}
	if !strings.Contains(string(body), "lunch") {
		t.Error("the archive should hold the whole transcript, not the extract")
	}
	// Promotion is a deliberate human act, so the default source is human.
	f, err := facts.ParseFile(filepath.Join(cfg.KBRoot, res.Written[0]))
	if err != nil {
		t.Fatal(err)
	}
	if f.Source != "human" {
		t.Errorf("source = %q, want human by default", f.Source)
	}
}

// A transcript that yields nothing is a normal outcome, and the transcript is
// still kept — it is the raw source whether or not it produced a claim.
func TestPromoteWithNothingDurableStillArchives(t *testing.T) {
	cfg := normalizeInstance(t)
	agent := fakeAgent(t, "Nothing in this conversation is durable knowledge.")
	res, err := normalize.Promote(cfg, nil, "Me: hello\nAssistant: hi\n", normalize.PromoteOptions{
		Options: normalize.Options{Commands: []string{agent}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Written) != 0 {
		t.Errorf("invented %d fact(s) from small talk", len(res.Written))
	}
	if res.Archived == "" {
		t.Error("transcript not archived when nothing was extracted")
	}
}

func TestPromoteRefusesAnOversizeTranscript(t *testing.T) {
	cfg := normalizeInstance(t)
	_, err := normalize.Promote(cfg, nil, strings.Repeat("a long conversation line\n", 5000),
		normalize.PromoteOptions{Options: normalize.Options{Commands: []string{"/nonexistent/agent"}}})
	if err == nil {
		t.Fatal("an oversize transcript should be refused rather than silently spent on")
	}
	if !strings.Contains(err.Error(), "max-prompt-bytes") {
		t.Errorf("the error should say how to override it, got %v", err)
	}
}

// The limit convention has two audiences and they disagree, so the translation
// between them is worth pinning. On the command line 0 means "no limit", because
// that is the convention and what the deferral message tells you to type.
// Internally 0 must mean "unset" so that a caller omitting the field gets the
// safe default rather than an unbounded spend.
func TestLimitZeroValueIsSafeButCLIZeroMeansUnlimited(t *testing.T) {
	cfg := normalizeInstance(t)
	putCapture(t, cfg, "claude@testbox", "big.md", strings.Repeat("a line of captured text\n", 5000))

	// Omitting the field entirely must apply the default and defer.
	outcomes, err := normalize.Run(cfg, nil, normalize.Options{Commands: []string{"/nonexistent/agent"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(outcomes[0].Note, "deferred") {
		t.Errorf("an unset limit must fall back to the default, got %q", outcomes[0].Note)
	}

	// The internal no-limit form must actually lift it — the agent is then
	// reached, and fails, which proves the guard did not stop us first.
	outcomes, err = normalize.Run(cfg, nil, normalize.Options{
		Commands:       []string{"/nonexistent/agent"},
		MaxPromptBytes: -1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(outcomes[0].Note, "deferred") {
		t.Errorf("a no-limit run still deferred: %q", outcomes[0].Note)
	}
	if !strings.Contains(outcomes[0].Note, "no agent answered") {
		t.Errorf("expected the run to reach the agent, got %q", outcomes[0].Note)
	}
}
