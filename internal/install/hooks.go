package install

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/prof18/regesto/internal/adapters"
)

type hookGroup struct {
	canonical string
	declared  []string
	owners    []string
	commands  []string
	registrar string
	event     string
	merge     func([]byte, []string) ([]byte, bool, error)
}

func planHooks(p *Plan, agents []adapters.Agent, sourceRoot string) error {
	groups := map[string]*hookGroup{}
	for _, agent := range agents {
		hooks := append([]adapters.Hook(nil), agent.Hooks...)
		for _, hook := range hooks {
			switch hook.Registrar {
			case "none", "":
				continue
			case "manual":
				item, err := planManualHook(p, agent, hook, sourceRoot)
				if err != nil {
					return err
				}
				if err := appendSharedHookItem(p, item); err != nil {
					return err
				}
			case "hermes-config-yaml-v1":
				items, err := planHermesHook(p, agent, hook, sourceRoot)
				if err != nil {
					return err
				}
				for _, item := range items {
					if err := appendSharedHookItem(p, item); err != nil {
						return err
					}
				}
			case "opencode-plugin-file-v1":
				item, err := planOpencodePluginLink(p, agent, hook, sourceRoot)
				if err != nil {
					return err
				}
				if err := appendSharedHookItem(p, item); err != nil {
					return err
				}
			case "pi-extension-file-v1":
				item, err := planPiExtensionLink(p, agent, hook, sourceRoot)
				if err != nil {
					return err
				}
				if err := appendSharedHookItem(p, item); err != nil {
					return err
				}
			case "antigravity-hooks-json-v1", "claude-settings-json-v1", "copilot-hooks-json-v1":
				if hook.Settings == "" {
					return fmt.Errorf("integration %q: hook registrar %s has no target", agent.Name, hook.Registrar)
				}
				canonical, err := CanonicalTarget(hook.Settings)
				if err != nil {
					return err
				}
				g := groups[canonical]
				if g == nil {
					g = &hookGroup{canonical: canonical}
					groups[canonical] = g
				}
				if g.registrar == "" {
					g.registrar = hook.Registrar
					g.merge = mergeClaudeSettings
					if hook.Registrar == "antigravity-hooks-json-v1" {
						g.merge = mergeAntigravityHooks
					}
					if hook.Registrar == "copilot-hooks-json-v1" {
						g.merge = mergeCopilotHooks
					}
				} else if g.registrar != hook.Registrar {
					return fmt.Errorf("integration %q: hook settings %s shared by incompatible registrars %s and %s", agent.Name, hook.Settings, g.registrar, hook.Registrar)
				}
				g.declared = append(g.declared, hook.Settings)
				g.owners = append(g.owners, agent.Name)
				command, event, err := protocolHookCommand(p.KBRoot, sourceRoot, hook.Protocol)
				if err != nil {
					return fmt.Errorf("integration %q: %w", agent.Name, err)
				}
				if g.event == "" {
					g.event = event
				} else if g.event != event {
					return fmt.Errorf("integration %q: hook settings %s shared by incompatible events %s and %s", agent.Name, hook.Settings, g.event, event)
				}
				g.commands = append(g.commands, command)
			default:
				return fmt.Errorf("integration %q: unsupported hook registrar %q", agent.Name, hook.Registrar)
			}
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
		g.commands = sortedUnique(g.commands)
		current, err := os.ReadFile(key)
		exists := err == nil
		if os.IsNotExist(err) {
			current = []byte("{}\n")
		} else if err != nil {
			return err
		}
		desired, already, err := g.merge(current, g.commands)
		if err != nil {
			return fmt.Errorf("plan hook settings %s: %w", key, err)
		}
		item := Item{
			ID:              "hook:" + key,
			Kind:            "hook",
			Owners:          g.owners,
			DeclaredTargets: g.declared,
			CanonicalTarget: key,
			IntendedState:   "Regesto's " + g.event + " hook added; other settings kept",
			before:          current,
			desired:         desired,
			mode:            0o644,
		}
		if exists {
			info, err := os.Stat(key)
			if err != nil {
				return err
			}
			item.mode = info.Mode().Perm()
		}
		if exists && already {
			item.Action = "current"
			item.CurrentState = "Regesto's " + g.event + " hook is already set up"
			item.BackupAction = "none"
			item.DryRun = "no change"
		} else if exists {
			item.Action = "update"
			item.CurrentState = "settings file exists, but Regesto's hook is missing"
			item.BackupAction = backupDescription(key, true)
			item.DryRun = "back up the settings file and add Regesto's " + g.event + " hook"
		} else {
			item.Action = "create"
			item.CurrentState = "settings file not found"
			item.before = nil
			item.BackupAction = backupDescription(key, false)
			item.DryRun = "create the settings file with Regesto's " + g.event + " hook"
		}
		p.Items = append(p.Items, item)
	}
	return nil
}

func appendSharedHookItem(p *Plan, item Item) error {
	for i := range p.Items {
		existing := &p.Items[i]
		if existing.ID != item.ID || existing.CanonicalTarget != item.CanonicalTarget {
			continue
		}
		if existing.Kind != item.Kind || existing.Action != item.Action || existing.CurrentState != item.CurrentState ||
			existing.IntendedState != item.IntendedState || existing.BackupAction != item.BackupAction || existing.DryRun != item.DryRun ||
			existing.mode != item.mode || !bytes.Equal(existing.desired, item.desired) || !bytes.Equal(existing.before, item.before) {
			return fmt.Errorf("shared hook target %s has conflicting requirements from %v and %v", item.CanonicalTarget, existing.Owners, item.Owners)
		}
		existing.Owners = sortedUnique(append(existing.Owners, item.Owners...))
		existing.DeclaredTargets = sortedUnique(append(existing.DeclaredTargets, item.DeclaredTargets...))
		return nil
	}
	p.Items = append(p.Items, item)
	return nil
}

func nonempty(value string) []string {
	if value == "" {
		return []string{}
	}
	return []string{value}
}

// hooksSettings is a parsed hooks-style settings file: a JSON object whose
// hooks object maps event names to arrays of command entries. Both supported
// hosts use this shape; they differ only in the entry layout they emit.
type hooksSettings struct {
	root     map[string]any
	sessions []any
	found    map[string]bool
}

// readHooksSettings parses one settings file and indexes the commands already
// registered for the event. Flat entries (`{"type":"command","command":...}`,
// Copilot's native layout) and Claude-style nested matcher groups are both
// recognized, so an install never duplicates a command a host already loaded
// from either layout.
func readHooksSettings(body []byte, event string) (*hooksSettings, error) {
	if err := validateUniqueJSON(body); err != nil {
		return nil, err
	}
	var root map[string]any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&root); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("settings must contain exactly one JSON object")
		}
		return nil, fmt.Errorf("invalid trailing JSON: %w", err)
	}
	if root == nil {
		return nil, fmt.Errorf("settings must be a JSON object")
	}
	hooks, ok := root["hooks"].(map[string]any)
	if !ok {
		if root["hooks"] != nil {
			return nil, fmt.Errorf("hooks must be a JSON object")
		}
		hooks = map[string]any{}
		root["hooks"] = hooks
	}
	sessions, ok := hooks[event].([]any)
	if !ok && hooks[event] != nil {
		return nil, fmt.Errorf("hooks.%s must be an array", event)
	}
	found := map[string]bool{}
	for _, rawSession := range sessions {
		session, _ := rawSession.(map[string]any)
		if command, _ := session["command"].(string); command != "" {
			found[command] = true
		}
		entries, _ := session["hooks"].([]any)
		for _, rawEntry := range entries {
			entry, _ := rawEntry.(map[string]any)
			command, _ := entry["command"].(string)
			if command != "" {
				found[command] = true
			}
		}
	}
	return &hooksSettings{root: root, sessions: sessions, found: found}, nil
}

