package tests

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/prof18/regesto/internal/config"
	"github.com/prof18/regesto/internal/mcp"
)

func mcpFixture(t *testing.T) *config.Config {
	t.Helper()
	root := filepath.Join(t.TempDir(), "kb")
	if err := os.CopyFS(root, os.DirFS(fixtureRoot(t))); err != nil {
		t.Fatalf("copy fixture: %v", err)
	}
	cfg, err := config.Load(filepath.Join(root, "config.toml"))
	if err != nil {
		t.Fatalf("load fixture config: %v", err)
	}
	return cfg
}

func rpcLine(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	return string(raw) + "\n"
}

func initializeLine(t *testing.T, id any) string {
	t.Helper()
	return rpcLine(t, map[string]any{
		"jsonrpc": "2.0", "id": id, "method": "initialize",
		"params": map[string]any{
			"protocolVersion": "2025-06-18", "capabilities": map[string]any{},
			"clientInfo": map[string]any{"name": "regesto-test", "title": "Regesto test", "version": "1"},
			"_meta":      map[string]any{"fixture": true},
		},
	})
}

func readyTranscript(t *testing.T) string {
	t.Helper()
	return initializeLine(t, 1) + rpcLine(t, map[string]any{
		"jsonrpc": "2.0", "method": "notifications/initialized", "params": map[string]any{},
	})
}

func decodeRPCLines(t *testing.T, raw string) []map[string]any {
	t.Helper()
	var messages []map[string]any
	scanner := bufio.NewScanner(strings.NewReader(raw))
	for scanner.Scan() {
		line := scanner.Bytes()
		if !json.Valid(line) {
			t.Fatalf("stdout line is not JSON-RPC JSON: %q", line)
		}
		var message map[string]any
		if err := json.Unmarshal(line, &message); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if message["jsonrpc"] != "2.0" {
			t.Fatalf("response missing jsonrpc 2.0: %s", line)
		}
		messages = append(messages, message)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan responses: %v", err)
	}
	return messages
}

func runMCP(t *testing.T, cfg *config.Config, transcript string) ([]map[string]any, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	if err := mcp.Serve(cfg, strings.NewReader(transcript), &stdout, &stderr); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	return decodeRPCLines(t, stdout.String()), stderr.String()
}

