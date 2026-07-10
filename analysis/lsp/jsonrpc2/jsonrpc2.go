// Package jsonrpc2 implements the small JSON-RPC 2.0 transport subset used by
// the LSP adapter. It deliberately owns no LSP or checker semantics.
package jsonrpc2

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

const (
	Version          = "2.0"
	DefaultMaxLength = 16 << 20

	ParseError     = -32700
	InvalidRequest = -32600
	MethodNotFound = -32601
	InvalidParams  = -32602
	InternalError  = -32603
)

// Request is one JSON-RPC request or notification. Batch requests are
// intentionally outside this minimal transport's v1 scope.
type Request struct {
	ID     json.RawMessage
	HasID  bool
	Method string
	Params json.RawMessage
}

func (r Request) Notification() bool { return !r.HasID }

// Error is a protocol error suitable for a JSON-RPC response.
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func (e *Error) Error() string { return e.Message }

func NewError(code int, message string, data any) *Error {
	return &Error{Code: code, Message: message, Data: data}
}

type failureResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Error   *Error          `json:"error"`
}

type successResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result"`
}

func Success(id json.RawMessage, result any) []byte {
	if len(id) == 0 {
		id = json.RawMessage("null")
	}
	data, _ := json.Marshal(successResponse{JSONRPC: Version, ID: id, Result: result})
	return data
}

func Failure(id json.RawMessage, problem *Error) []byte {
	if len(id) == 0 {
		id = json.RawMessage("null")
	}
	data, _ := json.Marshal(failureResponse{JSONRPC: Version, ID: id, Error: problem})
	return data
}

// ParseRequest validates one JSON-RPC request object. A JSON-RPC batch is
// rejected because the LSP server deliberately does not implement batches.
func ParseRequest(data []byte) (Request, *Error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return Request{}, NewError(ParseError, "empty JSON-RPC message", nil)
	}
	if trimmed[0] == '[' {
		return Request{}, NewError(InvalidRequest, "JSON-RPC batches are not supported", nil)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &object); err != nil {
		return Request{}, NewError(ParseError, "invalid JSON", nil)
	}
	version, ok := object["jsonrpc"]
	if !ok || string(version) != `"2.0"` {
		return Request{}, NewError(InvalidRequest, "jsonrpc must be 2.0", nil)
	}
	methodValue, ok := object["method"]
	if !ok || json.Unmarshal(methodValue, new(string)) != nil {
		return Request{}, NewError(InvalidRequest, "method must be a string", nil)
	}
	var method string
	_ = json.Unmarshal(methodValue, &method)
	if method == "" {
		return Request{}, NewError(InvalidRequest, "method must not be empty", nil)
	}
	request := Request{Method: method}
	if params, ok := object["params"]; ok {
		if !json.Valid(params) {
			return Request{}, NewError(InvalidParams, "invalid params", nil)
		}
		request.Params = append(json.RawMessage(nil), params...)
	}
	if id, ok := object["id"]; ok {
		if !validID(id) {
			return Request{}, NewError(InvalidRequest, "id must be a string, number, or null", nil)
		}
		request.HasID = true
		request.ID = append(json.RawMessage(nil), id...)
	}
	return request, nil
}

func validID(value json.RawMessage) bool {
	if bytes.Equal(value, []byte("null")) {
		return true
	}
	var text string
	if json.Unmarshal(value, &text) == nil {
		return true
	}
	var number json.Number
	return json.Unmarshal(value, &number) == nil
}

// Framer reads and writes LSP Content-Length framed JSON. It is independent of
// request dispatch so the LSP core can also be hosted over HTTP.
type Framer struct {
	reader    *bufio.Reader
	writer    io.Writer
	maxLength int
}

func NewFramer(reader io.Reader, writer io.Writer) *Framer {
	return &Framer{reader: bufio.NewReader(reader), writer: writer, maxLength: DefaultMaxLength}
}

func (f *Framer) SetMaxLength(length int) {
	if length > 0 {
		f.maxLength = length
	}
}

func (f *Framer) Read() ([]byte, error) {
	length := -1
	for {
		line, err := f.reader.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) && line == "" {
				return nil, io.EOF
			}
			return nil, fmt.Errorf("jsonrpc2: read header: %w", err)
		}
		line = strings.TrimSuffix(line, "\n")
		line = strings.TrimSuffix(line, "\r")
		if line == "" {
			break
		}
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			return nil, errors.New("jsonrpc2: malformed header")
		}
		if strings.EqualFold(strings.TrimSpace(name), "Content-Length") {
			if length >= 0 {
				return nil, errors.New("jsonrpc2: duplicate Content-Length")
			}
			parsed, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil || parsed < 0 {
				return nil, errors.New("jsonrpc2: invalid Content-Length")
			}
			length = parsed
		}
	}
	if length < 0 {
		return nil, errors.New("jsonrpc2: missing Content-Length")
	}
	if length > f.maxLength {
		return nil, fmt.Errorf("jsonrpc2: Content-Length %d exceeds limit %d", length, f.maxLength)
	}
	data := make([]byte, length)
	if _, err := io.ReadFull(f.reader, data); err != nil {
		return nil, fmt.Errorf("jsonrpc2: read content: %w", err)
	}
	return data, nil
}

func (f *Framer) Write(data []byte) error {
	if len(data) > f.maxLength {
		return fmt.Errorf("jsonrpc2: content length %d exceeds limit %d", len(data), f.maxLength)
	}
	_, err := fmt.Fprintf(f.writer, "Content-Length: %d\r\n\r\n", len(data))
	if err != nil {
		return err
	}
	_, err = f.writer.Write(data)
	return err
}
