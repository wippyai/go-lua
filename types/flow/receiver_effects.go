package flow

import (
	"cmp"
	"fmt"
	"strings"

	"github.com/wippyai/go-lua/types/access"
	"github.com/wippyai/go-lua/types/constraint"
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
	Mutations []ReceiverMutation
}

// ReceiverMutation is a parameter-relative mutable-footprint effect for a
// receiver slot. Segments are rooted at the runtime argument; an empty segment
// path denotes the argument itself. PresentElementWrite means the mutation was a
// dynamic element write with a definitely-present value, so existing key-presence
// facts for the same table survive just as they do for local writes.
type ReceiverMutation struct {
	Segments            []constraint.Segment
	PresentElementWrite bool
}

// ReceiverMutationFromAccessFootprint lowers a normalized source-access write
// footprint into the receiver-relative mutation vocabulary used in summaries.
func ReceiverMutationFromAccessFootprint(footprint access.WriteFootprint) (ReceiverMutation, bool) {
	if footprint.WritePath.IsEmpty() {
		return ReceiverMutation{}, false
	}
	return ReceiverMutation{
		Segments:            append([]constraint.Segment(nil), footprint.WritePath.Segments...),
		PresentElementWrite: footprint.PresentElementWrite,
	}, true
}

// RebaseReceiverMutations composes callee-relative mutations under the caller
// argument access path. An empty mutation list still means the argument itself
// was mutated.
func RebaseReceiverMutations(base ReceiverMutation, mutations []ReceiverMutation) []ReceiverMutation {
	if len(mutations) == 0 {
		return []ReceiverMutation{{
			Segments:            append([]constraint.Segment(nil), base.Segments...),
			PresentElementWrite: base.PresentElementWrite,
		}}
	}
	out := make([]ReceiverMutation, 0, len(mutations))
	for _, mutation := range mutations {
		segments := append([]constraint.Segment(nil), base.Segments...)
		segments = append(segments, mutation.Segments...)
		out = append(out, ReceiverMutation{
			Segments:            segments,
			PresentElementWrite: mutation.PresentElementWrite,
		})
	}
	return out
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

func ReceiverMustWriteWithMutations(slot int, v product.AbstractValue, mutations []ReceiverMutation) ReceiverEffects {
	return ReceiverEffectsOf([]ReceiverEffect{{Slot: slot, Value: v, MustWrite: true, Mutations: mutations}})
}

func ReceiverMutations(slot int, mutations []ReceiverMutation) ReceiverEffects {
	return ReceiverEffectsOf([]ReceiverEffect{{Slot: slot, Mutations: mutations}})
}

func (e ReceiverEffects) Entries() []ReceiverEffect {
	if e.bottom || e.top || len(e.entries) == 0 {
		return nil
	}
	return append([]ReceiverEffect(nil), e.entries...)
}

// HasMutations reports whether any finite receiver effect carries a mutable
// footprint. Top has no finite proof entries, matching Entries() semantics.
func (e ReceiverEffects) HasMutations() bool {
	if e.bottom || e.top {
		return false
	}
	for _, entry := range e.entries {
		if len(entry.Mutations) > 0 {
			return true
		}
	}
	return false
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
		value := "id"
		if !entry.Value.IsZero() {
			value = fmt.Sprintf("%s", entry.Value.ProjectValue())
		}
		mutationSuffix := ""
		if len(entry.Mutations) > 0 {
			mutationSuffix = fmt.Sprintf(":mut%d", len(entry.Mutations))
		}
		parts = append(parts, fmt.Sprintf("%d:%s:%s%s", entry.Slot, mode, value, mutationSuffix))
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
				!product.Domain.Equal(a.entries[i].Value, b.entries[i].Value) ||
				!receiverMutationsEqual(a.entries[i].Mutations, b.entries[i].Mutations) {
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
	if !receiverMutationsLessOrEq(a.Mutations, b.Mutations) {
		return false
	}
	valueLessOrEq := func(x, y product.AbstractValue) bool {
		if x.IsZero() {
			return true
		}
		if y.IsZero() {
			return false
		}
		return product.Domain.LessOrEq(x, y)
	}
	switch {
	case a.MustWrite && b.MustWrite:
		return valueLessOrEq(a.Value, b.Value)
	case a.MustWrite && !b.MustWrite:
		return valueLessOrEq(a.Value, b.Value)
	case !a.MustWrite && !b.MustWrite:
		return valueLessOrEq(a.Value, b.Value)
	default:
		return false
	}
}

func composeReceiverEffect(prev, next ReceiverEffect) ReceiverEffect {
	if next.MustWrite {
		next.Mutations = compactReceiverMutations(append(append([]ReceiverMutation(nil), prev.Mutations...), next.Mutations...))
		return next
	}
	value := receiverValueJoin(prev.Value, next.Value)
	return ReceiverEffect{
		Slot:      next.Slot,
		Value:     value,
		MustWrite: prev.MustWrite,
		Mutations: compactReceiverMutations(append(append([]ReceiverMutation(nil), prev.Mutations...), next.Mutations...)),
	}
}

func joinReceiverEffect(a, b ReceiverEffect) ReceiverEffect {
	return ReceiverEffect{
		Slot:      a.Slot,
		Value:     receiverValueJoin(a.Value, b.Value),
		MustWrite: a.MustWrite && b.MustWrite,
		Mutations: compactReceiverMutations(append(append([]ReceiverMutation(nil), a.Mutations...), b.Mutations...)),
	}
}

func widenReceiverEffect(prev, next ReceiverEffect) ReceiverEffect {
	return ReceiverEffect{
		Slot:      prev.Slot,
		Value:     receiverValueWiden(prev.Value, next.Value),
		MustWrite: prev.MustWrite && next.MustWrite,
		Mutations: compactReceiverMutations(append(append([]ReceiverMutation(nil), prev.Mutations...), next.Mutations...)),
	}
}

func receiverValueJoin(a, b product.AbstractValue) product.AbstractValue {
	switch {
	case a.IsZero():
		return b
	case b.IsZero():
		return a
	default:
		return product.Domain.Join(a, b)
	}
}

func receiverValueWiden(prev, next product.AbstractValue) product.AbstractValue {
	switch {
	case prev.IsZero():
		return next
	case next.IsZero():
		return prev
	default:
		return product.Domain.Widen(prev, next)
	}
}

func combineReceiverEffects(a, b ReceiverEffects, op func(ReceiverEffect, ReceiverEffect) ReceiverEffect) ReceiverEffects {
	var out []ReceiverEffect
	i, j := 0, 0
	for i < len(a.entries) || j < len(b.entries) {
		switch {
		case j >= len(b.entries) || (i < len(a.entries) && a.entries[i].Slot < b.entries[j].Slot):
			out = append(out, ReceiverEffect{
				Slot:      a.entries[i].Slot,
				Value:     a.entries[i].Value,
				MustWrite: false,
				Mutations: append([]ReceiverMutation(nil), a.entries[i].Mutations...),
			})
			i++
		case i >= len(a.entries) || b.entries[j].Slot < a.entries[i].Slot:
			out = append(out, ReceiverEffect{
				Slot:      b.entries[j].Slot,
				Value:     b.entries[j].Value,
				MustWrite: false,
				Mutations: append([]ReceiverMutation(nil), b.entries[j].Mutations...),
			})
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
		if e.Slot < 0 || receiverEffectIsEmpty(e) {
			continue
		}
		if len(dst) > 0 && dst[len(dst)-1].Slot == e.Slot {
			e.Mutations = compactReceiverMutations(e.Mutations)
			dst[len(dst)-1] = merge(dst[len(dst)-1], e)
			if receiverEffectIsEmpty(dst[len(dst)-1]) {
				dst = dst[:len(dst)-1]
			}
			continue
		}
		e.Mutations = compactReceiverMutations(e.Mutations)
		dst = append(dst, e)
	}
	return ReceiverEffects{entries: append([]ReceiverEffect(nil), dst...)}
}

func receiverEffectIsEmpty(e ReceiverEffect) bool {
	return valueIsBottom(e.Value) && len(compactReceiverMutations(e.Mutations)) == 0
}

func sortReceiverEffects(entries []ReceiverEffect) {
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0 && cmp.Compare(entries[j].Slot, entries[j-1].Slot) < 0; j-- {
			entries[j], entries[j-1] = entries[j-1], entries[j]
		}
	}
}

func compactReceiverMutations(in []ReceiverMutation) []ReceiverMutation {
	if len(in) == 0 {
		return nil
	}
	out := make([]ReceiverMutation, 0, len(in))
	for _, m := range in {
		out = append(out, ReceiverMutation{
			Segments:            append([]constraint.Segment(nil), m.Segments...),
			PresentElementWrite: m.PresentElementWrite,
		})
	}
	sortReceiverMutations(out)
	dst := out[:0]
	for _, m := range out {
		if len(dst) > 0 && compareReceiverMutation(dst[len(dst)-1], m) == 0 {
			continue
		}
		dst = append(dst, m)
	}
	return append([]ReceiverMutation(nil), dst...)
}

func receiverMutationsEqual(a, b []ReceiverMutation) bool {
	a = compactReceiverMutations(a)
	b = compactReceiverMutations(b)
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if compareReceiverMutation(a[i], b[i]) != 0 ||
			a[i].PresentElementWrite != b[i].PresentElementWrite {
			return false
		}
	}
	return true
}

func receiverMutationsLessOrEq(a, b []ReceiverMutation) bool {
	a = compactReceiverMutations(a)
	b = compactReceiverMutations(b)
	for _, x := range a {
		found := false
		for _, y := range b {
			if compareReceiverMutation(x, y) != 0 {
				continue
			}
			found = true
			break
		}
		if !found {
			return false
		}
	}
	return true
}

func sortReceiverMutations(entries []ReceiverMutation) {
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0 && receiverMutationLess(entries[j], entries[j-1]); j-- {
			entries[j], entries[j-1] = entries[j-1], entries[j]
		}
	}
}

func receiverMutationLess(a, b ReceiverMutation) bool {
	if c := compareConstraintSegments(a.Segments, b.Segments); c != 0 {
		return c < 0
	}
	return !a.PresentElementWrite && b.PresentElementWrite
}

func compareReceiverMutation(a, b ReceiverMutation) int {
	if c := compareConstraintSegments(a.Segments, b.Segments); c != 0 {
		return c
	}
	switch {
	case a.PresentElementWrite == b.PresentElementWrite:
		return 0
	case !a.PresentElementWrite:
		return -1
	default:
		return 1
	}
}
