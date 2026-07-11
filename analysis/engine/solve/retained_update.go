package solve

import (
	"context"
	"errors"
	"fmt"
	"sort"
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
	scratch                *RetainedSystem[Cell, State]
	result                 map[Cell]State
	resultVersions         map[Cell]uint64
	runOK, publishOK, done bool
	forceFull              bool
	fallbackEdges          map[edge[Cell]]struct{}
	regionalExtra          []Cell
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
	if u.base.released || u.base.generation != u.baseGeneration {
		return ErrUpdateStale
	}
	if len(u.changed) == 0 && !u.forceFull {
		u.scratch = cloneRetained(u.base)
		u.installBindings()
		u.runOK = true
		return nil
	}
	if u.forceFull {
		return u.runFull(ctx)
	}
	region := u.base.invalidationRegion(u.changed)
	for attempts := 0; attempts <= len(u.base.cells); attempts++ {
		scratch, expansion, backward, runErr := u.runRegion(ctx, region)
		if runErr != nil {
			return runErr
		}
		if backward != nil {
			u.fallbackEdges = map[edge[Cell]]struct{}{*backward: {}}
			if scratch != nil {
				scratch.Release()
			}
			return u.runFull(ctx)
		}
		if len(expansion) != 0 {
			if scratch != nil {
				scratch.Release()
			}
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
			region = u.base.extendRegion(region, expansion)
			continue
		}
		u.scratch = scratch
		u.runOK = true
		return nil
	}
	return u.runFull(ctx)
}

