package factbinding

import (
	"slices"
	"sort"
	"sync/atomic"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/diagram"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/scalar"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/semantic"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"github.com/wippyai/go-lua/analysis/identity"
)

const previewRootLimit uint64 = 1<<63 - 1

// Preflight consumes this fresh Binding and completes every local allocation
// and validation before carrier attaches any issuer. Its reserved initial root
// remains invisible until Attach performs the composition's final cut.
func (binding *Binding[K, V]) Preflight() (carrier.SlotOperation, bool) {
	if binding == nil {
		return nil, false
	}
	binding.lifecycle.Lock()
	defer binding.lifecycle.Unlock()
	if binding.prepared || binding.bound || !binding.sealed || binding.plane == nil {
		return nil, false
	}
	binding.prepared = true
	roots := newRootStore[planeFactor, K, V](binding.plane.domain)
	if roots == nil {
		return nil, false
	}
	empty, ok := binding.plane.domain.Empty()
	if !ok {
		return nil, false
	}
	initial, ok := roots.reserve(empty)
	if !ok {
		return nil, false
	}
	binding.roots, binding.initial = roots, initial
	return binding, true
}

// Attach binds the prepared Binding to one physical carrier slot and publishes
// its preflight reservation. There is no fallible work left at this point.
func (binding *Binding[K, V]) Attach(owner carrier.SlotOwner) carrier.RootHandle {
	if binding == nil {
		panic("invalid binding attachment")
	}
	binding.lifecycle.Lock()
	defer binding.lifecycle.Unlock()
	if !binding.prepared || binding.bound || binding.plane == nil || binding.roots == nil || binding.initial == nil {
		panic("invalid binding attachment")
	}
	binding.issuer.Attach(owner)
	id := binding.initial.Publish()
	root, ok := binding.issuer.IssueRoot(id)
	if !ok {
		panic("prepared binding root could not be issued")
	}
	binding.initial = nil
	binding.bound = true
	return root
}

// InitialRootReady proves Preflight reserved a valid initial root before the
// carrier begins its irreversible all-slot attachment cut.
func (binding *Binding[K, V]) InitialRootReady() bool {
	if binding == nil {
		return false
	}
	binding.lifecycle.Lock()
	defer binding.lifecycle.Unlock()
	return binding.prepared && !binding.bound && binding.plane != nil && binding.roots != nil && binding.initial != nil && binding.initial.store == binding.roots && binding.initial.base == binding.roots.directory.Load() && binding.initial.id != 0 && binding.plane.domain.Valid(binding.initial.plane)
}

// Guards returns the exact prepared guard authority.
func (binding *Binding[K, V]) Guards() *guard.Manager {
	if binding == nil || !binding.prepared || binding.plane == nil {
		return nil
	}
	return binding.plane.domain.Guards()
}

func (binding *Binding[K, V]) ValidRoot(handle carrier.RootHandle) bool {
	if binding == nil || !binding.live() || binding.plane == nil || binding.roots == nil {
		return false
	}
	id, ok := binding.issuer.ResolveRoot(handle)
	if !ok {
		return binding.issuer.ValidRoot(handle)
	}
	_, ok = binding.roots.Plane(id)
	return ok
}

func (binding *Binding[K, V]) live() bool {
	return binding != nil && binding.issuer.Live() && binding.bound
}

// Capability declarations are intentionally not inferred from K.  Full
// opaque capability identity is the map key; matching issuer/slot/kind alone
// is insufficient because it could otherwise admit an undeclared surface.
// Default is the Factor's declared absent-key value. It is the law a carry
// transform is held to: a map that moves the default invents a fact at every
// coordinate the Factor never wrote.
func (binding *Binding[K, V]) Default() (V, bool) {
	if binding == nil || binding.algebra == nil {
		var zero V
		return zero, false
	}
	return binding.algebra.Default()
}

// Top is the Factor's declared lattice top: the sound over-approximation of
// any value an opaque alternative could have written.
func (binding *Binding[K, V]) Top() (V, bool) {
	if binding == nil || binding.algebra == nil {
		var zero V
		return zero, false
	}
	return binding.algebra.Top()
}

// Equal is the Factor's declared value equality.
func (binding *Binding[K, V]) Equal(left, right V) bool {
	return binding != nil && binding.algebra != nil && binding.algebra.Equal(left, right)
}

func (binding *Binding[K, V]) ValidUnit(unit carrier.Unit) bool {
	if !binding.live() {
		return false
	}
	_, ok := binding.units[unit]
	return ok
}

func (binding *Binding[K, V]) ValidTarget(target carrier.Target) bool {
	if !binding.live() {
		return false
	}
	_, ok := binding.targets[target]
	return ok
}

// DeclaredUnit is the cold declaration-table membership proof consumed
// only by carrier.PreparedComposition.  It intentionally does not make the
// capability active: issuer Slot remains unavailable until Attach.
func (binding *Binding[K, V]) DeclaredUnit(unit carrier.Unit) bool {
	if binding == nil || !binding.prepared {
		return false
	}
	_, ok := binding.units[unit]
	return ok
}

// DeclaredTarget is the cold counterpart of ValidTarget.
func (binding *Binding[K, V]) DeclaredTarget(target carrier.Target) bool {
	if binding == nil || !binding.prepared {
		return false
	}
	_, ok := binding.targets[target]
	return ok
}

// TargetNotifications returns the possible reverse wake closure of one cold
// authored target. Target itself remains the exact opaque write scope; these
// Units are only consumers that a write through that scope can invalidate.
func (binding *Binding[K, V]) TargetNotifications(target carrier.Target) ([]carrier.Unit, bool) {
	if binding == nil || !binding.prepared {
		return nil, false
	}
	descriptor, ok := binding.targets[target]
	if !ok || len(descriptor.notifications) == 0 {
		return nil, false
	}
	return append([]carrier.Unit(nil), descriptor.notifications...), true
}

// PrepareWidening turns canonical Target capabilities into one Binding-local
// immutable key scope before execution starts. Carrier retains only the
// returned opaque ordinal in MergeScope; the hot merge path never scans Target
// capabilities or allocates key unions.
func (binding *Binding[K, V]) PrepareWidening(targets []carrier.Target) (uint64, bool) {
	return binding.prepareRecurrenceScope(carrier.Widen, targets, false)
}

// PrepareNarrowing is the key-local counterpart of PrepareWidening. A Narrow
// selection is never widened to an entire Factor: only its declared target
// keys receive the typed descent operation.
func (binding *Binding[K, V]) PrepareNarrowing(targets []carrier.Target) (uint64, bool) {
	return binding.prepareRecurrenceScope(carrier.Narrow, targets, false)
}

func (binding *Binding[K, V]) PrepareRuntimeWidening(targets []carrier.Target) (uint64, bool) {
	return binding.prepareRecurrenceScope(carrier.Widen, targets, true)
}

func (binding *Binding[K, V]) PrepareRuntimeNarrowing(targets []carrier.Target) (uint64, bool) {
	return binding.prepareRecurrenceScope(carrier.Narrow, targets, true)
}

func (binding *Binding[K, V]) prepareRecurrenceScope(kind carrier.MergeKind, targets []carrier.Target, runtime bool) (uint64, bool) {
	if binding == nil || len(targets) == 0 || (kind != carrier.Widen && kind != carrier.Narrow) {
		return 0, false
	}
	binding.lifecycle.Lock()
	defer binding.lifecycle.Unlock()
	if binding.scopeFrozen != runtime || !binding.live() || !binding.Supports(kind) {
		return 0, false
	}
	for index, target := range targets {
		descriptor, ok := binding.targets[target]
		if !ok || len(descriptor.keys) == 0 || index > 0 && !targetOrderLess(binding.targets[targets[index-1]].order, descriptor.order) {
			return 0, false
		}
	}
	scopes := &binding.widenScopes
	if kind == carrier.Narrow {
		scopes = &binding.narrowScopes
	}
	for index := range *scopes {
		if sameTargets((*scopes)[index].targets, targets) {
			return uint64(index + 1), true
		}
	}
	keys := make([]K, 0)
	for _, target := range targets {
		keys = append(keys, binding.targets[target].keys...)
	}
	sort.Slice(keys, func(left, right int) bool { return keys[left] < keys[right] })
	keys = uniqueKeys(keys)
	if len(keys) == 0 {
		return 0, false
	}
	*scopes = append(*scopes, widenScope[K]{targets: append([]carrier.Target(nil), targets...), keys: keys})
	return uint64(len(*scopes)), true
}

type widenScope[K scalar.Key] struct {
	targets []carrier.Target
	keys    []K
}

func sameTargets(left, right []carrier.Target) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if !left[index].Same(right[index]) {
			return false
		}
	}
	return true
}

func uniqueKeys[K scalar.Key](keys []K) []K {
	if len(keys) == 0 {
		return nil
	}
	write := 1
	for _, key := range keys[1:] {
		if key != keys[write-1] {
			keys[write] = key
			write++
		}
	}
	clear(keys[write:])
	return keys[:write]
}

// Supports declares which recurrence operations this sealed Factor admitted.
// Join is part of every Factor algebra; Widen and Narrow additionally require
// their well-founded measure and operation respectively.
func (binding *Binding[K, V]) Supports(kind carrier.MergeKind) bool {
	if binding == nil || !binding.prepared {
		return false
	}
	switch kind {
	case carrier.Join:
		return binding.algebra != nil && binding.algebra.join != nil
	case carrier.Widen:
		return binding.algebra != nil && binding.algebra.widen != nil && binding.algebra.widenRank.valid()
	case carrier.Narrow:
		return binding.algebra != nil && binding.algebra.narrow != nil && binding.algebra.narrowRank.valid()
	default:
		return false
	}
}

// NewWork creates one evaluator-local typed traversal store. Binding itself
// remains immutable semantic authority; it never retains caller work state.
func (binding *Binding[K, V]) NewWork() (carrier.SlotWork, bool) {
	if binding == nil {
		return nil, false
	}
	binding.lifecycle.Lock()
	defer binding.lifecycle.Unlock()
	if !binding.live() || binding.plane == nil || binding.roots == nil {
		return nil, false
	}
	binding.scopeFrozen = true
	observations, ok := binding.issuer.NewObservationWork()
	if !ok {
		return nil, false
	}
	roots := newRootStore[planeFactor, K, V](binding.plane.domain)
	if roots == nil {
		return nil, false
	}
	supportWork := support.New(binding.plane.domain.Guards())
	if supportWork == nil {
		return nil, false
	}
	// Read-only support inclusion gets a distinct long-lived shell.  The
	// construction shell above is sealed by Three/Union transactions; sharing
	// it with publication comparisons would make a later read observe a
	// terminal Work and fall back to the allocating Manager observer.
	entailsWork := support.New(binding.plane.domain.Guards())
	if entailsWork == nil {
		supportWork.Close()
		return nil, false
	}
	// One-key reads refine their region in a transaction of their own, opened
	// and sealed inside a single read.  That is a third lifecycle, so it gets a
	// third shell: supportWork stays open across a fold and entailsWork is
	// never sealed, and a read may borrow neither.
	readWork := support.New(binding.plane.domain.Guards())
	if readWork == nil {
		supportWork.Close()
		entailsWork.Close()
		return nil, false
	}
	return &bindingWork[K, V]{binding: binding, roots: roots, supportWork: supportWork, entailsWork: entailsWork, readWork: readWork, observations: observations, readMemo: newObservationReadMemo[K, V](binding.declaredKeyCount()), readKeyOffsets: binding.declaredKeyOffsets()}, true
}

// bindingWork is private typed evaluator state for exactly one Binding.
// SoleScratch carries no semantic authority and is reset between sequential
// operations. The enclosing carrier.Work owns this object exclusively.
type bindingWork[K scalar.Key, V any] struct {
	binding *Binding[K, V]
	// supportWork is this Binding's reusable Boolean shell. It is distinct
	// from carrier.Work's open delta candidate so nested coverage/support
	// merges never reset a concurrently constructed typed delta.
	supportWork *support.Work
	// entailsWork is a read-only, never-sealed shell for exact support
	// inclusion. It must not share supportWork: helper Boolean transactions
	// seal that shell, while publication comparisons need an open Work for the
	// entire evaluator epoch.
	entailsWork *support.Work
	// readWork is the shell lent to one-key plane reads. A read publishes its
	// refinement cells in a transaction of its own, so it can share neither the
	// fold-long construction shell nor the never-sealed comparison shell.
	readWork *support.Work
	// roots and epoch are paired exactly once by carrier.Work opening.  The
	// former retains this Work's dynamic typed planes; the latter is the only
	// opaque handle provenance accepted for those planes.
	roots            *rootStore[planeFactor, K, V]
	epoch            *carrier.RootEpoch
	checkpoint       carrier.Checkpoint
	poll             func() bool
	observations     carrier.ObservationWork
	observationLive  bool
	generation       identity.Generation
	nextObservation  uint64
	firstObservation uint64
	// Preview roots are Work-local just like ordinary dynamic roots.  A
	// Binding-wide table would let concurrent Works collide on compact preview
	// IDs and would retain an aborted epoch's typed planes.
	previewRoots map[uint64]semantic.Plane[planeFactor, K, V]
	previewNext  uint64
	records      []observationRecord
	entries      []ObservationEntry[V]
	pieces       []observationPiece[V]
	partials     []observationGroup
	nextPartials []observationGroup
	spine        []observationCell[V]
	buckets      map[uint64]int
	// Exact observation traversal is a Work-local state machine.  Both the
	// callback adapter and DirectObservation advance this same state; neither
	// path owns a second partition or emission implementation.
	observationCursorMode  directObservationMode
	observationCursorIndex int
	observationCursorRoot  carrier.RootHandle
	observationCursorUnit  carrier.Unit
	// observationCursorKeys is the observed Unit's base coordinate in the
	// sealed declared-key inventory; a key read under this cursor is at that
	// base plus the key's position in the Unit's frozen vector.
	observationCursorKeys   int
	readKeyOffsets          []int
	observationCursorFailed bool
	// readMemo retains owner-local immutable exact partitions for this evaluator
	// lane. Its capacity is fixed from the Binding's sealed Unit inventory. A
	// warm read of the same root/unit/region/key copies its already sealed cells
	// into generation scratch instead of reopening Boolean construction; a
	// different identity deterministically evicts one slot and reruns
	// PartitionKey. Entries are cleared at root-epoch close; RootHandle is the
	// complete immutable published-plane identity, so no fact revision can
	// reuse an entry accidentally. This is bounded scratch storage, never a
	// Binding/global answer cache.
	readMemo observationReadMemo[K, V]
	// partitionVisit is the Work-owned adapter for PartitionKey. Keeping the
	// callback on the same owner avoids minting a capturing closure for every
	// product source region; it reads only the current pieces scratch.
	partitionVisit func(V, bool, support.Mask) bool
	// scratch and changes belong to the enclosing evaluator's merge/compare
	// lifecycle, not to one observation generation. Every sole operation
	// begins with SoleScratch.prepare (which Clear's it); Merge3Under clears
	// changes on entry and exit. Observation close therefore clears only rows
	// and typed projection buffers, never an unrelated in-flight evaluator op.
	scratch diagram.SoleScratch[K, V]
	changes mergeChanges[K]
	// expand is expandChanges' reusable heap/output-vector storage for this
	// Work's own merges, kept separate from a Patch's copy since a nested
	// evaluator operation and a staged write never share one epoch.
	expand expandScratch[K]
	// Coverage maps are evaluator scratch only. They resolve opaque authored
	// Targets beside this typed Binding for one fold and are cleared before
	// the fold returns; no coverage or fact authority is retained here.
	coverageLeft              map[K]support.Mask
	coverageRight             map[K]support.Mask
	coverageOutput            map[K]support.Mask
	changed                   map[K]support.Mask
	changeRows                []semantic.ContributionChange[K]
	pointFoldPlanes           []semantic.Plane[planeFactor, K, V]
	pointFoldRows             []pointFoldCoverageRegion
	pointFoldRuns             []pointFoldRun
	pointFoldMerge            []pointFoldCoverageRegion
	pointFoldHeap             []int32
	pointFoldEffects          map[carrier.LineageToken]support.Mask
	pointFoldBaselines        map[carrier.LineageToken]support.Mask
	pointFoldBaselineOperands map[carrier.LineageToken]int
	// pointFoldRetained is the per-position complement of the displacement a
	// routed write authored, folded once for the whole fold rather than per
	// observed key. Positions with no routed row are absent.
	pointFoldRetained []pointFoldRetainedRegion
}

