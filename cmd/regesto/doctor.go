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

	// otherKnowledgeBase is set when this agent's skills belong to another
	// knowledge base; the text report merges that advice across agents.
	otherKnowledgeBase string
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
	TrustedSources []doctorTrustRule `json:"trusted_sources"`
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
	setUsage(fs, "regesto doctor [options]", "Check that Regesto and your agents are set up correctly, and say what to fix.")
	integrationID := fs.String("integration", "", "check only this agent `id`")
	jsonOutput := fs.Bool("json", false, "print the report as JSON")
	verbose := fs.Bool("verbose", false, "also list files that are fine, and the trust rules")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("doctor takes no arguments besides options; see `regesto doctor --help`")
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
	printDoctorReport(report, *verbose)
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
		message := "the knowledge base folder can't be opened"
		if statErr != nil {
			message += ": " + statErr.Error()
		}
		addDoctorCheck(&report, doctorCheck{ID: "kb-root", Status: "error", Message: message, Remediation: "Check that the knowledge base folder exists and is readable, then run `regesto doctor` again."})
	} else {
		addDoctorCheck(&report, doctorCheck{ID: "kb-root", Status: "ok", Message: "the knowledge base folder is readable"})
	}

	detectedIDs, detectErr := adapters.DetectProfiles(cfg.KBRoot)
	detected := map[string]bool{}
	if detectErr != nil {
		addDoctorCheck(&report, doctorCheck{ID: "profile-detection", Status: "error", Message: detectErr.Error(), Remediation: "Run `regesto upgrade` to restore Regesto's agent definitions, then run `regesto doctor` again."})
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
		resolved, resolveErr := adapters.Resolve(doctorScopedConfig(cfg, id))
		if resolveErr != nil {
			addDoctorCheck(&report, doctorCheck{ID: "detected-profile-" + id, Status: "error", Message: resolveErr.Error(), Remediation: "Run `regesto upgrade` to restore Regesto's agent definitions, then run `regesto doctor` again."})
			continue
		}
		if len(resolved) == 1 {
			inputs = append(inputs, diagnosticInput{agent: resolved[0], configured: false})
		}
	}
	if filter != "" && len(inputs) == 0 {
		return doctorReport{}, fmt.Errorf("agent %q is not connected and was not found on this machine", filter)
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
		addDoctorCheck(&report, doctorCheck{ID: "install-plan", Status: "ok", Message: "no connected agents to check"})
	} else {
		planCfg := cfg
		if filter != "" {
			planCfg = doctorScopedConfig(cfg, filter)
		}
		globalPlan, globalErr := regestoinstall.Build(planCfg, regestoinstall.Options{})
		if globalErr == nil {
			for _, input := range configuredInputs {
				plans[input.agent.Name] = globalPlan
			}
			addDoctorCheck(&report, doctorCheck{ID: "install-plan", Status: "ok", Message: "agent files can be checked"})
		} else {
			addDoctorCheck(&report, doctorCheck{ID: "install-plan", Status: "error", Message: globalErr.Error(), Remediation: "Fix the file problem reported above, then run `regesto install --dry-run`."})
			for _, input := range configuredInputs {
				individual, individualErr := regestoinstall.Build(doctorScopedConfig(cfg, input.agent.Name), regestoinstall.Options{})
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
	// Steps for connected agents come first, in agent order. Agents that are
	// installed but not connected share one step, since the fix is one command.
	var unconnected []doctorIntegration
	var needInstall []string
	var otherKBs []string
	otherKBNames := map[string][]string{}
	for _, input := range inputs {
		integration := diagnoseIntegration(input.agent, detected[input.agent.ProfileID], input.configured, canonicalKBRoot, plans[input.agent.Name], planErrors[input.agent.Name])
		report.Integrations = append(report.Integrations, integration)
		if statusRank(integration.Status) > statusRank(report.Status) {
			report.Status = integration.Status
		}
		if !integration.Configured {
			unconnected = append(unconnected, integration)
			continue
		}
		for _, remediation := range integration.Remediations {
			if kb := integration.otherKnowledgeBase; kb != "" && remediation == otherKBRemediation(integration.DisplayName, kb) {
				if len(otherKBNames[kb]) == 0 {
					otherKBs = append(otherKBs, kb)
				}
				otherKBNames[kb] = append(otherKBNames[kb], integration.DisplayName)
				continue
			}
			if remediation == installRemediation(integration.DisplayName) {
				needInstall = append(needInstall, integration.DisplayName)
				continue
			}
			report.Remediations = appendUnique(report.Remediations, remediation)
		}
	}
	var first []string
	for _, kb := range otherKBs {
		first = append(first, otherKBRemediation(joinWords(otherKBNames[kb]), kb))
	}
	if len(needInstall) > 0 {
		first = append(first, installRemediation(joinWords(needInstall)))
	}
	report.Remediations = append(first, report.Remediations...)
	switch len(unconnected) {
	case 0:
	case 1:
		for _, remediation := range unconnected[0].Remediations {
			report.Remediations = appendUnique(report.Remediations, remediation)
		}
	default:
		names, ids := make([]string, len(unconnected)), make([]string, len(unconnected))
		for i, integration := range unconnected {
			names[i], ids[i] = integration.DisplayName, integration.ID
		}
		report.Remediations = appendUnique(report.Remediations, joinWords(names)+" are installed but not connected. To connect them: `regesto integrations add "+strings.Join(ids, " ")+"`")
	}
	report.Trust = describeTrust(cfg)
	return report, nil
}

// otherKBRemediation explains that agents are already set up with a different
// knowledge base, which is usually intended rather than a problem.
func otherKBRemediation(names, kb string) string {
	return names + " already use the knowledge base in " + tildePath(kb) + ". If that's the one you want, there's nothing to do. " +
		"To switch to this one, remove their regesto-* skill links, then run `regesto install` here."
}

// otherLinkedKB reads which knowledge base a skipped skill link belongs to.
func otherLinkedKB(item regestoinstall.Item) string {
	target, err := os.Readlink(item.CanonicalTarget)
	if err != nil {
		return ""
	}
	return regestoinstall.OtherKnowledgeBase(target)
}

// otherAdapterLinkedKB reads which knowledge base a skipped adapter launcher
// link belongs to, for hosts whose hook registers through a linked file.
func otherAdapterLinkedKB(item regestoinstall.Item) string {
	target, err := os.Readlink(item.CanonicalTarget)
	if err != nil {
		return ""
	}
	return regestoinstall.OtherAdapterKnowledgeBase(target)
}

// installRemediation is the one step every agent with pending files shares,
// so the report can merge it into a single line.
func installRemediation(names string) string {
	return "Run `regesto install` to finish setting up " + names + " (preview first with `regesto install --dry-run`)."
}

func doctorScopedConfig(cfg *config.Config, id string) *config.Config {
	clone := *cfg
	clone.Integrations = []string{id}
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
		d.Capabilities.Detection = doctorCapability{Status: "unsupported", Detail: "can't be detected automatically"}
	} else if detected {
		d.Capabilities.Detection = doctorCapability{Status: "ok", Detail: "found on this machine"}
	} else {
		d.Capabilities.Detection = doctorCapability{Status: "warning", Detail: "connected, but not found on this machine"}
		d.Remediations = append(d.Remediations, agent.DisplayName+" is connected but doesn't seem to be installed here. Install it, or disconnect it: `regesto integrations remove "+agent.Name+"`")
	}

	if len(agent.SkillsDirs) == 0 {
		d.Capabilities.Skills = doctorCapability{Status: "unsupported", Detail: "not supported by this agent"}
	} else {
		d.Capabilities.Skills = doctorCapability{Status: "ok", Detail: "installed"}
	}
	if len(agent.InstructionsFiles) == 0 {
		d.Capabilities.Instructions = doctorCapability{Status: "unsupported", Detail: "not supported by this agent"}
	} else {
		d.Capabilities.Instructions = doctorCapability{Status: "ok", Detail: "installed"}
	}
	for _, hook := range agent.Hooks {
		status, detail := "ok", "installed"
		if hook.Protocol == "none" || hook.Registrar == "none" {
			status, detail = "unsupported", "not supported by this agent"
		}
		d.Capabilities.Hooks = append(d.Capabilities.Hooks, doctorHookCapability{Protocol: hook.Protocol, Registrar: hook.Registrar, Settings: hook.Settings, Status: status, Detail: detail})
	}
	if len(d.Capabilities.Hooks) == 0 {
		d.Capabilities.Hooks = []doctorHookCapability{{Protocol: "none", Registrar: "none", Status: "unsupported", Detail: "not supported by this agent"}}
	}
	// Memory advice is added last: it matters less than finishing the install.
	var memoryAdvice []string
	for _, source := range agent.MemorySources {
		memory := inspectMemorySource(source, agent.Name, kbRoot)
		d.Capabilities.Memory = append(d.Capabilities.Memory, memory)
		switch {
		case !configured:
		case memory.Status == "warning":
			memoryAdvice = append(memoryAdvice, "No memory files found for "+agent.DisplayName+" at "+tildePath(memory.Location)+". That's normal until it saves some; if they live elsewhere, set memory_location under [integrations."+agent.Name+"] in config.toml.")
		case memory.Status == "error":
			memoryAdvice = append(memoryAdvice, "Regesto can't collect memory for "+agent.DisplayName+": "+memory.Detail+". Check memory_location under [integrations."+agent.Name+"] in config.toml.")
		}
	}
	if len(d.Capabilities.Memory) == 0 {
		d.Capabilities.Memory = []doctorMemorySource{{Kind: "none", Status: "unsupported", Detail: "this agent has no memory files to collect"}}
	}
	defaultTrust := agent.DefaultTrust
	if defaultTrust == "" {
		defaultTrust = "quarantine"
	}
	trustDetail := "wait in the inbox for your review (quarantine)"
	if defaultTrust == "supervised" {
		trustDetail = "become facts automatically (supervised)"
	}
	d.Capabilities.Trust = doctorCapability{Status: "ok", Detail: trustDetail}

	if !configured {
		d.Status = "warning"
		d.Remediations = appendUnique(d.Remediations, agent.DisplayName+" is installed but not connected. To connect it: `regesto integrations add "+agent.Name+"`")
		markPlannedCapabilities(&d, "warning", "not connected")
		return d
	}

	if planErr != nil {
		d.Status = "error"
		d.Remediations = appendUnique(d.Remediations, "Fix the problem reported above, then run `regesto doctor --integration "+agent.Name+"` again.")
		markPlannedCapabilities(&d, "error", "couldn't be checked")
		return d
	}
	if plan == nil {
		d.Status = "error"
		d.Remediations = appendUnique(d.Remediations, "Fix the problem reported above, then run `regesto doctor` again.")
		markPlannedCapabilities(&d, "error", "couldn't be checked")
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
			target := tildePath(item.CanonicalTarget)
			switch {
			case item.Action == "manual":
				d.Remediations = appendUnique(d.Remediations, "Set up the "+agent.DisplayName+" hook by hand; `regesto install --dry-run` shows what to add.")
			case item.Action == "skip" && item.Kind == "skill-link" && len(item.DeclaredTargets) > 0 && otherLinkedKB(item) != "":
				d.otherKnowledgeBase = otherLinkedKB(item)
			case item.Action == "skip" && item.Kind == "hook-link" && otherAdapterLinkedKB(item) != "":
				d.otherKnowledgeBase = otherAdapterLinkedKB(item)
			case item.Action == "skip" && item.Kind == "instructions":
				d.Remediations = appendUnique(d.Remediations, "Create "+target+" so Regesto can add its instructions for "+agent.DisplayName+", then run `regesto install` again.")
			case item.Action == "skip":
				d.Remediations = appendUnique(d.Remediations, agent.DisplayName+": "+item.CurrentState+" at "+target+". Move it aside if Regesto should manage it, then run `regesto install` again.")
			default:
				d.Remediations = appendUnique(d.Remediations, installRemediation(agent.DisplayName))
			}
		}
		d.Artifacts = append(d.Artifacts, artifact)
		capStatus := artifactStatus(item.Action)
		switch item.Kind {
		case "hook", "hook-link":
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
	if d.otherKnowledgeBase != "" {
		// Installing here would move half of this agent (instructions, hook)
		// to this knowledge base while its skills stay with the other one, so
		// the only advice is the switch itself.
		kept := d.Remediations[:0]
		for _, remediation := range d.Remediations {
			if remediation != installRemediation(agent.DisplayName) {
				kept = append(kept, remediation)
			}
		}
		d.Remediations = append(kept, otherKBRemediation(agent.DisplayName, d.otherKnowledgeBase))
	}
	d.Status = integrationStatus(d)
	for _, advice := range memoryAdvice {
		d.Remediations = appendUnique(d.Remediations, advice)
	}
	if !hasOperationalCapability(d) {
		d.Remediations = appendUnique(d.Remediations, agent.DisplayName+" has nothing Regesto can set up. Check [integrations."+agent.Name+"] in config.toml.")
	}
	return d
}

func inspectMemorySource(source adapters.MemorySource, integration, kbRoot string) doctorMemorySource {
	out := doctorMemorySource{Kind: source.Kind, Location: source.Location}
	switch source.Kind {
	case "none", "":
		out.Status, out.Detail = "unsupported", "this agent has no memory files to collect"
		return out
	case "markdown-glob-v1":
		matches, err := filepath.Glob(source.Location)
		if err != nil {
			out.Status, out.Detail = "error", "memory location is not a valid path pattern: "+err.Error()
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
				out.Status, out.Detail = "error", fmt.Sprintf("the memory location for %s is inside the knowledge base (overlaps knowledge-base root), so Regesto won't collect from it", integration)
				return out
			}
			if _, statErr := os.Stat(resolved); statErr != nil {
				out.Status, out.Detail = "error", "memory location can't be read: "+statErr.Error()
				return out
			}
			out.Matches++
		}
		if out.Matches == 0 {
			out.Status, out.Detail = "warning", "no memory files found yet"
		} else {
			out.Status, out.Detail = "ok", plural(out.Matches, "memory folder")+" found"
		}
		return out
	default:
		out.Status, out.Detail = "unsupported", "this memory type isn't supported by this version of Regesto"
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
			"exact source policy", "exact trusted source", "longest matching source-policy pattern",
			"human inbox authority", "configured integration default", "quarantine fallback",
		},
		SourcePolicies: []doctorTrustRule{}, TrustedSources: []doctorTrustRule{},
	}
	rules, _ := cfg.SourcePolicyRules()
	for _, rule := range rules {
		out.SourcePolicies = append(out.SourcePolicies, doctorTrustRule{Source: rule.Source, Trust: rule.Trust, Pattern: rule.Pattern})
	}
	trusted := cfg.Section("trusted_sources")
	keys := make([]string, 0, len(trusted))
	for key := range trusted {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		out.TrustedSources = append(out.TrustedSources, doctorTrustRule{Source: key, Reason: trusted[key]})
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

func printDoctorReport(report doctorReport, verbose bool) {
	fmt.Printf("Regesto doctor: %s\n\n", doctorStatusLabel(report.Status))
	fmt.Printf("  Knowledge base  %s\n", tildePath(report.KBRoot))
	fmt.Printf("  Machine         %s\n", report.Machine.Name)
	for _, check := range report.Checks {
		if check.Status != "ok" || verbose {
			printWrapped("\n  "+doctorStatusTitle(check.Status)+": ", "    ", sentence(check.Message))
		}
	}
	if len(report.Remediations) > 0 {
		fmt.Println("\nWhat to do")
		for i, remediation := range report.Remediations {
			printWrapped(fmt.Sprintf("  %d. ", i+1), "     ", remediation)
		}
	}
	fmt.Println("\nAgents")
	if len(report.Integrations) == 0 {
		fmt.Println("  No agents connected or found on this machine.")
	}
	for _, integration := range report.Integrations {
		status := doctorStatusLabel(integration.Status)
		if !integration.Configured {
			status = "not connected"
		} else if integration.otherKnowledgeBase != "" {
			status = "uses another knowledge base"
		}
		fmt.Printf("\n  %s (%s): %s\n", integration.DisplayName, integration.ID, status)
		if !integration.Configured {
			fmt.Println("    Installed on this machine, but not listed in config.toml.")
			continue
		}
		if kb := integration.otherKnowledgeBase; kb != "" && !verbose {
			fmt.Printf("    Set up with the knowledge base in %s.\n", tildePath(kb))
			continue
		}
		if integration.Capabilities.Detection.Status != "ok" || verbose {
			printDoctorCapability("Installed", integration.Capabilities.Detection, verbose)
		}
		printDoctorCapability("Skills", integration.Capabilities.Skills, verbose)
		printDoctorCapability("Instructions", integration.Capabilities.Instructions, verbose)
		for _, hook := range integration.Capabilities.Hooks {
			printDoctorCapability("Hook", doctorCapability{Status: hook.Status, Detail: hook.Detail}, verbose)
			if verbose && hook.Settings != "" {
				fmt.Printf("                  %s\n", tildePath(hook.Settings))
			}
		}
		for _, memory := range integration.Capabilities.Memory {
			printDoctorCapability("Memory", doctorCapability{Status: memory.Status, Detail: memory.Detail}, verbose)
			if memory.Location != "" && (verbose || memory.Status != "ok") {
				fmt.Printf("                  %s\n", tildePath(memory.Location))
			}
		}
		printWrapped("    New notes     ", "                  ", integration.Capabilities.Trust.Detail)
		pending, current := 0, 0
		for _, artifact := range integration.Artifacts {
			if artifact.Action == "current" {
				current++
			} else {
				pending++
			}
			if verbose {
				fmt.Printf("      %-8s %-14s %s\n", actionLabel(artifact.Action), kindLabel(artifact.Kind), tildePath(artifact.CanonicalTarget))
			}
		}
		if pending > 0 {
			fmt.Printf("    Files         %s to make, %s up to date\n", plural(pending, "change"), plural(current, "file"))
		} else if current > 0 {
			fmt.Printf("    Files         %s up to date\n", plural(current, "file"))
		}
	}
	if verbose {
		fmt.Println("\nHow Regesto decides whether a note becomes a fact (first match wins)")
		for i, precedence := range report.Trust.Precedence {
			fmt.Printf("  %d. %s\n", i+1, trustRuleLabel(precedence))
		}
		for _, rule := range report.Trust.SourcePolicies {
			suffix := ""
			if rule.Pattern {
				suffix = "*"
			}
			printWrapped("\n  Source policy: ", "    ", rule.Source+suffix+": "+rule.Trust)
		}
		for _, rule := range report.Trust.TrustedSources {
			printWrapped("\n  Trusted source: ", "    ", rule.Source+": "+rule.Reason)
		}
	} else {
		fmt.Println("\nFor every file and the trust rules: regesto doctor --verbose")
	}
}

// printDoctorCapability prints one aligned row: the label, then either "OK" or
// what is wrong. Healthy rows show their detail only in verbose mode.
func printDoctorCapability(label string, capability doctorCapability, verbose bool) {
	text := "OK"
	if capability.Status != "ok" || verbose {
		text = tildeText(capability.Detail)
	}
	printWrapped(fmt.Sprintf("    %-14s", label), "                  ", text)
}

// trustRuleLabel explains a trust precedence step; the JSON keeps the short ids.
func trustRuleLabel(rule string) string {
	switch rule {
	case "exact source policy":
		return "an exact agent@machine entry in [source_policies]"
	case "exact trusted source":
		return "an entry in [trusted_sources]"
	case "longest matching source-policy pattern":
		return "the longest matching * pattern in [source_policies]"
	case "human inbox authority":
		return "notes you added yourself (source: human) are processed"
	case "configured integration default":
		return "the agent's own trust setting"
	case "quarantine fallback":
		return "otherwise the note waits in the inbox"
	default:
		return rule
	}
}

func doctorStatusLabel(status string) string {
	switch status {
	case "ok":
		return "OK"
	case "warning":
		return "needs attention"
	case "manual":
		return "needs a manual step"
	case "error":
		return "error"
	case "unsupported":
		return "not supported"
	default:
		return status
	}
}

func doctorStatusTitle(status string) string {
	switch status {
	case "ok":
		return "OK"
	case "error":
		return "Error"
	default:
		return "Warning"
	}
}
