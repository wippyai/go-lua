// Package lsp is a transport-agnostic Language Server Protocol adapter over a
// checker service.WorkspaceSession. It owns protocol translation, overlays,
// UTF-16 positions, debouncing, cancellation, and result-version publication;
// it contains no parser, resolver, checker, or diagnostic-rendering logic.
package lsp

import (
	"context"
	"errors"
	"fmt"
	"strings"
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

type semanticDocument struct {
	uri      string
	document embedding.DocumentID
	unitID   embedding.UnitID
	version  int64
	buffer   textBuffer
	tag      service.ResultTag
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
	case methodDefinition:
		result, err = s.definition(ctx, request.Params)
	case methodReferences:
		result, err = s.references(ctx, request.Params)
	case methodDocumentHighlight:
		result, err = s.documentHighlight(ctx, request.Params)
	case methodDocumentSymbol:
		result, err = s.documentSymbols(ctx, request.Params)
	case methodPrepareRename:
		result, err = s.prepareRename(ctx, request.Params)
	case methodRename:
		result, err = s.rename(ctx, request.Params)
	case methodCodeAction:
		result, err = s.codeActions(ctx, request.Params)
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
	var markdown strings.Builder
	var resultRange *Range
	if lookup.Expression != nil {
		markdown.WriteString("```lua\n")
		markdown.WriteString(lookup.Expression.Display)
		markdown.WriteString("\n```")
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
	}

	selector := service.ResultSelector{UnitID: unitID, SolveSeq: tag.SolveSeq, Profile: tag.Profile}
	for _, anchor := range lookup.SubjectAnchors {
		judgments, err := s.session.JudgmentsByAnchor(ctx, service.JudgmentsByAnchorRequest{
			Selector:  selector,
			AnchorKey: anchor.StableKey(),
		})
		if err != nil {
			return nil, fmt.Errorf("read hover judgment evidence: %w", err)
		}
		for _, presentation := range judgments.Presentations {
			if markdown.Len() != 0 {
				markdown.WriteString("\n\n")
			}
			markdown.WriteString("**")
			markdown.WriteString(string(presentation.Code))
			markdown.WriteString(" — ")
			verdict := "unknown"
			switch presentation.Verdict {
			case 1:
				verdict = "proven"
			case 2:
				verdict = "refuted"
			}
			markdown.WriteString(verdict)
			markdown.WriteString("**")
			for _, line := range diagnostic.EvidenceTrace(presentation.Evidence) {
				markdown.WriteString("\n- ")
				markdown.WriteString(line.Heading)
				markdown.WriteString(": ")
				markdown.WriteString(line.Message)
			}
		}
	}
	if markdown.Len() == 0 {
		return nil, nil
	}
	return hoverResult{Contents: markupContent{Kind: "markdown", Value: markdown.String()}, Range: resultRange}, nil
}

func (s *Server) definition(ctx context.Context, raw []byte) (any, error) {
	var params textDocumentPositionParams
	if err := decodeParams(raw, &params); err != nil {
		return nil, jsonrpc2.NewError(jsonrpc2.InvalidParams, "invalid definition params", err.Error())
	}
	document, binder, err := s.binderAtPosition(ctx, params.TextDocument.URI, params.Position)
	if err != nil || binder == nil {
		return nil, err
	}
	location, ok := document.protocolLocation(binder.Definition)
	if !ok {
		return nil, nil
	}
	return &location, nil
}

func (s *Server) references(ctx context.Context, raw []byte) (any, error) {
	var params referencesParams
	if err := decodeParams(raw, &params); err != nil {
		return nil, jsonrpc2.NewError(jsonrpc2.InvalidParams, "invalid references params", err.Error())
	}
	document, binder, err := s.binderAtPosition(ctx, params.TextDocument.URI, params.Position)
	if err != nil || binder == nil {
		return nil, err
	}
	locations := make([]Location, 0, len(binder.Occurrences)+1)
	if params.Context.IncludeDeclaration {
		if location, ok := document.protocolLocation(binder.Definition); ok {
			locations = append(locations, location)
		}
	}
	for _, occurrence := range binder.Occurrences {
		if location, ok := document.protocolLocation(occurrence.Location); ok {
			locations = append(locations, location)
		}
	}
	return locations, nil
}

func (s *Server) documentHighlight(ctx context.Context, raw []byte) (any, error) {
	var params textDocumentPositionParams
	if err := decodeParams(raw, &params); err != nil {
		return nil, jsonrpc2.NewError(jsonrpc2.InvalidParams, "invalid documentHighlight params", err.Error())
	}
	document, binder, err := s.binderAtPosition(ctx, params.TextDocument.URI, params.Position)
	if err != nil || binder == nil {
		return nil, err
	}
	highlights := make([]documentHighlight, 0, len(binder.Occurrences)+1)
	if location, ok := document.protocolLocation(binder.Definition); ok {
		highlights = append(highlights, documentHighlight{Range: location.Range, Kind: 3}) // Write
	}
	for _, occurrence := range binder.Occurrences {
		location, ok := document.protocolLocation(occurrence.Location)
		if !ok {
			continue
		}
		highlights = append(highlights, documentHighlight{Range: location.Range, Kind: highlightKind(occurrence.Role)})
	}
	return highlights, nil
}

func (s *Server) documentSymbols(ctx context.Context, raw []byte) (any, error) {
	var params documentSymbolParams
	if err := decodeParams(raw, &params); err != nil {
		return nil, jsonrpc2.NewError(jsonrpc2.InvalidParams, "invalid documentSymbol params", err.Error())
	}
	document, err := s.codec.DocumentForURI(params.TextDocument.URI)
	if err != nil {
		return nil, jsonrpc2.NewError(jsonrpc2.InvalidParams, "invalid document URI", err.Error())
	}
	current, ok, err := s.currentSemanticDocument(ctx, document)
	if err != nil || !ok {
		return nil, err
	}
	response, err := s.session.DocumentSymbols(ctx, service.DocumentSymbolsRequest{
		Selector: service.ResultSelector{UnitID: current.unitID, SolveSeq: current.tag.SolveSeq, Profile: current.tag.Profile},
		File:     document.OpaqueKey,
	})
	if err != nil {
		return nil, fmt.Errorf("read document symbols: %w", err)
	}
	items := make([]documentSymbol, 0, len(response.Symbols))
	for _, item := range response.Symbols {
		if symbol, ok := current.protocolDocumentSymbol(item); ok {
			items = append(items, symbol)
		}
	}
	return items, nil
}

func (s *Server) prepareRename(ctx context.Context, raw []byte) (any, error) {
	var params textDocumentPositionParams
	if err := decodeParams(raw, &params); err != nil {
		return nil, jsonrpc2.NewError(jsonrpc2.InvalidParams, "invalid prepareRename params", err.Error())
	}
	document, binder, err := s.renameBinderAtPosition(ctx, params.TextDocument.URI, params.Position)
	if err != nil {
		return nil, err
	}
	selectedRange, ok := document.binderRangeAt(binder, params.Position)
	if !ok {
		return nil, jsonrpc2.NewError(jsonrpc2.InvalidRequest, "rename cannot establish the selected binder range", nil)
	}
	return selectedRange, nil
}

func (s *Server) rename(ctx context.Context, raw []byte) (any, error) {
	var params renameParams
	if err := decodeParams(raw, &params); err != nil {
		return nil, jsonrpc2.NewError(jsonrpc2.InvalidParams, "invalid rename params", err.Error())
	}
	if !validLuaIdentifier(params.NewName) {
		return nil, jsonrpc2.NewError(jsonrpc2.InvalidRequest, "rename requires a valid Lua identifier", nil)
	}
	document, binder, err := s.renameBinderAtPosition(ctx, params.TextDocument.URI, params.Position)
	if err != nil {
		return nil, err
	}
	if params.NewName == binder.Name {
		return workspaceEdit{Changes: map[string][]workspaceTextEdit{}}, nil
	}
	all, err := s.session.BinderOccurrences(ctx, service.BinderOccurrencesRequest{
		Selector: service.ResultSelector{UnitID: document.unitID, SolveSeq: document.tag.SolveSeq, Profile: document.tag.Profile},
	})
	if err != nil {
		return nil, fmt.Errorf("read rename binder occurrences: %w", err)
	}
	if err := document.renameCollision(binder, params.NewName, all.Binders); err != nil {
		return nil, jsonrpc2.NewError(jsonrpc2.InvalidRequest, err.Error(), nil)
	}
	edits := make([]workspaceTextEdit, 0, len(binder.Occurrences)+1)
	for _, location := range binderLocations(*binder) {
		editRange, ok := document.protocolRange(location)
		if !ok {
			return nil, jsonrpc2.NewError(jsonrpc2.InvalidRequest, "rename cannot prove a complete module-local occurrence set", nil)
		}
		edits = append(edits, workspaceTextEdit{Range: editRange, NewText: params.NewName})
	}
	return workspaceEdit{Changes: map[string][]workspaceTextEdit{document.uri: edits}}, nil
}

func (s *Server) codeActions(ctx context.Context, raw []byte) (any, error) {
	var params codeActionParams
	if err := decodeParams(raw, &params); err != nil {
		return nil, jsonrpc2.NewError(jsonrpc2.InvalidParams, "invalid codeAction params", err.Error())
	}
	if len(params.Context.Only) != 0 && !containsCodeActionKind(params.Context.Only, "quickfix") {
		return []codeAction{}, nil
	}
	documentID, err := s.codec.DocumentForURI(params.TextDocument.URI)
	if err != nil {
		return nil, jsonrpc2.NewError(jsonrpc2.InvalidParams, "invalid document URI", err.Error())
	}
	document, ok, err := s.currentSemanticDocument(ctx, documentID)
	if err != nil || !ok {
		return nil, err
	}
	repairs, err := s.session.RepairActions(ctx, service.RepairActionsRequest{
		Selector: service.ResultSelector{UnitID: document.unitID, SolveSeq: document.tag.SolveSeq, Profile: document.tag.Profile},
	})
	if err != nil {
		return nil, fmt.Errorf("read repair actions: %w", err)
	}
	actions := make([]codeAction, 0, len(repairs.Actions))
	for _, repair := range repairs.Actions {
		targetRange, targetOK := document.protocolRange(repair.Target)
		if !targetOK || !rangesOverlap(targetRange, params.Range) || len(repair.Payload.Edits) == 0 {
			continue
		}
		edits := make([]workspaceTextEdit, 0, len(repair.Payload.Edits))
		complete := true
		for _, repairEdit := range repair.Payload.Edits {
			mappedRange, mapped := document.protocolRange(repairEdit.Target)
			if !mapped {
				complete = false
				break
			}
			edits = append(edits, workspaceTextEdit{Range: mappedRange, NewText: repairEdit.NewText})
		}
		if !complete {
			continue
		}
		actions = append(actions, codeAction{
			Title: repairTitle(string(repair.Kind)),
			Kind:  "quickfix",
			Edit:  workspaceEdit{Changes: map[string][]workspaceTextEdit{document.uri: edits}},
		})
	}
	return actions, nil
}

func (s *Server) renameBinderAtPosition(ctx context.Context, uri string, position Position) (semanticDocument, *service.BinderInfo, error) {
	document, binder, err := s.binderAtPosition(ctx, uri, position)
	if err != nil {
		return semanticDocument{}, nil, err
	}
	if binder == nil {
		return semanticDocument{}, nil, jsonrpc2.NewError(jsonrpc2.InvalidRequest, "rename requires a lexical binder occurrence", nil)
	}
	if !binder.ModuleLocal {
		return semanticDocument{}, nil, jsonrpc2.NewError(jsonrpc2.InvalidRequest, "rename rejects cross-module exported/global binders", nil)
	}
	if !binder.Scope.Valid() {
		return semanticDocument{}, nil, jsonrpc2.NewError(jsonrpc2.InvalidRequest, "rename cannot prove the binder's lexical scope", nil)
	}
	return document, binder, nil
}

func (s *Server) binderAtPosition(ctx context.Context, uri string, position Position) (semanticDocument, *service.BinderInfo, error) {
	document, err := s.codec.DocumentForURI(uri)
	if err != nil {
		return semanticDocument{}, nil, jsonrpc2.NewError(jsonrpc2.InvalidParams, "invalid document URI", err.Error())
	}
	current, ok, err := s.currentSemanticDocument(ctx, document)
	if err != nil || !ok {
		return semanticDocument{}, nil, err
	}
	offset, err := current.buffer.offsetForPosition(position)
	if err != nil {
		return semanticDocument{}, nil, jsonrpc2.NewError(jsonrpc2.InvalidParams, "invalid document position", err.Error())
	}
	lookup, err := s.session.PositionLookup(ctx, service.PositionLookupRequest{
		Selector: service.ResultSelector{UnitID: current.unitID, SolveSeq: current.tag.SolveSeq, Profile: current.tag.Profile},
		File:     document.OpaqueKey,
		Position: service.SourcePosition{Offset: offset},
	})
	if err != nil {
		return semanticDocument{}, nil, fmt.Errorf("read position lookup: %w", err)
	}
	if !lookup.Found || lookup.Binder == nil {
		return semanticDocument{}, nil, nil
	}
	occurrences, err := s.session.BinderOccurrences(ctx, service.BinderOccurrencesRequest{
		Selector: service.ResultSelector{UnitID: current.unitID, SolveSeq: current.tag.SolveSeq, Profile: current.tag.Profile},
	})
	if err != nil {
		return semanticDocument{}, nil, fmt.Errorf("read binder occurrences: %w", err)
	}
	for index := range occurrences.Binders {
		if occurrences.Binders[index].SymbolID == lookup.Binder.SymbolID {
			return current, &occurrences.Binders[index], nil
		}
	}
	return semanticDocument{}, nil, nil
}

func (s *Server) currentSemanticDocument(ctx context.Context, document embedding.DocumentID) (semanticDocument, bool, error) {
	s.mu.Lock()
	if err := s.requireRunningLocked(); err != nil {
		s.mu.Unlock()
		return semanticDocument{}, false, err
	}
	doc := s.documents[document]
	if doc == nil {
		s.mu.Unlock()
		return semanticDocument{}, false, nil
	}
	current := semanticDocument{
		uri:      doc.uri,
		document: document,
		unitID:   doc.unitID,
		version:  doc.version,
		buffer:   newTextBuffer(doc.buffer.bytes()),
	}
	s.mu.Unlock()

	completed, ok := s.session.LastComplete(ctx, service.ResultRequest{Selector: service.ResultSelector{UnitID: current.unitID}})
	if !ok {
		return semanticDocument{}, false, nil
	}
	current.tag = completed.Tag()
	if current.tag.DocumentVersion != current.version || current.tag.SourceDigests[document] != current.buffer.digest() {
		return semanticDocument{}, false, nil
	}
	return current, true, nil
}

func (d semanticDocument) protocolLocation(location service.SourceLocation) (Location, bool) {
	mappedRange, ok := d.protocolRange(location)
	if !ok {
		return Location{}, false
	}
	return Location{URI: d.uri, Range: mappedRange}, true
}

func (d semanticDocument) protocolRange(location service.SourceLocation) (Range, bool) {
	if location.File != d.document.OpaqueKey {
		return Range{}, false
	}
	item, err := d.buffer.rangeForCompilerSpan(
		location.Span.StartLine,
		location.Span.StartCol,
		location.Span.EndLine,
		location.Span.EndCol,
	)
	if err != nil {
		return Range{}, false
	}
	return item, true
}

func (d semanticDocument) protocolDocumentSymbol(item service.DocumentSymbol) (documentSymbol, bool) {
	location, ok := d.protocolLocation(item.Location)
	if !ok {
		return documentSymbol{}, false
	}
	result := documentSymbol{
		Name:           item.Name,
		Kind:           protocolDocumentSymbolKind(item.Kind),
		Range:          location.Range,
		SelectionRange: location.Range,
	}
	for _, child := range item.Children {
		if mapped, ok := d.protocolDocumentSymbol(child); ok {
			result.Children = append(result.Children, mapped)
		}
	}
	return result, true
}

func protocolDocumentSymbolKind(kind service.DocumentSymbolKind) int {
	if kind == service.DocumentSymbolFunction {
		return 12 // Function
	}
	return 8 // Field
}

func highlightKind(role service.BinderOccurrenceRole) int {
	if role == service.BinderWrite {
		return 3 // Write
	}
	if role == service.BinderRead || role == service.BinderCapture {
		return 2 // Read
	}
	return 1 // Text
}

func (d semanticDocument) binderRangeAt(binder *service.BinderInfo, position Position) (Range, bool) {
	if binder == nil {
		return Range{}, false
	}
	for _, location := range binderLocations(*binder) {
		mappedRange, ok := d.protocolRange(location)
		if ok && rangeContainsPosition(mappedRange, position) {
			return mappedRange, true
		}
	}
	return Range{}, false
}

func (d semanticDocument) renameCollision(target *service.BinderInfo, newName string, candidates []service.BinderInfo) error {
	for _, candidate := range candidates {
		if candidate.SymbolID == target.SymbolID || candidate.Name != newName {
			continue
		}
		if !candidate.Scope.Valid() {
			return fmt.Errorf("rename cannot prove that %q does not capture an unavailable binder", newName)
		}
		for _, targetLocation := range binderLocations(*target) {
			contains, known := d.scopeContains(candidate.Scope, targetLocation)
			if !known {
				return fmt.Errorf("rename cannot prove that %q does not capture across a document boundary", newName)
			}
			if contains {
				return fmt.Errorf("rename would capture or shadow the existing binder %q", newName)
			}
		}
	}
	return nil
}

func (d semanticDocument) scopeContains(scope, location service.SourceLocation) (bool, bool) {
	scopeRange, ok := d.protocolRange(scope)
	if !ok {
		return false, false
	}
	locationRange, ok := d.protocolRange(location)
	if !ok {
		return false, false
	}
	return positionBeforeOrEqual(scopeRange.Start, locationRange.Start) && positionBeforeOrEqual(locationRange.End, scopeRange.End), true
}

func binderLocations(binder service.BinderInfo) []service.SourceLocation {
	locations := make([]service.SourceLocation, 0, len(binder.Occurrences)+1)
	if binder.Definition.Valid() {
		locations = append(locations, binder.Definition)
	}
	for _, occurrence := range binder.Occurrences {
		locations = append(locations, occurrence.Location)
	}
	return locations
}

func rangeContainsPosition(item Range, position Position) bool {
	return positionBeforeOrEqual(item.Start, position) && positionBeforeOrEqual(position, item.End)
}

func rangesOverlap(left, right Range) bool {
	return positionBeforeOrEqual(left.Start, right.End) && positionBeforeOrEqual(right.Start, left.End)
}

func containsCodeActionKind(items []string, wanted string) bool {
	for _, item := range items {
		if item == wanted || strings.HasPrefix(item, wanted+".") {
			return true
		}
	}
	return false
}

func repairTitle(kind string) string {
	switch kind {
	case "remove_redundant_claim":
		return "Remove redundant type claim"
	case "remove_redundant_guard":
		return "Remove redundant guard"
	case "hoist_invariant_read":
		return "Hoist invariant read"
	case "initialize_discriminant":
		return "Initialize discriminant"
	case "add_nil_guard":
		return "Add nil guard"
	case "add_annotation":
		return "Add type annotation"
	default:
		return "Apply checker repair"
	}
}

func positionBeforeOrEqual(left, right Position) bool {
	return left.Line < right.Line || left.Line == right.Line && left.Character <= right.Character
}

func validLuaIdentifier(name string) bool {
	if name == "" || luaKeywords[name] {
		return false
	}
	for index := range name {
		value := name[index]
		if index == 0 {
			if value != '_' && (value < 'A' || value > 'Z') && (value < 'a' || value > 'z') {
				return false
			}
			continue
		}
		if value != '_' && (value < 'A' || value > 'Z') && (value < 'a' || value > 'z') && (value < '0' || value > '9') {
			return false
		}
	}
	return true
}

var luaKeywords = map[string]bool{
	"and": true, "break": true, "do": true, "else": true, "elseif": true, "end": true,
	"false": true, "for": true, "function": true, "goto": true, "if": true, "in": true,
	"local": true, "nil": true, "not": true, "or": true, "repeat": true, "return": true,
	"then": true, "true": true, "until": true, "while": true,
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
