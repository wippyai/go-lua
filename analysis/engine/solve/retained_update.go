package solve

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"

	"github.com/wippyai/go-lua/analysis/engine/cancellation"
)

// Transfer and TransferVersioned name replaceable equation bindings used by a
// retained update. They intentionally have the same contracts as the fields on
// EquationSystem.
type Transfer[Cell comparable, State any] func(Cell, func(Cell) State, func(Cell, State))
type TransferVersioned[Cell comparable, State any] func(Cell, func(Cell) (State, uint64), func(Cell, State))

var (
	ErrUpdateActive     = errors.New("solve: retained update already active")
	ErrUpdateState      = errors.New("solve: invalid retained update state")
	ErrRetainedReleased = errors.New("solve: retained system released")
	ErrTransferPanic    = errors.New("solve: transfer callback panicked")
	ErrUpdateStale      = errors.New("solve: retained update generation changed")
)

// Update is a transaction-owned overlay. No mutation reaches its base retained
// generation until Commit swaps the complete converged checkpoint.
type Update[Cell comparable, State any] struct {
	base                   *RetainedSystem[Cell, State]
	baseGeneration         uint64
	changed                []Cell
	transfer               Transfer[Cell, State]
	versioned              TransferVersioned[Cell, State]
	stats                  *Stats
	statsSet               bool
	scratch                *RetainedSystem[Cell, State]
	result                 map[Cell]State
	resultVersions         map[Cell]uint64
	runOK, publishOK, done bool
	forceFull              bool
	fallbackEdges          map[edge[Cell]]struct{}
	regionalExtra          []Cell
}

// SetStats transactionally replaces the observational counter receiving work
// performed by this update. Calling SetStats(nil) explicitly disables stats.
func (u *Update[Cell, State]) SetStats(stats *Stats) error {
	if u == nil || u.done || u.runOK {
		return ErrUpdateState
	}
	u.stats, u.statsSet = stats, true
	return nil
}

// BeginUpdate starts the only active transaction for r. changed names equation
// owners whose binding or dynamic environment changed.
func (r *RetainedSystem[Cell, State]) BeginUpdate(changed []Cell, transfer Transfer[Cell, State], versioned TransferVersioned[Cell, State]) (*Update[Cell, State], error) {
	if r == nil || r.updateGate == nil {
		return nil, ErrRetainedReleased
	}
	if !r.updateGate.CompareAndSwap(false, true) {
		return nil, ErrUpdateActive
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.released || r.plan == nil {
		r.updateGate.Store(false)
		return nil, ErrRetainedReleased
	}
	seen := make(map[Cell]struct{}, len(changed))
	normalized := make([]Cell, 0, len(changed))
	for _, cell := range changed {
		if _, declared := r.plan.index[cell]; !declared {
			r.updateGate.Store(false)
			return nil, fmt.Errorf("%w: changed owner is not a declared equation", ErrUpdateState)
		}
		if _, duplicate := seen[cell]; duplicate {
			continue
		}
		seen[cell] = struct{}{}
		normalized = append(normalized, cell)
	}
	sort.Slice(normalized, func(i, j int) bool { return r.plan.index[normalized[i]] < r.plan.index[normalized[j]] })
	bindingChanged := transfer != nil || versioned != nil
	useOriginal := transfer == nil
	if useOriginal {
		transfer = Transfer[Cell, State](r.system.Transfer)
	}
	if versioned == nil && r.system.TransferVersioned != nil && useOriginal {
		versioned = TransferVersioned[Cell, State](r.system.TransferVersioned)
	}
	return &Update[Cell, State]{base: r, baseGeneration: r.generation, changed: normalized, transfer: transfer, versioned: versioned, forceFull: bindingChanged && len(normalized) == 0}, nil
}

// RequireFullFallback marks an unowned external observation (or another
// caller-known identity change) that cannot safely be attributed to a region.
func (u *Update[Cell, State]) RequireFullFallback() {
	if u != nil && !u.done {
		u.forceFull = true
	}
}

// Run converges a transaction-owned pre-narrowing overlay.
func (u *Update[Cell, State]) Run(ctx context.Context) (err error) {
	if u == nil || u.done || u.runOK || u.base == nil {
		return ErrUpdateState
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			u.releaseScratch()
			err = fmt.Errorf("%w: %v", ErrTransferPanic, recovered)
		}
	}()
	if len(u.changed) == 0 && !u.forceFull {
		u.base.mu.RLock()
		if u.base.released || u.base.generation != u.baseGeneration {
			u.base.mu.RUnlock()
			return ErrUpdateStale
		}
		u.scratch = cloneRetained(u.base)
		u.base.mu.RUnlock()
		u.installBindings()
		u.runOK = true
		return nil
	}
	if u.forceFull {
		return u.runFull(ctx)
	}
	u.base.mu.RLock()
	if u.base.released || u.base.generation != u.baseGeneration {
		u.base.mu.RUnlock()
		return ErrUpdateStale
	}
	u.scratch = cloneRetained(u.base)
	u.base.mu.RUnlock()
	region := u.scratch.invalidationRegion(u.changed)
	for attempts := 0; attempts <= len(u.scratch.cells); attempts++ {
		scratch, expansion, backward, runErr := u.runRegion(ctx, u.scratch, region)
		if runErr != nil {
			return runErr
		}
		if backward != nil {
			u.fallbackEdges = map[edge[Cell]]struct{}{*backward: {}}
			u.releaseScratch()
			return u.runFull(ctx)
		}
		if len(expansion) != 0 {
			for _, cell := range expansion {
				seen := false
				for _, prior := range u.regionalExtra {
					if prior == cell {
						seen = true
						break
					}
				}
				if !seen {
					u.regionalExtra = append(u.regionalExtra, cell)
				}
			}
			region = u.scratch.extendRegion(region, expansion)
			continue
		}
		u.scratch = scratch
		u.runOK = true
		return nil
	}
	return u.runFull(ctx)
}

