package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTemp(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), FileName)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRemoveIntegrationSections(t *testing.T) {
	body := "# top\n" +
		"integrations = [\"claude\", \"codex\"]\n" +
		"\n" +
		"[integrations.codex]\n" +
		"trust = \"supervised\"\n" +
		"# note inside the dropped section\n" +
		"\n" +
		"[integrations.claude]\n" +
		"skills_dir = \"~/elsewhere\"\n" +
		"\n" +
		"# [integrations.commented] is a comment, not a section\n" +
		"[projects]\n" +
		"\"a\" = \"b\"\n"
	got, dropped := RemoveIntegrationSections(body, "codex")
	if strings.Join(dropped, ",") != "codex" {
		t.Fatalf("dropped = %v, want [codex]", dropped)
	}
	if strings.Contains(got, "[integrations.codex]") || strings.Contains(got, "supervised") || strings.Contains(got, "note inside the dropped") {
		t.Fatalf("codex section not fully removed:\n%s", got)
	}
	if !strings.Contains(got, "[integrations.claude]") || !strings.Contains(got, "~/elsewhere") {
		t.Fatalf("claude section must survive:\n%s", got)
	}
	if !strings.Contains(got, "# [integrations.commented]") || !strings.Contains(got, "[projects]") || !strings.Contains(got, "integrations = [") {
		t.Fatalf("unrelated lines lost:\n%s", got)
	}
	if _, dropped := RemoveIntegrationSections(body, "hermes"); len(dropped) != 0 {
		t.Fatalf("absent id reported as dropped: %v", dropped)
	}
}

func TestRemoveIntegrationSectionsRoundTripsThroughLoad(t *testing.T) {
	body := "integrations = [\"claude\", \"codex\"]\n[integrations.codex]\ntrust = \"supervised\"\n"
	got, dropped := RemoveIntegrationSections(body, "codex")
	if len(dropped) != 1 {
		t.Fatalf("dropped = %v", dropped)
	}
	parsed, err := Load(writeTemp(t, got))
	if err != nil {
		t.Fatalf("body without the section does not parse: %v\n%s", err, got)
	}
	if strings.Join(parsed.Integrations, ",") != "claude,codex" {
		t.Fatalf("the list line must be untouched: %v", parsed.Integrations)
	}
	if _, ok := parsed.Sections["integrations.codex"]; ok {
		t.Fatal("section still parses as present")
	}
}

func TestRewriteIntegrationsReplacesActiveLineOnly(t *testing.T) {
	body := "# header comment\n" +
		"integrations = [\"claude\"]\n" +
		"\n" +
		"# a commented example must survive untouched:\n" +
		"# integrations = [\"claude\", \"my-agent\"]\n" +
		"\n[projects]\n" +
		"\"fork\" = \"upstream\"\n"
	got := RewriteIntegrations(body, []string{"claude", "codex"})
	if !strings.Contains(got, `integrations = ["claude", "codex"]`) {
		t.Fatalf("active line not replaced:\n%s", got)
	}
	if strings.Contains(got, `# integrations = ["claude", "codex"]`) {
		t.Fatalf("a comment was rewritten:\n%s", got)
	}
	if !strings.Contains(got, `# integrations = ["claude", "my-agent"]`) {
		t.Fatalf("commented example lost:\n%s", got)
	}
	if !strings.Contains(got, "# header comment") || !strings.Contains(got, `"fork" = "upstream"`) {
		t.Fatalf("unrelated content lost:\n%s", got)
	}
	if strings.Count(got, "integrations = [") != 2 { // active + commented example
		t.Fatalf("expected one active and one commented line:\n%s", got)
	}
}

func TestRewriteIntegrationsLastDuplicateWins(t *testing.T) {
	// Load takes the last active top-level line as effective; the rewriter
	// must replace the same one, or the edit would not take effect.
	body := "integrations = [\"claude\"]\nintegrations = [\"codex\"]\n"
	got := RewriteIntegrations(body, []string{"hermes"})
	if !strings.Contains(got, "integrations = [\"claude\"]\n") {
		t.Fatalf("first duplicate should be left as-is:\n%s", got)
	}
	if strings.Count(got, "integrations = [") != 2 || !strings.Contains(got, "integrations = [\"hermes\"]\n") {
		t.Fatalf("last line not replaced:\n%s", got)
	}
	if !strings.HasSuffix(got, "integrations = [\"hermes\"]\n") {
		t.Fatalf("the replaced line should be the last one:\n%s", got)
	}
}

func TestRewriteIntegrationsInsertsBeforeTablesWhenMissing(t *testing.T) {
	body := "# only comments and tables\n\n[projects]\n\"a\" = \"b\"\n"
	got := RewriteIntegrations(body, []string{"claude"})
	if !strings.HasPrefix(got, "# only comments and tables\n\nintegrations = [\"claude\"]\n[projects]") {
		t.Fatalf("line not inserted before the first table:\n%s", got)
	}
	if _, err := Load(writeTemp(t, got)); err != nil {
		t.Fatalf("inserted body does not parse: %v", err)
	}
}

func TestRewriteIntegrationsAppendsWhenBodyHasNoTables(t *testing.T) {
	got := RewriteIntegrations("# nothing here\n", []string{"codex"})
	if got != "# nothing here\nintegrations = [\"codex\"]\n" {
		t.Fatalf("unexpected body: %q", got)
	}
}

func TestRewriteIntegrationsRoundTripsThroughLoad(t *testing.T) {
	// The generated template is the realistic body; whatever the rewriter
	// produces must parse back to exactly the intended list.
	body := "integrations = []   # none chosen — add yours here\n\n[projects]\n\"a\" = \"b\"\n"
	for _, list := range [][]string{{}, {"claude"}, {"claude", "codex", "hermes"}} {
		got := RewriteIntegrations(body, list)
		parsed, err := Load(writeTemp(t, got))
		if err != nil {
			t.Fatalf("list %v: rewritten body does not parse: %v\n%s", list, err, got)
		}
		if strings.Join(parsed.Integrations, ",") != strings.Join(list, ",") {
			t.Fatalf("list %v: parsed back as %v", list, parsed.Integrations)
		}
	}
}
