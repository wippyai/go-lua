package equation

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/canonical/state"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/flow/propagate"
	"github.com/wippyai/go-lua/types/lattice/solver"
)

const wideningDelayChanges = 3

// Builder assembles the combined equation graph (CFG point cells plus
// per-parameter contract cells) for one function and solves it over the single
// generic worklist, producing the converged FunctionState.
//
// It is the structural core of the flow: it owns the topology, the
// predecessor join, the forward emission to successors, the backward routing of
// demand into contract cells, and the entry point's reading of those contracts.
// The local node semantics are supplied as an injected NodeTransfer.
type Builder struct {
	graph         *cfg.Graph
	transfer      NodeTransfer
	narrower      EdgeNarrower
	numParams     int
	entry         flow.CaptureCells
	entryRefs     flow.FunctionRefs
	entryClosures flow.ClosureRefs
	entryValues   map[int]product.AbstractValue
	entrySymbols  map[cfg.SymbolID]product.AbstractValue
	projector     *propagate.ConditionProjector

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
	if p, ok := transfer.(ConditionProjectorProvider); ok {
		b.projector = p.ConditionProjector()
	}
	return b
}

// WithEntryCells returns b after installing the immutable captured-cell store
// visible at the function entry. SummaryQ computes it from the query key and
// lexical parent exports; the per-node transfer receives it as ordinary incoming
// point state, not as mutable transfer configuration.
func (b *Builder) WithEntryCells(entry flow.CaptureCells) *Builder {
	b.entry = entry
	return b
}

// WithEntryFunctionRefs returns b after installing function-identity facts
// visible at function entry. These facts are ordinary point-state product data:
// the entry point receives them, transfer consumes them, and summary projection
// observes only the solved result.
func (b *Builder) WithEntryFunctionRefs(refs flow.FunctionRefs) *Builder {
	b.entryRefs = flow.FunctionRefsDomain.Join(refs, nil)
	return b
}

// WithEntryClosureRefs returns b after installing closure-environment facts
// visible at function entry.
func (b *Builder) WithEntryClosureRefs(refs flow.ClosureRefs) *Builder {
	b.entryClosures = flow.ClosureRefsDomain.Join(refs, nil)
	return b
}

// WithEntryValues returns b after installing product values visible at function
// entry. SummaryQ derives these from interprocedural context such as prototype
// receiver self and caller argument evidence. They enter the point-state product
// before the local transfer seeds declared annotations and body contracts, so the
// ordinary entry transfer can compose the sources in one Env/Cells location.
func (b *Builder) WithEntryValues(values map[int]product.AbstractValue) *Builder {
	b.entryValues = cloneEntryValues(values)
	return b
}

// WithEntrySymbolValues returns b after installing immutable product values keyed
// directly by graph symbols. These are scope/fact-derived entry bindings (for
// example callback-scoped globals) and enter the product state before local
// transfer seeding, so the single fixpoint owns their precision.
func (b *Builder) WithEntrySymbolValues(values map[cfg.SymbolID]product.AbstractValue) *Builder {
	b.entrySymbols = cloneEntrySymbolValues(values)
	return b
}

