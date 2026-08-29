package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/prof18/regesto/internal/adapters"
	"github.com/prof18/regesto/internal/config"
	regestoinstall "github.com/prof18/regesto/internal/install"
)

type doctorReport struct {
	SchemaVersion int                 `json:"schema_version"`
	Status        string              `json:"status"`
	KBRoot        string              `json:"kb_root"`
	Config        string              `json:"config"`
	Machine       doctorMachine       `json:"machine"`
	Integrations  []doctorIntegration `json:"integrations"`
	Trust         doctorTrust         `json:"trust"`
	Checks        []doctorCheck       `json:"checks"`
	Remediations  []string            `json:"remediations"`
}

type doctorMachine struct {
	Name   string `json:"name"`
	Source string `json:"source"`
}

type doctorIntegration struct {
	ID           string             `json:"id"`
	ProfileID    string             `json:"profile_id"`
	DisplayName  string             `json:"display_name"`
	Configured   bool               `json:"configured"`
	Detected     bool               `json:"detected"`
	Status       string             `json:"status"`
	Capabilities doctorCapabilities `json:"capabilities"`
	Artifacts    []doctorArtifact   `json:"artifacts"`
	Remediations []string           `json:"remediations"`
}

type doctorCapabilities struct {
	Detection    doctorCapability       `json:"detection"`
	Skills       doctorCapability       `json:"skills"`
	Instructions doctorCapability       `json:"instructions"`
	Hooks        []doctorHookCapability `json:"hooks"`
	Memory       []doctorMemorySource   `json:"memory"`
	Trust        doctorCapability       `json:"trust"`
}

type doctorCapability struct {
	Status string `json:"status"`
	Detail string `json:"detail"`
}

type doctorHookCapability struct {
	Protocol  string `json:"protocol"`
	Registrar string `json:"registrar"`
	Settings  string `json:"settings,omitempty"`
	Status    string `json:"status"`
	Detail    string `json:"detail"`
}

type doctorMemorySource struct {
	Kind     string `json:"kind"`
	Location string `json:"location,omitempty"`
	Status   string `json:"status"`
	Matches  int    `json:"matches"`
	Detail   string `json:"detail"`
}

type doctorArtifact struct {
	ID              string   `json:"id"`
	Kind            string   `json:"kind"`
	Action          string   `json:"action"`
	DeclaredTargets []string `json:"declared_targets"`
	CanonicalTarget string   `json:"canonical_target"`
	CurrentState    string   `json:"current_state"`
	IntendedState   string   `json:"intended_state"`
	Remediation     string   `json:"remediation"`
}

type doctorTrust struct {
	Precedence     []string          `json:"precedence"`
	SourcePolicies []doctorTrustRule `json:"source_policies"`
	LegacySources  []doctorTrustRule `json:"legacy_trusted_sources"`
}

type doctorTrustRule struct {
	Source  string `json:"source"`
	Trust   string `json:"trust,omitempty"`
	Pattern bool   `json:"pattern,omitempty"`
	Reason  string `json:"reason,omitempty"`
}

type doctorCheck struct {
	ID          string `json:"id"`
	Status      string `json:"status"`
	Message     string `json:"message"`
	Remediation string `json:"remediation,omitempty"`
}

func runDoctor(cfg *config.Config, args []string) error {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	integrationID := fs.String("integration", "", "report only one configured or detected integration")
	jsonOutput := fs.Bool("json", false, "print the versioned diagnostic report as JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("doctor takes no positional arguments")
	}
	report, err := buildDoctorReport(cfg, *integrationID)
	if err != nil {
		return err
	}
	if *jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetEscapeHTML(false)
		return enc.Encode(report)
	}
	printDoctorReport(report)
	return nil
}

