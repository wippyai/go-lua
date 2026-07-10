package lsp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/check/service"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/embedding"
	"github.com/wippyai/go-lua/analysis/lsp/jsonrpc2"
)

func TestInProcessTransportLifecycleIncrementalCancellationAndVersionedDiagnostics(t *testing.T) {
	base := service.NewBatchSession()
	session := &blockFirstSolveSession{
		WorkspaceSession: base,
		started:          make(chan struct{}),
		canceled:         make(chan struct{}),
	}
	server := NewServer(session, Options{Debounce: time.Millisecond})
	client, peer := net.Pipe()
	t.Cleanup(func() {
		_ = client.Close()
		_ = peer.Close()
	})
	serveDone := make(chan error, 1)
	go func() { serveDone <- ServeStream(context.Background(), peer, peer, server) }()
	framer := jsonrpc2.NewFramer(client, client)

	writeRPC(t, framer, map[string]any{"jsonrpc": "2.0", "id": 1, "method": methodInitialize, "params": map[string]any{"capabilities": initializeCapabilities(true)}})
	initialize := readRPC(t, client, framer)
	assertCapabilities(t, initialize, true)
	var correlated struct {
		ID int `json:"id"`
	}
	if err := json.Unmarshal(initialize, &correlated); err != nil || correlated.ID != 1 {
		t.Fatalf("initialize response did not preserve request id: %s", initialize)
	}
	writeRPC(t, framer, map[string]any{"jsonrpc": "2.0", "method": methodInitialized, "params": map[string]any{}})

	uri := "file:///workspace/main.lua"
	writeRPC(t, framer, map[string]any{
		"jsonrpc": "2.0",
		"method":  methodDidOpen,
		"params": map[string]any{"textDocument": map[string]any{
			"uri": uri, "languageId": "lua", "version": 1, "text": "local value: number = 1\nreturn value\n",
		}},
	})
	select {
	case <-session.started:
	case <-time.After(5 * time.Second):
		t.Fatal("initial solve did not start")
	}

	writeRPC(t, framer, map[string]any{
		"jsonrpc": "2.0",
		"method":  methodDidChange,
		"params": map[string]any{
			"textDocument":   map[string]any{"uri": uri, "version": 2},
			"contentChanges": []map[string]any{{"text": "local value = missing_value\nreturn value\n"}},
		},
	})
	select {
	case <-session.canceled:
	case <-time.After(5 * time.Second):
		t.Fatal("superseded solve was not canceled")
	}

	published := readRPC(t, client, framer)
	var notification struct {
		Method string `json:"method"`
		Params struct {
			URI         string            `json:"uri"`
			Version     int64             `json:"version"`
			Diagnostics []json.RawMessage `json:"diagnostics"`
		} `json:"params"`
	}
	if err := json.Unmarshal(published, &notification); err != nil {
		t.Fatalf("decode publishDiagnostics: %v", err)
	}
	if notification.Method != methodPublishDiagnostics || notification.Params.URI != uri || notification.Params.Version != 2 {
		t.Fatalf("publish notification = %s", published)
	}
	if len(notification.Params.Diagnostics) == 0 {
		t.Fatalf("version 2 publish had no diagnostics: %s", published)
	}

	writeRPC(t, framer, map[string]any{"jsonrpc": "2.0", "id": 2, "method": methodPullDiagnostics, "params": map[string]any{"textDocument": map[string]any{"uri": uri}}})
	pull := readRPC(t, client, framer)
	var pullResponse struct {
		Result struct {
			Kind     string            `json:"kind"`
			ResultID string            `json:"resultId"`
			Items    []json.RawMessage `json:"items"`
		} `json:"result"`
	}
	if err := json.Unmarshal(pull, &pullResponse); err != nil {
		t.Fatalf("decode pull diagnostics: %v", err)
	}
	if pullResponse.Result.Kind != "full" || pullResponse.Result.ResultID == "" || len(pullResponse.Result.Items) == 0 {
		t.Fatalf("pull diagnostics = %s", pull)
	}

	writeRPC(t, framer, map[string]any{"jsonrpc": "2.0", "method": methodDidClose, "params": map[string]any{"textDocument": map[string]any{"uri": uri}}})
	closed := readRPC(t, client, framer)
	if err := json.Unmarshal(closed, &notification); err != nil {
		t.Fatalf("decode close publication: %v", err)
	}
	if notification.Method != methodPublishDiagnostics || notification.Params.Version != 2 || len(notification.Params.Diagnostics) != 0 {
		t.Fatalf("close publication = %s", closed)
	}

	writeRPC(t, framer, map[string]any{"jsonrpc": "2.0", "id": 3, "method": methodShutdown})
	_ = readRPC(t, client, framer)
	writeRPC(t, framer, map[string]any{"jsonrpc": "2.0", "method": methodExit})
	select {
	case err := <-serveDone:
		if err != nil {
			t.Fatalf("ServeStream: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("stream server did not exit")
	}
}

func TestHTTPPostAndLongPollFrontend(t *testing.T) {
	server := NewServer(service.NewBatchSession(), Options{Debounce: time.Millisecond})
	handler := NewHTTPHandler(server)
	request := httptest.NewRequest(http.MethodPost, "/lsp", rpcBody(t, map[string]any{"jsonrpc": "2.0", "id": "init", "method": methodInitialize, "params": map[string]any{"capabilities": initializeCapabilities(true)}}))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("initialize HTTP status = %d, body=%s", recorder.Code, recorder.Body.String())
	}
	assertCapabilities(t, recorder.Body.Bytes(), true)

	server.publish(context.Background(), "test/notification", map[string]string{"ok": "yes"})
	poll := httptest.NewRequest(http.MethodPost, "/notifications?timeoutMs=0", nil)
	pollRecorder := httptest.NewRecorder()
	handler.ServeHTTP(pollRecorder, poll)
	if pollRecorder.Code != http.StatusOK {
		t.Fatalf("notification poll status = %d", pollRecorder.Code)
	}
	var notification struct {
		Method string `json:"method"`
	}
	if err := json.Unmarshal(pollRecorder.Body.Bytes(), &notification); err != nil {
		t.Fatalf("decode long-poll notification: %v", err)
	}
	if notification.Method != "test/notification" {
		t.Fatalf("long-poll notification = %s", pollRecorder.Body.String())
	}
	_, _ = server.Handle(context.Background(), jsonrpc2.Request{Method: methodExit})
}

func TestInitializeNegotiatesPrepareRenameCapability(t *testing.T) {
	for _, prepareSupport := range []bool{false, true} {
		t.Run(fmt.Sprintf("prepare support %t", prepareSupport), func(t *testing.T) {
			server := NewServer(service.NewBatchSession(), Options{Debounce: time.Hour})
			defer func() { _, _ = server.Handle(context.Background(), jsonrpc2.Request{Method: methodExit}) }()
			result, problem := server.Handle(context.Background(), jsonrpc2.Request{
				Method: methodInitialize,
				Params: paramsJSON(t, map[string]any{"capabilities": initializeCapabilities(prepareSupport)}),
			})
			if problem != nil {
				t.Fatalf("initialize: %v", problem)
			}
			payload, err := json.Marshal(map[string]any{"result": result})
			if err != nil {
				t.Fatalf("marshal initialize result: %v", err)
			}
			assertCapabilities(t, payload, prepareSupport)
		})
	}
}

func TestCloseSerializesUnitRemovalWithReopen(t *testing.T) {
	base := service.NewBatchSession()
	session := &blockRemoveSession{
		WorkspaceSession: base,
		started:          make(chan struct{}),
		release:          make(chan struct{}),
	}
	server := NewServer(session, Options{Debounce: time.Hour})
	defer func() { _, _ = server.Handle(context.Background(), jsonrpc2.Request{Method: methodExit}) }()
	if _, problem := server.Handle(context.Background(), jsonrpc2.Request{Method: methodInitialize, Params: paramsJSON(t, map[string]any{"capabilities": initializeCapabilities(true)})}); problem != nil {
		t.Fatalf("initialize: %v", problem)
	}

	uri := "file:///workspace/reopen.lua"
	openRequest := func(version int64, text string) jsonrpc2.Request {
		return jsonrpc2.Request{
			Method: methodDidOpen,
			Params: paramsJSON(t, map[string]any{"textDocument": map[string]any{
				"uri": uri, "languageId": "lua", "version": version, "text": text,
			}}),
		}
	}
	if _, problem := server.Handle(context.Background(), openRequest(1, "return 1\n")); problem != nil {
		t.Fatalf("first open: %v", problem)
	}
	closeRequest := jsonrpc2.Request{
		Method: methodDidClose,
		Params: paramsJSON(t, map[string]any{"textDocument": map[string]any{"uri": uri}}),
	}
	reopenRequest := openRequest(2, "return 2\n")

	closeDone := make(chan *jsonrpc2.Error, 1)
	go func() {
		_, problem := server.Handle(context.Background(), closeRequest)
		closeDone <- problem
	}()
	select {
	case <-session.started:
	case <-time.After(5 * time.Second):
		t.Fatal("close did not begin unit removal")
	}

	reopenDone := make(chan *jsonrpc2.Error, 1)
	go func() {
		_, problem := server.Handle(context.Background(), reopenRequest)
		reopenDone <- problem
	}()
	if server.mu.TryLock() {
		server.mu.Unlock()
		t.Fatal("close released the server lock before removing its unit")
	}

	close(session.release)
	if problem := <-closeDone; problem != nil {
		t.Fatalf("close: %v", problem)
	}
	if problem := <-reopenDone; problem != nil {
		t.Fatalf("reopen: %v", problem)
	}
}

func TestPublishDiagnosticsVersionZeroIsStillTagged(t *testing.T) {
	payload, err := json.Marshal(publishDiagnosticsParams{URI: "file:///workspace/main.lua", Version: 0, Diagnostics: []protocolDiagnostic{}})
	if err != nil {
		t.Fatalf("marshal publish diagnostics: %v", err)
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("decode publish diagnostics: %v", err)
	}
	if got, ok := decoded["version"]; !ok || string(got) != "0" {
		t.Fatalf("publish diagnostics omitted version zero: %s", payload)
	}
}

func TestInProcessHoverProjectsSolvedTypeAndCanonicalProofTrace(t *testing.T) {
	server, uri := openSolvedDocument(t, "local value: number = 1\nlocal redundant = value as number\nreturn redundant\n")
	defer func() { _, _ = server.Handle(context.Background(), jsonrpc2.Request{Method: methodExit}) }()

	result, problem := server.Handle(context.Background(), jsonrpc2.Request{
		Method: methodHover,
		Params: paramsJSON(t, map[string]any{
			"textDocument": map[string]any{"uri": uri},
			"position":     map[string]any{"line": 1, "character": 18},
		}),
	})
	if problem != nil {
		t.Fatalf("hover: %v", problem)
	}
	hover, ok := result.(hoverResult)
	if !ok {
		t.Fatalf("hover result = %#v, want hoverResult", result)
	}
	if hover.Range == nil || hover.Range.Start != (Position{Line: 1, Character: 18}) {
		t.Fatalf("hover range = %#v, want start at the hovered expression", hover.Range)
	}
	for _, want := range []string{"```lua", "advice.redundant_claim — proven", "proven:"} {
		if !strings.Contains(hover.Contents.Value, want) {
			t.Fatalf("hover markdown missing %q:\n%s", want, hover.Contents.Value)
		}
	}
}

func TestInProcessBinderNavigationUsesOccurrencesAndUTF16Ranges(t *testing.T) {
	source := "local prefix = \"😀\"; local value = 1; value = value + 1\n"
	server, uri := openSolvedDocument(t, source)
	defer func() { _, _ = server.Handle(context.Background(), jsonrpc2.Request{Method: methodExit}) }()
	buffer := newTextBuffer([]byte(source))
	position, err := buffer.positionForOffset(strings.LastIndex(source, "value"))
	if err != nil {
		t.Fatalf("source position: %v", err)
	}
	if want := (Position{Line: 0, Character: 46}); position != want {
		t.Fatalf("UTF-16 request position = %#v, want %#v", position, want)
	}

	params := map[string]any{
		"textDocument": map[string]any{"uri": uri},
		"position":     position,
	}
	result, problem := server.Handle(context.Background(), jsonrpc2.Request{Method: methodDefinition, Params: paramsJSON(t, params)})
	if problem != nil {
		t.Fatalf("definition: %v", problem)
	}
	definition, ok := result.(*Location)
	if !ok || definition.URI != uri || definition.Range.Start != (Position{Line: 0, Character: 27}) {
		t.Fatalf("definition = %#v, want UTF-16 definition location", result)
	}

	result, problem = server.Handle(context.Background(), jsonrpc2.Request{Method: methodReferences, Params: paramsJSON(t, map[string]any{
		"textDocument": map[string]any{"uri": uri},
		"position":     position,
		"context":      map[string]any{"includeDeclaration": true},
	})})
	if problem != nil {
		t.Fatalf("references: %v", problem)
	}
	references, ok := result.([]Location)
	if !ok || len(references) != 3 {
		t.Fatalf("references = %#v, want definition plus write/read occurrences", result)
	}
	for index, want := range []int{27, 38, 46} {
		if got := references[index].Range.Start; got != (Position{Line: 0, Character: want}) {
			t.Fatalf("reference %d start = %#v, want character %d", index, got, want)
		}
	}

	result, problem = server.Handle(context.Background(), jsonrpc2.Request{Method: methodDocumentHighlight, Params: paramsJSON(t, params)})
	if problem != nil {
		t.Fatalf("document highlights: %v", problem)
	}
	highlights, ok := result.([]documentHighlight)
	if !ok || len(highlights) != 3 {
		t.Fatalf("highlights = %#v, want definition plus write/read occurrences", result)
	}
	for index, want := range []struct{ character, kind int }{{27, 3}, {38, 3}, {46, 2}} {
		if got := highlights[index]; got.Range.Start != (Position{Line: 0, Character: want.character}) || got.Kind != want.kind {
			t.Fatalf("highlight %d = %#v, want character=%d kind=%d", index, got, want.character, want.kind)
		}
	}
}

func TestInProcessSemanticTokensUseSolvedBinderKindsAndUTF16Deltas(t *testing.T) {
	source := "local prefix = \"😀\"; local value = 1; value = value + 1\n"
	server, uri := openSolvedDocument(t, source)
	defer func() { _, _ = server.Handle(context.Background(), jsonrpc2.Request{Method: methodExit}) }()

	result, problem := server.Handle(context.Background(), jsonrpc2.Request{
		Method: methodSemanticTokensFull,
		Params: paramsJSON(t, map[string]any{
			"textDocument": map[string]any{"uri": uri},
		}),
	})
	if problem != nil {
		t.Fatalf("semantic tokens: %v", problem)
	}
	tokens, ok := result.(semanticTokensResult)
	if !ok {
		t.Fatalf("semantic tokens result = %#v, want semanticTokensResult", result)
	}
	// prefix is at UTF-16 character 6. The emoji before value consumes two
	// UTF-16 units, so value's definition starts at character 27, then its
	// write/read occurrences start at 38 and 46. The stream is delta encoded.
	want := []uint32{
		0, 6, 6, 0, 0,
		0, 21, 5, 0, 0,
		0, 11, 5, 0, 0,
		0, 8, 5, 0, 0,
	}
	if !reflect.DeepEqual(tokens.Data, want) {
		t.Fatalf("semantic token data = %#v, want %#v", tokens.Data, want)
	}
}

func TestSemanticTokenEncoderCarriesCustomSolvedModifiers(t *testing.T) {
	document := semanticDocument{
		document: embedding.FileDocument("main.lua"),
		buffer:   newTextBuffer([]byte("😀tx\n")),
	}
	data, err := document.encodeSemanticTokens([]service.SemanticToken{{
		Kind: service.SemanticTokenVariable,
		Location: service.SourceLocation{File: "main.lua", Span: service.SourceSpan{
			StartLine: 1, StartCol: 5, EndLine: 1, EndCol: 6,
		}},
		Modifiers: []service.SemanticTokenModifier{
			service.SemanticTokenTypestateTracked,
			service.SemanticTokenPlacement,
		},
	}})
	if err != nil {
		t.Fatalf("encode semantic tokens: %v", err)
	}
	if want := []uint32{0, 2, 2, 0, 3}; !reflect.DeepEqual(data, want) {
		t.Fatalf("semantic token data = %#v, want %#v", data, want)
	}
}

func TestInProcessDocumentSymbolsPreserveServiceHierarchy(t *testing.T) {
	server, uri := openSolvedDocument(t, "function outer()\n  function inner()\n    return 1\n  end\n  return inner()\nend\nreturn { outer = outer }\n")
	defer func() { _, _ = server.Handle(context.Background(), jsonrpc2.Request{Method: methodExit}) }()

	result, problem := server.Handle(context.Background(), jsonrpc2.Request{
		Method: methodDocumentSymbol,
		Params: paramsJSON(t, map[string]any{
			"textDocument": map[string]any{"uri": uri},
		}),
	})
	if problem != nil {
		t.Fatalf("document symbols: %v", problem)
	}
	symbols, ok := result.([]documentSymbol)
	if !ok {
		t.Fatalf("document symbols = %#v, want []documentSymbol", result)
	}
	outer := documentSymbolNamed(symbols, "outer", 12)
	if outer == nil || len(outer.Children) != 1 || outer.Children[0].Name != "inner" || outer.Children[0].Kind != 12 {
		t.Fatalf("document symbols = %#v, want hierarchical outer/inner functions", symbols)
	}
	if outer.Range.Start != outer.SelectionRange.Start {
		t.Fatalf("outer symbol range = %#v, want matching service location ranges", outer)
	}
	if field := documentSymbolNamed(symbols, "outer", 8); field == nil {
		t.Fatalf("document symbols = %#v, want module field", symbols)
	}
}

func TestInProcessRenameRejectsCollisionAndCrossModuleExport(t *testing.T) {
	server, uri := openSolvedDocument(t, "local x = 1; local y = 2; return x\n")
	defer func() { _, _ = server.Handle(context.Background(), jsonrpc2.Request{Method: methodExit}) }()
	position := Position{Line: 0, Character: 6}
	if _, problem := server.Handle(context.Background(), jsonrpc2.Request{Method: methodPrepareRename, Params: paramsJSON(t, map[string]any{
		"textDocument": map[string]any{"uri": uri}, "position": position,
	})}); problem != nil {
		t.Fatalf("prepare local rename: %v", problem)
	}
	if _, problem := server.Handle(context.Background(), jsonrpc2.Request{Method: methodRename, Params: paramsJSON(t, map[string]any{
		"textDocument": map[string]any{"uri": uri}, "position": position, "newName": "y",
	})}); problem == nil || !strings.Contains(problem.Message, "capture or shadow") {
		t.Fatalf("collision rename problem = %#v, want clear capture/shadow rejection", problem)
	}
	if _, problem := server.Handle(context.Background(), jsonrpc2.Request{Method: methodPrepareRename, Params: paramsJSON(t, map[string]any{
		"textDocument": map[string]any{"uri": uri}, "position": Position{Line: 0, Character: 0},
	})}); problem == nil || !strings.Contains(problem.Message, "lexical binder") {
		t.Fatalf("non-binder prepareRename problem = %#v, want lexical-binder rejection", problem)
	}

	exported, exportedURI := openSolvedDocument(t, "function exported() return 1 end\nreturn exported\n")
	defer func() { _, _ = exported.Handle(context.Background(), jsonrpc2.Request{Method: methodExit}) }()
	if _, problem := exported.Handle(context.Background(), jsonrpc2.Request{Method: methodPrepareRename, Params: paramsJSON(t, map[string]any{
		"textDocument": map[string]any{"uri": exportedURI}, "position": Position{Line: 0, Character: 9},
	})}); problem == nil || !strings.Contains(problem.Message, "cross-module") {
		t.Fatalf("exported prepareRename problem = %#v, want cross-module rejection", problem)
	}
}

func TestInProcessRenameProjectsCompleteOccurrenceWorkspaceEdit(t *testing.T) {
	server, uri := openSolvedDocument(t, "local item = 1\nreturn item\n")
	defer func() { _, _ = server.Handle(context.Background(), jsonrpc2.Request{Method: methodExit}) }()
	result, problem := server.Handle(context.Background(), jsonrpc2.Request{Method: methodRename, Params: paramsJSON(t, map[string]any{
		"textDocument": map[string]any{"uri": uri}, "position": Position{Line: 1, Character: 7}, "newName": "renamed",
	})})
	if problem != nil {
		t.Fatalf("rename: %v", problem)
	}
	edit, ok := result.(workspaceEdit)
	if !ok || len(edit.DocumentChanges) != 1 {
		t.Fatalf("workspace edit = %#v, want one versioned document change", result)
	}
	documentChange := edit.DocumentChanges[0]
	if documentChange.TextDocument.URI != uri || documentChange.TextDocument.Version != 1 || len(documentChange.Edits) != 2 {
		t.Fatalf("document change = %#v, want version 1 definition/reference edits", documentChange)
	}
	for _, item := range documentChange.Edits {
		if item.NewText != "renamed" {
			t.Fatalf("rename edit = %#v, want new binder name", item)
		}
	}
	assertVersionedWorkspaceEditWire(t, edit)
}

func TestInProcessRenameRefusesUnsafeBinderProjections(t *testing.T) {
	tests := []struct {
		name   string
		source string
		needle string
		want   string
	}{
		{
			name:   "parameter",
			source: "local function f(value)\n  return value\nend\nreturn f(1)\n",
			needle: "value",
			want:   "parameters",
		},
		{
			name:   "implicit self",
			source: "local t = {}\nfunction t:m(v)\n  return self, v\nend\nreturn t\n",
			needle: "self",
			want:   "implicit self",
		},
		{
			name:   "vararg",
			source: "local function f(...)\n  return ...\nend\nreturn f(1)\n",
			needle: "...",
			want:   "varargs",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server, uri := openSolvedDocument(t, test.source)
			defer func() { _, _ = server.Handle(context.Background(), jsonrpc2.Request{Method: methodExit}) }()
			buffer := newTextBuffer([]byte(test.source))
			offset := strings.LastIndex(test.source, test.needle)
			if test.name == "vararg" {
				offset = strings.Index(test.source, test.needle)
			}
			position, err := buffer.positionForOffset(offset)
			if err != nil {
				t.Fatalf("selected %s position: %v", test.name, err)
			}
			for _, method := range []string{methodPrepareRename, methodRename} {
				_, problem := server.Handle(context.Background(), jsonrpc2.Request{Method: method, Params: paramsJSON(t, map[string]any{
					"textDocument": map[string]any{"uri": uri}, "position": position, "newName": "renamed",
				})})
				if problem == nil || !strings.Contains(problem.Message, test.want) {
					t.Fatalf("%s %s problem = %#v, want clear %q refusal", test.name, method, problem, test.want)
				}
			}
		})
	}
}