func rpcRequest(t *testing.T, id int, method string, params any) string {
	t.Helper()
	return rpcLine(t, map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
}

func responseResult(t *testing.T, message map[string]any) map[string]any {
	t.Helper()
	result, ok := message["result"].(map[string]any)
	if !ok {
		t.Fatalf("response has no object result: %#v", message)
	}
	return result
}

func TestMCPRecordedTranscript(t *testing.T) {
	cfg := mcpFixture(t)
	writeArgs := map[string]any{
		"source": "codex@testbox",
		"input": map[string]any{
			"id": "fact-mcp-transcript", "title": "MCP transcript writes through validation", "type": "fact", "scope": "global",
			"subject": "mcp", "relation": "write-path", "body": "The MCP adapter uses the shared writer.", "why": "Keep CLI and MCP behavior aligned.",
		},
	}
	transcript := readyTranscript(t) +
		rpcRequest(t, 2, "tools/list", map[string]any{}) +
		rpcRequest(t, 3, "tools/call", map[string]any{"name": "regesto_search", "arguments": map[string]any{"terms": []string{"imperative"}}}) +
		rpcRequest(t, 4, "resources/list", map[string]any{}) +
		rpcRequest(t, 5, "resources/read", map[string]any{"uri": "regesto://index"}) +
		rpcRequest(t, 6, "resources/read", map[string]any{"uri": "regesto://facts/dec-http-port-8080"}) +
		rpcRequest(t, 7, "tools/call", map[string]any{"name": "regesto_resolve_project", "arguments": map[string]any{"dir": cfg.KBRoot}}) +
		rpcRequest(t, 8, "tools/call", map[string]any{"name": "regesto_write_fact", "arguments": writeArgs}) +
		rpcRequest(t, 9, "tools/call", map[string]any{"name": "regesto_get_fact", "arguments": map[string]any{"id": "fact-mcp-transcript"}})

	responses, stderr := runMCP(t, cfg, transcript)
	if stderr != "" {
		t.Fatalf("valid transcript wrote diagnostics: %q", stderr)
	}
	if len(responses) != 9 {
		t.Fatalf("got %d responses, want 9 (initialized notification must be silent)", len(responses))
	}
	for i, message := range responses {
		if got := message["id"]; got != float64(i+1) {
			t.Fatalf("response %d id = %#v, want %d", i, got, i+1)
		}
		if _, failed := message["error"]; failed {
			t.Fatalf("response %d failed: %#v", i+1, message)
		}
	}

	initialize := responseResult(t, responses[0])
	if initialize["protocolVersion"] != "2025-06-18" {
		t.Fatalf("negotiated version = %#v", initialize["protocolVersion"])
	}
	capabilities := initialize["capabilities"].(map[string]any)
	if _, ok := capabilities["tools"]; !ok {
		t.Fatal("initialize did not declare tools")
	}
	if _, ok := capabilities["resources"]; !ok {
		t.Fatal("initialize did not declare resources")
	}

	tools := responseResult(t, responses[1])["tools"].([]any)
	var names []string
	for _, raw := range tools {
		tool := raw.(map[string]any)
		names = append(names, tool["name"].(string))
		if tool["inputSchema"].(map[string]any)["additionalProperties"] != false {
			t.Fatalf("tool %s schema permits undeclared arguments", tool["name"])
		}
	}
	wantNames := "regesto_search,regesto_get_fact,regesto_resolve_project,regesto_write_fact"
	if strings.Join(names, ",") != wantNames {
		t.Fatalf("tools = %v, want %s", names, wantNames)
	}

	searchResult := responseResult(t, responses[2])
	structured := searchResult["structuredContent"].(map[string]any)
	if got := len(structured["results"].([]any)); got != 1 {
		t.Fatalf("search returned %d results, want 1", got)
	}
	if searchResult["content"].([]any)[0].(map[string]any)["type"] != "text" {
		t.Fatal("search did not include backwards-compatible text content")
	}

	resources := responseResult(t, responses[3])["resources"].([]any)
	if len(resources) != 7 || resources[0].(map[string]any)["uri"] != "regesto://index" {
		t.Fatalf("resource listing = %#v", resources)
	}
	indexContents := responseResult(t, responses[4])["contents"].([]any)
	if !strings.Contains(indexContents[0].(map[string]any)["text"].(string), "## Controlled vocabulary") {
		t.Fatal("index resource is not generated from canonical facts")
	}
	factContents := responseResult(t, responses[5])["contents"].([]any)
	factText := factContents[0].(map[string]any)["text"].(string)
	if !strings.Contains(factText, "id: \"dec-http-port-8080\"") || !strings.Contains(factText, "listens on port 8080") {
		t.Fatalf("fact resource missing metadata/body:\n%s", factText)
	}

	writeResult := responseResult(t, responses[7])
	if writeResult["isError"] != false {
		t.Fatalf("write tool failed: %#v", writeResult)
	}
	written := filepath.Join(cfg.KBRoot, "knowledge", "facts", "global", "fact-mcp-transcript.md")
	if _, err := os.Stat(written); err != nil {
		t.Fatalf("shared writer did not publish fact: %v", err)
	}
	getResult := responseResult(t, responses[8])["structuredContent"].(map[string]any)
	if getResult["fact"].(map[string]any)["source"] != "codex@testbox" {
		t.Fatalf("fact provenance was not authority-owned: %#v", getResult)
	}
	resourceResponses, _ := runMCP(t, cfg, readyTranscript(t)+rpcRequest(t, 2, "resources/read", map[string]any{"uri": "regesto://facts/fact-mcp-transcript"}))
	writtenText := responseResult(t, resourceResponses[1])["contents"].([]any)[0].(map[string]any)["text"].(string)
	if !strings.Contains(writtenText, "topics: []") {
		t.Fatalf("empty topics did not render as an empty list:\n%s", writtenText)
	}
}

func TestMCPJSONRPCErrorsAndLifecycle(t *testing.T) {
	cfg := mcpFixture(t)
	beforeReady := rpcRequest(t, 1, "tools/list", map[string]any{})
	responses, _ := runMCP(t, cfg, beforeReady)
	if got := responses[0]["error"].(map[string]any)["code"]; got != float64(-32002) {
		t.Fatalf("pre-initialize error code = %#v", got)
	}

	// An initialize notification cannot move the server into operation, and a
	// request-method notification cannot perform a write.
	writeArgs := map[string]any{
		"source": "codex@testbox",
		"input":  map[string]any{"id": "fact-notification-write", "title": "Must stay absent", "type": "fact", "scope": "global", "subject": "mcp", "relation": "notification", "body": "ignored", "why": "no request id"},
	}
	transcript := rpcLine(t, map[string]any{
		"jsonrpc": "2.0", "method": "initialize",
		"params": map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{}, "clientInfo": map[string]any{"name": "notification", "version": "1"}},
	}) +
		rpcRequest(t, 3, "tools/list", map[string]any{}) +
		readyTranscript(t) +
		rpcLine(t, map[string]any{"jsonrpc": "2.0", "method": "tools/call", "params": map[string]any{"name": "regesto_write_fact", "arguments": writeArgs}}) +
		rpcRequest(t, 4, "notifications/initialized", map[string]any{}) +
		"{bad json}\n" +
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"regesto_search","arguments":{"terms":[],"terms":["x"]}}}` + "\n" +
		rpcRequest(t, 6, "resources/read", map[string]any{"uri": "regesto://facts/../../config.toml"})
	responses, _ = runMCP(t, cfg, transcript)
	if len(responses) != 6 {
		t.Fatalf("got %d responses, want 6: %#v", len(responses), responses)
	}
	if responses[0]["id"] != float64(3) || responses[0]["error"].(map[string]any)["code"] != float64(-32002) {
		t.Fatalf("initialize notification changed lifecycle: %#v", responses[0])
	}
	if responses[2]["id"] != float64(4) || responses[2]["error"].(map[string]any)["code"] != float64(-32600) {
		t.Fatalf("notification request was not rejected: %#v", responses[2])
	}
	if responses[3]["id"] != nil || responses[3]["error"].(map[string]any)["code"] != float64(-32700) {
		t.Fatalf("malformed JSON response = %#v", responses[3])
	}
	if responses[4]["error"].(map[string]any)["code"] != float64(-32700) {
		t.Fatalf("duplicate arguments response = %#v", responses[4])
	}
	if responses[5]["error"].(map[string]any)["message"] != "Resource not found" {
		t.Fatalf("traversal URI response = %#v", responses[5])
	}
	if _, err := os.Stat(filepath.Join(cfg.KBRoot, "knowledge", "facts", "global", "fact-notification-write.md")); !os.IsNotExist(err) {
		t.Fatalf("tools/call notification performed a write: %v", err)
	}
}

func TestMCPRejectsInvalidCursorAndExcessiveJSONDepth(t *testing.T) {
	cfg := mcpFixture(t)
	deep := strings.Repeat("[", 65) + "0" + strings.Repeat("]", 65)
	transcript := readyTranscript(t) +
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{"cursor":null}}` + "\n" +
		`{"jsonrpc":"2.0","id":3,"method":"ping","params":{"nested":` + deep + `}}` + "\n"
	responses, _ := runMCP(t, cfg, transcript)
	if got := responses[1]["error"].(map[string]any)["code"]; got != float64(-32602) {
		t.Fatalf("null cursor error = %#v", responses[1])
	}
	if got := responses[2]["error"].(map[string]any)["code"]; got != float64(-32700) {
		t.Fatalf("deep JSON error = %#v", responses[2])
	}
}

