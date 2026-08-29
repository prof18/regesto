// Package write owns the one validated path by which new facts enter the
// canonical store. Callers provide semantic claim fields; this package owns
// provenance, timestamps, schema version, placement, and atomic publication.
package write

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/prof18/regesto/internal/config"
	"github.com/prof18/regesto/internal/facts"
	"github.com/prof18/regesto/internal/lint"
)

var (
	idPattern    = regexp.MustCompile(`^(dec|pref|fact|pat)-[a-z0-9]+(?:-[a-z0-9]+)*$`)
	termPattern  = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
	typePrefixes = map[string]string{
		"decision": "dec-", "preference": "pref-", "fact": "fact-", "pattern": "pat-",
	}
)

// Input is the machine-facing write contract. Authority-owned fields are
// intentionally absent so JSON input cannot forge them.
type Input struct {
	ID         string   `json:"id"`
	Title      string   `json:"title"`
	Type       string   `json:"type"`
	Scope      string   `json:"scope"`
	Subject    string   `json:"subject"`
	Relation   string   `json:"relation"`
	Topics     []string `json:"topics,omitempty"`
	Status     string   `json:"status,omitempty"`
	Supersedes string   `json:"supersedes,omitempty"`
	Body       string   `json:"body"`
	Why        string   `json:"why"`
}

// Authority contains values only the invoking boundary may supply.
type Authority struct {
	Source string
	Now    time.Time
}

// PendingAction is a stable, map-free view of reconciliation work left for
// lint. Create never mutates an incumbent claim.
type PendingAction struct {
	Kind    string `json:"kind"`
	ID      string `json:"id"`
	Path    string `json:"path"`
	Message string `json:"message"`
}

// Result describes the published fact and any reconciliation still pending.
type Result struct {
	SchemaVersion         int             `json:"schema_version"`
	Path                  string          `json:"path"`
	PendingReconciliation bool            `json:"pending_reconciliation"`
	Actions               []PendingAction `json:"actions"`
	Reviews               []string        `json:"reviews"`
}

// Create validates and writes one machine-facing input object.
func Create(cfg *config.Config, in Input, authority Authority) (Result, error) {
	if strings.TrimSpace(in.Why) == "" {
		return Result{}, fmt.Errorf("why is required")
	}
	body := strings.TrimSpace(in.Body)
	if body == "" {
		return Result{}, fmt.Errorf("body is required")
	}
	body += "\n\n**Why:** " + strings.TrimSpace(in.Why)
	f := facts.Fact{
		ID: in.ID, Title: in.Title, Type: in.Type, Scope: in.Scope,
		Subject: in.Subject, Relation: in.Relation, Topics: append([]string(nil), in.Topics...),
		Status: in.Status, Supersedes: in.Supersedes, Body: body,
	}
	return CreateFact(cfg, f, authority)
}

