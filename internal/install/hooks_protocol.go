package install

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/prof18/regesto/internal/adapters"
)

func planManualHook(p *Plan, agent adapters.Agent, hook adapters.Hook, sourceRoot string) (Item, error) {
	command, event, err := protocolHookCommand(p.KBRoot, sourceRoot, hook.Protocol)
	if err != nil {
		return Item{}, fmt.Errorf("integration %q: %w", agent.Name, err)
	}
	canonical := ""
	if hook.Settings != "" {
		canonical, err = CanonicalTarget(hook.Settings)
		if err != nil {
			return Item{}, err
		}
	}
	recipe := fmt.Sprintf("in this agent's settings, run %s on the %s event", command, event)
	if hook.Protocol == "opencode-chat-message-v1" {
		recipe = fmt.Sprintf("link %s into the host's auto-discovered plugin directory", command)
	}
	idTarget := canonical
	if idTarget == "" {
		idTarget = agent.Name
	}
	return Item{
		ID:              "hook-manual:" + hook.Protocol + ":" + idTarget,
		Kind:            "hook",
		Action:          "manual",
		Owners:          []string{agent.Name},
		DeclaredTargets: nonempty(hook.Settings),
		CanonicalTarget: canonical,
		CurrentState:    "this agent's hook settings can't be edited automatically",
		IntendedState:   recipe,
		BackupAction:    "none",
		DryRun:          "set this up by hand: " + recipe,
	}, nil
}

