package flow

import (
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow/numeric"
	"github.com/wippyai/go-lua/types/lattice"
	latticeproduct "github.com/wippyai/go-lua/types/lattice/product"
)

// PointState is the unified per-program-point abstract state of the single
// canonical intraprocedural fixed point. It is the reduced product of the
// component domains a flow analysis tracks at a CFG point:
//
//   - Env: the value environment. Each typed value key (symbol, return slot,
//     contract slot) maps to its product.AbstractValue. An absent key denotes
//     product.Domain.Bottom() (no information), per MapLattice semantics — not
//     Lua nil.
//   - Cond: the DNF path condition that holds at this point.
//   - Num: the relational numeric / length state (a DBM over value keys). It
//     cannot live per-value because it expresses cross-value relations, so it is
//     a distinct component co-iterated in the same worklist.
//   - Rel: finite must-relations between point-local values, such as
//     multi-return sibling correlations. Join keeps only relations proven on
//     every incoming path.
//   - ReturnRel: finite must-relations proven by a return expression at this
//     point, projected into the interprocedural Summary.
//   - Cells: captured lexical cell values. Lua closures share mutable locations;
//     captured upvalues therefore live in the abstract state, not in a driver
//     side map. Captured reads/writes are ordinary point-state transfer once the
//     capture-cell migration is wired through transfer and Summary.
//   - CellEffects: caller-visible effects accumulated along the current path.
//     This is a transformer summary component, not store state: it records which
//     captured cells this function definitely or conditionally writes so Summary
//     can publish a call effect without confusing unchanged entry cells for
//     writes.
//   - ReceiverEffects: caller-visible effects accumulated along the current path
//     for mutable runtime argument slots. Method receiver writes are projected
//     here so the caller updates the concrete receiver place after the call,
//     instead of relying on prototype fallback entry evidence.
//   - PrototypeSelf: Lua split-pattern OOP receiver relation. The key is the
//     method-prototype symbol, and the value is the runtime instance value that
//     enters methods as `self`. It is a product-state axis, not a driver-side
//     module capture map.
//   - PrototypeInstances: flow-sensitive storage-place to prototype relation.
//     It remembers that a local/cell currently denotes a metatabled instance, so
//     later ordinary Place writes publish the updated value into PrototypeSelf.
//   - FunctionRefs: closure identity facts for function-valued storage paths.
//     The value domain carries the callable shape; this axis carries which
//     function body the path may denote, so summary projection can pair that
//     identity with the current capture-cell store.
//   - ClosureRefs: closure value facts for function-valued storage paths. This
//     refines FunctionRefs with the captured-cell and captured-function-ref stores
//     observed at closure creation, so returned/stored closures carry their
//     lexical environment as product state.
//   - StaticMembers: point-local must-facts for guarded exact static member
//     reads. This keeps branch-local presence/value precision for `.field`,
//     `["field"]`, `[""]`, and `[1]` paths out of typ.Record field overlays.
//   - KeyPresence: point-local must-facts for KeyOf(table, key), paired
//     keyed-iteration value-origin provenance, and live keys-array provenance.
//     Exact dynamic-index reads/writes consume this axis instead of scanning Cond
//     or the CFG.
//   - ValueOrigins: point-local must-facts for values derived from other values
//     (currently iterator variables from their source container). Backward
//     parameter demand consumes this axis so typed uses of a derived local can
//     constrain the source parameter inside the same fixed point.
//   - IndexWrites: point-local must-facts proving a dynamic-index replacement
//     write was admitted by the value-domain write law at this point. Observation
//     consumes this proof directly.
//
// PointState carries a lattice.Lattice (PointStateDomain) so the single generic
// solver in types/lattice/solver computes the least fixed point over it. The
// lattice is the componentwise product of the canonical component domains, so it
// inherits their laws (monotone Join,
// ACC-under-widen) by the product-of-lattices construction: a product of
// law-satisfying lattices is a law-satisfying lattice, and a product of
// ACC-under-widen components is ACC-under-widen, so the unified worklist
// terminates exactly when each component domain does.
//
// No adapter wraps the components. PointStateDomain populates each Lattice
// field by delegating to the component domains componentwise, exactly as
// product.Domain populates its fields over its axes.
type PointState struct {
	Env                map[ValueKey]product.AbstractValue
	Cond               constraint.Condition
	Num                *numeric.State
	Rel                PointRelations
	ReturnRel          ReturnRelations
	Cells              CaptureCells
	CellEffects        CaptureEffects
	ReceiverEffects    ReceiverEffects
	PrototypeSelf      PrototypeSelf
	PrototypeInstances PrototypeInstances
	FunctionRefs       FunctionRefs
	ClosureRefs        ClosureRefs
	StaticMembers      StaticMemberFacts
	KeyPresence        KeyPresenceFacts
	ValueOrigins       ValueOriginFacts
	IndexWrites        IndexWriteAdmissionFacts
}

