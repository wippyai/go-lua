// Interprocedural fact types for cross-function type analysis.
//
// These types represent the results of analyzing one function that are
// needed when analyzing other functions. They are keyed by GraphKey
// (graph ID + parent scope hash) to support context-sensitive analysis.
package api

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/typ"
)

// ParamHints maps function symbols to parameter type hints inferred from call sites.
// When a function is called with known argument types, those types are recorded
// as hints and propagated to the function's parameter declarations.
type ParamHints = map[cfg.SymbolID][]typ.Type

// FunctionFact is the canonical function-related interproc fact for one symbol.
// All return and local-function type evidence for a function converges here.
type FunctionFact struct {
	// Summary is the declared/pre-flow return vector.
	Summary []typ.Type
	// Narrow is the post-flow return vector.
	Narrow []typ.Type
	// Type is the canonical local function type evidence.
	Type typ.Type
}

// FunctionFacts maps function symbols to their canonical function facts.
type FunctionFacts map[cfg.SymbolID]FunctionFact

// Fact returns the canonical fact for sym.
func (facts FunctionFacts) Fact(sym cfg.SymbolID) (FunctionFact, bool) {
	if len(facts) == 0 || sym == 0 {
		return FunctionFact{}, false
	}
	ff, ok := facts[sym]
	return ff, ok
}

// Summary returns the declared/pre-flow return vector for sym.
func (facts FunctionFacts) Summary(sym cfg.SymbolID) []typ.Type {
	ff, ok := facts.Fact(sym)
	if !ok {
		return nil
	}
	return ff.Summary
}

// NarrowSummary returns the post-flow return vector for sym.
func (facts FunctionFacts) NarrowSummary(sym cfg.SymbolID) []typ.Type {
	ff, ok := facts.Fact(sym)
	if !ok {
		return nil
	}
	return ff.Narrow
}

// FunctionType returns the canonical local function type for sym.
func (facts FunctionFacts) FunctionType(sym cfg.SymbolID) typ.Type {
	ff, ok := facts.Fact(sym)
	if !ok {
		return nil
	}
	return ff.Type
}

// LiteralSigs maps anonymous function literal expressions to their signatures.
// Used when function literals are passed as arguments or assigned to variables
// without explicit type annotations.
type LiteralSigs = map[*ast.FunctionExpr]*typ.Function

// CapturedTypes maps captured symbols to their flow-derived types for a graph.
// These are computed from the parent function's flow facts at the definition
// point of the nested function and used as type hints for captured variables.
type CapturedTypes = map[cfg.SymbolID]typ.Type

// CapturedFieldAssigns maps nested function symbols to field assignments
// they make to captured variables from parent scopes.
//
// Structure: nestedFuncSymbol -> capturedVarSymbol -> fieldName -> fieldType
//
// This enables the parent scope to see which fields a nested function assigns
// to its captured variables, supporting constructor inference patterns.
type CapturedFieldAssigns = map[cfg.SymbolID]map[cfg.SymbolID]map[string]typ.Type

// ContainerMutation records a container element mutation on a captured variable.
// Segments capture the path from the base symbol (e.g., .ch, ["queue"]).
type ContainerMutation struct {
	Segments  []constraint.Segment
	ValueType typ.Type
}

// ContainerMutationKey returns the canonical path key for a container mutation.
func ContainerMutationKey(m ContainerMutation) string {
	return constraint.FormatSegments(m.Segments)
}

// CapturedContainerMutations maps nested function symbols to container mutations
// they make to captured variables from parent scopes.
//
// Structure: nestedFuncSymbol -> capturedVarSymbol -> []ContainerMutation
type CapturedContainerMutations = map[cfg.SymbolID]map[cfg.SymbolID][]ContainerMutation

// ConstructorFields maps class symbols to field assignments captured in constructors.
// Structure: classSymbol -> fieldName -> fieldType.
type ConstructorFields = map[cfg.SymbolID]map[string]typ.Type

// Facts bundles all interprocedural analysis results for a single function graph.
// These facts are computed during analysis and stored per (graph, parent) pair.
type Facts struct {
	FunctionFacts      FunctionFacts
	ParamHints         ParamHints
	LiteralSigs        LiteralSigs
	CapturedTypes      CapturedTypes
	CapturedFields     CapturedFieldAssigns
	CapturedContainers CapturedContainerMutations
	ConstructorFields  ConstructorFields
}
