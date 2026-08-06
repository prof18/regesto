package normalize

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/prof18/regesto/internal/config"
	"github.com/prof18/regesto/internal/facts"
)

// PromoteOptions controls one promotion.
type PromoteOptions struct {
	Options
	// Source is what promoted facts carry in `source:`. Promotion is a
	// deliberate human act — you chose this transcript and you review the
	// result — so it defaults to `human`, which SCHEMA.md treats as
	// authoritative and never auto-superseded by an agent claim. Override it
	// when the durable content is an agent's finding rather than your own
	// assertion.
	Source string
	// Name labels the archived transcript. Defaults to the input filename.
	Name string
	// Projects are the canonical project names in use, offered to the model so
	// it reuses one rather than inventing a spelling.
	Projects []string
}

// PromoteResult is what one promotion did.
type PromoteResult struct {
	Written     []string
	Rejected    []string
	Archived    string
	UsedCommand string
	Attempts    []string
	Note        string
}

// Promote turns a chat export into facts and archives the transcript
// (PLAN 2.e, DESIGN §5).
//
// This is the only ingestion path for the Claude and ChatGPT apps, which cannot
// participate automatically — there is no native memory to harvest and no hook
// to run. The transcript is the raw source, so it goes to archive/ immutable and
// is never edited; only the extracted claims enter knowledge/.
//
// It shares normalisation's contract: the model proposes candidates, and this
// function validates and writes them, so a surprising response fails a check
// rather than landing in the store.
func Promote(cfg *config.Config, all []facts.Fact, transcript string, opts PromoteOptions) (*PromoteResult, error) {
	res := &PromoteResult{}
	if strings.TrimSpace(transcript) == "" {
		return nil, fmt.Errorf("empty transcript")
	}

	source := strings.TrimSpace(opts.Source)
	if source == "" {
		source = "human"
	}
	name := strings.TrimSpace(opts.Name)
	if name == "" {
		name = "transcript"
	}

	vocabulary, ids := inventory(all)
	taken := map[string]bool{}
	for _, id := range ids {
		taken[id] = true
	}

	prompt := PromotePrompt(transcript, source, vocabulary, ids, opts.Projects)

	maxPrompt := resolveLimit(opts.MaxPromptBytes, DefaultMaxPromptBytes)
	if maxPrompt > 0 && len(prompt) > maxPrompt {
		return nil, fmt.Errorf("transcript makes a %d byte prompt (~%dk tokens), over the %d byte limit; "+
			"pass --max-prompt-bytes 0 to run it anyway, or split the export",
			len(prompt), len(prompt)/4000, maxPrompt)
	}

	if opts.DryRun {
		res.Note = fmt.Sprintf("would promote: %d byte prompt (~%d tokens), source %s",
			len(prompt), len(prompt)/4, source)
		return res, nil
	}

	response, used, tried, err := invokeChain(opts.Options, prompt)
	res.Attempts = tried
	if err != nil {
		return nil, fmt.Errorf("no agent answered: %w", err)
	}
	res.UsedCommand = used

	candidates := ParseFacts(response)
	if len(candidates) == 0 {
		res.Note = "nothing durable in this transcript"
	}

	stamp := time.Now().UTC().Format(time.RFC3339)
	pseudo := Capture{Path: name, Source: source}
	for _, candidate := range candidates {
		path, err := writeCandidate(cfg, candidate, pseudo, stamp, taken)
		if err != nil {
			res.Rejected = append(res.Rejected, err.Error())
			continue
		}
		res.Written = append(res.Written, path)
	}

	// The transcript is archived whatever the outcome. It is the raw source;
	// keeping it is what lets a thin extraction be re-run later against the
	// original rather than lost.
	archived, err := archiveTranscript(cfg, name, transcript)
	if err != nil {
		return res, fmt.Errorf("facts written but archiving the transcript failed: %w", err)
	}
	res.Archived = archived
	return res, nil
}

