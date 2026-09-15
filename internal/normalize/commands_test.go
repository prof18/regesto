package normalize

import (
	"errors"
	"strings"
	"testing"
)

func withInstalled(t *testing.T, installed ...string) {
	t.Helper()
	previous := lookPath
	lookPath = func(name string) (string, error) {
		for _, candidate := range installed {
			if candidate == name {
				return "/bin/" + name, nil
			}
		}
		return "", errors.New("not found")
	}
	t.Cleanup(func() { lookPath = previous })
}

func TestDefaultChainFollowsConnectedAgents(t *testing.T) {
	withInstalled(t, "copilot", "agy", "hermes")
	first := func(chain []string) string { return strings.Fields(chain[0])[0] }

	if got := CommandsFor("", "", []string{"claude", "hermes"}); len(got) != 2 || first(got) != "claude" {
		t.Errorf("claude connected: %v", got)
	}
	if got := CommandsFor("", "", []string{"codex"}); len(got) != 2 || first(got) != "codex" {
		t.Errorf("only codex connected should try codex first: %v", got)
	}
	got := CommandsFor("", "", []string{"hermes"})
	if len(got) != 3 || first(got) != "hermes" || !strings.HasPrefix(got[1], "copilot") || !strings.HasPrefix(got[2], "agy") {
		t.Errorf("without claude or codex, connected others first, then installed ones: %v", got)
	}
	if got := CommandsFor("mine --x", "", []string{"hermes"}); len(got) != 1 || got[0] != "mine --x" {
		t.Errorf("override must win: %v", got)
	}
	if got := CommandsFor("", "a ;; b", []string{"hermes"}); len(got) != 2 || got[0] != "a" {
		t.Errorf("configured chain must win: %v", got)
	}

	withInstalled(t)
	if got := CommandsFor("", "", []string{"hermes"}); !sameCommands(got, DefaultCommands) {
		t.Errorf("nothing installed should keep the claude and codex default: %v", got)
	}
}

func TestPromptArgumentReplacesStdin(t *testing.T) {
	out, err := invoke(Options{}, "printf %s "+PromptArgument, "the prompt text")
	if err != nil || out != "the prompt text" {
		t.Fatalf("prompt argument not passed: %q, %v", out, err)
	}
}
