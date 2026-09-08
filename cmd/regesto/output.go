package main

import (
	"fmt"
	"strings"
	"unicode/utf8"
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
	for _, word := range strings.Fields(text) {
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
