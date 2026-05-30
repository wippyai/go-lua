package equation

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/canonical/state"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/flow/propagate"
	"github.com/wippyai/go-lua/types/lattice/solver"
)

// Builder assembles the combined equation graph (CFG point cells plus
// per-parameter contract cells) for one function and solves it over the single
// generic worklist, producing the converged FunctionState.
//
// It is the structural core of the canonical flow: it owns the topology, the
// predecessor join, the forward emission to successors, the backward routing of
// demand into contract cells, and the entry point's reading of those contracts.
// The local node semantics are supplied as an injected NodeTransfer.
type Builder struct {
	graph     *cfg.Graph
	transfer  NodeTransfer
	narrower  EdgeNarrower
	numParams int

	// pointSet is the set of forward-reachable point cells, used to skip dead
	// predecessor edges (a CFG may list a predecessor that is not reachable from
	// Entry; reading its unseeded cell would be meaningless).
	pointSet map[cfg.Point]bool
}

// NewBuilder constructs a Builder for graph g with numParams parameter slots and
// the injected per-node transfer. When transfer also implements EdgeNarrower, the
// builder applies its per-successor narrowing to each branch edge before the
// predecessor join (path-sensitive narrowing); otherwise it joins unrefined.
func NewBuilder(g *cfg.Graph, numParams int, transfer NodeTransfer) *Builder {
	b := &Builder{graph: g, transfer: transfer, numParams: numParams}
	if n, ok := transfer.(EdgeNarrower); ok {
		b.narrower = n
	}
	return b
}

// Solve computes the single canonical intraprocedural fixed point and assembles
// it into a state.FunctionState.
//
// One worklist, one convergence test, ranges over point cells and contract cells
// together. Forward value flow (predecessor join -> transfer -> successors) and
// backward demand flow (body use -> contract cell -> entry) converge jointly.
// Widening fires at the combined feedback-vertex set: CFG loop headers for point
// cells, plus contract cells that lie on an entry<->body cycle.
func (b *Builder) Solve() state.FunctionState {
	points := b.pointCells()
	b.pointSet = make(map[cfg.Point]bool, len(points))
	for _, p := range points {
		b.pointSet[p] = true
	}
	demanded := b.discoverDemandedParams(points)
	cells := b.allCells(points)
	widenAt := b.wideningSites(demanded)

	sys := solver.EquationSystem[Cell, CellState]{
		Lattice:  CellStateDomain,
		Cells:    cells,
		Initial:  initialFor,
		Transfer: b.makeTransfer(),
		WidenAt:  func(c Cell) bool { return widenAt[c] },
	}

	result := solver.Solve(sys)
	return b.assemble(points, result)
}

// pointCells enumerates every reachable CFG point by BFS from Entry() via
// Successors(), in discovery order (deterministic given the CFG's Successors
// order). This fixes the solver's Cells order and thus the result.
func (b *Builder) pointCells() []cfg.Point {
	entry := b.graph.Entry()
	order := make([]cfg.Point, 0)
	seen := make(map[cfg.Point]bool)
	queue := []cfg.Point{entry}
	seen[entry] = true
	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]
		order = append(order, p)
		for _, succ := range b.graph.Successors(p) {
			if !seen[succ] {
				seen[succ] = true
				queue = append(queue, succ)
			}
		}
	}
	return order
}

// allCells lists every solver cell in deterministic order: all point cells
// (BFS order) followed by one contract cell per parameter index.
func (b *Builder) allCells(points []cfg.Point) []Cell {
	cells := make([]Cell, 0, len(points)+b.numParams)
	for _, p := range points {
		cells = append(cells, pointCellAt(p))
	}
	for i := 0; i < b.numParams; i++ {
		cells = append(cells, contractCellAt(i))
	}
	return cells
}

