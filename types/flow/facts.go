// facts.go provides mode-safe type access for the type checker.
//
// During type checking, it's critical to distinguish between declared types
// (from annotations) and refined types (from flow analysis). Mixing these
// can cause "early synthesis poisoning" where prematurely narrowed types
// influence later analysis incorrectly.
//
// The TypeFacts interface provides clean separation:
//   - DeclaredAt: Only annotation-based types
//   - RefinedAt: Only flow-narrowed types
//   - EffectiveTypeAt: Best available type for practical use
package flow

import (
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

// TypeFacts provides mode-safe type access by separating declared types from refined types.
//
// The interface has three access methods with distinct semantics:
//
//   - DeclaredAt: Returns source/static declaration facts only. It never
//     includes immutable value bindings such as named-function projections.
//
//   - RefinedAt: Returns type from flow analysis only. Returns nil Type if the
//     symbol has no flow-narrowed type at this point.
//
//   - EffectiveTypeAt: Returns refined type if available, else declared type.
//     This is the practical "best known type" for error checking.
//
// BindingValueFacts carries immutable binding values such as named-function
// projections. Keeping them outside DeclaredAt prevents early synthesis poisoning
// where:
//  1. A variable gets an imprecise type from RHS before narrowing
//  2. That type influences constraint extraction
//  3. Constraints based on the imprecise type cause false errors or missed errors
type TypeFacts interface {
	// DeclaredAt returns the declared (annotated) type for a symbol at a point.
	// Returns typ.Unknown if no declaration exists.
	DeclaredAt(p cfg.Point, sym cfg.SymbolID) TypedValue

	// RefinedAt returns the flow-narrowed type for a symbol at a point.
	// Returns TypedValue with nil Type if no refinement exists.
	RefinedAt(p cfg.Point, sym cfg.SymbolID) TypedValue

	// EffectiveTypeAt returns the best available type: refined if exists, else declared, else unknown.
	EffectiveTypeAt(p cfg.Point, sym cfg.SymbolID) TypedValue

	// IsAnnotated returns true if the symbol has an explicit type annotation.
	IsAnnotated(sym cfg.SymbolID) bool
}

// PathFact is a finite abstract-state fact for a concrete flow path.
type PathFact struct {
	Path constraint.Path
	Type typ.Type
}

// AnnotatedDeclaredPathAt projects an explicit source annotation through a
// static path. It intentionally ignores inferred declarations, binding values,
// and solved refinements: callers use it when annotation authority matters, such
// as deciding whether an indexed write may widen a target shape.
func AnnotatedDeclaredPathAt(facts TypeFacts, p cfg.Point, path constraint.Path) TypedValue {
	if facts == nil || path.Symbol == 0 || !facts.IsAnnotated(path.Symbol) {
		return TypedValue{Type: typ.Unknown, State: StateUnknown}
	}
	root := facts.DeclaredAt(p, path.Symbol)
	if root.State != StateResolved || root.Type == nil || typ.IsUnknown(root.Type) {
		return TypedValue{Type: typ.Unknown, State: StateUnknown}
	}
	current := root.Type
	for _, segment := range path.Segments {
		next, ok := declaredSegmentType(current, segment)
		if !ok || next == nil {
			return TypedValue{Type: typ.Unknown, State: StateUnknown}
		}
		current = next
	}
	return TypedValue{Type: current, State: StateResolved}
}

// AnnotatedDeclaredPathSealed reports whether a path is governed by a
// non-refinable explicit annotation. Fresh or inferred tables are not sealed by
// this predicate; they must flow through the mutable product-domain write laws.
func AnnotatedDeclaredPathSealed(facts TypeFacts, p cfg.Point, path constraint.Path) bool {
	tv := AnnotatedDeclaredPathAt(facts, p, path)
	return tv.State == StateResolved &&
		tv.Type != nil &&
		!typ.IsAbsentOrUnknown(tv.Type) &&
		!typ.IsRefinableAnnotation(tv.Type)
}

func declaredSegmentType(t typ.Type, segment constraint.Segment) (typ.Type, bool) {
	switch segment.Kind {
	case constraint.SegmentField, constraint.SegmentIndexString:
		next, ok := querycore.Field(t, segment.Name)
		if ok && next != nil {
			return next, true
		}
		return querycore.Index(t, typ.LiteralString(segment.Name))
	case constraint.SegmentIndexInt:
		return querycore.Index(t, typ.LiteralInt(int64(segment.Index)))
	default:
		return nil, false
	}
}

// BindingValueFacts exposes immutable value-binding facts, such as a named
// function binding's canonical signature. These facts are not source
// declarations and must not be returned by DeclaredAt.
type BindingValueFacts interface {
	BindingValueAt(p cfg.Point, sym cfg.SymbolID) TypedValue
}

// PathFacts exposes a flow-refined value below a root symbol. It is the path-keyed
// counterpart of RefinedAt for flows whose abstract state stores product/path
// evidence directly; consumers still reconcile the returned value against declared
// source types before using it at a checking boundary.
type PathFacts interface {
	// RefinedPathAt returns the flow-refined type for path at point p. It returns a
	// TypedValue with nil Type when no sound path refinement is available.
	RefinedPathAt(p cfg.Point, path constraint.Path) TypedValue
}

// ProductValue pairs a value-domain product fact with its resolution state. It is
// the product-valued counterpart of TypedValue for flows whose semantic facts live
// in AbstractValue rather than typ.Type.
type ProductValue struct {
	Value product.AbstractValue
	State TypeState
}

// ProductFacts exposes flow-refined product facts without projecting through
// typ.Type. Consumers use this when semantic carrier evidence (for example
// gradual-top provenance) matters at a checking boundary.
type ProductFacts interface {
	RefinedValueAt(p cfg.Point, sym cfg.SymbolID) ProductValue
}

// ProductPathFacts is the path-keyed counterpart of ProductFacts.
type ProductPathFacts interface {
	RefinedPathValueAt(p cfg.Point, path constraint.Path) ProductValue
}

// AssignmentSourceFacts exposes the source-owned RHS value for an assignment
// source. It evaluates AssignmentSource against solved abstract state without
// reconciling the result against the target annotation/static slot type.
type AssignmentSourceFacts interface {
	AssignmentSourceValueAt(p cfg.Point, target constraint.Path, source AssignmentSource) typ.Type
}

// TransferValueFacts exposes solved values produced by AST-free transfer
// evidence. Post-fixpoint reducers consume this producer-neutral surface.
type TransferValueFacts interface {
	AssignedValueTypeAt(p cfg.Point, target constraint.Path, static typ.Type, source AssignmentSource) typ.Type
	MutatorValueTypeAt(p cfg.Point, valuePath constraint.Path, staticType typ.Type, template ValueTemplate) typ.Type
	MutatorKeyTypeAt(p cfg.Point, keyPath constraint.Path, staticType typ.Type) typ.Type
}

// IndexWriteFacts exposes the solved transfer proof that a dynamic indexed
// replacement write was admitted by the abstract store law at a point.
type IndexWriteFacts interface {
	IndexWriteAdmission(q IndexWriteQuery) (typ.Type, bool)
}

// ConditionProofFacts exposes condition-only proof queries over the converged
// point condition. Implementations must not re-enter general narrowed-state
// transfer to answer these queries; they project the finite point condition over
// a finite seed/root type environment.
type ConditionProofFacts interface {
	ConditionAt(p cfg.Point) constraint.Condition
	ProvesTypeAt(p cfg.Point, path constraint.Path, t typ.Type) bool
	ConditionTypeAt(p cfg.Point, path constraint.Path) typ.Type
	ConditionedTypeAt(p cfg.Point, path constraint.Path, extra constraint.Condition) typ.Type
	ConditionedSeedTypeAt(p cfg.Point, seedPath constraint.Path, seedType typ.Type, queryPath constraint.Path, extra constraint.Condition) typ.Type
}

// ConstFacts exposes immutable constant-value facts by symbol identity. Names are
// resolved at the caller boundary; the flow fact surface stays symbol-keyed.
type ConstFacts interface {
	ConstValueAtSym(p cfg.Point, sym cfg.SymbolID) *ConstValue
}

// LengthFacts exposes a container symbol's proven length lower bound at a CFG
// point. The observation surface consults it to refine an index read `arr[k]`
// that the length proof shows is provably in range. A producer that does not
// implement it offers no length proof, so the read stays optional.
type LengthFacts interface {
	// LengthLowerBoundAt returns the proven lower bound on #sym (the container's
	// length) entering point p, and whether such a bound is known.
	LengthLowerBoundAt(p cfg.Point, sym cfg.SymbolID) (lower int64, ok bool)
}

// PathLengthFacts exposes length lower bounds for containers below a root symbol
// (for example #result.items). It is the path-keyed counterpart of LengthFacts;
// callers should prefer it when the indexed container path has segments.
type PathLengthFacts interface {
	LengthLowerBoundForPathAt(p cfg.Point, path constraint.Path) (lower int64, ok bool)
}

// NumericFacts exposes per-point numeric proofs keyed by the value's symbol
// (never a name or a versioned key, so the surface is independent of how the
// producer keys its numeric state internally). The observation surface consults
// it to refine a dynamic index read `arr[i]` whose index variable a loop bound
// or guard proves in range. A producer that does not implement it offers no
// numeric proof, so a dynamic-index read stays optional.
type NumericFacts interface {
	// NumericBoundsAt returns the proven integer bounds on sym entering point p,
	// using transitive (theory) inference where relations are present.
	NumericBoundsAt(p cfg.Point, sym cfg.SymbolID) (lower, upper int64, ok bool)

	// ArrayLenRefAt returns the container symbol arr and constant offset of a proven
	// `sym <= #arr + offset` relation on sym entering point p, and whether one holds.
	ArrayLenRefAt(p cfg.Point, sym cfg.SymbolID) (arr cfg.SymbolID, offset int64, ok bool)
}

// DeclaredTypes maps SymbolID to its declared type.
//
// This type alias documents that the map should contain only annotation-derived
// types, not types synthesized from expression analysis. This distinction is
// enforced by the code that populates the map, not by the type system.
type DeclaredTypes = map[cfg.SymbolID]typ.Type