func TestMCPValidatesPingAndInitializedMetadata(t *testing.T) {
	cfg := mcpFixture(t)
	transcript := initializeLine(t, 1) +
		`{"jsonrpc":"2.0","method":"notifications/initialized","params":{"_meta":null}}` + "\n" +
		rpcRequest(t, 2, "tools/list", map[string]any{}) +
		`{"jsonrpc":"2.0","id":3,"method":"ping","params":[]}` + "\n"
	responses, _ := runMCP(t, cfg, transcript)
	if got := responses[1]["error"].(map[string]any)["code"]; got != float64(-32002) {
		t.Fatalf("invalid initialized metadata advanced lifecycle: %#v", responses[1])
	}
	if got := responses[2]["error"].(map[string]any)["code"]; got != float64(-32602) {
		t.Fatalf("invalid ping params response: %#v", responses[2])
	}
}

func TestMCPWriteRejectsForgedAuthorityFields(t *testing.T) {
	cfg := mcpFixture(t)
	call := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"regesto_write_fact","arguments":{"source":"codex@testbox","input":{"id":"fact-forged","title":"Forged","type":"fact","scope":"global","subject":"mcp","relation":"forgery","body":"bad","why":"test","created":"2000-01-01T00:00:00Z"}}}}`
	responses, _ := runMCP(t, cfg, readyTranscript(t)+call+"\n")
	if len(responses) != 2 {
		t.Fatalf("responses = %#v", responses)
	}
	err := responses[1]["error"].(map[string]any)
	if err["code"] != float64(-32602) || !strings.Contains(fmt.Sprint(err["data"]), "unknown field") {
		t.Fatalf("forged authority field response = %#v", responses[1])
	}
}

func TestMCPVersionNegotiationAndRequestIDs(t *testing.T) {
	cfg := mcpFixture(t)
	unsupportedOlder := strings.Replace(initializeLine(t, "request-7"), "2025-06-18", "2024-11-05", 1)
	responses, _ := runMCP(t, cfg, unsupportedOlder)
	if responses[0]["id"] != "request-7" {
		t.Fatalf("string request id not preserved: %#v", responses[0])
	}
	if got := responseResult(t, responses[0])["protocolVersion"]; got != "2025-06-18" {
		t.Fatalf("unsupported older version fallback = %#v", got)
	}

	unsupported := strings.Replace(initializeLine(t, 8), "2025-06-18", "2099-01-01", 1)
	responses, _ = runMCP(t, cfg, unsupported)
	if got := responseResult(t, responses[0])["protocolVersion"]; got != "2025-06-18" {
		t.Fatalf("unsupported version fallback = %#v", got)
	}

	responses, _ = runMCP(t, cfg, `{"jsonrpc":"2.0","id":null,"method":"initialize","params":{}}`+"\n")
	if responses[0]["id"] != nil || responses[0]["error"].(map[string]any)["code"] != float64(-32600) {
		t.Fatalf("null request id response = %#v", responses[0])
	}
}

func TestMCPRejectsInvalidUTF8(t *testing.T) {
	cfg := mcpFixture(t)
	message := append([]byte(`{"jsonrpc":"2.0","id":1,"method":"ping","params":{"bad":"`), 0xff)
	message = append(message, []byte(`"}}\n`)...)
	responses, _ := runMCP(t, cfg, string(message))
	if got := responses[0]["error"].(map[string]any)["code"]; got != float64(-32700) {
		t.Fatalf("invalid UTF-8 error code = %#v", got)
	}
}

func TestMCPAllowsProtocolExtensionsButKeepsToolSchemasStrict(t *testing.T) {
	cfg := mcpFixture(t)
	initialize := strings.TrimSuffix(initializeLine(t, 1), "\n")
	initialize = strings.TrimSuffix(initialize, "}") + `,"futureExtension":{"enabled":true}}` + "\n"
	transcript := initialize +
		rpcLine(t, map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized", "params": map[string]any{"futureExtension": true}}) +
		rpcRequest(t, 2, "tools/list", map[string]any{"futureExtension": true}) +
		rpcRequest(t, 3, "tools/call", map[string]any{"name": "regesto_search", "futureExtension": true, "arguments": map[string]any{"unknownToolArgument": true}})
	responses, _ := runMCP(t, cfg, transcript)
	if len(responses) != 3 || responses[0]["error"] != nil || responses[1]["error"] != nil {
		t.Fatalf("protocol extension rejected: %#v", responses)
	}
	if got := responses[2]["error"].(map[string]any)["code"]; got != float64(-32602) {
		t.Fatalf("undeclared tool argument error code = %#v", got)
	}
}

func TestMCPResourceErrorsAreClassifiedAndSanitized(t *testing.T) {
	cfg := mcpFixture(t)
	responses, _ := runMCP(t, cfg, readyTranscript(t)+rpcRequest(t, 2, "resources/read", map[string]any{}))
	if got := responses[1]["error"].(map[string]any)["code"]; got != float64(-32602) {
		t.Fatalf("missing URI error code = %#v", got)
	}

	broken := filepath.Join(cfg.KBRoot, "knowledge", "facts", "global", "fact-broken.md")
	if err := os.WriteFile(broken, []byte("not frontmatter\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	responses, stderr := runMCP(t, cfg, readyTranscript(t)+rpcRequest(t, 2, "resources/read", map[string]any{"uri": "regesto://facts/dec-http-port-8080"}))
	err := responses[1]["error"].(map[string]any)
	if err["code"] != float64(-32603) || err["message"] != "Internal error" {
		t.Fatalf("backend resource error = %#v", responses[1])
	}
	if _, leaked := err["data"]; leaked {
		t.Fatalf("backend error leaked protocol data: %#v", err)
	}
	if !strings.Contains(stderr, broken) {
		t.Fatalf("diagnostic did not retain local path: %q", stderr)
	}
}

func TestMCPIndexResourceRejectsAmbiguousFactIDs(t *testing.T) {
	cfg := mcpFixture(t)
	source := filepath.Join(cfg.KBRoot, "knowledge", "facts", "global", "pref-git-commit-style.md")
	raw, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	duplicate := filepath.Join(cfg.KBRoot, "knowledge", "facts", "global", "duplicate-id.md")
	if err := os.WriteFile(duplicate, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	responses, stderr := runMCP(t, cfg, readyTranscript(t)+rpcRequest(t, 2, "resources/read", map[string]any{"uri": "regesto://index"}))
	rpcErr := responses[1]["error"].(map[string]any)
	if rpcErr["code"] != float64(-32603) || rpcErr["message"] != "Internal error" {
		t.Fatalf("ambiguous index response = %#v", responses[1])
	}
	if !strings.Contains(stderr, "occurs at both") {
		t.Fatalf("ambiguous index diagnostic = %q", stderr)
	}
	responses, _ = runMCP(t, cfg, readyTranscript(t)+rpcRequest(t, 2, "tools/call", map[string]any{"name": "regesto_search", "arguments": map[string]any{}}))
	searchResult := responseResult(t, responses[1])
	if searchResult["isError"] != true || !strings.Contains(searchResult["content"].([]any)[0].(map[string]any)["text"].(string), "occurs at both") {
		t.Fatalf("ambiguous search response = %#v", responses[1])
	}
}

func TestMCPResourcesRejectSymlinksAndUnsafeIDs(t *testing.T) {
	t.Run("symlink", func(t *testing.T) {
		cfg := mcpFixture(t)
		external := filepath.Join(t.TempDir(), "external.md")
		text := "---\nschema_version: 1\nid: fact-external\ntitle: External\ntype: fact\nscope: global\nsubject: mcp\nrelation: symlink\ntopics: [mcp]\nstatus: active\nsource: human\ncreated: 2026-08-29T00:00:00Z\nmodified: 2026-08-29T00:00:00Z\n---\n\nMust not be exposed.\n"
		if err := os.WriteFile(external, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		link := filepath.Join(cfg.KBRoot, "knowledge", "facts", "global", "fact-external.md")
		if err := os.Symlink(external, link); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
		responses, _ := runMCP(t, cfg, readyTranscript(t)+rpcRequest(t, 2, "resources/list", map[string]any{}))
		if got := responses[1]["error"].(map[string]any)["code"]; got != float64(-32603) {
			t.Fatalf("symlink resource response = %#v", responses[1])
		}
	})

	t.Run("unsafe id", func(t *testing.T) {
		cfg := mcpFixture(t)
		path := filepath.Join(cfg.KBRoot, "knowledge", "facts", "global", "unsafe-id.md")
		text := "---\nschema_version: 1\nid: bad?id\ntitle: Unsafe ID\ntype: fact\nscope: global\nsubject: mcp\nrelation: uri\ntopics: [mcp]\nstatus: active\nsource: human\ncreated: 2026-08-29T00:00:00Z\nmodified: 2026-08-29T00:00:00Z\n---\n\nMust not be advertised.\n"
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		responses, stderr := runMCP(t, cfg, readyTranscript(t)+rpcRequest(t, 2, "resources/list", map[string]any{}))
		if got := responses[1]["error"].(map[string]any)["code"]; got != float64(-32603) {
			t.Fatalf("unsafe ID response = %#v", responses[1])
		}
		if !strings.Contains(stderr, "not a canonical resource id") {
			t.Fatalf("unsafe ID diagnostic = %q", stderr)
		}
	})
}

func TestMCPBoundsResponseLines(t *testing.T) {
	cfg := mcpFixture(t)
	large := filepath.Join(cfg.KBRoot, "knowledge", "facts", "global", "fact-large-resource.md")
	body := strings.Repeat("x", 4<<20)
	text := "---\nschema_version: 1\nid: fact-large-resource\ntitle: Large local resource\ntype: fact\nscope: global\nsubject: mcp\nrelation: response-limit\ntopics: [mcp]\nstatus: active\nsource: human\ncreated: 2026-08-29T00:00:00Z\nmodified: 2026-08-29T00:00:00Z\n---\n\n" + body + "\n"
	if err := os.WriteFile(large, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	responses, stderr := runMCP(t, cfg, readyTranscript(t)+rpcRequest(t, 2, "resources/read", map[string]any{"uri": "regesto://facts/fact-large-resource"}))
	err := responses[1]["error"].(map[string]any)
	if err["code"] != float64(-32603) || err["message"] != "Response exceeds server limit" {
		t.Fatalf("oversized response = %#v", responses[1])
	}
	if !strings.Contains(stderr, "exceeds") {
		t.Fatalf("oversized response diagnostic = %q", stderr)
	}
}

func TestMCPWriteCannotPublishFactBeyondReadLimit(t *testing.T) {
	cfg := mcpFixture(t)
	body := strings.Repeat("x", (4<<20)-60_000)
	topics := make([]string, 12_000)
	for i := range topics {
		topics[i] = "x"
	}
	call := rpcRequest(t, 2, "tools/call", map[string]any{
		"name": "regesto_write_fact",
		"arguments": map[string]any{
			"source": "codex@testbox",
			"input":  map[string]any{"id": "fact-too-large", "title": "Too large", "type": "fact", "scope": "global", "subject": "mcp", "relation": "limit", "topics": topics, "body": body, "why": "exercise exact rendered size"},
		},
	})
	if len(call) >= 4<<20 {
		t.Fatalf("test request itself exceeds MCP framing limit: %d", len(call))
	}
	responses, _ := runMCP(t, cfg, readyTranscript(t)+call)
	result := responseResult(t, responses[1])
	if result["isError"] != true || !strings.Contains(result["content"].([]any)[0].(map[string]any)["text"].(string), "rendered fact") {
		t.Fatalf("oversized write result = %#v", result)
	}
	if _, err := os.Stat(filepath.Join(cfg.KBRoot, "knowledge", "facts", "global", "fact-too-large.md")); !os.IsNotExist(err) {
		t.Fatalf("oversized MCP fact reached disk: %v", err)
	}
}

func TestMCPToolExecutionFailureUsesToolResult(t *testing.T) {
	cfg := mcpFixture(t)
	call := rpcRequest(t, 2, "tools/call", map[string]any{
		"name": "regesto_write_fact",
		"arguments": map[string]any{
			"source": "codex@testbox",
			"input":  map[string]any{"id": "not-a-valid-id", "title": "Invalid", "type": "fact", "scope": "global", "subject": "mcp", "relation": "errors", "body": "invalid", "why": "exercise errors"},
		},
	})
	responses, _ := runMCP(t, cfg, readyTranscript(t)+call)
	if _, hasProtocolError := responses[1]["error"]; hasProtocolError {
		t.Fatalf("tool execution failure became JSON-RPC failure: %#v", responses[1])
	}
	result := responseResult(t, responses[1])
	if result["isError"] != true || !strings.Contains(result["content"].([]any)[0].(map[string]any)["text"].(string), "kebab slug") {
		t.Fatalf("tool execution error result = %#v", result)
	}
}

func TestMCPCommandStdoutIsOnlyJSONRPC(t *testing.T) {
	cfg := mcpFixture(t)
	cmd := exec.Command("go", "run", "./cmd/regesto", "--config", cfg.Path, "mcp")
	_, file, _, _ := runtime.Caller(0)
	cmd.Dir = filepath.Dir(filepath.Dir(file))
	cmd.Stdin = strings.NewReader(readyTranscript(t) + rpcRequest(t, 2, "tools/list", map[string]any{}))
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("regesto mcp: %v\nstderr:\n%s", err, stderr.String())
	}
	responses := decodeRPCLines(t, stdout.String())
	if len(responses) != 2 {
		t.Fatalf("CLI emitted %d responses, want 2; stdout:\n%s", len(responses), stdout.String())
	}
}

func TestMCPRejectsOversizedJSONRPCMessage(t *testing.T) {
	cfg := mcpFixture(t)
	oversized := strings.Repeat("x", 4<<20+1) + "\n"
	responses, stderr := runMCP(t, cfg, oversized)
	if len(responses) != 1 || responses[0]["error"].(map[string]any)["code"] != float64(-32700) {
		t.Fatalf("oversized response = %#v", responses)
	}
	if !strings.Contains(stderr, "input framing") {
		t.Fatalf("oversized diagnostic = %q", stderr)
	}
}
