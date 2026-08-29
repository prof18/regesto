// Package mcp exposes Regesto's local operations over newline-delimited MCP
// JSON-RPC on stdin/stdout. It deliberately implements no network transport.
package mcp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/prof18/regesto/internal/config"
	"github.com/prof18/regesto/internal/facts"
	"github.com/prof18/regesto/internal/index"
	"github.com/prof18/regesto/internal/project"
	"github.com/prof18/regesto/internal/search"
	"github.com/prof18/regesto/internal/version"
	writeop "github.com/prof18/regesto/internal/write"
)

const (
	protocolVersion   = "2025-06-18"
	maxMessageBytes   = 4 << 20
	maxResponseBytes  = 4 << 20
	maxFactBytes      = 4 << 20
	maxKnowledgeBytes = 8 << 20
	maxFactCount      = 10_000
)

var (
	errFactNotFound  = errors.New("fact not found")
	errFactAmbiguous = errors.New("fact id is ambiguous")
	errResponseLimit = errors.New("MCP response source exceeds server limit")
)

type server struct {
	cfg         *config.Config
	diagnostic  io.Writer
	initialized bool
	ready       bool
}

// Serve runs one stdio MCP session until input reaches EOF. Every output line
// is a complete JSON-RPC message; diagnostics, when requested, use diagnostic.
func Serve(cfg *config.Config, input io.Reader, output, diagnostic io.Writer) error {
	s := &server{cfg: cfg, diagnostic: diagnostic}
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 64*1024), maxMessageBytes)
	for scanner.Scan() {
		body := bytes.TrimSpace(scanner.Bytes())
		req, protocolErr := decodeRequest(body)
		if protocolErr != nil {
			if err := s.writeResponse(output, response{JSONRPC: jsonRPCVersion, ID: json.RawMessage("null"), Error: protocolErr}); err != nil {
				return err
			}
			continue
		}
		result, callErr := s.handle(req)
		if !req.HasID {
			continue
		}
		resp := response{JSONRPC: jsonRPCVersion, ID: req.ID, Result: result, Error: callErr}
		if callErr != nil {
			resp.Result = nil
		}
		if err := s.writeResponse(output, resp); err != nil {
			return err
		}
	}
	if err := scanner.Err(); err != nil {
		if diagnostic != nil {
			fmt.Fprintf(diagnostic, "regesto mcp: input framing: %v\n", err)
		}
		return s.writeResponse(output, response{JSONRPC: jsonRPCVersion, ID: json.RawMessage("null"), Error: &rpcError{Code: codeParseError, Message: "Parse error", Data: "message exceeds framing limit or input failed"}})
	}
	return nil
}

func (s *server) writeResponse(output io.Writer, resp response) error {
	raw, err := json.Marshal(resp)
	if err != nil {
		s.diagnose(fmt.Errorf("encode response: %w", err))
		raw, _ = json.Marshal(response{JSONRPC: jsonRPCVersion, ID: resp.ID, Error: &rpcError{Code: codeInternalError, Message: "Internal error"}})
	}
	if len(raw)+1 > maxResponseBytes {
		s.diagnose(fmt.Errorf("response for id %s exceeds %d bytes", resp.ID, maxResponseBytes))
		raw, _ = json.Marshal(response{JSONRPC: jsonRPCVersion, ID: resp.ID, Error: &rpcError{Code: codeInternalError, Message: "Response exceeds server limit"}})
	}
	raw = append(raw, '\n')
	written, err := output.Write(raw)
	if err == nil && written != len(raw) {
		return io.ErrShortWrite
	}
	return err
}

func (s *server) diagnose(err error) {
	if s.diagnostic != nil {
		fmt.Fprintf(s.diagnostic, "regesto mcp: %v\n", err)
	}
}

func (s *server) internalError(err error) *rpcError {
	s.diagnose(err)
	if errors.Is(err, errResponseLimit) {
		return &rpcError{Code: codeInternalError, Message: "Response exceeds server limit"}
	}
	return &rpcError{Code: codeInternalError, Message: "Internal error"}
}

