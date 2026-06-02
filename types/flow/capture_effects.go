package flow

import (
	"cmp"
	"fmt"
	"strings"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/lattice"
)

// CaptureEffect is one abstract caller-visible effect on a captured lexical
// cell. Absent symbols mean identity: the cell is left unchanged.
//
// MustWrite means every path represented by this effect writes Symbol to Value.
// !MustWrite means some path writes Symbol to Value and some path leaves the old
// cell value in place; applying it joins the old value with Value.
type CaptureEffect struct {
	Symbol    cfg.SymbolID
	Value     product.AbstractValue
	MustWrite bool
}

// CaptureEffects is a deterministic finite-map lattice of abstract cell
// transformers. It is separate from CaptureCells: CaptureCells is store state,
// while CaptureEffects is the summary/apply boundary for caller-visible writes.
type CaptureEffects struct {
	bottom  bool
	top     bool
	entries []CaptureEffect
}

// CaptureEffectsOf constructs a canonical effect map. Duplicate symbols are
// joined as alternative effects at the same control-flow point.
func CaptureEffectsOf(entries []CaptureEffect) CaptureEffects {
	return canonicalCaptureEffects(entries, joinCaptureEffect)
}

// CaptureEffectsTop returns the greatest, unknown cell effect.
func CaptureEffectsTop() CaptureEffects {
	return CaptureEffects{top: true}
}

// CaptureEffectsIdentity returns the reachable no-op cell effect.
func CaptureEffectsIdentity() CaptureEffects {
	return CaptureEffects{}
}

// CaptureMustWrite constructs the single-cell must-write effect.
func CaptureMustWrite(sym cfg.SymbolID, v product.AbstractValue) CaptureEffects {
	return CaptureEffectsOf([]CaptureEffect{{Symbol: sym, Value: v, MustWrite: true}})
}

// Entries returns a copy of the sorted finite effects. Top has no finite entry
// representation and returns nil.
func (e CaptureEffects) Entries() []CaptureEffect {
	if e.bottom || e.top || len(e.entries) == 0 {
		return nil
	}
	return append([]CaptureEffect(nil), e.entries...)
}

// May weakens every finite write to a may-write. It is used when a caller-visible
// callback may execute zero times: the written value is still possible, but the
// incoming cell value may remain unchanged.
func (e CaptureEffects) May() CaptureEffects {
	if e.bottom || e.top || len(e.entries) == 0 {
		return e
	}
	out := make([]CaptureEffect, len(e.entries))
	for i, entry := range e.entries {
		out[i] = CaptureEffect{Symbol: entry.Symbol, Value: entry.Value, MustWrite: false}
	}
	return CaptureEffectsOf(out)
}

// IsTop reports whether e is the greatest capture-effect value.
func (e CaptureEffects) IsTop() bool { return e.top }

// IsBottom reports whether e is the unreachable/no-path effect.
func (e CaptureEffects) IsBottom() bool { return e.bottom }

// WithMustWrite appends a sequential strong write to e.
func (e CaptureEffects) WithMustWrite(sym cfg.SymbolID, v product.AbstractValue) CaptureEffects {
	return e.Then(CaptureMustWrite(sym, v))
}

// Apply applies e to a captured-cell store.
func (e CaptureEffects) Apply(c CaptureCells) CaptureCells {
	if e.bottom {
		return c
	}
	if e.top {
		return CaptureCellsTop()
	}
	if c.top {
		return c
	}
	out := c
	for _, entry := range e.entries {
		if entry.MustWrite {
			out = out.With(entry.Symbol, entry.Value)
			continue
		}
		prev, _ := out.Value(entry.Symbol)
		out = out.With(entry.Symbol, product.Domain.Join(prev, entry.Value))
	}
	return out
}

// Then composes e followed by next.
func (e CaptureEffects) Then(next CaptureEffects) CaptureEffects {
	if e.bottom || next.bottom {
		return CaptureEffects{bottom: true}
	}
	if e.top || next.top {
		return CaptureEffectsTop()
	}
	var out []CaptureEffect
	i, j := 0, 0
	for i < len(e.entries) || j < len(next.entries) {
		switch {
		case j >= len(next.entries) || (i < len(e.entries) && e.entries[i].Symbol < next.entries[j].Symbol):
			out = append(out, e.entries[i])
			i++
		case i >= len(e.entries) || next.entries[j].Symbol < e.entries[i].Symbol:
			out = append(out, next.entries[j])
			j++
		default:
			out = append(out, composeCaptureEffect(e.entries[i], next.entries[j]))
			i++
			j++
		}
	}
	return canonicalCaptureEffects(out, joinCaptureEffect)
}

// CooccurringCaptureEffects joins two caller-visible effects whose relative
// execution order is unknown. It is the sound commutative abstraction of
// "a then b" or "b then a" and belongs to the CaptureEffects carrier because it
// is sequential transformer algebra, not call-driver policy.
func CooccurringCaptureEffects(a, b CaptureEffects) CaptureEffects {
	if CaptureEffectsDomain.Equal(a, CaptureEffectsDomain.Bottom()) {
		return b
	}
	if CaptureEffectsDomain.Equal(b, CaptureEffectsDomain.Bottom()) {
		return a
	}
	ab := a.Then(b)
	ba := b.Then(a)
	return CaptureEffectsDomain.Join(ab, ba)
}