// pointFoldRetainedRegion is one Target position's retained predecessor
// region: everything the routed writes at that position did not answer.
type pointFoldRetainedRegion struct {
	position int
	region   support.Mask
}

type pointFoldCoverageRegion struct {
	position int
	operand  int
	region   support.Mask
	lineage  carrier.LineageToken
	role     carrier.CoverageRole
}

// pointFoldRun is one operand's coverage rows inside pointFoldRows. Every run
// is ascending in descriptor position by admission: appendCoverage rejects a
// repeated or descending position instead of discovering the order afterwards.
type pointFoldRun struct {
	next, end int
}

// retainedAt answers one position's retained predecessor region. The second
// result reports whether any routed write authored a displacement there, so
// the ordinary position pays no conjunction at all.
func (work *bindingWork[K, V]) retainedAt(position int) (support.Mask, bool) {
	rows := work.pointFoldRetained
	index := sort.Search(len(rows), func(index int) bool { return rows[index].position >= position })
	if index >= len(rows) || rows[index].position != position {
		return support.Mask{}, false
	}
	return rows[index].region, true
}

// buildPointFoldRetained folds every authored displacement region into one
// retained complement per position. pointFoldRows is already ascending in
// position, so this is one linear pass over the same rows the key lookup
// binary-searches.
func (work *bindingWork[K, V]) buildPointFoldRetained(delta *support.Work) bool {
	work.pointFoldRetained = work.pointFoldRetained[:0]
	for index := 0; index < len(work.pointFoldRows); index++ {
		row := work.pointFoldRows[index]
		if row.role != carrier.CoverageDisplacement {
			continue
		}
		if count := len(work.pointFoldRetained); count != 0 && work.pointFoldRetained[count-1].position == row.position {
			region, valid := delta.Or(work.pointFoldRetained[count-1].region, row.region)
			if !valid {
				return false
			}
			work.pointFoldRetained[count-1].region = region
			continue
		}
		work.pointFoldRetained = append(work.pointFoldRetained, pointFoldRetainedRegion{position: row.position, region: row.region})
	}
	for index := range work.pointFoldRetained {
		region, valid := delta.Not(work.pointFoldRetained[index].region)
		if !valid {
			return false
		}
		work.pointFoldRetained[index].region = region
	}
	return true
}

// mergePointFoldRuns turns the concatenated per-operand runs into the single
// ascending row order the key lookup binary-searches. Runs are appended in
// operand order and each is already ascending, so breaking ties by run index
// reproduces the (position, operand) order exactly, in O(n log k) rather than
// re-sorting the concatenation.
func (work *bindingWork[K, V]) mergePointFoldRuns() {
	runs := work.pointFoldRuns
	if len(runs) < 2 {
		return
	}
	heap := work.pointFoldHeap[:0]
	for run := range runs {
		if runs[run].next < runs[run].end {
			heap = append(heap, int32(run))
		}
	}
	rows := work.pointFoldRows
	less := func(left, right int32) bool {
		leftPosition, rightPosition := rows[runs[left].next].position, rows[runs[right].next].position
		if leftPosition != rightPosition {
			return leftPosition < rightPosition
		}
		return left < right
	}
	for index := len(heap)/2 - 1; index >= 0; index-- {
		siftDownRuns(heap, index, less)
	}
	merged := work.pointFoldMerge[:0]
	for len(heap) != 0 {
		run := heap[0]
		merged = append(merged, rows[runs[run].next])
		runs[run].next++
		if runs[run].next == runs[run].end {
			last := len(heap) - 1
			heap[0], heap = heap[last], heap[:last]
		}
		if len(heap) != 0 {
			siftDownRuns(heap, 0, less)
		}
	}
	work.pointFoldHeap = heap[:0]
	clear(rows)
	work.pointFoldRows, work.pointFoldMerge = merged, rows[:0]
	work.pointFoldRuns = runs[:0]
}

func siftDownRuns(heap []int32, root int, less func(left, right int32) bool) {
	for {
		child := 2*root + 1
		if child >= len(heap) {
			return
		}
		if child+1 < len(heap) && less(heap[child+1], heap[child]) {
			child++
		}
		if !less(heap[child], heap[root]) {
			return
		}
		heap[root], heap[child] = heap[child], heap[root]
		root = child
	}
}

// BindRootEpoch is carrier's one evaluator-open lifecycle cut.  It registers
// only a compact-ID membership predicate; carrier never receives a Plane or
// payload.  A second Work over the same Binding gets a distinct store/token.
func (work *bindingWork[K, V]) BindRootEpoch(epoch *carrier.RootEpoch) bool {
	if work == nil || work.binding == nil || work.roots == nil || work.epoch != nil || epoch == nil || !work.binding.live() {
		return false
	}
	if !epoch.BindRootStore(work.binding.issuer, work.roots) {
		return false
	}
	work.epoch = epoch
	return true
}

// CloseRootEpoch drops all Work-owned typed roots and previews after carrier
// has revoked the shared epoch token.  Clearing the store reference is the
// reclamation cut: stale RootHandles retain only the tiny dead token, never
// this FDD/terminal arena.
func (work *bindingWork[K, V]) CloseRootEpoch() {
	if work == nil {
		return
	}
	work.closeObservation()
	clear(work.previewRoots)
	work.previewRoots = nil
	work.previewNext = 0
	work.roots = nil
	work.epoch = nil
	work.checkpoint = nil
	work.poll = nil
	work.scratch.Clear()
	work.changes.reset()
	work.changes.rows = nil
	clear(work.pointFoldPlanes)
	work.pointFoldPlanes = nil
	clear(work.pointFoldRows)
	work.pointFoldRows = nil
	clear(work.pointFoldMerge)
	work.pointFoldMerge = nil
	work.pointFoldRuns = nil
	work.pointFoldHeap = nil
	clear(work.pointFoldEffects)
	work.pointFoldEffects = nil
	clear(work.pointFoldBaselines)
	work.pointFoldBaselines = nil
	clear(work.pointFoldBaselineOperands)
	work.pointFoldBaselineOperands = nil
	clear(work.coverageLeft)
	work.coverageLeft = nil
	clear(work.coverageRight)
	work.coverageRight = nil
	clear(work.coverageOutput)
	work.coverageOutput = nil
	clear(work.changed)
	work.changed = nil
	clear(work.changeRows)
	work.changeRows = nil
	if work.supportWork != nil {
		work.supportWork.Close()
		work.supportWork = nil
	}
	if work.entailsWork != nil {
		work.entailsWork.Close()
		work.entailsWork = nil
	}
	if work.readWork != nil {
		work.readWork.Close()
		work.readWork = nil
	}
	work.clearObservationScratch()
	work.readMemo.clear()
	work.partitionVisit = nil
	work.binding = nil
}

// SetCheckpoint installs carrier's opaque epoch liveness probe on this typed
// traversal owner. Binding deliberately does not know why an epoch stopped;
// false merely abandons its local scratch/pending root before a ChangeHandle
// can cross the carrier publication cut.
func (work *bindingWork[K, V]) SetCheckpoint(checkpoint carrier.Checkpoint) bool {
	if work == nil || work.observationLive {
		return false
	}
	work.checkpoint = checkpoint
	work.poll = nil
	if checkpoint != nil {
		work.poll = func() bool { return checkpoint() }
	}
	work.scratch.SetCheckpoint(work.poll)
	return true
}

func (work *bindingWork[K, V]) live() bool {
	return work != nil && work.binding != nil && work.roots != nil && work.epoch != nil && work.epoch.Active() && (work.checkpoint == nil || work.checkpoint())
}

func (work *bindingWork[K, V]) newSupportWork() *support.Work {
	if !work.live() || work.binding == nil || work.binding.plane == nil {
		return nil
	}
	if work.supportWork == nil {
		work.supportWork = support.New(work.binding.plane.domain.Guards())
	}
	if work.supportWork == nil || !work.supportWork.BeginTransaction(work.poll) {
		return nil
	}
	return work.supportWork
}

func (work *bindingWork[K, V]) threeSupport(left, right support.Mask) (support.Split, bool) {
	if !work.live() || work.supportWork == nil {
		return support.Split{}, false
	}
	return support.ThreeWithWork(work.supportWork, work.poll, left, right)
}

// entailsSupport is the hot Boolean inclusion boundary for one typed Work.
// Its guard traversal uses the Work-owned read scratch rather than the
// Manager observer, whose per-call stack/map would allocate on every
// publication comparison.  The checkpoint remains authoritative for the
// whole traversal and the support Work still rejects foreign managers.
func (work *bindingWork[K, V]) entailsSupport(premise, conclusion support.Mask) bool {
	return work != nil && work.live() && work.entailsWork != nil && work.entailsWork.EntailsWithCheckpoint(work.poll, premise, conclusion)
}

// resolve is the sole typed root boundary for this evaluator.  Initial roots
// are immutable Binding-owned entries. A live dynamic root resolves through
// the exact epoch token that published it, so concurrent Works can read an
// immutable predecessor without sharing mutable scratch or colliding compact
// IDs. Preview roots remain strictly local to this Work.
func (work *bindingWork[K, V]) resolve(handle carrier.RootHandle) (semantic.Plane[planeFactor, K, V], bool) {
	if work == nil || work.binding == nil || work.binding.plane == nil || work.epoch == nil {
		return semantic.Plane[planeFactor, K, V]{}, false
	}
	binding := work.binding
	if id, ok := work.epoch.OwnsRoot(binding.issuer, handle); ok {
		if work.roots == nil {
			return semantic.Plane[planeFactor, K, V]{}, false
		}
		return work.roots.Plane(id)
	}
	if store, id, ok := binding.issuer.ResolveEpochRoot(handle); ok {
		roots, typed := store.(*rootStore[planeFactor, K, V])
		if !typed || roots == nil {
			return semantic.Plane[planeFactor, K, V]{}, false
		}
		return roots.Plane(id)
	}
	if id, ok := binding.issuer.ResolveRoot(handle); ok {
		if binding.roots == nil {
			return semantic.Plane[planeFactor, K, V]{}, false
		}
		return binding.roots.Plane(id)
	}
	preview, ok := work.epoch.ResolvePreviewRoot(binding.issuer, handle)
	if !ok {
		return semantic.Plane[planeFactor, K, V]{}, false
	}
	plane, present := work.previewRoots[preview]
	return plane, present && binding.plane.domain.Valid(plane)
}