func buildDoctorReport(cfg *config.Config, filter string) (doctorReport, error) {
	agents, err := adapters.Resolve(cfg)
	if err != nil {
		return doctorReport{}, err
	}
	sort.Slice(agents, func(i, j int) bool { return agents[i].Name < agents[j].Name })

	report := doctorReport{
		SchemaVersion: jsonSchemaVersion,
		Status:        "ok",
		KBRoot:        cfg.KBRoot,
		Config:        cfg.Path,
		Machine:       doctorMachine{Name: cfg.Machine, Source: cfg.MachineSource},
		Integrations:  []doctorIntegration{},
		Checks:        []doctorCheck{},
		Remediations:  []string{},
	}
	if info, statErr := os.Stat(cfg.KBRoot); statErr != nil || !info.IsDir() {
		message := "knowledge-base root is not an accessible directory"
		if statErr != nil {
			message += ": " + statErr.Error()
		}
		addDoctorCheck(&report, doctorCheck{ID: "kb-root", Status: "error", Message: message, Remediation: "Correct kb_root in config.toml and rerun regesto doctor."})
	} else {
		addDoctorCheck(&report, doctorCheck{ID: "kb-root", Status: "ok", Message: "knowledge-base root is accessible"})
	}

	detectedIDs, detectErr := adapters.DetectProfiles(cfg.KBRoot)
	detected := map[string]bool{}
	if detectErr != nil {
		addDoctorCheck(&report, doctorCheck{ID: "profile-detection", Status: "error", Message: detectErr.Error(), Remediation: "Repair adapters/profiles and rerun regesto doctor."})
	} else {
		for _, id := range detectedIDs {
			detected[id] = true
		}
	}

	configured := map[string]adapters.Agent{}
	configuredProfiles := map[string]bool{}
	for _, agent := range agents {
		configured[agent.Name] = agent
		configuredProfiles[agent.ProfileID] = true
	}
	type diagnosticInput struct {
		agent      adapters.Agent
		configured bool
	}
	inputs := make([]diagnosticInput, 0, len(agents)+len(detectedIDs))
	for _, agent := range agents {
		if filter == "" || agent.Name == filter {
			inputs = append(inputs, diagnosticInput{agent: agent, configured: true})
		}
	}
	for _, id := range detectedIDs {
		if _, exists := configured[id]; exists || configuredProfiles[id] || (filter != "" && id != filter) {
			continue
		}
		resolved, resolveErr := adapters.Resolve(doctorScopedConfig(cfg, id, false))
		if resolveErr != nil {
			addDoctorCheck(&report, doctorCheck{ID: "detected-profile-" + id, Status: "error", Message: resolveErr.Error(), Remediation: "Repair the detected profile metadata and rerun regesto doctor."})
			continue
		}
		if len(resolved) == 1 {
			inputs = append(inputs, diagnosticInput{agent: resolved[0], configured: false})
		}
	}
	if filter != "" && len(inputs) == 0 {
		return doctorReport{}, fmt.Errorf("integration %q is neither configured nor detected", filter)
	}
	sort.Slice(inputs, func(i, j int) bool { return inputs[i].agent.Name < inputs[j].agent.Name })

	plans := map[string]*regestoinstall.Plan{}
	planErrors := map[string]error{}
	configuredInputs := make([]diagnosticInput, 0, len(inputs))
	for _, input := range inputs {
		if input.configured {
			configuredInputs = append(configuredInputs, input)
		}
	}
	if len(configuredInputs) == 0 {
		addDoctorCheck(&report, doctorCheck{ID: "install-plan", Status: "ok", Message: "no configured integration artifacts selected for planning"})
	} else {
		planCfg := cfg
		if filter != "" {
			planCfg = doctorScopedConfig(cfg, filter, cfg.UsesLegacyAgents())
		}
		globalPlan, globalErr := regestoinstall.Build(planCfg, regestoinstall.Options{})
		if globalErr == nil {
			for _, input := range configuredInputs {
				plans[input.agent.Name] = globalPlan
			}
			addDoctorCheck(&report, doctorCheck{ID: "install-plan", Status: "ok", Message: "integration artifacts can be planned without writing"})
		} else {
			addDoctorCheck(&report, doctorCheck{ID: "install-plan", Status: "error", Message: globalErr.Error(), Remediation: "Resolve the reported host artifact or target conflict, then run regesto install --dry-run."})
			for _, input := range configuredInputs {
				individual, individualErr := regestoinstall.Build(doctorScopedConfig(cfg, input.agent.Name, cfg.UsesLegacyAgents()), regestoinstall.Options{})
				plans[input.agent.Name], planErrors[input.agent.Name] = individual, individualErr
			}
		}
	}

	canonicalKBRoot := cfg.KBRoot
	if resolved, resolveErr := filepath.EvalSymlinks(cfg.KBRoot); resolveErr == nil {
		if absolute, absoluteErr := filepath.Abs(resolved); absoluteErr == nil {
			canonicalKBRoot = absolute
		}
	}
	for _, input := range inputs {
		integration := diagnoseIntegration(input.agent, detected[input.agent.ProfileID], input.configured, canonicalKBRoot, plans[input.agent.Name], planErrors[input.agent.Name])
		report.Integrations = append(report.Integrations, integration)
		if statusRank(integration.Status) > statusRank(report.Status) {
			report.Status = integration.Status
		}
		for _, remediation := range integration.Remediations {
			report.Remediations = appendUnique(report.Remediations, remediation)
		}
	}
	report.Trust = describeTrust(cfg)
	sort.Strings(report.Remediations)
	return report, nil
}