// Format renders e deterministically for law-test diagnostics and journal notes.
func (e CaptureEffects) Format() string {
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
		parts = append(parts, fmt.Sprintf("%d:%s:%s", entry.Symbol, mode, entry.Value.ProjectValue()))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

// CaptureEffectsDomain is the lattice of caller-visible capture-cell effects.
var CaptureEffectsDomain = lattice.Lattice[CaptureEffects]{
	Bottom: func() CaptureEffects {
		return CaptureEffects{bottom: true}
	},
	Top: CaptureEffectsTop,
	Equal: func(a, b CaptureEffects) bool {
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
			if a.entries[i].Symbol != b.entries[i].Symbol ||
				a.entries[i].MustWrite != b.entries[i].MustWrite ||
				!product.Domain.Equal(a.entries[i].Value, b.entries[i].Value) {
				return false
			}
		}
		return true
	},
	LessOrEq: func(a, b CaptureEffects) bool {
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
			case j >= len(b.entries) || (i < len(a.entries) && a.entries[i].Symbol < b.entries[j].Symbol):
				return false
			case i >= len(a.entries) || b.entries[j].Symbol < a.entries[i].Symbol:
				if !identityLessOrEqCaptureEffect(b.entries[j]) {
					return false
				}
				j++
			default:
				if !captureEffectLessOrEq(a.entries[i], b.entries[j]) {
					return false
				}
				i++
				j++
			}
		}
		return true
	},
	Join: func(a, b CaptureEffects) CaptureEffects {
		if a.bottom {
			return b
		}
		if b.bottom {
			return a
		}
		if a.top || b.top {
			return CaptureEffectsTop()
		}
		return combineCaptureEffects(a, b, joinCaptureEffect)
	},
	Meet: nil,
	Widen: func(prev, next CaptureEffects) CaptureEffects {
		if prev.bottom {
			return next
		}
		if next.bottom {
			return prev
		}
		if prev.top || next.top {
			return CaptureEffectsTop()
		}
		return combineCaptureEffects(prev, next, widenCaptureEffect)
	},
}

func identityLessOrEqCaptureEffect(e CaptureEffect) bool {
	return !e.MustWrite
}

func captureEffectLessOrEq(a, b CaptureEffect) bool {
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

func composeCaptureEffect(prev, next CaptureEffect) CaptureEffect {
	if next.MustWrite {
		return next
	}
	value := next.Value
	must := false
	if prev.MustWrite {
		value = product.Domain.Join(prev.Value, next.Value)
		must = true
	} else {
		value = product.Domain.Join(prev.Value, next.Value)
	}
	return CaptureEffect{Symbol: next.Symbol, Value: value, MustWrite: must}
}

func joinCaptureEffect(a, b CaptureEffect) CaptureEffect {
	return CaptureEffect{
		Symbol:    a.Symbol,
		Value:     product.Domain.Join(a.Value, b.Value),
		MustWrite: a.MustWrite && b.MustWrite,
	}
}

func widenCaptureEffect(prev, next CaptureEffect) CaptureEffect {
	return CaptureEffect{
		Symbol:    prev.Symbol,
		Value:     product.Domain.Widen(prev.Value, next.Value),
		MustWrite: prev.MustWrite && next.MustWrite,
	}
}

func combineCaptureEffects(a, b CaptureEffects, op func(CaptureEffect, CaptureEffect) CaptureEffect) CaptureEffects {
	var out []CaptureEffect
	i, j := 0, 0
	for i < len(a.entries) || j < len(b.entries) {
		switch {
		case j >= len(b.entries) || (i < len(a.entries) && a.entries[i].Symbol < b.entries[j].Symbol):
			out = append(out, CaptureEffect{Symbol: a.entries[i].Symbol, Value: a.entries[i].Value, MustWrite: false})
			i++
		case i >= len(a.entries) || b.entries[j].Symbol < a.entries[i].Symbol:
			out = append(out, CaptureEffect{Symbol: b.entries[j].Symbol, Value: b.entries[j].Value, MustWrite: false})
			j++
		default:
			out = append(out, op(a.entries[i], b.entries[j]))
			i++
			j++
		}
	}
	return canonicalCaptureEffects(out, joinCaptureEffect)
}

func canonicalCaptureEffects(entries []CaptureEffect, merge func(CaptureEffect, CaptureEffect) CaptureEffect) CaptureEffects {
	if len(entries) == 0 {
		return CaptureEffectsIdentity()
	}
	out := append([]CaptureEffect(nil), entries...)
	sortCaptureEffects(out)
	dst := out[:0]
	for _, e := range out {
		if e.Symbol == 0 || valueIsBottom(e.Value) {
			continue
		}
		if len(dst) > 0 && dst[len(dst)-1].Symbol == e.Symbol {
			dst[len(dst)-1] = merge(dst[len(dst)-1], e)
			if valueIsBottom(dst[len(dst)-1].Value) {
				dst = dst[:len(dst)-1]
			}
			continue
		}
		dst = append(dst, e)
	}
	return CaptureEffects{entries: append([]CaptureEffect(nil), dst...)}
}

func sortCaptureEffects(entries []CaptureEffect) {
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0 && cmp.Compare(entries[j].Symbol, entries[j-1].Symbol) < 0; j-- {
			entries[j], entries[j-1] = entries[j-1], entries[j]
		}
	}
}
