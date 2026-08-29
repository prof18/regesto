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
	recipe := fmt.Sprintf("register event %s with command %s for protocol %s", event, command, hook.Protocol)
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
		CurrentState:    "custom host registration is not automatically editable",
		IntendedState:   recipe,
		BackupAction:    "none",
		DryRun:          recipe + "; no file will be rewritten",
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
		IntendedState:   "Hermes pre_llm_call registration:\n" + string(yaml),
		desired:         yaml,
		mode:            0o600,
	}
	current, readErr := os.ReadFile(configTarget)
	switch {
	case os.IsNotExist(readErr):
		configItem.Action = "create"
		configItem.CurrentState = "Hermes config missing"
		configItem.BackupAction = "none (target does not exist)"
		configItem.DryRun = "create the displayed minimal Hermes hook config"
	case readErr != nil:
		return nil, readErr
	case bytes.Equal(current, yaml):
		configItem.Action = "current"
		configItem.CurrentState = "exact Regesto-only Hermes hook config present"
		configItem.BackupAction = "none"
		configItem.DryRun = "leave Hermes hook config byte-for-byte unchanged"
	default:
		configItem.Action = "manual"
		configItem.CurrentState = "existing Hermes YAML preserved because a lossless merge is not proven"
		configItem.BackupAction = "none (no automatic YAML mutation)"
		configItem.DryRun = "manually merge the displayed pre_llm_call YAML into this file; no YAML will be rewritten"
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
		IntendedState:   "approve the exact pre_llm_call command while preserving foreign approvals",
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
		allowlistItem.CurrentState = "exact Hermes event/command approval present"
		allowlistItem.BackupAction = "none"
		allowlistItem.DryRun = "leave Hermes hook allowlist byte-for-byte unchanged"
	case allowlistExists:
		allowlistItem.Action = "update"
		allowlistItem.CurrentState = "Hermes allowlist present; exact approval missing"
		allowlistItem.BackupAction = backupDescription(allowlistCanonical, true)
		allowlistItem.DryRun = "back up the allowlist and append the exact event/command approval"
	default:
		allowlistItem.Action = "create"
		allowlistItem.CurrentState = "Hermes allowlist missing"
		allowlistItem.before = nil
		allowlistItem.BackupAction = backupDescription(allowlistCanonical, false)
		allowlistItem.DryRun = "create the Hermes allowlist with the exact event/command approval"
	}
	return []Item{configItem, allowlistItem}, nil
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func protocolHookCommand(root, sourceRoot, protocol string) (command, event string, err error) {
	var relative string
	switch protocol {
	case "claude-session-start-v1":
		relative, event = filepath.Join("adapters", "claude", "hooks", "session-start.sh"), "SessionStart"
	case "hermes-pre-llm-v1":
		relative, event = filepath.Join("adapters", "hermes", "hooks", "pre-llm.sh"), "pre_llm_call"
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