func (s *hooksSettings) write(event string) ([]byte, error) {
	s.root["hooks"].(map[string]any)[event] = s.sessions
	out, err := json.MarshalIndent(s.root, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}

func (s *hooksSettings) allPresent(commands []string) bool {
	for _, command := range commands {
		if !s.found[command] {
			return false
		}
	}
	return true
}

func mergeClaudeSettings(body []byte, commands []string) ([]byte, bool, error) {
	s, err := readHooksSettings(body, "SessionStart")
	if err != nil {
		return nil, false, err
	}
	for _, command := range commands {
		if s.found[command] {
			continue
		}
		s.sessions = append(s.sessions, map[string]any{"hooks": []any{map[string]any{"type": "command", "command": command}}})
	}
	out, err := s.write("SessionStart")
	if err != nil {
		return nil, false, err
	}
	return out, s.allPresent(commands), nil
}

// mergeCopilotHooks registers SessionStart commands in Copilot's native flat
// entry layout, preserving every foreign key in the settings file.
func mergeCopilotHooks(body []byte, commands []string) ([]byte, bool, error) {
	s, err := readHooksSettings(body, "SessionStart")
	if err != nil {
		return nil, false, err
	}
	for _, command := range commands {
		if s.found[command] {
			continue
		}
		s.sessions = append(s.sessions, map[string]any{"type": "command", "command": command})
	}
	out, err := s.write("SessionStart")
	if err != nil {
		return nil, false, err
	}
	return out, s.allPresent(commands), nil
}

// mergeAntigravityHooks registers PreInvocation handlers under the
// "regesto" hook name in Antigravity's named-hook map. Foreign hook names,
// foreign keys inside the regesto definition (such as `enabled`), and
// foreign handler entries are all preserved.
func mergeAntigravityHooks(body []byte, commands []string) ([]byte, bool, error) {
	if err := validateUniqueJSON(body); err != nil {
		return nil, false, err
	}
	var root map[string]any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&root); err != nil {
		return nil, false, fmt.Errorf("invalid JSON: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, false, fmt.Errorf("settings must contain exactly one JSON object")
		}
		return nil, false, fmt.Errorf("invalid trailing JSON: %w", err)
	}
	if root == nil {
		return nil, false, fmt.Errorf("settings must be a JSON object")
	}
	definition, ok := root["regesto"].(map[string]any)
	if !ok {
		if root["regesto"] != nil {
			return nil, false, fmt.Errorf("regesto hook definition must be an object")
		}
		definition = map[string]any{}
		root["regesto"] = definition
	}
	handlers, ok := definition["PreInvocation"].([]any)
	if !ok && definition["PreInvocation"] != nil {
		return nil, false, fmt.Errorf("regesto.PreInvocation must be an array")
	}
	found := map[string]bool{}
	for _, rawHandler := range handlers {
		handler, _ := rawHandler.(map[string]any)
		if command, _ := handler["command"].(string); command != "" {
			found[command] = true
		}
	}
	allPresent := true
	for _, command := range commands {
		if found[command] {
			continue
		}
		allPresent = false
		handlers = append(handlers, map[string]any{"type": "command", "command": command})
	}
	definition["PreInvocation"] = handlers
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return nil, false, err
	}
	return append(out, '\n'), allPresent, nil
}

func validateUniqueJSON(body []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := consumeJSONValue(decoder); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return fmt.Errorf("settings must contain exactly one JSON value")
		}
		return fmt.Errorf("invalid trailing JSON: %w", err)
	}
	return nil
}

func consumeJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, compound := token.(json.Delim)
	if !compound {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("object key is not a string")
			}
			if seen[key] {
				return fmt.Errorf("duplicate object key %q", key)
			}
			seen[key] = true
			if err := consumeJSONValue(decoder); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil {
			return err
		}
		if closing != json.Delim('}') {
			return fmt.Errorf("unterminated object")
		}
	case '[':
		for decoder.More() {
			if err := consumeJSONValue(decoder); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil {
			return err
		}
		if closing != json.Delim(']') {
			return fmt.Errorf("unterminated array")
		}
	default:
		return fmt.Errorf("unexpected delimiter %q", delim)
	}
	return nil
}
