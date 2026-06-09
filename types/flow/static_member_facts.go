package flow

import (
	"fmt"
	"strings"

	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/lattice"
)

// StaticMemberFact is one must-fact about a statically-known member path at a
// program point. Path is a structural path key, so `.x`, `["x"]`, `[""]`, and
// `[1]` are distinct facts. Value is the product-domain value proven for reads
// at that path on the current edge.
type StaticMemberFact struct {
	Path  constraint.PathKey
	Value product.AbstractValue
}

// StaticMemberAddressFact is the address-domain view of one materialized
// static-member fact.
type StaticMemberAddressFact struct {
	Address StableAddress
	Value   product.AbstractValue
}

// StaticMemberFacts is the point-local must-fact domain for guarded static member
// precision. It exists to keep exact-key presence/value evidence out of record
// field overlays: a branch can prove `m["k"]` present without pretending the
// structural bracket key is a dot field.
//
// The carrier is ordered like PointRelations: Bottom is unreachable, Top is the
// empty fact set, and finite states are sorted path facts. Join intersects paths
// proven on every predecessor and joins their values.
type StaticMemberFacts struct {
	bottom  bool
	entries []StaticMemberFact
}

// StaticMemberFactsOf constructs a canonical finite fact set. Duplicate paths are
// joined. Empty, zero, and bottom facts are dropped.
func StaticMemberFactsOf(entries []StaticMemberFact) StaticMemberFacts {
	return canonicalStaticMemberFacts(entries, product.Domain.Join)
}

// IsBottom reports the unreachable fact sentinel.
func (f StaticMemberFacts) IsBottom() bool { return f.bottom }

// HasProof reports whether f carries any finite static-member fact in a
// reachable state. Top/empty means no definite member proof.
func (f StaticMemberFacts) HasProof() bool {
	return !f.bottom && len(f.entries) > 0
}

// Entries returns a defensive copy of finite entries. Bottom has no consumable
// finite entries.
func (f StaticMemberFacts) Entries() []StaticMemberFact {
	if f.bottom || len(f.entries) == 0 {
		return nil
	}
	return append([]StaticMemberFact(nil), f.entries...)
}

// ValueAtAddress returns the fact for addr if it is proven in a reachable state.
func (f StaticMemberFacts) ValueAtAddress(addr StableAddress) (product.AbstractValue, bool) {
	if f.bottom || len(f.entries) == 0 {
		return product.Domain.Bottom(), false
	}
	key := addr.Key()
	if key == "" {
		return product.Domain.Bottom(), false
	}
	idx, ok := findStaticMemberFact(f.entries, key)
	if !ok {
		return product.Domain.Bottom(), false
	}
	return f.entries[idx].Value, true
}

// WithAddress returns f with addr strongly updated to value. Updating to Bottom
// removes the key, preserving absent-is-no-fact canonical form.
func (f StaticMemberFacts) WithAddress(addr StableAddress, value product.AbstractValue) StaticMemberFacts {
	if f.bottom {
		f = StaticMemberFacts{}
	}
	path := addr.Key()
	next := f.Entries()
	idx, ok := findStaticMemberFact(next, path)
	switch {
	case path == "" || valueIsBottom(value):
		if ok {
			next = append(next[:idx], next[idx+1:]...)
		}
	case ok:
		next[idx].Value = value
	default:
		next = append(next, StaticMemberFact{Path: path, Value: value})
	}
	return canonicalStaticMemberFacts(next, product.Domain.Join)
}

// KillSubtreeAddress removes facts at root and under root's structural suffix.
func (f StaticMemberFacts) KillSubtreeAddress(root StableAddress) StaticMemberFacts {
	if f.bottom || len(f.entries) == 0 {
		return f
	}
	out := make([]StaticMemberFact, 0, len(f.entries))
	for _, e := range f.entries {
		pathAddr, ok := StableAddressFromCanonicalKey(e.Path)
		if ok && pathAddr.HasPrefix(root) {
			continue
		}
		out = append(out, e)
	}
	return canonicalStaticMemberFacts(out, product.Domain.Join)
}

