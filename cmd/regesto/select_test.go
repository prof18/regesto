package main

import (
	"os"
	"strings"
	"testing"
)

func TestPickerStartsAllChecked(t *testing.T) {
	p := newPicker([]string{"claude", "codex", "hermes"})
	if got := p.selected(); strings.Join(got, ",") != "claude,codex,hermes" {
		t.Fatalf("new picker should start fully checked, got %v", got)
	}
}

func TestPickerSpaceTogglesOnlyCursor(t *testing.T) {
	p := newPicker([]string{"claude", "codex", "hermes"})
	if p.handle([]byte{' '}) != pickerNone {
		t.Fatal("space should not finish the picker")
	}
	if p.checked[0] {
		t.Fatal("first item should be unchecked after space")
	}
	if !p.checked[1] || !p.checked[2] {
		t.Fatal("space must not touch other items")
	}
	if got := p.selected(); strings.Join(got, ",") != "codex,hermes" {
		t.Fatalf("selected = %v, want [codex hermes]", got)
	}
	// Toggling back restores it, and the order is still list order.
	p.handle([]byte{' '})
	if got := p.selected(); strings.Join(got, ",") != "claude,codex,hermes" {
		t.Fatalf("selected after re-toggle = %v", got)
	}
}

func TestPickerMovementWraps(t *testing.T) {
	p := newPicker([]string{"claude", "codex", "hermes"})
	if p.cursor != 0 {
		t.Fatalf("cursor starts at 0, got %d", p.cursor)
	}
	p.handle([]byte{'j'})
	p.handle([]byte("\x1b[B"))
	if p.cursor != 2 {
		t.Fatalf("cursor after two downs = %d, want 2", p.cursor)
	}
	p.handle([]byte{'j'}) // wraps to 0
	if p.cursor != 0 {
		t.Fatalf("cursor should wrap down to 0, got %d", p.cursor)
	}
	p.handle([]byte{'k'}) // wraps back to the end
	if p.cursor != 2 {
		t.Fatalf("cursor should wrap up to 2, got %d", p.cursor)
	}
	p.handle([]byte("\x1b[A"))
	if p.cursor != 1 {
		t.Fatalf("cursor after up arrow = %d, want 1", p.cursor)
	}
}

func TestPickerToggleTracksCursor(t *testing.T) {
	p := newPicker([]string{"claude", "codex", "hermes"})
	p.handle([]byte{'j'})
	p.handle([]byte{'j'})
	p.handle([]byte{' '})
	if p.checked[2] {
		t.Fatal("third item should be unchecked")
	}
	if !p.checked[0] || !p.checked[1] {
		t.Fatal("other items must stay checked")
	}
}

func TestPickerConfirmKeys(t *testing.T) {
	for _, key := range []string{"\n", "\r"} {
		if newPicker([]string{"claude"}).handle([]byte(key)) != pickerConfirm {
			t.Fatalf("key %q should confirm", key)
		}
	}
}

func TestPickerAbortKeys(t *testing.T) {
	for _, key := range [][]byte{{'q'}, {27}, {3}, {4}} {
		if newPicker([]string{"claude"}).handle(key) != pickerAbort {
			t.Fatalf("key %q should abort", key)
		}
	}
}

func TestPickerHandlesKeyBursts(t *testing.T) {
	// One read can carry several keys — fast typing, a paste. The whole burst
	// must apply, and a confirm inside it stops the walk.
	p := newPicker([]string{"antigravity", "claude", "codex"})
	if act := p.handle([]byte("jj \r")); act != pickerConfirm {
		t.Fatalf("burst ending in enter should confirm, got %v", act)
	}
	if got := p.selected(); strings.Join(got, ",") != "antigravity,claude" {
		t.Fatalf("burst jj-space should uncheck the third item, got %v", got)
	}

	q := newPicker([]string{"antigravity", "claude", "codex"})
	if act := q.handle([]byte("\x1b[B\x1b[B ")); act != pickerNone {
		t.Fatalf("arrow burst should not finish the picker, got %v", act)
	}
	if q.cursor != 2 {
		t.Fatalf("arrow burst should land on the third item, cursor=%d", q.cursor)
	}
	if q.checked[2] {
		t.Fatal("space after the arrow burst should have unchecked the third item")
	}
}

func TestPickerApplicationModeArrows(t *testing.T) {
	// Terminals in cursor-key application mode (tmux, some full-screen apps)
	// send SS3 arrows: ESC O A/B instead of ESC [ A/B. They must move, not
	// read as a lone ESC and abort.
	p := newPicker([]string{"antigravity", "claude", "codex"})
	if act := p.handle([]byte("\x1bOB")); act != pickerNone {
		t.Fatalf("SS3 down arrow should not finish the picker, got %v", act)
	}
	if p.cursor != 1 {
		t.Fatalf("SS3 down arrow should move the cursor, at %d", p.cursor)
	}
	if act := p.handle([]byte("\x1bOA")); act != pickerNone {
		t.Fatalf("SS3 up arrow should not finish the picker, got %v", act)
	}
	if p.cursor != 0 {
		t.Fatalf("SS3 up arrow should move the cursor, at %d", p.cursor)
	}
}