func (s *server) handle(req request) (any, *rpcError) {
	if !req.HasID {
		if req.Method == "notifications/initialized" && s.initialized && metadataParams(req.Params) == nil {
			s.ready = true
		}
		// MCP methods with effects are requests, not notifications. Unknown
		// notifications are ignored as JSON-RPC requires, and cannot trigger a
		// Regesto operation merely by borrowing a request method name.
		return nil, nil
	}
	if req.Method == "ping" {
		if err := metadataParams(req.Params); err != nil {
			return nil, invalidParams(err)
		}
		return struct{}{}, nil
	}
	if req.Method == "initialize" {
		return s.initialize(req.Params)
	}
	if strings.HasPrefix(req.Method, "notifications/") {
		return nil, &rpcError{Code: codeInvalidRequest, Message: "Notifications must not have an id"}
	}
	if !s.ready {
		return nil, &rpcError{Code: codeNotInitialized, Message: "Server not initialized"}
	}

	switch req.Method {
	case "tools/list":
		if err := emptyOrCursorParams(req.Params); err != nil {
			return nil, invalidParams(err)
		}
		return map[string]any{"tools": toolDefinitions()}, nil
	case "tools/call":
		return s.callTool(req.Params)
	case "resources/list":
		if err := emptyOrCursorParams(req.Params); err != nil {
			return nil, invalidParams(err)
		}
		return s.listResources()
	case "resources/read":
		return s.readResource(req.Params)
	default:
		return nil, &rpcError{Code: codeMethodNotFound, Message: "Method not found"}
	}
}

func (s *server) initialize(raw json.RawMessage) (any, *rpcError) {
	if s.initialized {
		return nil, &rpcError{Code: codeInvalidRequest, Message: "Already initialized"}
	}
	var params struct {
		ProtocolVersion string          `json:"protocolVersion"`
		Capabilities    json.RawMessage `json:"capabilities"`
		ClientInfo      json.RawMessage `json:"clientInfo"`
		Meta            json.RawMessage `json:"_meta,omitempty"`
	}
	if err := decodeParams(raw, &params); err != nil {
		return nil, invalidParams(err)
	}
	if params.ProtocolVersion == "" {
		return nil, invalidParams(fmt.Errorf("protocolVersion is required"))
	}
	if !isJSONObject(params.Capabilities) {
		return nil, invalidParams(fmt.Errorf("capabilities must be an object"))
	}
	if len(params.Meta) != 0 && !isJSONObject(params.Meta) {
		return nil, invalidParams(fmt.Errorf("_meta must be an object"))
	}
	var clientInfo struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	if !isJSONObject(params.ClientInfo) || json.Unmarshal(params.ClientInfo, &clientInfo) != nil || clientInfo.Name == "" || clientInfo.Version == "" {
		return nil, invalidParams(fmt.Errorf("clientInfo must contain name and version"))
	}
	negotiated := protocolVersion
	s.initialized = true
	return map[string]any{
		"protocolVersion": negotiated,
		"capabilities": map[string]any{
			"resources": struct{}{},
			"tools":     struct{}{},
		},
		"serverInfo": map[string]string{
			"name":    "regesto",
			"title":   "Regesto",
			"version": version.Current(),
		},
		"instructions": "Search and read recorded claims before deciding; write only explicit, validated claims with provenance.",
	}, nil
}

func isJSONObject(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) > 0 && trimmed[0] == '{' && json.Valid(trimmed)
}

func emptyOrCursorParams(raw json.RawMessage) error {
	var params struct {
		Cursor json.RawMessage `json:"cursor,omitempty"`
		Meta   json.RawMessage `json:"_meta,omitempty"`
	}
	if err := decodeParams(raw, &params); err != nil {
		return err
	}
	if len(params.Meta) != 0 && !isJSONObject(params.Meta) {
		return fmt.Errorf("_meta must be an object")
	}
	if len(params.Cursor) == 0 {
		return nil
	}
	var cursor string
	if bytes.Equal(bytes.TrimSpace(params.Cursor), []byte("null")) || json.Unmarshal(params.Cursor, &cursor) != nil {
		return fmt.Errorf("cursor must be a string")
	}
	if cursor != "" {
		return fmt.Errorf("cursor is not valid because this list is not paginated")
	}
	return nil
}

func metadataParams(raw json.RawMessage) error {
	var params struct {
		Meta json.RawMessage `json:"_meta,omitempty"`
	}
	if err := decodeParams(raw, &params); err != nil {
		return err
	}
	if len(params.Meta) != 0 && !isJSONObject(params.Meta) {
		return fmt.Errorf("_meta must be an object")
	}
	return nil
}

