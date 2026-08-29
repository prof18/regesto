// Package install plans and applies the files an integration exposes to an
// agent. Planning is deliberately read-only: callers can print the complete
// plan, review resolved symlink targets, and only then apply it.
package install

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/prof18/regesto/internal/adapters"
	"github.com/prof18/regesto/internal/config"
)

const SchemaVersion = 1

// Item is one independently reviewable install operation. No-op and manual
// items are included so a dry run is a complete account rather than only a list
// of writes.
type Item struct {
	ID              string   `json:"id"`
	Kind            string   `json:"kind"`
	Action          string   `json:"action"`
	Owners          []string `json:"owners"`
	DeclaredTargets []string `json:"declared_targets"`
	CanonicalTarget string   `json:"canonical_target"`
	CurrentState    string   `json:"current_state"`
	IntendedState   string   `json:"intended_state"`
	BackupAction    string   `json:"backup_action"`
	DryRun          string   `json:"dry_run"`

	desired         []byte
	before          []byte
	ownership       []byte
	ownershipTarget string
	renderRoot      string
	linkTarget      string
	mode            os.FileMode
}

type Plan struct {
	SchemaVersion int    `json:"schema_version"`
	KBRoot        string `json:"kb_root"`
	Items         []Item `json:"items"`
}

// Options supplies deterministic renderer inputs. An absent instruction entry
// uses the shared portable instance template.
type Options struct {
	InstructionSections map[string][]byte
	EngineLink          string
	EngineTarget        string
	// SourceRoot separates engine-owned adapter sources from their real render
	// and host targets. Upgrade dry-runs point it at a temporary post-upgrade
	// view; ordinary installs leave it empty and read from KBRoot.
	SourceRoot string
}

// Build returns the complete install plan without modifying the filesystem.
func Build(cfg *config.Config, opts Options) (*Plan, error) {
	sourceRoot := opts.SourceRoot
	if sourceRoot == "" {
		sourceRoot = cfg.KBRoot
	}
	agents, err := adapters.ResolveFrom(cfg, sourceRoot)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(agents, func(i, j int) bool { return agents[i].Name < agents[j].Name })
	p := &Plan{SchemaVersion: SchemaVersion, KBRoot: cfg.KBRoot, Items: make([]Item, 0)}
	if err := planEngineLink(p, opts); err != nil {
		return nil, err
	}
	if err := planSkills(p, agents, sourceRoot); err != nil {
		return nil, err
	}
	if err := planHooks(p, agents, cfg.UsesLegacyAgents(), sourceRoot); err != nil {
		return nil, err
	}
	if err := planInstructions(p, agents, opts); err != nil {
		return nil, err
	}
	if err := validateTargetConflicts(p.Items); err != nil {
		return nil, err
	}
	sort.SliceStable(p.Items, func(i, j int) bool {
		if itemOrder(p.Items[i]) != itemOrder(p.Items[j]) {
			return itemOrder(p.Items[i]) < itemOrder(p.Items[j])
		}
		if p.Items[i].CanonicalTarget != p.Items[j].CanonicalTarget {
			return p.Items[i].CanonicalTarget < p.Items[j].CanonicalTarget
		}
		return p.Items[i].ID < p.Items[j].ID
	})
	return p, nil
}

func validateTargetConflicts(items []Item) error {
	seen := map[string]Item{}
	for _, item := range items {
		if item.CanonicalTarget == "" {
			continue
		}
		previous, exists := seen[item.CanonicalTarget]
		if !exists {
			seen[item.CanonicalTarget] = item
			continue
		}
		if previous.ID != item.ID {
			return fmt.Errorf("install target conflict at %s: %s (%s, owners %s) and %s (%s, owners %s) require the same canonical target; configure separate targets",
				item.CanonicalTarget, previous.ID, previous.Kind, strings.Join(previous.Owners, ","), item.ID, item.Kind, strings.Join(item.Owners, ","))
		}
	}
	return nil
}

