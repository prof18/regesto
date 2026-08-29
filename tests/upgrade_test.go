// An upgrade replaces files the user may have edited. Everything here is about
// the one failure that has no recovery: overwriting an intentional edit because
// the engine could not tell it from its own stale copy.
package tests

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	regesto "github.com/prof18/regesto"
	"github.com/prof18/regesto/internal/manifest"
)

func writeAt(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func planFor(t *testing.T, root string, engine map[string][]byte, m *manifest.Manifest) map[string]manifest.Status {
	t.Helper()
	out := map[string]manifest.Status{}
	for _, c := range manifest.Plan(root, engine, m) {
		out[c.Path] = c.Status
	}
	return out
}

func TestPlanClassifiesEveryCase(t *testing.T) {
	root := t.TempDir()
	engine := map[string][]byte{
		"same.md":    []byte("v2"),
		"absent.md":  []byte("v2"),
		"stale.md":   []byte("v2"),
		"edited.md":  []byte("v2"),
		"unknown.md": []byte("v2"),
	}
	writeAt(t, root, "same.md", "v2")  // already what the engine ships
	writeAt(t, root, "stale.md", "v1") // engine moved on, nobody touched it
	writeAt(t, root, "edited.md", "mine")
	writeAt(t, root, "unknown.md", "mine")
	// absent.md is deliberately not written.

	m := &manifest.Manifest{Files: map[string]string{
		"same.md":   manifest.Sum([]byte("v2")),
		"stale.md":  manifest.Sum([]byte("v1")),
		"edited.md": manifest.Sum([]byte("v1")), // written as v1, now says "mine"
		// unknown.md has no record at all.
	}}

	got := planFor(t, root, engine, m)
	want := map[string]manifest.Status{
		"same.md":    manifest.Current,
		"absent.md":  manifest.Missing,
		"stale.md":   manifest.Stale,
		"edited.md":  manifest.Modified,
		"unknown.md": manifest.Unknown,
	}
	for path, w := range want {
		if got[path] != w {
			t.Errorf("%s: got %v, want %v", path, got[path], w)
		}
	}
}

// The safety property the whole design turns on. Only these two may be written
// without the user asking for it; the other two mean somebody's work is on disk.
func TestOnlyUntouchedFilesAreWritable(t *testing.T) {
	writable := map[manifest.Status]bool{
		manifest.Current:  false,
		manifest.Missing:  true,
		manifest.Stale:    true,
		manifest.Modified: false,
		manifest.Unknown:  false,
	}
	for status, want := range writable {
		if got := status.Writable(); got != want {
			t.Errorf("%v.Writable() = %v, want %v", status, got, want)
		}
	}
}

// An instance scaffolded before manifests existed records nothing, so a file
// that differs cannot be attributed to the engine or to the user. It must land
// as Unknown rather than Stale — the difference is whether an upgrade destroys
// an edit made before the instance was ever upgradeable.
func TestPreManifestInstanceIsNeverAssumedUntouched(t *testing.T) {
	root := t.TempDir()
	engine := map[string][]byte{"skill.md": []byte("engine version")}
	writeAt(t, root, "skill.md", "carefully hand-edited years ago")

	empty, err := manifest.Load(root) // no manifest file at all
	if err != nil {
		t.Fatalf("a missing manifest must not be an error: %v", err)
	}
	if len(empty.Files) != 0 {
		t.Fatalf("expected an empty manifest, got %d entries", len(empty.Files))
	}

	got := planFor(t, root, engine, empty)
	if got["skill.md"] != manifest.Unknown {
		t.Errorf("status = %v, want unknown — an unattributable file must not be overwritten", got["skill.md"])
	}
}

// The other half of that: a pre-manifest instance whose files happen to match
// the engine adopts a manifest with no file being touched, which is how an
// existing instance becomes upgradeable at all.
func TestUntouchedPreManifestInstanceAdoptsCleanly(t *testing.T) {
	root := t.TempDir()
	engine := map[string][]byte{"a.md": []byte("v1"), "b.md": []byte("v1")}
	writeAt(t, root, "a.md", "v1")
	writeAt(t, root, "b.md", "v1")

	empty, _ := manifest.Load(root)
	for _, c := range manifest.Plan(root, engine, empty) {
		if c.Status != manifest.Current {
			t.Errorf("%s: got %v, want current", c.Path, c.Status)
		}
	}
}

// A file a previous engine wrote and this one no longer ships. Left in place it
// keeps being rendered and linked, so a retired skill goes on instructing agents
// after the engine disowned it.
func TestWithdrawnFilesAreFoundAndClassified(t *testing.T) {
	root := t.TempDir()
	engine := map[string][]byte{"kept.md": []byte("v1")}
	writeAt(t, root, "kept.md", "v1")
	writeAt(t, root, "retired.md", "as written")
	writeAt(t, root, "retired-edited.md", "mine now")

	m := &manifest.Manifest{Files: map[string]string{
		"kept.md":           manifest.Sum([]byte("v1")),
		"retired.md":        manifest.Sum([]byte("as written")),
		"retired-edited.md": manifest.Sum([]byte("as written")),
		"already-gone.md":   manifest.Sum([]byte("whatever")),
	}}

	got := map[string]manifest.Status{}
	for _, c := range manifest.PlanRemovals(root, engine, m) {
		got[c.Path] = c.Status
	}
	want := map[string]manifest.Status{
		"retired.md":        manifest.Withdrawn,
		"retired-edited.md": manifest.WithdrawnEdited,
		"already-gone.md":   manifest.Missing,
	}
	for path, w := range want {
		if got[path] != w {
			t.Errorf("%s: got %v, want %v", path, got[path], w)
		}
	}
	// A file the engine still ships is not a removal candidate, however it looks.
	if _, found := got["kept.md"]; found {
		t.Error("a file the engine still ships was offered for removal")
	}
}

// Deletion is the one operation with no undo, so it happens only where the file
// is byte for byte what the engine recorded writing.
func TestOnlyProvablyUntouchedFilesAreRemovable(t *testing.T) {
	removable := map[manifest.Status]bool{
		manifest.Withdrawn:       true,
		manifest.WithdrawnEdited: false,
		manifest.Missing:         false,
		manifest.Current:         false,
		manifest.Stale:           false,
		manifest.Modified:        false,
		manifest.Unknown:         false,
	}
	for status, want := range removable {
		if got := status.Removable(); got != want {
			t.Errorf("%v.Removable() = %v, want %v", status, got, want)
		}
	}
}

// Anything the manifest has no hash for is not the engine's to delete — which is
// also what keeps a hand-made file next to the adapters safe.
func TestUnrecordedFilesAreNeverRemoved(t *testing.T) {
	root := t.TempDir()
	writeAt(t, root, "adapters/skills/mine/SKILL.md", "a skill I wrote myself")
	m := &manifest.Manifest{Files: map[string]string{}}

	if got := manifest.PlanRemovals(root, map[string][]byte{}, m); len(got) != 0 {
		t.Errorf("offered %d unrecorded file(s) for removal: %+v", len(got), got)
	}
}

// The manifest is a file on disk and it decides what gets deleted, so a path
// that escapes the instance is corruption rather than an instruction.
func TestRemovalRefusesPathsOutsideTheInstance(t *testing.T) {
	root := t.TempDir()
	m := &manifest.Manifest{Files: map[string]string{
		"../../etc/passwd": manifest.Sum([]byte("x")),
		"/etc/hosts":       manifest.Sum([]byte("x")),
	}}
	for _, c := range manifest.PlanRemovals(root, map[string][]byte{}, m) {
		t.Errorf("escaping path %q was considered for removal", c.Path)
	}
}

func TestManifestRoundTrips(t *testing.T) {
	root := t.TempDir()
	when := time.Date(2026, 7, 30, 9, 0, 0, 0, time.UTC)
	in := &manifest.Manifest{
		Engine:  "v1.2.3",
		Written: when,
		Files:   map[string]string{"bin/regesto-search": manifest.Sum([]byte("x")), "SCHEMA.md": manifest.Sum([]byte("y"))},
	}
	if err := manifest.Save(root, in); err != nil {
		t.Fatal(err)
	}
	out, err := manifest.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if out.Engine != in.Engine {
		t.Errorf("engine = %q, want %q", out.Engine, in.Engine)
	}
	if !out.Written.Equal(when) {
		t.Errorf("written = %v, want %v", out.Written, when)
	}
	for p, h := range in.Files {
		if out.Files[p] != h {
			t.Errorf("%s: hash %q, want %q", p, out.Files[p], h)
		}
	}
}

// init and upgrade write the same set. Two lists would drift, and the symptom
// would be a file init creates that no upgrade ever maintains.
func TestInstanceFilesCoverTheInstanceSideEngine(t *testing.T) {
	files, err := regesto.InstanceFiles()
	if err != nil {
		t.Fatal(err)
	}
	wantFiles := []string{
		"SCHEMA.md",
		"bin/regesto-search",
		"bin/regesto-index",
		"bin/regesto-config",
		"bin/regesto-project",
		"bin/regesto-context",
		"bin/regesto-write",
		"bin/regesto-install",
		"adapters/claude/hooks/session-start.sh",
		"adapters/profiles/claude.json",
		"adapters/profiles/codex.json",
		"adapters/profiles/hermes.json",
		"adapters/profiles/generic.json",
		"adapters/instructions/regesto-section.md",
		"adapters/skills/regesto-search/SKILL.md",
		"adapters/skills/regesto-write/SKILL.md",
		"adapters/skills/regesto-promote/SKILL.md",
	}
	for _, want := range wantFiles {
		if _, ok := files[want]; !ok {
			t.Errorf("%s is missing from the instance file set", want)
		}
	}
	if len(files) != len(wantFiles) {
		t.Errorf("instance file set has %d files, want exactly %d", len(files), len(wantFiles))
	}
	// The engine's own source must never be shipped into a knowledge base.
	for p := range files {
		if filepath.Ext(p) == ".go" || p == "go.mod" {
			t.Errorf("%s is engine source and does not belong in an instance", p)
		}
	}
}

// Hooks and shims are executed by agents and schedulers; a lost executable bit
// makes the hook fail silently on every session start.
func TestScriptsAreMarkedExecutable(t *testing.T) {
	for _, p := range []string{"bin/regesto-search", "bin/regesto-write", "adapters/claude/hooks/session-start.sh"} {
		if !regesto.Executable(p) {
			t.Errorf("%s should be executable", p)
		}
	}
	for _, p := range []string{"SCHEMA.md", "adapters/skills/regesto-write/SKILL.md"} {
		if regesto.Executable(p) {
			t.Errorf("%s should not be executable", p)
		}
	}
}

func TestWriteShimPrefersCheckoutSourceOverStalePathEngine(t *testing.T) {
	files, err := regesto.InstanceFiles()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	shim := filepath.Join(root, "bin", "regesto-write")
	writeAt(t, root, "bin/regesto-write", string(files["bin/regesto-write"]))
	if err := os.Chmod(shim, 0o755); err != nil {
		t.Fatal(err)
	}
	writeAt(t, root, "go.mod", "module fixture\n")

	tools := t.TempDir()
	writeAt(t, tools, "go", "#!/bin/sh\nprintf 'source:%s\\n' \"$*\"\n")
	writeAt(t, tools, "regesto", "#!/bin/sh\nprintf 'stale-path-engine\\n'\nexit 42\n")
	for _, name := range []string{"go", "regesto"} {
		if err := os.Chmod(filepath.Join(tools, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command(shim, "--source", "codex@testbox", "--json-input")
	cmd.Env = append(os.Environ(), "PATH="+tools+string(os.PathListSeparator)+os.Getenv("PATH"))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("write shim: %v\n%s", err, out)
	}
	got := string(out)
	if strings.Contains(got, "stale-path-engine") || !strings.Contains(got, "source:-C "+root+" run ./cmd/regesto --config "+filepath.Join(root, "config.toml")+" write") {
		t.Fatalf("write shim did not prefer checkout source:\n%s", got)
	}
}

// Skills and the instructions block are read by agents, and every path they name
// has to exist in an instance. This is not hypothetical: the write skill used to
// say `{{kb_root}}/bin/regesto`, which is absent whenever the engine comes from a
// release — so an agent could not resolve a fact's scope or this machine's name
// and was left to guess them, which is silent corruption of `scope:` and
// `source:` rather than an error anyone would see.
func TestAdapterFilesOnlyReferencePathsThatExist(t *testing.T) {
	files, err := regesto.InstanceFiles()
	if err != nil {
		t.Fatal(err)
	}
	ref := regexp.MustCompile(`\{\{kb_root\}\}/([A-Za-z0-9_./-]+)`)

	for path, body := range files {
		if !strings.HasPrefix(path, "adapters/") {
			continue
		}
		for _, m := range ref.FindAllStringSubmatch(string(body), -1) {
			named := strings.TrimRight(m[1], ".,;:")
			// Only bin/ entries are executables the engine has to provide;
			// knowledge/, inbox/ and the like are created per instance.
			if !strings.HasPrefix(named, "bin/") {
				continue
			}
			if _, ok := files[named]; !ok {
				t.Errorf("%s tells an agent to run %q, which no instance has", path, named)
			}
		}
	}
}
