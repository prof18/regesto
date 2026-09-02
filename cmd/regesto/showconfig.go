package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/prof18/regesto/internal/adapters"
	"github.com/prof18/regesto/internal/config"
)

// runShowConfig prints the resolved instance configuration as key=value lines.
// This is the single source of truth for shell consumers like bin/regesto-install,
// so path policy lives in tested Go rather than being duplicated in bash.
func runShowConfig(cfg *config.Config, args []string) error {
	fs := flag.NewFlagSet("config", flag.ContinueOnError)
	jsonOutput := fs.Bool("json", false, "print resolved capabilities as JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("config: unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	resolved, err := adapters.Resolve(cfg)
	if err != nil {
		return err
	}
	if *jsonOutput {
		return json.NewEncoder(os.Stdout).Encode(resolvedConfig{SchemaVersion: jsonSchemaVersion, KBRoot: cfg.KBRoot, Machine: cfg.Machine, MachineSource: cfg.MachineSource, IntegrationIDs: append([]string{}, cfg.IntegrationIDs()...), Integrations: jsonIntegrations(resolved)})
	}
	fmt.Printf("kb_root=%s\n", cfg.KBRoot)
	fmt.Printf("machine=%s\n", cfg.Machine)
	fmt.Printf("machine_source=%s\n", cfg.MachineSource)
	fmt.Printf("integrations=%s\n", strings.Join(cfg.IntegrationIDs(), " "))
	for _, a := range resolved {
		fmt.Printf("integration.%s.profile=%s\n", a.Name, a.ProfileID)
		fmt.Printf("integration.%s.display_name=%s\n", a.Name, a.DisplayName)
		fmt.Printf("integration.%s.skills_dir=%s\n", a.Name, a.SkillsDir)
		fmt.Printf("integration.%s.instructions=%s\n", a.Name, a.InstructionsFile)
		fmt.Printf("integration.%s.settings=%s\n", a.Name, a.SettingsFile)
		fmt.Printf("integration.%s.memory=%s\n", a.Name, a.MemoryGlob)
		fmt.Printf("integration.%s.trust=%s\n", a.Name, a.DefaultTrust)
	}
	return nil
}

type resolvedConfig struct {
	SchemaVersion  int               `json:"schema_version"`
	KBRoot         string            `json:"kb_root"`
	Machine        string            `json:"machine"`
	MachineSource  string            `json:"machine_source"`
	IntegrationIDs []string          `json:"integration_ids"`
	Integrations   []jsonIntegration `json:"integrations"`
}