func doctorScopedConfig(cfg *config.Config, id string, legacy bool) *config.Config {
	clone := *cfg
	clone.Agents, clone.Integrations = nil, nil
	if legacy {
		clone.Agents = []string{id}
	} else {
		clone.Integrations = []string{id}
	}
	clone.Sections = make(map[string]map[string]string, len(cfg.Sections))
	for section, values := range cfg.Sections {
		if strings.HasPrefix(section, "integrations.") && section != "integrations."+id {
			continue
		}
		copyValues := make(map[string]string, len(values))
		for key, value := range values {
			copyValues[key] = value
		}
		clone.Sections[section] = copyValues
	}
	return &clone
}

func diagnoseIntegration(agent adapters.Agent, detected, configured bool, kbRoot string, plan *regestoinstall.Plan, planErr error) doctorIntegration {
	d := doctorIntegration{
		ID: agent.Name, ProfileID: agent.ProfileID, DisplayName: agent.DisplayName,
		Configured: configured, Detected: detected, Status: "ok", Artifacts: []doctorArtifact{}, Remediations: []string{},
	}
	if len(agent.Detect.Paths) == 0 && len(agent.Detect.Commands) == 0 {
		d.Capabilities.Detection = doctorCapability{Status: "unsupported", Detail: "profile declares no automatic detection signals"}
	} else if detected {
		d.Capabilities.Detection = doctorCapability{Status: "ok", Detail: "profile path or command signal detected"}
	} else {
		d.Capabilities.Detection = doctorCapability{Status: "warning", Detail: "configured, but no declared path or command signal was detected"}
		d.Remediations = append(d.Remediations, "Verify that "+agent.DisplayName+" is installed, or correct its profile detection metadata.")
	}

	if len(agent.SkillsDirs) == 0 {
		d.Capabilities.Skills = doctorCapability{Status: "unsupported", Detail: "profile declares no skills target"}
	} else {
		d.Capabilities.Skills = doctorCapability{Status: "ok", Detail: fmt.Sprintf("%s variant for %d target(s)", agent.SkillsVariant, len(agent.SkillsDirs))}
	}
	if len(agent.InstructionsFiles) == 0 {
		d.Capabilities.Instructions = doctorCapability{Status: "unsupported", Detail: "profile declares no instructions target"}
	} else {
		d.Capabilities.Instructions = doctorCapability{Status: "ok", Detail: fmt.Sprintf("%d declared target(s); create=%t", len(agent.InstructionsFiles), agent.InstructionsCreate)}
	}
	for _, hook := range agent.Hooks {
		status, detail := "ok", "registration is current"
		if hook.Protocol == "none" || hook.Registrar == "none" {
			status, detail = "unsupported", "profile declares no hook capability"
		}
		d.Capabilities.Hooks = append(d.Capabilities.Hooks, doctorHookCapability{Protocol: hook.Protocol, Registrar: hook.Registrar, Settings: hook.Settings, Status: status, Detail: detail})
	}
	if len(d.Capabilities.Hooks) == 0 {
		d.Capabilities.Hooks = []doctorHookCapability{{Protocol: "none", Registrar: "none", Status: "unsupported", Detail: "profile declares no hook capability"}}
	}
	for _, source := range agent.MemorySources {
		memory := inspectMemorySource(source, agent.Name, kbRoot)
		d.Capabilities.Memory = append(d.Capabilities.Memory, memory)
		if memory.Status == "warning" || memory.Status == "error" {
			d.Remediations = appendUnique(d.Remediations, "Verify the "+agent.Name+" memory location or set memory_kind and memory_location under [integrations."+agent.Name+"].")
		}
	}
	if len(d.Capabilities.Memory) == 0 {
		d.Capabilities.Memory = []doctorMemorySource{{Kind: "none", Status: "unsupported", Detail: "profile declares no memory source"}}
	}
	defaultTrust := agent.DefaultTrust
	if defaultTrust == "" {
		defaultTrust = "quarantine"
	}
	d.Capabilities.Trust = doctorCapability{Status: "ok", Detail: "new captures default to " + defaultTrust + "; exact, legacy, and pattern source policies take precedence"}

	if !configured {
		d.Status = "warning"
		d.Remediations = appendUnique(d.Remediations, "Add "+agent.Name+" to integrations = [...] before installing its declared capabilities.")
		markPlannedCapabilities(&d, "warning", "detected profile is not configured; artifacts were not planned")
		sort.Strings(d.Remediations)
		return d
	}

	if planErr != nil {
		d.Status = "error"
		d.Remediations = appendUnique(d.Remediations, "Resolve the install-plan error and rerun regesto doctor --integration "+agent.Name+".")
		markPlannedCapabilities(&d, "error", "install plan could not be evaluated")
		return d
	}
	if plan == nil {
		d.Status = "error"
		d.Remediations = appendUnique(d.Remediations, "Rerun regesto doctor after repairing the global install-plan error.")
		markPlannedCapabilities(&d, "error", "install plan could not be evaluated")
		return d
	}
	for _, item := range plan.Items {
		if !containsOwner(item.Owners, agent.Name) {
			continue
		}
		artifact := doctorArtifact{
			ID: item.ID, Kind: item.Kind, Action: item.Action,
			DeclaredTargets: append([]string(nil), item.DeclaredTargets...), CanonicalTarget: item.CanonicalTarget,
			CurrentState: item.CurrentState, IntendedState: item.IntendedState,
		}
		if item.Action != "current" {
			artifact.Remediation = item.DryRun
			d.Remediations = appendUnique(d.Remediations, "Review regesto install --dry-run, then run regesto install for "+agent.Name+".")
		}
		d.Artifacts = append(d.Artifacts, artifact)
		capStatus := artifactStatus(item.Action)
		switch item.Kind {
		case "hook":
			for i := range d.Capabilities.Hooks {
				if d.Capabilities.Hooks[i].Status != "unsupported" && hookArtifactMatches(d.Capabilities.Hooks[i], item) && statusRank(capStatus) >= statusRank(d.Capabilities.Hooks[i].Status) {
					d.Capabilities.Hooks[i].Status = capStatus
					d.Capabilities.Hooks[i].Detail = item.CurrentState
				}
			}
		case "instructions":
			mergeCapabilityStatus(&d.Capabilities.Instructions, capStatus, item.CurrentState)
		default:
			if strings.HasPrefix(item.Kind, "skill") {
				mergeCapabilityStatus(&d.Capabilities.Skills, capStatus, item.CurrentState)
			}
		}
	}
	d.Status = integrationStatus(d)
	if !hasOperationalCapability(d) {
		d.Remediations = appendUnique(d.Remediations, "Configure at least one supported skills, instructions, hook, or memory capability for "+agent.Name+".")
	}
	sort.Strings(d.Remediations)
	return d
}

