package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// Why a hand-rolled picker: go.mod has no dependencies and the engine keeps it
// that way. The selection UI init needs is one fixed-size list — no scrolling,
// no wrapping, no text entry — so the state machine below plus a cbreak-mode
// terminal (see select_darwin.go / select_linux.go) is the whole job.
//
// The state machine is deliberately pure: it eats key bytes and returns what
// happened, so tests drive it without a terminal.

// errNotATerminal tells the caller the picker cannot run — stdin or stdout is
// not a usable terminal, or the platform has no raw-mode support. Callers treat
// it as "fall back to non-interactive behavior", never as a user error.
var errNotATerminal = errors.New("not an interactive terminal")

// errAborted is the user pressing q, Esc or Ctrl-C: stop, write nothing.
var errAborted = errors.New("selection aborted")

type pickerAction int

const (
	pickerNone pickerAction = iota
	pickerConfirm
	pickerAbort
)

// picker is the state of one checkbox list: the items, which are checked, and
// where the cursor is. newPicker starts with everything checked, so pressing
// enter immediately reproduces the pre-picker behavior (configure every
// detected integration) and a forbidden tool is an explicit opt-out. Callers
// that need a different starting state or heading adjust the fields before
// handing it to runPicker.
type picker struct {
	title   string
	items   []string
	checked []bool
	cursor  int
}

func newPicker(items []string) *picker {
	checked := make([]bool, len(items))
	for i := range checked {
		checked[i] = true
	}
	return &picker{
		title:   "Integrations detected — uncheck any Regesto should not manage:",
		items:   items,
		checked: checked,
	}
}

// handle applies one read's worth of key bytes — a burst of typing or a paste
// can deliver several keys at once — and reports the first outcome that ends
// the picker, if any.
func (p *picker) handle(b []byte) pickerAction {
	for i := 0; i < len(b); {
		var action pickerAction
		// Both escape introducers open a 3-byte key: CSI (ESC [ A) normally,
		// SS3 (ESC O A) when the terminal is in cursor-key application mode —
		// left set by tmux configurations and some full-screen apps.
		if b[i] == 27 && i+2 < len(b) && (b[i+1] == '[' || b[i+1] == 'O') {
			action = p.apply(b[i : i+3])
			i += 3
		} else {
			action = p.apply(b[i : i+1])
			i++
		}
		if action != pickerNone {
			return action
		}
	}
	return pickerNone
}

// apply applies one key — a single byte, or a 3-byte escape sequence (a
// terminal sends an arrow atomically, so a split sequence is not a case worth
// distinguishing from a lone ESC) — and reports whether it ends the picker.
// Movement wraps around.
func (p *picker) apply(key []byte) pickerAction {
	switch {
	case key[0] == '\n' || key[0] == '\r':
		return pickerConfirm
	case key[0] == 3 || key[0] == 4: // Ctrl-C and Ctrl-D (EOF), bytes because ISIG is off
		return pickerAbort
	case len(key) == 1:
		switch key[0] {
		case 'q', 27:
			return pickerAbort
		case 'j':
			p.down()
		case 'k':
			p.up()
		case ' ':
			if len(p.items) > 0 {
				p.checked[p.cursor] = !p.checked[p.cursor]
			}
		}
	case key[0] == 27 && (key[1] == '[' || key[1] == 'O'):
		switch key[2] {
		case 'A':
			p.up()
		case 'B':
			p.down()
		}
	}
	return pickerNone
}

func (p *picker) up() {
	if len(p.items) > 0 {
		p.cursor = (p.cursor - 1 + len(p.items)) % len(p.items)
	}
}

func (p *picker) down() {
	if len(p.items) > 0 {
		p.cursor = (p.cursor + 1) % len(p.items)
	}
}

// selected returns the checked ids in list order.
func (p *picker) selected() []string {
	out := make([]string, 0, len(p.items))
	for i, id := range p.items {
		if p.checked[i] {
			out = append(out, id)
		}
	}
	return out
}

// render returns the frame as lines. The line count must stay constant across
// redraws: the terminal driver moves the cursor up by the previous frame's line
// count to overwrite it, so a frame that shrinks or grows would smear.
func (p *picker) render() []string {
	lines := make([]string, 0, len(p.items)+4)
	lines = append(lines, p.title, "")
	for i, id := range p.items {
		cursor, box := " ", "[ ]"
		if i == p.cursor {
			cursor = ">"
		}
		if p.checked[i] {
			box = "[x]"
		}
		lines = append(lines, fmt.Sprintf("  %s %s %s", cursor, box, id))
	}
	lines = append(lines, "", "up/down or j/k move, space toggles, enter confirms, q aborts")
	return lines
}

