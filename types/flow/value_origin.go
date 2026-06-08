package flow

import (
	"fmt"
	"sort"
	"strings"

	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/lattice"
)

type ValueOriginKind uint8

const (
	ValueOriginIndexedIterator ValueOriginKind = iota + 1
	ValueOriginKeyedIterator
	ValueOriginAssignmentAlias
)

// ValueOriginFact is point-local provenance for a value derived from another
// source value. Transfer uses it for backward demand: if a derived local is later
// consumed in a typed context, the corresponding source parameter receives the
// transformed container obligation.
type ValueOriginFact struct {
	Value    constraint.PathKey
	Source   constraint.PathKey
	Kind     ValueOriginKind
	VarIndex int
}

// ValueAddress returns the normalized value address carried by this origin.
func (f ValueOriginFact) ValueAddress() (StableAddress, bool) {
	return StableAddressFromCanonicalKey(f.Value)
}

// SourceAddress returns the normalized source address carried by this origin.
func (f ValueOriginFact) SourceAddress() (StableAddress, bool) {
	return StableAddressFromCanonicalKey(f.Source)
}

// SourcePath returns the source path carried by this origin when it is
// symbol-rooted.
func (f ValueOriginFact) SourcePath() (constraint.Path, bool) {
	addr, ok := f.SourceAddress()
	if !ok {
		return constraint.Path{}, false
	}
	return addr.Path()
}

// ValueOriginUse is a provenance fact that covers a consumed path. Remainder is
// the suffix under Origin.Value that was read by the consumer, so a demand for
// local.f on an origin local <- source becomes iterator evidence for {f: T}.
type ValueOriginUse struct {
	Origin    ValueOriginFact
	Remainder []constraint.Segment
}

type valueOriginRouteRemainder uint8

const (
	valueOriginRouteAnyRemainder valueOriginRouteRemainder = iota
	valueOriginRouteRequireRemainder
	valueOriginRouteForbidRemainder
)

type valueOriginRouteQuery struct {
	value     StableAddress
	kind      ValueOriginKind
	varIndex  int
	hasVar    bool
	remainder valueOriginRouteRemainder
}

// SourceAddress returns the normalized source address represented by this use.
func (u ValueOriginUse) SourceAddress() (StableAddress, bool) {
	return u.Origin.SourceAddress()
}

// SourcePath returns the symbol-rooted source path represented by this use.
func (u ValueOriginUse) SourcePath() (constraint.Path, bool) {
	addr, ok := u.SourceAddress()
	if !ok {
		return constraint.Path{}, false
	}
	return addr.Path()
}

// SourceRoute returns the normalized source route represented by this use.
func (u ValueOriginUse) SourceRoute() (sourceRoute, bool) {
	source, ok := u.SourceAddress()
	if !ok {
		return sourceRoute{}, false
	}
	return sourceRouteOfKind(provenanceRouteKindForValueOrigin(u.Origin.Kind), source, u.Remainder, u.Origin.VarIndex)
}

// ValueOriginFacts is a finite must-set lattice over value-origin provenance.
// Bottom is unreachable, Top is the empty fact set, and Join/Widen keep only
// origins proven by every predecessor.
type ValueOriginFacts struct {
	bottom  bool
	entries []ValueOriginFact
}

func (f ValueOriginFacts) IsBottom() bool { return f.bottom }

func (f ValueOriginFacts) Entries() []ValueOriginFact {
	if f.bottom || len(f.entries) == 0 {
		return nil
	}
	return append([]ValueOriginFact(nil), f.entries...)
}

func (f ValueOriginFacts) With(fact ValueOriginFact) ValueOriginFacts {
	if fact.Value == "" || fact.Source == "" || fact.Kind == 0 {
		return f
	}
	if f.bottom {
		f = ValueOriginFacts{}
	}
	next := f.Entries()
	if _, ok := findValueOriginFact(next, fact); !ok {
		next = append(next, fact)
	}
	return canonicalValueOriginFacts(next)
}

func (f ValueOriginFacts) WithAddresses(value, source StableAddress, kind ValueOriginKind, varIndex int) ValueOriginFacts {
	valueKey := value.Key()
	sourceKey := source.Key()
	if valueKey == "" || sourceKey == "" {
		return f
	}
	return f.With(ValueOriginFact{Value: valueKey, Source: sourceKey, Kind: kind, VarIndex: varIndex})
}

func (f ValueOriginFacts) OriginsOfAddress(value StableAddress) []ValueOriginFact {
	valueKey := value.Key()
	if f.bottom || valueKey == "" || len(f.entries) == 0 {
		return nil
	}
	var out []ValueOriginFact
	for _, entry := range f.entries {
		if entry.Value == valueKey {
			out = append(out, entry)
		}
	}
	return out
}

func (f ValueOriginFacts) OriginsCoveringAddress(value StableAddress) []ValueOriginUse {
	if f.bottom || value.Key() == "" || len(f.entries) == 0 {
		return nil
	}
	var out []ValueOriginUse
	for _, entry := range f.entries {
		entryValue, ok := entry.ValueAddress()
		if !ok {
			continue
		}
		remainder, ok := value.RemainderAfterPrefix(entryValue)
		if !ok {
			continue
		}
		out = append(out, ValueOriginUse{Origin: entry, Remainder: remainder})
	}
	sort.Slice(out, func(i, j int) bool {
		if len(out[i].Remainder) != len(out[j].Remainder) {
			return len(out[i].Remainder) < len(out[j].Remainder)
		}
		return valueOriginLess(out[i].Origin, out[j].Origin)
	})
	return out
}