// SymbolValue returns the low-level symbol slot value in ps.
//
// This helper is class-agnostic: when a Cells entry exists for the same symbol
// ID, Cells takes precedence over Env. It does not decide whether a lexical
// symbol should be stored in Env or Cells; canonical transfer owns that policy
// and should read through its symbol-storage boundary.
func SymbolValue(ps PointState, sym cfg.SymbolID) (product.AbstractValue, bool) {
	if sym == 0 {
		return product.AbstractValue{}, false
	}
	if av, ok := ps.Cells.Value(sym); ok && !av.IsZero() {
		return av, true
	}
	av, ok := ps.Env[SymbolValueKey(sym)]
	if !ok || av.IsZero() {
		return product.AbstractValue{}, false
	}
	return av, true
}

// ClonePointState returns a canonical, mutation-safe copy of ps.
//
// Transfer and narrowing code may speculatively update Env, Num, FunctionRefs,
// and ClosureRefs while deriving a successor state. Those mutable carriers are
// cloned here so predecessor states owned by the solver are never aliased. The
// remaining finite fact axes are persistent-by-construction: their update
// methods return new values, so copying the axis value is sufficient.
func ClonePointState(ps PointState) PointState {
	out := PointState{
		Env:                envDomain.Join(ps.Env, envDomain.Bottom()),
		Cond:               ps.Cond,
		Rel:                ps.Rel,
		ReturnRel:          ps.ReturnRel,
		Cells:              ps.Cells,
		CellEffects:        ps.CellEffects,
		ReceiverEffects:    ps.ReceiverEffects,
		PrototypeSelf:      ps.PrototypeSelf,
		PrototypeInstances: ps.PrototypeInstances,
		FunctionRefs:       FunctionRefsDomain.Join(ps.FunctionRefs, FunctionRefsDomain.Bottom()),
		ClosureRefs:        ClosureRefsDomain.Join(ps.ClosureRefs, ClosureRefsDomain.Bottom()),
		StaticMembers:      ps.StaticMembers,
		KeyPresence:        ps.KeyPresence,
		ValueOrigins:       ps.ValueOrigins,
		IndexWrites:        ps.IndexWrites,
	}
	if ps.Num != nil {
		out.Num = ps.Num.Clone()
	}
	return out
}

// envDomain lifts the value product pointwise over value keys: an absent key is
// product.Domain.Bottom(), Join/Widen are pointwise, and a key whose value is
// Bottom is canonicalized to absence so environments that denote the same
// function compare Equal.
var envDomain = latticeproduct.MapLattice[ValueKey](product.Domain)