// AddressEntriesUnder returns materialized facts at or below root as structured
// address facts.
func (f StaticMemberFacts) AddressEntriesUnder(root StableAddress) []StaticMemberAddressFact {
	if f.bottom || root.Key() == "" || len(f.entries) == 0 {
		return nil
	}
	var out []StaticMemberAddressFact
	for _, entry := range f.entries {
		addr, ok := StableAddressFromCanonicalKey(entry.Path)
		if !ok || !addr.HasPrefix(root) {
			continue
		}
		out = append(out, StaticMemberAddressFact{
			Address: addr,
			Value:   entry.Value,
		})
	}
	return out
}

// RebaseStaticMemberFactsUnder moves facts proven below source onto target,
// preserving the member/index suffix below source. The source root itself is not
// replayed: root values travel through the product value carrier, while this
// domain owns child path facts.
func RebaseStaticMemberFactsUnder(facts StaticMemberFacts, source, target StableAddress) StaticMemberFacts {
	if facts.bottom {
		return StaticMemberFactsDomain.Bottom()
	}
	if source.Key() == "" || target.Key() == "" {
		return StaticMemberFactsDomain.Top()
	}
	out := StaticMemberFactsDomain.Top()
	for _, fact := range facts.AddressEntriesUnder(source) {
		remainder, ok := fact.Address.RemainderAfterPrefix(source)
		if !ok || len(remainder) == 0 || fact.Value.IsZero() {
			continue
		}
		rebased, ok := target.Append(remainder)
		if !ok {
			continue
		}
		out = out.WithAddress(rebased, fact.Value)
	}
	return out
}

// ApplyStaticMemberFacts materializes a finite static-member fact set into a
// point state. It is the caller-side counterpart of summary return static-member
// replay after slot-relative facts have been rebased to concrete local targets.
func ApplyStaticMemberFacts(out *PointState, facts StaticMemberFacts) bool {
	if out == nil || facts.bottom {
		return false
	}
	changed := false
	for _, fact := range facts.Entries() {
		addr, ok := StableAddressFromCanonicalKey(fact.Path)
		if !ok || fact.Value.IsZero() {
			continue
		}
		changed = SetStaticMemberFact(out, addr, fact.Value) || changed
	}
	return changed
}

// DirectChildAddressesUnder returns the direct child addresses that have any
// materialized static-member fact below parent.
func (f StaticMemberFacts) DirectChildAddressesUnder(parent StableAddress) []StableAddress {
	if f.bottom || parent.Key() == "" || len(f.entries) == 0 {
		return nil
	}
	seen := make(map[constraint.PathKey]struct{})
	var out []StableAddress
	for _, entry := range f.entries {
		addr, ok := StableAddressFromCanonicalKey(entry.Path)
		if !ok {
			continue
		}
		remainder, ok := addr.RemainderAfterPrefix(parent)
		if !ok || len(remainder) == 0 {
			continue
		}
		child, ok := parent.Append(remainder[:1])
		if !ok {
			continue
		}
		key := child.Key()
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, child)
	}
	return out
}

// StaticMembersOf returns the static-member axis carried by state.
func StaticMembersOf(state PointState) StaticMemberFacts {
	return state.StaticMembers
}

// StaticMembersOfPoint returns the static-member axis carried by state.
func StaticMembersOfPoint(state *PointState) StaticMemberFacts {
	if state == nil {
		return StaticMemberFactsDomain.Top()
	}
	return state.StaticMembers
}

// StaticMemberAxisIsBottom reports whether the static-member axis is
// unreachable.
func StaticMemberAxisIsBottom(state *PointState) bool {
	return state == nil || state.StaticMembers.IsBottom()
}

// LiftStaticMembersEntry turns the unreachable entry seed into the reachable
// identity element for the static-member axis.
func LiftStaticMembersEntry(state *PointState) bool {
	if state == nil || !state.StaticMembers.IsBottom() {
		return false
	}
	state.StaticMembers = StaticMemberFactsDomain.Top()
	return true
}

