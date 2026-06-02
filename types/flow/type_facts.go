// type_facts.go provides phase-safe type access for the type checker.
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
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/domain/value/product"
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// TypeFacts provides phase-safe type access by separating declared types from refined types.
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
// evidence. Post-fixpoint reducers consume this surface instead of naming a
// concrete Solution carrier.
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

// PathReadPhase identifies the abstract state slice a path observation reads.
type PathReadPhase uint8

const (
	// PathReadCurrent observes the normal point value, preferring the point's
	// narrowed out/in value according to the producer's default policy.
	PathReadCurrent PathReadPhase = iota
	// PathReadPre observes the point-entry value before same-node effects.
	PathReadPre
	// PathReadPost observes the point-exit value after same-node effects.
	PathReadPost
)

// PathObservationSource records which abstract fact family answered a path
// observation. It is diagnostic metadata and keeps policy tests precise without
// exposing producer internals.
type PathObservationSource uint8

const (
	PathObservationUnknown PathObservationSource = iota
	PathObservationDirectPath
	PathObservationSolvedFlow
	PathObservationFactProjection
	PathObservationConditionProof
	PathObservationDeclared
)

// PathObservationQuery is the normalized, AST-free request for a source path's
// observed type at one CFG point. AST expressions are lowered to constraint.Path
// before this boundary; the query is therefore cache-friendly and independent of
// parser node identity.
type PathObservationQuery struct {
	Point cfg.Point
	Path  constraint.Path
	Phase PathReadPhase
	// StrictPhase forbids falling back to another phase when the requested phase
	// has no fact. Assignment sources that reference their own target use this to
	// read the pre-write value only.
	StrictPhase bool
	// AllowConditionProof permits condition-only proof projection to participate
	// in the observation. Expression reads use it; assignment-source reads keep it
	// disabled for parity with their existing solved-state policy.
	AllowConditionProof bool
	LocalCondition      *constraint.Condition
	PreserveProof       bool
	IndexRead           *PathObservationIndexRead
}

// PathObservationIndexRead is the normalized proof context for an indexed read
// expression. It is produced at the AST boundary and consumed by path observation
// without retaining parser node identity.
type PathObservationIndexRead struct {
	Container typ.Type
	TablePath constraint.Path
	KeyPath   constraint.Path

	IndexVarPath   constraint.Path
	IndexVarOffset int64
	HasIndexVar    bool

	LengthPath   constraint.Path
	LengthOffset int64
	HasLength    bool

	LiteralIndex    int64
	HasLiteralIndex bool
}

// PathObservation is the high-level result of observing one path through the
// reduced product.
type PathObservation struct {
	Type     typ.Type
	State    TypeState
	Source   PathObservationSource
	Declared typ.Type
	Solved   typ.Type
	Proof    typ.Type
}

// Resolved reports whether the observation carries a usable type.
func (o PathObservation) Resolved() bool {
	return o.State == StateResolved && !typ.IsAbsentOrUnknown(o.Type)
}

// PathObservationFacts owns the high-level path-read policy over a solved
// abstract state: direct path facts, solved-flow state, condition proofs, and
// declared fallback reconciliation. It is intentionally higher-level than
// FlowOps, which exposes primitive solved-flow queries only.
type PathObservationFacts interface {
	ObservePath(PathObservationQuery) PathObservation
}

// PathChildQuery is the normalized, AST-free request for the finite child facts
// materialized below one path at a CFG point. It deliberately returns only
// finite facts already present in the abstract state; it must not derive an
// unbounded descendant tree from recursive product types.
type PathChildQuery struct {
	Point cfg.Point
	Path  constraint.Path
	Phase PathReadPhase
}

// PathChildFacts exposes finite child path facts below a path. It is the child
// counterpart of PathObservationFacts: producers own how finite facts are
// represented, while consumers receive stable path/type pairs.
type PathChildFacts interface {
	ObserveChildPaths(PathChildQuery) []PathFact
}

// LengthFacts exposes a container symbol's proven length lower bound at a CFG
// point, the numeric proof a flow without a path-sensitive Solution (the canonical
// flow) carries in its numeric component. The observation surface consults it to
// refine an index read `arr[k]` that the length proof shows is provably in range,
// the read-side counterpart of the path-sensitive Solution.LengthBoundsAt. A flow
// that does not implement it offers no length proof, so the read stays optional.
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

// NumericFacts exposes the per-point numeric proofs a Solution-less flow (the
// canonical flow) carries in its numeric component, keyed by the value's SYMBOL
// (never a name or a versioned key, so the surface is independent of how the flow
// keys its numeric state internally). The observation surface consults it to refine
// a dynamic index read `arr[i]` whose index variable a loop bound or guard proves
// in range, the read-side counterpart of the path-sensitive Solution's BoundsAt /
// ArrayLenBoundWithOffsetAt. A flow that does not implement it offers no numeric
// proof, so a dynamic-index read stays optional (sound).
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

