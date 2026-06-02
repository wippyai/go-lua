package state

import (
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/flow/numeric"
	"github.com/wippyai/go-lua/types/lattice"
	"github.com/wippyai/go-lua/types/typ"
)

// TestFunctionStateDomain_Laws validates the canonical per-function carrier:
// the componentwise reduced product of pointsDomain (the per-point states) and
// paramevidence.ContractDomain (the per-parameter contract demand). Both halves
// are independently law-tested, so the product satisfies the laws by
// construction; this suite catches a COMPOSITION bug — a half delegating to the
// wrong component, or a Meet wired where a component lacks one — and exercises
// both maps non-trivially so a swapped delegation surfaces as a law violation.
func TestFunctionStateDomain_Laws(t *testing.T) {
	lattice.LawSuite[FunctionState]{
		Name:   "FunctionState",
		Domain: FunctionStateDomain,
		Sample: functionStateSample(),
		Format: formatFunctionState,
	}.Run(t)
}

// functionStateSample crosses both halves independently and jointly. Each
// PointState and ParamContract is a valid component element; an empty/nil map is
// the component Bottom per MapLattice semantics.
func functionStateSample() []FunctionState {
	psMixed := flow.PointState{
		Env:  map[flow.ValueKey]product.AbstractValue{flow.ValueKey("x"): product.FromType(typ.String)},
		Cond: constraint.Domain.Bottom(),
		Num:  numeric.StateDomain.Bottom(),
	}
	psTop := flow.PointStateDomain.Top()

	avString := product.FromType(typ.String)
	avNumber := product.FromType(typ.Number)

	mk := func(points map[cfg.Point]flow.PointState, contracts paramevidence.Contracts) FunctionState {
		return FunctionState{Points: points, Contracts: contracts}
	}

	return []FunctionState{
		FunctionStateDomain.Bottom(),
		FunctionStateDomain.Top(),

		// One half non-trivial at a time.
		mk(map[cfg.Point]flow.PointState{cfg.Point(0): psMixed}, nil),
		mk(nil, paramevidence.Contracts{0: avString}),

		// Both halves non-trivial, single and multi key.
		mk(map[cfg.Point]flow.PointState{cfg.Point(0): psMixed, cfg.Point(1): psTop},
			paramevidence.Contracts{0: avString, 1: avNumber}),
		mk(map[cfg.Point]flow.PointState{cfg.Point(3): psMixed},
			paramevidence.Contracts{2: avNumber}),
	}
}

func formatFunctionState(f FunctionState) string {
	return fmt.Sprintf("{Points:%d-pts Contracts:%v}", len(f.Points), len(f.Contracts))
}
