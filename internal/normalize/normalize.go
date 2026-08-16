// Package normalize converts raw inbox captures into canonical facts
// (PLAN 2.b, DESIGN §6 step 2).
//
// Normalising is a judgement task — choosing `subject` and `relation`, reusing
// existing vocabulary rather than minting a synonym — so a model does that part.
// But the model never writes to the store: it returns candidate facts, and this
// package validates them and writes the files. Write authority stays in the
// binary, so a bad or surprising response fails a check instead of landing in
// knowledge/.
//
// Trust follows the channel, not the agent (SCHEMA.md, Trust). Captures from a
// source the instance has not declared trusted are never normalised at all; they
// stay raw in the inbox, invisible to search and hooks, until a human promotes
// them. Quarantined has to mean invisible — a planted claim that reached a
// session's context before review has already done its damage.
//
// The default is deny because the safe direction is obvious: a trusted channel
// wrongly quarantined costs one line of config, while an untrusted one wrongly
// normalised costs a claim in the store that every later session believes.
package normalize

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Capture is one unconsumed file in the inbox.
type Capture struct {
	// Path is relative to the KB root.
	Path string
	// Agent and Machine come from the inbox/<agent>@<machine>/ directory.
	Agent   string
	Machine string
	// Source is the value a resulting fact carries in `source:`.
	Source string
	Body   string
	// IsDiff marks a capture that holds a unified diff rather than a whole
	// file, so only added lines are new information.
	IsDiff bool
}

// Quarantined reports whether a capture must be left untouched for a human.
//
// Only channels reachable by someone other than the user are quarantined. The
// installed agents write from the machine itself, so they are trusted like any
// supervised work; hermes is the one whose trust depends on which surface it is
// listening to, and that cannot be inferred from the capture alone.
func (c Capture) Quarantined(trustedSources map[string]bool) bool {
	if strings.HasPrefix(c.Agent, "hermes") {
		// Trusted only if the instance has explicitly declared this hermes
		// channel single-user. Absent that declaration, quarantine.
		return !trustedSources[c.Source]
	}
	return false
}

