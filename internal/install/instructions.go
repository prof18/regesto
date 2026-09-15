package install

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	regesto "github.com/prof18/regesto"
	"github.com/prof18/regesto/internal/adapters"
)

const (
	sectionStart = "<!-- regesto:section:start -->"
	sectionEnd   = "<!-- regesto:section:end -->"
)

type instructionGroup struct {
	canonical string
	declared  []string
	owners    []string
	sections  map[string][]byte
	create    bool
}

func planInstructions(p *Plan, agents []adapters.Agent, opts Options) error {
	sourceRoot := opts.SourceRoot
	if sourceRoot == "" {
		sourceRoot = p.KBRoot
	}
	var shared []byte
	var err error
	shared, err = os.ReadFile(filepath.Join(sourceRoot, "adapters", "instructions", "regesto-section.md"))
	if err != nil {
		if os.IsNotExist(err) {
			shared, err = regesto.Adapters.ReadFile("adapters/instructions/regesto-section.md")
			if err != nil {
				return fmt.Errorf("missing instruction template: %w", err)
			}
		} else {
			return err
		}
	}
	groups := map[string]*instructionGroup{}
	for _, agent := range agents {
		section := render(shared, p.KBRoot)
		if custom, ok := opts.InstructionSections[agent.Name]; ok {
			section = render(custom, p.KBRoot)
		}
		for _, declared := range agent.InstructionsFiles {
			canonical, err := CanonicalTarget(declared)
			if err != nil {
				return err
			}
			g := groups[canonical]
			if g == nil {
				g = &instructionGroup{canonical: canonical, sections: map[string][]byte{}}
				groups[canonical] = g
			}
			g.declared = append(g.declared, declared)
			g.owners = append(g.owners, agent.Name)
			g.sections[agent.Name] = section
			g.create = g.create || agent.InstructionsCreate
		}
	}
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		g := groups[key]
		g.owners = sortedUnique(g.owners)
		g.declared = sortedUnique(g.declared)
		var section []byte
		var firstOwner string
		for _, owner := range g.owners {
			candidate := g.sections[owner]
			if section == nil {
				section, firstOwner = candidate, owner
				continue
			}
			if !bytes.Equal(section, candidate) {
				return fmt.Errorf("instruction target conflict at %s: integrations %q and %q render different Regesto sections; configure separate instructions_file targets", g.canonical, firstOwner, owner)
			}
		}
		item := Item{
			ID:              "instructions:" + key,
			Kind:            "instructions",
			Owners:          g.owners,
			DeclaredTargets: g.declared,
			CanonicalTarget: key,
			IntendedState:   "Regesto section up to date; the rest of the file kept",
		}
		current, err := os.ReadFile(key)
		switch {
		case err == nil:
			desired, state, err := mergeInstruction(current, section)
			if err != nil {
				return fmt.Errorf("plan instructions %s: %w", key, err)
			}
			item.CurrentState = state
			item.before = current
			item.desired = desired
			info, statErr := os.Stat(key)
			if statErr != nil {
				return statErr
			}
			item.mode = info.Mode().Perm()
			if bytes.Equal(current, desired) {
				item.Action = "current"
				item.BackupAction = "none"
				item.DryRun = "no change"
			} else {
				item.Action = "update"
				item.BackupAction = backupDescription(key, true)
				item.DryRun = "back up the file and update its Regesto section"
			}
		case os.IsNotExist(err) && !g.create:
			item.Action = "skip"
			item.CurrentState = "file not found; Regesto doesn't create this file for this agent"
			item.BackupAction = "none"
			item.DryRun = "create this file yourself, then run `regesto install` again"
		case os.IsNotExist(err):
			item.Action = "create"
			item.CurrentState = "file not found"
			item.BackupAction = backupDescription(key, false)
			item.DryRun = "create the file with a Regesto section"
			item.desired = normalizeSection(section)
			item.mode = 0o644
		default:
			return err
		}
		p.Items = append(p.Items, item)
	}
	return nil
}

func normalizeSection(section []byte) []byte {
	return []byte(strings.TrimSpace(string(section)) + "\n")
}

func mergeInstruction(current, section []byte) ([]byte, string, error) {
	body := string(current)
	startCount := strings.Count(body, sectionStart)
	endCount := strings.Count(body, sectionEnd)
	if startCount != endCount || startCount > 1 {
		return nil, "", fmt.Errorf("expected zero or one balanced Regesto marker pair, found %d start and %d end markers", startCount, endCount)
	}
	want := strings.TrimSpace(string(section))
	if startCount == 0 {
		if body == "" {
			return []byte(want + "\n"), "file exists, without a Regesto section", nil
		}
		separator := "\n\n"
		if strings.HasSuffix(body, "\n\n") {
			separator = ""
		} else if strings.HasSuffix(body, "\n") {
			separator = "\n"
		}
		return []byte(body + separator + want + "\n"), "file exists, without a Regesto section", nil
	}
	start := strings.Index(body, sectionStart)
	endRel := strings.Index(body[start:], sectionEnd)
	if endRel < 0 {
		return nil, "", fmt.Errorf("Regesto section start has no end marker")
	}
	end := start + endRel + len(sectionEnd)
	installed := strings.TrimSpace(body[start:end])
	if installed == want {
		return append([]byte(nil), current...), "Regesto section is up to date", nil
	}
	return []byte(body[:start] + want + body[end:]), "Regesto section is out of date", nil
}