func (u *Update[Cell, State]) runFull(ctx context.Context) error {
	u.base.mu.RLock()
	if u.base.released || u.base.generation != u.baseGeneration {
		u.base.mu.RUnlock()
		return ErrUpdateStale
	}
	sys, plan, budget := u.base.system, u.base.plan, u.base.budget
	u.base.mu.RUnlock()
	sys.Transfer = u.transfer
	sys.TransferVersioned = u.versioned
	if u.statsSet {
		sys.Stats = u.stats
	}
	if len(u.fallbackEdges) != 0 {
		observed, err := discoverInfluences(ctx, sys)
		if err != nil {
			return err
		}
		for influence := range u.fallbackEdges {
			observed[influence] = struct{}{}
		}
		plan = rebuiltPlan(plan, observed)
	}
	_, _, scratch, err := BuildRetainedWTO(ctx, sys, plan, budget)
	if errors.Is(err, ErrWTOPlanUncovered) && len(u.fallbackEdges) == 0 {
		observed, discoverErr := discoverInfluences(ctx, sys)
		if discoverErr != nil {
			return discoverErr
		}
		plan = rebuiltPlan(plan, observed)
		_, _, scratch, err = BuildRetainedWTO(ctx, sys, plan, budget)
	}
	if err != nil {
		return err
	}
	u.scratch = scratch
	u.runOK = true
	return nil
}

func discoverInfluences[Cell comparable, State any](ctx context.Context, sys EquationSystem[Cell, State]) (map[edge[Cell]]struct{}, error) {
	observed := make(map[edge[Cell]]struct{})
	original, originalVersioned := sys.Transfer, sys.TransferVersioned
	var active Cell
	wrapRead := func(read func(Cell) State) func(Cell) State {
		return func(d Cell) State { observed[edge[Cell]{from: d, to: active}] = struct{}{}; return read(d) }
	}
	wrapEmit := func(emit func(Cell, State)) func(Cell, State) {
		return func(d Cell, value State) { observed[edge[Cell]{from: active, to: d}] = struct{}{}; emit(d, value) }
	}
	sys.Transfer = func(cell Cell, read func(Cell) State, emit func(Cell, State)) {
		active = cell
		original(cell, wrapRead(read), wrapEmit(emit))
	}
	if originalVersioned != nil {
		sys.TransferVersioned = func(cell Cell, read func(Cell) (State, uint64), emit func(Cell, State)) {
			active = cell
			originalVersioned(cell, func(d Cell) (State, uint64) { observed[edge[Cell]{from: d, to: active}] = struct{}{}; return read(d) }, wrapEmit(emit))
		}
	}
	sys.Stats = nil
	_, _, err := SolveContextWithVersions(ctx, sys)
	return observed, err
}

// Publish narrows a clone of the converged overlay. The retained ascent is not
// modified, including on cancellation or projection rejection by the caller.
func (u *Update[Cell, State]) Publish(ctx context.Context) (values map[Cell]State, versions map[Cell]uint64, err error) {
	if u == nil || u.done || !u.runOK || u.scratch == nil {
		return nil, nil, ErrUpdateState
	}
	u.publishOK = false
	u.result, u.resultVersions = nil, nil
	defer func() {
		if recovered := recover(); recovered != nil {
			values, versions = nil, nil
			err = fmt.Errorf("%w: %v", ErrTransferPanic, recovered)
		}
	}()
	cancel := newCancellationGuard(cancellation.FromContext(ctx))
	if err := cancel.err(0); err != nil {
		return nil, nil, err
	}
	if u.scratch.system.Lattice.Narrow == nil || u.scratch.system.WidenAt == nil {
		u.result, u.resultVersions = u.scratch.materializeValues(), u.scratch.materializeVersions()
		u.publishOK = true
		return u.result, u.resultVersions, nil
	}
	s := stateFromRetained(u.scratch)
	if err := s.runNarrowing(cancel); err != nil {
		return nil, nil, err
	}
	if err := cancel.err(0); err != nil {
		return nil, nil, err
	}
	// Update publication includes emitted-only cells because their narrowing is
	// part of the owned equation projection, even though the legacy Solve APIs
	// retain their historical declared-cells-only result contract.
	u.result, u.resultVersions = s.cur, s.versions
	u.publishOK = true
	return u.result, u.resultVersions, nil
}