// FoldPointRHSUnder performs one synchronized typed fold for a physical slot.
// The operand headers are borrowed for this call and copied only into the
// binding-owned scratch vectors; no intermediate semantic join is published.
func (work *bindingWork[K, V]) FoldPointRHSUnder(before, base carrier.RootHandle, baseSupport, finalSupport support.Mask, baseCoverage carrier.SlotCoverage, baseClosed, carryBaseOutside bool, terms []carrier.PointFoldTerm, delta *support.Work) (carrier.ChangeHandle, bool) {
	if !work.live() || work.binding == nil || work.binding.plane == nil || delta == nil || !delta.Open() || !baseSupport.Valid() || !finalSupport.Valid() || baseSupport.Manager() != work.binding.plane.domain.Guards() || finalSupport.Manager() != work.binding.plane.domain.Guards() || !work.entailsSupport(baseSupport, finalSupport) || carryBaseOutside != finalSupport.Equal(baseSupport) {
		return carrier.ChangeHandle{}, false
	}
	prior, ok := work.resolve(before)
	if !ok {
		return carrier.ChangeHandle{}, false
	}
	basePlane, ok := work.resolve(base)
	if !ok {
		return carrier.ChangeHandle{}, false
	}
	// A slot with no authored right operand is exact physical adoption when the
	// base is already globally closed, or when its outside-support branches are
	// deliberately retained. Do not open a terminal/FDD transaction merely to
	// rediscover that immutable root.
	if len(terms) == 0 && (baseClosed || carryBaseOutside) {
		if work.binding.plane.domain.Same(prior, basePlane) {
			return work.prepareChange(before, before, prior, false, support.Mask{}, nil, nil, delta)
		}
		return work.prepareChange(before, base, basePlane, false, support.Mask{}, nil, nil, delta)
	}
	count := len(terms) + 1
	if cap(work.pointFoldPlanes) < count {
		work.pointFoldPlanes = make([]semantic.Plane[planeFactor, K, V], count)
	} else {
		work.pointFoldPlanes = work.pointFoldPlanes[:count]
		clear(work.pointFoldPlanes)
	}
	defer func() {
		clear(work.pointFoldPlanes)
		clear(work.pointFoldRows)
		clear(work.pointFoldRetained)
		work.pointFoldPlanes = work.pointFoldPlanes[:0]
		work.pointFoldRows = work.pointFoldRows[:0]
		work.pointFoldRetained = work.pointFoldRetained[:0]
	}()
	clear(work.pointFoldRows)
	work.pointFoldRows = work.pointFoldRows[:0]
	work.pointFoldRuns = work.pointFoldRuns[:0]
	if work.pointFoldEffects == nil {
		work.pointFoldEffects = make(map[carrier.LineageToken]support.Mask)
	}
	if work.pointFoldBaselines == nil {
		work.pointFoldBaselines = make(map[carrier.LineageToken]support.Mask)
	}
	if work.pointFoldBaselineOperands == nil {
		work.pointFoldBaselineOperands = make(map[carrier.LineageToken]int)
	}
	appendCoverage := func(operand int, coverage carrier.SlotCoverage, within support.Mask) bool {
		start := len(work.pointFoldRows)
		prior := -1
		for rowIndex := 0; rowIndex < coverage.Count(); rowIndex++ {
			row, present := coverage.At(rowIndex)
			descriptor, declared := work.binding.targets[row.Target()]
			if !present || !declared || descriptor.position < prior || !row.Region().Valid() || row.Region().Manager() != baseSupport.Manager() || !work.entailsSupport(row.Region(), within) {
				return false
			}
			prior = descriptor.position
			work.pointFoldRows = append(work.pointFoldRows, pointFoldCoverageRegion{position: descriptor.position, operand: operand, region: row.Region(), lineage: row.Lineage(), role: row.Role()})
		}
		work.pointFoldRuns = append(work.pointFoldRuns, pointFoldRun{next: start, end: len(work.pointFoldRows)})
		return true
	}
	work.pointFoldPlanes[0] = basePlane
	if !appendCoverage(0, baseCoverage, baseSupport) {
		return carrier.ChangeHandle{}, false
	}
	for index, term := range terms {
		plane, valid := work.resolve(term.Root())
		if !valid || !term.Support().Valid() || term.Support().Manager() != baseSupport.Manager() || !work.entailsSupport(term.Support(), finalSupport) {
			return carrier.ChangeHandle{}, false
		}
		work.pointFoldPlanes[index+1] = plane
		if !appendCoverage(index+1, term.Coverage(), term.Support()) {
			return carrier.ChangeHandle{}, false
		}
	}
	work.mergePointFoldRuns()
	if !work.buildPointFoldRetained(delta) {
		return carrier.ChangeHandle{}, false
	}
	falseRegion := delta.False()
	if !falseRegion.Valid() {
		return carrier.ChangeHandle{}, false
	}
	baseOutside := falseRegion
	if carryBaseOutside {
		var valid bool
		baseOutside, valid = delta.Not(baseSupport)
		if !valid {
			return carrier.ChangeHandle{}, false
		}
	}
	next, ok := work.binding.plane.domain.JoinContributionsMany(prior, work.pointFoldPlanes, &work.scratch, delta, func(key K, output []support.Mask) bool {
		if len(output) != len(work.pointFoldPlanes) {
			return false
		}
		clear(work.pointFoldEffects)
		clear(work.pointFoldBaselines)
		clear(work.pointFoldBaselineOperands)
		for operand := range output {
			output[operand] = falseRegion
		}
		if carryBaseOutside {
			output[0] = baseOutside
		}
		for _, position := range work.binding.targetReverse[key] {
			clear(work.pointFoldEffects)
			clear(work.pointFoldBaselines)
			clear(work.pointFoldBaselineOperands)
			rowIndex := sort.Search(len(work.pointFoldRows), func(index int) bool { return work.pointFoldRows[index].position >= position })
			end := rowIndex
			// A routed write states the complete value of the coordinate it
			// read, so its region is the part of this cell the operation has
			// already answered. retainedAt is the complement of that region,
			// folded once for the whole position: every transported
			// predecessor keeps exactly what the routed writes did not
			// answer, which is the before/after program-point distinction
			// stated as one region conjunction.
			retained, displaced := work.retainedAt(position)
			for end < len(work.pointFoldRows) && work.pointFoldRows[end].position == position {
				row := work.pointFoldRows[end]
				if row.lineage.Valid() {
					var target map[carrier.LineageToken]support.Mask
					if row.role == carrier.CoverageBaseline {
						target = work.pointFoldBaselines
						if _, present := work.pointFoldBaselineOperands[row.lineage]; !present {
							work.pointFoldBaselineOperands[row.lineage] = row.operand
						}
					} else {
						target = work.pointFoldEffects
					}
					priorRegion, present := target[row.lineage]
					if !present {
						target[row.lineage] = row.region
					} else {
						region, valid := delta.Or(priorRegion, row.region)
						if !valid {
							return false
						}
						target[row.lineage] = region
					}
				}
				end++
			}
			for lineage, region := range work.pointFoldBaselines {
				operand := work.pointFoldBaselineOperands[lineage]
				if effect, present := work.pointFoldEffects[lineage]; present {
					complement, valid := delta.Not(effect)
					if !valid {
						return false
					}
					region, valid = delta.And(region, complement)
					if !valid {
						return false
					}
				}
				if displaced {
					var valid bool
					region, valid = delta.And(region, retained)
					if !valid {
						return false
					}
				}
				if support.Empty(region) {
					continue
				}
				merged, valid := delta.Or(output[operand], region)
				if !valid {
					return false
				}
				output[operand] = merged
			}
			for index := rowIndex; index < end; index++ {
				row := work.pointFoldRows[index]
				if row.role == carrier.CoverageBaseline && row.lineage.Valid() {
					continue
				}
				region, valid := delta.Or(output[row.operand], row.region)
				if !valid {
					return false
				}
				output[row.operand] = region
			}
		}
		return true
	})
	if !ok {
		return carrier.ChangeHandle{}, false
	}
	guards := work.binding.plane.domain.Guards()
	whole, wholeValid := support.FromGuard(guards, guards.True())
	if !wholeValid {
		return carrier.ChangeHandle{}, false
	}
	work.scratch.Clear()
	if work.binding.plane.domain.EqualUnder(prior, next, whole, &work.scratch) {
		return work.prepareChange(before, before, next, false, support.Mask{}, nil, nil, delta)
	}
	work.scratch.Clear()
	if work.binding.plane.domain.EqualUnder(basePlane, next, whole, &work.scratch) {
		return work.prepareChange(before, base, next, false, support.Mask{}, nil, nil, delta)
	}
	return work.prepareChange(before, carrier.RootHandle{}, next, true, support.Mask{}, nil, nil, delta)
}

func (work *bindingWork[K, V]) issuePreview(plane semantic.Plane[planeFactor, K, V]) (carrier.RootHandle, bool) {
	if !work.live() || work.binding == nil || work.binding.plane == nil || !work.binding.plane.domain.Valid(plane) || work.previewNext >= previewRootLimit {
		return carrier.RootHandle{}, false
	}
	work.previewNext++
	handle, ok := work.epoch.IssuePreviewRoot(work.binding.issuer, work.previewNext)
	if !ok {
		work.previewNext--
		return carrier.RootHandle{}, false
	}
	if work.previewRoots == nil {
		work.previewRoots = make(map[uint64]semantic.Plane[planeFactor, K, V])
	}
	work.previewRoots[work.previewNext] = plane
	return handle, true
}

func (work *bindingWork[K, V]) dropPreview(handle carrier.RootHandle) {
	if work == nil || work.binding == nil || work.epoch == nil {
		return
	}
	id, ok := work.epoch.ResolvePreviewRoot(work.binding.issuer, handle)
	if !ok {
		return
	}
	delete(work.previewRoots, id)
}

// prepareChange is the only typed construction of a carrier change proof for
// this evaluator.  A new root is always owned by this bindingWork's dynamic
// store; carrying an old root first proves it is either static or belongs to
// this exact Work epoch.
func (work *bindingWork[K, V]) prepareChange(before, after carrier.RootHandle, next semantic.Plane[planeFactor, K, V], newRoot bool, factor support.Mask, units []carrier.Unit, regions []support.Mask, candidate *support.Work) (carrier.ChangeHandle, bool) {
	if !work.live() || work.binding == nil || work.binding.plane == nil || !work.binding.plane.domain.Valid(next) {
		return carrier.ChangeHandle{}, false
	}
	if _, ok := work.resolve(before); !ok {
		return carrier.ChangeHandle{}, false
	}
	if !newRoot {
		if _, ok := work.resolve(after); !ok {
			return carrier.ChangeHandle{}, false
		}
	}
	var publisher carrier.RootPublisher
	if newRoot {
		after = carrier.RootHandle{}
		publisher = &pendingRoot[K, V]{work: work, plane: next}
	}
	change, ok := work.binding.issuer.IssueChange(before, after, publisher, factor, units, regions, candidate)
	if !ok && publisher != nil {
		publisher.Drop()
	}
	return change, ok
}

// observationRecord is Binding-owned typed semantic data.  The carrier sees
// only the companion ObservationHandle and support row; K and V stay here.
// root and unit are retained to make every later row resolution re-prove the
// exact source authority instead of trusting an opaque ID alone.
type observationRecord struct {
	root   carrier.RootHandle
	unit   carrier.Unit
	region support.Mask
	first  int
	count  int
}

// ObservationEntry is one stored-or-absent typed key observation. An absent
// entry carries the Factor Default as Value and false as Present; that bit is
// deliberately preserved so a stored Default can never collapse into sparse
// absence. Summary observations retain these entries in declared key order.
type ObservationEntry[V any] struct {
	value   V
	present bool
}

// Read returns this entry's typed semantic value together with its stored
// token. The pair is indivisible at the observation boundary: callers never
// resolve an observed value without also receiving whether it was present.
func (entry ObservationEntry[V]) Read() (V, bool) { return entry.value, entry.present }

// Observation is a generation-bound typed view over one Binding-owned entry
// sequence. Its accessors reject an escaped row after the callback generation
// closes, even if the SlotWork later reuses its flat scratch storage.
type Observation[V any] struct {
	resolver   observationResolver[V]
	generation identity.Generation
	id         uint64
}

type observationResolver[V any] interface {
	observationCount(generation identity.Generation, id uint64) (int, bool)
	observationEntry(generation identity.Generation, id uint64, index int) (ObservationEntry[V], bool)
}

// Valid reports whether this observation still belongs to its live callback
// generation.
func (observation Observation[V]) Valid() bool {
	if observation.resolver == nil {
		return false
	}
	_, ok := observation.resolver.observationCount(observation.generation, observation.id)
	return ok
}

// Count returns the frozen declared entry count while the callback remains
// live; stale observations report zero.
func (observation Observation[V]) Count() int {
	if observation.resolver == nil {
		return 0
	}
	count, ok := observation.resolver.observationCount(observation.generation, observation.id)
	if !ok {
		return 0
	}
	return count
}

// At returns one typed stored-or-absent entry in frozen declaration order.
func (observation Observation[V]) At(index int) (ObservationEntry[V], bool) {
	if observation.resolver == nil {
		return ObservationEntry[V]{}, false
	}
	return observation.resolver.observationEntry(observation.generation, observation.id, index)
}

type observationPiece[V any] struct {
	entry       ObservationEntry[V]
	fingerprint uint64
	region      support.Mask
}

// observationReadMemo holds the partition pieces of the declared keys it has
// read. An entry's coordinate is that key's position in the Binding's sealed
// key inventory - the Unit's offset plus the key's position in the Unit's
// frozen vector - so the memo is addressed rather than searched, and its size
// is the declared inventory rather than a function of how many reads were
// offered. Root and region remain stamped on the entry, because pieces read
// under one root or region are not the pieces read under another.
type observationReadMemo[K scalar.Key, V any] struct {
	entries []observationReadEntry[K, V]
}

type observationReadEntry[K scalar.Key, V any] struct {
	root   carrier.RootHandle
	key    K
	within support.Mask
	pieces []observationPiece[V]
	valid  bool
}

// newObservationReadMemo sizes the owner-local exact-read table from the
// Binding's sealed declared-key inventory. Every declared key of every Unit
// has its own coordinate, so no read ever displaces another key's entry.
func newObservationReadMemo[K scalar.Key, V any](keyCount int) observationReadMemo[K, V] {
	if keyCount <= 0 {
		return observationReadMemo[K, V]{}
	}
	return observationReadMemo[K, V]{entries: make([]observationReadEntry[K, V], keyCount)}
}

func (memo *observationReadMemo[K, V]) lookup(coordinate int, root carrier.RootHandle, key K, within support.Mask) ([]observationPiece[V], bool) {
	if memo == nil || coordinate < 0 || coordinate >= len(memo.entries) {
		return nil, false
	}
	dbgFactBinding.ReadMemoProbes++
	entry := &memo.entries[coordinate]
	if !entry.valid || entry.root != root || entry.key != key || entry.within != within {
		return nil, false
	}
	return entry.pieces, true
}

func (memo *observationReadMemo[K, V]) replace(coordinate int, root carrier.RootHandle, key K, within support.Mask, pieces []observationPiece[V]) {
	if memo == nil || coordinate < 0 || coordinate >= len(memo.entries) {
		return
	}
	entry := &memo.entries[coordinate]
	clear(entry.pieces)
	entry.root, entry.key, entry.within, entry.valid = root, key, within, true
	entry.pieces = append(entry.pieces[:0], pieces...)
}

func (memo *observationReadMemo[K, V]) clear() {
	if memo == nil {
		return
	}
	for index := range memo.entries {
		clear(memo.entries[index].pieces)
		memo.entries[index].pieces = memo.entries[index].pieces[:0]
		memo.entries[index].valid = false
		memo.entries[index].root = carrier.RootHandle{}
		var zero K
		memo.entries[index].key = zero
		memo.entries[index].within = support.Mask{}
	}
}

// observationCell is one entry of a discovered sequence linked to the prefix
// group it extends. Every group owns exactly one cell and shares its whole
// prefix with the group it was built from, so grouping storage is one cell per
// discovered group rather than one per group and declared key.
type observationCell[V any] struct {
	entry  ObservationEntry[V]
	parent int
	depth  int
}

// observationGroup names one discovered sequence by its terminal spine cell.
// count is that cell's depth, which is the declared entry count the emitted
// row carries.
type observationGroup struct {
	cell        int
	count       int
	fingerprint uint64
	region      support.Mask
	previous    int
}

// mergeChanges is the zipper-produced sparse key delta.  It is evaluator
// scratch, not a second fact representation: rows name only keys whose value
// changed from the left predecessor under the merge output support.
type mergeChanges[K scalar.Key] struct{ rows []mergeChange[K] }

type mergeChange[K scalar.Key] struct {
	key    K
	region support.Mask
}

func (changes *mergeChanges[K]) reset() {
	if changes == nil {
		return
	}
	clear(changes.rows)
	changes.rows = changes.rows[:0]
}
func (changes *mergeChanges[K]) Count() int { return len(changes.rows) }
func (changes *mergeChanges[K]) At(index int) (K, support.Mask, bool) {
	if index < 0 || index >= len(changes.rows) {
		var zero K
		return zero, support.Mask{}, false
	}
	row := changes.rows[index]
	return row.key, row.region, true
}

func (work *bindingWork[K, V]) EqualUnder(left, right carrier.RootHandle, within support.Mask) bool {
	if !work.live() {
		return false
	}
	binding := work.binding
	if binding == nil {
		return false
	}
	first, ok := work.resolve(left)
	if !ok {
		return false
	}
	second, ok := work.resolve(right)
	return ok && binding.plane.domain.EqualUnder(first, second, within, &work.scratch)
}

