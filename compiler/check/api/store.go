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
//	PostflowProjectionStore - Explicit noncanonical postflow projection boundary
//	IterationStore  - Full mutation capability for projection-product fixpoint paths
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
// intentionally excludes postflow projection product reads; callers that need final
// function facts should request FunctionFactProjectionReader, and noncanonical postflow
// code should request PostflowProjectionStore explicitly.
type StoreReader interface {
	ModuleStore
	GraphStore
	EvidenceForGraph(graph *cfg.Graph) FlowEvidence
	ParentScopes
	NestedMetaStore
	FunctionRefs
}

// PostflowProjectionReader exposes typed reads from the noncanonical postflow
// projection product. It is not a canonical Summary read surface.
type PostflowProjectionReader interface {
	CapturedTypeProjection(graph *cfg.Graph, parent *scope.State, sym cfg.SymbolID) (typ.Type, bool)
	CapturedFieldAssignsProjection(graph *cfg.Graph, parent *scope.State) CapturedFieldAssigns
	ConstructorFieldsProjection(classSym cfg.SymbolID) (FieldValues, bool)
}

// PostflowProjectionSink provides typed write access to per-iteration projection
// lanes. The store owns lowering these lanes into the internal product lattice.
type PostflowProjectionSink interface {
	MergeFunctionFactProjection(key GraphKey, sym cfg.SymbolID, fact FunctionFact)
	MergeCapturedTypeProjection(key GraphKey, sym cfg.SymbolID, value product.AbstractValue)
	MergeCapturedFieldProjection(key GraphKey, nestedSym cfg.SymbolID, capturedSym cfg.SymbolID, fields FieldValues)
	MergeConstructorFieldProjection(classSym cfg.SymbolID, fields FieldValues)
}

// CanonicalFunctionFactProjectionSink installs final Summary-derived FunctionFacts
// without participating in the projection-product iteration product.
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
// projection-product iteration and visible postflow projection-product reads.
type CanonicalStore interface {
	CanonicalFunctionFactProjectionSink

	SetModuleBindings(bindings *bind.BindingTable)
	SetParentScope(parentHash uint64, parent *scope.State)
	SetGraphParentHash(graphID, parentHash uint64)
	ParentGraphKeyForSymbol(sym cfg.SymbolID) (GraphKey, bool)
}

// NestedStore is the read-only store interface required by nested metadata
// consumers. Postflow projection nested inference uses PostflowProjectionStore instead.
type NestedStore interface {
	StoreReader
}

// PostflowProjectionStore is the explicitly named noncanonical postflow/compatibility
// boundary for typed postflow projection lanes. Normal checker/synth/module/export
// code must not request this interface.
type PostflowProjectionStore interface {
	StoreReader
	PostflowProjectionReader
	PostflowProjectionSink

	FunctionFactProjectionReader
	ParentGraphKeyForSymbol(sym cfg.SymbolID) (GraphKey, bool)
}

// IterationStore provides mutation operations required by postflow projection fixpoint paths.
type IterationStore interface {
	PostflowProjectionStore

	ClearPostflowProjectionState()
	AdvancePostflowProjections() bool
	PostflowProjectionDiffs() []string

	SetModuleBindings(bindings *bind.BindingTable)
	SetModuleAliases(aliases map[cfg.SymbolID]string)
	SetParentScope(parentHash uint64, parent *scope.State)
	SetGraphParentHash(graphID, parentHash uint64)
}