func (f ValueOriginFacts) sourceRoutes(q valueOriginRouteQuery) []sourceRoute {
	uses := f.OriginsCoveringAddress(q.value)
	if len(uses) == 0 {
		return nil
	}
	var out []sourceRoute
	for _, use := range uses {
		if !valueOriginRouteAccepts(q, use) {
			continue
		}
		route, ok := use.SourceRoute()
		if !ok {
			continue
		}
		out = append(out, route)
	}
	return out
}

func valueOriginRouteAccepts(q valueOriginRouteQuery, use ValueOriginUse) bool {
	if q.kind != 0 && use.Origin.Kind != q.kind {
		return false
	}
	if q.hasVar && use.Origin.VarIndex != q.varIndex {
		return false
	}
	switch q.remainder {
	case valueOriginRouteRequireRemainder:
		return len(use.Remainder) > 0
	case valueOriginRouteForbidRemainder:
		return len(use.Remainder) == 0
	default:
		return true
	}
}

func (f ValueOriginFacts) KillAffectedByWriteAddress(write StableAddress) ValueOriginFacts {
	if f.bottom || write.Key() == "" || len(f.entries) == 0 {
		return f
	}
	entries := make([]ValueOriginFact, 0, len(f.entries))
	for _, entry := range f.entries {
		value, valueOK := entry.ValueAddress()
		source, sourceOK := entry.SourceAddress()
		if (valueOK && value.Overlaps(write)) || (sourceOK && source.Overlaps(write)) {
			continue
		}
		entries = append(entries, entry)
	}
	return canonicalValueOriginFacts(entries)
}

func (f ValueOriginFacts) Format() string {
	if f.bottom {
		return "⊥"
	}
	if len(f.entries) == 0 {
		return "{}"
	}
	parts := make([]string, 0, len(f.entries))
	for _, entry := range f.entries {
		parts = append(parts, fmt.Sprintf("%s<-%s/%d/%d", entry.Value, entry.Source, entry.Kind, entry.VarIndex))
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

var ValueOriginFactsDomain = lattice.Lattice[ValueOriginFacts]{
	Bottom: func() ValueOriginFacts {
		return ValueOriginFacts{bottom: true}
	},
	Top: func() ValueOriginFacts {
		return ValueOriginFacts{}
	},
	Equal: func(a, b ValueOriginFacts) bool {
		if a.bottom || b.bottom {
			return a.bottom == b.bottom
		}
		return valueOriginRowIdentity.Equal(a.entries, b.entries)
	},
	LessOrEq: func(a, b ValueOriginFacts) bool {
		if a.bottom {
			return true
		}
		if b.bottom {
			return a.bottom
		}
		return valueOriginFactsContainAll(a.entries, b.entries)
	},
	Join: func(a, b ValueOriginFacts) ValueOriginFacts {
		if a.bottom {
			return b
		}
		if b.bottom {
			return a
		}
		return intersectValueOriginFacts(a, b)
	},
	Meet: nil,
	Widen: func(prev, next ValueOriginFacts) ValueOriginFacts {
		if prev.bottom {
			return next
		}
		if next.bottom {
			return prev
		}
		return intersectValueOriginFacts(prev, next)
	},
}

func canonicalValueOriginFacts(entries []ValueOriginFact) ValueOriginFacts {
	if len(entries) == 0 {
		return ValueOriginFacts{}
	}
	out := append([]ValueOriginFact(nil), entries...)
	sort.Slice(out, func(i, j int) bool { return valueOriginLess(out[i], out[j]) })
	dst := out[:0]
	for _, entry := range out {
		if entry.Value == "" || entry.Source == "" || entry.Kind == 0 {
			continue
		}
		if len(dst) > 0 && dst[len(dst)-1] == entry {
			continue
		}
		dst = append(dst, entry)
	}
	return ValueOriginFacts{entries: append([]ValueOriginFact(nil), dst...)}
}

func valueOriginLess(a, b ValueOriginFact) bool {
	if a.Value != b.Value {
		return a.Value < b.Value
	}
	if a.Source != b.Source {
		return a.Source < b.Source
	}
	if a.Kind != b.Kind {
		return a.Kind < b.Kind
	}
	return a.VarIndex < b.VarIndex
}

var valueOriginRowIdentity = orderedRowIdentity[ValueOriginFact]{
	less: valueOriginLess,
	same: func(a, b ValueOriginFact) bool { return a == b },
}

func findValueOriginFact(entries []ValueOriginFact, fact ValueOriginFact) (int, bool) {
	return valueOriginRowIdentity.Find(entries, fact)
}

func valueOriginFactsContainAll(have, want []ValueOriginFact) bool {
	return valueOriginRowIdentity.ContainsAll(have, want)
}

func intersectValueOriginFacts(a, b ValueOriginFacts) ValueOriginFacts {
	out := valueOriginRowIdentity.Intersect(a.entries, b.entries)
	return canonicalValueOriginFacts(out)
}
