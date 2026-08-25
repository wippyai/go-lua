package factbinding

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/scalar"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/stage"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
)

// Patch is one write-only typed candidate over exactly one immutable carrier
// predecessor and physical slot. The captured State support is the
// only region in which this patch can write.  A staged root consequently
// cannot be reattached to a wider, different, or later predecessor.
type Patch[K scalar.Key, V any] struct {
	binding  *Binding[K, V]
	state    carrier.State
	slot     shape.Slot
	support  support.Mask
	base     carrier.RootHandle
	patch    *stage.Patch[planeFactor, K, V]
	authored []carrier.TargetRegion
	// owned marks a Patch that lives in caller-owned scratch. Closing one keeps
	// its authored storage for the next transaction instead of dropping it.
	owned bool
	// changesScratch is Accept's reusable holder for the candidate KeyChanges
	// it hands to expandChanges. Passing its address keeps the keyChanges
	// interface value pointing at scratch already owned by this Patch instead
	// of boxing a fresh copy of the value handed back by stage.Patch.Accept.
	changesScratch stage.KeyChanges[K]
	expandScratch  expandScratch[K]
}

// TransformClosure is one Binding-owned immutable typed key vector compiled
// from a Rule's finite carried-target closure.  It cannot be fabricated with
// raw keys outside factbinding and is valid only for its issuing Binding.
type TransformClosure[K scalar.Key, V any] struct {
	binding *Binding[K, V]
	keys    []K
	targets []carrier.Target
}

// TransformClosure resolves and de-duplicates the sealed target closure once
// at binding time.  It neither observes a State nor creates a second carry
// representation.  An empty closure is valid for a Factor with no reachable
// observable coordinates.
func (binding *Binding[K, V]) TransformClosure(targets []carrier.Target) (TransformClosure[K, V], bool) {
	if binding == nil || !binding.live() {
		return TransformClosure[K, V]{}, false
	}
	seen := make(map[K]struct{})
	ordered := append([]carrier.Target(nil), targets...)
	sort.Slice(ordered, func(left, right int) bool { return ordered[left].Less(ordered[right]) })
	ordered = uniqueTargets(ordered)
	for _, target := range ordered {
		descriptor, present := binding.targets[target]
		if !present || !binding.ValidTarget(target) || len(descriptor.keys) == 0 {
			return TransformClosure[K, V]{}, false
		}
		for _, key := range descriptor.keys {
			seen[key] = struct{}{}
		}
	}
	keys := make([]K, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(left, right int) bool { return keys[left] < keys[right] })
	return TransformClosure[K, V]{binding: binding, keys: keys, targets: ordered}, true
}

// TargetTransformClosure is the closure over exactly one declared target. It
// is the singleton case of TransformClosure, answered from the key vector and
// the singleton the target already sealed, so a caller that needs one closure
// per published row - a routed carry - allocates nothing per row.
func (binding *Binding[K, V]) TargetTransformClosure(target carrier.Target) (TransformClosure[K, V], bool) {
	if binding == nil || !binding.live() {
		return TransformClosure[K, V]{}, false
	}
	descriptor, present := binding.targets[target]
	if !present || len(descriptor.keys) == 0 || len(descriptor.self) != 1 || descriptor.self[0] != target {
		return TransformClosure[K, V]{}, false
	}
	return TransformClosure[K, V]{binding: binding, keys: descriptor.keys, targets: descriptor.self}, true
}

func uniqueTargets(targets []carrier.Target) []carrier.Target {
	if len(targets) == 0 {
		return nil
	}
	write := 1
	for _, target := range targets[1:] {
		if !targets[write-1].Same(target) {
			targets[write] = target
			write++
		}
	}
	clear(targets[write:])
	return targets[:write]
}

