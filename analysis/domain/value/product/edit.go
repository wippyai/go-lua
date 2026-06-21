package product

import (
	"fmt"
	"reflect"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
)

// Editor batches multiple axis writes onto a single product value, copying the
// slot set once and canonicalizing/reducing once at Done. It is the multi-axis
// counterpart to Set: applying the same axis writes through an Editor produces a
// value byte-identical (Equal and Hash-equal) to the equivalent chain of Set
// calls, because Done runs the same canonicalize+reduce interning path each Set
// runs, and Done short-circuits to the source value when no write changed it.
//
// The Editor is a stack value (Edit returns it by value, the EditSet free
// function takes a pointer to the local) so batching a hot multi-axis write adds
// no heap allocation over the source value's own slot copy. The source slot set
// is shared read-only until the first mutation, so a no-op edit allocates
// nothing. Use it only where two or more axes are written to the same value in
// sequence; a single write should stay a plain Set.
type Editor struct {
	rt       *registryRuntime
	source   Value
	presence presence.Value
	slots    []slot
	owned    bool
	changed  bool
}

// Edit returns an Editor seeded with v's current shape, presence, and slots.
func Edit(reg *axis.Registry, v Value) Editor {
	rt := mustRuntime(reg)
	rt.validateValue(v)
	return Editor{
		rt:       rt,
		source:   v,
		presence: PresenceOf(v),
		slots:    v.slotsView(),
	}
}

// EditSet applies one typed axis write to the working slot set with the same
// per-axis semantics as Set: setting an axis to its Top value drops the slot,
// any other value upserts it. The first mutation copies the slot set so the
// source value's interned slots are never aliased.
func EditSet[T any](ed *Editor, key axis.Key[T], value T) {
	if key.ID() == presence.Key.ID() {
		panic("product: presence is a core lane; use SetPresence")
	}
	info, ok := ed.rt.axis(key.ID())
	if !ok {
		panic(fmt.Sprintf("product: unregistered axis %q", key.ID()))
	}
	wantType := reflect.TypeFor[T]()
	if wantType != info.topType {
		panic(fmt.Sprintf("product: axis %q has incompatible typed key type %v, want %v", key.ID(), wantType, info.topType))
	}
	if existing, ok := slotValue(ed.slots, key.ID()); ok {
		if info.spec.EqualAny(existing, value) {
			return
		}
	} else if info.spec.IsTopAny(value) {
		return
	}
	ed.ensureOwned()
	if info.spec.IsTopAny(value) {
		ed.slots = deleteSlot(ed.slots, key.ID())
	} else {
		ed.slots = upsertSlot(ed.slots, key.ID(), value)
	}
	ed.changed = true
}

// SetPresence stages a presence-lane change on the working value.
func (ed *Editor) SetPresence(p presence.Value) {
	if presence.Equal(ed.presence, p) {
		return
	}
	ed.presence = p
	ed.changed = true
}

// Done canonicalizes and reduces the accumulated writes once and returns the
// interned value, or the unchanged source value when no write altered it.
func (ed *Editor) Done() Value {
	if !ed.changed {
		return ed.source
	}
	return internRuntime(ed.rt, ShapeOf(ed.source), ed.presence, ed.slots)
}

func (ed *Editor) ensureOwned() {
	if ed.owned {
		return
	}
	ed.slots = copySlots(ed.source)
	ed.owned = true
}

func slotValue(slots []slot, key string) (any, bool) {
	for i := range slots {
		if slots[i].key == key {
			return slots[i].value, true
		}
	}
	return nil, false
}