func TestInProcessRenameRefusesDuplicateOccurrenceRanges(t *testing.T) {
	base := service.NewBatchSession()
	session := &duplicateBinderOccurrencesSession{WorkspaceSession: base, name: "item"}
	server, uri := openSolvedDocumentWithSession(t, "local item = 1\nreturn item\n", session, Options{Debounce: time.Millisecond})
	defer func() { _, _ = server.Handle(context.Background(), jsonrpc2.Request{Method: methodExit}) }()

	_, problem := server.Handle(context.Background(), jsonrpc2.Request{Method: methodRename, Params: paramsJSON(t, map[string]any{
		"textDocument": map[string]any{"uri": uri}, "position": Position{Line: 1, Character: 7}, "newName": "renamed",
	})})
	if problem == nil || !strings.Contains(problem.Message, "overlapping") {
		t.Fatalf("duplicate occurrence rename problem = %#v, want overlapping-range refusal", problem)
	}
}

func TestInProcessRenameRefusesOverlappingOccurrenceRanges(t *testing.T) {
	base := service.NewBatchSession()
	session := &overlappingBinderOccurrencesSession{WorkspaceSession: base, name: "item"}
	server, uri := openSolvedDocumentWithSession(t, "local item = 1\nreturn item\n", session, Options{Debounce: time.Millisecond})
	defer func() { _, _ = server.Handle(context.Background(), jsonrpc2.Request{Method: methodExit}) }()

	_, problem := server.Handle(context.Background(), jsonrpc2.Request{Method: methodRename, Params: paramsJSON(t, map[string]any{
		"textDocument": map[string]any{"uri": uri}, "position": Position{Line: 1, Character: 7}, "newName": "renamed",
	})})
	if problem == nil || !strings.Contains(problem.Message, "overlapping") {
		t.Fatalf("overlapping occurrence rename problem = %#v, want overlapping-range refusal", problem)
	}
}