// Compile-time check that Solution implements TypeFacts.
var _ TypeFacts = (*Solution)(nil)

// DeclaredAt returns the declared (annotated) type for a symbol.
// Lookup order: LiteralTypes > SiblingTypes (captured vars) > DeclaredTypes.
// Returns typ.Unknown if no declaration exists.
func (s *Solution) DeclaredAt(p cfg.Point, sym cfg.SymbolID) TypedValue {
	if s == nil || s.inputs == nil || sym == 0 {
		return TypedValue{Type: typ.Unknown, State: StateUnknown}
	}
	// For explicitly annotated symbols, prefer the declared type over literal overlays.
	if s.inputs.AnnotatedVars != nil && s.inputs.AnnotatedVars[sym] {
		if t := s.inputs.DeclaredTypes[sym]; t != nil {
			return TypedValue{Type: t, State: StateResolved}
		}
	}
	// Check literal types first (function literals synthesized in current scope),
	// but do not override explicit annotations.
	if s.inputs.AnnotatedVars == nil || !s.inputs.AnnotatedVars[sym] {
		if t := s.inputs.LiteralTypes[sym]; t != nil {
			return TypedValue{Type: t, State: StateResolved}
		}
	}
	// Check sibling types (captured variables from parent scope)
	if t := s.inputs.SiblingTypes[sym]; t != nil {
		return TypedValue{Type: t, State: StateResolved}
	}
	if t := s.inputs.DeclaredTypes[sym]; t != nil {
		return TypedValue{Type: t, State: StateResolved}
	}
	return TypedValue{Type: typ.Unknown, State: StateUnknown}
}

// RefinedAt returns the flow-narrowed type for a symbol at a point.
// Returns TypedValue with nil Type if no refinement exists.
func (s *Solution) RefinedAt(p cfg.Point, sym cfg.SymbolID) TypedValue {
	if s == nil || s.inputs == nil || s.pkResolver == nil || sym == 0 {
		return TypedValue{Type: nil, State: StateUnknown}
	}

	// Use canonical key for lookup
	path := constraint.Path{Symbol: sym}
	key := s.pkResolver.KeyAt(p, path)
	if key == "" {
		return TypedValue{Type: nil, State: StateUnknown}
	}

	// Check if we have a narrowed value for this version
	if t := s.valueAtPoint(p, string(key)); t != nil {
		narrowed := s.NarrowedTypeAt(p, path)
		if narrowed != nil {
			return TypedValue{Type: narrowed, State: StateResolved}
		}
		return TypedValue{Type: t, State: StateResolved}
	}

	return TypedValue{Type: nil, State: StateUnknown}
}

// EffectiveTypeAt returns the best available type: refined if exists, else declared.
func (s *Solution) EffectiveTypeAt(p cfg.Point, sym cfg.SymbolID) TypedValue {
	refined := s.RefinedAt(p, sym)
	if refined.Type != nil {
		declared := s.DeclaredAt(p, sym)
		if declared.Type != nil && declared.State == StateResolved && !typ.IsUnknown(refined.Type) {
			if reconciled, ok := value.ReconcilePathFactWithDeclaredRead(refined.Type, declared.Type); ok && reconciled != nil {
				return TypedValue{Type: reconciled, State: StateResolved}
			}
		}
		// For annotated symbols, only accept refinements that are subtypes of the declared type.
		// This prevents unsound narrowing that drops required fields from annotated records.
		if s != nil && s.inputs != nil && s.inputs.AnnotatedVars != nil && s.inputs.AnnotatedVars[sym] {
			if declared.Type != nil && declared.State == StateResolved {
				if !subtype.IsSubtype(refined.Type, declared.Type) {
					return declared
				}
				// If the declared type is not optional/union, keep its structural kind authoritative.
				declaredBase := unwrap.Alias(declared.Type)
				if _, ok := declaredBase.(*typ.Optional); !ok {
					if _, ok := declaredBase.(*typ.Union); !ok {
						if refined.Type.Kind() != declaredBase.Kind() {
							return declared
						}
					}
				}
			}
		}
		return refined
	}
	return s.DeclaredAt(p, sym)
}

// IsAnnotated returns true if the symbol has an explicit type annotation.
func (s *Solution) IsAnnotated(sym cfg.SymbolID) bool {
	if s == nil || s.inputs == nil {
		return false
	}
	return s.inputs.AnnotatedVars[sym]
}
