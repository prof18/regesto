package tests

import (
	"strings"
	"testing"

	"github.com/prof18/regesto/internal/facts"
)

func TestFactParseRejectsDuplicateFrontmatterKeys(t *testing.T) {
	base := strings.Join([]string{
		"schema_version: 1",
		"id: fact-parser-duplicates",
		"title: Parser duplicates",
		"type: fact",
		"scope: global",
		"subject: parser",
		"relation: duplicate-keys",
		"status: active",
		"source: claude@testbox",
		"created: 2026-08-29T12:00:00Z",
		"modified: 2026-08-29T12:00:00Z",
	}, "\n")

	for _, key := range []string{"source", "created", "modified", "schema_version", "custom_metadata"} {
		t.Run(key, func(t *testing.T) {
			extra := key + ": forged"
			if key == "custom_metadata" {
				extra = "custom_metadata: first\ncustom_metadata: forged"
			}
			raw := "---\n" + base + "\n" + extra + "\n---\n\nClaim.\n"
			_, err := facts.Parse([]byte(raw), "candidate")
			if err == nil || !strings.Contains(err.Error(), "duplicate frontmatter key \""+key+"\"") {
				t.Fatalf("Parse() error = %v, want duplicate %q", err, key)
			}
		})
	}
}

func TestFactParseAcceptsUniqueUnknownFrontmatterKey(t *testing.T) {
	raw := "---\n" + strings.Join([]string{
		"schema_version: 1",
		"id: fact-parser-unknown",
		"title: Parser unknown key",
		"type: fact",
		"scope: global",
		"subject: parser",
		"relation: unknown-key",
		"status: active",
		"custom_metadata: retained-for-lint",
	}, "\n") + "\n---\n\nClaim.\n"
	if _, err := facts.Parse([]byte(raw), "candidate"); err != nil {
		t.Fatalf("Parse() rejected a unique unknown key: %v", err)
	}
}