func invalidParams(err error) *rpcError {
	return &rpcError{Code: codeInvalidParams, Message: "Invalid params", Data: err.Error()}
}

type toolDefinition struct {
	Name        string         `json:"name"`
	Title       string         `json:"title"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

func toolDefinitions() []toolDefinition {
	stringProperty := func(description string) map[string]any {
		return map[string]any{"type": "string", "description": description}
	}
	return []toolDefinition{
		{
			Name: "regesto_search", Title: "Search Regesto", Description: "Search canonical facts by metadata and free text.",
			InputSchema: objectSchema(map[string]any{
				"subject": stringProperty("Exact subject match."), "relation": stringProperty("Exact relation match."),
				"scope":   stringProperty("global, project:<name>, or a bare project name."),
				"terms":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"history": map[string]any{"type": "boolean", "description": "Include superseded facts."},
			}, nil),
		},
		{
			Name: "regesto_get_fact", Title: "Read a Regesto fact", Description: "Read one canonical fact by exact ID.",
			InputSchema: objectSchema(map[string]any{"id": stringProperty("Exact fact ID.")}, []string{"id"}),
		},
		{
			Name: "regesto_resolve_project", Title: "Resolve a Regesto project", Description: "Resolve a working directory to its canonical project and scope.",
			InputSchema: objectSchema(map[string]any{"dir": stringProperty("Working directory; defaults to the server process directory.")}, nil),
		},
		{
			Name: "regesto_write_fact", Title: "Write a Regesto fact", Description: "Validate and atomically create one fact with explicit provenance.",
			InputSchema: objectSchema(map[string]any{
				"source": stringProperty("Required provenance: human or <integration>@<machine>."),
				"dir":    stringProperty("Directory used only when input.scope is project."),
				"input": map[string]any{
					"type": "object", "additionalProperties": false,
					"properties": map[string]any{
						"id": stringProperty("Fact ID."), "title": stringProperty("Short title."), "type": stringProperty("decision, preference, fact, or pattern."),
						"scope": stringProperty("global, project:<name>, or project with dir."), "subject": stringProperty("Controlled-vocabulary subject."),
						"relation": stringProperty("Controlled-vocabulary relation."), "topics": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
						"status": stringProperty("active or proposed."), "supersedes": stringProperty("Optional prior fact ID."),
						"body": stringProperty("Claim body."), "why": stringProperty("Reason for recording the claim."),
					},
					"required": []string{"id", "title", "type", "scope", "subject", "relation", "body", "why"},
				},
			}, []string{"source", "input"}),
		},
	}
}

func objectSchema(properties map[string]any, required []string) map[string]any {
	schema := map[string]any{"type": "object", "properties": properties, "additionalProperties": false}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

type callToolParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
	Meta      json.RawMessage `json:"_meta,omitempty"`
}

type textContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type toolResult struct {
	Content           []textContent `json:"content"`
	StructuredContent any           `json:"structuredContent,omitempty"`
	IsError           bool          `json:"isError"`
}

func (s *server) callTool(raw json.RawMessage) (any, *rpcError) {
	var call callToolParams
	if err := decodeParams(raw, &call); err != nil {
		return nil, invalidParams(err)
	}
	if call.Name == "" {
		return nil, invalidParams(fmt.Errorf("name is required"))
	}
	if len(call.Meta) != 0 && !isJSONObject(call.Meta) {
		return nil, invalidParams(fmt.Errorf("_meta must be an object"))
	}
	if len(call.Arguments) == 0 {
		call.Arguments = json.RawMessage(`{}`)
	}

	var payload any
	var err error
	switch call.Name {
	case "regesto_search":
		payload, err = s.search(call.Arguments)
	case "regesto_get_fact":
		payload, err = s.getFact(call.Arguments)
	case "regesto_resolve_project":
		payload, err = s.resolveProject(call.Arguments)
	case "regesto_write_fact":
		payload, err = s.writeFact(call.Arguments)
	default:
		return nil, invalidParams(fmt.Errorf("unknown tool %q", call.Name))
	}
	if err != nil {
		var argumentsErr *argumentError
		if errors.As(err, &argumentsErr) {
			return nil, invalidParams(argumentsErr.err)
		}
		s.diagnose(err)
		message := strings.ReplaceAll(err.Error(), s.cfg.KBRoot, "<kb>")
		return toolResult{Content: []textContent{{Type: "text", Text: message}}, IsError: true}, nil
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, &rpcError{Code: codeInternalError, Message: "Internal error"}
	}
	return toolResult{
		Content: []textContent{{Type: "text", Text: string(encoded)}}, StructuredContent: payload, IsError: false,
	}, nil
}

type argumentError struct{ err error }

func (e *argumentError) Error() string { return "invalid arguments: " + e.err.Error() }
func (e *argumentError) Unwrap() error { return e.err }

func invalidArguments(err error) error { return &argumentError{err: err} }

type factJSON struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Type        string   `json:"type"`
	Scope       string   `json:"scope"`
	Subject     string   `json:"subject"`
	Relation    string   `json:"relation"`
	Topics      []string `json:"topics"`
	Status      string   `json:"status"`
	Supersedes  string   `json:"supersedes"`
	Source      string   `json:"source"`
	Created     string   `json:"created"`
	Modified    string   `json:"modified"`
	ReviewAfter string   `json:"review_after"`
	Body        string   `json:"body"`
	Path        string   `json:"path"`
}

func factToJSON(f facts.Fact) factJSON {
	return factJSON{f.ID, f.Title, f.Type, f.Scope, f.Subject, f.Relation, append([]string{}, f.Topics...), f.Status, f.Supersedes, f.Source, f.Created, f.Modified, f.ReviewAfter, f.Body, f.RelPath}
}

func (s *server) search(raw json.RawMessage) (any, error) {
	var args struct {
		Subject  string   `json:"subject,omitempty"`
		Relation string   `json:"relation,omitempty"`
		Scope    string   `json:"scope,omitempty"`
		Terms    []string `json:"terms,omitempty"`
		History  bool     `json:"history,omitempty"`
	}
	if err := decodeStrictParams(raw, &args); err != nil {
		return nil, invalidArguments(err)
	}
	all, _, err := s.loadFacts()
	if err != nil {
		return nil, err
	}
	if err := requireUniqueFactIDs(all); err != nil {
		return nil, err
	}
	found := search.Run(all, search.Query{Subject: args.Subject, Relation: args.Relation, Scope: args.Scope, Terms: args.Terms, History: args.History})
	results := make([]factJSON, 0, len(found))
	for _, f := range found {
		results = append(results, factToJSON(f))
	}
	return struct {
		SchemaVersion int        `json:"schema_version"`
		Results       []factJSON `json:"results"`
	}{SchemaVersion: 1, Results: results}, nil
}

func (s *server) getFact(raw json.RawMessage) (any, error) {
	var args struct {
		ID string `json:"id"`
	}
	if err := decodeStrictParams(raw, &args); err != nil {
		return nil, invalidArguments(err)
	}
	if args.ID == "" {
		return nil, invalidArguments(fmt.Errorf("id is required"))
	}
	if !writeop.ValidID(args.ID) {
		return nil, invalidArguments(fmt.Errorf("id is not a canonical fact id"))
	}
	f, err := s.factByID(args.ID)
	if err != nil {
		return nil, err
	}
	return struct {
		SchemaVersion int      `json:"schema_version"`
		Fact          factJSON `json:"fact"`
	}{SchemaVersion: 1, Fact: factToJSON(f)}, nil
}

func (s *server) resolveProject(raw json.RawMessage) (any, error) {
	var args struct {
		Dir string `json:"dir,omitempty"`
	}
	if err := decodeStrictParams(raw, &args); err != nil {
		return nil, invalidArguments(err)
	}
	dir := args.Dir
	if dir == "" {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			return nil, err
		}
	}
	r := project.Resolve(s.cfg, dir)
	return struct {
		SchemaVersion int    `json:"schema_version"`
		Project       string `json:"project"`
		Scope         string `json:"scope"`
		Resolution    struct {
			How    string `json:"how"`
			Mapped bool   `json:"mapped"`
		} `json:"resolution"`
	}{SchemaVersion: 1, Project: r.Name, Scope: "project:" + r.Name, Resolution: struct {
		How    string `json:"how"`
		Mapped bool   `json:"mapped"`
	}{How: r.How, Mapped: r.Mapped}}, nil
}

func (s *server) writeFact(raw json.RawMessage) (any, error) {
	var args struct {
		Source string          `json:"source"`
		Dir    string          `json:"dir,omitempty"`
		Input  json.RawMessage `json:"input"`
	}
	if err := decodeStrictParams(raw, &args); err != nil {
		return nil, invalidArguments(err)
	}
	if strings.TrimSpace(args.Source) == "" {
		return nil, invalidArguments(fmt.Errorf("source is required"))
	}
	if !isJSONObject(args.Input) {
		return nil, invalidArguments(fmt.Errorf("input must be an object"))
	}
	var input writeop.Input
	if err := decodeStrictParams(args.Input, &input); err != nil {
		return nil, invalidArguments(fmt.Errorf("input: %w", err))
	}
	_, _, err := s.loadFacts()
	if err != nil {
		return nil, err
	}
	if args.Dir != "" {
		if input.Scope != "project" {
			return nil, invalidArguments(fmt.Errorf("dir requires input scope %q", "project"))
		}
		resolved := project.Resolve(s.cfg, args.Dir)
		input.Scope = "project:" + resolved.Name
	} else if input.Scope == "project" {
		return nil, invalidArguments(fmt.Errorf("input scope %q requires dir", "project"))
	}
	return writeop.Create(s.cfg, input, writeop.Authority{
		Source: args.Source, MaxFactBytes: maxFactBytes,
		MaxStoreBytes: maxKnowledgeBytes, MaxFactCount: maxFactCount,
	})
}

type resource struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
	Size        int    `json:"size,omitempty"`
}

type resourceContent struct {
	URI      string `json:"uri"`
	MimeType string `json:"mimeType"`
	Text     string `json:"text"`
}

func (s *server) listResources() (any, *rpcError) {
	all, _, err := s.loadFacts()
	if err != nil {
		return nil, s.internalError(err)
	}
	if err := requireUniqueFactIDs(all); err != nil {
		return nil, s.internalError(err)
	}
	indexText := index.Build(all).IndexMD
	resources := []resource{{URI: "regesto://index", Name: "index", Title: "Regesto index", Description: "Generated index and controlled vocabulary for all canonical facts.", MimeType: "text/markdown", Size: len([]byte(indexText))}}
	for _, f := range all {
		resources = append(resources, resource{URI: "regesto://facts/" + f.ID, Name: f.ID, Title: f.Title, Description: "Canonical Regesto fact.", MimeType: "text/markdown"})
	}
	return map[string]any{"resources": resources}, nil
}

func (s *server) readResource(raw json.RawMessage) (any, *rpcError) {
	var args struct {
		URI  string          `json:"uri"`
		Meta json.RawMessage `json:"_meta,omitempty"`
	}
	if err := decodeParams(raw, &args); err != nil {
		return nil, invalidParams(err)
	}
	if args.URI == "" {
		return nil, invalidParams(fmt.Errorf("uri is required"))
	}
	if len(args.Meta) != 0 && !isJSONObject(args.Meta) {
		return nil, invalidParams(fmt.Errorf("_meta must be an object"))
	}
	if args.URI == "regesto://index" {
		all, _, err := s.loadFacts()
		if err != nil {
			return nil, s.internalError(err)
		}
		if err := requireUniqueFactIDs(all); err != nil {
			return nil, s.internalError(err)
		}
		return map[string]any{"contents": []resourceContent{{URI: args.URI, MimeType: "text/markdown", Text: index.Build(all).IndexMD}}}, nil
	}
	id, ok := strings.CutPrefix(args.URI, "regesto://facts/")
	if !ok || !writeop.ValidID(id) {
		return nil, &rpcError{Code: codeNotInitialized, Message: "Resource not found", Data: args.URI}
	}
	f, err := s.factByID(id)
	if err != nil {
		if errors.Is(err, errFactNotFound) {
			return nil, &rpcError{Code: codeNotInitialized, Message: "Resource not found", Data: args.URI}
		}
		return nil, s.internalError(err)
	}
	return map[string]any{"contents": []resourceContent{{URI: args.URI, MimeType: "text/markdown", Text: renderFact(f)}}}, nil
}

func (s *server) factByID(id string) (facts.Fact, error) {
	all, _, err := s.loadFacts()
	if err != nil {
		return facts.Fact{}, err
	}
	if err := requireUniqueFactIDs(all); err != nil {
		return facts.Fact{}, err
	}
	var found *facts.Fact
	for _, f := range all {
		if f.ID == id {
			if found != nil {
				return facts.Fact{}, fmt.Errorf("%w: %q", errFactAmbiguous, id)
			}
			copy := f
			found = &copy
		}
	}
	if found != nil {
		return *found, nil
	}
	return facts.Fact{}, fmt.Errorf("%w: %q", errFactNotFound, id)
}

// loadFacts applies an MCP-specific resource bound while retaining the
// canonical parser, conflict-copy rule, relative paths, and deterministic
// ordering used by facts.LoadAll. Files are read through an opened descriptor
// and a limit, so concurrent growth or a symlink swap cannot evade the cap.
func (s *server) loadFacts() ([]facts.Fact, int64, error) {
	factsDir := filepath.Join(s.cfg.KBRoot, "knowledge", "facts")
	root, err := os.OpenRoot(s.cfg.KBRoot)
	if err != nil {
		return nil, 0, err
	}
	defer root.Close()
	var total int64
	var all []facts.Fact
	err = filepath.WalkDir(factsDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") || facts.IsConflict(entry.Name()) {
			return nil
		}
		rel, err := filepath.Rel(s.cfg.KBRoot, path)
		if err != nil {
			return err
		}
		info, err := root.Lstat(rel)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%s is a symlink or non-regular fact file", path)
		}
		file, err := root.Open(rel)
		if err != nil {
			return err
		}
		opened, err := file.Stat()
		if err != nil || !os.SameFile(info, opened) {
			file.Close()
			return fmt.Errorf("fact file %s changed while opening", path)
		}
		data, err := io.ReadAll(io.LimitReader(file, maxFactBytes+1))
		closeErr := file.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		if int64(len(data)) > maxFactBytes {
			return fmt.Errorf("%w: fact %s exceeds %d bytes", errResponseLimit, path, maxFactBytes)
		}
		if int64(len(data)) > maxKnowledgeBytes-total {
			return fmt.Errorf("%w: canonical facts exceed %d bytes", errResponseLimit, maxKnowledgeBytes)
		}
		total += int64(len(data))
		fact, err := facts.Parse(data, path)
		if err != nil {
			return err
		}
		fact.RelPath = filepath.ToSlash(rel)
		all = append(all, fact)
		if len(all) > maxFactCount {
			return fmt.Errorf("%w: canonical facts exceed %d entries", errResponseLimit, maxFactCount)
		}
		return nil
	})
	if err != nil {
		return nil, total, err
	}
	sort.Slice(all, func(i, j int) bool { return all[i].RelPath < all[j].RelPath })
	return all, total, nil
}

func requireUniqueFactIDs(all []facts.Fact) error {
	seen := make(map[string]string, len(all))
	for _, f := range all {
		if !writeop.ValidID(f.ID) {
			return fmt.Errorf("fact id %q at %s is not a canonical resource id", f.ID, f.RelPath)
		}
		if previous, ok := seen[f.ID]; ok {
			return fmt.Errorf("fact id %q occurs at both %s and %s", f.ID, previous, f.RelPath)
		}
		seen[f.ID] = f.RelPath
	}
	return nil
}

func renderFact(f facts.Fact) string {
	var b strings.Builder
	b.WriteString("---\n")
	writeField := func(key, value string) { fmt.Fprintf(&b, "%s: %s\n", key, strconv.Quote(value)) }
	writeField("schema_version", f.SchemaVersion)
	writeField("id", f.ID)
	writeField("title", f.Title)
	writeField("type", f.Type)
	writeField("scope", f.Scope)
	writeField("subject", f.Subject)
	writeField("relation", f.Relation)
	topicValues := append([]string{}, f.Topics...)
	topics, _ := json.Marshal(topicValues)
	fmt.Fprintf(&b, "topics: %s\n", topics)
	writeField("status", f.Status)
	if f.Supersedes != "" {
		writeField("supersedes", f.Supersedes)
	}
	writeField("source", f.Source)
	writeField("created", f.Created)
	writeField("modified", f.Modified)
	if f.ReviewAfter != "" {
		writeField("review_after", f.ReviewAfter)
	}
	b.WriteString("---\n\n")
	b.WriteString(f.Body)
	b.WriteByte('\n')
	return b.String()
}
