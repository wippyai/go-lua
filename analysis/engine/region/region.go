// Package region is the single generic authority for executing a semantic
// region to a fixed point.
//
// Region delegates the proven WTO mechanics to engine/solve instead of
// cloning them. It owns the execution contract, plan validation, policy
// instrumentation, final observations, statistics, and publication boundary.
// It has no dependency on checker, Lua, type, or concrete State packages.
package region

import (
	"context"
	"errors"
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/engine/solve"
)

// Observation is the final stable value and solve-local revision of one
// declared cell. Canceled runs publish no observations.
type Observation[Cell comparable, State any] struct {
	Cell     Cell
	Revision uint64
	Value    State
}

// Stats contains deterministic counters for one completed region execution.
type Stats struct {
	Cells          int
	Components     int
	TransferCalls  int
	WidenCalls     int
	NarrowCalls    int
	ObservationCnt int
	MaxRevision    uint64
}

// Options controls result-only instrumentation and never changes equations.
type Options struct {
	CaptureObservations bool
}

// System is the construction-friendly region form. Plan may supply a prebuilt
// immutable WTO; when non-nil Run validates and reuses it without rebuilding.
type System[Cell comparable, State any] struct {
	Lattice lattice.Lattice[State]
	Cells   []Cell
	Plan    *solve.WTOPlan[Cell]

	Successors        func(Cell) []Cell
	Initial           func(Cell) State
	InitialSparse     func(Cell) (State, bool)
	Transfer          func(Cell, func(Cell) State, func(Cell, State))
	TransferVersioned func(Cell, func(Cell) (State, uint64), func(Cell, State))

	WidenAt    func(Cell) bool
	WidenDelay func(Cell) int
	Abstract   func(Cell, State) State
	SolveStats *solve.Stats

	CaptureObservations bool
}

// Result is an atomically published completed region. Error returns always
// carry a zero Result.
type Result[Cell comparable, State any] struct {
	Values       map[Cell]State
	Revisions    map[Cell]uint64
	Observations []Observation[Cell, State]
	Stats        Stats
}

// Run builds a plan only when System.Plan is nil, then enters RunPrepared.
// A nil context selects the uncancelable batch path.
func Run[Cell comparable, State any](ctx context.Context, system System[Cell, State]) (Result[Cell, State], error) {
	if err := validateSystem(system); err != nil {
		return Result[Cell, State]{}, err
	}
	cells, err := uniqueCells(system.Cells)
	if err != nil {
		return Result[Cell, State]{}, err
	}
	plan := system.Plan
	if plan == nil {
		plan = solve.NewWTOPlan(cells, system.Successors)
	}
	equations := solve.EquationSystem[Cell, State]{
		Lattice: system.Lattice, Cells: cells,
		Initial: system.Initial, InitialSparse: system.InitialSparse,
		Transfer: system.Transfer, TransferVersioned: system.TransferVersioned,
		WidenAt: system.WidenAt, WidenDelay: system.WidenDelay,
		Abstract: system.Abstract, Stats: system.SolveStats,
	}
	return RunPrepared(ctx, equations, plan, Options{CaptureObservations: system.CaptureObservations})
}