func (work *bindingWork[K, V]) LessOrEqUnder(left, right carrier.RootHandle, within support.Mask) bool {
	if !work.live() {
		return false
	}
	binding := work.binding
	if binding == nil {
		return false
	}
	first, ok := work.resolve(left)
	if !ok {
		return false
	}
	second, ok := work.resolve(right)
	return ok && binding.plane.domain.LessOrEqUnder(first, second, within, &work.scratch)
}

// LessOrEqContributionUnder proves the lifted partial order for one closed
// typed slot.  Coverage is the entire presence authority; the sparse root is
// consulted only at left-present cells, where undefined means Default.
func (work *bindingWork[K, V]) LessOrEqContributionUnder(left, right carrier.RootHandle, leftSupport, rightSupport support.Mask, leftCoverage, rightCoverage carrier.SlotCoverage) (bool, bool) {
	if !work.live() || work.binding == nil || !leftSupport.Valid() || !rightSupport.Valid() || leftSupport.Manager() != work.binding.plane.domain.Guards() || rightSupport.Manager() != work.binding.plane.domain.Guards() || !work.entailsSupport(leftSupport, rightSupport) {
		return false, false
	}
	first, ok := work.resolve(left)
	if !ok {
		return false, false
	}
	second, ok := work.resolve(right)
	if !ok {
		return false, false
	}
	defer func() {
		clear(work.coverageLeft)
		clear(work.coverageRight)
	}()
	work.coverageLeft, ok = work.contributionCoverage(leftCoverage, leftSupport, work.coverageLeft)
	if !ok {
		return false, false
	}
	work.coverageRight, ok = work.contributionCoverage(rightCoverage, rightSupport, work.coverageRight)
	if !ok || !coverageMapContains(work.entailsWork, work.poll, work.coverageLeft, work.coverageRight) {
		return false, false
	}
	return work.binding.plane.domain.LessOrEqContribution(first, second, func(key K) (support.Mask, bool) {
		return contributionRegion(work.coverageLeft, key)
	}, &work.scratch), true
}

// AscentOrderedContributionUnder proves Kleene progress for one closed typed
// slot. It deliberately does not carry LessOrEqContributionUnder's coverage
// inclusion premise. Inclusion is what makes the successor contain the
// predecessor; progress asks only for a defined upper bound, and a coverage
// cell one side does not author reads as that side's Default under the union
// surface, where the domain's own Widen has to dominate both cells before the
// replacement is admitted. Keeping the stronger premise here refused every
// authored-coverage movement as an unanswerable operand instead of judging it.
//
// Support remains the outer feasibility fence: coverage is authorship inside a
// contribution, while a successor whose support does not contain the
// predecessor's is not a step of the same chain at all.
func (work *bindingWork[K, V]) AscentOrderedContributionUnder(left, right carrier.RootHandle, leftSupport, rightSupport support.Mask, leftCoverage, rightCoverage carrier.SlotCoverage) (bool, bool) {
	if !work.live() || work.binding == nil || !leftSupport.Valid() || !rightSupport.Valid() || leftSupport.Manager() != work.binding.plane.domain.Guards() || rightSupport.Manager() != work.binding.plane.domain.Guards() || !work.entailsSupport(leftSupport, rightSupport) {
		return false, false
	}
	first, ok := work.resolve(left)
	if !ok {
		return false, false
	}
	second, ok := work.resolve(right)
	if !ok {
		return false, false
	}
	defer func() {
		clear(work.coverageLeft)
		clear(work.coverageRight)
		clear(work.coverageOutput)
	}()
	work.coverageLeft, ok = work.contributionCoverage(leftCoverage, leftSupport, work.coverageLeft)
	if !ok {
		return false, false
	}
	work.coverageRight, ok = work.contributionCoverage(rightCoverage, rightSupport, work.coverageRight)
	if !ok {
		return false, false
	}
	work.coverageOutput, ok = unionContributionCoverage(work.coverageLeft, work.coverageRight, work.coverageOutput)
	if !ok {
		return false, false
	}
	return work.binding.plane.domain.AscentOrderedContribution(first, second, func(key K) (support.Mask, bool) {
		return contributionRegion(work.coverageOutput, key)
	}, &work.scratch), true
}

func coverageMapContains[K scalar.Key](work *support.Work, checkpoint func() bool, left, right map[K]support.Mask) bool {
	for key, region := range left {
		other, ok := right[key]
		if !ok || work == nil || !work.EntailsWithCheckpoint(checkpoint, region, other) {
			return false
		}
	}
	return true
}

// unionContributionCoverage is evaluator scratch, not a second presence
// representation.  It computes the extensional key x guard union of two
// carrier-owned Target relations immediately before one typed operation.
func unionContributionCoverage[K scalar.Key](left, right, result map[K]support.Mask) (map[K]support.Mask, bool) {
	if result == nil {
		result = make(map[K]support.Mask, len(left)+len(right))
	}
	clear(result)
	for key, region := range left {
		result[key] = region
	}
	for key, region := range right {
		if prior, present := result[key]; present {
			merged, ok := support.Union(prior, region)
			if !ok {
				return nil, false
			}
			result[key] = merged
		} else {
			result[key] = region
		}
	}
	return result, true
}

func contributionRegion[K scalar.Key](coverage map[K]support.Mask, key K) (support.Mask, bool) {
	region, present := coverage[key]
	return region, present
}

// contributionRegionAt resolves one sparse typed key against carrier-owned
// Target rows without expanding the complete Target×key incidence relation.
// targetReverse is immutable cold Binding geometry; coverage remains the sole
// dynamic authorship authority. The lookup cost is proportional to this key's
// declared target degree, not to every key of every authored Target.
func (work *bindingWork[K, V]) contributionRegionAt(coverage carrier.SlotCoverage, within support.Mask, key K) (support.Mask, bool, bool) {
	if !work.live() || work.binding == nil || work.binding.plane == nil || !within.Valid() || within.Manager() != work.binding.plane.domain.Guards() {
		return support.Mask{}, false, false
	}
	incidence := work.binding.targetReverse[key]
	if coverage.Count() == 0 || len(incidence) == 0 {
		return support.Mask{}, false, true
	}
	var result support.Mask
	present := false
	for _, position := range incidence {
		if position < 0 || position >= len(work.binding.targetList) {
			return support.Mask{}, false, false
		}
		target := work.binding.targetList[position]
		targetDescriptor, known := work.binding.targets[target]
		if !known || targetDescriptor.position != position {
			return support.Mask{}, false, false
		}
		low, high := 0, coverage.Count()
		for low < high {
			middle := low + (high-low)/2
			row, ok := coverage.At(middle)
			rowDescriptor, known := work.binding.targets[row.Target()]
			if !ok || !known {
				return support.Mask{}, false, false
			}
			if targetOrderLess(rowDescriptor.order, targetDescriptor.order) {
				low = middle + 1
			} else {
				high = middle
			}
		}
		row, ok := coverage.At(low)
		if !ok || !row.Target().Same(target) {
			continue
		}
		region := row.Region()
		if !region.Valid() || region.Manager() != work.binding.plane.domain.Guards() {
			return support.Mask{}, false, false
		}
		if !present {
			result, present = region, true
			continue
		}
		result, ok = support.Union(result, region)
		if !ok {
			return support.Mask{}, false, false
		}
	}
	return result, present, true
}

func (work *bindingWork[K, V]) contributionCoverage(coverage carrier.SlotCoverage, within support.Mask, result map[K]support.Mask) (map[K]support.Mask, bool) {
	if !work.live() || work.binding == nil || !within.Valid() || within.Manager() != work.binding.plane.domain.Guards() {
		return result, false
	}
	clear(result)
	if coverage.Count() == 0 {
		return result, true
	}
	if result == nil {
		result = make(map[K]support.Mask)
	}
	for index := 0; index < coverage.Count(); index++ {
		row, ok := coverage.At(index)
		descriptor, known := work.binding.targets[row.Target()]
		if !ok || !known || !work.binding.ValidTarget(row.Target()) || len(descriptor.keys) == 0 || !row.Region().Valid() || support.Empty(row.Region()) || !work.entailsSupport(row.Region(), within) {
			return nil, false
		}
		for _, key := range descriptor.keys {
			region := row.Region()
			if previous, present := result[key]; present {
				region, ok = support.Union(previous, region)
				if !ok {
					return nil, false
				}
			}
			result[key] = region
		}
	}
	return result, true
}

// CloseContributionUnder is the typed half of every RuleContribution
// issuance. It physically drops every root fiber outside the canonical closed
// surface under split.Right() before preparing the one final root publication.
// A candidate may still carry a stale cell for a target excluded from that
// surface; CloseContribution is the typed operation that removes such latent
// payload, so its moved region is not itself a rejection condition.
// ReplaceUnder consumes the same carrier-owned split, so typed delta rows are
// confined to its overlap and support-only growth remains carrier Added
// evidence. A preview candidate is intentionally never reused: its publisher
// owns the temporary root and the closed plane receives a fresh normal pending
// publisher.
func (work *bindingWork[K, V]) CloseContributionUnder(before, input carrier.RootHandle, split support.Split, coverage carrier.SlotCoverage, delta *support.Work) (carrier.ChangeHandle, bool) {
	if !work.live() || work.binding == nil || work.binding.plane == nil || delta == nil || !delta.Open() || !split.Valid(work.binding.plane.domain.Guards()) {
		return carrier.ChangeHandle{}, false
	}
	within := split.Right()
	prior, ok := work.resolve(before)
	if !ok {
		return carrier.ChangeHandle{}, false
	}
	candidate, ok := work.resolve(input)
	if !ok {
		return carrier.ChangeHandle{}, false
	}
	coverageValid := true
	closed, _, ok := work.binding.plane.domain.CloseContribution(candidate, within, func(key K) (support.Mask, bool) {
		region, present, valid := work.contributionRegionAt(coverage, within, key)
		coverageValid = coverageValid && valid
		return region, present && valid
	}, &work.scratch, delta)
	// The close's moved region is intentionally not required to be empty: a
	// carried predecessor may contain latent payload for a target omitted by
	// canonical coverage, while a typed Patch authors the replacement surface.
	// The close itself is the one traversal that removes that payload.
	if !ok || !coverageValid {
		return carrier.ChangeHandle{}, false
	}
	// When the predecessor and candidate name the same immutable root, the
	// equality proof above also proves that every cell in the replacement
	// overlap is unchanged.  ReplaceUnder would only rediscover that empty
	// typed delta while traversing the old and closed FDDs.  Keep the physical
	// close (it may still erase latent payload outside the canonical surface),
	// but publish no Factor/unit evidence.  A preview candidate must still
	// become a normal pending root, even when the close is semantically
	// whole-plane equal.
	if before == input {
		_, preview := work.epoch.ResolvePreviewRoot(work.binding.issuer, input)
		if !preview && work.binding.plane.domain.Same(candidate, closed) {
			return work.prepareChange(before, input, closed, false, support.Mask{}, nil, nil, delta)
		}
		return work.prepareChange(before, carrier.RootHandle{}, closed, true, support.Mask{}, nil, nil, delta)
	}
	// The close may erase stale payload outside the canonical authored surface,
	// including a carried target omitted by a target-scoped exclusion. Its
	// published delta is nevertheless derived from the actual predecessor to
	// the closed final root, rather than inherited from a staged patch whose
	// publisher named the unclosed preview candidate.
	work.changes.reset()
	defer work.changes.reset()
	report := func(key K, region support.Mask) bool {
		if len(work.changes.rows) != 0 && work.changes.rows[len(work.changes.rows)-1].key >= key {
			return false
		}
		work.changes.rows = append(work.changes.rows, mergeChange[K]{key: key, region: region})
		return true
	}
	if !work.binding.plane.domain.ReplaceUnder(prior, closed, split, &work.scratch, delta, report) {
		return carrier.ChangeHandle{}, false
	}
	factor, units, regions, ok := work.binding.expandChanges(&work.changes, delta, &work.expand)
	if !ok {
		return carrier.ChangeHandle{}, false
	}
	// A recurrence head re-closes its stable exact RHS against the root it
	// published last pass.  Whenever that close erases latent payload the
	// candidate cut below cannot apply, yet the closed plane is the plane the
	// predecessor already publishes.  Retaining that root is what makes the
	// converged head observe no structural change; minting an equal-valued root
	// would re-dirty the head every pass.  The reused root must be a normal
	// published one, exactly as on the candidate side.
	_, priorPreview := work.epoch.ResolvePreviewRoot(work.binding.issuer, before)
	if !priorPreview && work.binding.plane.domain.Same(prior, closed) {
		if work.changes.Count() != 0 {
			return carrier.ChangeHandle{}, false
		}
		return work.prepareChange(before, before, closed, false, support.Mask{}, nil, nil, delta)
	}
	// Initial/non-carried roots often already equal the final closed plane.
	// Reuse only a normal published root; a preview root must be converted into
	// this transaction's own pending publisher even if its plane is unchanged.
	_, preview := work.epoch.ResolvePreviewRoot(work.binding.issuer, input)
	if !preview && work.binding.plane.domain.Same(candidate, closed) {
		return work.prepareChange(before, input, closed, false, factor, units, regions, delta)
	}
	return work.prepareChange(before, carrier.RootHandle{}, closed, true, factor, units, regions, delta)
}

func (work *bindingWork[K, V]) ContributionClosedUnder(root carrier.RootHandle, within support.Mask, coverage carrier.SlotCoverage) bool {
	if !work.live() || work.binding == nil || work.binding.plane == nil || !within.Valid() || within.Manager() != work.binding.plane.domain.Guards() {
		return false
	}
	input, ok := work.resolve(root)
	if !ok {
		return false
	}
	defer func() { clear(work.coverageOutput) }()
	work.coverageOutput, ok = work.contributionCoverage(coverage, within, work.coverageOutput)
	if !ok {
		return false
	}
	return work.binding.plane.domain.ContributionClosed(input, within, func(key K) (support.Mask, bool) {
		return contributionRegion(work.coverageOutput, key)
	})
}

// ContributionPresenceIncludedUnder compares only the lifted authored
// presence surface. Target aliases are expanded beside the Binding, so this
// is extensional in concrete key x guard cells rather than syntactic target
// row inclusion. It deliberately does not inspect roots or values: Widen may
// raise a value, but it must not turn an already Present cell into Absent.
func (work *bindingWork[K, V]) ContributionPresenceIncludedUnder(leftSupport, rightSupport support.Mask, leftCoverage, rightCoverage carrier.SlotCoverage) bool {
	if !work.live() || work.binding == nil || work.binding.plane == nil || !leftSupport.Valid() || !rightSupport.Valid() || leftSupport.Manager() != work.binding.plane.domain.Guards() || rightSupport.Manager() != work.binding.plane.domain.Guards() || !work.entailsSupport(leftSupport, rightSupport) {
		return false
	}
	defer func() {
		clear(work.coverageLeft)
		clear(work.coverageRight)
	}()
	var ok bool
	work.coverageLeft, ok = work.contributionCoverage(leftCoverage, leftSupport, work.coverageLeft)
	if !ok {
		return false
	}
	work.coverageRight, ok = work.contributionCoverage(rightCoverage, rightSupport, work.coverageRight)
	return ok && coverageMapContains(work.entailsWork, work.poll, work.coverageLeft, work.coverageRight)
}

