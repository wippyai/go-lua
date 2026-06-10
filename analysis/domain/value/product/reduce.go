package product

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
)

const maxReducerPasses = 32

func reduce(reg *axis.Registry, shape Shape, p presence.Value, slots []slot) (Shape, presence.Value, []slot) {
	shape, p, _ = reducePresenceShape(shape, p)

	reducers := reg.Reducers()
	if len(reducers) == 0 {
		return shape, p, slots
	}

	editor := newReduceEditor(reg, p, slots)
	for pass := 0; pass < maxReducerPasses; pass++ {
		changed := false
		for _, reducer := range reducers {
			reducerChanged := reducer(editor)
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
	reg      *axis.Registry
	presence presence.Value
	values   map[string]any
	changed  bool
}

func newReduceEditor(reg *axis.Registry, p presence.Value, slots []slot) *reduceEditor {
	values := make(map[string]any, len(slots))
	for _, slot := range slots {
		values[slot.key] = slot.value
	}
	return &reduceEditor{reg: reg, presence: p, values: values}
}

func (e *reduceEditor) GetAny(key string) (any, bool) {
	if key == presence.Key.ID() {
		return e.presence, true
	}
	spec, ok := e.reg.LookupErased(key)
	if !ok {
		return nil, false
	}
	if v, ok := e.values[key]; ok {
		return v, true
	}
	return spec.TopAny(), true
}

func (e *reduceEditor) SetAny(key string, value any) {
	if key == presence.Key.ID() {
		next, ok := value.(presence.Value)
		if !ok {
			panic("product: reducer wrote non-presence value to presence")
		}
		if !presence.Equal(e.presence, next) {
			e.presence = next
			e.changed = true
		}
		return
	}
	spec, ok := e.reg.LookupErased(key)
	if !ok {
		panic("product: reducer wrote unregistered axis " + key)
	}
	if spec.IsTopAny(value) {
		if _, exists := e.values[key]; exists {
			delete(e.values, key)
			e.changed = true
		}
		return
	}
	if cur, exists := e.values[key]; exists && spec.EqualAny(cur, value) {
		return
	}
	e.values[key] = value
	e.changed = true
}

func (e *reduceEditor) consumeChanged() bool {
	changed := e.changed
	e.changed = false
	return changed
}

func (e *reduceEditor) slots() []slot {
	if len(e.values) == 0 {
		return nil
	}
	keys := make([]string, 0, len(e.values))
	for key := range e.values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]slot, 0, len(keys))
	for _, key := range keys {
		out = append(out, slot{key: key, value: e.values[key]})
	}
	return out
}
