// Package transfer wires CFG topology to the generic fixed-point solver.
package transfer

import (
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
	validateConfig(config)

	graph := config.Graph
	registry := config.Registry
	domain := state.Domain(registry)
	normalize := func(st state.State) state.State { return st }
	if config.StateLanes != nil {
		domain = state.DomainWithLanes(registry, config.StateLanes)
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

	sys := solve.EquationSystem[cfg.Point, state.State]{
		Lattice: domain,
		Cells:   cells,
		Initial: func(point cfg.Point) state.State {
			if config.Initial != nil {
				if initial, ok := config.Initial(point); ok {
					return normalize(initial)
				}
			}
			if point == entry {
				return normalize(config.EntryState)
			}
			return domain.Bottom()
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

			for _, succ := range graph.Successors(point) {
				cond, hasCond := graph.EdgeCond(point, succ)
				hasCond = hasCond && graph.IsBranch(point)
				edgeOut := normalize(edgeTransfer(EdgeContext{
					Graph:    graph,
					Registry: registry,
					Edge:     cfg.Edge{From: point, To: succ, Cond: cond},
					HasCond:  hasCond,
				}, out))
				emit(succ, edgeOut)
			}
		},
		WidenAt:    config.WidenAt,
		WidenDelay: config.WidenDelay,
		Stats:      solverStats(config.Stats),
	}

	return Result(solve.Solve(sys))
}

func solverStats(stats *Stats) *solve.Stats {
	if stats == nil {
		return nil
	}
	return &stats.Solver
}

func validateConfig(config Config) {
	if config.Graph == nil {
		panic("transfer: Config.Graph is nil")
	}
	if config.Registry == nil {
		panic("transfer: Config.Registry is nil")
	}
	if config.Entry != nil {
		for _, point := range config.Graph.RPO() {
			if point == *config.Entry {
				return
			}
		}
		panic("transfer: Config.Entry is not in graph.RPO()")
	}
}
