package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"unicode/utf8"
)

const (
	jsonRPCVersion = "2.0"
	maxJSONDepth   = 64

	codeParseError     = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
	codeInternalError  = -32603
	codeNotInitialized = -32002
)

type request struct {
	ID     json.RawMessage
	HasID  bool
	Method string
	Params json.RawMessage
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

func decodeRequest(body []byte) (request, *rpcError) {
	if !utf8.Valid(body) {
		return request{}, &rpcError{Code: codeParseError, Message: "Parse error", Data: "message is not valid UTF-8"}
	}
	if err := validateUniqueJSON(body); err != nil {
		return request{}, &rpcError{Code: codeParseError, Message: "Parse error", Data: err.Error()}
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil || fields == nil {
		return request{}, &rpcError{Code: codeInvalidRequest, Message: "Invalid Request"}
	}

	var version string
	if raw, ok := fields["jsonrpc"]; !ok || json.Unmarshal(raw, &version) != nil || version != jsonRPCVersion {
		return request{}, &rpcError{Code: codeInvalidRequest, Message: "Invalid Request", Data: "jsonrpc must be \"2.0\""}
	}
	var method string
	if raw, ok := fields["method"]; !ok || json.Unmarshal(raw, &method) != nil || method == "" {
		return request{}, &rpcError{Code: codeInvalidRequest, Message: "Invalid Request", Data: "method must be a nonempty string"}
	}

	r := request{Method: method, Params: fields["params"]}
	if raw, ok := fields["id"]; ok {
		r.HasID = true
		r.ID = append(json.RawMessage(nil), raw...)
		if !validRequestID(raw) {
			return request{}, &rpcError{Code: codeInvalidRequest, Message: "Invalid Request", Data: "id must be a string or number"}
		}
	}
	if raw := r.Params; len(raw) != 0 {
		trimmed := bytes.TrimSpace(raw)
		if len(trimmed) == 0 || (trimmed[0] != '{' && trimmed[0] != '[') {
			return request{}, &rpcError{Code: codeInvalidRequest, Message: "Invalid Request", Data: "params must be an object or array"}
		}
	}
	return r, nil
}

func validRequestID(raw json.RawMessage) bool {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return false
	}
	switch value.(type) {
	case string, json.Number:
		return true
	default:
		return false
	}
}

func decodeParams(raw json.RawMessage, out any) error {
	return decodeObject(raw, out, false)
}

func decodeStrictParams(raw json.RawMessage, out any) error {
	return decodeObject(raw, out, true)
}

func decodeObject(raw json.RawMessage, out any, strict bool) error {
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	if err := validateUniqueJSON(raw); err != nil {
		return err
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return fmt.Errorf("params must be an object")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if strict {
		decoder.DisallowUnknownFields()
	}
	if err := decoder.Decode(out); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return fmt.Errorf("params must contain exactly one object")
		}
		return err
	}
	return nil
}

func validateUniqueJSON(body []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := validateJSONValue(decoder, 0); err != nil {
		return err
	}
	if token, err := decoder.Token(); err != io.EOF {
		if err != nil {
			return err
		}
		return fmt.Errorf("trailing token %v", token)
	}
	return nil
}

func validateJSONValue(decoder *json.Decoder, depth int) error {
	if depth > maxJSONDepth {
		return fmt.Errorf("JSON nesting exceeds %d levels", maxJSONDepth)
	}
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
			if err := validateJSONValue(decoder, depth+1); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim('}') {
			return fmt.Errorf("unterminated object")
		}
	case '[':
		for decoder.More() {
			if err := validateJSONValue(decoder, depth+1); err != nil {
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
