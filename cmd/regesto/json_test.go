package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/prof18/regesto/internal/config"
)

func jsonFactFixture(t *testing.T, cfgRoot, id, title, body string) {
	t.Helper()
	path := filepath.Join(cfgRoot, "knowledge", "facts", "global", id+".md")
	content := "---\nschema_version: 1\nid: " + id + "\ntitle: " + title + "\ntype: decision\nscope: global\nsubject: json\nrelation: contract\nstatus: active\nsource: human\ncreated: 2026-08-29T12:00:00Z\nmodified: 2026-08-29T12:00:00Z\n---\n\n" + body + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func jsonCommandConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg := normalizeTestConfig(t)
	jsonFactFixture(t, cfg.KBRoot, "dec-z-json", "Z JSON", "needle")
	jsonFactFixture(t, cfg.KBRoot, "dec-a-json", "A JSON", "needle")
	return cfg
}

func TestJSONSearchSchemaOrderingEmptyAndTextCompatibility(t *testing.T) {
	cfg := jsonCommandConfig(t)
	text, err := captureNormalizeStdout(t, func() error { return runSearch(cfg, []string{"needle"}) })
	if err != nil {
		t.Fatal(err)
	}
	wantText := "dec-a-json\tA JSON\tknowledge/facts/global/dec-a-json.md\ndec-z-json\tZ JSON\tknowledge/facts/global/dec-z-json.md\n"
	if text != wantText {
		t.Errorf("search text changed:\n got %q\nwant %q", text, wantText)
	}
	out, err := captureNormalizeStdout(t, func() error { return runSearch(cfg, []string{"--json", "needle"}) })
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		SchemaVersion int        `json:"schema_version"`
		Results       []jsonFact `json:"results"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("invalid search JSON %q: %v", out, err)
	}
	if got.SchemaVersion != jsonSchemaVersion || len(got.Results) != 2 || got.Results[0].ID != "dec-a-json" || got.Results[1].ID != "dec-z-json" || got.Results[0].Path != "knowledge/facts/global/dec-a-json.md" {
		t.Errorf("search JSON = %s", out)
	}
	empty, err := captureNormalizeStdout(t, func() error { return runSearch(cfg, []string{"--json", "missing"}) })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(empty, `"results":[]`) {
		t.Errorf("empty results must encode as []: %s", empty)
	}
}

func TestJSONContextProjectAndConfigSchemas(t *testing.T) {
	cfg := jsonCommandConfig(t)
	textContext, err := captureNormalizeStdout(t, func() error { return runContext(cfg, []string{"--project", "aurora"}) })
	if err != nil {
		t.Fatal(err)
	}
	contextOut, err := captureNormalizeStdout(t, func() error { return runContext(cfg, []string{"--json", "--project", "aurora"}) })
	if err != nil {
		t.Fatal(err)
	}
	var contextJSON struct {
		SchemaVersion int `json:"schema_version"`
		Context       string
		Project       struct {
			Name       string `json:"name"`
			Resolution struct {
				How string `json:"how"`
			} `json:"resolution"`
		} `json:"project"`
	}
	if err := json.Unmarshal([]byte(contextOut), &contextJSON); err != nil {
		t.Fatalf("invalid context JSON %q: %v", contextOut, err)
	}
	if contextJSON.SchemaVersion != jsonSchemaVersion || contextJSON.Context != textContext || contextJSON.Project.Name != "aurora" || contextJSON.Project.Resolution.How != "flag" {
		t.Errorf("context JSON = %s", contextOut)
	}

	dir := t.TempDir()
	textProject, err := captureNormalizeStdout(t, func() error { return runProject(cfg, []string{"--dir", dir}) })
	if err != nil {
		t.Fatal(err)
	}
	projectOut, err := captureNormalizeStdout(t, func() error { return runProject(cfg, []string{"--json", "--dir", dir}) })
	if err != nil {
		t.Fatal(err)
	}
	var projectJSON struct {
		SchemaVersion int    `json:"schema_version"`
		Project       string `json:"project"`
		Scope         string `json:"scope"`
	}
	if err := json.Unmarshal([]byte(projectOut), &projectJSON); err != nil {
		t.Fatalf("invalid project JSON %q: %v", projectOut, err)
	}
	if projectJSON.SchemaVersion != jsonSchemaVersion || textProject != projectJSON.Project+"\n" || projectJSON.Scope != "project:"+projectJSON.Project {
		t.Errorf("project JSON = %s; text = %q", projectOut, textProject)
	}

	configOut, err := captureNormalizeStdout(t, func() error { return runShowConfig(cfg, []string{"--json"}) })
	if err != nil {
		t.Fatal(err)
	}
	var configJSON struct {
		SchemaVersion int               `json:"schema_version"`
		Integrations  []jsonIntegration `json:"integrations"`
	}
	if err := json.Unmarshal([]byte(configOut), &configJSON); err != nil {
		t.Fatalf("invalid config JSON %q: %v", configOut, err)
	}
	if configJSON.SchemaVersion != jsonSchemaVersion || len(configJSON.Integrations) != 1 || configJSON.Integrations[0].Name != "claude" || configJSON.Integrations[0].Detect.Paths == nil || configJSON.Integrations[0].Hooks == nil {
		t.Errorf("config JSON = %s", configOut)
	}
}

func TestJSONConfigEmptyIntegrationIDsAreAnArray(t *testing.T) {
	cfg := &config.Config{KBRoot: t.TempDir(), Machine: "testbox", Sections: map[string]map[string]string{}}
	out, err := captureNormalizeStdout(t, func() error { return runShowConfig(cfg, []string{"--json"}) })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"integration_ids":[]`) || !strings.Contains(out, `"integrations":[]`) {
		t.Fatalf("empty config collections must encode as []: %s", out)
	}
}
