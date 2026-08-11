package escape

import "github.com/wippyai/go-lua/analysis/domain/materialization"

// Value is a normalized finite may-relation over the schema's sealed root
// alternatives. A relation entry says that one materialized alternative
// may cross the coordinate at which this Value is stored.  Proof provenance is
// evidence, not a second fact vocabulary.
type Value struct {
	owner   *schema
	entries []entry
}

type entry struct {
	root uint32
	role materialization.Role
}

func (schema Schema) Bottom() (Value, bool) {
	if !schema.Valid() {
		return Value{}, false
	}
	return Value{owner: schema.owner}, true
}

// Default is the constant sparse empty relation.
func (schema Schema) Default() (Value, bool) { return schema.Bottom() }

// Of constructs one exact root/age alternative. Escape carries only
// allocation-derived Recent and Summary alternatives; Exact belongs to the
// owners of exact structural sources.
func (schema Schema) Of(root Root, role materialization.Role) (Value, bool) {
	if !schema.Valid() || !root.valid() || root.owner != schema.owner || role != materialization.Recent && role != materialization.Summary {
		return Value{}, false
	}
	return Value{owner: schema.owner, entries: []entry{{root: root.index, role: role}}}, true
}

func (schema Schema) Top() (Value, bool) {
	if !schema.Valid() {
		return Value{}, false
	}
	entries := make([]entry, 0, len(schema.owner.roots)*2)
	for index := range schema.owner.roots {
		root := uint32(index + 1)
		entries = append(entries, entry{root: root, role: materialization.Recent}, entry{root: root, role: materialization.Summary})
	}
	return Value{owner: schema.owner, entries: entries}, true
}

func (value Value) Valid() bool {
	if value.owner == nil {
		return false
	}
	previous := entry{}
	for index, current := range value.entries {
		if current.root == 0 || uint64(current.root) > uint64(len(value.owner.roots)) ||
			(current.role != materialization.Recent && current.role != materialization.Summary) ||
			index != 0 && !less(previous, current) {
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

// At exposes a relation member without exposing the Value's internal root
// index together with its exact materialization role.
func (value Value) At(index int) (Root, materialization.Role, bool) {
	if !value.Valid() || index < 0 || index >= len(value.entries) {
		return Root{}, materialization.Invalid, false
	}
	entry := value.entries[index]
	return Root{owner: value.owner, index: entry.root}, entry.role, true
}

// Materialize advances only the selected root. Colliding Summary entries are
// deduplicated in the same normalized relation; all other roots retain their
// exact role.
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
		if len(result) == 0 || result[len(result)-1] != current {
			result = append(result, current)
		}
	}
	return Value{owner: schema.owner, entries: result}, true
}

// Admit verifies one factor-cell value against its exact finite coordinate.
// Neither coordinates nor roots can be assembled by this operation.
func (schema Schema) Admit(coordinate Coordinate, value Value) bool {
	return schema.Valid() && coordinate.valid() && coordinate.owner == schema.owner &&
		value.Valid() && value.owner == schema.owner
}

func (schema Schema) owns(value Value) bool {
	return schema.Valid() && value.Valid() && value.owner == schema.owner
}

func less(left, right entry) bool {
	return left.root < right.root || left.root == right.root && left.role < right.role
}