// MergeContributionUnder resolves opaque target coverage only beside this
// typed Binding. For each concrete key, the effective left support is
// (left-state minus right authorship) union left authorship: right-only cells
// install the right terminal, covered overlap invokes Join, and uncovered
// right cells are fold identity. Sparse zero therefore means Default exactly
// when coverage says the producer authored that key/Guard.
func (work *bindingWork[K, V]) MergeContributionUnder(left, right carrier.RootHandle, leftSupport, rightSupport support.Mask, leftCoverage, rightCoverage carrier.SlotCoverage, delta *support.Work) (carrier.ChangeHandle, bool) {
	if !work.live() || work.binding == nil || work.binding.plane == nil || delta == nil || !delta.Open() || !leftSupport.Valid() || !rightSupport.Valid() || leftSupport.Manager() != work.binding.plane.domain.Guards() || rightSupport.Manager() != work.binding.plane.domain.Guards() {
		return carrier.ChangeHandle{}, false
	}
	first, ok := work.resolve(left)
	if !ok {
		return carrier.ChangeHandle{}, false
	}
	second, ok := work.resolve(right)
	if !ok {
		return carrier.ChangeHandle{}, false
	}
	if work.changed == nil {
		work.changed = make(map[K]support.Mask)
	}
	clear(work.changed)
	work.changeRows = work.changeRows[:0]
	defer func() {
		clear(work.changed)
		clear(work.changeRows)
		work.changeRows = work.changeRows[:0]
	}()
	for index := 0; index < rightCoverage.Count(); index++ {
		row, valid := rightCoverage.At(index)
		descriptor, owned := work.binding.targets[row.Target()]
		if !valid || !owned || !row.Region().Valid() || row.Region().Manager() != work.binding.plane.domain.Guards() || support.Empty(row.Region()) || !work.entailsSupport(row.Region(), rightSupport) {
			return carrier.ChangeHandle{}, false
		}
		for _, key := range descriptor.keys {
			region := row.Region()
			if prior, present := work.changed[key]; present {
				region, ok = support.Union(prior, region)
				if !ok {
					return carrier.ChangeHandle{}, false
				}
			}
			work.changed[key] = region
		}
	}
	if len(work.changed) == 0 {
		return work.prepareChange(left, left, first, false, support.Mask{}, nil, nil, delta)
	}
	if cap(work.changeRows) < len(work.changed) {
		work.changeRows = make([]semantic.ContributionChange[K], 0, len(work.changed))
	}
	for key, region := range work.changed {
		work.changeRows = append(work.changeRows, semantic.ContributionChange[K]{Key: key, Region: region})
	}
	sort.Slice(work.changeRows, func(left, right int) bool { return work.changeRows[left].Key < work.changeRows[right].Key })
	work.changes.reset()
	defer work.changes.reset()
	report := func(key K, region support.Mask) bool {
		if len(work.changes.rows) != 0 && work.changes.rows[len(work.changes.rows)-1].key >= key {
			return false
		}
		work.changes.rows = append(work.changes.rows, mergeChange[K]{key: key, region: region})
		return true
	}
	empty, ok := support.FromGuard(work.binding.plane.domain.Guards(), work.binding.plane.domain.Guards().False())
	if !ok {
		return carrier.ChangeHandle{}, false
	}
	next, ok := work.binding.plane.domain.JoinContributionChanges(first, second, work.changeRows, &work.scratch, delta, report, func(key K) (support.Mask, support.Mask, support.Mask, bool) {
		leftRegion, leftPresent, leftValid := work.contributionRegionAt(leftCoverage, leftSupport, key)
		if !leftValid {
			return support.Mask{}, support.Mask{}, support.Mask{}, false
		}
		if !leftPresent {
			leftRegion = empty
		}
		rightRegion, rightPresent := work.changed[key]
		if !rightPresent {
			return support.Mask{}, support.Mask{}, support.Mask{}, false
		}
		return leftRegion, rightRegion, leftRegion, true
	})
	if !ok {
		return carrier.ChangeHandle{}, false
	}
	factor, units, regions, ok := work.binding.expandChanges(&work.changes, delta, &work.expand)
	if !ok {
		return carrier.ChangeHandle{}, false
	}
	if work.binding.plane.domain.Same(first, next) {
		if work.changes.Count() != 0 {
			return carrier.ChangeHandle{}, false
		}
		return work.prepareChange(left, left, next, false, support.Mask{}, nil, nil, delta)
	}
	if work.binding.plane.domain.Same(second, next) {
		return work.prepareChange(left, right, next, false, factor, units, regions, delta)
	}
	return work.prepareChange(left, carrier.RootHandle{}, next, true, factor, units, regions, delta)
}

// OverlayPointRHSUnder is the directional PointRHS operation. Both operands
// keep the lifted-partial authored law: absent left coverage is Absent, not
// this Factor's Default. Its physical left region is nevertheless extended
// outside leftSupport so a sparse overlay retains latent point-root fibers
// byte-for-byte there. Inside leftSupport, only leftCoverage participates.
// Only closed right coverage changes an authored cell.
//
// The carrier admits this operation only when rightSupport is contained by
// leftSupport.  A support-growing rule must use the explicit total PointRHS
// semantic join instead; retaining an old root over that growth would make a
// latent out-of-support branch observable.
func (work *bindingWork[K, V]) OverlayPointRHSUnder(left, right carrier.RootHandle, leftSupport, rightSupport support.Mask, leftCoverage, rightCoverage carrier.SlotCoverage, delta *support.Work) (carrier.ChangeHandle, bool) {
	if !work.live() || work.binding == nil || work.binding.plane == nil || delta == nil || !delta.Open() || !leftSupport.Valid() || !rightSupport.Valid() || leftSupport.Manager() != work.binding.plane.domain.Guards() || rightSupport.Manager() != work.binding.plane.domain.Guards() || !work.entailsSupport(rightSupport, leftSupport) {
		return carrier.ChangeHandle{}, false
	}
	first, ok := work.resolve(left)
	if !ok {
		return carrier.ChangeHandle{}, false
	}
	second, ok := work.resolve(right)
	if !ok {
		return carrier.ChangeHandle{}, false
	}
	if work.changed == nil {
		work.changed = make(map[K]support.Mask)
	}
	clear(work.changed)
	work.changeRows = work.changeRows[:0]
	defer func() {
		clear(work.changed)
		clear(work.changeRows)
		work.changeRows = work.changeRows[:0]
	}()
	for index := 0; index < rightCoverage.Count(); index++ {
		row, valid := rightCoverage.At(index)
		descriptor, owned := work.binding.targets[row.Target()]
		if !valid || !owned || !row.Region().Valid() || row.Region().Manager() != work.binding.plane.domain.Guards() || support.Empty(row.Region()) || !work.entailsSupport(row.Region(), rightSupport) {
			return carrier.ChangeHandle{}, false
		}
		for _, key := range descriptor.keys {
			region := row.Region()
			if prior, present := work.changed[key]; present {
				region, ok = support.Union(prior, region)
				if !ok {
					return carrier.ChangeHandle{}, false
				}
			}
			work.changed[key] = region
		}
	}
	if len(work.changed) == 0 {
		return work.prepareChange(left, left, first, false, support.Mask{}, nil, nil, delta)
	}
	outside, ok := delta.Not(leftSupport)
	if !ok {
		return carrier.ChangeHandle{}, false
	}
	if cap(work.changeRows) < len(work.changed) {
		work.changeRows = make([]semantic.ContributionChange[K], 0, len(work.changed))
	}
	for key, region := range work.changed {
		work.changeRows = append(work.changeRows, semantic.ContributionChange[K]{Key: key, Region: region})
	}
	sort.Slice(work.changeRows, func(left, right int) bool { return work.changeRows[left].Key < work.changeRows[right].Key })
	work.changes.reset()
	defer work.changes.reset()
	report := func(key K, region support.Mask) bool {
		if len(work.changes.rows) != 0 && work.changes.rows[len(work.changes.rows)-1].key >= key {
			return false
		}
		work.changes.rows = append(work.changes.rows, mergeChange[K]{key: key, region: region})
		return true
	}
	// The semantic left region is the compact C surface, not full point
	// support. The physical region additionally carries every branch outside
	// point support, where it cannot participate in lifted semantics but must
	// remain shared until LiftRuleContribution closes it. reference remains the
	// semantic point support so out-of-support preservation has no fake delta.
	next, ok := work.binding.plane.domain.JoinContributionChanges(first, second, work.changeRows, &work.scratch, delta, report, func(key K) (support.Mask, support.Mask, support.Mask, bool) {
		leftRegion, leftPresent, leftValid := work.contributionRegionAt(leftCoverage, leftSupport, key)
		if !leftValid {
			return support.Mask{}, support.Mask{}, support.Mask{}, false
		}
		if !leftPresent {
			leftRegion, ok = support.FromGuard(work.binding.plane.domain.Guards(), work.binding.plane.domain.Guards().False())
			if !ok {
				return support.Mask{}, support.Mask{}, support.Mask{}, false
			}
		}
		leftPhysical, merged := delta.Or(leftRegion, outside)
		if !merged {
			return support.Mask{}, support.Mask{}, support.Mask{}, false
		}
		rightRegion, present := work.changed[key]
		if !present {
			return support.Mask{}, support.Mask{}, support.Mask{}, false
		}
		return leftPhysical, rightRegion, leftSupport, true
	})
	if !ok {
		return carrier.ChangeHandle{}, false
	}
	factor, units, regions, ok := work.binding.expandChanges(&work.changes, delta, &work.expand)
	if !ok {
		return carrier.ChangeHandle{}, false
	}
	if work.binding.plane.domain.Same(first, next) {
		if work.changes.Count() != 0 {
			return carrier.ChangeHandle{}, false
		}
		return work.prepareChange(left, left, next, false, support.Mask{}, nil, nil, delta)
	}
	// Never reuse right: even when its values cover the changed cells, the
	// PointRHS must retain left's latent out-of-support root branches.
	return work.prepareChange(left, carrier.RootHandle{}, next, true, factor, units, regions, delta)
}

// MergeTransportedPointUnder is the fused total PointState transport path.
// It deliberately does not consult source RHS coverage: an absent sparse
// PointState cell denotes the Factor Default on reachable source support.
// Target coverage is applied only when the transported semantic plane joins
// the output RHS and the final root is closed.
func (work *bindingWork[K, V]) MergeTransportedPointUnder(left, right carrier.RootHandle, leftSupport, sourceSupport, reindexedSupport, rightSupport support.Mask, relation guard.Reindex, leftCoverage, targetRightCoverage carrier.SlotCoverage, delta *support.Work) (carrier.ChangeHandle, bool) {
	if !work.live() || work.binding == nil || work.binding.plane == nil || delta == nil || !delta.Open() || !leftSupport.Valid() || !sourceSupport.Valid() || !reindexedSupport.Valid() || !rightSupport.Valid() || !relation.Valid() || leftSupport.Manager() != work.binding.plane.domain.Guards() || sourceSupport.Manager() != work.binding.plane.domain.Guards() || reindexedSupport.Manager() != work.binding.plane.domain.Guards() || rightSupport.Manager() != work.binding.plane.domain.Guards() {
		return carrier.ChangeHandle{}, false
	}
	first, ok := work.resolve(left)
	if !ok {
		return carrier.ChangeHandle{}, false
	}
	second, ok := work.resolve(right)
	if !ok {
		return carrier.ChangeHandle{}, false
	}
	// Coordinate identity changes no typed key or guard coordinate. Carrier has
	// already transported pre/post into rightSupport and targetRightCoverage,
	// so the complete Point edge is exactly a sparse closed contribution join
	// over those target-authored regions. Values outside that surface are never
	// consulted or published; a sparse missing terminal inside it still denotes
	// the Factor Default required by total PointState transport.
	if relation.CoordinateIdentity() {
		return work.MergeContributionUnder(left, right, leftSupport, rightSupport, leftCoverage, targetRightCoverage, delta)
	}
	transported := second
	transported, ok = work.binding.plane.domain.Reindex(second, sourceSupport, reindexedSupport, relation)
	if !ok {
		return carrier.ChangeHandle{}, false
	}
	transported, ok = work.binding.plane.domain.Restrict(transported, rightSupport)
	if !ok {
		return carrier.ChangeHandle{}, false
	}
	return work.mergeContributionPlanes(left, carrier.RootHandle{}, first, transported, leftSupport, rightSupport, leftCoverage, targetRightCoverage, delta, false)
}

func (work *bindingWork[K, V]) mergeContributionPlanes(leftHandle, rightHandle carrier.RootHandle, first, second semantic.Plane[planeFactor, K, V], leftSupport, rightSupport support.Mask, leftCoverage, rightCoverage carrier.SlotCoverage, delta *support.Work, reuseRight bool) (carrier.ChangeHandle, bool) {
	if !work.live() || work.binding == nil || work.binding.plane == nil || delta == nil || !delta.Open() || !leftSupport.Valid() || !rightSupport.Valid() || leftSupport.Manager() != work.binding.plane.domain.Guards() || rightSupport.Manager() != work.binding.plane.domain.Guards() {
		return carrier.ChangeHandle{}, false
	}
	work.changes.reset()
	defer work.changes.reset()
	empty, ok := support.FromGuard(work.binding.plane.domain.Guards(), work.binding.plane.domain.Guards().False())
	if !ok {
		return carrier.ChangeHandle{}, false
	}
	report := func(key K, region support.Mask) bool {
		if len(work.changes.rows) != 0 && work.changes.rows[len(work.changes.rows)-1].key >= key {
			return false
		}
		work.changes.rows = append(work.changes.rows, mergeChange[K]{key: key, region: region})
		return true
	}
	next, ok := work.binding.plane.domain.JoinContributions(first, second, &work.scratch, delta, report, func(key K) (support.Mask, support.Mask, support.Mask, bool) {
		leftRegion, leftPresent, leftValid := work.contributionRegionAt(leftCoverage, leftSupport, key)
		if !leftValid {
			return support.Mask{}, support.Mask{}, support.Mask{}, false
		}
		if !leftPresent {
			leftRegion = empty
		}
		rightRegion, rightPresent, rightValid := work.contributionRegionAt(rightCoverage, rightSupport, key)
		if !rightValid {
			return support.Mask{}, support.Mask{}, support.Mask{}, false
		}
		if !rightPresent {
			rightRegion = empty
		}
		// A physical key outside both compact coverage surfaces is hostile
		// residue.  Treat it as Absent on both sides so the output closes it.
		return leftRegion, rightRegion, leftRegion, true
	})
	if !ok {
		return carrier.ChangeHandle{}, false
	}
	// JoinContributions is the sealing constructor for this plane. It traverses
	// each sparse key under the two expanded authored surfaces, treats an
	// uncovered operand as Absent, and emits nothing outside their union.
	// Re-closing or re-scanning the result here would repeat that traversal in
	// every point fold without strengthening the construction proof.
	if work.binding.plane.domain.Same(first, next) {
		if work.changes.Count() != 0 {
			return carrier.ChangeHandle{}, false
		}
		return work.prepareChange(leftHandle, leftHandle, next, false, support.Mask{}, nil, nil, delta)
	}
	factor, units, regions, ok := work.binding.expandChanges(&work.changes, delta, &work.expand)
	if !ok {
		return carrier.ChangeHandle{}, false
	}
	if work.binding.plane.domain.Same(second, next) {
		if reuseRight {
			return work.prepareChange(leftHandle, rightHandle, next, false, factor, units, regions, delta)
		}
		// A transport never reuses the source root: even a same-looking output
		// must carry the post-closed physical plane, not hidden source fibers.
		return work.prepareChange(leftHandle, carrier.RootHandle{}, second, true, factor, units, regions, delta)
	}
	return work.prepareChange(leftHandle, carrier.RootHandle{}, next, true, factor, units, regions, delta)
}

