package normalize

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"regesto/internal/config"
	"regesto/internal/facts"
)

// Options controls a normalisation pass.
type Options struct {
	// Commands is the fallback chain of agent invocations, tried in order
	// until one answers. Each reads the prompt on stdin and prints the answer.
	Commands []string
	// DryRun builds prompts and reports, but neither invokes the agent nor
	// writes anything.
	DryRun bool
	// Timeout bounds a single agent invocation.
	Timeout time.Duration
	// MaxPromptBytes defers any capture whose prompt would exceed this, rather
	// than spending on it unattended. This runs on a schedule, so an
	// unexpectedly large capture must not turn into an unexpectedly large
	// bill.
	//
	// The zero value must be the safe one, since a caller that sets nothing
	// gets it: 0 means "unset — apply the default". Negative means no limit.
	// The CLI presents the friendlier convention, translating a typed 0 into
	// this no-limit form.
	MaxPromptBytes int
	// Projects are the canonical project names in use, offered to the model so
	// it reuses one rather than inventing a spelling.
	Projects []string
	// MaxCaptures bounds one run. A burst of captures should be spread over
	// several hourly passes, not spent all at once. Same convention as above.
	MaxCaptures int
}

// Defaults chosen so an unattended pass has a predictable ceiling: roughly
// 16k tokens per call and 20 calls, i.e. a few hundred thousand tokens in the
// worst hour rather than an open-ended amount.
const (
	DefaultMaxPromptBytes = 64 * 1024
	DefaultMaxCaptures    = 20
)

// resolveLimit applies the internal convention: 0 is the zero value, meaning
// unset, so the default applies; negative means no limit. Keeping the zero value
// safe matters because any caller that omits the field gets it.
func resolveLimit(given, fallback int) int {
	switch {
	case given == 0:
		return fallback
	case given < 0:
		return 0 // no limit
	default:
		return given
	}
}

// maxPromptIDs bounds the "ids already taken" list. It exists only to help the
// model avoid a collision the binary would reject anyway, so it must not grow
// the prompt without limit as the store does.
const maxPromptIDs = 200

// Outcome is what happened to one capture.
type Outcome struct {
	Capture  Capture
	Written  []string
	Rejected []string
	Note     string
	// Archived is where the raw capture was moved once consumed, relative to
	// the KB root.
	Archived string
	// UsedCommand is the agent invocation that actually answered.
	UsedCommand string
	// Attempts records providers that were tried and gave way, so falling
	// back is visible rather than silent.
	Attempts []string
}

// DefaultCommands is the fallback chain, tried in order until one answers.
//
// Claude first because it is far cheaper per call here: the prompt is ~1.3k
// tokens, while Codex carries ~23k tokens of its own overhead per invocation
// regardless of prompt size — measured, not assumed. Routine passes should take
// the cheap path.
//
// Codex second because the two bill against separate quotas, so it keeps the
// loop running when Claude's is exhausted. Continuity is what the fallback buys;
// ordering by cost is what the first position buys.
//
// Neither is hardcoded — any command that reads a prompt on stdin and prints the
// answer works, which is what keeps this from tying the engine to a vendor.
//
// Both are asked for cheap work deliberately. Normalising is extraction, not
// reasoning: Claude runs at Haiku, and Codex at low reasoning effort. Codex is
// pinned to read-only because this task is text in, text out — it has no
// business touching the filesystem.
var DefaultCommands = []string{
	"claude -p --model claude-haiku-4-5-20251001",
	`codex exec --sandbox read-only --skip-git-repo-check -c model_reasoning_effort="low"`,
}

// CommandSeparator splits a configured chain. Commands contain spaces and
// commas, so neither works; `;;` does not occur in a shell invocation.
const CommandSeparator = ";;"

// quotaSignals are the phrases that mean "this provider is out", as opposed to a
// broken invocation. They are matched case-insensitively against both the error
// and the output, because a CLI may report exhaustion on either.
var quotaSignals = []string{
	"usage limit", "rate limit", "rate_limit", "quota", "insufficient_quota",
	"429", "too many requests", "out of credits", "limit reached",
	"upgrade to continue", "you've reached",
}

