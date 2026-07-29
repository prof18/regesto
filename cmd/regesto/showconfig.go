package main

import (
	"fmt"
	"strings"

	"regesto/internal/adapters"
	"regesto/internal/config"
)

// runShowConfig prints the resolved instance configuration as key=value lines.
// This is the single source of truth for shell consumers like bin/regesto-install,
// so path policy lives in tested Go rather than being duplicated in bash.
func runShowConfig(cfg *config.Config) error {
	fmt.Printf("kb_root=%s\n", cfg.KBRoot)
	fmt.Printf("machine=%s\n", cfg.Machine)
	fmt.Printf("machine_source=%s\n", cfg.MachineSource)
	fmt.Printf("agents=%s\n", strings.Join(cfg.Agents, " "))
	for _, a := range adapters.For(cfg) {
		fmt.Printf("agent.%s.skills_dir=%s\n", a.Name, a.SkillsDir)
		fmt.Printf("agent.%s.instructions=%s\n", a.Name, a.InstructionsFile)
		fmt.Printf("agent.%s.settings=%s\n", a.Name, a.SettingsFile)
	}
	return nil
}
