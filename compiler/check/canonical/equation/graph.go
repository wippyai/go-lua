package equation

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/canonical/state"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	basecfg "github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
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
	entryFacts    flow.BoundaryFacts
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

// WithEntryReferences installs the caller/lexical reference context visible at
// function entry. The builder decomposes it only when seeding PointState, keeping
// summary/query/cache callers on the normalized reference-context carrier.
func (b *Builder) WithEntryReferences(references flow.ReferenceContext) *Builder {
	b.entry = references.CaptureCells()
	b.entryRefs = flow.FunctionRefsDomain.Join(references.FunctionRefs(), nil)
	b.entryClosures = flow.ClosureRefsDomain.Join(references.ClosureRefs(), nil)
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

// WithEntryFacts returns b after installing parameter-relative path facts
// visible at function entry. These facts are seeded into PointState before local
// node transfer so the same kill/reduction logic owns their lifetime.
func (b *Builder) WithEntryFacts(facts flow.BoundaryFacts) *Builder {
	b.entryFacts = facts.Clone()
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
			ps = b.rebasePhiConditions(pred, p, ps)
			incoming = flow.PointStateDomain.Join(incoming, ps)
			// Keep the predecessor fold inside the point's abstract condition
			// domain; otherwise a high-fan-in merge can build an exact DNF that
			// is immediately forgotten by the projector below.
			incoming = b.projectInPointState(p, incoming)
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
		incoming = b.projectInPointState(p, incoming)

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

		var entryFacts flow.BoundaryFacts
		if p == entry {
			entryFacts = b.entryFacts
		}
		next := b.transfer.Transfer(b.graph, p, incoming, entryContracts, entryFacts, demand)
		next = b.projectOutPointState(p, next)

		// Emit the post-transfer state into p's own cell; successors read it.
		emit(pointCellAt(p), pointState(next))
	}
}

func (b *Builder) abstractCellState(cell Cell, st CellState) CellState {
	if cell.Kind != PointCell || st.Kind != PointCell {
		return st
	}
	return pointState(b.projectOutPointState(cell.Point, st.Point))
}

func (b *Builder) projectInPointState(p cfg.Point, ps flow.PointState) flow.PointState {
	if b == nil || b.projector == nil || !b.projector.Enabled() {
		return ps
	}
	ps.Cond = b.projector.Project(p, ps.Cond)
	return ps
}

func (b *Builder) projectOutPointState(p cfg.Point, ps flow.PointState) flow.PointState {
	if b == nil || b.projector == nil || !b.projector.Enabled() {
		return ps
	}
	ps.Cond = b.projector.ProjectOut(p, ps.Cond)
	return ps
}

type phiVersionKey struct {
	sym basecfg.SymbolID
	id  int
}

func (b *Builder) rebasePhiConditions(pred, succ cfg.Point, ps flow.PointState) flow.PointState {
	if b == nil || b.graph == nil || !ps.Cond.HasConstraints() {
		return ps
	}
	renames := b.phiConditionRenames(pred, succ)
	if len(renames) == 0 {
		return ps
	}
	ps.Cond = ps.Cond.MapPaths(func(path constraint.Path) constraint.Path {
		if path.Symbol == 0 || path.Version == 0 {
			return path
		}
		to, ok := renames[phiVersionKey{sym: path.Symbol, id: path.Version}]
		if !ok {
			return path
		}
		path.Root = to.Root
		path.Symbol = to.Symbol
		path.Version = to.ID
		return path
	})
	return ps
}

func (b *Builder) phiConditionRenames(pred, succ cfg.Point) map[phiVersionKey]cfg.Version {
	phis := b.graph.PhiNodes()
	if len(phis) == 0 {
		return nil
	}
	var renames map[phiVersionKey]cfg.Version
	for _, phi := range phis {
		if phi.Point != succ || phi.Target.IsZero() {
			continue
		}
		for _, op := range phi.Operands {
			if op.From != pred || op.Version.IsZero() {
				continue
			}
			if op.Version.Symbol == 0 || op.Version.Symbol != phi.Target.Symbol {
				continue
			}
			if renames == nil {
				renames = make(map[phiVersionKey]cfg.Version)
			}
			renames[phiVersionKey{sym: op.Version.Symbol, id: op.Version.ID}] = phi.Target
		}
	}
	return renames
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

func (b *Builder) seedEntryFacts(out *flow.PointState) {
	if !b.entryFacts.HasProof() {
		return
	}
	seeder, ok := b.transfer.(EntryFactSeeder)
	if !ok || seeder == nil {
		return
	}
	seeder.SeedEntryFacts(out, b.entryFacts)
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
// SCC-header point cells (from propagate.FeedbackVertexSet), every contract
// cell, and the entry point cell when contract cells exist. A contract cell has
// no effect until a body use emits demand into it; once that happens it
// participates in the entry->body->contract->entry cycle through the entry
// point's contract read. The entry cell is the point-state side of that same
// feedback edge: contract widening bounds the demand carrier, while entry
// widening bounds the ordinary point-state facts regenerated from the changing
// assumed contract. The solver exact-joins pre-visit fan-in and delays widening
// for the first post-visit changes, so initial one-shot demand facts stay
// precise while continuing growth still terminates.
func (b *Builder) wideningSites() map[Cell]bool {
	sites := make(map[Cell]bool)
	for p, isFVS := range propagate.FeedbackVertexSet(b.graph) {
		if isFVS {
			sites[pointCellAt(p)] = true
		}
	}
	if b.numParams > 0 {
		sites[pointCellAt(b.graph.Entry())] = true
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
			ps = b.rebasePhiConditions(pred, p, ps)
			joined = flow.PointStateDomain.Join(joined, ps)
			any = true
		}
		if !any {
			continue
		}
		joined = b.projectInPointState(p, joined)
		if flow.PointStateDomain.Equal(joined, flow.PointStateDomain.Bottom()) {
			continue
		}
		in[p] = joined
	}
	return in
}