// PointStateDomain is the abstract domain of PointState: the componentwise
// reduced product of envDomain, constraint.Domain, numeric.StateDomain, and the
// finite relation domains.
//
// Meet is nil: two of the three components (Env via product.Domain, and
// numeric.StateDomain) are forward-only with no greatest-lower-bound surface,
// so the product has no Meet and the LawSuite skips the meet-side laws. A
// product Meet would require every component to provide one.
var PointStateDomain = lattice.Lattice[PointState]{
	Bottom: func() PointState {
		return PointState{
			Env:                envDomain.Bottom(),
			Cond:               constraint.Domain.Bottom(),
			Num:                numeric.StateDomain.Bottom(),
			Rel:                PointRelationsDomain.Bottom(),
			ReturnRel:          ReturnRelationsDomain.Bottom(),
			Cells:              CaptureCellsDomain.Bottom(),
			CellEffects:        CaptureEffectsDomain.Bottom(),
			ReceiverEffects:    ReceiverEffectsDomain.Bottom(),
			PrototypeSelf:      PrototypeSelfDomain.Bottom(),
			PrototypeInstances: PrototypeInstancesDomain.Bottom(),
			FunctionRefs:       FunctionRefsDomain.Bottom(),
			ClosureRefs:        ClosureRefsDomain.Bottom(),
			StaticMembers:      StaticMemberFactsDomain.Bottom(),
			KeyPresence:        KeyPresenceFactsDomain.Bottom(),
			ValueOrigins:       ValueOriginFactsDomain.Bottom(),
			IndexWrites:        IndexWriteAdmissionFactsDomain.Bottom(),
		}
	},
	Top: func() PointState {
		return PointState{
			Env:                envDomain.Top(),
			Cond:               constraint.Domain.Top(),
			Num:                numeric.StateDomain.Top(),
			Rel:                PointRelationsDomain.Top(),
			ReturnRel:          ReturnRelationsDomain.Top(),
			Cells:              CaptureCellsDomain.Top(),
			CellEffects:        CaptureEffectsDomain.Top(),
			ReceiverEffects:    ReceiverEffectsDomain.Top(),
			PrototypeSelf:      PrototypeSelfDomain.Top(),
			PrototypeInstances: PrototypeInstancesDomain.Top(),
			FunctionRefs:       FunctionRefsDomain.Top(),
			ClosureRefs:        ClosureRefsDomain.Top(),
			StaticMembers:      StaticMemberFactsDomain.Top(),
			KeyPresence:        KeyPresenceFactsDomain.Top(),
			ValueOrigins:       ValueOriginFactsDomain.Top(),
			IndexWrites:        IndexWriteAdmissionFactsDomain.Top(),
		}
	},
	Equal: func(a, b PointState) bool {
		return envDomain.Equal(a.Env, b.Env) &&
			constraint.Domain.Equal(a.Cond, b.Cond) &&
			numeric.StateDomain.Equal(a.Num, b.Num) &&
			PointRelationsDomain.Equal(a.Rel, b.Rel) &&
			ReturnRelationsDomain.Equal(a.ReturnRel, b.ReturnRel) &&
			CaptureCellsDomain.Equal(a.Cells, b.Cells) &&
			CaptureEffectsDomain.Equal(a.CellEffects, b.CellEffects) &&
			ReceiverEffectsDomain.Equal(a.ReceiverEffects, b.ReceiverEffects) &&
			PrototypeSelfDomain.Equal(a.PrototypeSelf, b.PrototypeSelf) &&
			PrototypeInstancesDomain.Equal(a.PrototypeInstances, b.PrototypeInstances) &&
			FunctionRefsDomain.Equal(a.FunctionRefs, b.FunctionRefs) &&
			ClosureRefsDomain.Equal(a.ClosureRefs, b.ClosureRefs) &&
			StaticMemberFactsDomain.Equal(a.StaticMembers, b.StaticMembers) &&
			KeyPresenceFactsDomain.Equal(a.KeyPresence, b.KeyPresence) &&
			ValueOriginFactsDomain.Equal(a.ValueOrigins, b.ValueOrigins) &&
			IndexWriteAdmissionFactsDomain.Equal(a.IndexWrites, b.IndexWrites)
	},
	LessOrEq: func(a, b PointState) bool {
		return envDomain.LessOrEq(a.Env, b.Env) &&
			constraint.Domain.LessOrEq(a.Cond, b.Cond) &&
			numeric.StateDomain.LessOrEq(a.Num, b.Num) &&
			PointRelationsDomain.LessOrEq(a.Rel, b.Rel) &&
			ReturnRelationsDomain.LessOrEq(a.ReturnRel, b.ReturnRel) &&
			CaptureCellsDomain.LessOrEq(a.Cells, b.Cells) &&
			CaptureEffectsDomain.LessOrEq(a.CellEffects, b.CellEffects) &&
			ReceiverEffectsDomain.LessOrEq(a.ReceiverEffects, b.ReceiverEffects) &&
			PrototypeSelfDomain.LessOrEq(a.PrototypeSelf, b.PrototypeSelf) &&
			PrototypeInstancesDomain.LessOrEq(a.PrototypeInstances, b.PrototypeInstances) &&
			FunctionRefsDomain.LessOrEq(a.FunctionRefs, b.FunctionRefs) &&
			ClosureRefsDomain.LessOrEq(a.ClosureRefs, b.ClosureRefs) &&
			StaticMemberFactsDomain.LessOrEq(a.StaticMembers, b.StaticMembers) &&
			KeyPresenceFactsDomain.LessOrEq(a.KeyPresence, b.KeyPresence) &&
			ValueOriginFactsDomain.LessOrEq(a.ValueOrigins, b.ValueOrigins) &&
			IndexWriteAdmissionFactsDomain.LessOrEq(a.IndexWrites, b.IndexWrites)
	},
	Join: func(a, b PointState) PointState {
		return PointState{
			Env:                envDomain.Join(a.Env, b.Env),
			Cond:               constraint.Domain.Join(a.Cond, b.Cond),
			Num:                numeric.StateDomain.Join(a.Num, b.Num),
			Rel:                PointRelationsDomain.Join(a.Rel, b.Rel),
			ReturnRel:          ReturnRelationsDomain.Join(a.ReturnRel, b.ReturnRel),
			Cells:              CaptureCellsDomain.Join(a.Cells, b.Cells),
			CellEffects:        CaptureEffectsDomain.Join(a.CellEffects, b.CellEffects),
			ReceiverEffects:    ReceiverEffectsDomain.Join(a.ReceiverEffects, b.ReceiverEffects),
			PrototypeSelf:      PrototypeSelfDomain.Join(a.PrototypeSelf, b.PrototypeSelf),
			PrototypeInstances: PrototypeInstancesDomain.Join(a.PrototypeInstances, b.PrototypeInstances),
			FunctionRefs:       FunctionRefsDomain.Join(a.FunctionRefs, b.FunctionRefs),
			ClosureRefs:        ClosureRefsDomain.Join(a.ClosureRefs, b.ClosureRefs),
			StaticMembers:      StaticMemberFactsDomain.Join(a.StaticMembers, b.StaticMembers),
			KeyPresence:        KeyPresenceFactsDomain.Join(a.KeyPresence, b.KeyPresence),
			ValueOrigins:       ValueOriginFactsDomain.Join(a.ValueOrigins, b.ValueOrigins),
			IndexWrites:        IndexWriteAdmissionFactsDomain.Join(a.IndexWrites, b.IndexWrites),
		}
	},
	Meet: nil,
	Widen: func(prev, next PointState) PointState {
		return PointState{
			Env:                envDomain.Widen(prev.Env, next.Env),
			Cond:               constraint.Domain.Widen(prev.Cond, next.Cond),
			Num:                numeric.StateDomain.Widen(prev.Num, next.Num),
			Rel:                PointRelationsDomain.Widen(prev.Rel, next.Rel),
			ReturnRel:          ReturnRelationsDomain.Widen(prev.ReturnRel, next.ReturnRel),
			Cells:              CaptureCellsDomain.Widen(prev.Cells, next.Cells),
			CellEffects:        CaptureEffectsDomain.Widen(prev.CellEffects, next.CellEffects),
			ReceiverEffects:    ReceiverEffectsDomain.Widen(prev.ReceiverEffects, next.ReceiverEffects),
			PrototypeSelf:      PrototypeSelfDomain.Widen(prev.PrototypeSelf, next.PrototypeSelf),
			PrototypeInstances: PrototypeInstancesDomain.Widen(prev.PrototypeInstances, next.PrototypeInstances),
			FunctionRefs:       FunctionRefsDomain.Widen(prev.FunctionRefs, next.FunctionRefs),
			ClosureRefs:        ClosureRefsDomain.Widen(prev.ClosureRefs, next.ClosureRefs),
			StaticMembers:      StaticMemberFactsDomain.Widen(prev.StaticMembers, next.StaticMembers),
			KeyPresence:        KeyPresenceFactsDomain.Widen(prev.KeyPresence, next.KeyPresence),
			ValueOrigins:       ValueOriginFactsDomain.Widen(prev.ValueOrigins, next.ValueOrigins),
			IndexWrites:        IndexWriteAdmissionFactsDomain.Widen(prev.IndexWrites, next.IndexWrites),
		}
	},
}
