// Package lsp is a transport-agnostic Language Server Protocol adapter over a
// checker service.WorkspaceSession. It owns protocol translation, overlays,
// UTF-16 positions, debouncing, cancellation, and result-version publication;
// it contains no parser, resolver, checker, or diagnostic-rendering logic.
package lsp

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/wippyai/go-lua/analysis/check/service"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/embedding"
	"github.com/wippyai/go-lua/analysis/lsp/jsonrpc2"
)

const defaultDebounce = 200 * time.Millisecond

// Options configures orchestration concerns owned by the LSP server.
type Options struct {
	Debounce time.Duration
	Codec    DocumentCodec
	Resolver ModuleResolver
}

// NotificationFunc receives server-initiated JSON-RPC notifications. Each
// frontend installs one thin writer or queue; the server does not know its
// transport.
type NotificationFunc func(context.Context, string, any) error

type lifecycle uint8

const (
	lifecycleNew lifecycle = iota
	lifecycleRunning
	lifecycleShuttingDown
	lifecycleExited
)

type openDocument struct {
	uri      string
	document embedding.DocumentID
	unitID   embedding.UnitID
	version  int64
	buffer   textBuffer

	generation uint64
	timer      *time.Timer
	running    bool
	runningGen uint64
	cancel     context.CancelFunc
	ready      bool

	// complete holds the exact overlay that produced the latest completed
	// semantic result. Pull diagnostics use it rather than projecting a stale
	// source location onto a newer buffer.
	complete map[embedding.Digest]textBuffer
}

type solveWork struct {
	uri        string
	document   embedding.DocumentID
	unitID     embedding.UnitID
	version    int64
	generation uint64
	buffer     textBuffer
}

// Server is safe for concurrent protocol calls. Each document has one active
// solve at a time; newer edits cancel it and only the latest generation may
// publish results.
type Server struct {
	session  service.WorkspaceSession
	codec    DocumentCodec
	resolver ModuleResolver
	debounce time.Duration

	mu        sync.Mutex
	state     lifecycle
	documents map[embedding.DocumentID]*openDocument
	notify    NotificationFunc
	rootCtx   context.Context
	cancel    context.CancelFunc
}

func NewServer(session service.WorkspaceSession, options Options) *Server {
	if session == nil {
		panic("lsp: nil WorkspaceSession")
	}
	if options.Debounce < 0 {
		options.Debounce = 0
	}
	if options.Debounce == 0 {
		options.Debounce = defaultDebounce
	}
	if options.Codec == nil {
		options.Codec = FileDocumentCodec{}
	}
	if options.Resolver == nil {
		options.Resolver = FileConventionsResolver{}
	}
	rootCtx, cancel := context.WithCancel(context.Background())
	return &Server{
		session:   session,
		codec:     options.Codec,
		resolver:  options.Resolver,
		debounce:  options.Debounce,
		documents: make(map[embedding.DocumentID]*openDocument),
		rootCtx:   rootCtx,
		cancel:    cancel,
	}
}

// SetNotifier attaches a frontend notification sink. It is intentionally a
// transport concern and can be replaced by tests or a different frontend.
func (s *Server) SetNotifier(notify NotificationFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.notify = notify
}

func (s *Server) Exited() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state == lifecycleExited
}

