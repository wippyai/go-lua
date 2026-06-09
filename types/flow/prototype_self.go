package flow

import (
	"cmp"
	"fmt"
	"strings"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/lattice"
)

// PrototypeSelfEntry is one abstract receiver-self value for a Lua method
// prototype table. Prototype is the stable symbol of the method table, and Value
// is the runtime instance value that enters methods as self.
type PrototypeSelfEntry struct {
	Prototype cfg.SymbolID
	Value     product.AbstractValue
}

// PrototypeSelf is a deterministic finite-map lattice:
//
//	prototype symbol -> runtime self value
//
// It is the product-state owner for split-pattern Lua OOP receiver semantics
// (`local mt = {__index = methods}; return setmetatable(instance, mt)`). An
// absent prototype denotes product.Bottom(), not Lua nil.
type PrototypeSelf struct {
	top     bool
	entries []PrototypeSelfEntry
}

// PrototypeSelfOf constructs a canonical finite receiver map. Duplicate
// prototypes are joined; zero, bottom, and symbol-0 entries are dropped.
func PrototypeSelfOf(entries []PrototypeSelfEntry) PrototypeSelf {
	return canonicalPrototypeSelf(entries, product.Domain.Join)
}

// PrototypeSelfTop returns the greatest receiver-self relation.
func PrototypeSelfTop() PrototypeSelf {
	return PrototypeSelf{top: true}
}

// PrototypeSelfAxisOf returns the receiver-self axis carried by state.
func PrototypeSelfAxisOf(state PointState) PrototypeSelf {
	return state.PrototypeSelf
}

// PrototypeSelfOfPoint returns the receiver-self axis carried by state. Nil
// points represent an empty receiver-self relation.
func PrototypeSelfOfPoint(state *PointState) PrototypeSelf {
	if state == nil {
		return PrototypeSelfDomain.Bottom()
	}
	return state.PrototypeSelf
}

// ReceiverSelfValueOfPoint returns proto's receiver-self value from state.
func ReceiverSelfValueOfPoint(state *PointState, proto cfg.SymbolID) (product.AbstractValue, bool) {
	if proto == 0 {
		return product.AbstractValue{}, false
	}
	av, ok := PrototypeSelfOfPoint(state).Value(proto)
	if !ok || av.IsZero() {
		return product.AbstractValue{}, false
	}
	return av, true
}

func updatePrototypeSelf(state *PointState, update func(PrototypeSelf) PrototypeSelf) bool {
	if state == nil || update == nil {
		return false
	}
	before := state.PrototypeSelf
	state.PrototypeSelf = update(state.PrototypeSelf)
	return !PrototypeSelfDomain.Equal(before, state.PrototypeSelf)
}

// JoinPrototypeSelf joins self into state's receiver-self axis.
func JoinPrototypeSelf(state *PointState, self PrototypeSelf) bool {
	if PrototypeSelfDomain.Equal(self, PrototypeSelfDomain.Bottom()) {
		return false
	}
	return updatePrototypeSelf(state, func(current PrototypeSelf) PrototypeSelf {
		return PrototypeSelfDomain.Join(current, self)
	})
}

// RecordPrototypeSelf joins value into proto's receiver-self value.
func RecordPrototypeSelf(state *PointState, proto cfg.SymbolID, value product.AbstractValue) bool {
	if state == nil || proto == 0 || value.IsZero() {
		return false
	}
	return updatePrototypeSelf(state, func(current PrototypeSelf) PrototypeSelf {
		return current.JoinValue(proto, value)
	})
}

// Entries returns a copy of the sorted finite entries. Top has no finite entry
// representation and returns nil.
func (p PrototypeSelf) Entries() []PrototypeSelfEntry {
	if p.top || len(p.entries) == 0 {
		return nil
	}
	return append([]PrototypeSelfEntry(nil), p.entries...)
}

// IsTop reports whether p is the greatest receiver-self relation.
func (p PrototypeSelf) IsTop() bool { return p.top }