// Commit atomically replaces the retained pre-narrowing generation. It is
// legal only after both Run and Publish succeeded.
func (u *Update[Cell, State]) Commit() error {
	if u == nil || u.done || !u.runOK || !u.publishOK || u.scratch == nil {
		return ErrUpdateState
	}
	r := u.base
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.released || r.generation != u.baseGeneration || r.updateGate == nil || !r.updateGate.Load() {
		u.releaseScratch()
		if r.updateGate != nil {
			r.updateGate.Store(false)
		}
		u.done = true
		u.result, u.resultVersions = nil, nil
		return ErrUpdateStale
	}
	next := u.scratch
	next.generation = r.generation + 1
	r.values, r.versions, r.initial = next.values, next.versions, next.initial
	r.valueBase, r.versionBase = next.valueBase, next.versionBase
	r.visits, r.widenChanges = next.visits, next.widenChanges
	r.cells, r.emittedOnly, r.nextVersion = next.cells, next.emittedOnly, next.nextVersion
	r.owners, r.ownerDelta, r.readers, r.outputOwners, r.outputOwnerDelta = next.owners, next.ownerDelta, next.readers, next.outputOwners, next.outputOwnerDelta
	r.contributions, r.contributionBase, r.contributionRemoved = next.contributions, next.contributionBase, next.contributionRemoved
	r.usage = next.usage
	r.system, r.plan, r.budget = next.system, next.plan, next.budget
	r.influences, r.influenceBase, r.cellsShared = next.influences, next.influenceBase, next.cellsShared
	r.generation, r.released = next.generation, false
	r.updateGate.Store(false)
	u.scratch = nil
	u.done = true
	return nil
}

// Abort discards all transaction-owned state and leaves the base generation
// byte-for-byte semantically unchanged.
func (u *Update[Cell, State]) Abort() {
	if u == nil || u.done {
		return
	}
	u.releaseScratch()
	if u.base != nil {
		u.base.mu.RLock()
		current := u.base.generation == u.baseGeneration
		u.base.mu.RUnlock()
		if current {
			u.base.updateGate.Store(false)
		}
	}
	u.done = true
	u.result, u.resultVersions = nil, nil
}

func (u *Update[Cell, State]) releaseScratch() {
	if u.scratch != nil {
		u.scratch.Release()
		u.scratch = nil
	}
}

func (u *Update[Cell, State]) installBindings() {
	u.scratch.system.Transfer = u.transfer
	u.scratch.system.TransferVersioned = u.versioned
	if u.statsSet {
		u.scratch.system.Stats = u.stats
	}
}

func cloneRetained[Cell comparable, State any](r *RetainedSystem[Cell, State]) *RetainedSystem[Cell, State] {
	copy := *r
	if r.valueBase == nil {
		copy.valueBase, copy.versionBase = r.values, r.versions
		copy.values = make(map[Cell]State)
		copy.versions = make(map[Cell]uint64)
	} else {
		copy.valueBase, copy.versionBase = r.valueBase, r.versionBase
		copy.values, copy.versions = cloneMap(r.values), cloneMap(r.versions)
	}
	// Initial boundary states are generation-immutable; regional replay never
	// replaces them (callers use full fallback for an initial binding change).
	copy.initial = r.initial
	copy.visits, copy.widenChanges = cloneMap(r.visits), cloneMap(r.widenChanges)
	copy.cells, copy.emittedOnly, copy.cellsShared = r.cells, r.emittedOnly, true
	copy.owners = r.owners
	copy.ownerDelta = cloneMap(r.ownerDelta)
	copy.readers = append([]retainedReverse[Cell](nil), r.readers...)
	copy.outputOwners = r.outputOwners
	copy.outputOwnerDelta = cloneMap(r.outputOwnerDelta)
	if r.contributionBase == nil {
		copy.contributionBase = r.contributions
		copy.contributions = make(map[Cell][]retainedContribution[Cell, State])
		copy.contributionRemoved = make(map[Cell]struct{})
	} else {
		copy.contributionBase = r.contributionBase
		copy.contributions = cloneContributions(r.contributions)
		copy.contributionRemoved = cloneMap(r.contributionRemoved)
	}
	if r.influenceBase == nil {
		copy.influenceBase = r.influences
		copy.influences = make(map[Cell][]Cell)
	} else {
		copy.influenceBase = r.influenceBase
		copy.influences = make(map[Cell][]Cell, len(r.influences))
		for cell, next := range r.influences {
			copy.influences[cell] = next
		}
	}
	copy.released = false
	copy.updateGate = &atomic.Bool{}
	copy.mu = &sync.RWMutex{}
	return &copy
}

