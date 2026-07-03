// Package transfer wires CFG topology to the generic fixed-point solver.
package transfer

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/solve"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// InitialState supplies an explicit starting state for a point.
//
// Points for which InitialState returns false start at bottom, except for the
// configured entry point, which starts at Config.EntryState.
type InitialState func(point cfg.Point) (state.State, bool)

// NodeTransfer maps a point's input state to the state sent to outgoing edges.
type NodeTransfer func(ctx NodeContext, in state.State) state.State

// EdgeTransfer maps node output across a single CFG edge.
type EdgeTransfer func(ctx EdgeContext, out state.State) state.State

// Stats holds caller-owned observational counters for transfer runs.
type Stats struct {
	Solver solve.Stats
}

// NodeContext is the generic context passed to node transfer hooks.
type NodeContext struct {
	Graph    cfg.Graph
	Registry *axis.Registry
	Point    cfg.Point
	Node     *cfg.Node
	Read     func(cfg.Point) state.State
}

// EdgeContext is the generic context passed to edge transfer hooks.
type EdgeContext struct {
	Graph    cfg.Graph
	Registry *axis.Registry
	Edge     cfg.Edge
	HasCond  bool
	Read     func(cfg.Point) state.State
}

// Config describes one forward dataflow run.
type Config struct {
	Graph    cfg.Graph
	Registry *axis.Registry

	// StateLanes selects the State product-lattice lanes used by this solve.
	// Nil uses the default lane set; a non-nil slice is the exact enabled set.
	StateLanes []state.LaneID

	// Entry is the point seeded with EntryState. Nil uses Graph.Entry().
	Entry      *cfg.Point
	EntryState state.State

	// Initial supplies explicit starting states for any point. When it returns
	// true for the entry point, it takes precedence over EntryState.
	Initial InitialState

	// NodeTransfer and EdgeTransfer default to identity.
	NodeTransfer NodeTransfer
	EdgeTransfer EdgeTransfer

	// WidenAt and WidenDelay are forwarded directly to the solver.
	WidenAt    func(cfg.Point) bool
	WidenDelay func(cfg.Point) int

	// Stats, when non-nil, receives observational counters for this run.
	Stats *Stats
}

// Result maps each reachable CFG point in Graph.RPO() to its input state.
type Result map[cfg.Point]state.State

// Run executes a one-off forward transfer run.
func Run(config Config) Result {
	result, err := TryRun(config)
	if err != nil {
		panic(err.Error())
	}
	return result
}

// TryRun executes a one-off forward transfer run, returning configuration
// errors instead of panicking.
func TryRun(config Config) (Result, error) {
	if err := validateConfig(config); err != nil {
		return nil, err
	}
	graph := config.Graph
	registry := config.Registry
	domain, err := state.TryDomainWithOptionalLanes(registry, config.StateLanes)
	if err != nil {
		return nil, err
	}
	normalize := func(st state.State) state.State { return st }
	if config.StateLanes != nil {
		normalize = func(st state.State) state.State {
			return state.NormalizeForDomain(domain, st)
		}
	}
	cells := graph.RPO()
	if len(cells) != 0 {
		cells = append([]cfg.Point(nil), cells...)
	}

	entry := graph.Entry()
	if config.Entry != nil {
		entry = *config.Entry
	}

	nodeTransfer := config.NodeTransfer
	if nodeTransfer == nil {
		nodeTransfer = func(_ NodeContext, in state.State) state.State {
			return in
		}
	}
	edgeTransfer := config.EdgeTransfer
	if edgeTransfer == nil {
		edgeTransfer = func(_ EdgeContext, out state.State) state.State {
			return out
		}
	}
	widenAt := config.WidenAt
	if widenAt == nil {
		widenAt = defaultWidenAtForRPO(graph, cells)
	}

	sys := solve.EquationSystem[cfg.Point, state.State]{
		Lattice: domain,
		Cells:   cells,
		InitialSparse: func(point cfg.Point) (state.State, bool) {
			if config.Initial != nil {
				if initial, ok := config.Initial(point); ok {
					return normalize(initial), true
				}
			}
			if point == entry {
				return normalize(config.EntryState), true
			}
			return state.State{}, false
		},
		Transfer: func(point cfg.Point, read func(cfg.Point) state.State, emit func(cfg.Point, state.State)) {
			in := normalize(read(point))
			out := normalize(nodeTransfer(NodeContext{
				Graph:    graph,
				Registry: registry,
				Point:    point,
				Node:     graph.Node(point),
				Read:     read,
			}, in))

			for _, succ := range cfg.SuccessorsReadOnly(graph, point) {
				cond, hasCond := graph.EdgeCond(point, succ)
				hasCond = hasCond && graph.IsBranch(point)
				edgeOut := normalize(edgeTransfer(EdgeContext{
					Graph:    graph,
					Registry: registry,
					Edge:     cfg.Edge{From: point, To: succ, Cond: cond},
					HasCond:  hasCond,
					Read:     read,
				}, out))
				emit(succ, edgeOut)
			}
		},
		WidenAt:    widenAt,
		WidenDelay: config.WidenDelay,
		Stats:      solverStats(config.Stats),
	}

	return Result(solve.Solve(sys)), nil
}

func solverStats(stats *Stats) *solve.Stats {
	if stats == nil {
		return nil
	}
	return &stats.Solver
}

func validateConfig(config Config) error {
	if config.Graph == nil {
		return errors.New("transfer: Config.Graph is nil")
	}
	if config.Registry == nil {
		return errors.New("transfer: Config.Registry is nil")
	}
	if config.Entry != nil {
		for _, point := range config.Graph.RPO() {
			if point == *config.Entry {
				return nil
			}
		}
		return errors.New("transfer: Config.Entry is not in graph.RPO()")
	}
	return nil
}
