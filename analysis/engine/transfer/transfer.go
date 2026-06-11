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

// NodeContext is the generic context passed to node transfer hooks.
type NodeContext struct {
	Graph    cfg.Graph
	Registry *axis.Registry
	Point    cfg.Point
	Node     *cfg.Node
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
}

// Result maps each reachable CFG point in Graph.RPO() to its input state.
type Result map[cfg.Point]state.State

// Runner is a reusable wrapper around Config.
type Runner struct {
	config Config
}

// New creates a runner for config.
func New(config Config) Runner {
	return Runner{config: config}
}

// Run executes a one-off forward transfer run.
func Run(config Config) Result {
	return New(config).Run()
}

// Run solves the configured forward dataflow system.
func (r Runner) Run() Result {
	config := r.config
	validateConfig(config)

	graph := config.Graph
	registry := config.Registry
	domain := state.Domain(registry)
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
					return initial
				}
			}
			if point == entry {
				return config.EntryState
			}
			return domain.Bottom()
		},
		Transfer: func(point cfg.Point, read func(cfg.Point) state.State, emit func(cfg.Point, state.State)) {
			in := read(point)
			out := nodeTransfer(NodeContext{
				Graph:    graph,
				Registry: registry,
				Point:    point,
				Node:     graph.Node(point),
			}, in)

			for _, succ := range graph.Successors(point) {
				cond, hasCond := graph.EdgeCond(point, succ)
				hasCond = hasCond && graph.IsBranch(point)
				edgeOut := edgeTransfer(EdgeContext{
					Graph:    graph,
					Registry: registry,
					Edge:     cfg.Edge{From: point, To: succ, Cond: cond},
					HasCond:  hasCond,
				}, out)
				emit(succ, edgeOut)
			}
		},
		WidenAt:    config.WidenAt,
		WidenDelay: config.WidenDelay,
	}

	return Result(solve.Solve(sys))
}

func validateConfig(config Config) {
	if config.Graph == nil {
		panic("transfer: Config.Graph is nil")
	}
	if config.Registry == nil {
		panic("transfer: Config.Registry is nil")
	}
}