func (f StaticMemberFacts) coversWithPresentValues(
	want StaticMemberFacts,
	present addressPresentValueProof,
	pred func(product.AbstractValue, product.AbstractValue) bool,
) bool {
	if f.bottom {
		return true
	}
	if want.bottom {
		return false
	}
	for _, entry := range want.entries {
		if idx, ok := findStaticMemberFact(f.entries, entry.Path); ok {
			if pred(f.entries[idx].Value, entry.Value) {
				continue
			}
		}
		if value, ok := staticMemberPresentValue(present, entry.Path); ok && pred(value, entry.Value) {
			continue
		}
		return false
	}
	return true
}

func (f StaticMemberFacts) withFactsMergedFromPresentValues(
	facts StaticMemberFacts,
	present addressPresentValueProof,
	op func(product.AbstractValue, product.AbstractValue) product.AbstractValue,
) StaticMemberFacts {
	if facts.bottom {
		return f
	}
	out := f
	for _, entry := range facts.entries {
		if _, ok := findStaticMemberFact(out.entries, entry.Path); ok {
			continue
		}
		value, ok := staticMemberPresentValue(present, entry.Path)
		if !ok {
			continue
		}
		joined := op(entry.Value, value)
		if joined.IsZero() || joined.IsBottom() {
			continue
		}
		addr, ok := StableAddressFromCanonicalKey(entry.Path)
		if !ok {
			continue
		}
		out = out.WithAddress(addr, joined)
	}
	return out
}

func staticMemberPresentValue(
	present addressPresentValueProof,
	key constraint.PathKey,
) (product.AbstractValue, bool) {
	if present == nil {
		return product.AbstractValue{}, false
	}
	addr, ok := StableAddressFromCanonicalKey(key)
	if !ok {
		return product.AbstractValue{}, false
	}
	return present(addr)
}

