package service

import "github.com/wippyai/go-lua/analysis/check/judgment"

// EmbeddingSchemaVersion pins the versioned, renderer-independent checker
// embedding surface. It is deliberately separate from JIRSchemaVersion: JIR
// pins judgments, while this pin covers navigation and repair DTOs.
const EmbeddingSchemaVersion = 2

// SourceSpan is the current source-location value used by the embedding
// surface. It is intentionally isolated from identity/display policy so a
// DocumentID-backed location can replace it mechanically in a later lane.
// Coordinates are one-indexed and both span ends are inclusive.
type SourceSpan struct {
	StartLine int
	StartCol  int
	EndLine   int
	EndCol    int
}

func (s SourceSpan) Valid() bool { return s.StartLine > 0 && s.StartCol > 0 }

// SourceLocation identifies a location in today's file-based service input.
type SourceLocation struct {
	File string
	Span SourceSpan
}

func (l SourceLocation) Valid() bool { return l.File != "" && l.Span.Valid() }

// BinderKind is the closed value-binder vocabulary exposed to embedding
// clients. It mirrors WIR SymbolInfo rather than leaking bind internals.
type BinderKind string

const (
	BinderUnknown  BinderKind = "unknown"
	BinderParam    BinderKind = "param"
	BinderLocal    BinderKind = "local"
	BinderGlobal   BinderKind = "global"
	BinderUpvalue  BinderKind = "upvalue"
	BinderFunction BinderKind = "function"
)

// BinderOccurrenceRole classifies a source occurrence of a lexical binder.
type BinderOccurrenceRole string

const (
	BinderRead    BinderOccurrenceRole = "read"
	BinderWrite   BinderOccurrenceRole = "write"
	BinderCapture BinderOccurrenceRole = "capture"
)

// BinderOccurrence is one non-definition source occurrence of a binder.
type BinderOccurrence struct {
	Role     BinderOccurrenceRole
	Location SourceLocation
}

// BinderInfo is the complete reference/rename substrate for one bind.Result
// symbol. SymbolID is deterministic within the completed result only.
type BinderInfo struct {
	SymbolID    uint64
	Name        string
	Kind        BinderKind
	Definition  SourceLocation
	Occurrences []BinderOccurrence
}

type BinderOccurrencesRequest struct{ Selector ResultSelector }

type BinderOccurrencesResponse struct {
	Meta    QueryMeta
	Binders []BinderInfo
}

// SourcePosition accepts either a byte Offset (when Line and Column are both
// zero) or a one-indexed line/column position. Offsets are canonical within
// the exact source digest named by the response result tag.
type SourcePosition struct {
	Offset int
	Line   int
	Column int
}

type PositionLookupRequest struct {
	Selector ResultSelector
	File     string
	Position SourcePosition
}

// ExpressionType is the type projection at an expression boundary. Display is
// a canonical type-format projection, never diagnostic prose.
type ExpressionType struct {
	Location SourceLocation
	Display  string
}

type EnclosingBody struct {
	ID       BodyID
	Location SourceLocation
}

type PositionLookupResponse struct {
	Meta           QueryMeta
	Found          bool
	Body           EnclosingBody
	SubjectAnchors []judgment.SubjectAnchor
	Expression     *ExpressionType
	Binder         *BinderInfo
}

type DocumentSymbolKind string

const (
	DocumentSymbolFunction    DocumentSymbolKind = "function"
	DocumentSymbolModuleField DocumentSymbolKind = "module_field"
)

// DocumentSymbol is a stable source symbol tree node. Anchor is an
// engine-owned deterministic key; it intentionally has no display semantics.
type DocumentSymbol struct {
	Name     string
	Kind     DocumentSymbolKind
	Anchor   string
	Location SourceLocation
	Children []DocumentSymbol
}

type DocumentSymbolsRequest struct {
	Selector ResultSelector
	File     string
}

type DocumentSymbolsResponse struct {
	Meta    QueryMeta
	Symbols []DocumentSymbol
}

type CalleeIdentity struct {
	Kind     string
	SymbolID uint64
	Name     string
}

// CallRelation projects one solved call fact. Callee is populated only when
// WIR/body facts prove a stable local function identity or signature identity.
type CallRelation struct {
	Location   SourceLocation
	Callee     *CalleeIdentity
	MaySuspend bool
}

type BodyCallRelations struct {
	Body  BodyID
	Calls []CallRelation
}

type CallRelationsRequest struct {
	Selector ResultSelector
	Body     BodyID // empty selects every completed body
}

type CallRelationsResponse struct {
	Meta   QueryMeta
	Bodies []BodyCallRelations
}

// RepairAction is a descriptor-declared, renderer-independent repair
// candidate. Payload contains semantic values only; user-facing wording and
// edit mechanics remain frontend concerns.
type RepairAction struct {
	Kind    judgment.RepairKind
	Target  SourceLocation
	Payload RepairPayload
}

type RepairPayload struct {
	Type string
}

type RepairActionsRequest struct {
	Selector ResultSelector
	Codes    []judgment.Code
}

type RepairActionsResponse struct {
	Meta    QueryMeta
	Actions []RepairAction
}
