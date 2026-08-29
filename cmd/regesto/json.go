package main

import (
	"github.com/prof18/regesto/internal/adapters"
	"github.com/prof18/regesto/internal/facts"
	"github.com/prof18/regesto/internal/project"
)

const jsonSchemaVersion = 1

type jsonFact struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Type        string   `json:"type"`
	Scope       string   `json:"scope"`
	Subject     string   `json:"subject"`
	Relation    string   `json:"relation"`
	Topics      []string `json:"topics"`
	Status      string   `json:"status"`
	Supersedes  string   `json:"supersedes"`
	Source      string   `json:"source"`
	Created     string   `json:"created"`
	Modified    string   `json:"modified"`
	ReviewAfter string   `json:"review_after"`
	Body        string   `json:"body"`
	Path        string   `json:"path"`
}

type jsonSearchResponse struct {
	SchemaVersion int        `json:"schema_version"`
	Results       []jsonFact `json:"results"`
}

func jsonFacts(all []facts.Fact) []jsonFact {
	out := make([]jsonFact, 0, len(all))
	for _, f := range all {
		topics := append([]string{}, f.Topics...)
		out = append(out, jsonFact{f.ID, f.Title, f.Type, f.Scope, f.Subject, f.Relation, topics, f.Status, f.Supersedes, f.Source, f.Created, f.Modified, f.ReviewAfter, f.Body, f.RelPath})
	}
	return out
}

type jsonResolution struct {
	How    string `json:"how"`
	Mapped bool   `json:"mapped"`
}

type jsonContextProject struct {
	Name       string         `json:"name"`
	Resolution jsonResolution `json:"resolution"`
}

type jsonContextResponse struct {
	SchemaVersion int                `json:"schema_version"`
	Context       string             `json:"context"`
	Project       jsonContextProject `json:"project"`
}

type jsonProjectResponse struct {
	SchemaVersion int            `json:"schema_version"`
	Project       string         `json:"project"`
	Scope         string         `json:"scope"`
	Resolution    jsonResolution `json:"resolution"`
}

func jsonProjectResolution(r project.Resolution) jsonResolution {
	return jsonResolution{How: r.How, Mapped: r.Mapped}
}

type jsonDetection struct {
	Paths    []string `json:"paths"`
	Commands []string `json:"commands"`
}

type jsonHook struct {
	Protocol  string `json:"protocol"`
	Registrar string `json:"registrar"`
	Settings  string `json:"settings"`
}

type jsonMemorySource struct {
	Kind     string `json:"kind"`
	Location string `json:"location"`
}

type jsonIntegration struct {
	Name               string             `json:"name"`
	ProfileID          string             `json:"profile_id"`
	DisplayName        string             `json:"display_name"`
	Detect             jsonDetection      `json:"detect"`
	SkillsDirs         []string           `json:"skills_dirs"`
	SkillsVariant      string             `json:"skills_variant"`
	InstructionsFiles  []string           `json:"instructions_files"`
	InstructionsCreate bool               `json:"instructions_create"`
	Hooks              []jsonHook         `json:"hooks"`
	MemorySources      []jsonMemorySource `json:"memory_sources"`
	DefaultTrust       string             `json:"default_trust"`
	SkillsDir          string             `json:"skills_dir"`
	InstructionsFile   string             `json:"instructions_file"`
	SettingsFile       string             `json:"settings_file"`
	MemoryGlob         string             `json:"memory_glob"`
	MaxCaptureBytes    int64              `json:"max_capture_bytes"`
	ExcludeGlobs       []string           `json:"exclude_globs"`
}

func jsonIntegrations(all []adapters.Agent) []jsonIntegration {
	out := make([]jsonIntegration, 0, len(all))
	for _, a := range all {
		hooks := make([]jsonHook, 0, len(a.Hooks))
		for _, h := range a.Hooks {
			hooks = append(hooks, jsonHook{h.Protocol, h.Registrar, h.Settings})
		}
		memory := make([]jsonMemorySource, 0, len(a.MemorySources))
		for _, m := range a.MemorySources {
			memory = append(memory, jsonMemorySource{m.Kind, m.Location})
		}
		out = append(out, jsonIntegration{
			Name: a.Name, ProfileID: a.ProfileID, DisplayName: a.DisplayName,
			Detect:     jsonDetection{Paths: append([]string{}, a.Detect.Paths...), Commands: append([]string{}, a.Detect.Commands...)},
			SkillsDirs: append([]string{}, a.SkillsDirs...), SkillsVariant: a.SkillsVariant,
			InstructionsFiles: append([]string{}, a.InstructionsFiles...), InstructionsCreate: a.InstructionsCreate,
			Hooks: hooks, MemorySources: memory, DefaultTrust: a.DefaultTrust,
			SkillsDir: a.SkillsDir, InstructionsFile: a.InstructionsFile, SettingsFile: a.SettingsFile,
			MemoryGlob: a.MemoryGlob, MaxCaptureBytes: a.MaxCaptureBytes, ExcludeGlobs: append([]string{}, a.ExcludeGlobs...),
		})
	}
	return out
}
