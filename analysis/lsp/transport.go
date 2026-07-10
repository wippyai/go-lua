package lsp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"

	"github.com/wippyai/go-lua/analysis/lsp/jsonrpc2"
)

// ServeStream is the stdio-style frontend for Content-Length-framed LSP. The
// server remains transport-agnostic; this function only frames JSON-RPC and
// serializes writes from responses and asynchronous notifications.
func ServeStream(ctx context.Context, input io.Reader, output io.Writer, server *Server) error {
	framer := jsonrpc2.NewFramer(input, output)
	var writeMu sync.Mutex
	write := func(payload []byte) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return framer.Write(payload)
	}
	server.SetNotifier(func(ctx context.Context, method string, params any) error {
		payload, err := json.Marshal(struct {
			JSONRPC string `json:"jsonrpc"`
			Method  string `json:"method"`
			Params  any    `json:"params"`
		}{JSONRPC: jsonrpc2.Version, Method: method, Params: params})
		if err != nil {
			return err
		}
		return write(payload)
	})

	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		payload, err := framer.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		request, problem := jsonrpc2.ParseRequest(payload)
		if problem != nil {
			if err := write(jsonrpc2.Failure(nil, problem)); err != nil {
				return err
			}
			continue
		}
		result, problem := server.Handle(ctx, request)
		if request.HasID {
			response := jsonrpc2.Success(request.ID, result)
			if problem != nil {
				response = jsonrpc2.Failure(request.ID, problem)
			}
			if err := write(response); err != nil {
				return err
			}
		}
		if server.Exited() {
			return nil
		}
	}
}

// ServeStdio names the stdio frontend explicitly for command entrypoints and
// in-process tests. It has the same behavior as ServeStream.
func ServeStdio(ctx context.Context, input io.Reader, output io.Writer, server *Server) error {
	return ServeStream(ctx, input, output, server)
}