func TestInProcessRenameRefusesWhenDocumentChangesBeforeResponse(t *testing.T) {
	base := service.NewBatchSession()
	session := &blockBinderOccurrencesSession{
		WorkspaceSession: base,
		blockCall:        2,
		started:          make(chan struct{}),
		release:          make(chan struct{}),
	}
	server, uri := openSolvedDocumentWithSession(t, "local item = 1\nreturn item\n", session, Options{Debounce: time.Millisecond})
	defer func() { _, _ = server.Handle(context.Background(), jsonrpc2.Request{Method: methodExit}) }()

	done := make(chan *jsonrpc2.Error, 1)
	go func() {
		_, problem := server.Handle(context.Background(), jsonrpc2.Request{Method: methodRename, Params: paramsJSON(t, map[string]any{
			"textDocument": map[string]any{"uri": uri}, "position": Position{Line: 1, Character: 7}, "newName": "renamed",
		})})
		done <- problem
	}()
	select {
	case <-session.started:
	case <-time.After(5 * time.Second):
		t.Fatal("rename did not reach its final occurrence query")
	}
	if _, problem := server.Handle(context.Background(), jsonrpc2.Request{Method: methodDidChange, Params: paramsJSON(t, map[string]any{
		"textDocument":   map[string]any{"uri": uri, "version": 2},
		"contentChanges": []map[string]any{{"text": "local prefix = 0\nlocal item = 1\nreturn item\n"}},
	})}); problem != nil {
		t.Fatalf("concurrent didChange: %v", problem)
	}
	close(session.release)
	select {
	case problem := <-done:
		if problem == nil || !strings.Contains(problem.Message, "document changed") {
			t.Fatalf("rename after didChange problem = %#v, want stale-edit refusal", problem)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("rename did not return after concurrent didChange")
	}
}

func TestInProcessCodeActionUsesStructuredEditAndClearsDiagnostic(t *testing.T) {
	policy := diagnostic.Policy{Rules: map[diagnostic.Code]diagnostic.Rule{
		diagnostic.Code(judgment.DiagnosticCodeAdviceRedundantClaim): diagnostic.Enable(),
	}}
	server, uri := openSolvedDocumentWithOptions(t, "local value: number = 1\nlocal redundant = value as number\nreturn redundant\n", Options{
		Debounce: time.Millisecond,
		Resolver: FileConventionsResolver{Template: service.UnitInput{DiagnosticPolicy: policy}},
	})
	defer func() { _, _ = server.Handle(context.Background(), jsonrpc2.Request{Method: methodExit}) }()
	before := waitForDocumentVersion(t, server, 1)
	diagnostics, err := server.session.Diagnostics(context.Background(), service.ListDiagnosticsRequest{Selector: service.ResultSelector{UnitID: before.UnitID, SolveSeq: before.SolveSeq, Profile: before.Profile}})
	if err != nil || !containsDiagnosticCode(diagnostics.Rendered, string(judgment.DiagnosticCodeAdviceRedundantClaim)) {
		t.Fatalf("diagnostics before repair = %#v, %v; want redundant-claim diagnostic", diagnostics.Rendered, err)
	}

	result, problem := server.Handle(context.Background(), jsonrpc2.Request{Method: methodCodeAction, Params: paramsJSON(t, map[string]any{
		"textDocument": map[string]any{"uri": uri},
		"range":        map[string]any{"start": map[string]any{"line": 1, "character": 0}, "end": map[string]any{"line": 1, "character": 33}},
		"context":      map[string]any{"only": []string{"quickfix"}},
	})})
	if problem != nil {
		t.Fatalf("code actions: %v", problem)
	}
	actions, ok := result.([]codeAction)
	if !ok || len(actions) != 1 || actions[0].Kind != "quickfix" || len(actions[0].Edit.DocumentChanges) != 1 {
		t.Fatalf("code actions = %#v, want one descriptor-backed quickfix", result)
	}
	if got := actions[0].Edit.DocumentChanges[0].TextDocument; got.URI != uri || got.Version != 1 {
		t.Fatalf("code action document version = %#v, want %s at version 1", got, uri)
	}
	buffer := newTextBuffer([]byte("local value: number = 1\nlocal redundant = value as number\nreturn redundant\n"))
	edit := actions[0].Edit.DocumentChanges[0].Edits[0]
	if err := buffer.apply([]TextDocumentContentChangeEvent{{Range: &edit.Range, Text: edit.NewText}}); err != nil {
		t.Fatalf("apply code action: %v", err)
	}
	if got, want := string(buffer.bytes()), "local value: number = 1\nlocal redundant = value\nreturn redundant\n"; got != want {
		t.Fatalf("applied code action = %q, want %q", got, want)
	}
	if _, problem := server.Handle(context.Background(), jsonrpc2.Request{Method: methodDidChange, Params: paramsJSON(t, map[string]any{
		"textDocument":   map[string]any{"uri": uri, "version": 2},
		"contentChanges": []map[string]any{{"text": string(buffer.bytes())}},
	})}); problem != nil {
		t.Fatalf("apply repaired document: %v", problem)
	}
	after := waitForDocumentVersion(t, server, 2)
	diagnostics, err = server.session.Diagnostics(context.Background(), service.ListDiagnosticsRequest{Selector: service.ResultSelector{UnitID: after.UnitID, SolveSeq: after.SolveSeq, Profile: after.Profile}})
	if err != nil {
		t.Fatalf("diagnostics after repair: %v", err)
	}
	if containsDiagnosticCode(diagnostics.Rendered, string(judgment.DiagnosticCodeAdviceRedundantClaim)) {
		t.Fatalf("diagnostics after repair = %#v, want redundant-claim diagnostic gone", diagnostics.Rendered)
	}
}

func TestInProcessHoverMapsThroughCapturedSnapshotBuffer(t *testing.T) {
	session := &blockPositionLookupSession{
		WorkspaceSession: service.NewBatchSession(),
		started:          make(chan struct{}),
		release:          make(chan struct{}),
	}
	source := "local value: number = 1\nlocal redundant = value as number\nreturn redundant\n"
	server, uri := openSolvedDocumentWithSession(t, source, session, Options{Debounce: time.Millisecond})
	defer func() { _, _ = server.Handle(context.Background(), jsonrpc2.Request{Method: methodExit}) }()

	done := make(chan struct {
		result  any
		problem *jsonrpc2.Error
	}, 1)
	go func() {
		result, problem := server.Handle(context.Background(), jsonrpc2.Request{Method: methodHover, Params: paramsJSON(t, map[string]any{
			"textDocument": map[string]any{"uri": uri}, "position": Position{Line: 1, Character: 18},
		})})
		done <- struct {
			result  any
			problem *jsonrpc2.Error
		}{result, problem}
	}()
	select {
	case <-session.started:
	case <-time.After(5 * time.Second):
		t.Fatal("hover did not reach position lookup")
	}
	if _, problem := server.Handle(context.Background(), jsonrpc2.Request{Method: methodDidChange, Params: paramsJSON(t, map[string]any{
		"textDocument":   map[string]any{"uri": uri, "version": 2},
		"contentChanges": []map[string]any{{"text": "local value: number = 1\n😀local redundant = value as number\nreturn redundant\n"}},
	})}); problem != nil {
		t.Fatalf("concurrent didChange: %v", problem)
	}
	close(session.release)
	select {
	case response := <-done:
		if response.problem != nil {
			t.Fatalf("hover after didChange: %v", response.problem)
		}
		hover, ok := response.result.(hoverResult)
		if !ok || hover.Range == nil || hover.Range.Start != (Position{Line: 1, Character: 18}) {
			t.Fatalf("hover range after didChange = %#v, want captured version-1 range", response.result)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("hover did not return after concurrent didChange")
	}
}

func containsDiagnosticCode(items []diagnostic.Diagnostic, code string) bool {
	for _, item := range items {
		if string(item.Code) == code {
			return true
		}
	}
	return false
}

func documentSymbolNamed(items []documentSymbol, name string, kind int) *documentSymbol {
	for index := range items {
		if items[index].Name == name && items[index].Kind == kind {
			return &items[index]
		}
	}
	return nil
}

func openSolvedDocument(t *testing.T, text string) (*Server, string) {
	t.Helper()
	return openSolvedDocumentWithOptions(t, text, Options{Debounce: time.Millisecond})
}

func openSolvedDocumentWithOptions(t *testing.T, text string, options Options) (*Server, string) {
	return openSolvedDocumentWithSession(t, text, service.NewBatchSession(), options)
}

func openSolvedDocumentWithSession(t *testing.T, text string, session service.WorkspaceSession, options Options) (*Server, string) {
	t.Helper()
	server := NewServer(session, options)
	if _, problem := server.Handle(context.Background(), jsonrpc2.Request{Method: methodInitialize, Params: paramsJSON(t, map[string]any{"capabilities": initializeCapabilities(true)})}); problem != nil {
		t.Fatalf("initialize: %v", problem)
	}
	uri := "file:///workspace/semantic.lua"
	if _, problem := server.Handle(context.Background(), jsonrpc2.Request{
		Method: methodDidOpen,
		Params: paramsJSON(t, map[string]any{"textDocument": map[string]any{
			"uri": uri, "languageId": "lua", "version": 1, "text": text,
		}}),
	}); problem != nil {
		t.Fatalf("didOpen: %v", problem)
	}
	_ = waitForDocumentVersion(t, server, 1)
	return server, uri
}

func waitForDocumentVersion(t *testing.T, server *Server, version int64) service.ResultTag {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		server.mu.Lock()
		var unitID service.UnitID
		for _, document := range server.documents {
			unitID = document.unitID
		}
		server.mu.Unlock()
		if unitID != "" {
			if completed, ok := server.session.LastComplete(context.Background(), service.ResultRequest{Selector: service.ResultSelector{UnitID: unitID}}); ok {
				tag := completed.Tag()
				if tag.DocumentVersion == version {
					return tag
				}
			}
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("document version %d solve did not complete", version)
	return service.ResultTag{}
}

type blockFirstSolveSession struct {
	service.WorkspaceSession
	mu       sync.Mutex
	first    bool
	started  chan struct{}
	canceled chan struct{}
}

type duplicateBinderOccurrencesSession struct {
	service.WorkspaceSession
	name string
}

type overlappingBinderOccurrencesSession struct {
	service.WorkspaceSession
	name string
}

func (s *overlappingBinderOccurrencesSession) BinderOccurrences(ctx context.Context, request service.BinderOccurrencesRequest) (service.BinderOccurrencesResponse, error) {
	response, err := s.WorkspaceSession.BinderOccurrences(ctx, request)
	if err != nil {
		return service.BinderOccurrencesResponse{}, err
	}
	for index := range response.Binders {
		binder := &response.Binders[index]
		if binder.Name != s.name || len(binder.Occurrences) == 0 {
			continue
		}
		overlap := binder.Occurrences[0]
		if overlap.Location.Span.StartCol < overlap.Location.Span.EndCol {
			overlap.Location.Span.StartCol++
		}
		binder.Occurrences = append(binder.Occurrences, overlap)
	}
	return response, nil
}

func (s *duplicateBinderOccurrencesSession) BinderOccurrences(ctx context.Context, request service.BinderOccurrencesRequest) (service.BinderOccurrencesResponse, error) {
	response, err := s.WorkspaceSession.BinderOccurrences(ctx, request)
	if err != nil {
		return service.BinderOccurrencesResponse{}, err
	}
	for index := range response.Binders {
		binder := &response.Binders[index]
		if binder.Name == s.name && len(binder.Occurrences) != 0 {
			binder.Occurrences = append(binder.Occurrences, binder.Occurrences[0])
		}
	}
	return response, nil
}

type blockBinderOccurrencesSession struct {
	service.WorkspaceSession
	mu        sync.Mutex
	calls     int
	blockCall int
	started   chan struct{}
	release   chan struct{}
}

type blockPositionLookupSession struct {
	service.WorkspaceSession
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (s *blockPositionLookupSession) PositionLookup(ctx context.Context, request service.PositionLookupRequest) (service.PositionLookupResponse, error) {
	s.once.Do(func() { close(s.started) })
	select {
	case <-s.release:
	case <-ctx.Done():
		return service.PositionLookupResponse{}, ctx.Err()
	}
	return s.WorkspaceSession.PositionLookup(ctx, request)
}

func (s *blockBinderOccurrencesSession) BinderOccurrences(ctx context.Context, request service.BinderOccurrencesRequest) (service.BinderOccurrencesResponse, error) {
	s.mu.Lock()
	s.calls++
	block := s.calls == s.blockCall
	s.mu.Unlock()
	if block {
		close(s.started)
		select {
		case <-s.release:
		case <-ctx.Done():
			return service.BinderOccurrencesResponse{}, ctx.Err()
		}
	}
	return s.WorkspaceSession.BinderOccurrences(ctx, request)
}

type blockRemoveSession struct {
	service.WorkspaceSession
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (s *blockRemoveSession) RemoveUnit(ctx context.Context, id service.UnitID) error {
	s.once.Do(func() { close(s.started) })
	select {
	case <-s.release:
		return s.WorkspaceSession.RemoveUnit(ctx, id)
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *blockFirstSolveSession) EnsureSolved(ctx context.Context, request service.SolveRequest) (service.ResultTag, error) {
	s.mu.Lock()
	first := !s.first
	if first {
		s.first = true
	}
	s.mu.Unlock()
	if first {
		close(s.started)
		<-ctx.Done()
		close(s.canceled)
		return service.ResultTag{}, ctx.Err()
	}
	return s.WorkspaceSession.EnsureSolved(ctx, request)
}

func writeRPC(t *testing.T, framer *jsonrpc2.Framer, value any) {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := framer.Write(payload); err != nil {
		t.Fatalf("write RPC: %v", err)
	}
}

func rpcBody(t *testing.T, value any) *bytes.Reader {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return bytes.NewReader(payload)
}

func paramsJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func readRPC(t *testing.T, connection net.Conn, framer *jsonrpc2.Framer) []byte {
	t.Helper()
	if err := connection.SetReadDeadline(time.Now().Add(10 * time.Second)); err != nil {
		t.Fatal(err)
	}
	payload, err := framer.Read()
	if err != nil {
		t.Fatalf("read RPC: %v", err)
	}
	if err := connection.SetReadDeadline(time.Time{}); err != nil {
		t.Fatal(err)
	}
	return payload
}

func initializeCapabilities(prepareSupport bool) map[string]any {
	return map[string]any{
		"workspace":    map[string]any{"workspaceEdit": map[string]any{"documentChanges": true}},
		"textDocument": map[string]any{"rename": map[string]any{"prepareSupport": prepareSupport}},
	}
}

func assertCapabilities(t *testing.T, payload []byte, prepareSupport bool) {
	t.Helper()
	var response struct {
		Result struct {
			Capabilities struct {
				TextDocumentSync struct {
					OpenClose bool `json:"openClose"`
					Change    int  `json:"change"`
				} `json:"textDocumentSync"`
				DiagnosticProvider        json.RawMessage `json:"diagnosticProvider"`
				HoverProvider             bool            `json:"hoverProvider"`
				DefinitionProvider        bool            `json:"definitionProvider"`
				ReferencesProvider        bool            `json:"referencesProvider"`
				DocumentHighlightProvider bool            `json:"documentHighlightProvider"`
				DocumentSymbolProvider    bool            `json:"documentSymbolProvider"`
				RenameProvider            json.RawMessage `json:"renameProvider"`
				PrepareRenameProvider     json.RawMessage `json:"prepareRenameProvider"`
				CodeActionProvider        bool            `json:"codeActionProvider"`
				SemanticTokensProvider    struct {
					Legend struct {
						TokenTypes     []string `json:"tokenTypes"`
						TokenModifiers []string `json:"tokenModifiers"`
					} `json:"legend"`
					Full  bool `json:"full"`
					Range bool `json:"range"`
				} `json:"semanticTokensProvider"`
			} `json:"capabilities"`
		} `json:"result"`
	}
	if err := json.Unmarshal(payload, &response); err != nil {
		t.Fatalf("decode initialize result: %v", err)
	}
	semantic := response.Result.Capabilities.SemanticTokensProvider
	if !response.Result.Capabilities.TextDocumentSync.OpenClose || response.Result.Capabilities.TextDocumentSync.Change != textDocumentSyncIncremental || len(response.Result.Capabilities.DiagnosticProvider) == 0 || !response.Result.Capabilities.HoverProvider || !response.Result.Capabilities.DefinitionProvider || !response.Result.Capabilities.ReferencesProvider || !response.Result.Capabilities.DocumentHighlightProvider || !response.Result.Capabilities.DocumentSymbolProvider || len(response.Result.Capabilities.RenameProvider) == 0 || len(response.Result.Capabilities.PrepareRenameProvider) != 0 || !response.Result.Capabilities.CodeActionProvider || !semantic.Full || semantic.Range || !reflect.DeepEqual(semantic.Legend.TokenTypes, []string{"variable", "parameter", "function"}) || !reflect.DeepEqual(semantic.Legend.TokenModifiers, []string{"typestate-tracked", "placement"}) {
		t.Fatalf("advertised capabilities = %s", payload)
	}
	if prepareSupport {
		var options renameOptions
		if err := json.Unmarshal(response.Result.Capabilities.RenameProvider, &options); err != nil || !options.PrepareProvider {
			t.Fatalf("rename capability = %s, want prepareProvider", response.Result.Capabilities.RenameProvider)
		}
		return
	}
	var renameProvider bool
	if err := json.Unmarshal(response.Result.Capabilities.RenameProvider, &renameProvider); err != nil || !renameProvider {
		t.Fatalf("rename capability = %s, want boolean true", response.Result.Capabilities.RenameProvider)
	}
}

func assertVersionedWorkspaceEditWire(t *testing.T, edit workspaceEdit) {
	t.Helper()
	payload, err := json.Marshal(edit)
	if err != nil {
		t.Fatalf("marshal workspace edit: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		t.Fatalf("decode workspace edit: %v", err)
	}
	if _, found := fields["changes"]; found || len(fields["documentChanges"]) == 0 {
		t.Fatalf("workspace edit wire shape = %s, want only versioned documentChanges", payload)
	}
}