func inspectMemorySource(source adapters.MemorySource, integration, kbRoot string) doctorMemorySource {
	out := doctorMemorySource{Kind: source.Kind, Location: source.Location}
	switch source.Kind {
	case "none", "":
		out.Status, out.Detail = "unsupported", "profile declares no native memory source"
		return out
	case "markdown-glob-v1":
		matches, err := filepath.Glob(source.Location)
		if err != nil {
			out.Status, out.Detail = "error", "invalid memory location pattern: "+err.Error()
			return out
		}
		for _, match := range matches {
			resolved, resolveErr := filepath.EvalSymlinks(match)
			if resolveErr != nil {
				out.Status, out.Detail = "error", "memory location cannot be resolved: "+resolveErr.Error()
				return out
			}
			resolved, resolveErr = filepath.Abs(resolved)
			if resolveErr != nil {
				out.Status, out.Detail = "error", "memory location cannot be resolved: "+resolveErr.Error()
				return out
			}
			if kbRoot != "" && doctorPathsOverlap(kbRoot, resolved) {
				out.Status, out.Detail = "error", fmt.Sprintf("integration %s memory source overlaps knowledge-base root; harvesting would be refused", integration)
				return out
			}
			if _, statErr := os.Stat(resolved); statErr != nil {
				out.Status, out.Detail = "error", "memory location is not accessible: "+statErr.Error()
				return out
			}
			out.Matches++
		}
		if out.Matches == 0 {
			out.Status, out.Detail = "warning", "declared memory location is not currently available"
		} else {
			out.Status, out.Detail = "ok", fmt.Sprintf("%d memory location(s) available", out.Matches)
		}
		return out
	default:
		out.Status, out.Detail = "unsupported", "memory kind is not supported by this engine"
		return out
	}
}