func cloneOwners[Cell comparable, State any](in []retainedOwner[Cell, State]) []retainedOwner[Cell, State] {
	out := make([]retainedOwner[Cell, State], len(in))
	for i := range in {
		out[i] = retainedOwner[Cell, State]{owner: in[i].owner, reads: append([]retainedRead[Cell, State](nil), in[i].reads...), outputs: append([]retainedOutput[Cell, State](nil), in[i].outputs...)}
	}
	return out
}
func cloneReverse[Cell comparable](in []retainedReverse[Cell]) []retainedReverse[Cell] {
	out := make([]retainedReverse[Cell], len(in))
	for i := range in {
		out[i] = retainedReverse[Cell]{cell: in[i].cell, owners: append([]Cell(nil), in[i].owners...)}
	}
	return out
}

func cloneContributions[Cell comparable, State any](in map[Cell][]retainedContribution[Cell, State]) map[Cell][]retainedContribution[Cell, State] {
	out := make(map[Cell][]retainedContribution[Cell, State], len(in))
	for cell, items := range in {
		out[cell] = items
	}
	return out
}

func (r *RetainedSystem[Cell, State]) retainedValue(cell Cell) (State, bool) {
	if value, ok := r.values[cell]; ok {
		return value, true
	}
	value, ok := r.valueBase[cell]
	return value, ok
}

func (r *RetainedSystem[Cell, State]) retainedVersion(cell Cell) uint64 {
	if version, ok := r.versions[cell]; ok {
		return version
	}
	return r.versionBase[cell]
}

func (r *RetainedSystem[Cell, State]) materializeValues() map[Cell]State {
	if r.valueBase == nil {
		return r.values
	}
	out := cloneMap(r.valueBase)
	for cell, value := range r.values {
		out[cell] = value
	}
	return out
}

func (r *RetainedSystem[Cell, State]) materializeVersions() map[Cell]uint64 {
	if r.versionBase == nil {
		return r.versions
	}
	out := cloneMap(r.versionBase)
	for cell, version := range r.versions {
		out[cell] = version
	}
	return out
}

func (r *RetainedSystem[Cell, State]) retainedContributions(cell Cell) []retainedContribution[Cell, State] {
	if _, removed := r.contributionRemoved[cell]; removed {
		return nil
	}
	if items, ok := r.contributions[cell]; ok {
		return items
	}
	return r.contributionBase[cell]
}

func (r *RetainedSystem[Cell, State]) retainedInfluences(cell Cell) []Cell {
	if items, ok := r.influences[cell]; ok {
		return items
	}
	return r.influenceBase[cell]
}

func (r *RetainedSystem[Cell, State]) retainedOwnerAt(index int) retainedOwner[Cell, State] {
	if owner, ok := r.ownerDelta[index]; ok {
		return owner
	}
	return r.owners[index]
}

func (r *RetainedSystem[Cell, State]) setRetainedOwner(index int, owner retainedOwner[Cell, State]) {
	if r.ownerDelta == nil {
		r.ownerDelta = make(map[int]retainedOwner[Cell, State])
	}
	r.ownerDelta[index] = owner
}

func (r *RetainedSystem[Cell, State]) materializeOwners() []retainedOwner[Cell, State] {
	out := append([]retainedOwner[Cell, State](nil), r.owners...)
	for index, owner := range r.ownerDelta {
		out[index] = owner
	}
	return out
}

func (r *RetainedSystem[Cell, State]) ensureCellsOwned() {
	if !r.cellsShared {
		return
	}
	r.cells = append([]Cell(nil), r.cells...)
	r.emittedOnly = append([]Cell(nil), r.emittedOnly...)
	r.cellsShared = false
}

func contributionIndex[Cell comparable, State any](owners []retainedOwner[Cell, State]) map[Cell][]retainedContribution[Cell, State] {
	out := make(map[Cell][]retainedContribution[Cell, State])
	for _, owner := range owners {
		for _, output := range owner.outputs {
			out[output.destination] = append(out[output.destination], retainedContribution[Cell, State]{owner: owner.owner, contribution: output.contribution})
		}
	}
	return out
}

func stateFromRetained[Cell comparable, State any](r *RetainedSystem[Cell, State]) *solveState[Cell, State] {
	s := newStructuredState(r.system, true)
	s.cur, s.versions, s.initial = r.materializeValues(), r.materializeVersions(), cloneMap(r.initial)
	s.visits, s.widenChanges = cloneMap(r.visits), cloneMap(r.widenChanges)
	s.nextVersion = r.nextVersion
	s.emittedOrder = append([]Cell(nil), r.emittedOnly...)
	s.declaredCur = len(s.order)
	return s
}

func rebuiltPlan[Cell comparable](p *WTOPlan[Cell], extra map[edge[Cell]]struct{}) *WTOPlan[Cell] {
	sets := make(map[Cell]map[Cell]struct{})
	add := func(e edge[Cell]) {
		if _, ok := p.index[e.from]; !ok {
			return
		}
		if _, ok := p.index[e.to]; !ok {
			return
		}
		if sets[e.from] == nil {
			sets[e.from] = make(map[Cell]struct{})
		}
		sets[e.from][e.to] = struct{}{}
	}
	for e := range p.edges {
		add(e)
	}
	for e := range extra {
		add(e)
	}
	adj := make(map[Cell][]Cell, len(sets))
	for from, set := range sets {
		for to := range set {
			adj[from] = append(adj[from], to)
		}
		sort.Slice(adj[from], func(i, j int) bool { return p.index[adj[from][i]] < p.index[adj[from][j]] })
	}
	return NewWTOPlan(p.cells, func(from Cell) []Cell { return adj[from] })
}

