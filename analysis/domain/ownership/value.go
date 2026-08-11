package ownership

import "github.com/wippyai/go-lua/analysis/domain/materialization"

// Bound is one endpoint of the finite obligation interval.  Many is an
// abstract unbounded cardinality, never a solver capacity or execution limit.
type Bound uint8

const (
	Zero Bound = iota
	One
	Many
)

func (bound Bound) Valid() bool { return bound <= Many }

// Value is the normalized finite relation from sealed root alternative to an
// outstanding-duty interval at the coordinate where it is stored.  The
// coordinate already carries the exact declared ownership Role.
type Value struct {
	owner   *schema
	entries []entry
}
type entry struct {
	root uint32
	role materialization.Role
	min  Bound
	max  Bound
}

func (schema Schema) Bottom() (Value, bool) {
	if !schema.Valid() {
		return Value{}, false
	}
	return Value{owner: schema.owner}, true
}
func (schema Schema) Default() (Value, bool) { return schema.Bottom() }

// Of constructs one exact allocation-root age and outstanding-duty interval.
func (schema Schema) Of(root Root, role materialization.Role, min, max Bound) (Value, bool) {
	if !schema.Valid() || !root.valid() || root.owner != schema.owner || (role != materialization.Recent && role != materialization.Summary) || !min.Valid() || !max.Valid() || min > max {
		return Value{}, false
	}
	return Value{owner: schema.owner, entries: []entry{{root: root.index, role: role, min: min, max: max}}}, true
}
func (schema Schema) Top() (Value, bool) {
	if !schema.Valid() {
		return Value{}, false
	}
	entries := make([]entry, 0, len(schema.owner.roots)*2)
	for index := range schema.owner.roots {
		root := uint32(index + 1)
		entries = append(entries, entry{root: root, role: materialization.Recent, min: Zero, max: Many}, entry{root: root, role: materialization.Summary, min: Zero, max: Many})
	}
	return Value{owner: schema.owner, entries: entries}, true
}
func (value Value) Valid() bool {
	if value.owner == nil {
		return false
	}
	previous := entry{}
	for index, current := range value.entries {
		if current.root == 0 || uint64(current.root) > uint64(len(value.owner.roots)) || (current.role != materialization.Recent && current.role != materialization.Summary) || !current.min.Valid() || !current.max.Valid() || current.min > current.max || index != 0 && !less(previous, current) {
			return false
		}
		previous = current
	}
	return true
}
func (value Value) IsBottom() bool { return value.Valid() && len(value.entries) == 0 }
func (value Value) Count() int {
	if !value.Valid() {
		return 0
	}
	return len(value.entries)
}
func (value Value) At(index int) (Root, materialization.Role, Bound, Bound, bool) {
	if !value.Valid() || index < 0 || index >= len(value.entries) {
		return Root{}, materialization.Invalid, Zero, Zero, false
	}
	item := value.entries[index]
	return Root{owner: value.owner, index: item.root}, item.role, item.min, item.max, true
}

// Materialize advances only the selected allocation root. When an advanced
// Recent interval collides with an existing Summary interval, their result is
// the Ownership interval hull.
func (schema Schema) Materialize(value Value, root Root) (Value, bool) {
	if !schema.owns(value) || !root.valid() || root.owner != schema.owner {
		return Value{}, false
	}
	entries := append([]entry(nil), value.entries...)
	changed := false
	for index := range entries {
		if entries[index].root != root.index {
			continue
		}
		next, advanced := materialization.RecentToSummary(entries[index].role)
		if advanced {
			entries[index].role = next
			changed = true
		}
	}
	if !changed {
		return value, true
	}
	result := entries[:0]
	for _, current := range entries {
		if len(result) == 0 || !sameRoot(result[len(result)-1], current) {
			result = append(result, current)
			continue
		}
		last := &result[len(result)-1]
		if current.min < last.min {
			last.min = current.min
		}
		if current.max > last.max {
			last.max = current.max
		}
	}
	return Value{owner: schema.owner, entries: result}, true
}
func (schema Schema) Admit(coordinate Coordinate, value Value) bool {
	return schema.Valid() && coordinate.valid() && coordinate.owner == schema.owner && value.Valid() && value.owner == schema.owner
}
func (schema Schema) owns(value Value) bool {
	return schema.Valid() && value.Valid() && value.owner == schema.owner
}
func less(left, right entry) bool {
	return left.root < right.root || left.root == right.root && left.role < right.role
}