func doctorPathsOverlap(first, second string) bool {
	return doctorPathWithin(first, second) || doctorPathWithin(second, first)
}

func doctorPathWithin(base, target string) bool {
	rel, err := filepath.Rel(base, target)
	return err == nil && (rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))))
}

func describeTrust(cfg *config.Config) doctorTrust {
	out := doctorTrust{
		Precedence: []string{
			"exact source policy", "legacy exact trusted source", "longest matching source-policy pattern",
			"human inbox authority", "configured integration default", "quarantine fallback",
		},
		SourcePolicies: []doctorTrustRule{}, LegacySources: []doctorTrustRule{},
	}
	rules, _ := cfg.SourcePolicyRules()
	for _, rule := range rules {
		out.SourcePolicies = append(out.SourcePolicies, doctorTrustRule{Source: rule.Source, Trust: rule.Trust, Pattern: rule.Pattern})
	}
	legacy := cfg.Section("trusted_sources")
	keys := make([]string, 0, len(legacy))
	for key := range legacy {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		out.LegacySources = append(out.LegacySources, doctorTrustRule{Source: key, Reason: legacy[key]})
	}
	return out
}

func addDoctorCheck(report *doctorReport, check doctorCheck) {
	report.Checks = append(report.Checks, check)
	if statusRank(check.Status) > statusRank(report.Status) {
		report.Status = check.Status
	}
	if check.Remediation != "" {
		report.Remediations = appendUnique(report.Remediations, check.Remediation)
	}
}

func artifactStatus(action string) string {
	switch action {
	case "current":
		return "ok"
	case "manual":
		return "manual"
	default:
		return "warning"
	}
}

func mergeCapabilityStatus(capability *doctorCapability, status, detail string) {
	if capability.Status == "unsupported" {
		return
	}
	if statusRank(status) >= statusRank(capability.Status) {
		capability.Status, capability.Detail = status, detail
	}
}

func markPlannedCapabilities(d *doctorIntegration, status, detail string) {
	mergeCapabilityStatus(&d.Capabilities.Skills, status, detail)
	mergeCapabilityStatus(&d.Capabilities.Instructions, status, detail)
	for i := range d.Capabilities.Hooks {
		if d.Capabilities.Hooks[i].Status != "unsupported" {
			d.Capabilities.Hooks[i].Status, d.Capabilities.Hooks[i].Detail = status, detail
		}
	}
}

