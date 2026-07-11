package transfer

import (
	"sync/atomic"

	"github.com/wippyai/go-lua/analysis/domain/lattice"
	"github.com/wippyai/go-lua/analysis/engine/cancellation"
	"github.com/wippyai/go-lua/analysis/engine/solve"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// equationPlan is the immutable equation layer shared by every clean schedule
// for one TryRun. Keeping construction in one place prevents FIFO, WTO, and a
// future transactional executor from silently solving different equations.
// It is deliberately package-private: body continues to publish only Results.
type equationPlan struct {
	system       solve.EquationSystem[cfg.Point, state.State]
	cells        []cfg.Point
	wto          *solve.WTOPlan[cfg.Point]
	observing    bool
	identity     equationPlanIdentity
	solverPolicy equationSolverPolicy
}

type equationPlanIdentity struct{ generation uint64 }

// The generation identifies this exact plan instance only. TryRun intentionally
// builds a fresh plan today. Cross-phase retention must retain this opaque plan
// and supply a separate replaceable execution layer (context, summary provider,
// stats and observation hooks); reconstructing a plan and comparing function
// pointers is not a valid identity check.

// equationSolverPolicy records equation semantics that are currently implicit
// in the generic solver. The version must change if narrowing or contribution
// publication rules change before plans can be retained across phases.
type equationSolverPolicy struct {
	version uint32
}

const currentEquationSolverPolicyVersion uint32 = 1

var nextEquationPlanGeneration atomic.Uint64

// equationPlanHooks are owner-scoped observations of exact solver reads and
// emissions. Clean execution supplies nil hooks, so this refactor has no hot
// path or semantic effect. A future provenance owner can attach hooks to this
// same canonical plan rather than rebuilding transfer equations elsewhere.
type equationPlanHooks struct {
	read func(owner, dependency cfg.Point, value state.State, revision uint64, versioned bool)
	emit func(owner, destination cfg.Point, contribution state.State)
}

func newEquationPlan(config Config, domain lattice.Lattice[state.State], hooks equationPlanHooks) equationPlan {
	graph := config.Graph
	registry := config.Registry
	normalize := func(st state.State) state.State { return st }
	if config.StateLanes != nil {
		normalize = func(st state.State) state.State { return state.NormalizeForDomain(domain, st) }
	}
	reachableEmpty := normalize(state.Reachable(state.State{}))
	tracksReachability := !domain.Equal(reachableEmpty, domain.Bottom())
	markReachable := func(st state.State) state.State {
		if !tracksReachability {
			return normalize(st)
		}
		return normalize(state.Reachable(st))
	}
	cells := append([]cfg.Point(nil), graph.RPO()...)
	entry := graph.Entry()
	if config.Entry != nil {
		entry = *config.Entry
	}
	nodeTransfer := config.NodeTransfer
	if nodeTransfer == nil {
		nodeTransfer = func(_ NodeContext, in state.State) state.State { return in }
	}
	edgeTransfer := config.EdgeTransfer
	if edgeTransfer == nil {
		edgeTransfer = func(_ EdgeContext, out state.State) state.State { return out }
	}
	widenAt := config.WidenAt
	if widenAt == nil {
		widenAt = defaultWidenAtForRPO(graph, cells)
	}
	observing := config.ObserveNode != nil && config.RecordNodeObservation != nil

	system := solve.EquationSystem[cfg.Point, state.State]{
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
		WidenAt: widenAt, WidenDelay: config.WidenDelay, Stats: solverStats(config.Stats),
	}
	system.Transfer = func(point cfg.Point, read func(cfg.Point) state.State, emit func(cfg.Point, state.State)) {
		if config.Session.Token().Canceled() {
			return
		}
		if config.BeforePoint != nil {
			config.BeforePoint(point)
		}
		defer func() {
			if config.AfterPoint != nil {
				config.AfterPoint(point)
			}
		}()
		in := normalize(read(point))
		if tracksReachability && domain.Equal(in, domain.Bottom()) {
			return
		}
		out := normalize(nodeTransfer(NodeContext{Context: config.Context, Session: config.Session, Graph: graph, Registry: registry, Point: point, Node: graph.Node(point), Read: read}, in))
		poll := cancellation.NewPoller(config.Session.Token(), cancellation.EveryCheap)
		for _, succ := range cfg.SuccessorsReadOnly(graph, point) {
			if poll.Poll() {
				return
			}
			cond, hasCond := graph.EdgeCond(point, succ)
			hasCond = hasCond && graph.IsBranch(point)
			edgeOut := normalize(edgeTransfer(EdgeContext{Context: config.Context, Session: config.Session, Graph: graph, Registry: registry, Edge: cfg.Edge{From: point, To: succ, Cond: cond}, HasCond: hasCond, Read: read}, out))
			emit(succ, edgeOut)
		}
	}
	system.TransferVersioned = func(point cfg.Point, read func(cfg.Point) (state.State, uint64), emit func(cfg.Point, state.State)) {
		if config.Session.Token().Canceled() {
			return
		}
		if config.BeforePoint != nil {
			config.BeforePoint(point)
		}
		defer func() {
			if config.AfterPoint != nil {
				config.AfterPoint(point)
			}
		}()
		readState := func(other cfg.Point) state.State {
			value, _ := read(other)
			return value
		}
		in, inputVersion := read(point)
		in = normalize(in)
		if tracksReachability && domain.Equal(in, domain.Bottom()) {
			return
		}
		if !observing || !config.ObserveNode(point) {
			out := normalize(nodeTransfer(NodeContext{Context: config.Context, Session: config.Session, Graph: graph, Registry: registry, Point: point, Node: graph.Node(point), Read: readState}, in))
			poll := cancellation.NewPoller(config.Session.Token(), cancellation.EveryCheap)
			for _, succ := range cfg.SuccessorsReadOnly(graph, point) {
				if poll.Poll() {
					return
				}
				cond, hasCond := graph.EdgeCond(point, succ)
				hasCond = hasCond && graph.IsBranch(point)
				edgeOut := normalize(edgeTransfer(EdgeContext{Context: config.Context, Session: config.Session, Graph: graph, Registry: registry, Edge: cfg.Edge{From: point, To: succ, Cond: cond}, HasCond: hasCond}, out))
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
		out := normalize(nodeTransfer(NodeContext{Context: config.Context, Session: config.Session, Graph: graph, Registry: registry, Point: point, Node: graph.Node(point), Read: readForNode}, in))
		config.RecordNodeObservation(NodeObservation{Point: point, InputVersion: inputVersion, Output: out, Reads: reads})
		poll := cancellation.NewPoller(config.Session.Token(), cancellation.EveryCheap)
		for _, succ := range cfg.SuccessorsReadOnly(graph, point) {
			if poll.Poll() {
				return
			}
			cond, hasCond := graph.EdgeCond(point, succ)
			hasCond = hasCond && graph.IsBranch(point)
			edgeOut := normalize(edgeTransfer(EdgeContext{Context: config.Context, Session: config.Session, Graph: graph, Registry: registry, Edge: cfg.Edge{From: point, To: succ, Cond: cond}, HasCond: hasCond}, out))
			emit(succ, edgeOut)
		}
	}
	// Instrumented plans wrap the already-canonical equation. The production
	// clean plan takes the nil-hook branch once here and retains the exact base
	// closures: no per-transfer hook checks, wrappers, or allocations.
	if hooks.read != nil || hooks.emit != nil {
		baseTransfer := system.Transfer
		system.Transfer = func(point cfg.Point, read func(cfg.Point) state.State, emit func(cfg.Point, state.State)) {
			if hooks.read != nil {
				baseRead := read
				read = func(dependency cfg.Point) state.State {
					value := baseRead(dependency)
					hooks.read(point, dependency, value, 0, false)
					return value
				}
			}
			if hooks.emit != nil {
				baseEmit := emit
				emit = func(destination cfg.Point, contribution state.State) {
					hooks.emit(point, destination, contribution)
					baseEmit(destination, contribution)
				}
			}
			baseTransfer(point, read, emit)
		}
		baseVersioned := system.TransferVersioned
		system.TransferVersioned = func(point cfg.Point, read func(cfg.Point) (state.State, uint64), emit func(cfg.Point, state.State)) {
			if hooks.read != nil {
				baseRead := read
				read = func(dependency cfg.Point) (state.State, uint64) {
					value, revision := baseRead(dependency)
					hooks.read(point, dependency, value, revision, true)
					return value, revision
				}
			}
			if hooks.emit != nil {
				baseEmit := emit
				emit = func(destination cfg.Point, contribution state.State) {
					hooks.emit(point, destination, contribution)
					baseEmit(destination, contribution)
				}
			}
			baseVersioned(point, read, emit)
		}
	}
	if !observing {
		system.TransferVersioned = nil
	}
	return equationPlan{
		system: system, cells: cells, wto: config.WTOPlan, observing: observing,
		identity:     equationPlanIdentity{generation: nextEquationPlanGeneration.Add(1)},
		solverPolicy: equationSolverPolicy{version: currentEquationSolverPolicyVersion},
	}
}

func (p equationPlan) sameIdentity(other equationPlan) bool {
	return p.identity.generation != 0 && p.identity == other.identity && p.solverPolicy == other.solverPolicy
}

func (p equationPlan) withStats(stats *solve.Stats) solve.EquationSystem[cfg.Point, state.State] {
	system := p.system
	system.Stats = stats
	return system
}
