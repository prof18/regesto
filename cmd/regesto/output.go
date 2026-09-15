package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/prof18/regesto/internal/adapters"
)

// Wrap prose at a conservative terminal width. Keep paths and other long tokens
// intact so they can still be copied; terminals can wrap those naturally.
func printWrapped(first, continuation, text string) {
	// Structured snippets (notably manual YAML instructions) must retain
	// their line breaks and indentation for copying.
	if strings.Contains(text, "\n") {
		lines := strings.Split(text, "\n")
		fmt.Println(first + lines[0])
		for _, line := range lines[1:] {
			fmt.Println(continuation + line)
		}
		return
	}
	line := first
	for _, word := range wrapWords(text) {
		separator := ""
		if line != first && line != continuation {
			separator = " "
		}
		if utf8.RuneCountInString(line+separator+word) > 80 && line != first && line != continuation {
			fmt.Println(line)
			line = continuation + word
		} else {
			line += separator + word
		}
	}
	if strings.TrimSpace(line) != "" {
		fmt.Println(line)
	}
}

// wrapWords splits prose into words, keeping each `backquoted` command whole so
// a command is never broken across two lines.
func wrapWords(text string) []string {
	var words []string
	inCode := false
	for _, field := range strings.Fields(text) {
		if inCode {
			words[len(words)-1] += " " + field
		} else {
			words = append(words, field)
		}
		if strings.Count(field, "`")%2 == 1 {
			inCode = !inCode
		}
	}
	return words
}

// plural renders a count with its noun: "1 fact", "3 facts".
func plural(n int, singular string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, singular)
	}
	if strings.HasSuffix(singular, "y") && !strings.HasSuffix(singular, "ey") {
		return fmt.Sprintf("%d %sies", n, strings.TrimSuffix(singular, "y"))
	}
	if strings.HasSuffix(singular, "s") || strings.HasSuffix(singular, "x") || strings.HasSuffix(singular, "ch") {
		return fmt.Sprintf("%d %ses", n, singular)
	}
	return fmt.Sprintf("%d %ss", n, singular)
}

// tildePath shortens a path under the home directory to ~/..., which is how
// people read and type those paths. Human output only; JSON keeps full paths.
func tildePath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" || path == "" {
		return path
	}
	home = filepath.Clean(home)
	if path == home {
		return "~"
	}
	if strings.HasPrefix(path, home+string(filepath.Separator)) {
		return "~" + path[len(home):]
	}
	return path
}

// tildeText shortens every home-directory path inside a sentence.
func tildeText(text string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return text
	}
	return strings.ReplaceAll(text, filepath.Clean(home)+string(filepath.Separator), "~"+string(filepath.Separator))
}

// integrationNames renders integration ids with their human names.
func integrationNames(ids []string) string {
	names := make([]string, len(ids))
	for i, id := range ids {
		names[i] = adapters.DisplayName(id)
	}
	return joinWords(names)
}

// joinWords joins a list the way a sentence does: "a", "a and b", "a, b and c".
func joinWords(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	default:
		return strings.Join(items[:len(items)-1], ", ") + " and " + items[len(items)-1]
	}
}

// setUsage replaces Go's default "Usage of x:" flag dump with the same layout
// every command uses: a usage line, one sentence on what the command does,
// then its options with double-dash names.
func setUsage(fs *flag.FlagSet, usage, description string) {
	fs.Usage = func() {
		w := fs.Output()
		fmt.Fprintf(w, "Usage: %s\n\n%s\n", usage, description)
		var rows [][2]string
		width := 0
		fs.VisitAll(func(f *flag.Flag) {
			name, text := flag.UnquoteUsage(f)
			left := "--" + f.Name
			if len(f.Name) == 1 {
				left = "-" + f.Name
			}
			switch name {
			case "string":
				name = "value"
			case "int":
				name = "number"
			}
			if name != "" {
				left += " <" + name + ">"
			}
			if f.DefValue != "" && f.DefValue != "false" && f.DefValue != "0" && f.DefValue != "-1" && !strings.Contains(text, "default") {
				text += " (default: " + f.DefValue + ")"
			}
			rows = append(rows, [2]string{left, text})
			if len(left) > width {
				width = len(left)
			}
		})
		if len(rows) == 0 {
			return
		}
		fmt.Fprintln(w, "\nOptions:")
		for _, row := range rows {
			fmt.Fprintf(w, "  %-*s  %s\n", width, row[0], row[1])
		}
	}
}
