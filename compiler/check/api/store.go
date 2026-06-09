// Store interfaces define the contract between the type checker and its
// backing storage. Canonical checking uses Summary as its interprocedural
// authority; the postflow projection surfaces below are typed compatibility/export
// lanes for noncanonical paths and final projections.
//
// The interfaces form a hierarchy with increasing capability:
//
//	ModuleStore     - Module-level bindings and aliases
//	GraphStore      - CFG graph lookup by ID
//	ParentScopes    - Parent scope lookup for nested functions
//	NestedMetaStore - Nested function metadata
//	FunctionRefs    - Symbol/function bidirectional lookup
//	StoreReader     - Read-only combination of above
//	CanonicalStore  - Canonical-owned metadata plus final fact projection
//	NestedStore     - StoreReader required by nested metadata consumers
//	Postflow*Projection* - Explicit noncanonical postflow projection lane boundaries
//	IterationStore  - Full mutation capability for postflow projection lanes
package api

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

// FunctionRef is the canonical mapping for a function symbol.
// It records the relationship between a function's symbol ID, its CFG,
// its parent context, and the AST node. Used by interprocedural analysis
// to resolve function identities across call sites.
type FunctionRef struct {
	Sym           cfg.SymbolID
	GraphID       uint64
	ParentGraphID uint64
	DefPoint      cfg.Point
	Func          *ast.FunctionExpr
}

// NestedMeta holds parent metadata for a nested function graph.
type NestedMeta struct {
	ParentGraphID uint64
	DefPoint      cfg.Point
}

// GraphProvider maps function literals to CFGs and exposes the canonical
// abstract-interpreter evidence for each graph.
type GraphProvider interface {
	GetOrBuildCFG(fn *ast.FunctionExpr) *cfg.Graph
	EvidenceForGraph(graph *cfg.Graph) FlowEvidence
}

// ModuleStore provides module-level bindings and alias maps.
type ModuleStore interface {
	ModuleBindings() *bind.BindingTable
	ModuleAliases() map[cfg.SymbolID]string
}

// GraphStore provides access to known CFGs.
type GraphStore interface {
	Graphs() map[uint64]*cfg.Graph
}

// ParentScopes provides parent scope lookup by graph.
type ParentScopes interface {
	Parents() map[uint64]*scope.State
	GraphParentHashOf(graphID uint64) uint64
	GraphKeyFor(graph *cfg.Graph, parent *scope.State) (GraphKey, bool)
}

// NestedMetaStore provides nested graph metadata.
type NestedMetaStore interface {
	NestedMetaFor(graphID uint64) (NestedMeta, bool)
}

// FunctionRefs provides symbol/function lookup for function graphs.
type FunctionRefs interface {
	RegisterFunctionRef(sym cfg.SymbolID, fn *ast.FunctionExpr, graph *cfg.Graph, parentGraphID uint64, defPoint cfg.Point)
	FunctionRefBySym(sym cfg.SymbolID) *FunctionRef
	FuncForSymbol(sym cfg.SymbolID) *ast.FunctionExpr
	FuncForGraph(graph *cfg.Graph) *ast.FunctionExpr
	SymbolForFunc(fn *ast.FunctionExpr) (cfg.SymbolID, bool)
	FunctionRefsByParentGraph(parentGraphID uint64) []FunctionRef
}

// StoreReader is the read contract shared by normal checker phases. It
// intentionally excludes postflow projection lane reads; callers that need final
// function facts should request FunctionFactProjectionReader, and noncanonical postflow
// code should request the specific postflow lane it owns.
type StoreReader interface {
	ModuleStore
	GraphStore
	EvidenceForGraph(graph *cfg.Graph) FlowEvidence
	ParentScopes
	NestedMetaStore
	FunctionRefs
}

// PostflowCapturedTypeProjectionReader exposes captured-symbol type projections
// from the noncanonical postflow product. It is not a canonical Summary read surface.
type PostflowCapturedTypeProjectionReader interface {
	CapturedTypeProjection(graph *cfg.Graph, parent *scope.State, sym cfg.SymbolID) (typ.Type, bool)
}

// PostflowCapturedTypeProjectionWriter records one captured-symbol type projection.
type PostflowCapturedTypeProjectionWriter interface {
	MergeCapturedTypeProjection(key GraphKey, sym cfg.SymbolID, value product.AbstractValue)
}