// RunPrepared executes an existing equation system with a prebuilt validated
// WTO. This is the production migration seam: callers retain equation
// construction while region becomes the sole generic WTO execution authority.
func RunPrepared[Cell comparable, State any](ctx context.Context, equations solve.EquationSystem[Cell, State], plan *solve.WTOPlan[Cell], options Options) (Result[Cell, State], error) {
	if _, err := uniqueCells(equations.Cells); err != nil {
		return Result[Cell, State]{}, err
	}
	if plan == nil || !plan.Matches(equations.Cells) {
		return Result[Cell, State]{}, solve.ErrWTOPlanUncovered
	}
	if ctx != nil {
		if err := canceled(ctx); err != nil {
			return Result[Cell, State]{}, err
		}
	}

	stats := Stats{Cells: len(equations.Cells), Components: countComponents(plan.Elements())}
	domain := equations.Lattice
	if domain.Widen != nil {
		widen := domain.Widen
		domain.Widen = func(previous, next State) State {
			stats.WidenCalls++
			return widen(previous, next)
		}
	}
	if domain.Narrow != nil {
		narrow := domain.Narrow
		domain.Narrow = func(previous, next State) State {
			stats.NarrowCalls++
			return narrow(previous, next)
		}
	}
	equations.Lattice = domain
	localSolveStats := solve.Stats{}
	if equations.Stats == nil {
		equations.Stats = &localSolveStats
	}
	beforeTransfers := equations.Stats.TransferCalls

	var values map[Cell]State
	var revisions map[Cell]uint64
	var err error
	if ctx == nil {
		values, revisions, err = solve.SolveWTOWithVersions(equations, plan)
	} else {
		values, revisions, err = solve.SolveWTOContextWithVersions(ctx, equations, plan)
	}
	if err != nil {
		return Result[Cell, State]{}, err
	}
	if ctx != nil {
		if err := canceled(ctx); err != nil {
			return Result[Cell, State]{}, err
		}
	}
	stats.TransferCalls = equations.Stats.TransferCalls - beforeTransfers
	for _, revision := range revisions {
		if revision > stats.MaxRevision {
			stats.MaxRevision = revision
		}
	}
	var observations []Observation[Cell, State]
	if options.CaptureObservations {
		observations = make([]Observation[Cell, State], len(equations.Cells))
		for index, cell := range equations.Cells {
			observations[index] = Observation[Cell, State]{Cell: cell, Revision: revisions[cell], Value: values[cell]}
		}
		stats.ObservationCnt = len(observations)
	}
	return Result[Cell, State]{Values: values, Revisions: revisions, Observations: observations, Stats: stats}, nil
}

// BuildRetainedPrepared is the retained-provenance counterpart of
// RunPrepared. Region validates the execution boundary and delegates retained
// history ownership to solve; no second retained algorithm exists here.
func BuildRetainedPrepared[Cell comparable, State any](ctx context.Context, equations solve.EquationSystem[Cell, State], plan *solve.WTOPlan[Cell], budget solve.RetainedBudget) (map[Cell]State, map[Cell]uint64, *solve.RetainedSystem[Cell, State], error) {
	if _, err := uniqueCells(equations.Cells); err != nil {
		return nil, nil, nil, err
	}
	if plan == nil || !plan.Matches(equations.Cells) {
		return nil, nil, nil, solve.ErrWTOPlanUncovered
	}
	return solve.BuildRetainedWTO(ctx, equations, plan, budget)
}

func validateSystem[Cell comparable, State any](system System[Cell, State]) error {
	if len(system.Cells) == 0 {
		return errors.New("region: Cells is empty")
	}
	if system.Lattice.Bottom == nil || system.Lattice.Equal == nil || system.Lattice.LessOrEq == nil || system.Lattice.Join == nil {
		return errors.New("region: incomplete lattice")
	}
	if system.Transfer == nil {
		return errors.New("region: Transfer is nil")
	}
	if system.Plan == nil && system.Successors == nil {
		return errors.New("region: Successors is nil without a prebuilt Plan")
	}
	if system.WidenAt != nil && system.Lattice.Widen == nil {
		return errors.New("region: WidenAt requires lattice Widen")
	}
	return nil
}

func uniqueCells[Cell comparable](input []Cell) ([]Cell, error) {
	out := make([]Cell, 0, len(input))
	seen := make(map[Cell]struct{}, len(input))
	for _, cell := range input {
		if _, duplicate := seen[cell]; duplicate {
			return nil, fmt.Errorf("region: duplicate cell %v", cell)
		}
		seen[cell] = struct{}{}
		out = append(out, cell)
	}
	return out, nil
}

func countComponents[Cell comparable](elements []solve.WTOElement[Cell]) int {
	count := 0
	for _, element := range elements {
		if element.IsComponent() {
			count++
		}
		count += countComponents(element.Body)
	}
	return count
}

func canceled(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return errors.Join(solve.ErrCanceled, err)
	}
	return nil
}
