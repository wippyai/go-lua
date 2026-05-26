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
	"github.com/wippyai/go-lua/types/flow"
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

// CapturedTypes maps captured symbols to their flow-derived types for a graph.
// These are computed from the parent function's flow facts at the definition
// point of the nested function and used as type hints for captured variables.
// The carrier holds interned product.AbstractValue lifted at admission and
// projected at egress.
type CapturedTypes = map[cfg.SymbolID]product.AbstractValue

// CapturedFieldAssigns maps nested function symbols to field assignments
// they make to captured variables from parent scopes.
//
// Structure: nestedFuncSymbol -> capturedVarSymbol -> fieldName -> fieldType
//
// This enables the parent scope to see which fields a nested function assigns
// to its captured variables, supporting constructor inference patterns. The
// field type carrier holds interned product.AbstractValue.
type CapturedFieldAssigns = map[cfg.SymbolID]map[cfg.SymbolID]map[string]product.AbstractValue

// ContainerMutationKind describes the operator used for a captured container
// mutation. Different operators have different abstract interpreter effects in
// the parent flow.
type ContainerMutationKind uint8

const (
	// ContainerMutationContainerElement widens generic container element types,
	// such as channel:send(value) through a ContainerElementUnion effect.
	ContainerMutationContainerElement ContainerMutationKind = iota
	// ContainerMutationTableElement widens Lua table array/map-array element
	// types, such as table.insert(t, value).
	ContainerMutationTableElement
	// ContainerMutationMapElement widens Lua table map values from dynamic
	// assignments such as t[k] = value.
	ContainerMutationMapElement
)

// ContainerMutation records a container element mutation on a captured variable.
// Segments capture the path from the base symbol (e.g., .ch, ["queue"]).
type ContainerMutation struct {
	Kind      ContainerMutationKind
	Segments  []constraint.Segment
	KeyType   product.AbstractValue
	ValueMode flow.MapMutationValueMode
	ValueType product.AbstractValue
}

// ContainerMutationKey returns the canonical path key for a container mutation.
func ContainerMutationKey(m ContainerMutation) string {
	return containerMutationKindKey(m.Kind) + containerMutationKeyMode(m) + ":" + constraint.FormatSegments(m.Segments)
}

func containerMutationKeyMode(m ContainerMutation) string {
	if m.Kind != ContainerMutationTableElement && m.Kind != ContainerMutationMapElement {
		return ""
	}
	if m.Kind == ContainerMutationMapElement && m.ValueMode == flow.MapMutationValueUpdate {
		return ":update"
	}
	if !m.KeyType.IsZero() {
		return ":keyed"
	}
	return ":append"
}

func containerMutationKindKey(kind ContainerMutationKind) string {
	switch kind {
	case ContainerMutationMapElement:
		return "map"
	case ContainerMutationTableElement:
		return "table"
	default:
		return "container"
	}
}

// CapturedContainerMutations maps nested function symbols to container mutations
// they make to captured variables from parent scopes.
//
// Structure: nestedFuncSymbol -> capturedVarSymbol -> []ContainerMutation
type CapturedContainerMutations = map[cfg.SymbolID]map[cfg.SymbolID][]ContainerMutation

// ConstructorFields maps class symbols to field assignments captured in constructors.
// Structure: classSymbol -> fieldName -> fieldType. The field type carrier holds
// interned product.AbstractValue.
type ConstructorFields = map[cfg.SymbolID]map[string]product.AbstractValue

// Facts bundles one canonical interprocedural product slice. Most slices are
// stored per (graph, parent) pair; module-wide facts use ModuleFactsKey.
type Facts struct {
	FunctionFacts      FunctionFacts
	LiteralSigs        LiteralSigs
	CapturedTypes      CapturedTypes
	CapturedFields     CapturedFieldAssigns
	CapturedContainers CapturedContainerMutations
	ConstructorFields  ConstructorFields
}