// Value returns the runtime self value for proto. For absent finite entries it
// returns product.Bottom() and ok=false; for Top it returns product.Top() and
// ok=true.
func (p PrototypeSelf) Value(proto cfg.SymbolID) (product.AbstractValue, bool) {
	if proto == 0 {
		return product.Domain.Bottom(), false
	}
	if p.top {
		return product.Domain.Top(), true
	}
	idx, ok := findPrototypeSelf(p.entries, proto)
	if !ok {
		return product.Domain.Bottom(), false
	}
	return p.entries[idx].Value, true
}

// With returns p with proto strongly updated to v. Updating a finite entry to
// Bottom removes it, preserving absent-is-bottom canonical form.
func (p PrototypeSelf) With(proto cfg.SymbolID, v product.AbstractValue) PrototypeSelf {
	if p.top {
		return p
	}
	next := p.Entries()
	idx, ok := findPrototypeSelf(next, proto)
	switch {
	case proto == 0 || valueIsBottom(v):
		if ok {
			next = append(next[:idx], next[idx+1:]...)
		}
	case ok:
		next[idx].Value = v
	default:
		next = append(next, PrototypeSelfEntry{Prototype: proto, Value: v})
	}
	return canonicalPrototypeSelf(next, product.Domain.Join)
}

// JoinValue joins v into proto's current receiver-self value.
func (p PrototypeSelf) JoinValue(proto cfg.SymbolID, v product.AbstractValue) PrototypeSelf {
	if p.top {
		return p
	}
	prev, _ := p.Value(proto)
	return p.With(proto, product.Domain.Join(prev, v))
}

