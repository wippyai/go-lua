package lsp

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/wippyai/go-lua/analysis/check/service"
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
	defer client.Close()
	defer peer.Close()
	serveDone := make(chan error, 1)
	go func() { serveDone <- ServeStream(context.Background(), peer, peer, server) }()
	framer := jsonrpc2.NewFramer(client, client)

	writeRPC(t, framer, map[string]any{"jsonrpc": "2.0", "id": 1, "method": methodInitialize, "params": map[string]any{}})
	initialize := readRPC(t, client, framer)
	assertCapabilities(t, initialize)
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
	request := httptest.NewRequest(http.MethodPost, "/lsp", rpcBody(t, map[string]any{"jsonrpc": "2.0", "id": "init", "method": methodInitialize}))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("initialize HTTP status = %d, body=%s", recorder.Code, recorder.Body.String())
	}
	assertCapabilities(t, recorder.Body.Bytes())

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

func TestCloseSerializesUnitRemovalWithReopen(t *testing.T) {
	base := service.NewBatchSession()
	session := &blockRemoveSession{
		WorkspaceSession: base,
		started:          make(chan struct{}),
		release:          make(chan struct{}),
	}
	server := NewServer(session, Options{Debounce: time.Hour})
	defer func() { _, _ = server.Handle(context.Background(), jsonrpc2.Request{Method: methodExit}) }()
	if _, problem := server.Handle(context.Background(), jsonrpc2.Request{Method: methodInitialize}); problem != nil {
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

type blockFirstSolveSession struct {
	service.WorkspaceSession
	mu       sync.Mutex
	first    bool
	started  chan struct{}
	canceled chan struct{}
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

func assertCapabilities(t *testing.T, payload []byte) {
	t.Helper()
	var response struct {
		Result struct {
			Capabilities struct {
				TextDocumentSync struct {
					OpenClose bool `json:"openClose"`
					Change    int  `json:"change"`
				} `json:"textDocumentSync"`
				DiagnosticProvider json.RawMessage `json:"diagnosticProvider"`
				HoverProvider      bool            `json:"hoverProvider"`
			} `json:"capabilities"`
		} `json:"result"`
	}
	if err := json.Unmarshal(payload, &response); err != nil {
		t.Fatalf("decode initialize result: %v", err)
	}
	if !response.Result.Capabilities.TextDocumentSync.OpenClose || response.Result.Capabilities.TextDocumentSync.Change != textDocumentSyncIncremental || len(response.Result.Capabilities.DiagnosticProvider) == 0 || !response.Result.Capabilities.HoverProvider {
		t.Fatalf("advertised capabilities = %s", payload)
	}
}