func (r *RetainedSystem[Cell, State]) influenceGraph() (map[Cell][]Cell, []Cell) {
	nodes := append([]Cell(nil), r.cells...)
	known := make(map[Cell]struct{}, len(nodes))
	for _, c := range nodes {
		known[c] = struct{}{}
	}
	adjSet := make(map[Cell]map[Cell]struct{})
	add := func(from, to Cell) {
		if _, ok := known[from]; !ok {
			known[from] = struct{}{}
			nodes = append(nodes, from)
		}
		if _, ok := known[to]; !ok {
			known[to] = struct{}{}
			nodes = append(nodes, to)
		}
		if adjSet[from] == nil {
			adjSet[from] = make(map[Cell]struct{})
		}
		adjSet[from][to] = struct{}{}
	}
	for e := range r.plan.edges {
		add(e.from, e.to)
	}
	for _, rev := range r.readers {
		for _, owner := range rev.owners {
			add(rev.cell, owner)
		}
	}
	for i := range r.owners {
		owner := r.retainedOwnerAt(i)
		for _, output := range owner.outputs {
			add(owner.owner, output.destination)
		}
	}
	order := make(map[Cell]int, len(nodes))
	for i, c := range nodes {
		order[c] = i
	}
	adj := make(map[Cell][]Cell, len(adjSet))
	for from, set := range adjSet {
		for to := range set {
			adj[from] = append(adj[from], to)
		}
		sort.Slice(adj[from], func(i, j int) bool { return order[adj[from][i]] < order[adj[from][j]] })
	}
	return adj, nodes
}

func (r *RetainedSystem[Cell, State]) invalidationRegion(changed []Cell) map[Cell]struct{} {
	region := make(map[Cell]struct{}, len(changed))
	queue := make([]Cell, 0, len(changed))
	for _, cell := range changed {
		if _, ok := region[cell]; !ok {
			region[cell] = struct{}{}
			queue = append(queue, cell)
		}
	}
	for head := 0; head < len(queue); head++ {
		for _, next := range r.retainedInfluences(queue[head]) {
			if _, ok := region[next]; ok {
				continue
			}
			region[next] = struct{}{}
			queue = append(queue, next)
		}
	}
	return region
}

func (r *RetainedSystem[Cell, State]) extendRegion(region map[Cell]struct{}, expansion []Cell) map[Cell]struct{} {
	changed := make([]Cell, 0, len(region)+len(expansion))
	for cell := range region {
		if _, declared := r.plan.index[cell]; declared {
			changed = append(changed, cell)
		}
	}
	for _, cell := range expansion {
		if _, declared := r.plan.index[cell]; declared {
			changed = append(changed, cell)
		}
	}
	next := r.invalidationRegion(changed)
	for cell := range region {
		next[cell] = struct{}{}
	}
	for _, cell := range expansion {
		next[cell] = struct{}{}
	}
	return next
}

func stronglyConnected[Cell comparable](nodes []Cell, adj map[Cell][]Cell) (map[Cell]int, [][]Cell) {
	index, low := make(map[Cell]int), make(map[Cell]int)
	onStack := make(map[Cell]bool)
	stack := make([]Cell, 0, len(nodes))
	next := 1
	component := make(map[Cell]int)
	members := make([][]Cell, 0)
	var visit func(Cell)
	visit = func(v Cell) {
		index[v], low[v] = next, next
		next++
		stack = append(stack, v)
		onStack[v] = true
		for _, w := range adj[v] {
			if index[w] == 0 {
				visit(w)
				if low[w] < low[v] {
					low[v] = low[w]
				}
			} else if onStack[w] && index[w] < low[v] {
				low[v] = index[w]
			}
		}
		if low[v] != index[v] {
			return
		}
		id := len(members)
		group := make([]Cell, 0)
		for {
			w := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			onStack[w] = false
			component[w] = id
			group = append(group, w)
			if w == v {
				break
			}
		}
		members = append(members, group)
	}
	for _, cell := range nodes {
		if index[cell] == 0 {
			visit(cell)
		}
	}
	return component, members
}