// Find lists unconsumed captures under inbox/, newest last so a run processes
// them in the order they were written.
func Find(kbRoot string) ([]Capture, error) {
	inbox := filepath.Join(kbRoot, "inbox")
	var out []Capture
	entries, err := os.ReadDir(inbox)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	for _, agentDir := range entries {
		if !agentDir.IsDir() {
			continue
		}
		agent, machine, ok := strings.Cut(agentDir.Name(), "@")
		if !ok {
			// A directory that is not <agent>@<machine> is not ours to read.
			continue
		}
		root := filepath.Join(inbox, agentDir.Name())
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || info.Name() == ".gitkeep" {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			rel, err := filepath.Rel(kbRoot, path)
			if err != nil {
				return nil
			}
			out = append(out, Capture{
				Path:    filepath.ToSlash(rel),
				Agent:   agent,
				Machine: machine,
				Source:  agent + "@" + machine,
				Body:    string(data),
				IsDiff:  strings.HasSuffix(path, ".diff"),
			})
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

// Prompt builds the normalisation request for one capture.
//
// It carries the controlled vocabulary explicitly, because reusing an existing
// (subject, relation) pair is the whole defence against undetectable
// contradictions — a model that cannot see the vocabulary will invent synonyms.
// The same applies to project names: a model that cannot see them writes
// `project:aurora-2` where the store uses `aurora`, and the project's
// knowledge quietly splits in two.
func Prompt(c Capture, vocabulary []string, existingIDs []string, projects ...string) string {
	var b strings.Builder
	b.WriteString("You are normalising a raw capture from an AI agent's native memory into\n")
	b.WriteString("canonical knowledge-base facts.\n\n")

	b.WriteString("## Rules\n\n")
	b.WriteString("- One claim per fact. Split a capture that asserts several things.\n")
	b.WriteString("- Record only what a competent stranger would need told and would struggle\n")
	b.WriteString("  to rediscover: decisions and why, conventions differing from defaults,\n")
	b.WriteString("  corrections given twice, preferences, non-obvious facts about the\n")
	b.WriteString("  environment, the tooling, or the user's own life and admin. Not only code.\n")
	b.WriteString("- Do NOT record: anything derivable from a source already at hand (the code,\n")
	b.WriteString("  official documentation), or transient state (branch names, in-flight work).\n")
	b.WriteString("- Emitting nothing is a valid and common outcome. Say so rather than\n")
	b.WriteString("  inventing a fact to justify the run.\n")
	b.WriteString("- Reuse an existing (subject, relation) pair **exactly** when one fits.\n")
	b.WriteString("  Only mint a new pair when nothing does. A synonym pair means a future\n")
	b.WriteString("  contradiction is never detected.\n")
	b.WriteString("- `scope` is `global`, or `project:<name>` when the claim is specific to one\n")
	b.WriteString("  project. Prefer global with the projects listed in `topics`.\n\n")

	if c.IsDiff {
		b.WriteString("## This capture is a unified diff\n\n")
		b.WriteString("Only added lines (`+`) are new. Context lines are shown for orientation\n")
		b.WriteString("and were already known; do not normalise them.\n\n")
	}

	b.WriteString("## Controlled vocabulary in use — prefer these\n\n")
	if len(vocabulary) == 0 {
		b.WriteString("(empty — this is the first fact)\n")
	}
	for _, v := range vocabulary {
		b.WriteString("- " + v + "\n")
	}
	b.WriteString("\n")

	if len(projects) > 0 {
		b.WriteString("## Project names in use — use one of these exactly, or `global`\n\n")
		for _, p := range projects {
			b.WriteString("- project:" + p + "\n")
		}
		b.WriteString("\nOnly invent a new project name when the claim genuinely belongs to a\n")
		b.WriteString("project not listed here. A different spelling of a listed name splits\n")
		b.WriteString("that project's knowledge in two.\n\n")
	}

	b.WriteString("## Ids already taken — yours must differ\n\n")
	b.WriteString(strings.Join(existingIDs, ", "))
	b.WriteString("\n\n")

	fmt.Fprintf(&b, "## Capture from %s (%s)\n\n", c.Source, c.Path)
	b.WriteString("```\n")
	b.WriteString(c.Body)
	if !strings.HasSuffix(c.Body, "\n") {
		b.WriteString("\n")
	}
	b.WriteString("```\n\n")

	b.WriteString("## Output format\n\n")
	b.WriteString("Emit zero or more facts, each in its own fenced block tagged `regesto-fact`,\n")
	b.WriteString("containing YAML frontmatter then the claim and a `**Why:**` line:\n\n")
	b.WriteString("```regesto-fact\n---\nschema_version: 1\nid: <prefix>-<kebab-slug>\n")
	b.WriteString("title: <one line, at most 80 chars>\ntype: decision|preference|fact|pattern\n")
	b.WriteString("scope: global\nsubject: <term>\nrelation: <term>\ntopics: [a, b]\n")
	b.WriteString("status: active\nsource: " + c.Source + "\n---\n\n<the claim>\n\n")
	b.WriteString("**Why:** <the reasoning>\n```\n\n")
	b.WriteString("Id prefixes: dec- decision, pref- preference, fact- fact, pat- pattern.\n")
	b.WriteString("Omit `created`/`modified`; they are stamped for you.\n")
	b.WriteString("If nothing in this capture is worth recording, emit no blocks at all.\n")
	return b.String()
}

// ParseFacts extracts the fenced regesto-fact blocks from a model response.
// Anything outside those fences is commentary and ignored, so a chatty response
// is not an error.
func ParseFacts(response string) []string {
	var out []string
	lines := strings.Split(strings.ReplaceAll(response, "\r\n", "\n"), "\n")
	inBlock := false
	var cur []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !inBlock {
			if strings.HasPrefix(trimmed, "```regesto-fact") {
				inBlock = true
				cur = nil
			}
			continue
		}
		if trimmed == "```" {
			inBlock = false
			body := strings.TrimSpace(strings.Join(cur, "\n"))
			if body != "" {
				out = append(out, body+"\n")
			}
			continue
		}
		cur = append(cur, line)
	}
	return out
}