func planHermesHook(p *Plan, agent adapters.Agent, hook adapters.Hook, sourceRoot string) ([]Item, error) {
	if hook.Settings == "" {
		return nil, fmt.Errorf("integration %q: Hermes registrar has no config target", agent.Name)
	}
	executable, _, err := protocolHookCommand(p.KBRoot, sourceRoot, hook.Protocol)
	if err != nil {
		return nil, fmt.Errorf("integration %q: %w", agent.Name, err)
	}
	command := shellQuote(executable)
	configTarget, err := CanonicalTarget(hook.Settings)
	if err != nil {
		return nil, err
	}
	yaml := []byte("hooks:\n  pre_llm_call:\n    - command: " + strconv.Quote(command) + "\n      timeout: 10\n")
	configItem := Item{
		ID:              "hook-hermes-config:" + configTarget,
		Kind:            "hook",
		Owners:          []string{agent.Name},
		DeclaredTargets: []string{hook.Settings},
		CanonicalTarget: configTarget,
		IntendedState:   "Add this to your Hermes config:\n" + string(yaml),
		desired:         yaml,
		mode:            0o600,
	}
	current, readErr := os.ReadFile(configTarget)
	switch {
	case os.IsNotExist(readErr):
		configItem.Action = "create"
		configItem.CurrentState = "Hermes config file not found"
		configItem.BackupAction = "none"
		configItem.DryRun = "create a Hermes config containing the hook shown"
	case readErr != nil:
		return nil, readErr
	case bytes.Equal(current, yaml):
		configItem.Action = "current"
		configItem.CurrentState = "Hermes hook is already set up"
		configItem.BackupAction = "none"
		configItem.DryRun = "no change"
	default:
		configItem.Action = "manual"
		configItem.CurrentState = "Hermes config already exists; Regesto doesn't edit it automatically"
		configItem.BackupAction = "none"
		configItem.DryRun = "copy the hook shown into this file by hand"
	}

	allowlistTarget := filepath.Join(filepath.Dir(hook.Settings), "shell-hooks-allowlist.json")
	allowlistCanonical, err := CanonicalTarget(allowlistTarget)
	if err != nil {
		return nil, err
	}
	allowlistCurrent, allowlistErr := os.ReadFile(allowlistCanonical)
	allowlistExists := allowlistErr == nil
	if os.IsNotExist(allowlistErr) {
		allowlistCurrent = []byte("{}\n")
	} else if allowlistErr != nil {
		return nil, allowlistErr
	}
	allowlistDesired, already, err := mergeHermesAllowlist(allowlistCurrent, command)
	if err != nil {
		return nil, fmt.Errorf("plan Hermes hook allowlist %s: %w", allowlistCanonical, err)
	}
	allowlistItem := Item{
		ID:              "hook-hermes-allowlist:" + allowlistCanonical,
		Kind:            "hook",
		Owners:          []string{agent.Name},
		DeclaredTargets: []string{allowlistTarget},
		CanonicalTarget: allowlistCanonical,
		IntendedState:   "Regesto's hook approved; other approvals kept",
		before:          allowlistCurrent,
		desired:         allowlistDesired,
		mode:            0o600,
	}
	if allowlistExists {
		info, err := os.Stat(allowlistCanonical)
		if err != nil {
			return nil, err
		}
		allowlistItem.mode = info.Mode().Perm()
	}
	switch {
	case allowlistExists && already:
		allowlistItem.Action = "current"
		allowlistItem.CurrentState = "Regesto's hook is already approved"
		allowlistItem.BackupAction = "none"
		allowlistItem.DryRun = "no change"
	case allowlistExists:
		allowlistItem.Action = "update"
		allowlistItem.CurrentState = "Hermes allowlist exists, but doesn't approve Regesto's hook"
		allowlistItem.BackupAction = backupDescription(allowlistCanonical, true)
		allowlistItem.DryRun = "back up the allowlist and approve Regesto's hook"
	default:
		allowlistItem.Action = "create"
		allowlistItem.CurrentState = "Hermes allowlist not found"
		allowlistItem.before = nil
		allowlistItem.BackupAction = backupDescription(allowlistCanonical, false)
		allowlistItem.DryRun = "create the Hermes allowlist approving Regesto's hook"
	}
	return []Item{configItem, allowlistItem}, nil
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func protocolHookCommand(root, sourceRoot, protocol string) (command, event string, err error) {
	var relative string
	switch protocol {
	case "antigravity-pre-invocation-v1":
		relative, event = filepath.Join("adapters", "antigravity", "hooks", "pre-invocation.sh"), "PreInvocation"
	case "claude-session-start-v1":
		relative, event = filepath.Join("adapters", "claude", "hooks", "session-start.sh"), "SessionStart"
	case "copilot-session-start-v1":
		relative, event = filepath.Join("adapters", "copilot", "hooks", "session-start.sh"), "SessionStart"
	case "hermes-pre-llm-v1":
		relative, event = filepath.Join("adapters", "hermes", "hooks", "pre-llm.sh"), "pre_llm_call"
	case "opencode-chat-message-v1":
		// The plugin is loaded, not executed, so a regular file is enough
		// and no executable bit is required.
		relative, event = filepath.FromSlash(opencodePluginRelative), "chat.message"
		command = filepath.Join(root, relative)
		if err := requirePluginLauncher(sourceRoot); err != nil {
			return "", "", err
		}
		return command, event, nil
	default:
		return "", "", fmt.Errorf("unsupported hook protocol %q", protocol)
	}
	command = filepath.Join(root, relative)
	source := filepath.Join(sourceRoot, relative)
	info, statErr := os.Stat(source)
	if statErr != nil {
		return "", "", fmt.Errorf("hook executable %s is unavailable: %w", command, statErr)
	}
	if info.IsDir() || info.Mode()&0o111 == 0 {
		return "", "", fmt.Errorf("hook is not executable: %s", command)
	}
	return command, event, nil
}

func mergeHermesAllowlist(body []byte, command string) ([]byte, bool, error) {
	if err := validateUniqueJSON(body); err != nil {
		return nil, false, err
	}
	var root map[string]any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&root); err != nil {
		return nil, false, fmt.Errorf("invalid JSON: %w", err)
	}
	if root == nil {
		return nil, false, fmt.Errorf("allowlist must be a JSON object")
	}
	approvals, ok := root["approvals"].([]any)
	if !ok && root["approvals"] != nil {
		return nil, false, fmt.Errorf("approvals must be an array")
	}
	for _, raw := range approvals {
		approval, ok := raw.(map[string]any)
		if ok && approval["event"] == "pre_llm_call" && approval["command"] == command {
			return body, true, nil
		}
	}
	root["approvals"] = append(approvals, map[string]any{"event": "pre_llm_call", "command": command})
	desired, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return nil, false, err
	}
	return append(desired, '\n'), false, nil
}