// selectIntegrations runs the init picker — everything detected, all
// pre-checked — on a terminal. See runPicker for the terminal contract.
func selectIntegrations(in, out *os.File, items []string) ([]string, error) {
	if len(items) == 0 {
		return nil, nil
	}
	return runPicker(in, out, newPicker(items))
}

// runPicker runs a prepared picker on a terminal: draw, read a key, apply,
// redraw, until the user confirms or aborts. It draws and erases with plain
// ANSI (cursor-up + line-clear), which every target terminal understands, and
// restores the terminal mode on every exit path.
//
// Returns errNotATerminal when it cannot run — the caller decides the
// fallback, so this function never silently guesses a selection.
func runPicker(in, out *os.File, p *picker) ([]string, error) {
	if len(p.items) == 0 {
		return nil, nil
	}
	if !isCharDevice(in) || !isCharDevice(out) {
		return nil, errNotATerminal
	}
	restore, err := makeRaw(in)
	if err != nil {
		return nil, errNotATerminal
	}
	defer restore()

	// The redraw accounting assumes one line = one terminal row. A line wider
	// than the terminal wraps and breaks that, so every frame line is clipped
	// to the terminal width first; width 0 (unknown) means no clipping. The
	// line count is untouched — clipping never changes the count.
	width := terminalWidth(out)
	lines := clipLines(p.render(), width)
	drawFrame(out, lines, 0)
	prev := len(lines)

	buf := make([]byte, 16)
	for {
		n, err := in.Read(buf)
		if err != nil {
			clearFrame(out, prev) // leave the terminal as clean as the other exits
			return nil, err
		}
		switch p.handle(buf[:n]) {
		case pickerConfirm:
			clearFrame(out, prev)
			return p.selected(), nil
		case pickerAbort:
			clearFrame(out, prev)
			return nil, errAborted
		}
		lines = clipLines(p.render(), width)
		drawFrame(out, lines, prev)
	}
}

// clipLines truncates each line to at most width cells (by rune: every rune
// in these frames is single-width). A width of 0 or less disables clipping.
func clipLines(lines []string, width int) []string {
	if width <= 0 {
		return lines
	}
	out := make([]string, len(lines))
	for i, l := range lines {
		if len(l) <= width { // byte length bounds the rune count from above
			out[i] = l
			continue
		}
		runes := []rune(l)
		if len(runes) > width {
			runes = runes[:width]
		}
		out[i] = string(runes)
	}
	return out
}

// drawFrame overwrites the previous frame (prev lines above the cursor) with
// lines. prev is 0 for the first draw.
//
// Every frame ends with a newline: with output processing on, that leaves the
// cursor one row *below* the frame at column 0, so prev cursor-ups land on the
// frame's first row exactly — the count stays true even when the bottom of the
// frame hits the last screen row and the newline scrolls the terminal, because
// the frame top moves up with it. Without the trailing newline the cursor ends
// inside the last line and every redraw drifts up one row and right one
// column.
func drawFrame(w io.Writer, lines []string, prev int) {
	var b strings.Builder
	for i := 0; i < prev; i++ {
		b.WriteString("\x1b[1A\x1b[2K")
	}
	b.WriteString("\x1b[J") // erase anything below, in case the frame shrank
	b.WriteString(strings.Join(lines, "\n"))
	b.WriteString("\n")
	fmt.Fprint(w, b.String())
}

// clearFrame erases a drawn frame, leaving the cursor where the frame began.
func clearFrame(w io.Writer, prev int) {
	var b strings.Builder
	for i := 0; i < prev; i++ {
		b.WriteString("\x1b[1A\x1b[2K")
	}
	b.WriteString("\x1b[J")
	fmt.Fprint(w, b.String())
}

// isCharDevice is the same check promote.go uses to tell a pipe or file from a
// terminal. It is a cheap first gate only: /dev/null is also a character
// device, so the real gate is makeRaw failing with ENOTTY.
func isCharDevice(f *os.File) bool {
	stat, err := f.Stat()
	return err == nil && stat.Mode()&os.ModeCharDevice != 0
}
