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
	binding *Binding[K, V]
	state   carrier.State
	slot    shape.Slot
	support support.Mask
	base    carrier.RootHandle
	patch   *stage.Patch[planeFactor, K, V]
	targets []carrier.Target
	regions []support.Mask
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
	patch := stage.Begin(binding.plane.diagram, typedPlane.Root(), within, stage.Config[K, V]{
		KeyEnd:     binding.algebra.keyEnd,
		Default:    binding.algebra.default_,
		AdmitAt:    binding.algebra.admitAt,
		Equal:      binding.algebra.equal,
		LessOrEq:   binding.algebra.lessOrEq,
		JoinStable: binding.algebra.joinStable,
		Join: func(left, right V) (V, bool) {
			return binding.algebra.join(left, right), true
		},
	})
	if patch == nil {
		return nil
	}
	return &Patch[K, V]{binding: binding, state: state, slot: slot, support: within, base: base, patch: patch}
}

// Write is the only factbinding mutation entry.  Target is a presealed
// Binding capability: strong updates have exact singleton authority, while a
// weak target joins only the finite typed surface frozen at declaration time.
func (patch *Patch[K, V]) Write(target carrier.Target, when support.Mask, value V) bool {
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
	patch.targets = append(patch.targets, target)
	patch.regions = append(patch.regions, when)
	return true
}

// Transform applies one owner-issued carry map over this patch's precompiled
// carried closure. The stage patch enforces default preservation, admission,
// and Join-stability for each reachable terminal. It runs
// before ordinary row writes, against the same immutable predecessor and the
// same guard, so Accept exposes one atomic net ChangeSet.
func (patch *Patch[K, V]) Transform(closure TransformClosure[K, V], when support.Mask, apply func(V) (V, bool)) bool {
	if patch == nil || patch.binding == nil || patch.patch == nil || closure.binding != patch.binding || !when.Valid() || support.Empty(when) || !when.Entails(patch.support) || apply == nil {
		return false
	}
	if !patch.patch.Transform(closure.keys, when, apply) {
		return false
	}
	for _, target := range closure.targets {
		patch.targets = append(patch.targets, target)
		patch.regions = append(patch.regions, when)
	}
	return true
}

// TransformClosures applies one owner map over one authored closure plus one
// Binding-owned route closure. The closures may share targets; their sorted
// key vectors are merged by the stage Patch without allocating a route-sized
// union, so a shared route coordinate is transformed exactly once. The route
// closure itself remains Binding-owned and can therefore be shared by every
// Rule member. This private engine cut deliberately accepts at most two
// closures: authored static carry plus the one Factor route closure.
func (patch *Patch[K, V]) TransformClosures(closures []TransformClosure[K, V], when support.Mask, apply func(V) (V, bool)) bool {
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
	if !patch.patch.TransformSortedUnion(left.keys, right.keys, when, apply) {
		return false
	}
	leftIndex, rightIndex := 0, 0
	var prior carrier.Target
	havePrior := false
	for leftIndex < len(left.targets) || rightIndex < len(right.targets) {
		var target carrier.Target
		if rightIndex >= len(right.targets) || leftIndex < len(left.targets) && left.targets[leftIndex].Less(right.targets[rightIndex]) {
			target, leftIndex = left.targets[leftIndex], leftIndex+1
		} else if leftIndex >= len(left.targets) || right.targets[rightIndex].Less(left.targets[leftIndex]) {
			target, rightIndex = right.targets[rightIndex], rightIndex+1
		} else {
			target = left.targets[leftIndex]
			leftIndex++
			rightIndex++
		}
		if havePrior && prior.Same(target) {
			continue
		}
		prior, havePrior = target, true
		patch.targets = append(patch.targets, target)
		patch.regions = append(patch.regions, when)
	}
	return true
}

// WriteJoined performs one strong exact write after reducing every value for
// that exact target through the owning Factor's admitted Join. It is used by
// a route-output batch when distinct semantic routes resolve to one physical
// coordinate. The target is still a presealed singleton capability; this
// method does not turn a may-alias surface into a generic strong update.
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
	patch.targets = append(patch.targets, target)
	patch.regions = append(patch.regions, when)
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
	targets := append([]carrier.Target(nil), patch.targets...)
	authoredRegions := append([]support.Mask(nil), patch.regions...)
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
		var expanded bool
		factor, units, regions, expanded = binding.expandChanges(changes, work)
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
	if len(targets) == 0 {
		result, accepted = work.Accept(state, change)
	} else {
		result, accepted = work.AcceptAuthored(state, change, targets, authoredRegions)
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

func (binding *Binding[K, V]) expandChanges(changes keyChanges[K], unions *support.Work) (support.Mask, []carrier.Unit, []support.Mask, bool) {
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

	heap := make([]changeCursor, 0, changes.Count())
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
	if len(heap) == 0 {
		return factor, nil, nil, true
	}
	changeHeapify(heap)
	units := make([]carrier.Unit, 0, changes.Count())
	regions := make([]support.Mask, 0, changes.Count())
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
	clear(patch.targets)
	patch.targets = nil
	clear(patch.regions)
	patch.regions = nil
}
