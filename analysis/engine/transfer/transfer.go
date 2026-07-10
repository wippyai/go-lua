// Package transfer wires CFG topology to the generic fixed-point solver.
package transfer

import (
	"context"
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

// StateRead is one solve-local dependency observed while evaluating a node
// transfer. Version changes whenever the solver replaces that point's state,
// including a replacement made by narrowing.
type StateRead struct {
	Point   cfg.Point
	Version uint64
}

// NodeObservation is an ephemeral transfer artifact. It is produced only for
// points selected by Config.ObserveNode and is intended to be validated and
// projected before the solved result escapes. It is not a generic point-state
// query index.
type NodeObservation struct {
	Point        cfg.Point
	InputVersion uint64
	Output       state.State
	Reads        []StateRead
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
	// Context cooperatively stops the worklist between transfer iterations. A
	// nil context preserves the legacy uncancelable Run/TryRun behavior.
	Context context.Context

	Graph    cfg.Graph
	Registry *axis.Registry

	// StateLanes selects the State product-lattice lanes used by this solve.
	// Nil uses the default lane set; a non-nil slice is the exact enabled set.
	StateLanes []state.LaneID
	// StateOptions are per-solve lattice options such as widening thresholds.
	StateOptions state.DomainOptions

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

	// ObserveNode selects points whose latest node-transfer output should be
	// captured. RecordNodeObservation is called in deterministic worklist order
	// and FinalizeNodeObservations receives the final state revisions after both
	// worklist convergence and narrowing. These hooks are solve-local; they do
	// not retain arbitrary point state in Result.
	ObserveNode              func(cfg.Point) bool
	RecordNodeObservation    func(NodeObservation)
	FinalizeNodeObservations func(finalVersion func(cfg.Point) uint64)
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
	domain, err := state.TryDomainWithOptionalLanesAndOptions(registry, config.StateLanes, config.StateOptions)
	if err != nil {
		return nil, err
	}
	normalize := func(st state.State) state.State { return st }
	if config.StateLanes != nil {
		normalize = func(st state.State) state.State {
			return state.NormalizeForDomain(domain, st)
		}
	}
	reachableEmpty := normalize(state.Reachable(state.State{}))
	tracksReachability := !domain.Equal(reachableEmpty, domain.Bottom())
	markReachable := func(st state.State) state.State {
		if !tracksReachability {
			return normalize(st)
		}
		return normalize(state.Reachable(st))
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
	observing := config.ObserveNode != nil && config.RecordNodeObservation != nil

	sys := solve.EquationSystem[cfg.Point, state.State]{
		Lattice: domain,
		Cells:   cells,
		InitialSparse: func(point cfg.Point) (state.State, bool) {
			if config.Initial != nil {
				if initial, ok := config.Initial(point); ok {
					return markReachable(initial), true
				}
			}
			if point == entry {
				return markReachable(config.EntryState), true
			}
			return state.State{}, false
		},
		Transfer: func(point cfg.Point, read func(cfg.Point) state.State, emit func(cfg.Point, state.State)) {
			in := normalize(read(point))
			if tracksReachability && domain.Equal(in, domain.Bottom()) {
				return
			}
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
		TransferVersioned: func(point cfg.Point, read func(cfg.Point) (state.State, uint64), emit func(cfg.Point, state.State)) {
			in, inputVersion := read(point)
			in = normalize(in)
			if tracksReachability && domain.Equal(in, domain.Bottom()) {
				return
			}
			if !observing || !config.ObserveNode(point) {
				// Preserve the ordinary transfer spelling for unplanned points so
				// observation has no retention or per-read bookkeeping there.
				out := normalize(nodeTransfer(NodeContext{
					Graph:    graph,
					Registry: registry,
					Point:    point,
					Node:     graph.Node(point),
					Read: func(other cfg.Point) state.State {
						value, _ := read(other)
						return value
					},
				}, in))
				for _, succ := range cfg.SuccessorsReadOnly(graph, point) {
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
				return
			}

			reads := make([]StateRead, 0, 2)
			readForNode := func(other cfg.Point) state.State {
				value, version := read(other)
				reads = append(reads, StateRead{Point: other, Version: version})
				return value
			}
			out := normalize(nodeTransfer(NodeContext{
				Graph:    graph,
				Registry: registry,
				Point:    point,
				Node:     graph.Node(point),
				Read:     readForNode,
			}, in))
			config.RecordNodeObservation(NodeObservation{
				Point:        point,
				InputVersion: inputVersion,
				Output:       out,
				Reads:        reads,
			})
			for _, succ := range cfg.SuccessorsReadOnly(graph, point) {
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
		WidenAt:    widenAt,
		WidenDelay: config.WidenDelay,
		Stats:      solverStats(config.Stats),
	}
	if !observing {
		sys.TransferVersioned = nil
	}

	if config.Context == nil {
		if !observing {
			return Result(solve.Solve(sys)), nil
		}
		result, versions := solve.SolveWithVersions(sys)
		if config.FinalizeNodeObservations != nil {
			config.FinalizeNodeObservations(func(point cfg.Point) uint64 { return versions[point] })
		}
		return Result(result), nil
	}
	if !observing {
		result, err := solve.SolveContext(config.Context, sys)
		if err != nil {
			return nil, err
		}
		return Result(result), nil
	}
	result, versions, err := solve.SolveContextWithVersions(config.Context, sys)
	if err != nil {
		return nil, err
	}
	if config.FinalizeNodeObservations != nil {
		config.FinalizeNodeObservations(func(point cfg.Point) uint64 { return versions[point] })
	}
	return Result(result), nil
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