// Format renders p deterministically for law-test diagnostics and journal notes.
func (p PrototypeSelf) Format() string {
	if p.top {
		return "⊤"
	}
	if len(p.entries) == 0 {
		return "⊥"
	}
	parts := make([]string, 0, len(p.entries))
	for _, e := range p.entries {
		parts = append(parts, fmt.Sprintf("%d:%s", e.Prototype, e.Value.ProjectValue()))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

// PrototypeSelfDomain is the finite-map lattice over prototype receiver values.
var PrototypeSelfDomain = lattice.Lattice[PrototypeSelf]{
	Bottom: func() PrototypeSelf {
		return PrototypeSelf{}
	},
	Top: PrototypeSelfTop,
	Equal: func(a, b PrototypeSelf) bool {
		if a.top || b.top {
			return a.top && b.top
		}
		if len(a.entries) != len(b.entries) {
			return false
		}
		for i := range a.entries {
			if a.entries[i].Prototype != b.entries[i].Prototype ||
				!product.Domain.Equal(a.entries[i].Value, b.entries[i].Value) {
				return false
			}
		}
		return true
	},
	LessOrEq: func(a, b PrototypeSelf) bool {
		if b.top {
			return true
		}
		if a.top {
			return false
		}
		return prototypeSelfPointwise(a, b, product.Domain.LessOrEq)
	},
	Join: func(a, b PrototypeSelf) PrototypeSelf {
		if a.top || b.top {
			return PrototypeSelfTop()
		}
		return combinePrototypeSelf(a, b, product.Domain.Join)
	},
	Meet: nil,
	Widen: func(prev, next PrototypeSelf) PrototypeSelf {
		if prev.top || next.top {
			return PrototypeSelfTop()
		}
		return combinePrototypeSelf(prev, next, product.Domain.Widen)
	},
}

func findPrototypeSelf(entries []PrototypeSelfEntry, proto cfg.SymbolID) (int, bool) {
	lo, hi := 0, len(entries)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		if entries[mid].Prototype < proto {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo, lo < len(entries) && entries[lo].Prototype == proto
}

func canonicalPrototypeSelf(entries []PrototypeSelfEntry, merge func(product.AbstractValue, product.AbstractValue) product.AbstractValue) PrototypeSelf {
	if len(entries) == 0 {
		return PrototypeSelf{}
	}
	out := append([]PrototypeSelfEntry(nil), entries...)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && cmp.Compare(out[j].Prototype, out[j-1].Prototype) < 0; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	dst := out[:0]
	for _, e := range out {
		if e.Prototype == 0 || valueIsBottom(e.Value) {
			continue
		}
		if len(dst) > 0 && dst[len(dst)-1].Prototype == e.Prototype {
			dst[len(dst)-1].Value = merge(dst[len(dst)-1].Value, e.Value)
			if valueIsBottom(dst[len(dst)-1].Value) {
				dst = dst[:len(dst)-1]
			}
			continue
		}
		dst = append(dst, e)
	}
	return PrototypeSelf{entries: append([]PrototypeSelfEntry(nil), dst...)}
}

func combinePrototypeSelf(a, b PrototypeSelf, op func(product.AbstractValue, product.AbstractValue) product.AbstractValue) PrototypeSelf {
	var out []PrototypeSelfEntry
	i, j := 0, 0
	bottom := product.Domain.Bottom()
	for i < len(a.entries) || j < len(b.entries) {
		switch {
		case j >= len(b.entries) || (i < len(a.entries) && a.entries[i].Prototype < b.entries[j].Prototype):
			out = append(out, PrototypeSelfEntry{Prototype: a.entries[i].Prototype, Value: op(a.entries[i].Value, bottom)})
			i++
		case i >= len(a.entries) || b.entries[j].Prototype < a.entries[i].Prototype:
			out = append(out, PrototypeSelfEntry{Prototype: b.entries[j].Prototype, Value: op(bottom, b.entries[j].Value)})
			j++
		default:
			out = append(out, PrototypeSelfEntry{Prototype: a.entries[i].Prototype, Value: op(a.entries[i].Value, b.entries[j].Value)})
			i++
			j++
		}
	}
	return canonicalPrototypeSelf(out, product.Domain.Join)
}

func prototypeSelfPointwise(a, b PrototypeSelf, pred func(product.AbstractValue, product.AbstractValue) bool) bool {
	i, j := 0, 0
	bottom := product.Domain.Bottom()
	for i < len(a.entries) || j < len(b.entries) {
		switch {
		case j >= len(b.entries) || (i < len(a.entries) && a.entries[i].Prototype < b.entries[j].Prototype):
			if !pred(a.entries[i].Value, bottom) {
				return false
			}
			i++
		case i >= len(a.entries) || b.entries[j].Prototype < a.entries[i].Prototype:
			if !pred(bottom, b.entries[j].Value) {
				return false
			}
			j++
		default:
			if !pred(a.entries[i].Value, b.entries[j].Value) {
				return false
			}
			i++
			j++
		}
	}
	return true
}

// PrototypeInstanceEntry records that a point-local storage symbol currently
// denotes an instance whose metatable routes method lookup through one or more
// prototype tables.
type PrototypeInstanceEntry struct {
	Symbol     cfg.SymbolID
	Prototypes []cfg.SymbolID
}

// PrototypeInstances is a deterministic finite-map lattice:
//
//	storage symbol -> prototype symbols
//
// It is the flow-sensitive companion to PrototypeSelf. PrototypeSelf carries the
// abstract runtime self value for a prototype at method-entry/publication
// boundaries; PrototypeInstances remembers which local storage places are
// currently bound to a prototype while construction proceeds. Ordinary writes to
// those locals update the local value only. Publication into PrototypeSelf happens
// when the instance is returned/escapes, while writes to an actual method-entry
// self slot publish immediately.
type PrototypeInstances struct {
	top     bool
	entries []PrototypeInstanceEntry
}

// PrototypeInstancesOf constructs a canonical finite instance map.
func PrototypeInstancesOf(entries []PrototypeInstanceEntry) PrototypeInstances {
	return canonicalPrototypeInstances(entries)
}

// PrototypeInstancesTop returns the greatest instance/prototype relation.
func PrototypeInstancesTop() PrototypeInstances { return PrototypeInstances{top: true} }

// PrototypeInstancesAxisOf returns the instance/prototype axis carried by state.
func PrototypeInstancesAxisOf(state PointState) PrototypeInstances {
	return state.PrototypeInstances
}

// PrototypeInstancesOfPoint returns the instance/prototype axis carried by
// state. Nil points represent an empty instance/prototype relation.
func PrototypeInstancesOfPoint(state *PointState) PrototypeInstances {
	if state == nil {
		return PrototypeInstancesDomain.Bottom()
	}
	return state.PrototypeInstances
}

// PrototypeInstancePrototypesOfPoint returns sym's prototype set from state.
func PrototypeInstancePrototypesOfPoint(state *PointState, sym cfg.SymbolID) ([]cfg.SymbolID, bool) {
	if sym == 0 {
		return nil, false
	}
	return PrototypeInstancesOfPoint(state).Prototypes(sym)
}

func updatePrototypeInstances(state *PointState, update func(PrototypeInstances) PrototypeInstances) bool {
	if state == nil || update == nil {
		return false
	}
	before := state.PrototypeInstances
	state.PrototypeInstances = update(state.PrototypeInstances)
	return !PrototypeInstancesDomain.Equal(before, state.PrototypeInstances)
}

// JoinPrototypeInstances joins instances into state's instance/prototype axis.
func JoinPrototypeInstances(state *PointState, instances PrototypeInstances) bool {
	if PrototypeInstancesDomain.Equal(instances, PrototypeInstancesDomain.Bottom()) {
		return false
	}
	return updatePrototypeInstances(state, func(current PrototypeInstances) PrototypeInstances {
		return PrototypeInstancesDomain.Join(current, instances)
	})
}

// BindPrototypeInstance strongly binds sym to proto in state's instance map.
func BindPrototypeInstance(state *PointState, sym cfg.SymbolID, proto cfg.SymbolID) bool {
	if state == nil || sym == 0 || proto == 0 {
		return false
	}
	return updatePrototypeInstances(state, func(current PrototypeInstances) PrototypeInstances {
		return current.WithPrototype(sym, proto)
	})
}

// ClearPrototypeInstance removes sym from state's instance map.
func ClearPrototypeInstance(state *PointState, sym cfg.SymbolID) bool {
	if state == nil || sym == 0 {
		return false
	}
	return updatePrototypeInstances(state, func(current PrototypeInstances) PrototypeInstances {
		return current.With(sym, nil)
	})
}

// Entries returns a copy of the sorted finite entries. Top has no finite entry
// representation.
func (p PrototypeInstances) Entries() []PrototypeInstanceEntry {
	if p.top || len(p.entries) == 0 {
		return nil
	}
	out := make([]PrototypeInstanceEntry, 0, len(p.entries))
	for _, e := range p.entries {
		out = append(out, PrototypeInstanceEntry{
			Symbol:     e.Symbol,
			Prototypes: append([]cfg.SymbolID(nil), e.Prototypes...),
		})
	}
	return out
}

// Prototypes returns the prototype set for sym. For Top it returns nil,true to
// avoid inventing finite symbols.
func (p PrototypeInstances) Prototypes(sym cfg.SymbolID) ([]cfg.SymbolID, bool) {
	if sym == 0 {
		return nil, false
	}
	if p.top {
		return nil, true
	}
	idx, ok := findPrototypeInstance(p.entries, sym)
	if !ok {
		return nil, false
	}
	return append([]cfg.SymbolID(nil), p.entries[idx].Prototypes...), true
}

// With strongly updates sym's prototype set. An empty set removes the finite
// entry, preserving absent-is-bottom canonical form.
func (p PrototypeInstances) With(sym cfg.SymbolID, protos []cfg.SymbolID) PrototypeInstances {
	if p.top {
		return p
	}
	next := p.Entries()
	idx, ok := findPrototypeInstance(next, sym)
	canon := canonicalPrototypeSet(protos)
	switch {
	case sym == 0 || len(canon) == 0:
		if ok {
			next = append(next[:idx], next[idx+1:]...)
		}
	case ok:
		next[idx].Prototypes = canon
	default:
		next = append(next, PrototypeInstanceEntry{Symbol: sym, Prototypes: canon})
	}
	return canonicalPrototypeInstances(next)
}

// WithPrototype strongly updates sym to denote exactly proto.
func (p PrototypeInstances) WithPrototype(sym cfg.SymbolID, proto cfg.SymbolID) PrototypeInstances {
	if proto == 0 {
		return p.With(sym, nil)
	}
	return p.With(sym, []cfg.SymbolID{proto})
}

// Format renders p deterministically for tests and diagnostics.
func (p PrototypeInstances) Format() string {
	if p.top {
		return "⊤"
	}
	if len(p.entries) == 0 {
		return "⊥"
	}
	parts := make([]string, 0, len(p.entries))
	for _, e := range p.entries {
		protos := make([]string, 0, len(e.Prototypes))
		for _, proto := range e.Prototypes {
			protos = append(protos, fmt.Sprintf("%d", proto))
		}
		parts = append(parts, fmt.Sprintf("%d:[%s]", e.Symbol, strings.Join(protos, ",")))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

// PrototypeInstancesDomain is the finite-map lattice over point-local
// instance/prototype bindings. Join and Widen are both finite set union.
var PrototypeInstancesDomain = lattice.Lattice[PrototypeInstances]{
	Bottom: func() PrototypeInstances {
		return PrototypeInstances{}
	},
	Top: PrototypeInstancesTop,
	Equal: func(a, b PrototypeInstances) bool {
		if a.top || b.top {
			return a.top && b.top
		}
		if len(a.entries) != len(b.entries) {
			return false
		}
		for i := range a.entries {
			if a.entries[i].Symbol != b.entries[i].Symbol || !prototypeSetEqual(a.entries[i].Prototypes, b.entries[i].Prototypes) {
				return false
			}
		}
		return true
	},
	LessOrEq: func(a, b PrototypeInstances) bool {
		if b.top {
			return true
		}
		if a.top {
			return false
		}
		return prototypeInstancesPointwise(a, b, prototypeSetSubset)
	},
	Join: func(a, b PrototypeInstances) PrototypeInstances {
		if a.top || b.top {
			return PrototypeInstancesTop()
		}
		return combinePrototypeInstances(a, b, prototypeSetUnion)
	},
	Meet: nil,
	Widen: func(prev, next PrototypeInstances) PrototypeInstances {
		if prev.top || next.top {
			return PrototypeInstancesTop()
		}
		return combinePrototypeInstances(prev, next, prototypeSetUnion)
	},
}

func findPrototypeInstance(entries []PrototypeInstanceEntry, sym cfg.SymbolID) (int, bool) {
	lo, hi := 0, len(entries)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		if entries[mid].Symbol < sym {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo, lo < len(entries) && entries[lo].Symbol == sym
}

func canonicalPrototypeInstances(entries []PrototypeInstanceEntry) PrototypeInstances {
	if len(entries) == 0 {
		return PrototypeInstances{}
	}
	out := make([]PrototypeInstanceEntry, 0, len(entries))
	for _, e := range entries {
		protos := canonicalPrototypeSet(e.Prototypes)
		if e.Symbol == 0 || len(protos) == 0 {
			continue
		}
		out = append(out, PrototypeInstanceEntry{Symbol: e.Symbol, Prototypes: protos})
	}
	if len(out) == 0 {
		return PrototypeInstances{}
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && cmp.Compare(out[j].Symbol, out[j-1].Symbol) < 0; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	dst := out[:0]
	for _, e := range out {
		if len(dst) > 0 && dst[len(dst)-1].Symbol == e.Symbol {
			dst[len(dst)-1].Prototypes = prototypeSetUnion(dst[len(dst)-1].Prototypes, e.Prototypes)
			continue
		}
		dst = append(dst, e)
	}
	return PrototypeInstances{entries: append([]PrototypeInstanceEntry(nil), dst...)}
}

func combinePrototypeInstances(a, b PrototypeInstances, op func([]cfg.SymbolID, []cfg.SymbolID) []cfg.SymbolID) PrototypeInstances {
	var out []PrototypeInstanceEntry
	i, j := 0, 0
	for i < len(a.entries) || j < len(b.entries) {
		switch {
		case j >= len(b.entries) || (i < len(a.entries) && a.entries[i].Symbol < b.entries[j].Symbol):
			out = append(out, PrototypeInstanceEntry{Symbol: a.entries[i].Symbol, Prototypes: op(a.entries[i].Prototypes, nil)})
			i++
		case i >= len(a.entries) || b.entries[j].Symbol < a.entries[i].Symbol:
			out = append(out, PrototypeInstanceEntry{Symbol: b.entries[j].Symbol, Prototypes: op(nil, b.entries[j].Prototypes)})
			j++
		default:
			out = append(out, PrototypeInstanceEntry{Symbol: a.entries[i].Symbol, Prototypes: op(a.entries[i].Prototypes, b.entries[j].Prototypes)})
			i++
			j++
		}
	}
	return canonicalPrototypeInstances(out)
}

func prototypeInstancesPointwise(a, b PrototypeInstances, pred func([]cfg.SymbolID, []cfg.SymbolID) bool) bool {
	i, j := 0, 0
	for i < len(a.entries) || j < len(b.entries) {
		switch {
		case j >= len(b.entries) || (i < len(a.entries) && a.entries[i].Symbol < b.entries[j].Symbol):
			if !pred(a.entries[i].Prototypes, nil) {
				return false
			}
			i++
		case i >= len(a.entries) || b.entries[j].Symbol < a.entries[i].Symbol:
			if !pred(nil, b.entries[j].Prototypes) {
				return false
			}
			j++
		default:
			if !pred(a.entries[i].Prototypes, b.entries[j].Prototypes) {
				return false
			}
			i++
			j++
		}
	}
	return true
}

func canonicalPrototypeSet(in []cfg.SymbolID) []cfg.SymbolID {
	if len(in) == 0 {
		return nil
	}
	out := append([]cfg.SymbolID(nil), in...)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	dst := out[:0]
	for _, proto := range out {
		if proto == 0 || (len(dst) > 0 && dst[len(dst)-1] == proto) {
			continue
		}
		dst = append(dst, proto)
	}
	return append([]cfg.SymbolID(nil), dst...)
}

func prototypeSetUnion(a, b []cfg.SymbolID) []cfg.SymbolID {
	return canonicalPrototypeSet(append(append([]cfg.SymbolID(nil), a...), b...))
}

func prototypeSetSubset(a, b []cfg.SymbolID) bool {
	bb := canonicalPrototypeSet(b)
	for _, proto := range canonicalPrototypeSet(a) {
		idx, ok := binarySearchSymbol(bb, proto)
		if !ok || bb[idx] != proto {
			return false
		}
	}
	return true
}

func prototypeSetEqual(a, b []cfg.SymbolID) bool {
	aa := canonicalPrototypeSet(a)
	bb := canonicalPrototypeSet(b)
	if len(aa) != len(bb) {
		return false
	}
	for i := range aa {
		if aa[i] != bb[i] {
			return false
		}
	}
	return true
}

func binarySearchSymbol(vals []cfg.SymbolID, target cfg.SymbolID) (int, bool) {
	lo, hi := 0, len(vals)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		if vals[mid] < target {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo, lo < len(vals) && vals[lo] == target
}
