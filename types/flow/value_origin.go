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

// SourceAddress returns the normalized source address carried by this origin.
func (f ValueOriginFact) SourceAddress() (StableAddress, bool) {
	return StableAddressFromKey(f.Source)
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
		entryAddr, ok := StableAddressFromKey(entry.Value)
		if !ok {
			continue
		}
		remainder, ok := value.RemainderAfterPrefix(entryAddr)
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

func (f ValueOriginFacts) KillAffectedByWriteAddress(write StableAddress) ValueOriginFacts {
	if f.bottom || write.Key() == "" || len(f.entries) == 0 {
		return f
	}
	entries := make([]ValueOriginFact, 0, len(f.entries))
	for _, entry := range f.entries {
		valueAddr, valueOK := StableAddressFromKey(entry.Value)
		sourceAddr, sourceOK := StableAddressFromKey(entry.Source)
		if (valueOK && valueAddr.Overlaps(write)) || (sourceOK && sourceAddr.Overlaps(write)) {
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
		if len(a.entries) != len(b.entries) {
			return false
		}
		for i := range a.entries {
			if a.entries[i] != b.entries[i] {
				return false
			}
		}
		return true
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

func findValueOriginFact(entries []ValueOriginFact, fact ValueOriginFact) (int, bool) {
	i := sort.Search(len(entries), func(i int) bool {
		return !valueOriginLess(entries[i], fact)
	})
	if i < len(entries) && entries[i] == fact {
		return i, true
	}
	return -1, false
}

func valueOriginFactsContainAll(have, want []ValueOriginFact) bool {
	for _, w := range want {
		if _, ok := findValueOriginFact(have, w); !ok {
			return false
		}
	}
	return true
}

func intersectValueOriginFacts(a, b ValueOriginFacts) ValueOriginFacts {
	var out []ValueOriginFact
	for _, entry := range a.entries {
		if _, ok := findValueOriginFact(b.entries, entry); ok {
			out = append(out, entry)
		}
	}
	return canonicalValueOriginFacts(out)
}
