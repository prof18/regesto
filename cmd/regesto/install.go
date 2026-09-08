package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/prof18/regesto/internal/config"
	regestoinstall "github.com/prof18/regesto/internal/install"
)

type installResponse struct {
	SchemaVersion int                    `json:"schema_version"`
	DryRun        bool                   `json:"dry_run"`
	Plan          *regestoinstall.Plan   `json:"plan"`
	Result        *regestoinstall.Result `json:"result,omitempty"`
}

func runInstall(cfg *config.Config, args []string) error {
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	dryRun := fs.Bool("dry-run", false, "preview planned changes and write nothing")
	jsonOutput := fs.Bool("json", false, "print the versioned install plan as JSON")
	engineLink := fs.String("engine-link", "", "launcher: PATH link to plan")
	engineTarget := fs.String("engine-target", "", "launcher: engine target for PATH link")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("install takes no positional arguments")
	}
	plan, err := regestoinstall.Build(cfg, regestoinstall.Options{EngineLink: *engineLink, EngineTarget: *engineTarget})
	if err != nil {
		return err
	}
	response := installResponse{SchemaVersion: jsonSchemaVersion, DryRun: *dryRun, Plan: plan}
	if !*dryRun {
		result, err := regestoinstall.Apply(plan)
		if err != nil {
			return err
		}
		response.Result = &result
	}
	if *jsonOutput {
		return json.NewEncoder(os.Stdout).Encode(response)
	}
	printInstallPlan(plan, *dryRun)
	if *dryRun {
		fmt.Printf("\ndry run — %d change(s) planned; nothing was written.\n", plan.Changes())
	} else {
		fmt.Printf("\ndone — %d change(s) applied. Open a new agent session to load the installed integration.\n", response.Result.Applied)
		for _, path := range response.Result.Backups {
			fmt.Printf("  backup %s\n", path)
		}
	}
	return nil
}

func printInstallPlan(plan *regestoinstall.Plan, dryRun bool) {
	fmt.Printf("Instance: %s\n", plan.KBRoot)
	current := 0
	for _, item := range plan.Items {
		if item.Action == "current" {
			current++
			continue
		}
		verb := item.Action
		if dryRun && item.Action != "current" && item.Action != "skip" && item.Action != "manual" {
			verb = "would " + item.Action
		}
		owners := strings.Join(item.Owners, ", ")
		if owners == "" {
			owners = "regesto"
		}
		fmt.Printf("\n  %s — %s\n", verb, item.Kind)
		fmt.Printf("    Target: %s\n", item.CanonicalTarget)
		printWrapped("    Used by: ", "      ", owners)
		for _, declared := range item.DeclaredTargets {
			if declared != item.CanonicalTarget {
				fmt.Printf("    Declared: %s\n", declared)
			}
		}
		printWrapped("    Current: ", "      ", item.CurrentState)
		printWrapped("    Planned: ", "      ", item.IntendedState)
		printWrapped("    Backup: ", "      ", item.BackupAction)
		if dryRun {
			printWrapped("    Dry run: ", "      ", item.DryRun)
		}
	}
	if current > 0 {
		fmt.Printf("\n  Already current: %d files or links.\n", current)
	}
}
