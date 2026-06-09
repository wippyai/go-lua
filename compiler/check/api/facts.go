// Final projection types.
//
// Canonical checking uses Summary as its interprocedural authority. These types
// are final/public projection shapes, not canonical input.
package api

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

// FunctionCallProjection is the public caller-contract lane.
type FunctionCallProjection struct {
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
}

// FunctionBodyProjection is the function-body obligation lane.
type FunctionBodyProjection struct {
	// Params is the body contract vector inferred from the function body. It
	// records semantic requirements that the body imposes on its parameters. It
	// is not call-entry evidence and must not initialize the same body's
	// abstract state; callers and diagnostics consume it as an obligation.
	Params []product.AbstractValue
}

// FunctionEntryProjection is the observed call-entry interpretation lane.
type FunctionEntryProjection struct {
	// Params is observed call-entry parameter evidence for interpreting this
	// function's body. It preserves structural discriminants used by
	// path-sensitive flow and is never projected as a public caller contract.
	Params []product.AbstractValue
}

// FunctionReturnProjection is the public return projection lane.
type FunctionReturnProjection struct {
	// Preflow is the declared/pre-flow return vector.
	Preflow []product.AbstractValue
	// Postflow is the post-flow return vector.
	Postflow []product.AbstractValue
}

// FunctionPublicProjection is the source/public function-shape lane.
type FunctionPublicProjection struct {
	// Signature is the source-level function shape: source annotations, arity,
	// variadic information, effects/specs, and refinement metadata. Inferred
	// parameter and return facts are projected into a function type from the
	// product channels; they are not stored here as an independent authority.
	Signature *typ.Function
}

// FunctionEffectProjection is the function effect/refinement lane.
type FunctionEffectProjection struct {
	// Refinement is the canonical effect/refinement summary for the function.
	Refinement *constraint.FunctionRefinement
}

// FunctionExportProjection is the module/export dependency lane.
type FunctionExportProjection struct {
	// EnvReturns records exported closure return dependencies on caller-visible
	// module environment paths. It is projected into contract specs at export
	// and consumed by the abstract interpreter at call sites.
	EnvReturns []contract.EnvReturnSpec
}

// FunctionFact is the final/public function-fact projection for one symbol.
// Canonical Summary owns semantic convergence; FunctionFact is output/export
// shape and compatibility data, not canonical input.
type FunctionFact struct {
	Call    FunctionCallProjection
	Body    FunctionBodyProjection
	Entry   FunctionEntryProjection
	Returns FunctionReturnProjection
	Public  FunctionPublicProjection
	Effects FunctionEffectProjection
	Export  FunctionExportProjection
}

// FunctionFacts maps function symbols to final/public function-fact projections.
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