// Begin opens the only candidate write scope for one exact sealed carrier
// predecessor at this Binding's attached physical slot. The caller supplies
// the evaluator which owns that predecessor, so admission stays O(1) instead
// of scanning every Factor root through State.Valid. There is deliberately no raw-root overload:
// a root alone has no support authority and cannot identify a publication
// predecessor.  Read projection is compiled and evaluated separately from
// this write transaction.
func (binding *Binding[K, V]) Begin(work *carrier.Work, state carrier.State) *Patch[K, V] {
	if binding == nil || work == nil || !work.OwnsState(state) {
		return nil
	}
	slot, ok := binding.issuer.Slot()
	if !ok {
		return nil
	}
	slotWork, ok := work.SlotWork(slot)
	typedWork, typed := slotWork.(*bindingWork[K, V])
	if !ok || !typed || typedWork.binding != binding || !typedWork.live() {
		return nil
	}
	base, ok := state.HandleAt(slot)
	if !ok {
		return nil
	}
	typedPlane, ok := typedWork.resolve(base)
	if !ok {
		return nil
	}
	within := state.Support()
	if !within.Valid() || within.Manager() != binding.plane.domain.Guards() {
		return nil
	}
	patch := stage.Begin(binding.plane.diagram, typedPlane.Root(), within, binding.stageConfig)
	if patch == nil {
		return nil
	}
	return &Patch[K, V]{binding: binding, state: state, slot: slot, support: within, base: base, patch: patch}
}

// PatchScratch is caller-owned storage for one write transaction. A worker that
// stages once per invocation keeps one of these for its whole lifetime, so the
// candidate page, the candidate FDD and this wrapper are grown once instead of
// per write. It carries no authority of its own: every admission Begin makes is
// made here too, and the returned Patch is closed by Accept or Discard exactly
// as an allocated one is.
type PatchScratch[K scalar.Key, V any] struct {
	patch Patch[K, V]
	stage stage.Patch[planeFactor, K, V]
}

// BeginInto opens the same write scope over caller-owned scratch.
func (binding *Binding[K, V]) BeginInto(scratch *PatchScratch[K, V], work *carrier.Work, state carrier.State) *Patch[K, V] {
	if binding == nil || scratch == nil || scratch.patch.patch != nil || work == nil || !work.OwnsState(state) {
		return nil
	}
	slot, ok := binding.issuer.Slot()
	if !ok {
		return nil
	}
	slotWork, ok := work.SlotWork(slot)
	typedWork, typed := slotWork.(*bindingWork[K, V])
	if !ok || !typed || typedWork.binding != binding || !typedWork.live() {
		return nil
	}
	base, ok := state.HandleAt(slot)
	if !ok {
		return nil
	}
	typedPlane, ok := typedWork.resolve(base)
	if !ok {
		return nil
	}
	within := state.Support()
	if !within.Valid() || within.Manager() != binding.plane.domain.Guards() {
		return nil
	}
	if !stage.BeginInto(&scratch.stage, binding.plane.diagram, typedPlane.Root(), within, binding.stageConfig) {
		return nil
	}
	scratch.patch.binding, scratch.patch.state = binding, state
	scratch.patch.slot, scratch.patch.support, scratch.patch.base = slot, within, base
	scratch.patch.patch = &scratch.stage
	scratch.patch.owned = true
	scratch.patch.authored = scratch.patch.authored[:0]
	return &scratch.patch
}

// Write is the only factbinding mutation entry.  Target is a presealed
// Binding capability: strong updates have exact singleton authority, while a
// weak target joins only the finite typed surface frozen at declaration time.
func (patch *Patch[K, V]) Write(target carrier.Target, when support.Mask, value V) bool {
	return patch.write(target, when, value, false)
}

// WriteRouted is the routed half of the same mutation. A routed output names
// a selected read on its own Factor and reduces the cell that read observed,
// so the staged value is the complete value of that coordinate after the
// operation rather than one more term reaching it. The authored row therefore
// states a displacement, and the point fold gives it the predecessor region
// it read instead of joining the two.
func (patch *Patch[K, V]) WriteRouted(target carrier.Target, when support.Mask, value V) bool {
	if target.Mode() != carrier.StrongTarget {
		return false
	}
	return patch.write(target, when, value, true)
}

