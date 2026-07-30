package lsp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/wippyai/go-lua/analysis/lsp/jsonrpc2"
)

// NewHTTPHandler exposes the same JSON-RPC methods over plain HTTP POST. POST
// /lsp (or /) is request/response; POST /notifications is a websocket-free
// long-poll for server notifications such as publishDiagnostics.
func NewHTTPHandler(server *Server) http.Handler {
	queue := &notificationQueue{wake: make(chan struct{})}
	server.SetNotifier(func(ctx context.Context, method string, params any) error {
		payload, err := json.Marshal(struct {
			JSONRPC string `json:"jsonrpc"`
			Method  string `json:"method"`
			Params  any    `json:"params"`
		}{JSONRPC: jsonrpc2.Version, Method: method, Params: params})
		if err == nil {
			queue.push(payload)
		}
		return err
	})
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			writer.Header().Set("Allow", http.MethodPost)
			http.Error(writer, "POST required", http.StatusMethodNotAllowed)
			return
		}
		if request.URL.Path == "/notifications" {
			handleNotificationPoll(writer, request, queue)
			return
		}
		if request.URL.Path != "/" && request.URL.Path != "/lsp" {
			http.NotFound(writer, request)
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(writer, request.Body, jsonrpc2.DefaultMaxLength))
		if err != nil {
			http.Error(writer, "invalid JSON-RPC body", http.StatusBadRequest)
			return
		}
		parsed, problem := jsonrpc2.ParseRequest(body)
		if problem != nil {
			writeJSONRPC(writer, jsonrpc2.Failure(nil, problem))
			return
		}
		result, problem := server.Handle(request.Context(), parsed)
		if !parsed.HasID {
			writer.WriteHeader(http.StatusNoContent)
			return
		}
		if problem != nil {
			writeJSONRPC(writer, jsonrpc2.Failure(parsed.ID, problem))
			return
		}
		writeJSONRPC(writer, jsonrpc2.Success(parsed.ID, result))
	})
}

func writeJSONRPC(writer http.ResponseWriter, payload []byte) {
	writer.Header().Set("Content-Type", "application/vscode-jsonrpc")
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write(payload)
}

func handleNotificationPoll(writer http.ResponseWriter, request *http.Request, queue *notificationQueue) {
	timeout := 25 * time.Second
	if raw := request.URL.Query().Get("timeoutMs"); raw != "" {
		if milliseconds, err := strconv.Atoi(raw); err == nil && milliseconds >= 0 {
			timeout = time.Duration(milliseconds) * time.Millisecond
			if timeout > 30*time.Second {
				timeout = 30 * time.Second
			}
		}
	}
	ctx, cancel := context.WithTimeout(request.Context(), timeout)
	defer cancel()
	payload, ok := queue.pop(ctx)
	if !ok {
		writer.WriteHeader(http.StatusNoContent)
		return
	}
	writeJSONRPC(writer, payload)
}

type notificationQueue struct {
	mu       sync.Mutex
	messages [][]byte
	wake     chan struct{}
}

func (q *notificationQueue) push(payload []byte) {
	q.mu.Lock()
	q.messages = append(q.messages, append([]byte(nil), payload...))
	close(q.wake)
	q.wake = make(chan struct{})
	q.mu.Unlock()
}

func (q *notificationQueue) pop(ctx context.Context) ([]byte, bool) {
	for {
		q.mu.Lock()
		if len(q.messages) != 0 {
			payload := q.messages[0]
			q.messages = q.messages[1:]
			q.mu.Unlock()
			return payload, true
		}
		wake := q.wake
		q.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, false
		case <-wake:
		}
	}
}