// BeginObservation opens one explicit projection generation for this slot.
// It resets reusable output and partition scratch once; later ObserveUnder
// calls append rows to this same generation until EndObservation closes it.
func (work *bindingWork[K, V]) BeginObservation() bool {
	if !work.live() || !work.binding.live() || work.binding.plane == nil || work.observationLive || work.nextObservation == ^uint64(0) {
		return false
	}
	generation, ok := work.binding.issuer.BeginObservation(work.observations)
	if !ok {
		return false
	}
	work.resetObservationScratch(generation)
	return true
}

// EndObservation invalidates every row and typed entry view from this slot's
// current projection generation. It is the normal executor cleanup after all
// staged reads and final Rule callbacks have completed.
func (work *bindingWork[K, V]) EndObservation() bool {
	if work == nil || !work.observationLive || work.binding == nil {
		return false
	}
	work.closeObservation()
	return true
}

func (work *bindingWork[K, V]) closeObservation() {
	if work == nil || !work.observationLive {
		return
	}
	// These buffers are reused for their capacity, but they may retain typed
	// values, root issuers, and support pages. Clear every live element before
	// truncating so an escaped stale Observation cannot pin an old graph until
	// this SlotWork happens to begin another generation.
	work.clearObservationScratch()
	work.observationLive = false
	if work.binding != nil {
		work.binding.issuer.EndObservation(work.observations, work.generation)
	}
}

// ObserveUnder partitions exactly one declared Unit under the supplied root
// and shared support. It requires the caller's live projection generation;
// exact Units use the single-key fast path and summaries preserve frozen
// stored-or-absent entry sequences. Any failure closes that generation so a
// partially emitted row can never remain resolvable after a failed read.
func (work *bindingWork[K, V]) ObserveUnder(root carrier.RootHandle, unit carrier.Unit, within support.Mask, visit func(carrier.ObservationRow) bool) (completed bool) {
	if !work.live() || !work.observationLive || visit == nil {
		return false
	}
	defer func() {
		if !completed {
			work.closeObservation()
		}
	}()
	binding := work.binding
	if binding == nil || !within.Valid() || binding.plane == nil || within.Manager() != binding.plane.domain.Guards() {
		return false
	}
	input, ok := work.resolve(root)
	if !ok || !binding.ValidUnit(unit) {
		return false
	}
	descriptor, ok := binding.units[unit]
	if !ok {
		return false
	}
	if support.Empty(within) {
		return true
	}
	// The fold is sealed into the Unit at declaration. A summary declared
	// coordinate-wise is folded per declared key, so its cost is the sum of
	// the per-key partitions rather than their joint product.
	if descriptor.distributive {
		return work.observeDistributiveSummary(input, root, unit, descriptor.keys, within, visit)
	}
	if len(descriptor.keys) == 1 {
		return work.observeExact(input, root, unit, descriptor.keys[0], within, visit)
	}
	return work.observeSummary(input, root, unit, descriptor.keys, within, visit)
}

func (work *bindingWork[K, V]) resetObservationScratch(generation identity.Generation) {
	work.clearObservationScratch()
	work.observationLive = true
	work.generation = generation
	work.firstObservation = work.nextObservation + 1
}

func (work *bindingWork[K, V]) clearObservationScratch() {
	if work == nil {
		return
	}
	clear(work.records)
	work.records = work.records[:0]
	clear(work.entries)
	work.entries = work.entries[:0]
	clear(work.pieces)
	work.pieces = work.pieces[:0]
	clear(work.partials)
	work.partials = work.partials[:0]
	clear(work.nextPartials)
	work.nextPartials = work.nextPartials[:0]
	clear(work.spine)
	work.spine = work.spine[:0]
	clear(work.buckets)
	work.observationCursorMode = directObservationDone
	work.observationCursorIndex = 0
	work.observationCursorRoot = carrier.RootHandle{}
	work.observationCursorUnit = carrier.Unit{}
	work.observationCursorFailed = false
}

func (work *bindingWork[K, V]) observeExact(input semantic.Plane[planeFactor, K, V], root carrier.RootHandle, unit carrier.Unit, key K, within support.Mask, visit func(carrier.ObservationRow) bool) bool {
	if !work.beginExactObservation(input, root, unit, key, within) {
		return false
	}
	return work.emitDirectObservations(visit)
}

func (work *bindingWork[K, V]) exactPiecesDistinct() bool {
	for index, piece := range work.pieces {
		for previous := 0; previous < index; previous++ {
			if work.entryEqual(piece.entry, work.pieces[previous].entry) {
				return false
			}
		}
	}
	return true
}

// observeDistributiveSummary traverses each declared key once and emits the
// single row that carries the per-coordinate fold over that key's pieces.
// It builds no candidate support transaction and no group product: the
// declared fold makes the joint partition unobservable, so the whole of
// within is one region.
func (work *bindingWork[K, V]) observeDistributiveSummary(input semantic.Plane[planeFactor, K, V], root carrier.RootHandle, unit carrier.Unit, keys []K, within support.Mask, visit func(carrier.ObservationRow) bool) bool {
	if !work.beginSummaryObservation(input, root, unit, keys, true, within) {
		return false
	}
	return work.emitDirectObservations(visit)
}

func (work *bindingWork[K, V]) observeSummary(input semantic.Plane[planeFactor, K, V], root carrier.RootHandle, unit carrier.Unit, keys []K, within support.Mask, visit func(carrier.ObservationRow) bool) bool {
	if !work.beginSummaryObservation(input, root, unit, keys, false, within) {
		return false
	}
	return work.emitDirectObservations(visit)
}

// partitionKey is the only read of a declared typed key. semantic.PartitionKey
// returns its exact terminal/presence pieces in one traversal, avoiding the
// old PartitionValue followed by SummaryUnder retraversal.
//
// It lends readWork for the refinement. This read runs once per declared key of
// every observed fold, which is the engine's densest Boolean call site, so the
// shell is owned here rather than minted inside the read.
// declaredKeyBase resolves the observed Unit's base coordinate in the sealed
// declared-key inventory. An undeclared Unit has no coordinate and its reads
// take the ordinary recomputing path.
func (work *bindingWork[K, V]) declaredKeyBase(unit carrier.Unit) int {
	descriptor, declared := work.binding.units[unit]
	if !declared || descriptor.position < 0 || descriptor.position >= len(work.readKeyOffsets) {
		return -1
	}
	return work.readKeyOffsets[descriptor.position]
}

func (work *bindingWork[K, V]) partitionKey(input semantic.Plane[planeFactor, K, V], key K, keyIndex int, within support.Mask) bool {
	dbgFactBinding.ReadMemoReads++
	coordinate := -1
	if work.observationCursorKeys >= 0 {
		coordinate = work.observationCursorKeys + keyIndex
	}
	if pieces, found := work.readMemo.lookup(coordinate, work.observationCursorRoot, key, within); found {
		work.pieces = append(work.pieces[:0], pieces...)
		return true
	}
	work.pieces = work.pieces[:0]
	if work.partitionVisit == nil {
		work.partitionVisit = work.acceptPartition
	}
	completed := work.binding.plane.domain.PartitionKey(input, key, within, work.readWork, work.partitionVisit)
	if completed {
		work.readMemo.replace(coordinate, work.observationCursorRoot, key, within, work.pieces)
	}
	return completed
}

func (work *bindingWork[K, V]) acceptPartition(value V, present bool, region support.Mask) bool {
	if work == nil {
		return false
	}
	entry := ObservationEntry[V]{value: value, present: present}
	work.pieces = append(work.pieces, observationPiece[V]{entry: entry, fingerprint: work.entryFingerprint(entry), region: region})
	return true
}

// constantOverRegion reports whether the partition just read for one declared
// key covers the whole observed region with a single piece. That piece's
// region is then the observed region itself, so the key names the same
// stored-or-absent entry at every valuation of the region and distinguishes
// nothing within it.
func (work *bindingWork[K, V]) constantOverRegion(within support.Mask) bool {
	return len(work.pieces) == 1 && work.pieces[0].region.SameHandle(within)
}

func (work *bindingWork[K, V]) seedGroups(unions *support.Work) bool {
	if unions == nil || len(work.pieces) == 0 {
		return false
	}
	work.partials = work.partials[:0]
	work.resetSpine()
	work.clearBuckets()
	empty := unions.False()
	for _, piece := range work.pieces {
		if !work.live() {
			return false
		}
		region, ok := unions.Or(empty, piece.region)
		if !ok || !work.addGroup(&work.partials, -1, piece.entry, foldObservationFingerprint(observationFingerprintSeed, piece.fingerprint), region, unions) {
			return false
		}
	}
	return len(work.partials) != 0
}

func (work *bindingWork[K, V]) extendGroups(unions *support.Work, within support.Mask) bool {
	if unions == nil || len(work.partials) == 0 || len(work.pieces) == 0 {
		return false
	}
	dbgFactBinding.SummaryExtendKeys++
	if uint64(len(work.partials)) > dbgFactBinding.SummaryMaxPartials {
		dbgFactBinding.SummaryMaxPartials = uint64(len(work.partials))
	}
	// A key constant over the observed region refines no prefix. Every partial
	// region is a subset of that region - the seed is a partition of it and
	// each later extension intersects - so the conjunction of a partial with
	// the single covering piece is that same partial. The extension is then a
	// pure sequence step and the whole key costs no Boolean work.
	constant := work.constantOverRegion(within)
	if constant {
		dbgFactBinding.SummaryConstantKeys++
	}
	work.nextPartials = work.nextPartials[:0]
	work.clearBuckets()
	for _, prefix := range work.partials {
		if !work.live() {
			return false
		}
		for _, piece := range work.pieces {
			if !work.live() {
				return false
			}
			dbgFactBinding.SummaryPairs++
			region := prefix.region
			if !constant {
				dbgFactBinding.SummaryConjunctions++
				intersected, ok := unions.And(prefix.region, piece.region)
				if !ok {
					return false
				}
				region = intersected
			}
			view, ok := unions.Decompose(region)
			if !ok {
				return false
			}
			if view.Terminal && !view.Value {
				continue
			}
			if !work.addGroup(&work.nextPartials, prefix.cell, piece.entry, foldObservationFingerprint(prefix.fingerprint, piece.fingerprint), region, unions) {
				return false
			}
		}
	}
	work.partials, work.nextPartials = work.nextPartials, work.partials
	return len(work.partials) != 0
}

// addGroup coalesces every semantically equal complete or partial sequence,
// not merely adjacent symbolic cells. Bucket lookup is an optimization only:
// Factor Equal and the stored/absent bit decide equality. groups remains the
// deterministic first-discovery order used for final emission. prefix names
// the spine cell this sequence extends, so a new group costs one cell and
// keeps the prefix it was built from.
func (work *bindingWork[K, V]) addGroup(groups *[]observationGroup, prefix int, tail ObservationEntry[V], fingerprint uint64, region support.Mask, unions *support.Work) bool {
	if groups == nil || unions == nil || prefix >= len(work.spine) {
		return false
	}
	count := work.cellDepth(prefix) + 1
	for index, found := work.buckets[fingerprint]; found && index >= 0; {
		candidate := &(*groups)[index]
		if candidate.count != count || !work.sequenceEqual(candidate.cell, prefix, tail) {
			index = candidate.previous
			continue
		}
		merged, ok := unions.Or(candidate.region, region)
		if !ok {
			return false
		}
		candidate.region = merged
		return true
	}
	previous := -1
	if last, found := work.buckets[fingerprint]; found {
		previous = last
	}
	*groups = append(*groups, observationGroup{cell: work.appendCell(prefix, tail), count: count, fingerprint: fingerprint, region: region, previous: previous})
	work.buckets[fingerprint] = len(*groups) - 1
	return true
}

// sequenceEqual compares one stored sequence against the prefix spine cell
// extended by tail. Both walks have the same length because the caller has
// already matched the stored count, and reaching one shared cell proves the
// remaining prefixes are the same sequence.
func (work *bindingWork[K, V]) sequenceEqual(stored, prefix int, tail ObservationEntry[V]) bool {
	if stored < 0 || stored >= len(work.spine) {
		return false
	}
	if !work.entryEqual(work.spine[stored].entry, tail) {
		return false
	}
	for left, right := work.spine[stored].parent, prefix; left != right; {
		if left < 0 || right < 0 {
			return false
		}
		if !work.entryEqual(work.spine[left].entry, work.spine[right].entry) {
			return false
		}
		left, right = work.spine[left].parent, work.spine[right].parent
	}
	return true
}

// appendCell stores one sequence terminal linked to the prefix cell it
// extends. A negative parent starts a new sequence.
func (work *bindingWork[K, V]) appendCell(parent int, entry ObservationEntry[V]) int {
	work.spine = append(work.spine, observationCell[V]{entry: entry, parent: parent, depth: work.cellDepth(parent) + 1})
	return len(work.spine) - 1
}

func (work *bindingWork[K, V]) cellDepth(cell int) int {
	if cell < 0 || cell >= len(work.spine) {
		return 0
	}
	return work.spine[cell].depth
}

func (work *bindingWork[K, V]) resetSpine() {
	clear(work.spine)
	work.spine = work.spine[:0]
}

func (work *bindingWork[K, V]) entryEqual(left, right ObservationEntry[V]) bool {
	return left.present == right.present && work.binding.algebra.equal(left.value, right.value)
}