func (patch *Patch[K, V]) write(target carrier.Target, when support.Mask, value V, routed bool) bool {
	if patch == nil || patch.binding == nil || patch.patch == nil || !when.Valid() || support.Empty(when) || !when.Entails(patch.support) {
		return false
	}
	descriptor, ok := patch.binding.targets[target]
	if !ok || !patch.binding.ValidTarget(target) || len(descriptor.keys) == 0 {
		return false
	}
	if target.Mode() == carrier.StrongTarget {
		if len(descriptor.keys) != 1 || len(descriptor.units) != 1 || descriptor.units[0].Kind() != carrier.ExactUnit || !patch.patch.Set(descriptor.keys[0], when, value) {
			return false
		}
	} else if target.Mode() == carrier.WeakTarget {
		if !patch.patch.WeakJoinMany(descriptor.keys, when, value) {
			return false
		}
	} else {
		return false
	}
	if routed {
		patch.authored = append(patch.authored, carrier.NewDisplacementRegion(target, when))
		return true
	}
	patch.authored = append(patch.authored, carrier.NewTargetRegion(target, when))
	return true
}

// Transform applies one owner-issued carry map over this patch's precompiled
// carried closure. The stage patch enforces default preservation, admission,
// and Join-stability for each reachable terminal. It runs
// before ordinary row writes, against the same immutable predecessor and the
// same guard, so Accept exposes one atomic net ChangeSet.
func (patch *Patch[K, V]) Transform(closure TransformClosure[K, V], source carrier.SlotCoverage, when support.Mask, apply func(V) (V, bool)) bool {
	if patch == nil || patch.binding == nil || patch.patch == nil || closure.binding != patch.binding || !when.Valid() || support.Empty(when) || !when.Entails(patch.support) || apply == nil {
		return false
	}
	if !patch.validateTransformSource(closure, TransformClosure[K, V]{}, source, when) || !patch.patch.Transform(closure.keys, when, apply) {
		return false
	}
	return patch.appendTransformSource(closure, TransformClosure[K, V]{}, source, when)
}

// TransformClosures applies one owner map over one authored closure plus one
// Binding-owned route closure. The closures may share targets; their sorted
// key vectors are merged by the stage Patch without allocating a route-sized
// union, so a shared route coordinate is transformed exactly once. The route
// closure itself remains Binding-owned and can therefore be shared by every
// Rule member. This private engine cut deliberately accepts at most two
// closures: authored static carry plus the one Factor route closure.
func (patch *Patch[K, V]) TransformClosures(closures []TransformClosure[K, V], source carrier.SlotCoverage, when support.Mask, apply func(V) (V, bool)) bool {
	if patch == nil || patch.binding == nil || patch.patch == nil || !when.Valid() || support.Empty(when) || !when.Entails(patch.support) || apply == nil || len(closures) == 0 {
		return false
	}
	if len(closures) > 2 {
		return false
	}
	for _, closure := range closures {
		if closure.binding != patch.binding {
			return false
		}
	}
	var left, right TransformClosure[K, V]
	left = closures[0]
	if len(closures) == 2 {
		right = closures[1]
	}
	if !patch.validateTransformSource(left, right, source, when) || !patch.patch.TransformSortedUnion(left.keys, right.keys, when, apply) {
		return false
	}
	return patch.appendTransformSource(left, right, source, when)
}