func itemOrder(item Item) int {
	switch item.Kind {
	case "engine-link":
		return 5
	case "hook":
		return 10
	case "instructions":
		return 20
	case "skill-render":
		if filepath.Base(item.CanonicalTarget) == ".regesto-owned" {
			return 30
		}
		return 31
	case "skill-render-file", "skill-render-prune":
		return 32
	case "skills-directory":
		return 40
	case "skill-link":
		return 50
	default:
		return 100
	}
}

func planEngineLink(p *Plan, opts Options) error {
	if opts.EngineLink == "" && opts.EngineTarget == "" {
		return nil
	}
	if opts.EngineLink == "" || opts.EngineTarget == "" {
		return fmt.Errorf("engine link planning requires both link and target")
	}
	canonical, err := canonicalLinkPath(opts.EngineLink)
	if err != nil {
		return err
	}
	item := Item{
		ID:              "engine-link:" + canonical,
		Kind:            "engine-link",
		Owners:          []string{"engine"},
		DeclaredTargets: []string{opts.EngineLink},
		CanonicalTarget: canonical,
		IntendedState:   "symbolic link to " + opts.EngineTarget,
		BackupAction:    "none",
		linkTarget:      opts.EngineTarget,
	}
	info, err := os.Lstat(canonical)
	if os.IsNotExist(err) {
		item.Action, item.CurrentState, item.DryRun = "create", "missing", "create engine link in an existing PATH directory"
		p.Items = append(p.Items, item)
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink == 0 {
		item.Action, item.CurrentState, item.DryRun = "skip", "foreign non-symlink entry preserved", "leave foreign PATH entry unchanged"
		p.Items = append(p.Items, item)
		return nil
	}
	raw, err := os.Readlink(canonical)
	if err != nil {
		return err
	}
	if !filepath.IsAbs(raw) {
		raw = filepath.Join(filepath.Dir(canonical), raw)
	}
	raw = filepath.Clean(raw)
	if raw == filepath.Clean(opts.EngineTarget) {
		item.Action, item.CurrentState, item.DryRun = "current", "engine link current", "leave engine link unchanged"
	} else {
		item.Action, item.CurrentState, item.DryRun = "skip", "foreign symlink to "+raw+" preserved", "leave foreign PATH link unchanged"
	}
	p.Items = append(p.Items, item)
	return nil
}

func (p *Plan) Changes() int {
	n := 0
	for _, item := range p.Items {
		if mutates(item.Action) {
			n++
		}
	}
	return n
}

func mutates(action string) bool {
	switch action {
	case "create", "update", "replace", "remove":
		return true
	default:
		return false
	}
}

// CanonicalTarget resolves a path without creating it. When the final path is
// missing, the closest existing ancestor is resolved and the missing suffix is
// appended. This is the same operation used again immediately before apply.
func CanonicalTarget(path string) (string, error) {
	abs, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	cur := abs
	var suffix []string
	for {
		_, err := os.Lstat(cur)
		if err == nil {
			resolved, err := filepath.EvalSymlinks(cur)
			if err != nil {
				return "", fmt.Errorf("resolve %s: %w", path, err)
			}
			for i := len(suffix) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, suffix[i])
			}
			return filepath.Clean(resolved), nil
		}
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("inspect %s: %w", path, err)
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", fmt.Errorf("resolve %s: no existing ancestor", path)
		}
		suffix = append(suffix, filepath.Base(cur))
		cur = parent
	}
}

func rootDisplay(root string) string {
	home, err := os.UserHomeDir()
	if err == nil && root != home && strings.HasPrefix(root, home+string(filepath.Separator)) {
		return "~" + strings.TrimPrefix(root, home)
	}
	return root
}

func render(body []byte, root string) []byte {
	return []byte(strings.ReplaceAll(string(body), "{{kb_root}}", rootDisplay(root)))
}

func sortedUnique(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out
}

func backupDescription(target string, exists bool) string {
	if !exists {
		return "none (target does not exist)"
	}
	return target + ".regesto-backup.<UTC timestamp>"
}

func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