// CreateFact is the shared normalization/promotion boundary. Any source,
// schema, or timestamps present on the proposed fact are discarded and
// replaced by Authority.
func CreateFact(cfg *config.Config, proposed facts.Fact, authority Authority) (Result, error) {
	source := strings.TrimSpace(authority.Source)
	if source == "" {
		return Result{}, fmt.Errorf("source is required")
	}
	if strings.ContainsAny(source, "\r\n\x00") {
		return Result{}, fmt.Errorf("source must fit on one line")
	}
	if source != "human" {
		if _, _, ok := config.ParseSourceID(source); !ok {
			return Result{}, fmt.Errorf("source %q must be human or <integration>@<machine>", source)
		}
	}
	now := authority.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	stamp := now.Format(time.RFC3339)

	f := facts.Fact{
		SchemaVersion: "1",
		ID:            strings.TrimSpace(proposed.ID),
		Title:         strings.TrimSpace(proposed.Title),
		Type:          strings.TrimSpace(proposed.Type),
		Scope:         strings.TrimSpace(proposed.Scope),
		Subject:       strings.TrimSpace(proposed.Subject),
		Relation:      strings.TrimSpace(proposed.Relation),
		Topics:        append([]string(nil), proposed.Topics...),
		Status:        strings.TrimSpace(proposed.Status),
		Supersedes:    strings.TrimSpace(proposed.Supersedes),
		Source:        source,
		Created:       stamp,
		Modified:      stamp,
		ReviewAfter:   strings.TrimSpace(proposed.ReviewAfter),
		Body:          strings.TrimSpace(proposed.Body),
	}
	if f.Status == "" {
		f.Status = facts.StatusActive
	}
	rel, err := validateAndPath(f)
	if err != nil {
		return Result{}, err
	}
	f.RelPath = rel
	release, err := acquireIDLock(cfg, f.ID)
	if err != nil {
		return Result{}, err
	}
	defer release()

	existing, err := facts.LoadAll(cfg.KBRoot)
	if err != nil {
		return Result{}, err
	}
	for _, old := range existing {
		if old.ID == f.ID {
			return Result{}, fmt.Errorf("id %q already exists at %s", f.ID, old.RelPath)
		}
		if f.Supersedes == old.ID {
			if !strings.EqualFold(f.Subject, old.Subject) || !strings.EqualFold(f.Relation, old.Relation) {
				return Result{}, fmt.Errorf("supersedes %q has identity (%s, %s), not (%s, %s)", old.ID, old.Subject, old.Relation, f.Subject, f.Relation)
			}
			if source != "human" && old.Source == "human" {
				f.Status = facts.StatusProposed
			}
		}
	}

	// Validate against the whole store so dangling supersedes and the shared
	// lint contract are enforced before any directory or file is created.
	report := lint.Run(append(append([]facts.Fact(nil), existing...), f), now)
	if err := candidateErrors(report, f); err != nil {
		return Result{}, err
	}

	// Agent assertions that contest a human claim are born proposed, with the
	// intent recorded. This keeps the human claim authoritative even before a
	// later lint pass applies any unrelated reconciliation.
	for _, action := range report.Actions {
		if action.Kind != "review" || action.ID != f.ID {
			continue
		}
		if status := action.Updates["status"]; status != "" {
			f.Status = status
		}
		if supersedes := action.Updates["supersedes"]; supersedes != "" {
			f.Supersedes = supersedes
		}
	}
	report = lint.Run(append(append([]facts.Fact(nil), existing...), f), now)
	if err := candidateErrors(report, f); err != nil {
		return Result{}, err
	}

	data := render(f)
	// Parse our own serialization before it can reach disk. This catches a
	// renderer/parser drift as a failed write rather than a corrupt fact.
	parsed, err := facts.Parse(data, rel)
	if err != nil {
		return Result{}, fmt.Errorf("rendered candidate is invalid: %w", err)
	}
	parsed.RelPath = rel
	if err := candidateErrors(lint.Run(append(append([]facts.Fact(nil), existing...), parsed), now), parsed); err != nil {
		return Result{}, err
	}

	target := filepath.Join(cfg.KBRoot, filepath.FromSlash(rel))
	if err := publishExclusive(cfg.KBRoot, target, data); err != nil {
		if errors.Is(err, os.ErrExist) {
			if _, statErr := os.Lstat(target); statErr == nil {
				return Result{}, fmt.Errorf("%s already exists", rel)
			}
		}
		return Result{}, err
	}

	actions := relevantActions(report, f)
	reviews := make([]string, 0)
	for _, review := range report.Reviews {
		if strings.Contains(review, f.ID) {
			reviews = append(reviews, review)
		}
	}
	if reviews == nil {
		reviews = []string{}
	}
	return Result{
		SchemaVersion:         1,
		Path:                  rel,
		PendingReconciliation: len(actions) > 0 || len(reviews) > 0,
		Actions:               actions,
		Reviews:               reviews,
	}, nil
}