// validateTransformSource authenticates every canonical source row and region
// against the Patch's guard manager. It performs no allocation. A source row
// may legitimately belong to another member's closure; appendTransformSource
// projects such a row out after stage while retaining this full validation.
func (patch *Patch[K, V]) validateTransformSource(left, right TransformClosure[K, V], source carrier.SlotCoverage, when support.Mask) bool {
	if patch == nil || patch.binding == nil || !when.Valid() || support.Empty(when) || left.binding != patch.binding {
		return false
	}
	if right.binding != nil && right.binding != patch.binding {
		return false
	}
	for index := 0; index < source.Count(); index++ {
		row, ok := source.At(index)
		if !ok {
			return false
		}
		if !row.Region().Valid() || row.Region().Manager() != when.Manager() {
			return false
		}
	}
	return true
}

// appendTransformSource copies only source-present coverage after stage has
// transformed the semantic root. Regions are clipped to when and empty clips
// are omitted, so Absent remains no authored row while explicit Default keeps
// its sparse authored row.
func (patch *Patch[K, V]) appendTransformSource(left, right TransformClosure[K, V], source carrier.SlotCoverage, when support.Mask) bool {
	for index := 0; index < source.Count(); index++ {
		row, ok := source.At(index)
		if !ok {
			return false
		}
		if !containsTransformTarget(left, right, row.Target()) {
			continue
		}
		region, ok := support.Intersect(row.Region(), when)
		if !ok {
			return false
		}
		if support.Empty(region) {
			continue
		}
		patch.authored = append(patch.authored, row.WithRegion(region).WithLineage(row.Lineage(), carrier.CoverageEffect))
	}
	return true
}

func containsTransformTarget[K scalar.Key, V any](left, right TransformClosure[K, V], target carrier.Target) bool {
	index := sort.Search(len(left.targets), func(index int) bool {
		return !left.targets[index].Less(target)
	})
	if index < len(left.targets) && left.targets[index].Same(target) {
		return true
	}
	if right.binding == nil {
		return false
	}
	index = sort.Search(len(right.targets), func(index int) bool {
		return !right.targets[index].Less(target)
	})
	if index < len(right.targets) && right.targets[index].Same(target) {
		return true
	}
	return false
}

// WriteJoined performs one strong routed write after reducing every value for
// that exact target through the owning Factor's admitted Join. It is used by
// a route-output batch when distinct semantic routes resolve to one physical
// coordinate. The target is still a presealed singleton capability; this
// method does not turn a may-alias surface into a generic strong update. The
// authored row is a displacement for the same reason WriteRouted's is: the
// joined value is the complete value of one routed coordinate.
func (patch *Patch[K, V]) WriteJoined(target carrier.Target, when support.Mask, values []V) bool {
	if patch == nil || patch.binding == nil || patch.patch == nil || !when.Valid() || support.Empty(when) || !when.Entails(patch.support) || len(values) == 0 {
		return false
	}
	descriptor, ok := patch.binding.targets[target]
	if !ok || !patch.binding.ValidTarget(target) || target.Mode() != carrier.StrongTarget || len(descriptor.keys) != 1 || len(descriptor.units) != 1 || descriptor.units[0].Kind() != carrier.ExactUnit {
		return false
	}
	joined := values[0]
	for _, value := range values[1:] {
		joined = patch.binding.algebra.join(joined, value)
	}
	if !patch.patch.Set(descriptor.keys[0], when, joined) {
		return false
	}
	patch.authored = append(patch.authored, carrier.NewDisplacementRegion(target, when))
	return true
}

