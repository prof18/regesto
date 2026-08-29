// Package hooks translates host hook payloads into their exact response
// protocols. Known protocols are deliberately fail-open: malformed input or an
// unavailable knowledge base produces the host's empty response and exit zero.
package hooks

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/prof18/regesto/internal/config"
)

const maxPayloadBytes = 1 << 20

// Run reads one host payload and writes only that host's response framing.
// Diagnostics are isolated on diagnostic; operational failures in a known hook
// never propagate to the host as a non-zero exit.
func Run(cfg *config.Config, protocol string, input io.Reader, output, diagnostic io.Writer) error {
	payload, readErr := readPayload(input)
	switch protocol {
	case "claude-session-start-v1":
		if readErr != nil {
			diagnose(diagnostic, protocol, readErr)
			return nil
		}
		context, err := runClaude(cfg, payload)
		if err != nil {
			diagnose(diagnostic, protocol, err)
			return nil
		}
		if _, err := io.WriteString(output, context); err != nil {
			diagnose(diagnostic, protocol, err)
		}
		return nil

	case "hermes-pre-llm-v1":
		response := []byte("{}")
		if readErr != nil {
			diagnose(diagnostic, protocol, readErr)
		} else if context, err := runHermes(cfg, payload); err != nil {
			diagnose(diagnostic, protocol, err)
		} else if context != "" {
			response, err = json.Marshal(struct {
				Context string `json:"context"`
			}{Context: context})
			if err != nil {
				diagnose(diagnostic, protocol, err)
				response = []byte("{}")
			}
		}
		if _, err := output.Write(response); err != nil {
			diagnose(diagnostic, protocol, err)
		}
		return nil

	default:
		return fmt.Errorf("unknown hook protocol %q", protocol)
	}
}

func readPayload(input io.Reader) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(input, maxPayloadBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxPayloadBytes {
		return nil, fmt.Errorf("hook payload exceeds %d bytes", maxPayloadBytes)
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return nil, fmt.Errorf("hook payload is empty")
	}
	return body, nil
}

func decodePayload(body []byte, out any) error {
	if err := validateUniqueJSON(body); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	if err := decoder.Decode(out); err != nil {
		return fmt.Errorf("invalid hook JSON: %w", err)
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return fmt.Errorf("invalid hook JSON: trailing value")
	}
	return nil
}

func validateUniqueJSON(body []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := validateJSONValue(decoder); err != nil {
		return fmt.Errorf("invalid hook JSON: %w", err)
	}
	if token, err := decoder.Token(); err != io.EOF {
		if err != nil {
			return fmt.Errorf("invalid hook JSON: %w", err)
		}
		return fmt.Errorf("invalid hook JSON: trailing token %v", token)
	}
	return nil
}

func validateJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
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
			if err := validateJSONValue(decoder); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim('}') {
			return fmt.Errorf("unterminated object")
		}
	case '[':
		for decoder.More() {
			if err := validateJSONValue(decoder); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim(']') {
			return fmt.Errorf("unterminated array")
		}
	default:
		return fmt.Errorf("unexpected delimiter %q", delim)
	}
	return nil
}

func diagnose(output io.Writer, protocol string, err error) {
	if output != nil {
		fmt.Fprintf(output, "regesto hook %s: %v\n", protocol, err)
	}
}
