// Package returns provides return type inference for nested Lua functions.
//
// This package implements the return type inference pass that computes return
// types for locally-defined functions. Since Lua allows omitting return type
// annotations, the type checker must infer return types from the function body.
// This is complicated by mutual recursion: functions in the same scope can call
// each other, creating circular type dependencies.
//
// # SCC-Based Inference
//
// Return type inference uses Strongly Connected Component (SCC) decomposition.
// Functions are grouped into SCCs based on their call relationships. Within
// an SCC, functions are processed together using fixpoint iteration until
// return types stabilize.
//
// # Return Vectors
//
// A return vector represents the types returned by a function. For
// `return a, b, c`, the vector is [typeof(a), typeof(b), typeof(c)]. Vectors
// are accumulated across all return statements in a
// function body and joined to produce the final return type.
//
// # Canonical Function Facts vs Iteration Vectors
//
// The stored authority is api.FunctionFacts. During SCC solving, the inferencer
// also keeps a provisional map of return vectors for the current iteration.
//
// During analysis, iteration vectors are used for functions in the current SCC
// to avoid circular dependence, while canonical function facts are used for
// functions outside the SCC (whose types are already known).
//
// # Parameter Evidence Propagation
//
// For unannotated parameters, the system propagates evidence from call sites.
// If function `f` is called as `f(42)`, the first parameter of `f` records
// number evidence. Evidence is joined across all call sites and propagated through
// the call graph until fixpoint.
package returns

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/typ"
)

// LocalFuncInfo holds information about a local function for SCC computation.
//
// Each LocalFuncInfo represents a function that may participate in mutual
// recursion with other local functions. The info includes the function's
// AST, CFG, definition context, and any parameter evidence inferred from
// call sites.
type LocalFuncInfo struct {
	Sym      cfg.SymbolID
	Fn       *ast.FunctionExpr
	DefScope *scope.State
	Graph    *cfg.Graph
	// ParentGraph is the graph where this local function is defined.
	// Used for parent-scope callsite evidence propagation.
	ParentGraph *cfg.Graph
	ParentFn    *ast.FunctionExpr
	DefPoint    cfg.Point
	// ParameterEvidence holds inferred effective-parameter types from call sites in the
	// parent graph. For methods, index 0 is self.
	ParameterEvidence []typ.Type
}