// Handle dispatches one already-decoded JSON-RPC request. Notifications use
// the same handler and simply have their result suppressed by the frontend.
func (s *Server) Handle(ctx context.Context, request jsonrpc2.Request) (any, *jsonrpc2.Error) {
	var result any
	var err error
	switch request.Method {
	case methodInitialize:
		result, err = s.initialize()
	case methodInitialized:
		err = s.initialized()
	case methodShutdown:
		result, err = s.shutdown()
	case methodExit:
		err = s.exit()
	case methodDidOpen:
		err = s.didOpen(ctx, request.Params)
	case methodDidChange:
		err = s.didChange(ctx, request.Params)
	case methodDidClose:
		err = s.didClose(ctx, request.Params)
	case methodPullDiagnostics:
		result, err = s.pullDiagnostics(ctx, request.Params)
	case methodHover:
		result, err = s.hover(ctx, request.Params)
	default:
		return nil, jsonrpc2.NewError(jsonrpc2.MethodNotFound, "method not found", request.Method)
	}
	if err == nil {
		return result, nil
	}
	var protocol *jsonrpc2.Error
	if errors.As(err, &protocol) {
		return nil, protocol
	}
	return nil, jsonrpc2.NewError(jsonrpc2.InternalError, "internal LSP server error", err.Error())
}

func (s *Server) initialize() (any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state != lifecycleNew {
		return nil, jsonrpc2.NewError(jsonrpc2.InvalidRequest, "server is already initialized", nil)
	}
	s.state = lifecycleRunning
	return defaultInitializeResult(), nil
}

func (s *Server) initialized() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state != lifecycleRunning {
		return jsonrpc2.NewError(jsonrpc2.InvalidRequest, "initialized outside an active session", nil)
	}
	return nil
}

func (s *Server) shutdown() (any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state != lifecycleRunning {
		return nil, jsonrpc2.NewError(jsonrpc2.InvalidRequest, "shutdown outside an active session", nil)
	}
	s.state = lifecycleShuttingDown
	s.cancelSolvesLocked()
	return nil, nil
}

func (s *Server) exit() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state == lifecycleExited {
		return nil
	}
	s.state = lifecycleExited
	s.cancelSolvesLocked()
	s.cancel()
	return nil
}

