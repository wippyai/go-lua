// Package summary defines fixed-point function summaries for analysis checks.
package summary

import (
	"github.com/wippyai/go-lua/analysis/check/fixpoint/ref"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

// Digest is an explicit caller-provided digest for future entry dimensions.
type Digest uint64

// EntryKey identifies the abstract call-entry dimensions for a summary key.
type EntryKey struct {
	Values     Digest
	Facts      Digest
	References Digest
}

// SummaryKey identifies one exact summary entry.
type SummaryKey struct {
	Ref   ref.FuncRef
	Entry EntryKey
}

// DefaultSummaryKey returns the default summary key for r.
func DefaultSummaryKey(r ref.FuncRef) SummaryKey {
	return SummaryKey{Ref: r}
}

// Less reports whether k sorts before other.
func (k SummaryKey) Less(other SummaryKey) bool {
	if k.Ref != other.Ref {
		return k.Ref.Less(other.Ref)
	}
	if k.Entry.Values != other.Entry.Values {
		return k.Entry.Values < other.Entry.Values
	}
	if k.Entry.Facts != other.Entry.Facts {
		return k.Entry.Facts < other.Entry.Facts
	}
	return k.Entry.References < other.Entry.References
}

// Summary is the fixed-point analysis summary payload for one function entry.
type Summary struct {
	Returns []product.Value
}

// Normalize returns s with trailing bottom return slots removed.
func Normalize(reg *axis.Registry, s Summary) Summary {
	out := s.Clone()
	bottom := product.Bottom(reg)
	for len(out.Returns) > 0 && product.Equal(reg, out.Returns[len(out.Returns)-1], bottom) {
		out.Returns = out.Returns[:len(out.Returns)-1]
	}
	if len(out.Returns) == 0 {
		return Summary{}
	}
	return out
}

// Equal reports whether a and b have equal return tuples. Missing slots are
// bottom.
func Equal(reg *axis.Registry, a, b Summary) bool {
	n := max(len(a.Returns), len(b.Returns))
	for i := range n {
		if !product.Equal(reg, returnAt(reg, a, i), returnAt(reg, b, i)) {
			return false
		}
	}
	return true
}

// LessOrEq reports whether a is less than or equal to b componentwise. Missing
// return slots are bottom.
func LessOrEq(reg *axis.Registry, a, b Summary) bool {
	n := max(len(a.Returns), len(b.Returns))
	for i := range n {
		if !product.LessOrEq(reg, returnAt(reg, a, i), returnAt(reg, b, i)) {
			return false
		}
	}
	return true
}

// Join returns the componentwise join of a and b. Missing return slots are
// bottom.
func Join(reg *axis.Registry, a, b Summary) Summary {
	n := max(len(a.Returns), len(b.Returns))
	if n == 0 {
		return Summary{}
	}
	out := Summary{Returns: make([]product.Value, n)}
	for i := range n {
		out.Returns[i] = product.Join(reg, returnAt(reg, a, i), returnAt(reg, b, i))
	}
	return Normalize(reg, out)
}

// Widen returns the componentwise widening from prev to next. Missing return
// slots are bottom.
func Widen(reg *axis.Registry, prev, next Summary) Summary {
	n := max(len(prev.Returns), len(next.Returns))
	if n == 0 {
		return Summary{}
	}
	out := Summary{Returns: make([]product.Value, n)}
	for i := range n {
		out.Returns[i] = product.Widen(reg, returnAt(reg, prev, i), returnAt(reg, next, i))
	}
	return Normalize(reg, out)
}

// Clone returns an independent copy of s.
func (s Summary) Clone() Summary {
	if len(s.Returns) == 0 {
		return Summary{}
	}
	out := Summary{Returns: make([]product.Value, len(s.Returns))}
	copy(out.Returns, s.Returns)
	return out
}

func returnAt(reg *axis.Registry, s Summary, i int) product.Value {
	if i < len(s.Returns) {
		return s.Returns[i]
	}
	return product.Bottom(reg)
}

// Reader reads exact summary keys.
type Reader interface {
	Read(SummaryKey) (Summary, bool)
}

// EntrySummary binds a key to a summary for snapshot construction.
type EntrySummary struct {
	Key     SummaryKey
	Summary Summary
}

// Snapshot is an immutable exact-key summary reader.
type Snapshot struct {
	reg     *axis.Registry
	entries map[SummaryKey]Summary
}

// NewSnapshot returns a snapshot containing entries.
func NewSnapshot(reg *axis.Registry, entries ...EntrySummary) Snapshot {
	if len(entries) == 0 {
		return Snapshot{reg: reg}
	}
	out := Snapshot{
		reg:     reg,
		entries: make(map[SummaryKey]Summary, len(entries)),
	}
	for _, entry := range entries {
		out.entries[entry.Key] = Normalize(reg, entry.Summary)
	}
	return out
}

// Read returns the summary for k. It never falls back to other entries for the
// same function reference.
func (s Snapshot) Read(k SummaryKey) (Summary, bool) {
	if len(s.entries) == 0 {
		return Summary{}, false
	}
	got, ok := s.entries[k]
	if !ok {
		return Summary{}, false
	}
	return got.Clone(), true
}