// Format renders f deterministically for tests and diagnostics.
func (f StaticMemberFacts) Format() string {
	if f.bottom {
		return "⊥"
	}
	if len(f.entries) == 0 {
		return "⊤"
	}
	parts := make([]string, 0, len(f.entries))
	for _, e := range f.entries {
		parts = append(parts, fmt.Sprintf("%s:%s", e.Path, e.Value.ProjectValue()))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

// StaticMemberFactsDomain is the finite must-map lattice over static member path
// facts. Join is path intersection; Widen is the same path intersection with
// product-domain widening over values proven on both sides.
var StaticMemberFactsDomain = lattice.Lattice[StaticMemberFacts]{
	Bottom: func() StaticMemberFacts {
		return StaticMemberFacts{bottom: true}
	},
	Top: func() StaticMemberFacts {
		return StaticMemberFacts{}
	},
	Equal: func(a, b StaticMemberFacts) bool {
		if a.bottom || b.bottom {
			return a.bottom == b.bottom
		}
		return staticMemberRowIdentity.EqualBy(a.entries, b.entries, staticMemberFactEqual)
	},
	LessOrEq: func(a, b StaticMemberFacts) bool {
		if a.bottom {
			return true
		}
		if b.bottom {
			return false
		}
		return staticMemberFactsContainAll(a.entries, b.entries, product.Domain.LessOrEq)
	},
	Join: func(a, b StaticMemberFacts) StaticMemberFacts {
		if a.bottom {
			return b
		}
		if b.bottom {
			return a
		}
		return intersectStaticMemberFacts(a, b, product.Domain.Join)
	},
	Meet: nil,
	Widen: func(prev, next StaticMemberFacts) StaticMemberFacts {
		if prev.bottom {
			return next
		}
		if next.bottom {
			return prev
		}
		return intersectStaticMemberFacts(prev, next, product.Domain.Widen)
	},
}

func canonicalStaticMemberFacts(entries []StaticMemberFact, merge func(product.AbstractValue, product.AbstractValue) product.AbstractValue) StaticMemberFacts {
	if len(entries) == 0 {
		return StaticMemberFacts{}
	}
	out := append([]StaticMemberFact(nil), entries...)
	sortStaticMemberFacts(out)
	dst := out[:0]
	for _, e := range out {
		if e.Path == "" || valueIsBottom(e.Value) {
			continue
		}
		if len(dst) > 0 && dst[len(dst)-1].Path == e.Path {
			dst[len(dst)-1].Value = merge(dst[len(dst)-1].Value, e.Value)
			if valueIsBottom(dst[len(dst)-1].Value) {
				dst = dst[:len(dst)-1]
			}
			continue
		}
		dst = append(dst, e)
	}
	return StaticMemberFacts{entries: append([]StaticMemberFact(nil), dst...)}
}

func sortStaticMemberFacts(entries []StaticMemberFact) {
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0 && entries[j].Path < entries[j-1].Path; j-- {
			entries[j], entries[j-1] = entries[j-1], entries[j]
		}
	}
}

func findStaticMemberFact(entries []StaticMemberFact, path constraint.PathKey) (int, bool) {
	return staticMemberRowIdentity.Find(entries, StaticMemberFact{Path: path})
}

func staticMemberFactsContainAll(have, want []StaticMemberFact, pred func(product.AbstractValue, product.AbstractValue) bool) bool {
	return staticMemberRowIdentity.ContainsAllBy(have, want, func(have, want StaticMemberFact) bool {
		return have.Path == want.Path && pred(have.Value, want.Value)
	})
}

func intersectStaticMemberFacts(a, b StaticMemberFacts, op func(product.AbstractValue, product.AbstractValue) product.AbstractValue) StaticMemberFacts {
	out := staticMemberRowIdentity.MergeIntersect(a.entries, b.entries, func(left, right StaticMemberFact) (StaticMemberFact, bool) {
		return StaticMemberFact{
			Path:  left.Path,
			Value: op(left.Value, right.Value),
		}, true
	})
	return canonicalStaticMemberFacts(out, product.Domain.Join)
}

func staticMemberFactEqual(a, b StaticMemberFact) bool {
	return a.Path == b.Path && product.Domain.Equal(a.Value, b.Value)
}

func staticMemberFactSamePath(a, b StaticMemberFact) bool {
	return a.Path == b.Path
}

func staticMemberFactLess(a, b StaticMemberFact) bool {
	return a.Path < b.Path
}

var staticMemberRowIdentity = orderedRowIdentity[StaticMemberFact]{
	less: staticMemberFactLess,
	same: staticMemberFactSamePath,
}

func pointStaticMembersLessOrEq(a, b PointState) bool {
	return a.StaticMembers.coversWithPresentValues(
		b.StaticMembers,
		func(addr StableAddress) (product.AbstractValue, bool) { return pointAddressPresentValue(a, addr) },
		product.Domain.LessOrEq,
	)
}

func pointStaticMembersJoin(
	a, b PointState,
	op func(product.AbstractValue, product.AbstractValue) product.AbstractValue,
) StaticMemberFacts {
	if a.StaticMembers.bottom {
		return b.StaticMembers
	}
	if b.StaticMembers.bottom {
		return a.StaticMembers
	}
	joined := intersectStaticMemberFacts(a.StaticMembers, b.StaticMembers, op)
	joined = pointStaticMembersJoinOneSided(joined, a.StaticMembers, b, op)
	joined = pointStaticMembersJoinOneSided(joined, b.StaticMembers, a, op)
	return joined
}

func pointStaticMembersJoinOneSided(
	out StaticMemberFacts,
	facts StaticMemberFacts,
	other PointState,
	op func(product.AbstractValue, product.AbstractValue) product.AbstractValue,
) StaticMemberFacts {
	return out.withFactsMergedFromPresentValues(
		facts,
		func(addr StableAddress) (product.AbstractValue, bool) { return pointAddressPresentValue(other, addr) },
		op,
	)
}