func (s *Server) didOpen(ctx context.Context, raw []byte) error {
	var params didOpenParams
	if err := decodeParams(raw, &params); err != nil {
		return jsonrpc2.NewError(jsonrpc2.InvalidParams, "invalid didOpen params", err.Error())
	}
	if params.TextDocument.URI == "" {
		return jsonrpc2.NewError(jsonrpc2.InvalidParams, "didOpen requires textDocument.uri", nil)
	}
	document, err := s.codec.DocumentForURI(params.TextDocument.URI)
	if err != nil {
		return jsonrpc2.NewError(jsonrpc2.InvalidParams, "invalid document URI", err.Error())
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.requireRunningLocked(); err != nil {
		return err
	}
	if _, exists := s.documents[document]; exists {
		return jsonrpc2.NewError(jsonrpc2.InvalidParams, "document is already open", params.TextDocument.URI)
	}
	buffer := newTextBuffer([]byte(params.TextDocument.Text))
	unit, err := s.materializeAndUpsertLocked(ctx, document, params.TextDocument.Version, buffer)
	if err != nil {
		return err
	}
	doc := &openDocument{
		uri:      params.TextDocument.URI,
		document: document,
		unitID:   unit.ID,
		version:  params.TextDocument.Version,
		buffer:   buffer,
		complete: make(map[embedding.Digest]textBuffer),
	}
	s.documents[document] = doc
	s.scheduleLocked(doc)
	return nil
}

func (s *Server) didChange(ctx context.Context, raw []byte) error {
	var params didChangeParams
	if err := decodeParams(raw, &params); err != nil {
		return jsonrpc2.NewError(jsonrpc2.InvalidParams, "invalid didChange params", err.Error())
	}
	document, err := s.codec.DocumentForURI(params.TextDocument.URI)
	if err != nil {
		return jsonrpc2.NewError(jsonrpc2.InvalidParams, "invalid document URI", err.Error())
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.requireRunningLocked(); err != nil {
		return err
	}
	doc, exists := s.documents[document]
	if !exists {
		return jsonrpc2.NewError(jsonrpc2.InvalidParams, "didChange for unopened document", params.TextDocument.URI)
	}
	if params.TextDocument.Version <= doc.version {
		return jsonrpc2.NewError(jsonrpc2.InvalidParams, "document versions must increase", params.TextDocument.Version)
	}
	next := newTextBuffer(doc.buffer.bytes())
	if err := next.apply(params.ContentChanges); err != nil {
		return jsonrpc2.NewError(jsonrpc2.InvalidParams, "invalid incremental change", err.Error())
	}
	unit, err := s.materializeAndUpsertLocked(ctx, document, params.TextDocument.Version, next)
	if err != nil {
		return err
	}
	doc.version = params.TextDocument.Version
	doc.buffer = next
	doc.unitID = unit.ID
	s.scheduleLocked(doc)
	return nil
}

func (s *Server) didClose(ctx context.Context, raw []byte) error {
	var params didCloseParams
	if err := decodeParams(raw, &params); err != nil {
		return jsonrpc2.NewError(jsonrpc2.InvalidParams, "invalid didClose params", err.Error())
	}
	document, err := s.codec.DocumentForURI(params.TextDocument.URI)
	if err != nil {
		return jsonrpc2.NewError(jsonrpc2.InvalidParams, "invalid document URI", err.Error())
	}
	s.mu.Lock()
	if err := s.requireRunningLocked(); err != nil {
		s.mu.Unlock()
		return err
	}
	doc, exists := s.documents[document]
	if !exists {
		s.mu.Unlock()
		return nil // LSP close is safely idempotent for a previously closed file.
	}
	s.stopDocumentLocked(doc)
	unitID, uri, version := doc.unitID, doc.uri, doc.version
	if err := s.session.RemoveUnit(ctx, unitID); err != nil {
		s.mu.Unlock()
		return jsonrpc2.NewError(jsonrpc2.InternalError, "remove closed unit", err.Error())
	}
	// Keep close and remove serialized with didOpen. If removal happened after
	// releasing this lock, a concurrent reopen of the same DocumentID could
	// upsert a new unit and then have that new unit removed by this close.
	delete(s.documents, document)
	s.mu.Unlock()
	s.publish(context.Background(), methodPublishDiagnostics, publishDiagnosticsParams{URI: uri, Version: version, Diagnostics: []protocolDiagnostic{}})
	return nil
}

func (s *Server) pullDiagnostics(ctx context.Context, raw []byte) (any, error) {
	var params pullDiagnosticsParams
	if err := decodeParams(raw, &params); err != nil {
		return nil, jsonrpc2.NewError(jsonrpc2.InvalidParams, "invalid diagnostic params", err.Error())
	}
	document, err := s.codec.DocumentForURI(params.TextDocument.URI)
	if err != nil {
		return nil, jsonrpc2.NewError(jsonrpc2.InvalidParams, "invalid document URI", err.Error())
	}
	s.mu.Lock()
	if err := s.requireRunningLocked(); err != nil {
		s.mu.Unlock()
		return nil, err
	}
	doc, exists := s.documents[document]
	if !exists {
		s.mu.Unlock()
		return diagnosticReport{Kind: "full", Items: []protocolDiagnostic{}}, nil
	}
	unitID := doc.unitID
	s.mu.Unlock()

	completed, ok := s.session.LastComplete(ctx, service.ResultRequest{Selector: service.ResultSelector{UnitID: unitID}})
	if !ok {
		return diagnosticReport{Kind: "full", Items: []protocolDiagnostic{}}, nil
	}
	tag := completed.Tag()
	s.mu.Lock()
	doc, exists = s.documents[document]
	if !exists || doc.unitID != unitID {
		s.mu.Unlock()
		return diagnosticReport{Kind: "full", Items: []protocolDiagnostic{}}, nil
	}
	digest, ok := tag.SourceDigests[document]
	buffer, bufferOK := doc.complete[digest]
	s.mu.Unlock()
	if !ok || !bufferOK {
		return diagnosticReport{Kind: "full", ResultID: resultID(tag), Items: []protocolDiagnostic{}}, nil
	}
	response, err := s.session.Diagnostics(ctx, service.ListDiagnosticsRequest{Selector: service.ResultSelector{UnitID: unitID, SolveSeq: tag.SolveSeq, Profile: tag.Profile}})
	if err != nil {
		return nil, jsonrpc2.NewError(jsonrpc2.InternalError, "read completed diagnostics", err.Error())
	}
	return diagnosticReport{Kind: "full", ResultID: resultID(tag), Items: s.mapDiagnostics(document, buffer, tag, response.Rendered)}, nil
}

func (s *Server) hover(ctx context.Context, raw []byte) (any, error) {
	var params hoverParams
	if err := decodeParams(raw, &params); err != nil {
		return nil, jsonrpc2.NewError(jsonrpc2.InvalidParams, "invalid hover params", err.Error())
	}
	document, err := s.codec.DocumentForURI(params.TextDocument.URI)
	if err != nil {
		return nil, jsonrpc2.NewError(jsonrpc2.InvalidParams, "invalid document URI", err.Error())
	}
	s.mu.Lock()
	if err := s.requireRunningLocked(); err != nil {
		s.mu.Unlock()
		return nil, err
	}
	doc, exists := s.documents[document]
	if !exists {
		s.mu.Unlock()
		return nil, nil
	}
	offset, err := doc.buffer.offsetForPosition(params.Position)
	if err != nil {
		s.mu.Unlock()
		return nil, jsonrpc2.NewError(jsonrpc2.InvalidParams, "invalid hover position", err.Error())
	}
	unitID, version, digest := doc.unitID, doc.version, doc.buffer.digest()
	s.mu.Unlock()

	completed, ok := s.session.LastComplete(ctx, service.ResultRequest{Selector: service.ResultSelector{UnitID: unitID}})
	if !ok {
		return nil, nil
	}
	tag := completed.Tag()
	if tag.DocumentVersion != version || tag.SourceDigests[document] != digest {
		return nil, nil // never map an older semantic fact onto a newer overlay.
	}
	lookup, err := s.session.PositionLookup(ctx, service.PositionLookupRequest{
		Selector: service.ResultSelector{UnitID: unitID, SolveSeq: tag.SolveSeq, Profile: tag.Profile},
		File:     document.OpaqueKey,
		Position: service.SourcePosition{Offset: offset},
	})
	if err != nil || !lookup.Found {
		return nil, nil
	}
	value := ""
	var resultRange *Range
	if lookup.Expression != nil {
		value = "```lua\n" + lookup.Expression.Display + "\n```"
		s.mu.Lock()
		current := s.documents[document]
		if current != nil {
			mapped, mapErr := current.buffer.rangeForCompilerSpan(
				lookup.Expression.Location.Span.StartLine,
				lookup.Expression.Location.Span.StartCol,
				lookup.Expression.Location.Span.EndLine,
				lookup.Expression.Location.Span.EndCol,
			)
			if mapErr == nil {
				resultRange = &mapped
			}
		}
		s.mu.Unlock()
	} else if lookup.Binder != nil {
		value = "```lua\n" + lookup.Binder.Name + "\n```"
	}
	if value == "" {
		return nil, nil
	}
	return hoverResult{Contents: markupContent{Kind: "markdown", Value: value}, Range: resultRange}, nil
}

func (s *Server) requireRunningLocked() error {
	if s.state != lifecycleRunning {
		return jsonrpc2.NewError(jsonrpc2.InvalidRequest, "server is not accepting document requests", nil)
	}
	return nil
}

func (s *Server) materializeAndUpsertLocked(ctx context.Context, document embedding.DocumentID, version int64, buffer textBuffer) (service.UnitInput, error) {
	snapshot := embedding.SourceSnapshot{
		Document:         document,
		ProviderRevision: fmt.Sprintf("lsp:%d", version),
		ContentDigest:    buffer.digest(),
		Content:          buffer.bytes(),
	}
	unit, err := s.resolver.Resolve(ctx, MaterializedDocument{Document: document, Version: version, Snapshot: snapshot})
	if err != nil {
		return service.UnitInput{}, jsonrpc2.NewError(jsonrpc2.InternalError, "materialize document unit", err.Error())
	}
	if unit.ID == "" || unit.EntryDocument != document || unit.DocumentVersion != version {
		return service.UnitInput{}, jsonrpc2.NewError(jsonrpc2.InternalError, "resolver returned an incomplete materialized unit", nil)
	}
	if _, err := s.session.UpsertUnit(ctx, unit); err != nil {
		return service.UnitInput{}, jsonrpc2.NewError(jsonrpc2.InternalError, "upsert materialized unit", err.Error())
	}
	return unit, nil
}

func (s *Server) scheduleLocked(doc *openDocument) {
	doc.generation++
	// A debounce timer for an older generation may already have marked a
	// running solve ready. A newer edit receives its own full debounce window.
	doc.ready = false
	if doc.cancel != nil {
		doc.cancel()
	}
	if doc.timer != nil {
		doc.timer.Stop()
	}
	generation := doc.generation
	document := doc.document
	doc.timer = time.AfterFunc(s.debounce, func() { s.fire(document, generation) })
}

func (s *Server) fire(document embedding.DocumentID, generation uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	doc, exists := s.documents[document]
	if !exists || doc.generation != generation || s.state != lifecycleRunning {
		return
	}
	doc.timer = nil
	if doc.running {
		doc.ready = true
		return
	}
	s.startSolveLocked(doc)
}

func (s *Server) startSolveLocked(doc *openDocument) {
	ctx, cancel := context.WithCancel(s.rootCtx)
	doc.running = true
	doc.runningGen = doc.generation
	doc.cancel = cancel
	work := solveWork{
		uri:        doc.uri,
		document:   doc.document,
		unitID:     doc.unitID,
		version:    doc.version,
		generation: doc.generation,
		buffer:     newTextBuffer(doc.buffer.bytes()),
	}
	go s.solve(ctx, work)
}

func (s *Server) solve(ctx context.Context, work solveWork) {
	tag, err := s.session.EnsureSolved(ctx, service.SolveRequest{
		UnitID:          work.unitID,
		Trigger:         service.TriggerKeystroke,
		Freshness:       service.FreshnessRequireNew,
		DocumentVersion: work.version,
	})
	if err == nil && ctx.Err() == nil {
		s.mu.Lock()
		current := s.documents[work.document]
		currentEnough := current != nil && current.generation == work.generation && current.version == work.version && current.unitID == work.unitID && tag.DocumentVersion == work.version
		s.mu.Unlock()
		if currentEnough {
			response, readErr := s.session.Diagnostics(context.Background(), service.ListDiagnosticsRequest{
				Selector: service.ResultSelector{UnitID: work.unitID, SolveSeq: tag.SolveSeq, Profile: tag.Profile},
			})
			if readErr == nil {
				s.publishCompletedDiagnostics(work, tag, response.Rendered)
			}
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	doc := s.documents[work.document]
	if doc == nil || doc.runningGen != work.generation {
		return
	}
	doc.running = false
	doc.cancel = nil
	if doc.ready && s.state == lifecycleRunning {
		doc.ready = false
		s.startSolveLocked(doc)
	}
}

// publishCompletedDiagnostics makes the current-generation test and the
// notification one critical section. This prevents an old solve from racing a
// newly accepted didChange and emitting an untagged/stale publication after it.
func (s *Server) publishCompletedDiagnostics(work solveWork, tag service.ResultTag, items []diagnostic.Diagnostic) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current := s.documents[work.document]
	if s.state != lifecycleRunning || current == nil || current.generation != work.generation || current.version != work.version || current.unitID != work.unitID {
		return
	}
	current.complete = map[embedding.Digest]textBuffer{work.buffer.digest(): work.buffer}
	notify := s.notify
	if notify == nil {
		return
	}
	diagnostics := s.mapDiagnostics(work.document, work.buffer, tag, items)
	_ = notify(context.Background(), methodPublishDiagnostics, publishDiagnosticsParams{URI: work.uri, Version: work.version, Diagnostics: diagnostics})
}

func (s *Server) stopDocumentLocked(doc *openDocument) {
	if doc.timer != nil {
		doc.timer.Stop()
		doc.timer = nil
	}
	if doc.cancel != nil {
		doc.cancel()
	}
}

func (s *Server) cancelSolvesLocked() {
	for _, doc := range s.documents {
		s.stopDocumentLocked(doc)
	}
}

func (s *Server) publish(ctx context.Context, method string, params any) {
	s.mu.Lock()
	notify := s.notify
	s.mu.Unlock()
	if notify != nil {
		_ = notify(ctx, method, params)
	}
}

func (s *Server) mapDiagnostics(document embedding.DocumentID, buffer textBuffer, tag service.ResultTag, items []diagnostic.Diagnostic) []protocolDiagnostic {
	digest, ok := tag.SourceDigests[document]
	if !ok || digest != buffer.digest() {
		return []protocolDiagnostic{}
	}
	result := make([]protocolDiagnostic, 0, len(items))
	for _, item := range items {
		if item.Location.Document != document || item.Location.ContentDigest != digest {
			continue
		}
		mapped, err := buffer.rangeForSpan(item.Location.Span)
		if err != nil {
			continue
		}
		diagnostic := protocolDiagnostic{
			Range:    mapped,
			Severity: protocolSeverity(item.Severity),
			Code:     item.Code.String(),
			Source:   "go-lua",
			Message:  item.Message,
			Data: map[string]any{
				"goLua": map[string]any{
					"solveSeq":        uint64(tag.SolveSeq),
					"documentVersion": tag.DocumentVersion,
					"sourceDigest":    digest.String(),
				},
			},
		}
		for _, label := range item.Labels {
			if label.Location.Document != document || label.Location.ContentDigest != digest {
				continue
			}
			labelRange, err := buffer.rangeForSpan(label.Location.Span)
			if err == nil {
				diagnostic.RelatedInformation = append(diagnostic.RelatedInformation, diagnosticRelatedInformation{
					Location: Location{URI: s.uriFor(document), Range: labelRange},
					Message:  label.Message,
				})
			}
		}
		for _, evidence := range item.Explanation.Evidence() {
			if !evidence.Span.Valid() || (evidence.File != "" && evidence.File != item.Position.File && evidence.File != document.OpaqueKey) {
				continue
			}
			evidenceRange, err := buffer.rangeForCompilerSpan(evidence.Span.StartLine, evidence.Span.StartCol, evidence.Span.EndLine, evidence.Span.EndCol)
			if err != nil {
				continue
			}
			message := evidence.Message
			if message == "" {
				message = evidence.Kind.String()
			}
			diagnostic.RelatedInformation = append(diagnostic.RelatedInformation, diagnosticRelatedInformation{
				Location: Location{URI: s.uriFor(document), Range: evidenceRange},
				Message:  message,
			})
		}
		result = append(result, diagnostic)
	}
	return result
}

func (s *Server) uriFor(document embedding.DocumentID) string {
	uri, err := s.codec.URIForDocument(document)
	if err != nil {
		return ""
	}
	return uri
}

func protocolSeverity(severity diagnostic.Severity) int {
	switch severity {
	case diagnostic.SeverityError:
		return 1
	case diagnostic.SeverityWarning:
		return 2
	case diagnostic.SeverityHint:
		return 4
	default:
		return 3
	}
}

func resultID(tag service.ResultTag) string {
	return fmt.Sprintf("go-lua:%d:%d:%s", tag.SolveSeq, tag.DocumentVersion, tag.UnitDigest.String())
}