// acquireIDLock serializes one fact identity across every scope and configured
// machine sharing this KB filesystem. Atomic target publication protects a
// path; this KB-wide lock also protects the schema's stronger global-ID
// invariant when two writers choose different scopes concurrently. A crashed
// writer leaves a visible lock and future
// writes fail closed after a bounded wait instead of risking a duplicate.
func acquireIDLock(cfg *config.Config, id string) (func(), error) {
	dir := filepath.Join(cfg.KBRoot, ".state", "write-locks")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, id+".lock")
	deadline := time.Now().Add(5 * time.Second)
	for {
		lock, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			if _, writeErr := fmt.Fprintf(lock, "%d\n", os.Getpid()); writeErr != nil {
				lock.Close()
				os.Remove(path)
				return nil, writeErr
			}
			if closeErr := lock.Close(); closeErr != nil {
				os.Remove(path)
				return nil, closeErr
			}
			return func() {
				_ = os.Remove(path)
			}, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("write lock for id %q is still held at %s", id, path)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func validateAndPath(f facts.Fact) (string, error) {
	if !idPattern.MatchString(f.ID) {
		return "", fmt.Errorf("id %q must be a kebab slug with a dec-, pref-, fact-, or pat- prefix", f.ID)
	}
	prefix, ok := typePrefixes[f.Type]
	if !ok {
		return "", fmt.Errorf("type %q is not decision, preference, fact, or pattern", f.Type)
	}
	if !strings.HasPrefix(f.ID, prefix) {
		return "", fmt.Errorf("type %s requires id prefix %q", f.Type, prefix)
	}
	if f.Title == "" || strings.ContainsAny(f.Title, "\r\n\x00") {
		return "", fmt.Errorf("title is required and must fit on one line")
	}
	if len([]rune(f.Title)) > 80 {
		return "", fmt.Errorf("title is longer than 80 characters")
	}
	for name, value := range map[string]string{"subject": f.Subject, "relation": f.Relation} {
		if value == "" || strings.ContainsAny(value, "\r\n\x00") {
			return "", fmt.Errorf("%s is required and must fit on one line", name)
		}
	}
	for _, topic := range f.Topics {
		if !termPattern.MatchString(topic) {
			return "", fmt.Errorf("topic %q must be a portable slug", topic)
		}
	}
	if f.Status != facts.StatusActive && f.Status != facts.StatusProposed {
		return "", fmt.Errorf("status %q must be active or proposed", f.Status)
	}
	if f.Supersedes != "" && !idPattern.MatchString(f.Supersedes) {
		return "", fmt.Errorf("supersedes %q is not a valid fact id", f.Supersedes)
	}
	if f.Body == "" {
		return "", fmt.Errorf("body is required")
	}
	if f.Scope == "global" {
		return "knowledge/facts/global/" + f.ID + ".md", nil
	}
	project, ok := strings.CutPrefix(f.Scope, "project:")
	if !ok || !safePathComponent(project) {
		return "", fmt.Errorf("scope %q must be global or project:<safe-name>", f.Scope)
	}
	return "knowledge/facts/projects/" + project + "/" + f.ID + ".md", nil
}

func safePathComponent(value string) bool {
	return value != "" && value != "." && value != ".." &&
		!strings.ContainsAny(value, "/\\\r\n\x00") && filepath.Base(value) == value
}

func candidateErrors(report *lint.Report, f facts.Fact) error {
	var messages []string
	for _, finding := range report.Findings {
		if finding.Severity == lint.Error && (finding.ID == f.ID || finding.Path == f.RelPath) {
			messages = append(messages, finding.Message)
		}
	}
	if len(messages) == 0 {
		return nil
	}
	sort.Strings(messages)
	return fmt.Errorf("candidate %q is invalid: %s", f.ID, strings.Join(messages, "; "))
}

func relevantActions(report *lint.Report, f facts.Fact) []PendingAction {
	relevantIDs := map[string]bool{f.ID: true}
	for _, action := range report.Actions {
		if action.ID == f.ID || strings.Contains(action.Message, f.ID) {
			relevantIDs[action.ID] = true
		}
	}
	out := make([]PendingAction, 0)
	for _, action := range report.Actions {
		if !relevantIDs[action.ID] {
			continue
		}
		out = append(out, PendingAction{Kind: action.Kind, ID: action.ID, Path: action.Path, Message: action.Message})
	}
	if out == nil {
		return []PendingAction{}
	}
	return out
}

func render(f facts.Fact) []byte {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("schema_version: 1\n")
	fprintf := func(key, value string) { fmt.Fprintf(&b, "%s: %s\n", key, strconv.Quote(value)) }
	fprintf("id", f.ID)
	fprintf("title", f.Title)
	fprintf("type", f.Type)
	fprintf("scope", f.Scope)
	fprintf("subject", f.Subject)
	fprintf("relation", f.Relation)
	if len(f.Topics) > 0 {
		quoted := make([]string, len(f.Topics))
		for i, topic := range f.Topics {
			quoted[i] = strconv.Quote(topic)
		}
		fmt.Fprintf(&b, "topics: [%s]\n", strings.Join(quoted, ", "))
	}
	fprintf("status", f.Status)
	if f.Supersedes != "" {
		fprintf("supersedes", f.Supersedes)
	}
	fprintf("source", f.Source)
	fprintf("created", f.Created)
	fprintf("modified", f.Modified)
	if f.ReviewAfter != "" {
		fprintf("review_after", f.ReviewAfter)
	}
	b.WriteString("---\n\n")
	b.WriteString(strings.TrimSpace(f.Body))
	b.WriteString("\n")
	return []byte(b.String())
}

// publishExclusive makes a complete same-directory temporary file visible in
// one operation. Rooted filesystem operations reject traversal and symlink
// escapes without a check-then-open race. Linking instead of renaming preserves
// O_EXCL semantics: a concurrent writer can never replace an existing fact.
func publishExclusive(kbRoot, path string, data []byte) error {
	root, err := os.OpenRoot(kbRoot)
	if err != nil {
		return err
	}
	defer root.Close()
	rel, err := filepath.Rel(kbRoot, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("fact target %s escapes knowledge base %s", path, kbRoot)
	}
	dir := filepath.Dir(rel)
	if err := root.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("fact directory is outside knowledge base: %w", err)
	}
	tmp, tmpName, err := createRootTemp(root, dir)
	if err != nil {
		return fmt.Errorf("create fact temporary file: %w", err)
	}
	defer root.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return root.Link(tmpName, rel)
}

func createRootTemp(root *os.Root, dir string) (*os.File, string, error) {
	for i := 0; i < 100; i++ {
		var random [16]byte
		if _, err := rand.Read(random[:]); err != nil {
			return nil, "", err
		}
		name := filepath.Join(dir, ".regesto-write-"+hex.EncodeToString(random[:]))
		file, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			return file, name, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, "", err
		}
	}
	return nil, "", fmt.Errorf("could not allocate a unique temporary name")
}
