package service

import "github.com/wippyai/go-lua/analysis/check/judgment"

// EmbeddingSchemaVersion pins the versioned, renderer-independent checker
// embedding surface. It is deliberately separate from JIRSchemaVersion: JIR
// pins judgments, while this pin covers navigation and repair DTOs.
const EmbeddingSchemaVersion = 5

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
	// Scope is the enclosing lexical function scope. It intentionally
	// over-approximates block scope, allowing rename clients to reject any
	// possible capture rather than make an unsound edit.
	Scope SourceLocation
}

// BinderInfo is the complete reference/rename substrate for one bind.Result
// symbol. SymbolID is deterministic within the completed result only.
type BinderInfo struct {
	SymbolID    uint64
	Name        string
	Kind        BinderKind
	ModuleLocal bool
	Definition  SourceLocation
	Scope       SourceLocation
	Occurrences []BinderOccurrence
}

type BinderOccurrencesRequest struct{ Selector ResultSelector }

type BinderOccurrencesResponse struct {
	Meta    QueryMeta
	Binders []BinderInfo
}

// SemanticTokenKind is the checker-owned classification of a source token.
// Protocol adapters translate this closed vocabulary to their token legends;
// they must not infer kinds from lexer text.
type SemanticTokenKind string

const (
	SemanticTokenVariable  SemanticTokenKind = "variable"
	SemanticTokenParameter SemanticTokenKind = "parameter"
	SemanticTokenFunction  SemanticTokenKind = "function"
)

// SemanticTokenModifier is a solved semantic property of a token. The
// modifier vocabulary is deliberately small: protocol clients can map it to
// custom semantic-token modifiers without inspecting checker state.
type SemanticTokenModifier string

const (
	// SemanticTokenTypestateTracked marks a binder proven to participate in a
	// declared lifecycle protocol at a solved call site.
	SemanticTokenTypestateTracked SemanticTokenModifier = "typestate-tracked"
	// SemanticTokenPlacement marks a solved allocation site with either a
	// frame-local or decomposable placement license. The exact license remains
	// available through PlacementPlan and hover evidence.
	SemanticTokenPlacement SemanticTokenModifier = "placement"
)

// SemanticToken is a digest-bound, non-overlapping source classification.
// Locations are the checker source coordinates; adapters perform only the
// exact-snapshot UTF conversion and LSP delta encoding.
type SemanticToken struct {
	Kind      SemanticTokenKind
	Location  SourceLocation
	Modifiers []SemanticTokenModifier
}

type SemanticTokensRequest struct {
	Selector ResultSelector
	File     string
}

type SemanticTokensResponse struct {
	Meta   QueryMeta
	Tokens []SemanticToken
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
	Code    judgment.Code
	Kind    judgment.RepairKind
	Target  SourceLocation
	Payload RepairPayload
}

// RepairEdit is one exact, source-bound change verified by the checker when
// projecting a descriptor-declared repair. Frontends must apply these edits as
// given and must not infer additional edits from diagnostic text.
type RepairEdit struct {
	Target  SourceLocation
	NewText string
}

type RepairPayload struct {
	Type  string
	Edits []RepairEdit
}

type RepairActionsRequest struct {
	Selector ResultSelector
	Codes    []judgment.Code
}

type RepairActionsResponse struct {
	Meta    QueryMeta
	Actions []RepairAction
}
