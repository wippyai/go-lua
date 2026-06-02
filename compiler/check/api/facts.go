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
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

// FunctionFact is the canonical function-related interproc fact for one symbol.
// All return and local-function type evidence for a function converges here.
type FunctionFact struct {
	// Params is the public call-boundary parameter evidence vector. For method
	// calls, slot 0 is the receiver/self argument and the remaining slots are
	// source args. This vector is used to project callable contracts to callers.
	//
	// The carrier holds interned product.AbstractValue per slot: producers lift
	// their computed typ.Type evidence through product.FromType at admission and
	// consumers project it back through product.ProjectValue at egress. The
	// per-slot semantic merge keeps its precise typ.Type logic at the merge
	// boundary; only the carrier and the convergence equality are value-domain.
	Params []product.AbstractValue
	// BodyParams is the body contract vector inferred from the function body.
	// It records semantic requirements that the body imposes on its parameters.
	// It is not call-entry evidence and must not initialize the same body's
	// abstract state; callers and diagnostics consume it as an obligation.
	BodyParams []product.AbstractValue
	// EntryParams is observed call-entry parameter evidence for interpreting
	// this function's body. It preserves structural discriminants used by
	// path-sensitive flow and is never projected as a public caller contract.
	EntryParams []product.AbstractValue
	// Summary is the declared/pre-flow return vector.
	Summary []product.AbstractValue
	// Narrow is the post-flow return vector.
	Narrow []product.AbstractValue
	// Signature is the source-level function shape: source annotations, arity,
	// variadic information, effects/specs, and refinement metadata. Inferred
	// parameter and return facts are projected into a function type from the
	// product channels; they are not stored here as an independent authority.
	Signature *typ.Function
	// Refinement is the canonical effect/refinement summary for the function.
	Refinement *constraint.FunctionRefinement
	// EnvReturns records exported closure return dependencies on caller-visible
	// module environment paths. It is projected into contract specs at export
	// and consumed by the abstract interpreter at call sites.
	EnvReturns []contract.EnvReturnSpec
}

// FunctionFacts maps function symbols to their canonical function facts.
type FunctionFacts map[cfg.SymbolID]FunctionFact

// LiteralSigs maps anonymous function literal expressions to their signatures.
// Used when function literals are passed as arguments or assigned to variables
// without explicit type annotations.
type LiteralSigs = map[*ast.FunctionExpr]*typ.Function

// LiteralSignatureLookup is the normalized lookup surface for function-literal
// signatures. It prevents consumers from depending on a concrete AST-keyed map
// when a producer can provide a lazy or projected carrier.
type LiteralSignatureLookup interface {
	Lookup(fn *ast.FunctionExpr) *typ.Function
}

// LiteralSigsLookup lifts an external AST-keyed map into LiteralSignatureLookup.
type LiteralSigsLookup map[*ast.FunctionExpr]*typ.Function

// LiteralSignatureLookupFromMap normalizes an external AST-keyed map into the
// lookup surface consumed by in-process analysis.
func LiteralSignatureLookupFromMap(signatures map[*ast.FunctionExpr]*typ.Function) LiteralSignatureLookup {
	if len(signatures) == 0 {
		return nil
	}
	return LiteralSigsLookup(signatures)
}

// Lookup returns the signature for fn.
func (m LiteralSigsLookup) Lookup(fn *ast.FunctionExpr) *typ.Function {
	if fn == nil || len(m) == 0 {
		return nil
	}
	return m[fn]
}

// CapturedTypes maps captured symbols to their flow-derived types for a graph.
// These are computed from the parent function's flow facts at the definition
// point of the nested function and used as type hints for captured variables.
// The carrier holds interned product.AbstractValue lifted at admission and
// projected at egress.
type CapturedTypes = map[cfg.SymbolID]product.AbstractValue

// FieldValues maps a typed field/path segment to its product-domain value.
// Boundary collection/projection APIs may still speak in source field names;
// Facts store typed keys so the interprocedural product has one identity
// language.
type FieldValues = map[constraint.Segment]product.AbstractValue

// CapturedFieldAssigns maps nested function symbols to field assignments
// they make to captured variables from parent scopes.
//
// Structure: nestedFuncSymbol -> capturedVarSymbol -> fieldKey -> fieldType
//
// This enables the parent scope to see which fields a nested function assigns
// to its captured variables, supporting constructor inference patterns. The
// field type carrier holds interned product.AbstractValue.
type CapturedFieldAssigns = map[cfg.SymbolID]map[cfg.SymbolID]FieldValues

// ConstructorFields maps class symbols to field assignments captured in constructors.
// Structure: classSymbol -> fieldKey -> fieldType. The field type carrier holds
// interned product.AbstractValue.
type ConstructorFields = map[cfg.SymbolID]FieldValues

// Facts bundles one canonical interprocedural product slice. Most slices are
// stored per (graph, parent) pair; module-wide facts use ModuleFactsKey.
type Facts struct {
	FunctionFacts     FunctionFacts
	LiteralSigs       LiteralSigs
	CapturedTypes     CapturedTypes
	CapturedFields    CapturedFieldAssigns
	ConstructorFields ConstructorFields
}