func (u *Update[Cell, State]) runRegion(ctx context.Context, r *RetainedSystem[Cell, State], region map[Cell]struct{}) (*RetainedSystem[Cell, State], []Cell, *edge[Cell], error) {
	r.system.Transfer, r.system.TransferVersioned = u.transfer, u.versioned
	if u.statsSet {
		r.system.Stats = u.stats
	}
	for _, cell := range u.regionalExtra {
		if _, declared := r.plan.index[cell]; declared {
			continue
		}
		known := false
		for _, existing := range r.cells {
			if existing == cell {
				known = true
				break
			}
		}
		if !known {
			r.ensureCellsOwned()
			r.cells = append(r.cells, cell)
			r.emittedOnly = append(r.emittedOnly, cell)
		}
	}
	cancel := newCancellationGuard(cancellation.FromContext(ctx))
	if err := cancel.err(0); err != nil {
		r.Release()
		return nil, nil, nil, err
	}
	ownerIndex := make(map[Cell]int, len(r.owners))
	for i := range r.owners {
		ownerIndex[r.retainedOwnerAt(i).owner] = i
	}
	// Retract every invalidated equation bag before reconstructing region values.
	for owner := range region {
		if i, ok := ownerIndex[owner]; ok {
			r.replaceOwnerReads(i, nil)
			r.replaceOwnerOutputs(i, nil)
		}
		delete(r.visits, owner)
		delete(r.widenChanges, owner)
	}
	for cell := range region {
		r.resetFromOwners(cell)
	}

	var active Cell
	stagedReads := make(map[Cell]retainedRead[Cell, State])
	stagedOutputs := make(map[Cell]State)
	stagedOrder := make([]Cell, 0, 8)
	var expansion []Cell
	expandSet := make(map[Cell]struct{})
	var backward *edge[Cell]
	read := func(d Cell) State {
		e := edge[Cell]{from: d, to: active}
		if !r.plan.coversInfluence(e.from, e.to) && backward == nil {
			copy := e
			backward = &copy
		}
		value, ok := r.retainedValue(d)
		if !ok {
			value = r.system.Lattice.Bottom()
			r.values[d] = value
			r.bumpRetained(d)
			if _, declared := r.plan.index[d]; !declared {
				r.ensureCellsOwned()
				r.cells = append(r.cells, d)
				r.emittedOnly = append(r.emittedOnly, d)
			}
			r.usage.StateRefs++
		}
		stagedReads[d] = retainedRead[Cell, State]{dependency: d, value: value, revision: r.retainedVersion(d)}
		return value
	}
	emit := func(d Cell, value State) {
		if previous, ok := stagedOutputs[d]; ok {
			stagedOutputs[d] = r.system.Lattice.Join(previous, value)
		} else {
			stagedOutputs[d] = value
			stagedOrder = append(stagedOrder, d)
		}
	}
	readVersioned := func(d Cell) (State, uint64) { value := read(d); return value, r.retainedVersion(d) }
	var iteration uint64
	runCell := func(cell Cell) error {
		if _, selected := region[cell]; !selected {
			return nil
		}
		if err := cancel.err(iteration); err != nil {
			return err
		}
		iteration++
		active = cell
		clear(stagedReads)
		clear(stagedOutputs)
		stagedOrder = stagedOrder[:0]
		if r.system.Stats != nil {
			r.system.Stats.TransferCalls++
		}
		if u.versioned != nil {
			u.versioned(cell, readVersioned, emit)
		} else {
			u.transfer(cell, read, emit)
		}
		if err := cancel.err(0); err != nil {
			return err
		}
		if backward != nil {
			return nil
		}
		for _, destination := range stagedOrder {
			e := edge[Cell]{from: cell, to: destination}
			// Emissions are influences too. Validate them even when the
			// destination is already selected: a new self or backward edge can
			// change the component/nesting required to converge the region.
			if !r.plan.coversEmission(e.from, e.to) {
				copy := e
				backward = &copy
				return nil
			}
			_, declared := r.plan.index[destination]
			if _, covered := region[destination]; covered {
				continue
			}
			if declared && r.plan.coversInfluence(e.from, e.to) {
				if _, seen := expandSet[destination]; !seen {
					expandSet[destination] = struct{}{}
					expansion = append(expansion, destination)
				}
				continue
			}
			if !declared {
				if _, seen := expandSet[destination]; !seen {
					expandSet[destination] = struct{}{}
					expansion = append(expansion, destination)
				}
				continue
			}
			// The declared uncovered case was rejected above.
		}
		if len(expansion) != 0 {
			return nil
		}
		reads := make([]retainedRead[Cell, State], 0, len(stagedReads))
		for _, d := range r.cells {
			if item, ok := stagedReads[d]; ok {
				reads = append(reads, item)
			}
		}
		outputs := make([]retainedOutput[Cell, State], 0, len(stagedOrder))
		for _, d := range stagedOrder {
			outputs = append(outputs, retainedOutput[Cell, State]{destination: d, contribution: stagedOutputs[d]})
		}
		i, ok := ownerIndex[cell]
		if !ok {
			r.owners = r.materializeOwners()
			r.ownerDelta = nil
			i = len(r.owners)
			ownerIndex[cell] = i
			r.owners = append(r.owners, retainedOwner[Cell, State]{owner: cell})
			r.usage.Owners++
		}
		old := append([]retainedOutput[Cell, State](nil), r.retainedOwnerAt(i).outputs...)
		r.replaceOwnerReads(i, reads)
		r.replaceOwnerOutputs(i, outputs)
		affected := make(map[Cell]struct{}, len(old)+len(outputs))
		for _, item := range old {
			affected[item.destination] = struct{}{}
		}
		for _, item := range outputs {
			affected[item.destination] = struct{}{}
		}
		for destination := range affected {
			r.accumulateFromOwners(destination)
		}
		if r.system.WidenAt != nil && r.system.WidenAt(cell) {
			r.visits[cell]++
		}
		return nil
	}
	var runPartition func([]WTOElement[Cell]) error
	runPartition = func(items []WTOElement[Cell]) error {
		for _, item := range items {
			if backward != nil || len(expansion) != 0 {
				return nil
			}
			if !item.IsComponent() {
				if err := runCell(item.Vertex); err != nil {
					return err
				}
				continue
			}
			if _, selected := region[item.Vertex]; !selected {
				continue
			}
			for {
				before := r.retainedVersion(item.Vertex)
				if err := runCell(item.Vertex); err != nil {
					return err
				}
				if err := runPartition(item.Body); err != nil {
					return err
				}
				if backward != nil || len(expansion) != 0 || r.retainedVersion(item.Vertex) == before {
					break
				}
			}
		}
		return nil
	}
	if err := runPartition(r.plan.elements); err != nil {
		r.Release()
		return nil, nil, nil, err
	}
	if backward != nil || len(expansion) != 0 {
		return r, expansion, backward, nil
	}
	if err := retainedBudgetError(r.budget, r.usage); err != nil {
		r.Release()
		return nil, nil, nil, err
	}
	return r, nil, nil, nil
}

