package factbinding

import (
	"sort"
	"sync/atomic"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/diagram"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/scalar"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/semantic"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
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

func (binding *Binding[K, V]) ValidSelector(selector carrier.Selector, kind carrier.SelectorKind) bool {
	if !binding.live() || selector.Kind() != kind {
		return false
	}
	_, ok := binding.selectors[selector]
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
	return binding.prepareRecurrenceScope(carrier.Widen, targets)
}

// PrepareNarrowing is the key-local counterpart of PrepareWidening. A Narrow
// selection is never widened to an entire Factor: only its declared target
// keys receive the typed descent operation.
func (binding *Binding[K, V]) PrepareNarrowing(targets []carrier.Target) (uint64, bool) {
	return binding.prepareRecurrenceScope(carrier.Narrow, targets)
}

func (binding *Binding[K, V]) prepareRecurrenceScope(kind carrier.MergeKind, targets []carrier.Target) (uint64, bool) {
	if binding == nil || len(targets) == 0 || (kind != carrier.Widen && kind != carrier.Narrow) {
		return 0, false
	}
	binding.lifecycle.Lock()
	defer binding.lifecycle.Unlock()
	if binding.scopeFrozen || !binding.live() || !binding.Supports(kind) {
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

// DeclaredSelector is the cold counterpart of ValidSelector.
func (binding *Binding[K, V]) DeclaredSelector(selector carrier.Selector, kind carrier.SelectorKind) bool {
	if binding == nil || !binding.prepared || selector.Kind() != kind {
		return false
	}
	_, ok := binding.selectors[selector]
	return ok
}

// DeclaredSelectorTargets returns the complete finite positional surface of
// one cold target selector. Index is the selector's candidate ordinal, so
// order and duplicate entries are preserved exactly. It is deliberately
// limited to opaque target capabilities rather than keys or values.
func (binding *Binding[K, V]) DeclaredSelectorTargets(selector carrier.Selector) ([]carrier.Target, bool) {
	if binding == nil || !binding.prepared || selector.Kind() != carrier.TargetSelector {
		return nil, false
	}
	descriptor, ok := binding.selectors[selector]
	if !ok || len(descriptor.candidatesTargets) == 0 {
		return nil, false
	}
	return append([]carrier.Target(nil), descriptor.candidatesTargets...), true
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
	return &bindingWork[K, V]{binding: binding, roots: roots, supportWork: supportWork, observations: observations}, true
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
	// roots and epoch are paired exactly once by carrier.Work opening.  The
	// former retains this Work's dynamic typed planes; the latter is the only
	// opaque handle provenance accepted for those planes.
	roots            *rootStore[planeFactor, K, V]
	epoch            *carrier.RootEpoch
	checkpoint       carrier.Checkpoint
	poll             func() bool
	observations     carrier.ObservationWork
	observationLive  bool
	generation       uint64
	nextObservation  uint64
	firstObservation uint64
	// Preview roots are Work-local just like ordinary dynamic roots.  A
	// Binding-wide table would let concurrent Works collide on compact preview
	// IDs and would retain an aborted epoch's typed planes.
	previewRoots   map[uint64]semantic.Plane[planeFactor, K, V]
	previewNext    uint64
	records        []observationRecord
	entries        []ObservationEntry[V]
	pieces         []observationPiece[V]
	partials       []observationGroup
	partialEntries []ObservationEntry[V]
	nextPartials   []observationGroup
	nextEntries    []ObservationEntry[V]
	buckets        map[uint64]int
	// scratch and changes belong to the enclosing evaluator's merge/compare
	// lifecycle, not to one observation generation. Every sole operation
	// begins with SoleScratch.prepare (which Clear's it); Merge3Under clears
	// changes on entry and exit. Observation close therefore clears only rows
	// and typed projection buffers, never an unrelated in-flight evaluator op.
	scratch diagram.SoleScratch[K, V]
	changes mergeChanges[K]
	// Coverage maps are evaluator scratch only. They resolve opaque authored
	// Targets beside this typed Binding for one fold and are cleared before
	// the fold returns; no coverage or fact authority is retained here.
	coverageLeft  map[K]support.Mask
	coverageRight map[K]support.Mask
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
	clear(work.coverageLeft)
	work.coverageLeft = nil
	clear(work.coverageRight)
	work.coverageRight = nil
	if work.supportWork != nil {
		work.supportWork.Close()
		work.supportWork = nil
	}
	work.clearObservationScratch()
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

func (work *bindingWork[K, V]) unionSupport(left, right support.Mask) (support.Mask, bool) {
	if !work.live() || work.supportWork == nil {
		return support.Mask{}, false
	}
	return support.UnionWithWork(work.supportWork, work.poll, left, right)
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
	generation uint64
	id         uint64
}

type observationResolver[V any] interface {
	observationCount(generation, id uint64) (int, bool)
	observationEntry(generation, id uint64, index int) (ObservationEntry[V], bool)
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

type observationGroup struct {
	first       int
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
		if !ok || !known || !work.binding.ValidTarget(row.Target()) || len(descriptor.keys) == 0 || !row.Region().Valid() || support.Empty(row.Region()) || !row.Region().Entails(within) {
			return nil, false
		}
		for _, key := range descriptor.keys {
			region := row.Region()
			if previous, present := result[key]; present {
				region, ok = work.unionSupport(previous, region)
				if !ok {
					return nil, false
				}
			}
			result[key] = region
		}
	}
	return result, true
}

// MergeContributionUnder resolves opaque target coverage only beside this
// typed Binding. For each concrete key, the effective left support is
// (left-state minus right authorship) union left authorship: right-only cells
// install the right terminal, covered overlap invokes Join, and uncovered
// right cells are fold identity. Sparse zero therefore means Default exactly
// when coverage says the producer authored that key/Guard.
func (work *bindingWork[K, V]) MergeContributionUnder(left, right carrier.RootHandle, leftSupport, rightSupport support.Mask, leftCoverage, rightCoverage carrier.SlotCoverage, delta *support.Work) (carrier.ChangeHandle, bool) {
	if !work.live() || work.binding == nil || delta == nil || !delta.Open() || !leftSupport.Valid() || !rightSupport.Valid() || leftSupport.Manager() != work.binding.plane.domain.Guards() || rightSupport.Manager() != work.binding.plane.domain.Guards() {
		return carrier.ChangeHandle{}, false
	}
	work.changes.reset()
	defer work.changes.reset()
	defer func() {
		clear(work.coverageLeft)
		clear(work.coverageRight)
	}()
	first, ok := work.resolve(left)
	if !ok {
		return carrier.ChangeHandle{}, false
	}
	second, ok := work.resolve(right)
	if !ok {
		return carrier.ChangeHandle{}, false
	}
	work.coverageLeft, ok = work.contributionCoverage(leftCoverage, leftSupport, work.coverageLeft)
	if !ok {
		return carrier.ChangeHandle{}, false
	}
	work.coverageRight, ok = work.contributionCoverage(rightCoverage, rightSupport, work.coverageRight)
	if !ok {
		return carrier.ChangeHandle{}, false
	}
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
		rightRegion, rightCovered := work.coverageRight[key]
		if !rightCovered {
			return leftSupport, empty, leftSupport, true
		}
		leftRegion, leftCovered := work.coverageLeft[key]
		if !leftCovered {
			leftRegion = empty
		}
		split, valid := work.threeSupport(leftSupport, rightRegion)
		if !valid {
			return support.Mask{}, support.Mask{}, support.Mask{}, false
		}
		effective, valid := work.unionSupport(split.LeftOnly(), leftRegion)
		if !valid {
			return support.Mask{}, support.Mask{}, support.Mask{}, false
		}
		return effective, rightRegion, leftSupport, true
	})
	if !ok {
		return carrier.ChangeHandle{}, false
	}
	nextSupport, ok := work.unionSupport(leftSupport, rightSupport)
	if !ok {
		return carrier.ChangeHandle{}, false
	}
	if work.binding.plane.domain.EqualUnder(first, next, nextSupport, &work.scratch) {
		if work.changes.Count() != 0 {
			return carrier.ChangeHandle{}, false
		}
		return work.prepareChange(left, left, next, false, support.Mask{}, nil, nil, delta)
	}
	factor, units, regions, ok := work.binding.expandChanges(&work.changes, delta)
	if !ok {
		return carrier.ChangeHandle{}, false
	}
	if work.binding.plane.domain.EqualUnder(second, next, nextSupport, &work.scratch) {
		return work.prepareChange(left, right, next, false, factor, units, regions, delta)
	}
	return work.prepareChange(left, carrier.RootHandle{}, next, true, factor, units, regions, delta)
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
	if !ok || len(descriptor.keys) == 0 {
		return false
	}
	if support.Empty(within) {
		return true
	}
	if len(descriptor.keys) == 1 {
		return work.observeExact(input, root, unit, descriptor.keys[0], within, visit)
	}
	return work.observeSummary(input, root, unit, descriptor.keys, within, visit)
}

func (work *bindingWork[K, V]) resetObservationScratch(generation uint64) {
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
	clear(work.partialEntries)
	work.partialEntries = work.partialEntries[:0]
	clear(work.nextPartials)
	work.nextPartials = work.nextPartials[:0]
	clear(work.nextEntries)
	work.nextEntries = work.nextEntries[:0]
	clear(work.buckets)
}

func (work *bindingWork[K, V]) observeExact(input semantic.Plane[planeFactor, K, V], root carrier.RootHandle, unit carrier.Unit, key K, within support.Mask, visit func(carrier.ObservationRow) bool) bool {
	if !work.partitionKey(input, key, within) {
		return false
	}
	if len(work.pieces) == 0 {
		return false
	}
	// A sole terminal cell already is the complete exact cover. It has no
	// equal sibling to coalesce, so avoid opening a disposable guard candidate
	// transaction or building summary buckets on this hot one-key path.
	if len(work.pieces) == 1 {
		return work.emitExactPiece(root, unit, work.pieces[0], visit)
	}
	// Distinct exact entries require no union construction: the FDD partition
	// already supplied their disjoint sealed cells in canonical low/high order.
	// Retain the coalescing path only when equal entries really need a union.
	if work.exactPiecesDistinct() {
		for _, piece := range work.pieces {
			if !work.emitExactPiece(root, unit, piece, visit) {
				return false
			}
		}
		return true
	}
	unions := work.newSupportWork()
	if unions == nil || !work.seedGroups(unions) || !unions.Seal() {
		if unions != nil {
			unions.Discard()
		}
		return false
	}
	return work.emitGroups(root, unit, visit)
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

func (work *bindingWork[K, V]) observeSummary(input semantic.Plane[planeFactor, K, V], root carrier.RootHandle, unit carrier.Unit, keys []K, within support.Mask, visit func(carrier.ObservationRow) bool) bool {
	if len(keys) == 0 {
		return false
	}
	// A declared summary of constant (including absent) keys has exactly one
	// raw ordered sequence for all of within. It needs neither pairwise
	// intersections nor a candidate support transaction; retain that work for
	// the first genuinely branched declared key.
	clear(work.partialEntries)
	work.partialEntries = work.partialEntries[:0]
	constant := true
	for _, key := range keys {
		if !work.live() {
			return false
		}
		if !work.partitionKey(input, key, within) {
			return false
		}
		if len(work.pieces) != 1 {
			constant = false
			break
		}
		work.partialEntries = append(work.partialEntries, work.pieces[0].entry)
	}
	if constant {
		clear(work.partials)
		work.partials = work.partials[:0]
		work.partials = append(work.partials, observationGroup{first: 0, count: len(work.partialEntries), region: within, previous: -1})
		return work.emitGroups(root, unit, visit)
	}
	// The preliminary constant probe intentionally avoids a second semantic
	// authority. On a branched summary, restart the existing exact grouping
	// algorithm from its first declared key rather than combining a partial
	// prefix with a different traversal state.
	clear(work.partialEntries)
	work.partialEntries = work.partialEntries[:0]
	unions := work.newSupportWork()
	if unions == nil || !work.partitionKey(input, keys[0], within) || !work.seedGroups(unions) {
		if unions != nil {
			unions.Discard()
		}
		return false
	}
	for _, key := range keys[1:] {
		if !work.live() {
			unions.Discard()
			return false
		}
		if !work.partitionKey(input, key, within) || !work.extendGroups(unions) {
			unions.Discard()
			return false
		}
	}
	if !unions.Seal() {
		unions.Discard()
		return false
	}
	return work.emitGroups(root, unit, visit)
}

// partitionKey is the only read of a declared typed key. semantic.PartitionKey
// returns its exact terminal/presence pieces in one traversal, avoiding the
// old PartitionValue followed by SummaryUnder retraversal.
func (work *bindingWork[K, V]) partitionKey(input semantic.Plane[planeFactor, K, V], key K, within support.Mask) bool {
	work.pieces = work.pieces[:0]
	return work.binding.plane.domain.PartitionKey(input, key, within, func(value V, present bool, region support.Mask) bool {
		entry := ObservationEntry[V]{value: value, present: present}
		work.pieces = append(work.pieces, observationPiece[V]{entry: entry, fingerprint: work.entryFingerprint(entry), region: region})
		return true
	})
}

func (work *bindingWork[K, V]) seedGroups(unions *support.Work) bool {
	if unions == nil || len(work.pieces) == 0 {
		return false
	}
	work.partials = work.partials[:0]
	work.partialEntries = work.partialEntries[:0]
	work.clearBuckets()
	empty := unions.False()
	for _, piece := range work.pieces {
		if !work.live() {
			return false
		}
		region, ok := unions.Or(empty, piece.region)
		if !ok || !work.addGroup(&work.partials, &work.partialEntries, nil, piece.entry, foldObservationFingerprint(observationFingerprintSeed, piece.fingerprint), region, unions) {
			return false
		}
	}
	return len(work.partials) != 0
}

func (work *bindingWork[K, V]) extendGroups(unions *support.Work) bool {
	if unions == nil || len(work.partials) == 0 || len(work.pieces) == 0 {
		return false
	}
	work.nextPartials = work.nextPartials[:0]
	work.nextEntries = work.nextEntries[:0]
	work.clearBuckets()
	for _, prefix := range work.partials {
		if !work.live() {
			return false
		}
		entries := work.partialEntries[prefix.first : prefix.first+prefix.count]
		for _, piece := range work.pieces {
			if !work.live() {
				return false
			}
			region, ok := unions.And(prefix.region, piece.region)
			if !ok {
				return false
			}
			view, ok := unions.Decompose(region)
			if !ok {
				return false
			}
			if view.Terminal && !view.Value {
				continue
			}
			if !work.addGroup(&work.nextPartials, &work.nextEntries, entries, piece.entry, foldObservationFingerprint(prefix.fingerprint, piece.fingerprint), region, unions) {
				return false
			}
		}
	}
	work.partials, work.nextPartials = work.nextPartials, work.partials
	work.partialEntries, work.nextEntries = work.nextEntries, work.partialEntries
	return len(work.partials) != 0
}

// addGroup coalesces every semantically equal complete or partial sequence,
// not merely adjacent symbolic cells. Bucket lookup is an optimization only:
// Factor Equal and the stored/absent bit decide equality. groups remains the
// deterministic first-discovery order used for final emission.
func (work *bindingWork[K, V]) addGroup(groups *[]observationGroup, entries *[]ObservationEntry[V], prefix []ObservationEntry[V], tail ObservationEntry[V], fingerprint uint64, region support.Mask, unions *support.Work) bool {
	if groups == nil || entries == nil || unions == nil {
		return false
	}
	for index, found := work.buckets[fingerprint]; found && index >= 0; {
		candidate := &(*groups)[index]
		if candidate.count != len(prefix)+1 || !work.sequenceEqual((*entries)[candidate.first:candidate.first+candidate.count], prefix, tail) {
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
	start := len(*entries)
	*entries = append(*entries, prefix...)
	*entries = append(*entries, tail)
	previous := -1
	if last, found := work.buckets[fingerprint]; found {
		previous = last
	}
	*groups = append(*groups, observationGroup{first: start, count: len(prefix) + 1, fingerprint: fingerprint, region: region, previous: previous})
	work.buckets[fingerprint] = len(*groups) - 1
	return true
}

func (work *bindingWork[K, V]) sequenceEqual(stored, prefix []ObservationEntry[V], tail ObservationEntry[V]) bool {
	if len(stored) != len(prefix)+1 {
		return false
	}
	for index, entry := range prefix {
		if !work.entryEqual(stored[index], entry) {
			return false
		}
	}
	return work.entryEqual(stored[len(stored)-1], tail)
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

func (work *bindingWork[K, V]) emitGroups(root carrier.RootHandle, unit carrier.Unit, visit func(carrier.ObservationRow) bool) bool {
	if !work.observationLive || len(work.partials) == 0 {
		return false
	}
	for _, group := range work.partials {
		if !work.live() {
			return false
		}
		if work.nextObservation == ^uint64(0) || !group.region.Valid() {
			return false
		}
		first := len(work.entries)
		work.entries = append(work.entries, work.partialEntries[group.first:group.first+group.count]...)
		work.nextObservation++
		id := work.nextObservation
		handle, ok := work.binding.issuer.IssueObservation(work.observations, work.generation, id)
		if !ok {
			return false
		}
		row, ok := work.observations.Row(handle, group.region)
		if !ok {
			return false
		}
		work.records = append(work.records, observationRecord{root: root, unit: unit, region: group.region, first: first, count: group.count})
		if !visit(row) {
			return false
		}
	}
	return true
}

func (work *bindingWork[K, V]) emitExactPiece(root carrier.RootHandle, unit carrier.Unit, piece observationPiece[V], visit func(carrier.ObservationRow) bool) bool {
	if !work.observationLive || work.nextObservation == ^uint64(0) || !piece.region.Valid() {
		return false
	}
	first := len(work.entries)
	work.entries = append(work.entries, piece.entry)
	work.nextObservation++
	id := work.nextObservation
	handle, ok := work.binding.issuer.IssueObservation(work.observations, work.generation, id)
	if !ok {
		return false
	}
	row, ok := work.observations.Row(handle, piece.region)
	if !ok {
		return false
	}
	work.records = append(work.records, observationRecord{root: root, unit: unit, region: piece.region, first: first, count: 1})
	return visit(row)
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

func (work *bindingWork[K, V]) record(generation, id uint64) (observationRecord, bool) {
	if work == nil || !work.observationLive || generation == 0 || generation != work.generation || id < work.firstObservation {
		return observationRecord{}, false
	}
	index := id - work.firstObservation
	if index >= uint64(len(work.records)) {
		return observationRecord{}, false
	}
	return work.records[index], true
}

func (work *bindingWork[K, V]) observationCount(generation, id uint64) (int, bool) {
	record, ok := work.record(generation, id)
	return record.count, ok
}

func (work *bindingWork[K, V]) observationEntry(generation, id uint64, index int) (ObservationEntry[V], bool) {
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
		factor, units, regions, ok = binding.expandChanges(&work.changes, delta)
		if !ok {
			return carrier.ChangeHandle{}, false
		}
	}
	if binding.plane.domain.EqualUnder(second, next, within, &work.scratch) {
		return work.prepareChange(left, right, next, false, factor, units, regions, delta)
	}
	return work.prepareChange(left, carrier.RootHandle{}, next, true, factor, units, regions, delta)
}

// MergeSelectedUnder keeps recurrence local to the selected widening keys.
// The selected operand is used only at those keys; all remaining keys install
// the exact-right plane. The final diff is then derived directly from current
// to that mixed plane, so the carrier receives one change proof rather than a
// sequence of selected and exact-right publications.
func (work *bindingWork[K, V]) MergeSelectedUnder(kind carrier.MergeKind, scope uint64, current, selectedRight, exactRight carrier.RootHandle, selectedSplit, exactSplit support.Split, delta *support.Work) (carrier.ChangeHandle, bool) {
	if !work.live() || delta == nil || !delta.Open() || (kind != carrier.Widen && kind != carrier.Narrow) {
		return carrier.ChangeHandle{}, false
	}
	work.changes.reset()
	defer work.changes.reset()
	binding := work.binding
	if binding == nil {
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
	switch kind {
	case carrier.Widen:
		if scope == 0 {
			return carrier.ChangeHandle{}, false
		}
		chosen, valid := binding.widenScope(scope)
		if !valid {
			return carrier.ChangeHandle{}, false
		}
		widened, valid := binding.plane.domain.WidenUnderKeys(left, selectedPlane, selectedSplit, &work.scratch, nil, nil, chosen.contains)
		if !valid {
			return carrier.ChangeHandle{}, false
		}
		mixedSplit, valid := work.threeSupport(selectedSplit.Right(), exactSplit.Right())
		if !valid {
			return carrier.ChangeHandle{}, false
		}
		next, valid = binding.plane.domain.SelectUnderKeys(widened, exactPlane, mixedSplit, &work.scratch, nil, nil, chosen.contains)
		if !valid {
			return carrier.ChangeHandle{}, false
		}
	case carrier.Narrow:
		if scope == 0 {
			return carrier.ChangeHandle{}, false
		}
		chosen, valid := binding.narrowScope(scope)
		if !valid {
			return carrier.ChangeHandle{}, false
		}
		narrowed, valid := binding.plane.domain.NarrowUnderKeys(left, selectedPlane, selectedSplit, &work.scratch, nil, nil, chosen.contains)
		if !valid {
			return carrier.ChangeHandle{}, false
		}
		mixedSplit, valid := work.threeSupport(selectedSplit.Right(), exactSplit.Right())
		if !valid {
			return carrier.ChangeHandle{}, false
		}
		next, valid = binding.plane.domain.SelectUnderKeys(narrowed, exactPlane, mixedSplit, &work.scratch, nil, nil, chosen.contains)
		if !valid {
			return carrier.ChangeHandle{}, false
		}
	}
	report := func(key K, region support.Mask) bool {
		if len(work.changes.rows) != 0 && work.changes.rows[len(work.changes.rows)-1].key >= key {
			return false
		}
		work.changes.rows = append(work.changes.rows, mergeChange[K]{key: key, region: region})
		return true
	}
	if !binding.plane.domain.ReplaceUnder(left, next, exactSplit, &work.scratch, delta, report) {
		return carrier.ChangeHandle{}, false
	}
	factor, units, regions, ok := binding.expandChanges(&work.changes, delta)
	if !ok {
		return carrier.ChangeHandle{}, false
	}
	return work.prepareChange(current, carrier.RootHandle{}, next, true, factor, units, regions, delta)
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
	factor, units, regions, ok := binding.expandChanges(&work.changes, delta)
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

type rootDirectory[F ~uint64, K scalar.Key, V any] struct {
	pages []*rootPage[F, K, V]
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
	store     *rootStore[F, K, V]
	base      *rootDirectory[F, K, V]
	next      *rootDirectory[F, K, V]
	page      *rootPage[F, K, V]
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
	if pageIndex >= uint64(len(directory.pages)) || directory.pages[pageIndex] == nil {
		capacity := len(directory.pages)
		if capacity == 0 {
			capacity = 1
		} else {
			if capacity > int(^uint(0)>>1)/2 {
				return nil, false
			}
			capacity *= 2
		}
		for pageIndex >= uint64(capacity) {
			if capacity > int(^uint(0)>>1)/2 {
				return nil, false
			}
			capacity *= 2
		}
		next := &rootDirectory[F, K, V]{pages: make([]*rootPage[F, K, V], capacity)}
		copy(next.pages, directory.pages)
		next.pages[pageIndex] = &rootPage[F, K, V]{}
		next.pages[pageIndex].entries[offset] = plane
		reservation.next, reservation.page = next, next.pages[pageIndex]
		return reservation, true
	}
	reservation.page = directory.pages[pageIndex]
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
	} else {
		reservation.page.entries[reservation.offset] = reservation.plane
		reservation.base.count.Store(reservation.id)
	}
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
	if pageIndex >= uint64(len(directory.pages)) || directory.pages[pageIndex] == nil {
		return semantic.Plane[F, K, V]{}, false
	}
	plane := directory.pages[pageIndex].entries[offset]
	return plane, store.domain.Valid(plane)
}

// EpochRootValid is carrier's payload-free proof that an epoch-issued compact
// ID names one sealed Plane in this typed slot store.  It intentionally does
// not expose the Plane across the carrier boundary.
func (store *rootStore[F, K, V]) EpochRootValid(id uint64) bool {
	_, ok := store.Plane(id)
	return ok
}