func TestDrawFrameEndsWithNewlineAndClearsPrevLines(t *testing.T) {
	// The cursor-up accounting in drawFrame/clearFrame is only exact when a
	// drawn frame ends with a newline: the cursor then rests one row below the
	// frame at column 0, and prev ups land on the frame's first row. This is
	// the assertion that fails if someone removes that newline again.
	var b strings.Builder
	drawFrame(&b, []string{"a", "b"}, 0)
	out := b.String()
	if !strings.HasSuffix(out, "a\nb\n") {
		t.Fatalf("first draw should end with a newline, got %q", out)
	}
	if strings.Contains(out, "\x1b[1A") {
		t.Fatalf("first draw should not move the cursor up, got %q", out)
	}

	b.Reset()
	drawFrame(&b, []string{"a", "b"}, 2)
	out = b.String()
	if got := strings.Count(out, "\x1b[1A\x1b[2K"); got != 2 {
		t.Fatalf("redraw clears prev lines, got %d ups, output %q", got, out)
	}
	if !strings.HasSuffix(out, "a\nb\n") {
		t.Fatalf("redraw should end with a newline, got %q", out)
	}

	b.Reset()
	clearFrame(&b, 2)
	out = b.String()
	if got := strings.Count(out, "\x1b[1A\x1b[2K"); got != 2 {
		t.Fatalf("clear erases exactly the frame, got %d ups, output %q", got, out)
	}
	if !strings.HasSuffix(out, "\x1b[J") {
		t.Fatalf("clear should end with erase-below, got %q", out)
	}
}

func TestClipLinesKeepsLineCountAndWidth(t *testing.T) {
	// Wrapping is what breaks the redraw accounting; clipping every line to
	// the terminal width prevents wrapping and must not change line count.
	lines := []string{
		"Integrations detected — uncheck any Regesto should not manage:",
		"",
		"  > [x] antigravity",
		"up/down or j/k move, space toggles, enter confirms, q aborts",
	}
	clipped := clipLines(lines, 40)
	if len(clipped) != len(lines) {
		t.Fatalf("clip changed the line count: %d → %d", len(lines), len(clipped))
	}
	for i, l := range clipped {
		if len([]rune(l)) > 40 {
			t.Fatalf("line %d still wider than 40: %q", i, l)
		}
	}
	if clipped[1] != "" || clipped[2] != lines[2] {
		t.Fatalf("short lines must be untouched: %q %q", clipped[1], clipped[2])
	}
	if same := clipLines(lines, 0); &same == &lines {
		// width 0 disables clipping; identity is the important part
		_ = same
	}
	for i := range clipLines(lines, 0) {
		if clipLines(lines, 0)[i] != lines[i] {
			t.Fatalf("width 0 must leave lines unchanged: %q", lines[i])
		}
	}
}

func TestPickerIgnoresOtherKeys(t *testing.T) {
	p := newPicker([]string{"claude", "codex"})
	for _, key := range []string{"x", "\x1b[C", "\x1b[D", "\t"} {
		if p.handle([]byte(key)) != pickerNone {
			t.Fatalf("key %q should be ignored", key)
		}
		if got := p.selected(); len(got) != 2 {
			t.Fatalf("key %q changed the selection: %v", key, got)
		}
	}
}

func TestPickerRenderShape(t *testing.T) {
	p := newPicker([]string{"claude", "codex"})
	p.handle([]byte{' '}) // uncheck first, cursor stays on it
	lines := p.render()
	if len(lines) != len(p.items)+4 {
		t.Fatalf("frame is %d lines, want %d", len(lines), len(p.items)+4)
	}
	if !strings.Contains(lines[2], "> [ ] claude") {
		t.Fatalf("cursor line should show unchecked claude: %q", lines[2])
	}
	if !strings.Contains(lines[3], "  [x] codex") {
		t.Fatalf("second line should show checked codex: %q", lines[3])
	}
	// Toggling must not change the frame's line count: redraw counts lines.
	p.handle([]byte{'j'})
	if len(p.render()) != len(lines) {
		t.Fatal("frame line count changed after a key")
	}
}

func TestSelectIntegrationsSkipsWithoutTerminal(t *testing.T) {
	// In `go test`, stdin is not usable as an interactive terminal even when
	// it is a character device, so the picker must decline, not hang.
	if _, err := selectIntegrations(os.Stdin, os.Stdout, []string{"claude"}); err == nil {
		t.Fatal("expected an error, got a selection")
	}
}

func TestParseIntegrationsFlag(t *testing.T) {
	got, err := parseIntegrationsFlag("claude, codex,claude")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, ",") != "claude,codex" {
		t.Fatalf("parsed %v, want [claude codex]", got)
	}
}

func TestParseIntegrationsFlagRejectsUnknown(t *testing.T) {
	_, err := parseIntegrationsFlag("claude, nosuch")
	if err == nil {
		t.Fatal("unknown integration should be rejected")
	}
	if !strings.Contains(err.Error(), "claude") || !strings.Contains(err.Error(), "hermes") {
		t.Fatalf("error should list known integrations: %v", err)
	}
}