// Solve computes the single canonical intraprocedural fixed point and assembles
// it into a state.FunctionState.
//
// One worklist, one convergence test, ranges over point cells and contract cells
// together. Forward value flow (predecessor join -> transfer -> successors) and
// backward demand flow (body use -> contract cell -> entry) converge jointly.
// Widening is available at the combined feedback-vertex set: CFG loop headers
// for point cells, plus parameter contract cells, which close the entry->body
// demand cycle when any body use emits a demand. The generic solver exact-joins
// initial fan-in and delays widening for the first few post-visit updates, so
// one-shot facts stay exact while genuine ascending cycles still terminate by
// the domain Widen.
func (b *Builder) Solve() state.FunctionState {
	points := b.pointCells()
	b.pointSet = make(map[cfg.Point]bool, len(points))
	for _, p := range points {
		b.pointSet[p] = true
	}
	cells := b.allCells(points)
	widenAt := b.wideningSites()

	sys := solver.EquationSystem[Cell, CellState]{
		Lattice:    CellStateDomain,
		Cells:      cells,
		Initial:    initialFor,
		Transfer:   b.makeTransfer(),
		WidenAt:    func(c Cell) bool { return widenAt[c] },
		WidenDelay: func(Cell) int { return wideningDelayChanges },
		Abstract:   b.abstractCellState,
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
// post-transfer states into incoming; at the entry point only, read every
// contract cell into entryContracts; run the injected NodeTransfer (which may
// emit demand into contract cells); emit the resulting state into p's OWN cell.
// The cell of a point therefore holds out[p] — the state LEAVING p — so a
// successor reading p as its predecessor sees out[p] as its incoming, the
// standard forward dataflow equation in[s] = join over preds out[pred]. The
// solver re-queues p whenever a predecessor it read changes; contract changes
// re-queue entry, and entry's changed out-state then propagates through the CFG.
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
	entry := b.graph.Entry()
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
		if p == entry && !flow.CaptureCellsDomain.Equal(b.entry, flow.CaptureCellsDomain.Bottom()) {
			incoming.Cells = flow.CaptureCellsDomain.Join(incoming.Cells, b.entry)
		}
		if p == entry && !flow.FunctionRefsDomain.Equal(b.entryRefs, flow.FunctionRefsDomain.Bottom()) {
			incoming.FunctionRefs = flow.FunctionRefsDomain.Join(incoming.FunctionRefs, b.entryRefs)
		}
		if p == entry && !flow.ClosureRefsDomain.Equal(b.entryClosures, flow.ClosureRefsDomain.Bottom()) {
			incoming.ClosureRefs = flow.ClosureRefsDomain.Join(incoming.ClosureRefs, b.entryClosures)
		}
		if p == entry {
			b.seedEntrySymbolValues(&incoming)
			b.seedEntryValues(&incoming)
		}
		incoming = b.projectPointState(p, incoming)

		// Backward demand context: only entry reads contract cells. A grown
		// contract therefore re-triggers entry, and any changed entry out-state
		// flows forward through ordinary predecessor dependencies. Non-entry
		// points must not receive contracts as a side channel.
		var entryContracts paramevidence.Contracts
		if p == entry {
			entryContracts = b.readContracts(read)
		}

		// demand sink: a body use Joins an obligation into the parameter's
		// contract cell.
		demand := func(param int, c paramevidence.ParamContract) {
			if param < 0 || param >= b.numParams {
				return
			}
			emit(contractCellAt(param), contractState(c))
		}

		next := b.transfer.Transfer(b.graph, p, incoming, entryContracts, demand)
		next = b.projectPointState(p, next)

		// Emit the post-transfer state into p's own cell; successors read it.
		emit(pointCellAt(p), pointState(next))
	}
}

func (b *Builder) abstractCellState(cell Cell, st CellState) CellState {
	if cell.Kind != PointCell || st.Kind != PointCell {
		return st
	}
	return pointState(b.projectPointState(cell.Point, st.Point))
}

func (b *Builder) projectPointState(p cfg.Point, ps flow.PointState) flow.PointState {
	if b == nil || b.projector == nil || !b.projector.Enabled() {
		return ps
	}
	ps.Cond = b.projector.Project(p, ps.Cond)
	return ps
}

func cloneEntryValues(in map[int]product.AbstractValue) map[int]product.AbstractValue {
	if len(in) == 0 {
		return nil
	}
	out := make(map[int]product.AbstractValue, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneEntrySymbolValues(in map[cfg.SymbolID]product.AbstractValue) map[cfg.SymbolID]product.AbstractValue {
	if len(in) == 0 {
		return nil
	}
	out := make(map[cfg.SymbolID]product.AbstractValue, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func (b *Builder) seedEntrySymbolValues(out *flow.PointState) {
	if len(b.entrySymbols) == 0 {
		return
	}
	seeder, ok := b.transfer.(EntrySymbolValueSeeder)
	if !ok || seeder == nil {
		return
	}
	seeder.SeedEntrySymbolValues(out, b.entrySymbols)
}

func (b *Builder) seedEntryValues(out *flow.PointState) {
	if len(b.entryValues) == 0 {
		return
	}
	seeder, ok := b.transfer.(EntryValueSeeder)
	if !ok || seeder == nil {
		return
	}
	seeder.SeedEntryValues(out, b.entryValues)
}

// readContracts reads every contract cell into a paramevidence.Contracts map.
// It is called only by the entry point equation, recording the dependency so a
// contract change re-queues entry. Bottom (no obligation) cells are omitted,
// matching the MapLattice canonicalization.
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

// wideningSites is the combined feedback-vertex set: CFG loop-header / non-loop
// SCC-header point cells (from propagate.FeedbackVertexSet) plus every contract
// cell. A contract cell has no effect until a body use emits demand into it; once
// that happens it participates in the entry->body->contract->entry cycle through
// the entry point's contract read. The solver exact-joins pre-visit fan-in and
// delays widening for the first post-visit changes, so initial one-shot demand
// facts stay precise while continuing growth still terminates.
func (b *Builder) wideningSites() map[Cell]bool {
	sites := make(map[Cell]bool)
	for p, isFVS := range propagate.FeedbackVertexSet(b.graph) {
		if isFVS {
			sites[pointCellAt(p)] = true
		}
	}
	for param := 0; param < b.numParams; param++ {
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
		joined = b.projectPointState(p, joined)
		if flow.PointStateDomain.Equal(joined, flow.PointStateDomain.Bottom()) {
			continue
		}
		in[p] = joined
	}
	return in
}
