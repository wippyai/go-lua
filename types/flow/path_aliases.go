package flow

import (
	"fmt"
	"sort"
	"strings"

	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/lattice"
)

// PathAliasFact is point-local identity provenance for assignment aliases.
//
// It records that Value currently denotes the same runtime key/value identity as
// Source. Unlike ValueOriginFacts, this carrier is not semantic demand evidence
// and not reference-mutation provenance. It exists so path-sensitive consumers
// such as index-write readback can follow `last = id` even when the payload type
// is strict dynamic.
type PathAliasFact struct {
	Value  constraint.PathKey
	Source constraint.PathKey
}

// SourceAddress returns the normalized source address carried by this alias.
func (f PathAliasFact) SourceAddress() (StableAddress, bool) {
	return StableAddressFromKey(f.Source)
}

// SourcePath returns the source path carried by this alias when it is
// symbol-rooted.
func (f PathAliasFact) SourcePath() (constraint.Path, bool) {
	addr, ok := f.SourceAddress()
	if !ok {
		return constraint.Path{}, false
	}
	return addr.Path()
}

// PathAliasUse is a path-alias fact covering a consumed path. Remainder is the
// suffix under Alias.Value, so an alias `b <- a` also covers `b.x` as `a.x`.
type PathAliasUse struct {
	Alias     PathAliasFact
	Remainder []constraint.Segment
}

// PathAliasFacts is a finite must-set lattice over assignment identity facts.
// Bottom is unreachable; Top is the empty fact set.
type PathAliasFacts struct {
	bottom  bool
	entries []PathAliasFact
}

func (f PathAliasFacts) IsBottom() bool { return f.bottom }

func (f PathAliasFacts) Entries() []PathAliasFact {
	if f.bottom || len(f.entries) == 0 {
		return nil
	}
	return append([]PathAliasFact(nil), f.entries...)
}

func (f PathAliasFacts) With(fact PathAliasFact) PathAliasFacts {
	if fact.Value == "" || fact.Source == "" || fact.Value == fact.Source {
		return f
	}
	if f.bottom {
		f = PathAliasFacts{}
	}
	next := f.Entries()
	if _, ok := findPathAliasFact(next, fact); !ok {
		next = append(next, fact)
	}
	return canonicalPathAliasFacts(next)
}

func (f PathAliasFacts) WithAddresses(value, source StableAddress) PathAliasFacts {
	valueKey := value.Key()
	sourceKey := source.Key()
	if valueKey == "" || sourceKey == "" {
		return f
	}
	return f.With(PathAliasFact{Value: valueKey, Source: sourceKey})
}

func (f PathAliasFacts) AliasesOfAddress(value StableAddress) []PathAliasFact {
	valueKey := value.Key()
	if f.bottom || valueKey == "" || len(f.entries) == 0 {
		return nil
	}
	var out []PathAliasFact
	for _, entry := range f.entries {
		if entry.Value == valueKey {
			out = append(out, entry)
		}
	}
	return out
}

func (f PathAliasFacts) AliasesCoveringAddress(value StableAddress) []PathAliasUse {
	if f.bottom || value.Key() == "" || len(f.entries) == 0 {
		return nil
	}
	var out []PathAliasUse
	for _, entry := range f.entries {
		remainder, ok := value.RemainderAfterAddressKey(entry.Value)
		if !ok {
			continue
		}
		out = append(out, PathAliasUse{Alias: entry, Remainder: remainder})
	}
	sort.Slice(out, func(i, j int) bool {
		if len(out[i].Remainder) != len(out[j].Remainder) {
			return len(out[i].Remainder) < len(out[j].Remainder)
		}
		return pathAliasLess(out[i].Alias, out[j].Alias)
	})
	return out
}

func (f PathAliasFacts) KillAffectedByWriteAddress(write StableAddress) PathAliasFacts {
	if f.bottom || write.Key() == "" || len(f.entries) == 0 {
		return f
	}
	entries := make([]PathAliasFact, 0, len(f.entries))
	for _, entry := range f.entries {
		if AddressKeyOverlaps(entry.Value, write) || AddressKeyOverlaps(entry.Source, write) {
			continue
		}
		entries = append(entries, entry)
	}
	return canonicalPathAliasFacts(entries)
}

