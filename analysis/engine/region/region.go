// Package region is the single generic authority for executing a semantic
// region to a fixed point.
//
// This first POC deliberately delegates the proven worklist/WTO mechanics to
// engine/solve instead of cloning them. Region owns the public execution
// contract, deterministic plan construction, policy instrumentation, final
// observations, statistics, cancellation publication boundary, and results.
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
// declared cell. Observations are created only after successful convergence;
// canceled runs publish neither values nor observations.
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

// System describes one monotone regional equation system. Cells and each
// Successors result must be canonical. Transfer is the sole equation callback;
// dynamic dependencies outside the immutable WTO fail closed in engine/solve.
type System[Cell comparable, State any] struct {
	Lattice lattice.Lattice[State]
	Cells   []Cell

	Successors    func(Cell) []Cell
	Initial       func(Cell) State
	InitialSparse func(Cell) (State, bool)
	Transfer      func(Cell, func(Cell) State, func(Cell, State))

	WidenAt    func(Cell) bool
	WidenDelay func(Cell) int
	Abstract   func(Cell, State) State

	CaptureObservations bool
}

// Result is an atomically published completed region. Maps and slices are
// caller-owned. Error returns always carry a zero Result.
type Result[Cell comparable, State any] struct {
	Values       map[Cell]State
	Revisions    map[Cell]uint64
	Observations []Observation[Cell, State]
	Stats        Stats
}

// Run constructs the deterministic region WTO and delegates its execution to
// the generic solve kernel. Region wraps widening and narrowing solely for
// observation-free counters; it does not implement a second iteration loop.
func Run[Cell comparable, State any](ctx context.Context, system System[Cell, State]) (Result[Cell, State], error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := validate(system); err != nil {
		return Result[Cell, State]{}, err
	}
	cells, err := uniqueCells(system.Cells)
	if err != nil {
		return Result[Cell, State]{}, err
	}
	if err := canceled(ctx); err != nil {
		return Result[Cell, State]{}, err
	}

	stats := Stats{Cells: len(cells)}
	domain := system.Lattice
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
	solveStats := solve.Stats{}
	equations := solve.EquationSystem[Cell, State]{
		Lattice: domain, Cells: cells,
		Initial: system.Initial, InitialSparse: system.InitialSparse,
		Transfer: system.Transfer, WidenAt: system.WidenAt,
		WidenDelay: system.WidenDelay, Abstract: system.Abstract, Stats: &solveStats,
	}
	plan := solve.NewWTOPlan(cells, system.Successors)
	stats.Components = countComponents(plan.Elements())
	values, revisions, err := solve.SolveWTOContextWithVersions(ctx, equations, plan)
	if err != nil {
		return Result[Cell, State]{}, err
	}
	if err := canceled(ctx); err != nil {
		return Result[Cell, State]{}, err
	}
	stats.TransferCalls = solveStats.TransferCalls
	for _, revision := range revisions {
		if revision > stats.MaxRevision {
			stats.MaxRevision = revision
		}
	}
	var observations []Observation[Cell, State]
	if system.CaptureObservations {
		observations = make([]Observation[Cell, State], len(cells))
		for index, cell := range cells {
			observations[index] = Observation[Cell, State]{
				Cell: cell, Revision: revisions[cell], Value: values[cell],
			}
		}
		stats.ObservationCnt = len(observations)
	}
	return Result[Cell, State]{
		Values: values, Revisions: revisions, Observations: observations, Stats: stats,
	}, nil
}

func validate[Cell comparable, State any](system System[Cell, State]) error {
	if len(system.Cells) == 0 {
		return errors.New("region: Cells is empty")
	}
	if system.Lattice.Bottom == nil || system.Lattice.Equal == nil || system.Lattice.LessOrEq == nil || system.Lattice.Join == nil {
		return errors.New("region: incomplete lattice")
	}
	if system.Transfer == nil {
		return errors.New("region: Transfer is nil")
	}
	if system.Successors == nil {
		return errors.New("region: Successors is nil")
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
