package read

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/geometry"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/store"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

// Tuple is an authenticated key vector issued by one Reader.Tuple call. Its
// values are opaque semantic tokens; Lookup accepts a tuple from another
// reader only when the tuple belongs to the same committed aggregate and
// runtime fence.
type Tuple struct {
	owner  *reader
	values []binding.ValueToken
	fence  binding.Fence
}

func (tuple Tuple) Available() bool {
	if tuple.owner == nil || !tuple.fence.Available() || !tuple.owner.fence.Available() || !tuple.fence.Same(tuple.owner.fence) || len(tuple.values) != len(tuple.owner.types) || !tuple.owner.root.Available() {
		return false
	}
	for index, value := range tuple.values {
		if !value.ValidFor(tuple.fence) || value.Type() != tuple.owner.types[index] {
			return false
		}
	}
	return true
}

// Equal compares exact ordered opaque values, not physical row coordinates.
func (tuple Tuple) Equal(other Tuple) bool {
	if !tuple.Available() || !other.Available() || !tuple.fence.Same(other.fence) || len(tuple.values) != len(other.values) {
		return false
	}
	for index := range tuple.values {
		if !tuple.values[index].Same(other.values[index]) {
			return false
		}
	}
	return true
}

// Width returns the sealed key-vector arity.
func (tuple Tuple) Width() int {
	if !tuple.Available() {
		return 0
	}
	return len(tuple.values)
}

// TypeAt returns the authenticated nominal type at one key-vector position.
func (tuple Tuple) TypeAt(index int) (model.TypeID, bool) {
	if !tuple.Available() || index < 0 || index >= len(tuple.values) {
		return model.TypeID{}, false
	}
	return tuple.values[index].Type(), true
}

func (handle Reader) Tuple(row Row) (Tuple, bool) {
	if !handle.Available() {
		return Tuple{}, false
	}
	return handle.value.Tuple(row)
}

func (handle Reader) TupleFrom(values []binding.ValueToken) (Tuple, bool) {
	if !handle.Available() {
		return Tuple{}, false
	}
	return handle.value.TupleFrom(values)
}

func (value *reader) Tuple(sourceRow Row) (Tuple, bool) {
	candidate, ok := sourceRow.(*row)
	if !ok || !value.available() || candidate == nil || !candidate.rowFrom(value) || !candidate.Available() {
		return Tuple{}, false
	}
	values, ok := value.keyValues(candidate.key, candidate.mask)
	if !ok || len(values) != len(value.types) {
		return Tuple{}, false
	}
	return value.makeTuple(values)
}

// TupleFrom issues an owner-bound tuple for the exact target Reader. Values
// are validated against the reader's mounted key types and fence; callers
// cannot smuggle a foreign semantic token into Lookup.
func (value *reader) TupleFrom(values []binding.ValueToken) (Tuple, bool) {
	if !value.available() || len(values) != len(value.types) {
		return Tuple{}, false
	}
	return value.makeTuple(values)
}

func (value *reader) makeTuple(values []binding.ValueToken) (Tuple, bool) {
	copyOf := append([]binding.ValueToken(nil), values...)
	for index, token := range copyOf {
		if !token.ValidFor(value.fence) || token.Type() != value.types[index] {
			return Tuple{}, false
		}
	}
	result := Tuple{owner: value, values: copyOf, fence: value.fence}
	if !result.Available() {
		return Tuple{}, false
	}
	return result, true
}

func (value *reader) keyValues(key geometry.Key, within support.Mask) ([]binding.ValueToken, bool) {
	if !value.available() || !within.Valid() || within.Manager() != value.manager {
		return nil, false
	}
	columns := value.layout.KeyColumns()
	values := make([]binding.ValueToken, len(columns))
	for position, columnID := range columns {
		found := false
		failed := false
		completed, valid := value.root.Store().Read(columnID, key, within, value.scratch, func(part store.ReadPart) bool {
			if !part.Value().Available() || part.Type() != value.types[position] || !part.Region().Valid() || part.Region().Manager() != value.manager || support.Empty(part.Region()) {
				failed = true
				return false
			}
			if !found {
				values[position] = part.Value()
				found = true
				return true
			}
			if !values[position].Same(part.Value()) {
				failed = true
				return false
			}
			return true
		})
		if failed || !completed || !valid || !found {
			return nil, false
		}
	}
	return values, true
}