func (work *bindingWork[K, V]) entryFingerprint(entry ObservationEntry[V]) uint64 {
	fingerprint := work.binding.algebra.fingerprint(entry.value)
	if entry.present {
		return fingerprint ^ 0x9e3779b97f4a7c15
	}
	return fingerprint ^ 0x517cc1b727220a95
}

const observationFingerprintSeed uint64 = 0x6a09e667f3bcc909

func foldObservationFingerprint(current, next uint64) uint64 {
	return current ^ (next + 0x9e3779b97f4a7c15 + current<<6 + current>>2)
}

func (work *bindingWork[K, V]) clearBuckets() {
	if work.buckets == nil {
		work.buckets = make(map[uint64]int)
		return
	}
	clear(work.buckets)
}

// emitGroup is the callback-free emission authority for one grouped
// observation.  ObserveUnder's summary adapter and the exact cursor both use
// this constructor; only the former invokes the caller visitor.
func (work *bindingWork[K, V]) emitGroup(root carrier.RootHandle, unit carrier.Unit, group observationGroup) (carrier.ObservationRow, bool) {
	if !work.observationLive || !work.live() || work.nextObservation == ^uint64(0) || !group.region.Valid() {
		return carrier.ObservationRow{}, false
	}
	first := len(work.entries)
	if !work.appendSequence(group.cell, group.count) {
		return carrier.ObservationRow{}, false
	}
	work.nextObservation++
	id := work.nextObservation
	handle, ok := work.binding.issuer.IssueObservation(work.observations, work.generation, id)
	if !ok {
		return carrier.ObservationRow{}, false
	}
	row, ok := work.observations.Row(handle, group.region)
	if !ok {
		return carrier.ObservationRow{}, false
	}
	work.records = append(work.records, observationRecord{root: root, unit: unit, region: group.region, first: first, count: group.count})
	return row, true
}

// appendSequence materializes one group's declared-order sequence into the
// flat entry slab that observationEntry indexes by position. The spine links
// terminal to prefix, so the sequence is filled from its last entry back. A
// group of the sealed empty projection has no terminal cell and materializes
// no entry, which the walk below expresses as a vacuous fill.
func (work *bindingWork[K, V]) appendSequence(cell, count int) bool {
	if count < 0 {
		return false
	}
	first := len(work.entries)
	work.entries = slices.Grow(work.entries, count)[:first+count]
	for index := count - 1; index >= 0; index-- {
		if cell < 0 || cell >= len(work.spine) {
			work.entries = work.entries[:first]
			return false
		}
		work.entries[first+index] = work.spine[cell].entry
		cell = work.spine[cell].parent
	}
	if cell >= 0 {
		work.entries = work.entries[:first]
		return false
	}
	return true
}

func (work *bindingWork[K, V]) emitExactPiece(root carrier.RootHandle, unit carrier.Unit, piece observationPiece[V]) (carrier.ObservationRow, bool) {
	if !work.observationLive || !work.live() || work.nextObservation == ^uint64(0) || !piece.region.Valid() {
		return carrier.ObservationRow{}, false
	}
	first := len(work.entries)
	work.entries = append(work.entries, piece.entry)
	work.nextObservation++
	id := work.nextObservation
	handle, ok := work.binding.issuer.IssueObservation(work.observations, work.generation, id)
	if !ok {
		return carrier.ObservationRow{}, false
	}
	row, ok := work.observations.Row(handle, piece.region)
	if !ok {
		return carrier.ObservationRow{}, false
	}
	work.records = append(work.records, observationRecord{root: root, unit: unit, region: piece.region, first: first, count: 1})
	return row, true
}

// ResolveObservation returns the generation-bound typed entry sequence for
// this Binding's exact SlotWork. A row from another issuer, root, unit, work,
// or stale callback generation is rejected before a typed entry can escape.
func (binding *Binding[K, V]) ResolveObservation(slot carrier.SlotWork, row carrier.ObservationRow) (Observation[V], bool) {
	work, ok := slot.(*bindingWork[K, V])
	if !binding.live() || !ok || work.binding != binding || !work.observationLive {
		return Observation[V]{}, false
	}
	handle, region, ok := work.observations.ResolveRow(row)
	if !ok || !region.Valid() || binding.plane == nil || region.Manager() != binding.plane.domain.Guards() {
		return Observation[V]{}, false
	}
	id, ok := binding.issuer.ResolveObservation(work.observations, work.generation, handle)
	record, valid := work.record(work.generation, id)
	// An observation row may have been emitted while a carrier Preview owns
	// its predecessor.  Its root is readable for that transaction, but must
	// still satisfy the ordinary published-root law once the Preview ends.
	_, rootLive := work.resolve(record.root)
	if !ok || !valid || !rootLive || !binding.ValidUnit(record.unit) || !record.region.SameHandle(region) {
		return Observation[V]{}, false
	}
	return Observation[V]{resolver: work, generation: work.generation, id: id}, true
}

func (work *bindingWork[K, V]) record(generation identity.Generation, id uint64) (observationRecord, bool) {
	if work == nil || !work.observationLive || !generation.Available() || generation != work.generation || id < work.firstObservation {
		return observationRecord{}, false
	}
	index := id - work.firstObservation
	if index >= uint64(len(work.records)) {
		return observationRecord{}, false
	}
	return work.records[index], true
}

func (work *bindingWork[K, V]) observationCount(generation identity.Generation, id uint64) (int, bool) {
	record, ok := work.record(generation, id)
	return record.count, ok
}

func (work *bindingWork[K, V]) observationEntry(generation identity.Generation, id uint64, index int) (ObservationEntry[V], bool) {
	record, ok := work.record(generation, id)
	if !ok || index < 0 || index >= record.count || record.first < 0 || record.first > len(work.entries)-record.count {
		return ObservationEntry[V]{}, false
	}
	return work.entries[record.first+index], true
}

func (work *bindingWork[K, V]) Merge3Under(kind carrier.MergeKind, recurrence bool, scope uint64, left, right carrier.RootHandle, split support.Split, delta *support.Work) (carrier.ChangeHandle, bool) {
	if !work.live() {
		return carrier.ChangeHandle{}, false
	}
	work.changes.reset()
	defer work.changes.reset()
	binding := work.binding
	if binding == nil || delta == nil || !delta.Open() {
		return carrier.ChangeHandle{}, false
	}
	first, ok := work.resolve(left)
	if !ok {
		return carrier.ChangeHandle{}, false
	}
	second, ok := work.resolve(right)
	if !ok {
		return carrier.ChangeHandle{}, false
	}
	var next semantic.Plane[planeFactor, K, V]
	if !recurrence && kind == carrier.Narrow {
		// Narrow publishes only the right support. An unselected Factor may
		// therefore survive only when its left/right meanings agree there;
		// returning the right root avoids reconstructing a union that would
		// resurrect left-only facts outside the narrowed State.
		if !binding.plane.domain.EqualUnder(first, second, split.Right(), &work.scratch) {
			return carrier.ChangeHandle{}, false
		}
		return work.prepareChange(left, right, second, false, support.Mask{}, nil, nil, delta)
	}
	var report diagram.SoleChange[K]
	if recurrence || kind == carrier.Widen {
		report = func(key K, region support.Mask) bool {
			if len(work.changes.rows) != 0 && work.changes.rows[len(work.changes.rows)-1].key >= key {
				return false
			}
			work.changes.rows = append(work.changes.rows, mergeChange[K]{key: key, region: region})
			return true
		}
	}
	if !recurrence {
		if kind == carrier.Widen {
			// Widen is carrier-monotone: its caller proved left support is a
			// subset of right support.  An unselected Factor is therefore a
			// structural coordinate replacement, not a preserved union.  Keep
			// right's exact root even when overlap meanings agree so later
			// support growth cannot reveal an old off-support value.
			if !support.Empty(split.LeftOnly()) || !binding.plane.domain.ReplaceUnder(first, second, split, &work.scratch, delta, report) {
				return carrier.ChangeHandle{}, false
			}
			next = second
		} else {
			next, ok = binding.plane.domain.PreserveUnder(first, second, split, &work.scratch, nil, nil)
		}
	} else {
		switch kind {
		case carrier.Join:
			next, ok = binding.plane.domain.JoinUnder(first, second, split, &work.scratch, delta, report)
		case carrier.Widen:
			if scope == 0 {
				return carrier.ChangeHandle{}, false
			}
			selected, valid := binding.widenScope(scope)
			if !valid {
				return carrier.ChangeHandle{}, false
			}
			next, ok = binding.plane.domain.WidenUnderKeys(first, second, split, &work.scratch, delta, report, selected.contains)
		case carrier.Narrow:
			if scope == 0 {
				return carrier.ChangeHandle{}, false
			}
			selected, valid := binding.narrowScope(scope)
			if !valid {
				return carrier.ChangeHandle{}, false
			}
			next, ok = binding.plane.domain.NarrowUnderKeys(first, second, split, &work.scratch, delta, report, selected.contains)
		default:
			return carrier.ChangeHandle{}, false
		}
	}
	if !ok {
		return carrier.ChangeHandle{}, false
	}
	// A semantic no-op keeps the predecessor identity.  RootStore deliberately
	// does not deduplicate, so this check belongs at the typed operation that
	// knows the reachable output support rather than in storage.
	within := split.Union()
	if kind == carrier.Narrow {
		within = split.Right()
	}
	if (recurrence || kind != carrier.Widen) && binding.plane.domain.EqualUnder(first, next, within, &work.scratch) {
		if work.changes.Count() != 0 {
			return carrier.ChangeHandle{}, false
		}
		return work.prepareChange(left, left, next, false, support.Mask{}, nil, nil, delta)
	}
	var factor support.Mask
	var units []carrier.Unit
	var regions []support.Mask
	if recurrence || kind == carrier.Widen {
		factor, units, regions, ok = binding.expandChanges(&work.changes, delta, &work.expand)
		if !ok {
			return carrier.ChangeHandle{}, false
		}
	}
	if binding.plane.domain.EqualUnder(second, next, within, &work.scratch) {
		return work.prepareChange(left, right, next, false, factor, units, regions, delta)
	}
	return work.prepareChange(left, carrier.RootHandle{}, next, true, factor, units, regions, delta)
}

// MergeSelectedContributionUnder is the closed recurrence publication.  The
// selected Widen/Narrow calculation remains key-local, but its result is
// required to agree with, and is physically masked to, the exact RHS
// authored surface.  Thus historical current coverage can never leak through
// a selected root after the exact RHS has changed.
func (work *bindingWork[K, V]) MergeSelectedContributionUnder(kind carrier.MergeKind, scope uint64, current, selectedRight, exactRight carrier.RootHandle, selectedSplit, exactSplit support.Split, _ carrier.SlotCoverage, _ carrier.SlotCoverage, exactCoverage carrier.SlotCoverage, delta *support.Work) (carrier.ChangeHandle, bool) {
	if !work.live() || delta == nil || !delta.Open() || (kind != carrier.Widen && kind != carrier.Narrow) {
		return carrier.ChangeHandle{}, false
	}
	work.changes.reset()
	defer work.changes.reset()
	binding := work.binding
	if binding == nil || binding.plane == nil {
		return carrier.ChangeHandle{}, false
	}
	left, ok := work.resolve(current)
	if !ok {
		return carrier.ChangeHandle{}, false
	}
	selectedPlane, ok := work.resolve(selectedRight)
	if !ok {
		return carrier.ChangeHandle{}, false
	}
	exactPlane, ok := work.resolve(exactRight)
	if !ok {
		return carrier.ChangeHandle{}, false
	}
	var next semantic.Plane[planeFactor, K, V]
	var chosen widenScope[K]
	switch kind {
	case carrier.Widen:
		chosen, ok = binding.widenScope(scope)
		if !ok {
			return carrier.ChangeHandle{}, false
		}
		widened, valid := binding.plane.domain.WidenUnderKeys(left, selectedPlane, selectedSplit, &work.scratch, nil, nil, chosen.contains)
		if !valid {
			return carrier.ChangeHandle{}, false
		}
		mixed, valid := work.threeSupport(selectedSplit.Right(), exactSplit.Right())
		if !valid {
			return carrier.ChangeHandle{}, false
		}
		next, ok = binding.plane.domain.SelectUnderKeys(widened, exactPlane, mixed, &work.scratch, nil, nil, chosen.contains)
	case carrier.Narrow:
		chosen, ok = binding.narrowScope(scope)
		if !ok {
			return carrier.ChangeHandle{}, false
		}
		narrowed, valid := binding.plane.domain.NarrowUnderKeys(left, selectedPlane, selectedSplit, &work.scratch, nil, nil, chosen.contains)
		if !valid {
			return carrier.ChangeHandle{}, false
		}
		mixed, valid := work.threeSupport(selectedSplit.Right(), exactSplit.Right())
		if !valid {
			return carrier.ChangeHandle{}, false
		}
		next, ok = binding.plane.domain.SelectUnderKeys(narrowed, exactPlane, mixed, &work.scratch, nil, nil, chosen.contains)
	}
	if !ok {
		return carrier.ChangeHandle{}, false
	}
	outputSupport := exactSplit.Right()
	defer func() { clear(work.coverageOutput) }()
	work.coverageOutput, ok = work.contributionCoverage(exactCoverage, outputSupport, work.coverageOutput)
	if !ok {
		return carrier.ChangeHandle{}, false
	}
	closed, _, ok := binding.plane.domain.CloseContribution(next, outputSupport, func(key K) (support.Mask, bool) {
		return contributionRegion(work.coverageOutput, key)
	}, &work.scratch, delta)
	// Unlike Finish, a selected recurrence is allowed to derive a raw semantic
	// value on a historical current fiber that exact RHS authorship has since
	// removed.  That fiber is Absent in the publishable contribution, so the
	// final closed plane (not the pre-close raw recurrence plane) is the
	// semantic result.  ReplaceUnder below deliberately derives the published
	// delta from that closed result.
	if !ok {
		return carrier.ChangeHandle{}, false
	}
	report := func(key K, region support.Mask) bool {
		if len(work.changes.rows) != 0 && work.changes.rows[len(work.changes.rows)-1].key >= key {
			return false
		}
		work.changes.rows = append(work.changes.rows, mergeChange[K]{key: key, region: region})
		return true
	}
	if !binding.plane.domain.ReplaceUnder(left, closed, exactSplit, &work.scratch, delta, report) {
		return carrier.ChangeHandle{}, false
	}
	factor, units, regions, ok := binding.expandChanges(&work.changes, delta, &work.expand)
	if !ok {
		return carrier.ChangeHandle{}, false
	}
	if binding.plane.domain.Same(left, closed) {
		if work.changes.Count() != 0 {
			return carrier.ChangeHandle{}, false
		}
		return work.prepareChange(current, current, closed, false, support.Mask{}, nil, nil, delta)
	}
	if binding.plane.domain.Same(exactPlane, closed) {
		return work.prepareChange(current, exactRight, closed, false, factor, units, regions, delta)
	}
	return work.prepareChange(current, carrier.RootHandle{}, closed, true, factor, units, regions, delta)
}