func (r *RetainedSystem[Cell, State]) initialOf(cell Cell) State {
	if value, ok := r.initial[cell]; ok {
		return value
	}
	return r.system.Lattice.Bottom()
}
func (r *RetainedSystem[Cell, State]) aggregate(cell Cell) State {
	value := r.initialOf(cell)
	for _, item := range r.retainedContributions(cell) {
		value = r.system.Lattice.Join(value, item.contribution)
	}
	if r.system.Abstract != nil {
		value = r.system.Abstract(cell, value)
	}
	return value
}

func (r *RetainedSystem[Cell, State]) replaceOwnerOutputs(index int, outputs []retainedOutput[Cell, State]) {
	item := r.retainedOwnerAt(index)
	owner := item.owner
	old := item.outputs
	r.usage.Outputs += len(outputs) - len(old)
	r.usage.StateRefs += len(outputs) - len(old)
	affected := make(map[Cell]struct{}, len(old)+len(outputs))
	for _, output := range old {
		affected[output.destination] = struct{}{}
	}
	for _, output := range outputs {
		affected[output.destination] = struct{}{}
	}
	item.outputs = outputs
	r.setRetainedOwner(index, item)
	for destination := range affected {
		items := r.retainedContributions(destination)
		kept := make([]retainedContribution[Cell, State], 0, len(items)+1)
		for _, item := range items {
			if item.owner != owner {
				kept = append(kept, item)
			}
		}
		for _, output := range outputs {
			if output.destination == destination {
				kept = append(kept, retainedContribution[Cell, State]{owner: owner, contribution: output.contribution})
				break
			}
		}
		if len(kept) == 0 {
			delete(r.contributions, destination)
			if r.contributionRemoved == nil {
				r.contributionRemoved = make(map[Cell]struct{})
			}
			r.contributionRemoved[destination] = struct{}{}
		} else {
			r.contributions[destination] = kept
			delete(r.contributionRemoved, destination)
		}
		present := false
		for _, output := range outputs {
			if output.destination == destination {
				present = true
				break
			}
		}
		r.setOutputReverse(destination, owner, present)
		r.refreshInfluence(owner, destination)
	}
}

func (r *RetainedSystem[Cell, State]) replaceOwnerReads(index int, reads []retainedRead[Cell, State]) {
	item := r.retainedOwnerAt(index)
	owner, old := item.owner, item.reads
	r.usage.Reads += len(reads) - len(old)
	r.usage.StateRefs += len(reads) - len(old)
	item.reads = reads
	r.setRetainedOwner(index, item)
	affected := make(map[Cell]struct{}, len(old)+len(reads))
	for _, item := range old {
		affected[item.dependency] = struct{}{}
	}
	for _, item := range reads {
		affected[item.dependency] = struct{}{}
	}
	for dependency := range affected {
		present := false
		for _, item := range reads {
			if item.dependency == dependency {
				present = true
				break
			}
		}
		r.setReverse(&r.readers, dependency, owner, present)
		r.refreshInfluence(dependency, owner)
	}
}

func (r *RetainedSystem[Cell, State]) setReverse(reverse *[]retainedReverse[Cell], cell, owner Cell, present bool) {
	index := -1
	for i := range *reverse {
		if (*reverse)[i].cell == cell {
			index = i
			break
		}
	}
	if index < 0 {
		if present {
			*reverse = append(*reverse, retainedReverse[Cell]{cell: cell, owners: []Cell{owner}})
		}
		return
	}
	owners := (*reverse)[index].owners
	found := -1
	for i, existing := range owners {
		if existing == owner {
			found = i
			break
		}
	}
	if (found >= 0) == present {
		return
	}
	next := append([]Cell(nil), owners...)
	if present {
		next = append(next, owner)
		sort.Slice(next, func(i, j int) bool { return r.plan.index[next[i]] < r.plan.index[next[j]] })
	} else {
		next = append(next[:found], next[found+1:]...)
	}
	(*reverse)[index].owners = next
}

