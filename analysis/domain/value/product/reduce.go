package product

import (
	"fmt"
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
)

// reducerEntry is the frozen, ordinal-resolved contract for one reduced-product
// rule. Entries and dependency lists are ordered by owner id, independently of
// axis registration order.
type reducerEntry struct {
	owner          string
	apply          axis.Reducer
	reads          []uint16
	readsPresence  bool
	readAllowed    []bool
	writeAllowed   []bool
	writesPresence bool
}

const (
	inlineReducerQueue = 16
	inlineReducerAxes  = 64
)

// applicable reports whether every declared sparse input is materialized. Top
// axes are omitted from slots; the reducer contract says an omitted input is a
// no-op. Core presence is always materialized and therefore never gates here.
func (e reducerEntry) applicable(slots []slot) bool {
	for _, read := range e.reads {
		found := false
		for i := range slots {
			if slots[i].ordinal == read {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func anyReducerApplicable(reducers []reducerEntry, slots []slot) bool {
	for i := range reducers {
		if reducers[i].applicable(slots) {
			return true
		}
	}
	return false
}

func (rt *registryRuntime) buildReducers(view axis.ReducersView) error {
	if view.Len() == 0 {
		return nil
	}
	order := make([]int, view.Len())
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(i, j int) bool {
		return view.OwnerAt(order[i]) < view.OwnerAt(order[j])
	})

	rt.reducers = make([]reducerEntry, 0, view.Len())
	rt.reducerDeps = make([][]int, len(rt.axes))
	for _, sourceIndex := range order {
		entry := reducerEntry{
			owner:        view.OwnerAt(sourceIndex),
			apply:        view.At(sourceIndex),
			readAllowed:  make([]bool, len(rt.axes)),
			writeAllowed: make([]bool, len(rt.axes)),
		}
		for _, id := range view.ReadsAt(sourceIndex) {
			if id == presence.Key.ID() {
				entry.readsPresence = true
				continue
			}
			info, ok := rt.axis(id)
			if !ok {
				return fmt.Errorf("product: reducer %q reads unregistered axis %q", entry.owner, id)
			}
			if entry.readAllowed[info.ordinal] {
				continue
			}
			entry.readAllowed[info.ordinal] = true
			entry.reads = append(entry.reads, info.ordinal)
		}
		if len(entry.reads) == 0 {
			return fmt.Errorf("product: reducer %q requires at least one declared sparse read", entry.owner)
		}
		for _, id := range view.WritesAt(sourceIndex) {
			if id == presence.Key.ID() {
				if presence.Spec().ReductionRank.Width <= 0 {
					return fmt.Errorf("product: reducer %q writes core presence without a ReductionRank", entry.owner)
				}
				entry.writesPresence = true
				rt.presenceReducerWritten = true
				continue
			}
			info, ok := rt.axis(id)
			if !ok {
				return fmt.Errorf("product: reducer %q writes unregistered axis %q", entry.owner, id)
			}
			if info.spec.ReductionRankWidth() <= 0 {
				return fmt.Errorf("product: reducer %q writes axis %q without a ReductionRank", entry.owner, id)
			}
			entry.writeAllowed[info.ordinal] = true
			rt.axes[info.ordinal].reducerWritten = true
		}
		sort.Slice(entry.reads, func(i, j int) bool { return entry.reads[i] < entry.reads[j] })
		reducerIndex := len(rt.reducers)
		rt.reducers = append(rt.reducers, entry)
		for _, ordinal := range entry.reads {
			rt.reducerDeps[ordinal] = append(rt.reducerDeps[ordinal], reducerIndex)
		}
		if entry.readsPresence {
			rt.presenceDeps = append(rt.presenceDeps, reducerIndex)
		}
	}
	// Freeze this canonical sparse subset once. Reduced merge validation is on
	// the cyclic hot path and must not filter every registered axis to find the
	// few coordinates a reducer is permitted to change.
	rt.reducerWrittenOrdinals = rt.reducerWrittenOrdinals[:0]
	for ordinal := range rt.axes {
		if rt.axisOrdinal(uint16(ordinal)).reducerWritten {
			rt.reducerWrittenOrdinals = append(rt.reducerWrittenOrdinals, uint16(ordinal))
		}
	}
	return nil
}

func reduce(rt *registryRuntime, shape Shape, p presence.Value, slots []slot) (Shape, presence.Value, []slot) {
	shape, p, _ = reducePresenceShape(shape, p)
	if rt.isProductBottom(p, slots) {
		return ShapeBottom, presence.Bottom(), rt.bottomSlots
	}

	if len(rt.reducers) == 0 || !anyReducerApplicable(rt.reducers, slots) {
		return shape, p, slots
	}

	editor := newReduceEditor(rt, p, slots)
	editor.initTracking()
	var inlineQueue [inlineReducerQueue]int
	var inlineEnqueued [inlineReducerQueue]bool
	var queue []int
	var enqueued []bool
	if len(rt.reducers) <= inlineReducerQueue {
		queue = inlineQueue[:len(rt.reducers)]
		enqueued = inlineEnqueued[:len(rt.reducers)]
	} else {
		queue = make([]int, len(rt.reducers))
		enqueued = make([]bool, len(rt.reducers))
	}
	for i := range rt.reducers {
		queue[i] = i
		enqueued[i] = true
	}
	for head := 0; head < len(queue); head++ {
		reducerIndex := queue[head]
		enqueued[reducerIndex] = false
		reducer := &rt.reducers[reducerIndex]
		if !reducer.applicable(editor.values) {
			continue
		}
		editor.begin(reducer)
		_ = reducer.apply(&editor)
		nextShape, nextPresence, shapePresenceChanged := reducePresenceShape(shape, editor.presence)
		if shapePresenceChanged {
			shape = nextShape
			if !presence.Equal(editor.presence, nextPresence) {
				editor.presence = nextPresence
				editor.recordPresenceChanged()
			}
		}
		if editor.isProductBottom() {
			return ShapeBottom, presence.Bottom(), rt.bottomSlots
		}

		for _, ordinal := range editor.changedAxes() {
			for _, dependent := range rt.reducerDeps[ordinal] {
				if !enqueued[dependent] {
					queue = append(queue, dependent)
					enqueued[dependent] = true
				}
			}
		}
		if editor.presenceChanged {
			for _, dependent := range rt.presenceDeps {
				if !enqueued[dependent] {
					queue = append(queue, dependent)
					enqueued[dependent] = true
				}
			}
		}
		editor.clearChanges()
	}
	return shape, editor.presence, editor.slots()
}

func reducePresenceShape(shape Shape, p presence.Value) (Shape, presence.Value, bool) {
	nextShape := shape
	nextPresence := p

	if presence.Equal(nextPresence, presence.Bottom()) {
		nextShape = ShapeBottom
	} else if presence.Equal(nextPresence, presence.Absent()) {
		nextShape = ShapeBottom
	} else if nextShape == ShapeBottom {
		switch {
		case presence.Equal(nextPresence, presence.Present()):
			nextPresence = presence.Bottom()
		case presence.Equal(nextPresence, presence.Top()):
			nextPresence = presence.Absent()
		}
	}

	return nextShape, nextPresence, nextShape != shape || !presence.Equal(nextPresence, p)
}

type reduceEditor struct {
	rt               *registryRuntime
	presence         presence.Value
	values           []slot
	needsSort        bool
	active           *reducerEntry
	changed          []uint16
	changedSet       []bool
	presenceChanged  bool
	inlineChanged    [inlineReducerQueue]uint16
	inlineChangedSet [inlineReducerAxes]bool
}

func newReduceEditor(rt *registryRuntime, p presence.Value, slots []slot) reduceEditor {
	// Slots are few (one per non-top axis) and arrive canonically ordered, so a
	// linear-scanned slice avoids the per-reduction map allocation. internRuntime
	// passes reducer-owned work slices here: exported product writes copy before
	// mutation, merge/meet build fresh stack slices, and bottom returns before a
	// reducer can touch rt.bottomSlots. The interner still stores its own immutable
	// copy after reduction.
	return reduceEditor{
		rt:       rt,
		presence: p,
		values:   slots,
	}
}

func (e *reduceEditor) initTracking() {
	e.changed = e.inlineChanged[:0]
	if len(e.rt.axes) <= inlineReducerAxes {
		e.changedSet = e.inlineChangedSet[:len(e.rt.axes)]
	} else {
		e.changedSet = make([]bool, len(e.rt.axes))
	}
}

func (e *reduceEditor) begin(reducer *reducerEntry) {
	e.active = reducer
}

func (e *reduceEditor) GetAny(key string) (any, bool) {
	if key == presence.Key.ID() {
		panic("product: presence is a core lane; use presence.Get")
	}
	info, ok := e.rt.axis(key)
	if !ok {
		return nil, false
	}
	if e.active != nil && !e.active.readAllowed[info.ordinal] {
		panic(fmt.Sprintf("product: reducer %q read undeclared axis %q", e.active.owner, key))
	}
	for i := range e.values {
		if e.values[i].ordinal == info.ordinal {
			return e.values[i].value, true
		}
	}
	return info.topAny, true
}

func (e *reduceEditor) SetAny(key string, value any) {
	if key == presence.Key.ID() {
		panic("product: presence is a core lane; use presence.Set")
	}
	info, ok := e.rt.axis(key)
	if !ok {
		panic("product: reducer wrote unregistered axis " + key)
	}
	if e.active == nil || !e.active.writeAllowed[info.ordinal] {
		owner := "<none>"
		if e.active != nil {
			owner = e.active.owner
		}
		panic(fmt.Sprintf("product: reducer %q wrote undeclared axis %q", owner, key))
	}
	current := info.topAny
	currentIndex := -1
	for i := range e.values {
		if e.values[i].ordinal == info.ordinal {
			current = e.values[i].value
			currentIndex = i
			break
		}
	}
	if info.spec.EqualAny(current, value) {
		return
	}
	if !info.spec.LessOrEqAny(value, current) {
		panic(fmt.Sprintf("product: reducer %q made non-reductive write to axis %q", e.active.owner, key))
	}
	if !info.spec.EqualAny(value, current) && !reductionRankDescends(info, current, value) {
		panic(fmt.Sprintf("product: reducer %q made a ReductionRank-non-descending write to axis %q", e.active.owner, key))
	}
	if info.spec.IsTopAny(value) {
		if currentIndex >= 0 {
			e.values = append(e.values[:currentIndex], e.values[currentIndex+1:]...)
			e.recordChanged(info.ordinal)
		}
		return
	}
	if currentIndex >= 0 {
		e.values[currentIndex].value = value
		e.recordChanged(info.ordinal)
		return
	}
	e.values = append(e.values, slot{ordinal: info.ordinal, value: value})
	e.needsSort = true
	e.recordChanged(info.ordinal)
}

func (e *reduceEditor) Presence() presence.Value {
	if e.active != nil && !e.active.readsPresence {
		panic(fmt.Sprintf("product: reducer %q read undeclared axis %q", e.active.owner, presence.Key.ID()))
	}
	return e.presence
}

func (e *reduceEditor) SetPresence(p presence.Value) {
	if e.active == nil || !e.active.writesPresence {
		owner := "<none>"
		if e.active != nil {
			owner = e.active.owner
		}
		panic(fmt.Sprintf("product: reducer %q wrote undeclared axis %q", owner, presence.Key.ID()))
	}
	next := normalizePresence(p)
	if presence.Equal(e.presence, next) {
		return
	}
	if !e.presence.Covers(next) {
		panic(fmt.Sprintf("product: reducer %q made non-reductive write to axis %q", e.active.owner, presence.Key.ID()))
	}
	if !presence.Equal(e.presence, next) && !presenceReductionRankDescends(e.presence, next) {
		panic(fmt.Sprintf("product: reducer %q made a ReductionRank-non-descending write to axis %q", e.active.owner, presence.Key.ID()))
	}
	e.presence = next
	e.recordPresenceChanged()
}

func reductionRankDescends(info axisRuntimeAxis, before, after any) bool {
	for component := 0; component < info.spec.ReductionRankWidth(); component++ {
		beforeRank := info.spec.ReductionRankAtAny(before, component)
		afterRank := info.spec.ReductionRankAtAny(after, component)
		switch {
		case afterRank < beforeRank:
			return true
		case afterRank > beforeRank:
			return false
		}
	}
	return false
}

func presenceReductionRankDescends(before, after presence.Value) bool {
	rank := presence.Spec().ReductionRank
	for component := 0; component < rank.Width; component++ {
		beforeRank := rank.At(before, component)
		afterRank := rank.At(after, component)
		switch {
		case afterRank < beforeRank:
			return true
		case afterRank > beforeRank:
			return false
		}
	}
	return false
}

func (e *reduceEditor) recordChanged(ordinal uint16) {
	if e.changedSet[ordinal] {
		return
	}
	e.changedSet[ordinal] = true
	e.changed = append(e.changed, ordinal)
}

func (e *reduceEditor) recordPresenceChanged() {
	e.presenceChanged = true
}

func (e *reduceEditor) changedAxes() []uint16 {
	sort.Slice(e.changed, func(i, j int) bool { return e.changed[i] < e.changed[j] })
	return e.changed
}

func (e *reduceEditor) clearChanges() {
	for _, ordinal := range e.changed {
		e.changedSet[ordinal] = false
	}
	e.changed = e.changed[:0]
	e.presenceChanged = false
	e.active = nil
}

func (e *reduceEditor) isProductBottom() bool {
	if presence.Equal(e.presence, presence.Bottom()) {
		return true
	}
	for i := range e.values {
		info := e.rt.axisOrdinal(e.values[i].ordinal)
		if info.spec.EqualAny(e.values[i].value, info.bottomAny) {
			return true
		}
	}
	return false
}

func (e *reduceEditor) slots() []slot {
	if len(e.values) == 0 {
		return nil
	}
	if e.needsSort {
		sort.Slice(e.values, func(i, j int) bool { return e.values[i].ordinal < e.values[j].ordinal })
		e.needsSort = false
	}
	return e.values
}
