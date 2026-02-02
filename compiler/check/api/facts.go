// Interprocedural fact types for cross-function type analysis.
//
// These types represent the results of analyzing one function that are
// needed when analyzing other functions. They are keyed by GraphKey
// (graph ID + parent scope hash) to support context-sensitive analysis.
package api

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/typ"
)

// ReturnSummaries maps function symbols to their inferred return type vectors.
// Each entry is a slice of types representing the tuple of values returned
// by the function. For example, a function returning (value, error) has
// a two-element slice [valueType, errorType].
type ReturnSummaries = map[cfg.SymbolID][]typ.Type

// NarrowReturnSummaries holds post-flow return summaries with narrowing applied.
// These are computed after flow analysis and reflect the precise types at
// each return statement, accounting for control flow narrowing.
type NarrowReturnSummaries = map[cfg.SymbolID][]typ.Type

// ParamHints maps function symbols to parameter type hints inferred from call sites.
// When a function is called with known argument types, those types are recorded
// as hints and propagated to the function's parameter declarations.
type ParamHints = map[cfg.SymbolID][]typ.Type

// FuncTypes maps local function symbols to their canonical function types.
// Used for sibling function lookups where the function is defined in the
// same scope as the call site.
type FuncTypes = map[cfg.SymbolID]typ.Type

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

// ConstructorFields maps class symbols to field assignments captured in constructors.
// Structure: classSymbol -> fieldName -> fieldType.
type ConstructorFields = map[cfg.SymbolID]map[string]typ.Type

// Facts bundles all interprocedural analysis results for a single function graph.
// These facts are computed during analysis and stored per (graph, parent) pair.
type Facts struct {
	ReturnSummaries   ReturnSummaries
	NarrowReturns     NarrowReturnSummaries
	ParamHints        ParamHints
	FuncTypes         FuncTypes
	LiteralSigs       LiteralSigs
	CapturedTypes     CapturedTypes
	CapturedFields    CapturedFieldAssigns
	ConstructorFields ConstructorFields
}
