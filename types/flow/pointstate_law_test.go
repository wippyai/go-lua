package flow

import (
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow/numeric"
	"github.com/wippyai/go-lua/types/lattice"
	"github.com/wippyai/go-lua/types/typ"
)

// TestPointStateDomain_Laws validates the canonical intraprocedural carrier.
//
// PointStateDomain is the componentwise reduced product of three independently
// law-tested domains (envDomain over product.Domain, constraint.Domain,
// numeric.StateDomain). A product of law-satisfying lattices is itself a
// law-satisfying lattice, so the laws must hold here by construction; this
// suite exists to catch a COMPOSITION bug — a field delegating to the wrong
// component, a component left nil where the carrier requires a value, or a
// Meet wired where a component lacks one — rather than to re-prove the
// components.
//
// The sample crosses each component independently (one component non-trivial
// at a time) and jointly (all three non-trivial), so a swapped delegation
// surfaces as an antisymmetry / upper-bound / termination violation.
func TestPointStateDomain_Laws(t *testing.T) {
	lattice.LawSuite[PointState]{
		Name:   "PointState",
		Domain: PointStateDomain,
		Sample: pointStateSample(),
		Format: formatPointState,
	}.Run(t)
}

// pointStateSample builds Bottom, Top, and a structural cross-section in which
// each component varies independently and jointly. Every PointState sets all
// three fields to a valid component element: Env may be nil (MapLattice reads
// absence as product.Domain.Bottom()), but Cond and Num must be real domain
// values, never their Go zero value.
func pointStateSample() []PointState {
	x := constraint.Path{Root: "x", Symbol: cfg.SymbolID(1)}
	y := constraint.Path{Root: "y", Symbol: cfg.SymbolID(2)}

	// Condition cross-section: domain extremes plus two finite conditions.
	condTruthy := constraint.FromConstraints(constraint.Truthy{Path: x})
	condTwo := constraint.And(
		constraint.FromConstraints(constraint.Truthy{Path: x}),
		constraint.FromConstraints(constraint.NotNil{Path: y}),
	)

	// Numeric cross-section: domain extremes plus a bounded state.
	numBounded := numeric.NewState()
	numBounded.ApplyGeConst("x", 0)
	numBounded.ApplyLeConst("x", 100)

	// Env cross-section: empty (Bottom), single key, multi key, top sentinel.
	envOne := map[string]product.AbstractValue{"x": product.FromType(typ.String)}
	envTwo := map[string]product.AbstractValue{
		"x": product.FromType(typ.Number),
		"y": product.FromType(typ.Integer),
	}

	mk := func(env map[string]product.AbstractValue, cond constraint.Condition, num *numeric.State) PointState {
		return PointState{Env: env, Cond: cond, Num: num}
	}

	return []PointState{
		PointStateDomain.Bottom(),
		PointStateDomain.Top(),

		// One component non-trivial at a time, the other two at Bottom.
		mk(envOne, constraint.Domain.Bottom(), numeric.StateDomain.Bottom()),
		mk(envDomain.Bottom(), condTruthy, numeric.StateDomain.Bottom()),
		mk(envDomain.Bottom(), constraint.Domain.Bottom(), numBounded),

		// Pairs and a fully-mixed point.
		mk(envTwo, condTwo, numeric.StateDomain.Bottom()),
		mk(envOne, constraint.Domain.Bottom(), numBounded),
		mk(envTwo, condTwo, numBounded),

		// One component at Top, others finite — exercises the envDomain top
		// sentinel and condition/numeric Top against finite neighbours.
		mk(envDomain.Top(), condTruthy, numBounded),
		mk(envOne, constraint.Domain.Top(), numeric.StateDomain.Top()),
	}
}

func formatPointState(p PointState) string {
	return fmt.Sprintf("{Env:%v Cond:%v Num:%v}",
		p.Env, constraint.Domain.Equal(p.Cond, constraint.Domain.Top()), numeric.StateDomain.Equal(p.Num, numeric.StateDomain.Top()))
}