func (u *Update[Cell, State]) runFull(ctx context.Context) error {
	sys := u.base.system
	sys.Transfer = u.transfer
	sys.TransferVersioned = u.versioned
	plan := u.base.plan
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
	_, _, scratch, err := BuildRetainedWTO(ctx, sys, plan, u.base.budget)
	if errors.Is(err, ErrWTOPlanUncovered) && len(u.fallbackEdges) == 0 {
		observed, discoverErr := discoverInfluences(ctx, sys)
		if discoverErr != nil {
			return discoverErr
		}
		plan = rebuiltPlan(plan, observed)
		_, _, scratch, err = BuildRetainedWTO(ctx, sys, plan, u.base.budget)
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
	u.result, u.resultVersions = cloneMap(s.cur), cloneMap(s.versions)
	u.publishOK = true
	return cloneMap(u.result), cloneMap(u.resultVersions), nil
}

// Commit atomically replaces the retained pre-narrowing generation. It is
// legal only after both Run and Publish succeeded.
func (u *Update[Cell, State]) Commit() error {
	if u == nil || u.done || !u.runOK || !u.publishOK || u.scratch == nil {
		return ErrUpdateState
	}
	r := u.base
	if r.released || r.generation != u.baseGeneration || r.updateGate == nil || !r.updateGate.Load() {
		u.Abort()
		return ErrUpdateStale
	}
	next := u.scratch
	next.generation = r.generation + 1
	gate := r.updateGate
	*r = *next
	r.updateGate = gate
	gate.Store(false)
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
	if u.base != nil && u.base.generation == u.baseGeneration {
		u.base.updateGate.Store(false)
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
}

func cloneRetained[Cell comparable, State any](r *RetainedSystem[Cell, State]) *RetainedSystem[Cell, State] {
	copy := *r
	copy.values, copy.versions, copy.initial = cloneMap(r.values), cloneMap(r.versions), cloneMap(r.initial)
	copy.visits, copy.widenChanges = cloneMap(r.visits), cloneMap(r.widenChanges)
	copy.cells, copy.emittedOnly = append([]Cell(nil), r.cells...), append([]Cell(nil), r.emittedOnly...)
	copy.owners = cloneOwners(r.owners)
	copy.readers = cloneReverse(r.readers)
	copy.outputOwners = cloneReverse(r.outputOwners)
	copy.released = false
	copy.updateGate = &atomic.Bool{}
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

func stateFromRetained[Cell comparable, State any](r *RetainedSystem[Cell, State]) *solveState[Cell, State] {
	s := newStructuredState(r.system)
	s.cur, s.versions, s.initial = cloneMap(r.values), cloneMap(r.versions), cloneMap(r.initial)
	s.visits, s.widenChanges = cloneMap(r.visits), cloneMap(r.widenChanges)
	s.nextVersion = r.nextVersion
	s.emittedOrder = append([]Cell(nil), r.emittedOnly...)
	s.declaredCur = len(s.order)
	return s
}

func rebuiltPlan[Cell comparable](p *WTOPlan[Cell], extra map[edge[Cell]]struct{}) *WTOPlan[Cell] {
	return NewWTOPlan(p.cells, func(from Cell) []Cell {
		out := make([]Cell, 0)
		for _, to := range p.cells {
			e := edge[Cell]{from: from, to: to}
			if _, ok := p.edges[e]; ok {
				out = append(out, to)
				continue
			}
			if _, ok := extra[e]; ok {
				out = append(out, to)
			}
		}
		return out
	})
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
	for _, owner := range r.owners {
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
	adj, nodes := r.influenceGraph()
	component, members := stronglyConnected(nodes, adj)
	selected := make(map[int]struct{})
	queue := make([]int, 0, len(changed))
	for _, cell := range changed {
		id := component[cell]
		if _, ok := selected[id]; !ok {
			selected[id] = struct{}{}
			queue = append(queue, id)
		}
	}
	componentEdges := make(map[int]map[int]struct{})
	for from, tos := range adj {
		for _, to := range tos {
			a, b := component[from], component[to]
			if a == b {
				continue
			}
			if componentEdges[a] == nil {
				componentEdges[a] = make(map[int]struct{})
			}
			componentEdges[a][b] = struct{}{}
		}
	}
	for head := 0; head < len(queue); head++ {
		for next := range componentEdges[queue[head]] {
			if _, ok := selected[next]; ok {
				continue
			}
			selected[next] = struct{}{}
			queue = append(queue, next)
		}
	}
	region := make(map[Cell]struct{})
	for id := range selected {
		for _, cell := range members[id] {
			region[cell] = struct{}{}
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

func (u *Update[Cell, State]) runRegion(ctx context.Context, region map[Cell]struct{}) (*RetainedSystem[Cell, State], []Cell, *edge[Cell], error) {
	r := cloneRetained(u.base)
	r.system.Transfer, r.system.TransferVersioned = u.transfer, u.versioned
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
		ownerIndex[r.owners[i].owner] = i
	}
	// Retract every invalidated equation bag before reconstructing region values.
	for owner := range region {
		if i, ok := ownerIndex[owner]; ok {
			r.owners[i].reads = nil
			r.owners[i].outputs = nil
		}
		delete(r.visits, owner)
		delete(r.widenChanges, owner)
	}
	for cell := range region {
		r.resetFromOwners(cell)
	}

	var active Cell
	var stagedReads map[Cell]retainedRead[Cell, State]
	var stagedOutputs map[Cell]State
	var stagedOrder []Cell
	var expansion []Cell
	expandSet := make(map[Cell]struct{})
	var backward *edge[Cell]
	read := func(d Cell) State {
		e := edge[Cell]{from: d, to: active}
		if !r.plan.coversInfluence(e.from, e.to) && backward == nil {
			copy := e
			backward = &copy
		}
		value, ok := r.values[d]
		if !ok {
			value = r.system.Lattice.Bottom()
			r.values[d] = value
			r.bumpRetained(d)
			if _, declared := r.plan.index[d]; !declared {
				r.cells = append(r.cells, d)
				r.emittedOnly = append(r.emittedOnly, d)
			}
		}
		stagedReads[d] = retainedRead[Cell, State]{dependency: d, value: value, revision: r.versions[d]}
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
	readVersioned := func(d Cell) (State, uint64) { value := read(d); return value, r.versions[d] }
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
		stagedReads = make(map[Cell]retainedRead[Cell, State])
		stagedOutputs = make(map[Cell]State)
		stagedOrder = nil
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
			i = len(r.owners)
			ownerIndex[cell] = i
			r.owners = append(r.owners, retainedOwner[Cell, State]{owner: cell})
		}
		old := r.owners[i].outputs
		r.owners[i].reads, r.owners[i].outputs = reads, outputs
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
				before := r.versions[item.Vertex]
				if err := runCell(item.Vertex); err != nil {
					return err
				}
				if err := runPartition(item.Body); err != nil {
					return err
				}
				if backward != nil || len(expansion) != 0 || r.versions[item.Vertex] == before {
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
	r.rebuildDerived()
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
	for _, owner := range r.owners {
		for _, output := range owner.outputs {
			if output.destination == cell {
				value = r.system.Lattice.Join(value, output.contribution)
			}
		}
	}
	if r.system.Abstract != nil {
		value = r.system.Abstract(cell, value)
	}
	return value
}
func (r *RetainedSystem[Cell, State]) resetFromOwners(cell Cell) {
	next := r.aggregate(cell)
	prev, ok := r.values[cell]
	if !ok || !r.system.Lattice.Equal(prev, next) {
		r.values[cell] = next
		r.bumpRetained(cell)
	}
}
func (r *RetainedSystem[Cell, State]) accumulateFromOwners(cell Cell) {
	aggregate := r.aggregate(cell)
	prev, ok := r.values[cell]
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
		r.values[cell] = next
		r.bumpRetained(cell)
	}
}
func (r *RetainedSystem[Cell, State]) bumpRetained(cell Cell) {
	r.nextVersion++
	r.versions[cell] = r.nextVersion
}

func (r *RetainedSystem[Cell, State]) rebuildDerived() {
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
	r.usage = usage
}
