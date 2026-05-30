package flow

import (
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow/numeric"
	"github.com/wippyai/go-lua/types/lattice"
	latticeproduct "github.com/wippyai/go-lua/types/lattice/product"
)

// PointState is the unified per-program-point abstract state of the single
// canonical intraprocedural fixed point. It is the reduced product of the three
// component domains a flow analysis tracks at a CFG point:
//
//   - Env: the value environment. Each value key (SSA path, field, virtual
//     contract slot) maps to its product.AbstractValue. An absent key denotes
//     product.Domain.Bottom() (no information), per MapLattice semantics — not
//     Lua nil.
//   - Cond: the DNF path condition that holds at this point.
//   - Num: the relational numeric / length state (a DBM over value keys). It
//     cannot live per-value because it expresses cross-value relations, so it is
//     a distinct component co-iterated in the same worklist.
//
// PointState carries a lattice.Lattice (PointStateDomain) so the single generic
// solver in types/lattice/solver computes the least fixed point over it. The
// lattice is the componentwise product of the canonical component domains —
// product.Domain lifted pointwise over keys by MapLattice, constraint.Domain,
// and numeric.StateDomain — so it inherits their laws (monotone Join,
// ACC-under-widen) by the product-of-lattices construction: a product of
// law-satisfying lattices is a law-satisfying lattice, and a product of
// ACC-under-widen components is ACC-under-widen, so the unified worklist
// terminates exactly when each component domain does.
//
// No adapter wraps the components. PointStateDomain populates each Lattice
// field by delegating to the component domains componentwise, exactly as
// product.Domain populates its fields over its axes.
type PointState struct {
	Env  map[string]product.AbstractValue
	Cond constraint.Condition
	Num  *numeric.State
}

// envDomain lifts the value product pointwise over value keys: an absent key is
// product.Domain.Bottom(), Join/Widen are pointwise, and a key whose value is
// Bottom is canonicalized to absence so environments that denote the same
// function compare Equal.
var envDomain = latticeproduct.MapLattice[string](product.Domain)

// PointStateDomain is the abstract domain of PointState: the componentwise
// reduced product of envDomain, constraint.Domain, and numeric.StateDomain.
//
// Meet is nil: two of the three components (Env via product.Domain, and
// numeric.StateDomain) are forward-only with no greatest-lower-bound surface,
// so the product has no Meet and the LawSuite skips the meet-side laws. A
// product Meet would require every component to provide one.
var PointStateDomain = lattice.Lattice[PointState]{
	Bottom: func() PointState {
		return PointState{
			Env:  envDomain.Bottom(),
			Cond: constraint.Domain.Bottom(),
			Num:  numeric.StateDomain.Bottom(),
		}
	},
	Top: func() PointState {
		return PointState{
			Env:  envDomain.Top(),
			Cond: constraint.Domain.Top(),
			Num:  numeric.StateDomain.Top(),
		}
	},
	Equal: func(a, b PointState) bool {
		return envDomain.Equal(a.Env, b.Env) &&
			constraint.Domain.Equal(a.Cond, b.Cond) &&
			numeric.StateDomain.Equal(a.Num, b.Num)
	},
	LessOrEq: func(a, b PointState) bool {
		return envDomain.LessOrEq(a.Env, b.Env) &&
			constraint.Domain.LessOrEq(a.Cond, b.Cond) &&
			numeric.StateDomain.LessOrEq(a.Num, b.Num)
	},
	Join: func(a, b PointState) PointState {
		return PointState{
			Env:  envDomain.Join(a.Env, b.Env),
			Cond: constraint.Domain.Join(a.Cond, b.Cond),
			Num:  numeric.StateDomain.Join(a.Num, b.Num),
		}
	},
	Meet: nil,
	Widen: func(prev, next PointState) PointState {
		return PointState{
			Env:  envDomain.Widen(prev.Env, next.Env),
			Cond: constraint.Domain.Widen(prev.Cond, next.Cond),
			Num:  numeric.StateDomain.Widen(prev.Num, next.Num),
		}
	},
}
