// Store interfaces define the contract between the type checker and its
// backing storage for interprocedural analysis data. These interfaces
// enable different storage implementations (in-memory, persistent) and
// support testing with mock implementations.
//
// The interfaces form a hierarchy with increasing capability:
//
//	ModuleStore     - Module-level bindings and aliases
//	GraphStore      - CFG graph lookup by ID
//	ParentScopes    - Parent scope lookup for nested functions
//	NestedMetaStore - Nested function metadata
//	InterprocFactReader   - Visible interproc fact products
//	FunctionRefs    - Symbol/function bidirectional lookup
//	StoreReader     - Read-only combination of above
//	CanonicalStore  - Canonical-owned metadata plus final fact projection
//	NestedStore     - StoreReader + legacy fact product writes
//	IterationStore  - Full mutation capability for legacy fixpoint paths
package api

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/scope"
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

// InterprocFactProduct is the typed view over one visible interprocedural fact
// product. Consumers read the slot they own; there is no public whole-product
// snapshot path.
type InterprocFactProduct interface {
	// FunctionFacts returns the visible function-fact slot for export and
	// assertions. Hot symbol reads must use FunctionFact.
	FunctionFacts() FunctionFacts
	FunctionFact(sym cfg.SymbolID) (FunctionFact, bool)
	LiteralSig(fn *ast.FunctionExpr) (*typ.Function, bool)
	CapturedType(sym cfg.SymbolID) (typ.Type, bool)
	CapturedFieldAssigns() CapturedFieldAssigns
	ConstructorFields(classSym cfg.SymbolID) (FieldValues, bool)
}

// InterprocFactReader exposes visible interproc fact products.
type InterprocFactReader interface {
	ModuleFacts() InterprocFactProduct
	InterprocFacts(graph *cfg.Graph, parent *scope.State) InterprocFactProduct
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

// StoreReader is the read contract shared by checker phases.
type StoreReader interface {
	ModuleStore
	GraphStore
	EvidenceForGraph(graph *cfg.Graph) FlowEvidence
	ParentScopes
	NestedMetaStore
	InterprocFactReader
	FunctionRefs
}

// InterprocFactSink provides write access to per-iteration interproc facts.
type InterprocFactSink interface {
	MergeInterprocFactsNext(key GraphKey, delta Facts)
}

// CanonicalFunctionFactProjectionSink installs final canonical FunctionFacts
// without participating in the legacy interproc iteration product.
type CanonicalFunctionFactProjectionSink interface {
	SetCanonicalFunctionFactsProjection(facts map[GraphKey]FunctionFacts)
}

// CanonicalStore is the store surface the canonical summary engine is allowed to
// use: module binding publication, graph-parent publication, parent-key lookup,
// and final Summary-derived FunctionFacts projection. It intentionally excludes
// legacy interproc iteration and visible interproc fact-product reads.
type CanonicalStore interface {
	CanonicalFunctionFactProjectionSink

	SetModuleBindings(bindings *bind.BindingTable)
	SetParentScope(parentHash uint64, parent *scope.State)
	SetGraphParentHash(graphID, parentHash uint64)
	ParentGraphKeyForSymbol(sym cfg.SymbolID) (GraphKey, bool)
}

// NestedStore is the store interface required by nested processing.
type NestedStore interface {
	StoreReader
	InterprocFactSink
}

// IterationStore provides mutation operations required by legacy fixpoint paths.
type IterationStore interface {
	NestedStore

	ClearInterprocState()
	FixpointSwap() bool
	FixpointDiffs() []string

	SetModuleBindings(bindings *bind.BindingTable)
	SetModuleAliases(aliases map[cfg.SymbolID]string)
	SetParentScope(parentHash uint64, parent *scope.State)
	SetGraphParentHash(graphID, parentHash uint64)

	MergeInterprocFactsNext(key GraphKey, delta Facts)
	ParentGraphKeyForSymbol(sym cfg.SymbolID) (GraphKey, bool)
}