func (f PathAliasFacts) coversWithAbsentValues(want PathAliasFacts, absent func(constraint.PathKey) bool) bool {
	if f.bottom {
		return true
	}
	if want.bottom {
		return false
	}
	for _, entry := range want.entries {
		if _, ok := findPathAliasFact(f.entries, entry); ok {
			continue
		}
		if pathAliasAbsent(absent, entry.Value) {
			continue
		}
		return false
	}
	return true
}

func (f PathAliasFacts) withAliasesProvedByAbsentValues(facts PathAliasFacts, absent func(constraint.PathKey) bool) PathAliasFacts {
	if facts.bottom {
		return f
	}
	out := f
	for _, entry := range facts.entries {
		if _, ok := findPathAliasFact(out.entries, entry); ok {
			continue
		}
		if pathAliasAbsent(absent, entry.Value) {
			out = out.With(entry)
		}
	}
	return out
}

func pathAliasAbsent(absent func(constraint.PathKey) bool, key constraint.PathKey) bool {
	return absent != nil && absent(key)
}

func (f PathAliasFacts) Format() string {
	if f.bottom {
		return "⊥"
	}
	if len(f.entries) == 0 {
		return "{}"
	}
	parts := make([]string, 0, len(f.entries))
	for _, entry := range f.entries {
		parts = append(parts, fmt.Sprintf("%s<-%s", entry.Value, entry.Source))
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

var PathAliasFactsDomain = lattice.Lattice[PathAliasFacts]{
	Bottom: func() PathAliasFacts {
		return PathAliasFacts{bottom: true}
	},
	Top: func() PathAliasFacts {
		return PathAliasFacts{}
	},
	Equal: func(a, b PathAliasFacts) bool {
		if a.bottom || b.bottom {
			return a.bottom == b.bottom
		}
		return orderedRowsEqual(a.entries, b.entries, func(a, b PathAliasFact) bool { return a == b })
	},
	LessOrEq: func(a, b PathAliasFacts) bool {
		if a.bottom {
			return true
		}
		if b.bottom {
			return false
		}
		return pathAliasFactsContainAll(a.entries, b.entries)
	},
	Join: func(a, b PathAliasFacts) PathAliasFacts {
		if a.bottom {
			return b
		}
		if b.bottom {
			return a
		}
		return intersectPathAliasFacts(a, b)
	},
	Meet: nil,
	Widen: func(prev, next PathAliasFacts) PathAliasFacts {
		if prev.bottom {
			return next
		}
		if next.bottom {
			return prev
		}
		return intersectPathAliasFacts(prev, next)
	},
}

func canonicalPathAliasFacts(entries []PathAliasFact) PathAliasFacts {
	if len(entries) == 0 {
		return PathAliasFacts{}
	}
	out := append([]PathAliasFact(nil), entries...)
	sort.Slice(out, func(i, j int) bool { return pathAliasLess(out[i], out[j]) })
	dst := out[:0]
	for _, entry := range out {
		if entry.Value == "" || entry.Source == "" || entry.Value == entry.Source {
			continue
		}
		if len(dst) > 0 && dst[len(dst)-1] == entry {
			continue
		}
		dst = append(dst, entry)
	}
	return PathAliasFacts{entries: append([]PathAliasFact(nil), dst...)}
}

func pathAliasLess(a, b PathAliasFact) bool {
	if a.Value != b.Value {
		return a.Value < b.Value
	}
	return a.Source < b.Source
}

func findPathAliasFact(entries []PathAliasFact, fact PathAliasFact) (int, bool) {
	return orderedRowsFind(entries, fact, pathAliasLess, func(a, b PathAliasFact) bool { return a == b })
}

func pathAliasFactsContainAll(have, want []PathAliasFact) bool {
	return orderedRowsContainAll(have, want, pathAliasLess, func(a, b PathAliasFact) bool { return a == b })
}

func intersectPathAliasFacts(a, b PathAliasFacts) PathAliasFacts {
	out := orderedRowsIntersect(a.entries, b.entries, pathAliasLess, func(a, b PathAliasFact) bool { return a == b })
	return canonicalPathAliasFacts(out)
}
