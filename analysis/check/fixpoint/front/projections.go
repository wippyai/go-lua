package front

import (
	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// DraftsView is the executable equation publication admitted by the front.
// Consumers of evaluation state do not also receive source graph or span data.
type DraftsView interface {
	DraftArtifact() equation.Artifact
	CyclicDraft() *equation.CyclicArtifact
	FrozenDraft() equation.CyclicArtifact
	ValidateDraftWires() error
	ControlDiagnosticDrafts() []ControlDiagnostic
	PolicyDiagnosticDrafts() []ControlDiagnostic
	NativeContractDrafts() []NativeContract
}

// BoundaryView is the stable lexical identity and declared boundary of one
// admitted body. It excludes executable drafts and source topology.
type BoundaryView interface {
	BodyID() equation.BodyID
	PrototypeID() wir.FunctionSymbolID
	PrototypeDisplayName() string
	BodyLexicalPath() []uint32
	BodyBoundary() wir.BodyBoundary
	BoundaryRebound() bool
	ResolvedTypeDefinitions() map[string]typ.Type
}

// SpansView is source-owned diagnostic projection metadata. It contains no
// executable equation or lowered instruction graph.
type SpansView interface {
	ClaimSpanIndex() map[string]wir.Span
	ClaimTargetSpanIndex() map[string]wir.Span
	ClaimNameSpanIndex() map[string]wir.Span
	BranchJoinSpanIndex() map[string]wir.Span
	CallSpanIndex() map[string]wir.Span
	BranchSpanIndex() map[string]wir.Span
	EffectSpanIndex() map[string]wir.Span
	ExpressionSpanIndex() map[string]wir.Span
	ReturnSpanIndex() map[string]wir.Span
	TableMemberValueSpanIndex() map[string]map[string]wir.Span
	QualifiedClaimSpanIndex() map[SpanKey]wir.Span
	QualifiedClaimTargetSpanIndex() map[SpanKey]wir.Span
	QualifiedCallSpanIndex() map[SpanKey]wir.Span
	QualifiedBranchSpanIndex() map[SpanKey]wir.Span
	QualifiedEffectSpanIndex() map[SpanKey]wir.Span
	TypeFieldSpanIndex() map[string]map[string]wir.Span
}

// GraphView is the immutable lowered source topology and nested-body catalog.
// It is descriptive input only and cannot be evaluated without a DraftsView.
type GraphView interface {
	LoweredBody() *wir.Body
	SourceGraph() cfg.Graph
	NestedCompilations() []Compilation
	CompilationCatalog() BodyCatalog
}

// The composed views below name only combinations used by downstream
// consumers. They keep function signatures honest when a calculation crosses
// two or three front-owned projections without restoring the whole Compilation
// bag as its input.
type DraftsBoundaryView interface {
	DraftsView
	BoundaryView
}

type DraftsSpansView interface {
	DraftsView
	SpansView
}

type DraftsGraphView interface {
	DraftsView
	GraphView
}

type BoundarySpansView interface {
	BoundaryView
	SpansView
}

type BoundaryGraphView interface {
	BoundaryView
	GraphView
}

type SpansGraphView interface {
	SpansView
	GraphView
}

type DraftsBoundarySpansView interface {
	DraftsView
	BoundaryView
	SpansView
}

type DraftsBoundaryGraphView interface {
	DraftsView
	BoundaryView
	GraphView
}

type DraftsSpansGraphView interface {
	DraftsView
	SpansView
	GraphView
}

type BoundarySpansGraphView interface {
	BoundaryView
	SpansView
	GraphView
}

type ProjectionView interface {
	DraftsView
	BoundaryView
	SpansView
	GraphView
}

var (
	_ DraftsView   = Compilation{}
	_ BoundaryView = Compilation{}
	_ SpansView    = Compilation{}
	_ GraphView    = Compilation{}
)

func (c Compilation) DraftArtifact() equation.Artifact             { return c.Artifact }
func (c Compilation) CyclicDraft() *equation.CyclicArtifact        { return c.Cyclic }
func (c Compilation) FrozenDraft() equation.CyclicArtifact         { return c.frozen }
func (c Compilation) ControlDiagnosticDrafts() []ControlDiagnostic { return c.controlDiagnostics }
func (c Compilation) PolicyDiagnosticDrafts() []ControlDiagnostic  { return c.policyDiagnostics }
func (c Compilation) NativeContractDrafts() []NativeContract       { return c.nativeContracts }
func (c Compilation) BodyID() equation.BodyID                      { return c.Body }
func (c Compilation) PrototypeID() wir.FunctionSymbolID            { return c.Prototype }
func (c Compilation) PrototypeDisplayName() string                 { return c.PrototypeName }
func (c Compilation) BodyLexicalPath() []uint32                    { return c.LexicalPath }
func (c Compilation) BodyBoundary() wir.BodyBoundary               { return c.Boundary }
func (c Compilation) BoundaryRebound() bool                        { return c.rebindsBoundary }
func (c Compilation) ResolvedTypeDefinitions() map[string]typ.Type { return c.typeDefinitions }
func (c Compilation) ClaimSpanIndex() map[string]wir.Span          { return c.claimSpans }
func (c Compilation) ClaimTargetSpanIndex() map[string]wir.Span    { return c.claimTargetSpans }
func (c Compilation) ClaimNameSpanIndex() map[string]wir.Span      { return c.claimNameSpans }
func (c Compilation) BranchJoinSpanIndex() map[string]wir.Span     { return c.branchJoinSpans }
func (c Compilation) CallSpanIndex() map[string]wir.Span           { return c.callSpans }
func (c Compilation) BranchSpanIndex() map[string]wir.Span         { return c.branchSpans }
func (c Compilation) EffectSpanIndex() map[string]wir.Span         { return c.effectSpans }
func (c Compilation) ExpressionSpanIndex() map[string]wir.Span     { return c.expressionSpans }
func (c Compilation) ReturnSpanIndex() map[string]wir.Span         { return c.returnSpans }
func (c Compilation) TableMemberValueSpanIndex() map[string]map[string]wir.Span {
	return c.tableMemberValueSpans
}
func (c Compilation) QualifiedClaimSpanIndex() map[SpanKey]wir.Span {
	return c.qualifiedClaimSpans
}
func (c Compilation) QualifiedClaimTargetSpanIndex() map[SpanKey]wir.Span {
	return c.qualifiedClaimTargetSpans
}
func (c Compilation) QualifiedCallSpanIndex() map[SpanKey]wir.Span {
	return c.qualifiedCallSpans
}
func (c Compilation) QualifiedBranchSpanIndex() map[SpanKey]wir.Span {
	return c.qualifiedBranchSpans
}
func (c Compilation) QualifiedEffectSpanIndex() map[SpanKey]wir.Span {
	return c.qualifiedEffectSpans
}
func (c Compilation) TypeFieldSpanIndex() map[string]map[string]wir.Span {
	return c.typeFieldSpans
}
func (c Compilation) LoweredBody() *wir.Body            { return c.WIR }
func (c Compilation) SourceGraph() cfg.Graph            { return c.Graph }
func (c Compilation) NestedCompilations() []Compilation { return c.nested }
func (c Compilation) CompilationCatalog() BodyCatalog   { return c.catalog }