// Accept seals the candidate once and returns the sole predecessor-bound
// publication. The typed Binding expands operation-produced KeyChanges
// through its immutable exact+summary reverse closure, then its attached
// Issuer couples those canonical regions to the before/after roots.
func (patch *Patch[K, V]) Accept(work *carrier.Work) (result carrier.Patch, accepted bool) {
	if patch == nil {
		return carrier.Patch{}, false
	}
	staged := patch.patch
	defer patch.clear()
	if staged == nil {
		return carrier.Patch{}, false
	}
	// Until carrier returns an accepted Patch, this wrapper remains the sole
	// owner of the stage candidate. Discard is idempotent after stage.Accept
	// has already closed its own transaction.
	defer func() {
		if !accepted {
			_ = staged.Discard()
		}
	}()
	if work == nil || patch.binding == nil {
		return carrier.Patch{}, false
	}
	binding, state, base := patch.binding, patch.state, patch.base
	// The carrier canonicalizes and keeps its own copy of the authored rows, so
	// this boundary hands over the staged rows directly. They are still owned
	// here until Accept returns, and clear() runs after it.
	authored := patch.authored
	slotWork, slotOK := work.SlotWork(patch.slot)
	currentWork, currentOK := slotWork.(*bindingWork[K, V])
	if !slotOK || !currentOK || !currentWork.live() || !work.OwnsState(state) {
		return carrier.Patch{}, false
	}
	// A staged patch has no root-store identity yet.  It may cross to another
	// Work over the same live immutable predecessor, but only that target
	// Work's bindingWork may publish its output root.  Re-resolving base here
	// rejects a source epoch that closed after staging.
	if _, ok := currentWork.resolve(base); !ok {
		return carrier.Patch{}, false
	}
	var factor support.Mask
	var units []carrier.Unit
	var regions []support.Mask
	changeCount := 0
	root, ok := staged.Accept(func(changes stage.KeyChanges[K], work *support.Work) bool {
		changeCount = changes.Count()
		patch.changesScratch = changes
		var expanded bool
		factor, units, regions, expanded = binding.expandChanges(&patch.changesScratch, work, &patch.expandScratch)
		return expanded
	})
	if !ok {
		return carrier.Patch{}, false
	}
	plane, okay := binding.plane.domain.Plane(root)
	if !okay {
		return carrier.Patch{}, false
	}
	change, okay := currentWork.prepareChange(base, base, plane, changeCount != 0, factor, units, regions, nil)
	if !okay {
		return carrier.Patch{}, false
	}
	if len(authored) == 0 {
		result, accepted = work.Accept(state, change)
	} else {
		result, accepted = work.AcceptAuthoredRows(state, change, authored)
	}
	if !accepted {
		_ = work.DiscardChange(change)
	}
	return result, accepted
}

type keyChanges[K scalar.Key] interface {
	Count() int
	At(int) (K, support.Mask, bool)
}

// expandScratch is expandChanges' reusable working storage. Its owner is
// whichever per-invocation object already reuses the paired keyChanges (a
// Patch or a bindingWork), so a repeated write grows these buffers once and
// resets them per call instead of allocating a fresh heap and output vectors
// on every accepted change.
type expandScratch[K scalar.Key] struct {
	heap    []changeCursor
	units   []carrier.Unit
	regions []support.Mask
}