func (r *RetainedSystem[Cell, State]) outputOwnersOf(cell Cell) []Cell {
	if owners, ok := r.outputOwnerDelta[cell]; ok {
		return owners
	}
	for _, item := range r.outputOwners {
		if item.cell == cell {
			return item.owners
		}
	}
	return nil
}

func (r *RetainedSystem[Cell, State]) setOutputReverse(cell, owner Cell, present bool) {
	owners := r.outputOwnersOf(cell)
	found := -1
	for i, existing := range owners {
		if existing == owner {
			found = i
			break
		}
	}
	if (found >= 0) == present {
		return
	}
	next := append([]Cell(nil), owners...)
	if present {
		next = append(next, owner)
		sort.Slice(next, func(i, j int) bool { return r.plan.index[next[i]] < r.plan.index[next[j]] })
	} else {
		next = append(next[:found], next[found+1:]...)
	}
	if r.outputOwnerDelta == nil {
		r.outputOwnerDelta = make(map[Cell][]Cell)
	}
	r.outputOwnerDelta[cell] = next
}

func (r *RetainedSystem[Cell, State]) refreshInfluence(from, to Cell) {
	_, wanted := r.plan.edges[edge[Cell]{from: from, to: to}]
	if !wanted {
		for _, item := range r.readers {
			if item.cell == from {
				for _, owner := range item.owners {
					if owner == to {
						wanted = true
						break
					}
				}
				break
			}
		}
	}
	if !wanted {
		for _, owner := range r.outputOwnersOf(to) {
			if owner == from {
				wanted = true
				break
			}
		}
	}
	items := r.retainedInfluences(from)
	found := -1
	for i, item := range items {
		if item == to {
			found = i
			break
		}
	}
	if (found >= 0) == wanted {
		return
	}
	next := append([]Cell(nil), items...)
	if wanted {
		next = append(next, to)
		sort.Slice(next, func(i, j int) bool { return r.plan.index[next[i]] < r.plan.index[next[j]] })
	} else {
		next = append(next[:found], next[found+1:]...)
	}
	r.influences[from] = next
}
func (r *RetainedSystem[Cell, State]) resetFromOwners(cell Cell) {
	next := r.aggregate(cell)
	prev, ok := r.retainedValue(cell)
	if !ok || !r.system.Lattice.Equal(prev, next) {
		if !ok {
			r.usage.StateRefs++
		}
		r.values[cell] = next
		r.bumpRetained(cell)
	}
}
func (r *RetainedSystem[Cell, State]) accumulateFromOwners(cell Cell) {
	aggregate := r.aggregate(cell)
	prev, ok := r.retainedValue(cell)
	if !ok {
		prev = r.system.Lattice.Bottom()
	}
	next := r.system.Lattice.Join(prev, aggregate)
	delay := 0
	if r.system.WidenDelay != nil {
		delay = max(0, r.system.WidenDelay(cell))
	}
	if r.system.WidenAt != nil && r.system.WidenAt(cell) && r.visits[cell] > 0 {
		if r.widenChanges[cell] >= delay {
			next = r.system.Lattice.Widen(prev, next)
		} else if !r.system.Lattice.Equal(prev, next) {
			r.widenChanges[cell]++
		}
	}
	if r.system.Abstract != nil {
		next = r.system.Abstract(cell, next)
	}
	if !ok || !r.system.Lattice.Equal(prev, next) {
		if !ok {
			r.usage.StateRefs++
		}
		r.values[cell] = next
		r.bumpRetained(cell)
	}
}
func (r *RetainedSystem[Cell, State]) bumpRetained(cell Cell) {
	r.nextVersion++
	r.versions[cell] = r.nextVersion
}

func (r *RetainedSystem[Cell, State]) rebuildDerived() {
	r.owners = r.materializeOwners()
	r.ownerDelta = nil
	readers, outputs := make(map[Cell][]Cell), make(map[Cell][]Cell)
	usage := RetainedUsage{Owners: len(r.owners), StateRefs: len(r.values) + len(r.initial)}
	for _, owner := range r.owners {
		usage.Reads += len(owner.reads)
		usage.Outputs += len(owner.outputs)
		usage.StateRefs += len(owner.reads) + len(owner.outputs)
		for _, read := range owner.reads {
			readers[read.dependency] = append(readers[read.dependency], owner.owner)
		}
		for _, output := range owner.outputs {
			outputs[output.destination] = append(outputs[output.destination], owner.owner)
		}
	}
	r.readers, r.outputOwners = compactRetainedReverse(r.cells, readers), compactRetainedReverse(r.cells, outputs)
	r.outputOwnerDelta = nil
	r.contributions = contributionIndex(r.owners)
	r.contributionBase, r.contributionRemoved = nil, nil
	r.rebuildInfluences()
	r.usage = usage
}

func (r *RetainedSystem[Cell, State]) rebuildInfluences() {
	adj, _ := r.influenceGraph()
	r.influences = adj
	r.influenceBase = nil
}
