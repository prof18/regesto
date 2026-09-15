package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

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
	setUsage(fs, "regesto install [options]", "Add Regesto's skills, instructions and hooks to your connected agents.")
	dryRun := fs.Bool("dry-run", false, "show what would change without writing anything")
	jsonOutput := fs.Bool("json", false, "print the plan as JSON")
	engineLink := fs.String("engine-link", "", "also create a link to the regesto command at this path (used by installers)")
	engineTarget := fs.String("engine-target", "", "the regesto binary that --engine-link points to")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("install takes no arguments besides options; see `regesto install --help`")
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
	if *dryRun {
		fmt.Print("Install preview (nothing has been written)\n\n")
	}
	fmt.Printf("Knowledge base: %s\n\n", tildePath(plan.KBRoot))
	printInstallPlan(plan)
	if *dryRun {
		if plan.Changes() == 0 {
			fmt.Println("\nNothing to change.")
		} else {
			fmt.Printf("\n%s to make. Run `regesto install` to apply.\n", plural(plan.Changes(), "change"))
		}
	} else {
		fmt.Printf("\nDone: %s applied.", plural(response.Result.Applied, "change"))
		if response.Result.Applied > 0 {
			fmt.Print(" Start a new agent session to pick them up.")
		}
		fmt.Println()
		for _, path := range response.Result.Backups {
			fmt.Printf("  Backup saved: %s\n", tildePath(path))
		}
	}
	return nil
}

// printInstallPlan shows the plan grouped by agent, one line per change. Items
// already up to date are only counted, and Regesto's internal copies of skill
// files (kept inside the knowledge base) are summarised in a single line.
func printInstallPlan(plan *regestoinstall.Plan) {
	type group struct {
		name   string
		items  []regestoinstall.Item
		copies map[string]int
		// skipped skill links, counted by reason, so one reason prints once
		skipped map[string]int
	}
	groups := map[string]*group{}
	var order []string
	current := 0
	for _, item := range plan.Items {
		if item.Action == "current" {
			current++
			continue
		}
		name := "Regesto"
		if len(item.Owners) > 0 {
			name = integrationNames(item.Owners)
		}
		g, ok := groups[name]
		if !ok {
			g = &group{name: name, copies: map[string]int{}, skipped: map[string]int{}}
			groups[name] = g
			order = append(order, name)
		}
		if strings.HasPrefix(item.Kind, "skill-render") {
			g.copies[item.Action]++
			continue
		}
		if item.Kind == "skill-link" && item.Action == "skip" {
			if g.skipped[item.CurrentState] == 0 {
				g.items = append(g.items, item)
			}
			g.skipped[item.CurrentState]++
			continue
		}
		g.items = append(g.items, item)
	}
	sort.Strings(order)

	for i, name := range order {
		g := groups[name]
		if i > 0 {
			fmt.Println()
		}
		fmt.Println(g.name)
		for _, item := range g.items {
			target := tildePath(item.CanonicalTarget)
			if target == "" && len(item.DeclaredTargets) > 0 {
				target = tildePath(item.DeclaredTargets[0])
			}
			if n := g.skipped[item.CurrentState]; item.Kind == "skill-link" && item.Action == "skip" && n > 1 {
				target = plural(n, "skill") + " in " + tildePath(filepath.Dir(item.CanonicalTarget))
				fmt.Printf("  %-8s %-14s %s\n", actionLabel(item.Action), "Skills", target)
			} else {
				fmt.Printf("  %-8s %-14s %s\n", actionLabel(item.Action), kindLabel(item.Kind), target)
			}
			switch item.Action {
			case "skip":
				printWrapped("           ", "           ", sentence(tildeText(item.CurrentState)))
				if item.DryRun != "no change" {
					printWrapped("           To fix: ", "                   ", item.DryRun)
				}
			case "manual":
				printWrapped("           ", "           ", sentence(item.CurrentState))
				printWrapped("           ", "           ", item.IntendedState)
			default:
				if item.BackupAction != "" && item.BackupAction != "none" {
					printWrapped("           Backup: ", "                   ", tildePath(item.BackupAction))
				}
			}
		}
		actions := make([]string, 0, len(g.copies))
		for action := range g.copies {
			actions = append(actions, action)
		}
		sort.Strings(actions)
		for _, action := range actions {
			fmt.Printf("  %-8s %-14s %s inside the knowledge base\n", actionLabel(action), "Skill copies", plural(g.copies[action], "file"))
		}
	}
	if current > 0 {
		if len(order) > 0 {
			fmt.Println()
		}
		fmt.Printf("%s already up to date.\n", plural(current, "file"))
	}
}

func actionLabel(action string) string {
	switch action {
	case "create":
		return "Create"
	case "update", "replace":
		return "Update"
	case "remove":
		return "Remove"
	case "skip":
		return "Skip"
	case "manual":
		return "By hand"
	default:
		return action
	}
}

func kindLabel(kind string) string {
	switch kind {
	case "hook", "hook-link":
		return "Hook"
	case "instructions":
		return "Instructions"
	case "skill-link":
		return "Skill"
	case "skills-directory":
		return "Skills folder"
	case "engine-link":
		return "Command link"
	case "skill-render", "skill-render-file", "skill-render-prune":
		return "Skill copy"
	default:
		return kind
	}
}

// sentence capitalises a plan description so it reads as a sentence.
func sentence(text string) string {
	if text == "" {
		return text
	}
	r, size := utf8.DecodeRuneInString(text)
	text = string(unicode.ToUpper(r)) + text[size:]
	if !strings.HasSuffix(text, ".") {
		text += "."
	}
	return text
}
