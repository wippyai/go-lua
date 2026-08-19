// Package exactkey owns the Target-contract-wide canonical Lua key directory.
// Boot, operation bindings, and subedge key coordinates all consume this one
// immutable table; no consumer stores a second literal pool or lookup map.
package exactkey

import (
	"errors"
	"sort"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/scalar"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	sealedrows "github.com/wippyai/go-lua/internal/rows"
)

// Compile normalizes, canonicalizes, deduplicates, and seals one Target key
// directory. Input may be in any order and may repeat canonical atoms.
func Compile(input []keyspace.LiteralValue) (Table, error) {
	values := make([]keyspace.LiteralValue, 0, len(input))
	seen := make(map[keyspace.LiteralValue]struct{}, len(input))
	for _, value := range input {
		normalized, ok := scalar.Normalize(value)
		if !ok || normalized != value {
			return Table{}, errors.New("target/exactkey: unnormalized exact key")
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		values = append(values, normalized)
	}
	sort.Slice(values, func(left, right int) bool {
		order, ok := scalar.Compare(values[left], values[right])
		return ok && order < 0
	})
	if _, err := vocabulary.CheckedStoredLength("exact key table", len(values)); err != nil {
		return Table{}, err
	}
	return Table{values: sealedrows.NewRows(values)}, nil
}

// Table is the immutable exact-key owner. Handles are dense and stable for
// the lifetime of the table.
type Table struct {
	values sealedrows.Rows[keyspace.LiteralValue]
}

func (t *Table) Count() int {
	if t == nil {
		return 0
	}
	return t.values.Count()
}

func (t *Table) At(index int) (vocabulary.ExactKey, bool) {
	if t == nil || index < 0 || index >= t.values.Count() {
		return 0, false
	}
	_, ok := t.values.At(index)
	return vocabulary.ExactKey(index + 1), ok
}

func (t *Table) Value(key vocabulary.ExactKey) (keyspace.LiteralValue, bool) {
	if t == nil || key == 0 || uint64(key) > uint64(t.values.Count()) {
		return keyspace.LiteralValue{}, false
	}
	return t.values.At(int(key - 1))
}

// Handle returns the exact dense coordinate of one canonical key atom.
// Lookup is allocation-free and uses the canonical sorted value directory.
func (t *Table) Handle(value keyspace.LiteralValue) (vocabulary.ExactKey, bool) {
	if t == nil {
		return 0, false
	}
	index := sort.Search(t.values.Count(), func(index int) bool {
		candidate, ok := t.values.At(index)
		if !ok {
			return false
		}
		order, ok := scalar.Compare(candidate, value)
		return ok && order >= 0
	})
	candidate, ok := t.values.At(index)
	if !ok || candidate != value {
		return 0, false
	}
	return vocabulary.ExactKey(index + 1), true
}