func hookArtifactMatches(hook doctorHookCapability, item regestoinstall.Item) bool {
	for _, declared := range item.DeclaredTargets {
		if declared == hook.Settings {
			return true
		}
		if hook.Registrar == "hermes-config-yaml-v1" && hook.Settings != "" &&
			filepath.Dir(declared) == filepath.Dir(hook.Settings) && filepath.Base(declared) == "shell-hooks-allowlist.json" {
			return true
		}
	}
	return hook.Settings == "" && strings.Contains(item.ID, ":"+hook.Protocol+":")
}

func integrationStatus(d doctorIntegration) string {
	status := "ok"
	for _, candidate := range []string{d.Capabilities.Detection.Status, d.Capabilities.Skills.Status, d.Capabilities.Instructions.Status, d.Capabilities.Trust.Status} {
		if statusRank(candidate) > statusRank(status) {
			status = candidate
		}
	}
	for _, hook := range d.Capabilities.Hooks {
		if statusRank(hook.Status) > statusRank(status) {
			status = hook.Status
		}
	}
	for _, memory := range d.Capabilities.Memory {
		if statusRank(memory.Status) > statusRank(status) {
			status = memory.Status
		}
	}
	if status == "manual" {
		return "warning"
	}
	if !hasOperationalCapability(d) {
		return "warning"
	}
	return status
}

func hasOperationalCapability(d doctorIntegration) bool {
	if d.Capabilities.Skills.Status != "unsupported" || d.Capabilities.Instructions.Status != "unsupported" {
		return true
	}
	for _, hook := range d.Capabilities.Hooks {
		if hook.Status != "unsupported" {
			return true
		}
	}
	for _, memory := range d.Capabilities.Memory {
		if memory.Status != "unsupported" {
			return true
		}
	}
	return false
}

func statusRank(status string) int {
	switch status {
	case "error":
		return 3
	case "warning", "manual":
		return 2
	case "ok":
		return 1
	default:
		return 0
	}
}

func containsOwner(owners []string, want string) bool {
	for _, owner := range owners {
		if owner == want {
			return true
		}
	}
	return false
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func printDoctorReport(report doctorReport) {
	fmt.Printf("regesto doctor: %s\n", report.Status)
	fmt.Printf("instance: %s\nmachine: %s (%s)\n", report.KBRoot, report.Machine.Name, report.Machine.Source)
	for _, integration := range report.Integrations {
		fmt.Printf("integration %s (%s): %s; detected=%t\n", integration.ID, integration.ProfileID, integration.Status, integration.Detected)
		fmt.Printf("  skills=%s instructions=%s trust=%s\n", integration.Capabilities.Skills.Status, integration.Capabilities.Instructions.Status, integration.Capabilities.Trust.Detail)
		for _, hook := range integration.Capabilities.Hooks {
			fmt.Printf("  hook %s via %s: %s\n", hook.Protocol, hook.Registrar, hook.Status)
		}
		for _, memory := range integration.Capabilities.Memory {
			fmt.Printf("  memory %s %s: %s\n", memory.Kind, memory.Location, memory.Status)
		}
		for _, artifact := range integration.Artifacts {
			fmt.Printf("  artifact %-18s %-8s %s\n", artifact.Kind, artifact.Action, artifact.CanonicalTarget)
		}
	}
	fmt.Printf("trust precedence: %s\n", strings.Join(report.Trust.Precedence, " → "))
	for _, rule := range report.Trust.SourcePolicies {
		suffix := ""
		if rule.Pattern {
			suffix = "*"
		}
		fmt.Printf("trust source-policy %s%s: %s\n", rule.Source, suffix, rule.Trust)
	}
	for _, rule := range report.Trust.LegacySources {
		fmt.Printf("trust legacy-source %s: %s\n", rule.Source, rule.Reason)
	}
	for _, check := range report.Checks {
		if check.Status != "ok" {
			fmt.Printf("check %s: %s — %s\n", check.ID, check.Status, check.Message)
		}
	}
	for _, remediation := range report.Remediations {
		fmt.Printf("remedy: %s\n", remediation)
	}
}
