package flow

import (
	"cmp"
	"fmt"
	"strings"

	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/lattice"
)

// ReceiverEffect is one caller-visible write effect on a runtime argument slot.
// It is the parameter-slot analogue of CaptureEffect: a callee may mutate a
// table passed as an argument, and the caller must apply that transformer to the
// concrete argument place after the call.
type ReceiverEffect struct {
	Slot      int
	Value     product.AbstractValue
	MustWrite bool
}

// ReceiverEffects is a deterministic finite-map lattice of runtime-argument
// transformers. It is summary state, not store state: unchanged entry values are
// not effects, and branch joins weaken one-sided writes to may-writes.
type ReceiverEffects struct {
	bottom  bool
	top     bool
	entries []ReceiverEffect
}

func ReceiverEffectsOf(entries []ReceiverEffect) ReceiverEffects {
	return canonicalReceiverEffects(entries, joinReceiverEffect)
}

func ReceiverEffectsTop() ReceiverEffects { return ReceiverEffects{top: true} }

func ReceiverEffectsIdentity() ReceiverEffects { return ReceiverEffects{} }

func ReceiverMustWrite(slot int, v product.AbstractValue) ReceiverEffects {
	return ReceiverEffectsOf([]ReceiverEffect{{Slot: slot, Value: v, MustWrite: true}})
}

func (e ReceiverEffects) Entries() []ReceiverEffect {
	if e.bottom || e.top || len(e.entries) == 0 {
		return nil
	}
	return append([]ReceiverEffect(nil), e.entries...)
}

func (e ReceiverEffects) IsTop() bool { return e.top }

func (e ReceiverEffects) IsBottom() bool { return e.bottom }

func (e ReceiverEffects) WithMustWrite(slot int, v product.AbstractValue) ReceiverEffects {
	return e.Then(ReceiverMustWrite(slot, v))
}

func (e ReceiverEffects) Then(next ReceiverEffects) ReceiverEffects {
	if e.bottom || next.bottom {
		return ReceiverEffects{bottom: true}
	}
	if e.top || next.top {
		return ReceiverEffectsTop()
	}
	var out []ReceiverEffect
	i, j := 0, 0
	for i < len(e.entries) || j < len(next.entries) {
		switch {
		case j >= len(next.entries) || (i < len(e.entries) && e.entries[i].Slot < next.entries[j].Slot):
			out = append(out, e.entries[i])
			i++
		case i >= len(e.entries) || next.entries[j].Slot < e.entries[i].Slot:
			out = append(out, next.entries[j])
			j++
		default:
			out = append(out, composeReceiverEffect(e.entries[i], next.entries[j]))
			i++
			j++
		}
	}
	return canonicalReceiverEffects(out, joinReceiverEffect)
}

// CooccurringReceiverEffects joins two receiver transformers whose relative
// execution order is unknown.
func CooccurringReceiverEffects(a, b ReceiverEffects) ReceiverEffects {
	if ReceiverEffectsDomain.Equal(a, ReceiverEffectsDomain.Bottom()) {
		return b
	}
	if ReceiverEffectsDomain.Equal(b, ReceiverEffectsDomain.Bottom()) {
		return a
	}
	ab := a.Then(b)
	ba := b.Then(a)
	return ReceiverEffectsDomain.Join(ab, ba)
}