func (binding *Binding[K, V]) expandChanges(changes keyChanges[K], unions *support.Work, scratch *expandScratch[K]) (support.Mask, []carrier.Unit, []support.Mask, bool) {
	if !binding.live() {
		return support.Mask{}, nil, nil, false
	}
	if changes.Count() == 0 {
		return support.Mask{}, nil, nil, true
	}
	if unions == nil || !unions.Open() {
		return support.Mask{}, nil, nil, false
	}

	// The Factor region is the semantic root delta. It is deliberately
	// computed before consulting the reverse Unit table: a Factor can change
	// at a key with no typed-read subscriber, but whole-Factor Carry must
	// still be invalidated. Unit rows remain only exact typed-read evidence.
	factor := unions.False()
	if !factor.Valid() && !unions.Valid(factor) {
		return support.Mask{}, nil, nil, false
	}
	for index := 0; index < changes.Count(); index++ {
		_, region, ok := changes.At(index)
		view, regionOK := unions.Decompose(region)
		if !ok || !regionOK || view.Terminal && !view.Value {
			return support.Mask{}, nil, nil, false
		}
		var joined bool
		factor, joined = unions.Or(factor, region)
		if !joined {
			return support.Mask{}, nil, nil, false
		}
	}

	heap := scratch.heap[:0]
	for index := 0; index < changes.Count(); index++ {
		key, region, ok := changes.At(index)
		closure := binding.reverse[key]
		if !ok {
			return support.Mask{}, nil, nil, false
		}
		if len(closure) == 0 {
			continue
		}
		heap = append(heap, changeCursor{units: closure, region: region})
	}
	scratch.heap = heap
	if len(heap) == 0 {
		return factor, nil, nil, true
	}
	changeHeapify(heap)
	units := scratch.units[:0]
	regions := scratch.regions[:0]
	for len(heap) != 0 {
		cursor := changeHeapPop(&heap)
		unit, region := cursor.units[cursor.index], cursor.region
		cursor.index++
		if cursor.index < len(cursor.units) {
			changeHeapPush(&heap, cursor)
		}
		for len(heap) != 0 && heap[0].units[heap[0].index].Same(unit) {
			duplicate := changeHeapPop(&heap)
			joined, valid := unions.Or(region, duplicate.region)
			if !valid {
				scratch.heap, scratch.units, scratch.regions = heap, units, regions
				return support.Mask{}, nil, nil, false
			}
			region = joined
			duplicate.index++
			if duplicate.index < len(duplicate.units) {
				changeHeapPush(&heap, duplicate)
			}
		}
		units = append(units, unit)
		regions = append(regions, region)
	}
	scratch.heap, scratch.units, scratch.regions = heap, units, regions
	return factor, units, regions, true
}

// changeCursor is one already-canonical reverse-closure stream. The small
// binary heap performs a k-way merge over changed keys only; it never scans
// the complete declared Unit namespace and never sorts caller-authored rows.
type changeCursor struct {
	units  []carrier.Unit
	region support.Mask
	index  int
}

func changeCursorLess(left, right changeCursor) bool {
	return left.units[left.index].Less(right.units[right.index])
}

func changeHeapify(heap []changeCursor) {
	for index := len(heap)/2 - 1; index >= 0; index-- {
		changeHeapDown(heap, index)
	}
}

func changeHeapPush(heap *[]changeCursor, value changeCursor) {
	*heap = append(*heap, value)
	index := len(*heap) - 1
	for index > 0 {
		parent := (index - 1) / 2
		if !changeCursorLess((*heap)[index], (*heap)[parent]) {
			break
		}
		(*heap)[parent], (*heap)[index] = (*heap)[index], (*heap)[parent]
		index = parent
	}
}

func changeHeapPop(heap *[]changeCursor) changeCursor {
	last := len(*heap) - 1
	result := (*heap)[0]
	(*heap)[0] = (*heap)[last]
	(*heap)[last] = changeCursor{}
	*heap = (*heap)[:last]
	if last != 0 {
		changeHeapDown(*heap, 0)
	}
	return result
}

func changeHeapDown(heap []changeCursor, index int) {
	for {
		left := index*2 + 1
		if left >= len(heap) {
			return
		}
		smallest := left
		if right := left + 1; right < len(heap) && changeCursorLess(heap[right], heap[left]) {
			smallest = right
		}
		if !changeCursorLess(heap[smallest], heap[index]) {
			return
		}
		heap[index], heap[smallest] = heap[smallest], heap[index]
		index = smallest
	}
}

// Discard revokes the sole candidate write scope without publishing a root.
func (patch *Patch[K, V]) Discard() bool {
	if patch == nil || patch.patch == nil {
		return false
	}
	ok := patch.patch.Discard()
	patch.clear()
	return ok
}

func (patch *Patch[K, V]) clear() {
	if patch == nil {
		return
	}
	patch.patch = nil
	patch.binding = nil
	patch.state = carrier.State{}
	patch.slot = 0
	patch.support = support.Mask{}
	patch.base = carrier.RootHandle{}
	clear(patch.authored)
	if patch.owned {
		patch.authored = patch.authored[:0]
		return
	}
	patch.authored = nil
}