// PostflowCapturedFieldProjectionReader exposes captured-field assignment projections
// from the noncanonical postflow product.
type PostflowCapturedFieldProjectionReader interface {
	CapturedFieldAssignsProjection(graph *cfg.Graph, parent *scope.State) CapturedFieldAssigns
}

// PostflowCapturedFieldProjectionWriter records captured-field assignment projections.
type PostflowCapturedFieldProjectionWriter interface {
	MergeCapturedFieldProjection(key GraphKey, nestedSym cfg.SymbolID, capturedSym cfg.SymbolID, fields FieldValues)
}

// PostflowCapturedProjectionStore owns the captured-symbol and captured-field
// postflow lanes. Components should depend on this only when they need both
// captured reads and writes.
type PostflowCapturedProjectionStore interface {
	PostflowCapturedTypeProjectionReader
	PostflowCapturedTypeProjectionWriter
	PostflowCapturedFieldProjectionReader
	PostflowCapturedFieldProjectionWriter
}

// PostflowConstructorProjectionReader exposes constructor field projections from
// the noncanonical postflow product.
type PostflowConstructorProjectionReader interface {
	ConstructorFieldsProjection(classSym cfg.SymbolID) (FieldValues, bool)
}

// PostflowConstructorProjectionWriter records constructor field projections.
type PostflowConstructorProjectionWriter interface {
	MergeConstructorFieldProjection(classSym cfg.SymbolID, fields FieldValues)
}

// PostflowConstructorProjectionStore owns the constructor-field postflow lane.
type PostflowConstructorProjectionStore interface {
	PostflowConstructorProjectionReader
	PostflowConstructorProjectionWriter
}

// PostflowFunctionFactWriter records final/public FunctionFact projection rows in
// the noncanonical postflow product.
type PostflowFunctionFactWriter interface {
	MergeFunctionFactProjection(key GraphKey, sym cfg.SymbolID, fact FunctionFact)
}

// CanonicalFunctionFactProjectionSink installs final Summary-derived FunctionFacts
// without participating in postflow projection iteration.
type CanonicalFunctionFactProjectionSink interface {
	SetCanonicalFunctionFactsProjection(facts map[GraphKey]FunctionFacts)
}

// FunctionFactProjectionReader exposes final/public FunctionFact projection.
// It is a read-only output surface; it is not a canonical semantic input.
type FunctionFactProjectionReader interface {
	FunctionFactProjection(graph *cfg.Graph, parent *scope.State, sym cfg.SymbolID) (FunctionFact, bool)
	FunctionFactsProjectionForExport(graph *cfg.Graph, parent *scope.State) FunctionFacts
}

// CanonicalStore is the store surface the canonical summary engine is allowed to
// use: module binding publication, graph-parent publication, parent-key lookup,
// and final Summary-derived FunctionFacts projection. It intentionally excludes
// postflow projection iteration and visible postflow projection lane reads.
type CanonicalStore interface {
	CanonicalFunctionFactProjectionSink

	SetModuleBindings(bindings *bind.BindingTable)
	SetParentScope(parentHash uint64, parent *scope.State)
	SetGraphParentHash(graphID, parentHash uint64)
	ParentGraphKeyForSymbol(sym cfg.SymbolID) (GraphKey, bool)
}

// NestedStore is the read-only store interface required by nested metadata
// consumers. Postflow projection nested inference requests only the lanes it owns.
type NestedStore interface {
	StoreReader
}

// IterationStore provides mutation operations required by postflow projection fixpoint paths.
type IterationStore interface {
	StoreReader
	FunctionFactProjectionReader
	PostflowFunctionFactWriter
	PostflowCapturedProjectionStore
	PostflowConstructorProjectionStore
	ParentGraphKeyForSymbol(sym cfg.SymbolID) (GraphKey, bool)

	ClearPostflowProjectionState()
	AdvancePostflowProjections() bool
	PostflowProjectionDiffs() []string

	SetModuleBindings(bindings *bind.BindingTable)
	SetModuleAliases(aliases map[cfg.SymbolID]string)
	SetParentScope(parentHash uint64, parent *scope.State)
	SetGraphParentHash(graphID, parentHash uint64)
}
