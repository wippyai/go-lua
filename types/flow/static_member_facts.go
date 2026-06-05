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

// Entries returns a defensive copy of finite entries. Bottom has no consumable
// finite entries.
func (f StaticMemberFacts) Entries() []StaticMemberFact {
	if f.bottom || len(f.entries) == 0 {
		return nil
	}
	return append([]StaticMemberFact(nil), f.entries...)
}

// Value returns the fact for path if it is proven in a reachable state.
func (f StaticMemberFacts) Value(path constraint.PathKey) (product.AbstractValue, bool) {
	if f.bottom || path == "" || len(f.entries) == 0 {
		return product.Domain.Bottom(), false
	}
	idx, ok := findStaticMemberFact(f.entries, path)
	if !ok {
		return product.Domain.Bottom(), false
	}
	return f.entries[idx].Value, true
}

// With returns f with path strongly updated to value. Updating to Bottom removes
// the key, preserving absent-is-no-fact canonical form.
func (f StaticMemberFacts) With(path constraint.PathKey, value product.AbstractValue) StaticMemberFacts {
	if f.bottom {
		f = StaticMemberFacts{}
	}
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

// KillSubtree removes facts at root and under root's structural path suffix.
func (f StaticMemberFacts) KillSubtree(root constraint.PathKey) StaticMemberFacts {
	if f.bottom || root == "" || len(f.entries) == 0 {
		return f
	}
	rootAddr, ok := StableAddressFromKey(root)
	if !ok {
		return f
	}
	out := make([]StaticMemberFact, 0, len(f.entries))
	for _, e := range f.entries {
		pathAddr, ok := StableAddressFromKey(e.Path)
		if ok && pathAddr.HasPrefix(rootAddr) {
			continue
		}
		out = append(out, e)
	}
	return canonicalStaticMemberFacts(out, product.Domain.Join)
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
		if len(a.entries) != len(b.entries) {
			return false
		}
		for i := range a.entries {
			if a.entries[i].Path != b.entries[i].Path ||
				!product.Domain.Equal(a.entries[i].Value, b.entries[i].Value) {
				return false
			}
		}
		return true
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
	lo, hi := 0, len(entries)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		if entries[mid].Path < path {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo, lo < len(entries) && entries[lo].Path == path
}

func staticMemberFactsContainAll(have, want []StaticMemberFact, pred func(product.AbstractValue, product.AbstractValue) bool) bool {
	for _, w := range want {
		idx, ok := findStaticMemberFact(have, w.Path)
		if !ok || !pred(have[idx].Value, w.Value) {
			return false
		}
	}
	return true
}

func intersectStaticMemberFacts(a, b StaticMemberFacts, op func(product.AbstractValue, product.AbstractValue) product.AbstractValue) StaticMemberFacts {
	var out []StaticMemberFact
	i, j := 0, 0
	for i < len(a.entries) && j < len(b.entries) {
		switch {
		case a.entries[i].Path < b.entries[j].Path:
			i++
		case b.entries[j].Path < a.entries[i].Path:
			j++
		default:
			out = append(out, StaticMemberFact{
				Path:  a.entries[i].Path,
				Value: op(a.entries[i].Value, b.entries[j].Value),
			})
			i++
			j++
		}
	}
	return canonicalStaticMemberFacts(out, product.Domain.Join)
}
