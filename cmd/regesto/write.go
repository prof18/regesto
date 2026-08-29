package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/prof18/regesto/internal/config"
	"github.com/prof18/regesto/internal/project"
	writeop "github.com/prof18/regesto/internal/write"
)

func runWrite(cfg *config.Config, args []string) error {
	fs := flag.NewFlagSet("write", flag.ContinueOnError)
	source := fs.String("source", "", "authoritative provenance for the new fact")
	dir := fs.String("dir", "", "resolve input scope `project` from this directory")
	jsonInput := fs.Bool("json-input", false, "read one strict JSON write object from stdin")
	jsonOutput := fs.Bool("json", false, "print the write result as JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return fmt.Errorf("write: unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	if strings.TrimSpace(*source) == "" {
		return fmt.Errorf("write: --source is required")
	}
	if !*jsonInput {
		return fmt.Errorf("write: --json-input is required")
	}

	input, err := decodeWriteInput(os.Stdin)
	if err != nil {
		return fmt.Errorf("write: invalid JSON input: %w", err)
	}

	if *dir != "" {
		if input.Scope != "project" {
			return fmt.Errorf("write: --dir requires input scope %q", "project")
		}
		resolved := project.Resolve(cfg, *dir)
		input.Scope = "project:" + resolved.Name
	} else if input.Scope == "project" {
		return fmt.Errorf("write: input scope %q requires --dir", "project")
	}

	result, err := writeop.Create(cfg, input, writeop.Authority{Source: *source})
	if err != nil {
		return err
	}
	if *jsonOutput {
		return json.NewEncoder(os.Stdout).Encode(result)
	}
	fmt.Println(result.Path)
	if result.PendingReconciliation {
		fmt.Println("pending reconciliation:")
		for _, action := range result.Actions {
			fmt.Printf("  %s %s: %s\n", action.Kind, action.ID, action.Message)
		}
		for _, review := range result.Reviews {
			fmt.Printf("  review: %s\n", review)
		}
	}
	return nil
}

func decodeWriteInput(r io.Reader) (writeop.Input, error) {
	decoder := json.NewDecoder(r)
	first, err := decoder.Token()
	if err != nil {
		return writeop.Input{}, err
	}
	if delim, ok := first.(json.Delim); !ok || delim != '{' {
		return writeop.Input{}, fmt.Errorf("input must be one JSON object")
	}
	fields := map[string]json.RawMessage{}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return writeop.Input{}, err
		}
		key, ok := token.(string)
		if !ok {
			return writeop.Input{}, fmt.Errorf("object key is not a string")
		}
		if _, duplicate := fields[key]; duplicate {
			return writeop.Input{}, fmt.Errorf("duplicate field %q", key)
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return writeop.Input{}, err
		}
		fields[key] = value
	}
	closing, err := decoder.Token()
	if err != nil {
		return writeop.Input{}, err
	}
	if delim, ok := closing.(json.Delim); !ok || delim != '}' {
		return writeop.Input{}, fmt.Errorf("unterminated JSON object")
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return writeop.Input{}, fmt.Errorf("input must contain exactly one JSON object")
		}
		return writeop.Input{}, err
	}

	canonical, err := json.Marshal(fields)
	if err != nil {
		return writeop.Input{}, err
	}
	strict := json.NewDecoder(bytes.NewReader(canonical))
	strict.DisallowUnknownFields()
	var input writeop.Input
	if err := strict.Decode(&input); err != nil {
		return writeop.Input{}, err
	}
	return input, nil
}
