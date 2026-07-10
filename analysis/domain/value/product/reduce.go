package product

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
)

const maxReducerPasses = 32

// reducerEntry pairs a reducer with the axis ids it depends on, so reduction can
// gate on slot presence before allocating a reduce editor.
type reducerEntry struct {
	apply axis.Reducer
	reads []string
}

// applicable reports whether the reducer can possibly fire on these slots: every
// axis it reads must be present (a top axis is never stored as a slot, so an
// absent read axis means the reducer would observe top and no-op). An empty
// reads list means the reducer is always applicable.
func (e reducerEntry) applicable(slots []slot) bool {
	for _, read := range e.reads {
		found := false
		for i := range slots {
			if slots[i].key == read {
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

func reduce(rt *registryRuntime, shape Shape, p presence.Value, slots []slot) (Shape, presence.Value, []slot) {
	shape, p, _ = reducePresenceShape(shape, p)
	if rt.isProductBottom(p, slots) {
		return ShapeBottom, presence.Bottom(), rt.bottomSlots
	}

	if len(rt.reducers) == 0 || !anyReducerApplicable(rt.reducers, slots) {
		return shape, p, slots
	}

	editor := newReduceEditor(rt, p, slots)
	for pass := 0; pass < maxReducerPasses; pass++ {
		changed := false
		for _, reducer := range rt.reducers {
			reducerChanged := reducer.apply(&editor)
			editorChanged := editor.consumeChanged()
			if reducerChanged || editorChanged {
				changed = true
			}
		}
		nextShape, nextPresence, shapePresenceChanged := reducePresenceShape(shape, editor.presence)
		if shapePresenceChanged {
			shape = nextShape
			editor.presence = nextPresence
			changed = true
		}
		if editor.isProductBottom() {
			return ShapeBottom, presence.Bottom(), rt.bottomSlots
		}
		if !changed {
			return shape, editor.presence, editor.slots()
		}
	}
	panic("product: reducer loop did not converge")
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
	rt        *registryRuntime
	presence  presence.Value
	values    []slot
	needsSort bool
	changed   bool
}

func newReduceEditor(rt *registryRuntime, p presence.Value, slots []slot) reduceEditor {
	// Slots are few (one per non-top axis) and arrive canonically ordered, so a
	// linear-scanned slice avoids the per-reduction map allocation. internRuntime
	// passes reducer-owned work slices here: exported product writes copy before
	// mutation, merge/meet build fresh stack slices, and bottom returns before a
	// reducer can touch rt.bottomSlots. The interner still stores its own immutable
	// copy after reduction.
	return reduceEditor{rt: rt, presence: p, values: slots}
}

func (e *reduceEditor) GetAny(key string) (any, bool) {
	if key == presence.Key.ID() {
		panic("product: presence is a core lane; use presence.Get")
	}
	info, ok := e.rt.axis(key)
	if !ok {
		return nil, false
	}
	for i := range e.values {
		if e.values[i].key == key {
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
	if info.spec.IsTopAny(value) {
		for i := range e.values {
			if e.values[i].key == key {
				e.values = append(e.values[:i], e.values[i+1:]...)
				e.changed = true
				return
			}
		}
		return
	}
	for i := range e.values {
		if e.values[i].key == key {
			if !info.spec.EqualAny(e.values[i].value, value) {
				e.values[i].value = value
				e.changed = true
			}
			return
		}
	}
	e.values = append(e.values, slot{key: key, value: value})
	e.needsSort = true
	e.changed = true
}

func (e *reduceEditor) Presence() presence.Value {
	return e.presence
}

func (e *reduceEditor) SetPresence(p presence.Value) {
	next := normalizePresence(p)
	if !presence.Equal(e.presence, next) {
		e.presence = next
		e.changed = true
	}
}

func (e *reduceEditor) consumeChanged() bool {
	changed := e.changed
	e.changed = false
	return changed
}

func (e *reduceEditor) isProductBottom() bool {
	if presence.Equal(e.presence, presence.Bottom()) {
		return true
	}
	for i := range e.values {
		info, ok := e.rt.axis(e.values[i].key)
		if !ok {
			panic("product: unregistered axis slot " + e.values[i].key)
		}
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
		sort.Slice(e.values, func(i, j int) bool { return e.values[i].key < e.values[j].key })
		e.needsSort = false
	}
	return e.values
}