func looksLikeQuota(s string) bool {
	lower := strings.ToLower(s)
	for _, sig := range quotaSignals {
		if strings.Contains(lower, sig) {
			return true
		}
	}
	return false
}

// Commands resolves the chain: an explicit override, else the configured chain,
// else the default.
func Commands(override string, configured string) []string {
	if strings.TrimSpace(override) != "" {
		return []string{strings.TrimSpace(override)}
	}
	if strings.TrimSpace(configured) != "" {
		var out []string
		for _, part := range strings.Split(configured, CommandSeparator) {
			if p := strings.TrimSpace(part); p != "" {
				out = append(out, p)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	return DefaultCommands
}

// Run normalises every unconsumed capture.
//
// The model proposes and this function disposes: each candidate is parsed,
// checked for the schema rules that matter at write time, and only then written.
// A capture whose candidates all fail is left in the inbox, so nothing is lost
// and the next pass can retry it.
func Run(cfg *config.Config, all []facts.Fact, opts Options) ([]Outcome, error) {
	captures, err := Find(cfg.KBRoot)
	if err != nil {
		return nil, err
	}
	if len(captures) == 0 {
		return nil, nil
	}

	trusted := map[string]bool{}
	for src := range cfg.Section("trusted_sources") {
		trusted[src] = true
	}

	vocabulary, ids := inventory(all)
	taken := map[string]bool{}
	for _, id := range ids {
		taken[id] = true
	}

	maxPrompt := resolveLimit(opts.MaxPromptBytes, DefaultMaxPromptBytes)
	maxCaptures := resolveLimit(opts.MaxCaptures, DefaultMaxCaptures)

	var outcomes []Outcome
	processed := 0
	for _, c := range captures {
		out := Outcome{Capture: c}

		if c.Quarantined(trusted) {
			out.Note = "quarantined — reachable by third parties; left raw for a human to promote"
			outcomes = append(outcomes, out)
			continue
		}

		prompt := Prompt(c, vocabulary, ids, opts.Projects...)

		// Both guards leave the capture in the inbox, so nothing is lost —
		// it is picked up by a later pass, or by a run that raises the limit.
		if maxPrompt > 0 && len(prompt) > maxPrompt {
			out.Note = fmt.Sprintf("deferred — %d byte prompt (~%dk tokens) exceeds the %d byte limit; "+
				"run `regesto normalize --max-prompt-bytes 0` to process it deliberately",
				len(prompt), len(prompt)/4000, maxPrompt)
			outcomes = append(outcomes, out)
			continue
		}
		if maxCaptures > 0 && processed >= maxCaptures {
			out.Note = fmt.Sprintf("deferred — %d capture(s) already processed this run; the rest wait for the next pass", processed)
			outcomes = append(outcomes, out)
			continue
		}

		if opts.DryRun {
			out.Note = fmt.Sprintf("would normalise (%d byte prompt, ~%d tokens)", len(prompt), len(prompt)/4)
			outcomes = append(outcomes, out)
			continue
		}
		processed++

		response, used, tried, err := invokeChain(opts, prompt)
		// Report every provider that was tried and why it gave way, so a
		// silent fallback never hides that the first choice is exhausted.
		for _, t := range tried {
			out.Attempts = append(out.Attempts, t)
		}
		if err != nil {
			out.Note = "no agent answered: " + err.Error()
			outcomes = append(outcomes, out)
			continue
		}
		out.UsedCommand = used

		candidates := ParseFacts(response)
		if len(candidates) == 0 {
			out.Note = "nothing durable in this capture"
		}

		stamp := time.Now().UTC().Format(time.RFC3339)
		for _, candidate := range candidates {
			path, err := writeCandidate(cfg, candidate, c, stamp, taken)
			if err != nil {
				out.Rejected = append(out.Rejected, err.Error())
				continue
			}
			out.Written = append(out.Written, path)
		}

		// Consumed captures move to archive/, which is immutable and never
		// deleted, so a bad normalisation can be re-run against the original
		// (PLAN 2.d). Without this the scheduled loop would re-read the same
		// capture every hour, paying for the model each time and colliding on
		// ids it already wrote.
		//
		// A capture with rejected candidates stays put: leaving it is what
		// makes the retry on the next pass meaningful.
		if len(out.Rejected) == 0 {
			archived, err := archive(cfg, c)
			if err != nil {
				out.Rejected = append(out.Rejected, "archiving failed: "+err.Error())
			} else {
				out.Archived = archived
			}
		}
		outcomes = append(outcomes, out)
	}
	return outcomes, nil
}

// archive moves a consumed capture under archive/inbox/<date>/, preserving the
// agent@machine namespace so provenance survives.
func archive(cfg *config.Config, c Capture) (string, error) {
	date := time.Now().UTC().Format("2006-01-02")
	relDir := filepath.Join("archive", "inbox", date, c.Agent+"@"+c.Machine)
	destDir := filepath.Join(cfg.KBRoot, relDir)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", err
	}

	name := filepath.Base(c.Path)
	// Captures are timestamped directories; keep that stamp in the archived
	// name so two captures of one file never collide.
	if parent := filepath.Base(filepath.Dir(c.Path)); parent != "" {
		name = parent + "__" + name
	}
	dest := filepath.Join(destDir, name)
	src := filepath.Join(cfg.KBRoot, c.Path)

	if err := os.Rename(src, dest); err != nil {
		// Rename fails across filesystems; fall back to copy-then-remove.
		data, readErr := os.ReadFile(src)
		if readErr != nil {
			return "", readErr
		}
		if writeErr := os.WriteFile(dest, data, 0o644); writeErr != nil {
			return "", writeErr
		}
		if rmErr := os.Remove(src); rmErr != nil {
			return "", rmErr
		}
	}
	pruneEmptyDirs(filepath.Join(cfg.KBRoot, "inbox"), filepath.Dir(src))
	return filepath.ToSlash(filepath.Join(relDir, name)), nil
}

// pruneEmptyDirs removes now-empty capture directories, stopping at the inbox
// root so the agent@machine directories themselves survive.
func pruneEmptyDirs(stopAt, dir string) {
	for strings.HasPrefix(dir, stopAt) && dir != stopAt {
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) > 0 {
			return
		}
		if err := os.Remove(dir); err != nil {
			return
		}
		dir = filepath.Dir(dir)
	}
}

// writeCandidate validates one proposed fact and writes it, returning the path
// relative to the KB root.
func writeCandidate(cfg *config.Config, candidate string, c Capture, stamp string, taken map[string]bool) (string, error) {
	// Stamp the times here rather than trusting the model with them: a guessed
	// date is the most common error in hand-written facts, and `modified` drives
	// reconciliation.
	candidate = ensureField(candidate, "created", stamp)
	candidate = ensureField(candidate, "modified", stamp)
	candidate = ensureField(candidate, "source", c.Source)
	candidate = ensureField(candidate, "schema_version", "1")

	f, err := facts.Parse([]byte(candidate), c.Path)
	if err != nil {
		return "", fmt.Errorf("unparseable candidate from %s: %w", c.Path, err)
	}
	if taken[f.ID] {
		return "", fmt.Errorf("id %q already exists — candidate discarded", f.ID)
	}
	prefixes := map[string]string{"decision": "dec-", "preference": "pref-", "fact": "fact-", "pattern": "pat-"}
	prefix, ok := prefixes[f.Type]
	if !ok {
		return "", fmt.Errorf("candidate %q has type %q, which is not a schema type", f.ID, f.Type)
	}
	if !strings.HasPrefix(f.ID, prefix) {
		return "", fmt.Errorf("candidate %q has type %s but not the %q prefix", f.ID, f.Type, prefix)
	}
	if f.Subject == "" || f.Relation == "" {
		return "", fmt.Errorf("candidate %q is missing subject or relation", f.ID)
	}
	// An agent-sourced claim may not be born superseded, and may not declare
	// itself the winner of a contest it has not been through — reconciliation
	// decides that (SCHEMA.md, Superseding).
	if f.Status != facts.StatusActive && f.Status != facts.StatusProposed {
		return "", fmt.Errorf("candidate %q has status %q; normalisation may only produce active or proposed", f.ID, f.Status)
	}

	dir := filepath.Join(cfg.KBRoot, "knowledge", "facts", "global")
	rel := "knowledge/facts/global/" + f.ID + ".md"
	if project := f.ProjectName(); project != "" {
		dir = filepath.Join(cfg.KBRoot, "knowledge", "facts", "projects", project)
		rel = "knowledge/facts/projects/" + project + "/" + f.ID + ".md"
	} else if f.Scope != "global" {
		return "", fmt.Errorf("candidate %q has scope %q, which is neither global nor project:<name>", f.ID, f.Scope)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	target := filepath.Join(dir, f.ID+".md")
	if _, err := os.Stat(target); err == nil {
		return "", fmt.Errorf("%s already exists — candidate discarded", rel)
	}
	if err := os.WriteFile(target, []byte(candidate), 0o644); err != nil {
		return "", err
	}
	taken[f.ID] = true
	return rel, nil
}

// ensureField adds a frontmatter field when the candidate omitted it, and
// overwrites it when present — the caller is authoritative for these.
func ensureField(candidate, key, value string) string {
	lines := strings.Split(candidate, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return candidate
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			// Not found in the block: insert before the closing marker.
			out := append([]string{}, lines[:i]...)
			out = append(out, key+": "+value)
			return strings.Join(append(out, lines[i:]...), "\n")
		}
		if k, _, ok := strings.Cut(lines[i], ":"); ok && strings.TrimSpace(k) == key {
			lines[i] = key + ": " + value
			return strings.Join(lines, "\n")
		}
	}
	return candidate
}

func inventory(all []facts.Fact) (vocabulary, ids []string) {
	seen := map[string]bool{}
	for _, f := range all {
		pair := fmt.Sprintf("(%s, %s)", f.Subject, f.Relation)
		if !seen[pair] {
			seen[pair] = true
			vocabulary = append(vocabulary, pair)
		}
		ids = append(ids, f.ID)
	}
	// The vocabulary is load-bearing and stays whole — reusing a pair is the
	// only defence against undetectable contradictions. The id list is a
	// convenience the binary enforces anyway, so it is capped rather than
	// allowed to grow the prompt linearly with the store.
	if len(ids) > maxPromptIDs {
		ids = ids[len(ids)-maxPromptIDs:]
	}
	return vocabulary, ids
}

// invokeChain tries each command until one answers, returning the response, the
// command that produced it, and a note for each that gave way.
func invokeChain(opts Options, prompt string) (response, used string, tried []string, err error) {
	commands := opts.Commands
	if len(commands) == 0 {
		commands = DefaultCommands
	}
	var last error
	for i, command := range commands {
		out, err := invoke(opts, command, prompt)
		if err == nil && !looksLikeQuota(out) {
			return out, command, tried, nil
		}
		reason := "failed"
		if err != nil {
			reason = err.Error()
			if looksLikeQuota(reason) {
				reason = "quota or rate limit reached"
			}
		} else {
			// Exited cleanly but said it was out; treat as exhausted rather
			// than parsing an apology as a fact.
			reason = "quota or rate limit reached"
			err = fmt.Errorf("%s reported a usage limit", name(command))
		}
		last = err
		if i < len(commands)-1 {
			tried = append(tried, fmt.Sprintf("%s gave way (%s) — falling back to %s",
				name(command), reason, name(commands[i+1])))
		} else {
			tried = append(tried, fmt.Sprintf("%s gave way (%s) — no fallback left", name(command), reason))
		}
	}
	return "", "", tried, last
}

// name is the leading word of a command, for readable reporting.
func name(command string) string {
	if f := strings.Fields(command); len(f) > 0 {
		return f[0]
	}
	return command
}

func invoke(opts Options, command, prompt string) (string, error) {
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return "", fmt.Errorf("empty normalize command")
	}
	cmd := exec.Command(parts[0], parts[1:]...)
	cmd.Stdin = strings.NewReader(prompt)
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf

	if err := cmd.Start(); err != nil {
		return "", err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 5 * time.Minute
	}
	select {
	case err := <-done:
		if err != nil {
			return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(errBuf.String()))
		}
	case <-time.After(timeout):
		_ = cmd.Process.Kill()
		return "", fmt.Errorf("timed out after %s", timeout)
	}
	return out.String(), nil
}