// makeTransfer builds the solver Transfer closure for the combined graph.
//
// For a POINT cell p: read every reachable predecessor point cell and Join their
// post-transfer states into incoming; read every contract cell into
// entryContracts; run the injected NodeTransfer (which may emit demand into
// contract cells); emit the resulting state into p's OWN cell. The cell of a
// point therefore holds out[p] — the state LEAVING p — so a successor reading p
// as its predecessor sees out[p] as its incoming, the standard forward dataflow
// equation in[s] = join over preds out[pred]. The solver re-queues p whenever a
// predecessor or a contract it read changes (read records the dependency), and
// re-queues p's successors whenever p's own cell changes (emit-side requeue).
//
// Dead predecessor edges — points the CFG lists as predecessors but that are not
// forward-reachable from Entry, e.g. an orphaned loop-exit fragment — are
// skipped: their cells are unseeded and contribute Bottom, so ignoring them is
// the same value with no spurious dependency on an absent cell.
//
// For a CONTRACT cell: it is a pure accumulator. It has no forward equation
// beyond holding the joined demand emitted by point transfers; its Transfer is a
// no-op (the solver still propagates its value to readers via emit-side
// re-queueing).
func (b *Builder) makeTransfer() func(Cell, func(Cell) CellState, func(Cell, CellState)) {
	return func(cell Cell, read func(Cell) CellState, emit func(Cell, CellState)) {
		if cell.Kind == ContractCell {
			return
		}

		p := cell.Point

		// Forward: join reachable predecessor point states. The entry point has
		// no CFG predecessors, so incoming starts at Bottom and the transfer
		// folds in the assumed contracts.
		incoming := flow.PointStateDomain.Bottom()
		for _, pred := range b.graph.Predecessors(p) {
			if !b.pointSet[pred] {
				continue
			}
			ps := read(pointCellAt(pred)).Point
			// Path-sensitive narrowing: when pred is a branch, refine its out-state by
			// the guard the edge pred -> p carries (the guard on the true edge, its
			// negation on the false edge) before joining. The merge-point join recovers
			// the unnarrowed value, so the narrowing is local to the guarded edge.
			if b.narrower != nil {
				ps = b.narrower.NarrowEdge(b.graph, pred, p, ps)
			}
			incoming = flow.PointStateDomain.Join(incoming, ps)
		}

		// Backward demand context: read every contract cell so this point
		// depends on them. The entry point USES entryContracts as the assumed
		// entry value; reading the contracts here is what makes a grown contract
		// re-trigger entry, closing the entry<->body cycle.
		entryContracts := b.readContracts(read)

		// demand sink: a body use Joins an obligation into the parameter's
		// contract cell.
		demand := func(param int, c paramevidence.ParamContract) {
			if param < 0 || param >= b.numParams {
				return
			}
			emit(contractCellAt(param), contractState(c))
		}

		next := b.transfer.Transfer(b.graph, p, incoming, entryContracts, demand)

		// Emit the post-transfer state into p's own cell; successors read it.
		emit(pointCellAt(p), pointState(next))
	}
}

// readContracts reads every contract cell into a paramevidence.Contracts map,
// recording the dependency so a contract change re-queues the reader. Bottom
// (no obligation) cells are omitted, matching the MapLattice canonicalization.
func (b *Builder) readContracts(read func(Cell) CellState) paramevidence.Contracts {
	contracts := make(paramevidence.Contracts)
	for i := 0; i < b.numParams; i++ {
		c := read(contractCellAt(i)).Contract
		if paramevidence.ParamContractDomain.Equal(c, paramevidence.ParamContractDomain.Bottom()) {
			continue
		}
		contracts[i] = c
	}
	return contracts
}

// discoverDemandedParams runs the injected transfer once per point against
// Bottom inputs to observe which parameters any body use constrains, with no
// effect on the solver state. A demanded contract cell lies on the
// entry<->body<->contract cycle (the entry reads every contract and the contract
// feeds entry forward), so it must be a widening site; an undemanded contract
// cell never moves above Bottom and stays exact.
func (b *Builder) discoverDemandedParams(points []cfg.Point) map[int]bool {
	demanded := make(map[int]bool)
	bottom := flow.PointStateDomain.Bottom()
	emptyContracts := paramevidence.Contracts(nil)
	sink := func(param int, _ paramevidence.ParamContract) {
		if param >= 0 && param < b.numParams {
			demanded[param] = true
		}
	}
	for _, p := range points {
		b.transfer.Transfer(b.graph, p, bottom, emptyContracts, sink)
	}
	return demanded
}