func archiveTranscript(cfg *config.Config, name, body string) (string, error) {
	dir := filepath.Join(cfg.KBRoot, "archive", "chat-exports")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	base := strings.TrimSuffix(filepath.Base(name), filepath.Ext(filepath.Base(name)))
	if base == "" || base == "." {
		base = "transcript"
	}
	stamp := time.Now().UTC().Format("2006-01-02T150405Z")
	rel := filepath.Join("archive", "chat-exports", stamp+"-"+base+".md")
	if err := os.WriteFile(filepath.Join(cfg.KBRoot, rel), []byte(body), 0o644); err != nil {
		return "", err
	}
	return filepath.ToSlash(rel), nil
}

// PromotePrompt builds the extraction request for a transcript.
//
// It differs from the capture prompt in one important way: a conversation is
// mostly not durable knowledge. Saying so plainly matters, because a model handed
// a long transcript and asked for facts will otherwise manufacture them.
func PromotePrompt(transcript, source string, vocabulary, existingIDs, projects []string) string {
	var b strings.Builder
	b.WriteString("You are extracting durable knowledge from a chat transcript into canonical\n")
	b.WriteString("knowledge-base facts.\n\n")

	b.WriteString("## What to extract\n\n")
	b.WriteString("Most of a conversation is not durable knowledge. Expect to emit few facts,\n")
	b.WriteString("often none. Do not manufacture facts to justify the exercise.\n\n")
	b.WriteString("Extract only — and not only about code:\n")
	b.WriteString("- decisions that were agreed, with the reason they were agreed\n")
	b.WriteString("- conventions that differ from a tool's default\n")
	b.WriteString("- preferences the user stated about how work should be done\n")
	b.WriteString("- non-obvious facts about the environment, the tooling, a project, or the\n")
	b.WriteString("  user's own life and admin\n")
	b.WriteString("- corrections the user gave, especially repeated ones\n\n")
	b.WriteString("Never extract: anything derivable from a source already at hand (the code,\n")
	b.WriteString("official documentation), transient state (what was being debugged, branch\n")
	b.WriteString("names, what ran today), or the assistant's own explanations.\n\n")
	b.WriteString("Test each candidate: would a competent stranger need to be told this, and\n")
	b.WriteString("would they struggle to find it out?\n\n")

	b.WriteString("## Reuse the controlled vocabulary\n\n")
	b.WriteString("Reuse an existing (subject, relation) pair **exactly** when one fits. Only\n")
	b.WriteString("mint a new pair when nothing does — a synonym pair means a future\n")
	b.WriteString("contradiction is never detected.\n\n")
	if len(vocabulary) == 0 {
		b.WriteString("(empty — this is the first fact)\n")
	}
	for _, v := range vocabulary {
		b.WriteString("- " + v + "\n")
	}
	if len(projects) > 0 {
		b.WriteString("\n## Project names in use — use one of these exactly, or `global`\n\n")
		for _, p := range projects {
			b.WriteString("- project:" + p + "\n")
		}
		b.WriteString("\nA different spelling of a listed name splits that project's knowledge.\n")
	}
	b.WriteString("\n## Ids already taken — yours must differ\n\n")
	b.WriteString(strings.Join(existingIDs, ", "))
	b.WriteString("\n\n## Transcript\n\n```\n")
	b.WriteString(transcript)
	if !strings.HasSuffix(transcript, "\n") {
		b.WriteString("\n")
	}
	b.WriteString("```\n\n")

	b.WriteString("## Output format\n\n")
	b.WriteString("Emit zero or more facts, each in its own fenced block tagged `regesto-fact`:\n\n")
	b.WriteString("```regesto-fact\n---\nschema_version: 1\nid: <prefix>-<kebab-slug>\n")
	b.WriteString("title: <one line, at most 80 chars>\ntype: decision|preference|fact|pattern\n")
	b.WriteString("scope: global\nsubject: <term>\nrelation: <term>\ntopics: [a, b]\n")
	b.WriteString("status: active\nsource: " + source + "\n---\n\n<the claim>\n\n")
	b.WriteString("**Why:** <the reasoning>\n```\n\n")
	b.WriteString("Id prefixes: dec- decision, pref- preference, fact- fact, pat- pattern.\n")
	b.WriteString("`scope` is `global`, or `project:<name>` when specific to one project.\n")
	b.WriteString("Omit `created`/`modified`; they are stamped for you.\n")
	b.WriteString("If nothing in this transcript is worth recording, emit no blocks at all.\n")
	return b.String()
}