func (e ReceiverEffects) Format() string {
	if e.bottom {
		return "⊥"
	}
	if e.top {
		return "⊤"
	}
	if len(e.entries) == 0 {
		return "id"
	}
	parts := make([]string, 0, len(e.entries))
	for _, entry := range e.entries {
		mode := "may"
		if entry.MustWrite {
			mode = "must"
		}
		parts = append(parts, fmt.Sprintf("%d:%s:%s", entry.Slot, mode, entry.Value.ProjectValue()))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

var ReceiverEffectsDomain = lattice.Lattice[ReceiverEffects]{
	Bottom: func() ReceiverEffects { return ReceiverEffects{bottom: true} },
	Top:    ReceiverEffectsTop,
	Equal: func(a, b ReceiverEffects) bool {
		if a.bottom || b.bottom {
			return a.bottom && b.bottom
		}
		if a.top || b.top {
			return a.top && b.top
		}
		if len(a.entries) != len(b.entries) {
			return false
		}
		for i := range a.entries {
			if a.entries[i].Slot != b.entries[i].Slot ||
				a.entries[i].MustWrite != b.entries[i].MustWrite ||
				!product.Domain.Equal(a.entries[i].Value, b.entries[i].Value) {
				return false
			}
		}
		return true
	},
	LessOrEq: func(a, b ReceiverEffects) bool {
		if a.bottom {
			return true
		}
		if b.bottom {
			return false
		}
		if b.top {
			return true
		}
		if a.top {
			return false
		}
		i, j := 0, 0
		for i < len(a.entries) || j < len(b.entries) {
			switch {
			case j >= len(b.entries) || (i < len(a.entries) && a.entries[i].Slot < b.entries[j].Slot):
				return false
			case i >= len(a.entries) || b.entries[j].Slot < a.entries[i].Slot:
				if !identityLessOrEqReceiverEffect(b.entries[j]) {
					return false
				}
				j++
			default:
				if !receiverEffectLessOrEq(a.entries[i], b.entries[j]) {
					return false
				}
				i++
				j++
			}
		}
		return true
	},
	Join: func(a, b ReceiverEffects) ReceiverEffects {
		if a.bottom {
			return b
		}
		if b.bottom {
			return a
		}
		if a.top || b.top {
			return ReceiverEffectsTop()
		}
		return combineReceiverEffects(a, b, joinReceiverEffect)
	},
	Meet: nil,
	Widen: func(prev, next ReceiverEffects) ReceiverEffects {
		if prev.bottom {
			return next
		}
		if next.bottom {
			return prev
		}
		if prev.top || next.top {
			return ReceiverEffectsTop()
		}
		return combineReceiverEffects(prev, next, widenReceiverEffect)
	},
}

func identityLessOrEqReceiverEffect(e ReceiverEffect) bool {
	return !e.MustWrite
}

func receiverEffectLessOrEq(a, b ReceiverEffect) bool {
	switch {
	case a.MustWrite && b.MustWrite:
		return product.Domain.LessOrEq(a.Value, b.Value)
	case a.MustWrite && !b.MustWrite:
		return product.Domain.LessOrEq(a.Value, b.Value)
	case !a.MustWrite && !b.MustWrite:
		return product.Domain.LessOrEq(a.Value, b.Value)
	default:
		return false
	}
}

func composeReceiverEffect(prev, next ReceiverEffect) ReceiverEffect {
	if next.MustWrite {
		return next
	}
	value := product.Domain.Join(prev.Value, next.Value)
	return ReceiverEffect{Slot: next.Slot, Value: value, MustWrite: prev.MustWrite}
}

func joinReceiverEffect(a, b ReceiverEffect) ReceiverEffect {
	return ReceiverEffect{
		Slot:      a.Slot,
		Value:     product.Domain.Join(a.Value, b.Value),
		MustWrite: a.MustWrite && b.MustWrite,
	}
}

func widenReceiverEffect(prev, next ReceiverEffect) ReceiverEffect {
	return ReceiverEffect{
		Slot:      prev.Slot,
		Value:     product.Domain.Widen(prev.Value, next.Value),
		MustWrite: prev.MustWrite && next.MustWrite,
	}
}

func combineReceiverEffects(a, b ReceiverEffects, op func(ReceiverEffect, ReceiverEffect) ReceiverEffect) ReceiverEffects {
	var out []ReceiverEffect
	i, j := 0, 0
	for i < len(a.entries) || j < len(b.entries) {
		switch {
		case j >= len(b.entries) || (i < len(a.entries) && a.entries[i].Slot < b.entries[j].Slot):
			out = append(out, ReceiverEffect{Slot: a.entries[i].Slot, Value: a.entries[i].Value, MustWrite: false})
			i++
		case i >= len(a.entries) || b.entries[j].Slot < a.entries[i].Slot:
			out = append(out, ReceiverEffect{Slot: b.entries[j].Slot, Value: b.entries[j].Value, MustWrite: false})
			j++
		default:
			out = append(out, op(a.entries[i], b.entries[j]))
			i++
			j++
		}
	}
	return canonicalReceiverEffects(out, joinReceiverEffect)
}

func canonicalReceiverEffects(entries []ReceiverEffect, merge func(ReceiverEffect, ReceiverEffect) ReceiverEffect) ReceiverEffects {
	if len(entries) == 0 {
		return ReceiverEffectsIdentity()
	}
	out := append([]ReceiverEffect(nil), entries...)
	sortReceiverEffects(out)
	dst := out[:0]
	for _, e := range out {
		if e.Slot < 0 || valueIsBottom(e.Value) {
			continue
		}
		if len(dst) > 0 && dst[len(dst)-1].Slot == e.Slot {
			dst[len(dst)-1] = merge(dst[len(dst)-1], e)
			if valueIsBottom(dst[len(dst)-1].Value) {
				dst = dst[:len(dst)-1]
			}
			continue
		}
		dst = append(dst, e)
	}
	return ReceiverEffects{entries: append([]ReceiverEffect(nil), dst...)}
}

func sortReceiverEffects(entries []ReceiverEffect) {
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0 && cmp.Compare(entries[j].Slot, entries[j-1].Slot) < 0; j-- {
			entries[j], entries[j-1] = entries[j-1], entries[j]
		}
	}
}