// ReindexUnder applies carrier's one sealed source-to-target relation to this
// Binding's typed plane. Source support is mandatory: Domain.Reindex totals a
// column only on that region before joining reachable fibers, preventing an
// off-support branch from contributing to a target value. Boundary transport
// is not a demand/publication event, so it deliberately reports no unit or
// Factor delta across incomparable coordinate scopes.
func (work *bindingWork[K, V]) ReindexUnder(left carrier.RootHandle, source, target support.Mask, relation guard.Reindex, delta *support.Work) (carrier.ChangeHandle, bool) {
	if !work.live() || delta == nil || !delta.Open() || !relation.Valid() {
		return carrier.ChangeHandle{}, false
	}
	binding := work.binding
	if binding == nil || binding.plane == nil {
		return carrier.ChangeHandle{}, false
	}
	first, ok := work.resolve(left)
	if !ok {
		return carrier.ChangeHandle{}, false
	}
	next, ok := binding.plane.domain.Reindex(first, source, target, relation)
	if !ok {
		return carrier.ChangeHandle{}, false
	}
	// Whole-plane equality is the complete proof needed to retain a root even
	// across a nonidentity boundary. An old-support comparison would be too
	// weak because a later target support growth could reveal a hidden branch.
	if binding.plane.domain.Same(first, next) {
		return work.prepareChange(left, left, first, false, support.Mask{}, nil, nil, delta)
	}
	return work.prepareChange(left, carrier.RootHandle{}, next, true, support.Mask{}, nil, nil, delta)
}

// ReindexContributionUnder is lifted-partial contribution transport.  It
// never delegates to raw Reindex: raw State must totalize Default over outer
// support, while this path may totalize only source-present coverage.
func (work *bindingWork[K, V]) ReindexContributionUnder(left carrier.RootHandle, source, target support.Mask, relation guard.Reindex, sourceCoverage, targetCoverage carrier.SlotCoverage, delta *support.Work) (carrier.ChangeHandle, bool) {
	if !work.live() || delta == nil || !delta.Open() || !relation.Valid() {
		return carrier.ChangeHandle{}, false
	}
	binding := work.binding
	if binding == nil || binding.plane == nil || !source.Valid() || !target.Valid() || source.Manager() != binding.plane.domain.Guards() || target.Manager() != binding.plane.domain.Guards() {
		return carrier.ChangeHandle{}, false
	}
	first, ok := work.resolve(left)
	if !ok {
		return carrier.ChangeHandle{}, false
	}
	defer func() {
		clear(work.coverageRight)
		clear(work.coverageOutput)
	}()
	work.coverageRight, ok = work.contributionCoverage(sourceCoverage, source, work.coverageRight)
	if !ok {
		return carrier.ChangeHandle{}, false
	}
	work.coverageOutput, ok = work.contributionCoverage(targetCoverage, target, work.coverageOutput)
	if !ok {
		return carrier.ChangeHandle{}, false
	}
	next, ok := binding.plane.domain.ReindexContribution(first, source, target, relation, func(key K) (support.Mask, bool) {
		return contributionRegion(work.coverageRight, key)
	}, func(key K) (support.Mask, bool) {
		return contributionRegion(work.coverageOutput, key)
	})
	if !ok {
		return carrier.ChangeHandle{}, false
	}
	if binding.plane.domain.Same(first, next) {
		return work.prepareChange(left, left, first, false, support.Mask{}, nil, nil, delta)
	}
	return work.prepareChange(left, carrier.RootHandle{}, next, true, support.Mask{}, nil, nil, delta)
}

// ReindexPointContributionUnder keeps semantic PointState transport total:
// every reachable sparse absence is Default before the sealed relation is
// applied.  The resulting target root is then closed to the contribution's
// transported C, making the root suitable for RHS folding without changing
// raw State transport semantics elsewhere.
func (work *bindingWork[K, V]) ReindexPointContributionUnder(left carrier.RootHandle, source, target support.Mask, relation guard.Reindex, targetCoverage carrier.SlotCoverage, delta *support.Work) (carrier.ChangeHandle, bool) {
	if !work.live() || delta == nil || !delta.Open() || !relation.Valid() {
		return carrier.ChangeHandle{}, false
	}
	binding := work.binding
	if binding == nil || binding.plane == nil || !source.Valid() || !target.Valid() || source.Manager() != binding.plane.domain.Guards() || target.Manager() != binding.plane.domain.Guards() {
		return carrier.ChangeHandle{}, false
	}
	first, ok := work.resolve(left)
	if !ok {
		return carrier.ChangeHandle{}, false
	}
	next := first
	if !relation.CoordinateIdentity() {
		next, ok = binding.plane.domain.Reindex(first, source, target, relation)
		if !ok {
			return carrier.ChangeHandle{}, false
		}
		next, ok = binding.plane.domain.Restrict(next, target)
		if !ok {
			return carrier.ChangeHandle{}, false
		}
	}
	defer func() { clear(work.coverageOutput) }()
	work.coverageOutput, ok = work.contributionCoverage(targetCoverage, target, work.coverageOutput)
	if !ok {
		return carrier.ChangeHandle{}, false
	}
	closed, _, ok := binding.plane.domain.CloseContribution(next, target, func(key K) (support.Mask, bool) {
		return contributionRegion(work.coverageOutput, key)
	}, &work.scratch, delta)
	if !ok {
		return carrier.ChangeHandle{}, false
	}
	if binding.plane.domain.Same(first, closed) {
		return work.prepareChange(left, left, closed, false, support.Mask{}, nil, nil, delta)
	}
	return work.prepareChange(left, carrier.RootHandle{}, closed, true, support.Mask{}, nil, nil, delta)
}

func (binding *Binding[K, V]) widenScope(id uint64) (widenScope[K], bool) {
	if binding == nil || id == 0 || id > uint64(len(binding.widenScopes)) {
		return widenScope[K]{}, false
	}
	scope := binding.widenScopes[id-1]
	return scope, len(scope.keys) != 0
}

func (binding *Binding[K, V]) narrowScope(id uint64) (widenScope[K], bool) {
	if binding == nil || id == 0 || id > uint64(len(binding.narrowScopes)) {
		return widenScope[K]{}, false
	}
	scope := binding.narrowScopes[id-1]
	return scope, len(scope.keys) != 0
}

func (scope widenScope[K]) contains(key K) bool {
	low, high := 0, len(scope.keys)
	for low < high {
		middle := low + (high-low)/2
		if scope.keys[middle] < key {
			low = middle + 1
		} else {
			high = middle
		}
	}
	return low < len(scope.keys) && scope.keys[low] == key
}

// ReplaceUnder is the typed half of carrier's structural coordinate
// replacement. It always retains the already-valid right root, even when its
// overlap meaning equals left: a later support expansion must not reveal a
// hidden old root. The fused zipper merely derives old-to-right unit regions
// on split.Overlap(); it never applies a lattice operation or Factor Narrow.
func (work *bindingWork[K, V]) ReplaceUnder(left, right carrier.RootHandle, split support.Split, delta *support.Work) (carrier.ChangeHandle, bool) {
	if !work.live() {
		return carrier.ChangeHandle{}, false
	}
	work.changes.reset()
	defer work.changes.reset()
	binding := work.binding
	if binding == nil || delta == nil || !delta.Open() {
		return carrier.ChangeHandle{}, false
	}
	first, ok := work.resolve(left)
	if !ok {
		return carrier.ChangeHandle{}, false
	}
	second, ok := work.resolve(right)
	if !ok {
		return carrier.ChangeHandle{}, false
	}
	// Exact root identity needs neither a sparse/FDD traversal nor a typed
	// comparison: no unit can differ. This keeps structural replacement at
	// O(F + changed-plane diff) when most carried roots are unchanged.
	if left == right {
		return work.prepareChange(left, right, second, false, support.Mask{}, nil, nil, delta)
	}
	report := func(key K, region support.Mask) bool {
		if len(work.changes.rows) != 0 && work.changes.rows[len(work.changes.rows)-1].key >= key {
			return false
		}
		work.changes.rows = append(work.changes.rows, mergeChange[K]{key: key, region: region})
		return true
	}
	if !binding.plane.domain.ReplaceUnder(first, second, split, &work.scratch, delta, report) {
		return carrier.ChangeHandle{}, false
	}
	factor, units, regions, ok := binding.expandChanges(&work.changes, delta, &work.expand)
	if !ok {
		return carrier.ChangeHandle{}, false
	}
	// right, not a zipper reconstruction, is the published successor root.
	return work.prepareChange(left, right, second, false, factor, units, regions, delta)
}

// rootStore belongs to one typed slot arena, not to Diagram.  The Binding's
// one composition-owned instance stores only its immutable attached initial
// root; every bindingWork creates another instance for its dynamic epoch.
// It assigns compact handles only after semantic.Domain accepted a sealed
// Plane. Closing that Work drops its store and RootEpoch membership together,
// so old dynamic States cannot pin a prior solve. IDs are process-local and
// never manifest data.
//
// Append has one publisher: the serialized rule-publication path for this
// Factor slot.  Readers are lock-free.  Each entry is fully initialized before
// its directory count becomes observable through an SC atomic store.
type rootStore[F ~uint64, K scalar.Key, V any] struct {
	domain    *semantic.Domain[F, K, V]
	directory atomic.Pointer[rootDirectory[F, K, V]]
}

// rootDirectory holds one page slot per reserved page span. Slots are atomic
// so a reservation can install a missing page inside the current span without
// replacing the directory: page installation and lock-free readers would
// otherwise race on the same slot.
type rootDirectory[F ~uint64, K scalar.Key, V any] struct {
	pages []atomic.Pointer[rootPage[F, K, V]]
	count atomic.Uint64
}

const rootsPerPage = 256

type rootPage[F ~uint64, K scalar.Key, V any] struct {
	entries [rootsPerPage]semantic.Plane[F, K, V]
}

func newRootStore[F ~uint64, K scalar.Key, V any](domain *semantic.Domain[F, K, V]) *rootStore[F, K, V] {
	if domain == nil {
		return nil
	}
	store := &rootStore[F, K, V]{domain: domain}
	store.directory.Store(&rootDirectory[F, K, V]{})
	return store
}

// rootReservation is a private, mutation-free preflight for one new root
// identity. It owns any replacement directory/page until Publish, so a later
// Factor failure can drop every reservation without retaining a root.
type rootReservation[F ~uint64, K scalar.Key, V any] struct {
	store *rootStore[F, K, V]
	base  *rootDirectory[F, K, V]
	next  *rootDirectory[F, K, V]
	page  *rootPage[F, K, V]
	// install marks a page the reservation owns for a slot the base directory
	// already spans; Publish stores it into that slot.
	install   bool
	pageIndex uint64
	offset    uint64
	id        uint64
	plane     semantic.Plane[F, K, V]
}

func (store *rootStore[F, K, V]) reserve(plane semantic.Plane[F, K, V]) (*rootReservation[F, K, V], bool) {
	if store == nil || store.domain == nil || !store.domain.Valid(plane) {
		return nil, false
	}
	directory := store.directory.Load()
	if directory == nil || directory.count.Load() >= previewRootLimit {
		return nil, false
	}
	id := directory.count.Load() + 1
	index := id - 1
	pageIndex, offset := index/rootsPerPage, index%rootsPerPage
	if pageIndex > uint64(^uint(0)>>1) {
		return nil, false
	}
	reservation := &rootReservation[F, K, V]{store: store, base: directory, pageIndex: pageIndex, offset: offset, id: id, plane: plane}
	if pageIndex < uint64(len(directory.pages)) {
		if page := directory.pages[pageIndex].Load(); page != nil {
			reservation.page = page
			return reservation, true
		}
		page := &rootPage[F, K, V]{}
		page.entries[offset] = plane
		reservation.page, reservation.install = page, true
		return reservation, true
	}
	capacity := len(directory.pages)
	if capacity == 0 {
		capacity = 1
	}
	for pageIndex >= uint64(capacity) {
		if capacity > int(^uint(0)>>1)/2 {
			return nil, false
		}
		capacity *= 2
	}
	next := &rootDirectory[F, K, V]{pages: make([]atomic.Pointer[rootPage[F, K, V]], capacity)}
	for slot := range directory.pages {
		next.pages[slot].Store(directory.pages[slot].Load())
	}
	page := &rootPage[F, K, V]{}
	page.entries[offset] = plane
	next.pages[pageIndex].Store(page)
	reservation.next, reservation.page = next, page
	return reservation, true
}

// Publish consumes a preflight reservation. It is total after reserve under
// rootStore's single-publisher contract: every fallible ownership/domain/
// overflow check happened before any store mutation.
func (reservation *rootReservation[F, K, V]) Publish() uint64 {
	if reservation == nil || reservation.store == nil || reservation.base == nil || reservation.page == nil || reservation.store.directory.Load() != reservation.base {
		panic("invalid root publication reservation")
	}
	if reservation.next != nil {
		reservation.next.count.Store(reservation.id)
		reservation.store.directory.Store(reservation.next)
		return reservation.id
	}
	if reservation.install {
		reservation.base.pages[reservation.pageIndex].Store(reservation.page)
	} else {
		reservation.page.entries[reservation.offset] = reservation.plane
	}
	reservation.base.count.Store(reservation.id)
	return reservation.id
}

// Plane resolves a compact local root identity without locks or payload
// boxing.  Count acquisition makes the corresponding immutable page entry
// visible; caller ownership is proved separately by Issuer.ResolveRoot.
func (store *rootStore[F, K, V]) Plane(id uint64) (semantic.Plane[F, K, V], bool) {
	if store == nil || store.domain == nil || id == 0 {
		return semantic.Plane[F, K, V]{}, false
	}
	directory := store.directory.Load()
	if directory == nil || id > directory.count.Load() {
		return semantic.Plane[F, K, V]{}, false
	}
	index := id - 1
	pageIndex, offset := index/rootsPerPage, index%rootsPerPage
	if pageIndex >= uint64(len(directory.pages)) {
		return semantic.Plane[F, K, V]{}, false
	}
	page := directory.pages[pageIndex].Load()
	if page == nil {
		return semantic.Plane[F, K, V]{}, false
	}
	plane := page.entries[offset]
	return plane, store.domain.Valid(plane)
}

// EpochRootValid is carrier's payload-free proof that an epoch-issued compact
// ID names one sealed Plane in this typed slot store.  It intentionally does
// not expose the Plane across the carrier boundary.
func (store *rootStore[F, K, V]) EpochRootValid(id uint64) bool {
	_, ok := store.Plane(id)
	return ok
}