// wideningSites is the combined feedback-vertex set: CFG loop-header / non-loop
// SCC-header point cells (from propagate.FeedbackVertexSet) plus every demanded
// contract cell. Acyclic contract cells are absent, so they use exact Join.
func (b *Builder) wideningSites(demanded map[int]bool) map[Cell]bool {
	sites := make(map[Cell]bool)
	for p, isFVS := range propagate.FeedbackVertexSet(b.graph) {
		if isFVS {
			sites[pointCellAt(p)] = true
		}
	}
	for param := range demanded {
		sites[contractCellAt(param)] = true
	}
	return sites
}

// assemble projects the solved cell map into a state.FunctionState. Point cells
// become Points (Bottom states dropped to match MapLattice absence); contract
// cells become Contracts (Bottom obligations dropped likewise). The per-point
// IN-states are derived from the same solved cell map (assembleInStates), so the
// diagnostic surface reads the solver's exact edge-narrowed merge rather than
// re-deriving it.
func (b *Builder) assemble(points []cfg.Point, result map[Cell]CellState) state.FunctionState {
	fs := state.FunctionState{
		Points:    make(map[cfg.Point]flow.PointState),
		Contracts: make(paramevidence.Contracts),
	}
	for _, p := range points {
		ps := result[pointCellAt(p)].Point
		if flow.PointStateDomain.Equal(ps, flow.PointStateDomain.Bottom()) {
			continue
		}
		fs.Points[p] = ps
	}
	for i := 0; i < b.numParams; i++ {
		c := result[contractCellAt(i)].Contract
		if paramevidence.ParamContractDomain.Equal(c, paramevidence.ParamContractDomain.Bottom()) {
			continue
		}
		fs.Contracts[i] = c
	}
	fs.InPoints = b.assembleInStates(points, result)
	return fs
}

// assembleInStates derives each reachable point's IN-state from the solved cell
// map, using the SAME reachable point set and per-edge narrower the solver's
// makeTransfer used (graph.go:142-156). It is the single source of truth for a
// point's entering state: the entry point's in-state is its own solved out-state
// (which already folds the assumed contracts), and every other point's in-state
// is the join over its reachable predecessors of NarrowEdge(pred -> p, out[pred]).
//
// The merge contract holds by construction: a join over EVERY reachable
// predecessor's edge-narrowed out-state, so a guarded branch's narrowing meets
// the opposite branch's unnarrowed (or oppositely-narrowed) state and the LUB
// recovers the union; no single narrowed predecessor is ever selected alone. A
// point with no reachable predecessor (the entry, or an isolated point) takes its
// own out-state, the value established AT the point. A Bottom in-state is dropped
// to match the MapLattice absence convention of Points.
func (b *Builder) assembleInStates(points []cfg.Point, result map[Cell]CellState) map[cfg.Point]flow.PointState {
	entry := b.graph.Entry()
	in := make(map[cfg.Point]flow.PointState, len(points))
	for _, p := range points {
		preds := b.graph.Predecessors(p)
		if len(preds) == 0 || p == entry {
			// The entry (and any source point) enters with its own solved out-state,
			// which carries the seeded parameter values folded from the contracts.
			ps := result[pointCellAt(p)].Point
			if !flow.PointStateDomain.Equal(ps, flow.PointStateDomain.Bottom()) {
				in[p] = ps
			}
			continue
		}
		joined := flow.PointStateDomain.Bottom()
		any := false
		for _, pred := range preds {
			if !b.pointSet[pred] {
				continue
			}
			ps := result[pointCellAt(pred)].Point
			if b.narrower != nil {
				ps = b.narrower.NarrowEdge(b.graph, pred, p, ps)
			}
			joined = flow.PointStateDomain.Join(joined, ps)
			any = true
		}
		if !any {
			continue
		}
		if flow.PointStateDomain.Equal(joined, flow.PointStateDomain.Bottom()) {
			continue
		}
		in[p] = joined
	}
	return in
}
