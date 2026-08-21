package boundary

import (
	"bytes"
	"sort"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/link/internal/radix"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
)

func (v Values) live() bool {
	return v.component != nil && v.component.authority != nil && v.component.authority.valueTable != nil
}

func (v Values) valid(value Value) bool {
	return v.live() && v.component.authority.valueTable != nil && value.component == v.component && value.ordinal < uint32(len(v.component.authority.valueTable.rows))
}

// Count reports the complete canonical Boundary value universe.
func (v Values) Count() int {
	if !v.live() {
		return 0
	}
	return len(v.component.authority.valueTable.rows)
}

// At issues one owner-fenced Boundary Value in canonical mount/family order.
func (v Values) At(index int) (Value, bool) {
	if !v.live() || index < 0 || index >= len(v.component.authority.valueTable.rows) {
		return Value{}, false
	}
	return Value{component: v.component, ordinal: uint32(index)}, true
}

// Origin returns the exact Project Shard and Program Term named by value.
func (v Values) Origin(value Value) (linkproject.Shard, keyspace.Term, bool) {
	if !v.valid(value) {
		return linkproject.Shard{}, 0, false
	}
	row := v.component.authority.valueTable.rows[value.ordinal]
	mounts := v.component.authority.project.Mounts()
	shard, ok := mounts.At(int(row.shard) - 1)
	if !ok || row.term == 0 {
		return linkproject.Shard{}, 0, false
	}
	return shard, row.term, true
}

// ID returns a replay-stable identity derived only from the source mount's
// value relation and semantic Program Term.  Target, Host, Module, Static,
// and enclosing Boundary/Link identities are intentionally absent.
func (v Values) ID(value Value) (identity.ContentID, bool) {
	if !v.valid(value) {
		return identity.ContentID{}, false
	}
	row := v.component.authority.valueTable.rows[value.ordinal]
	if row.shard == 0 || uint64(row.shard) > uint64(len(v.component.authority.valueTable.relations)) {
		return identity.ContentID{}, false
	}
	return valueID(v.component.authority.valueTable.relations[row.shard-1], row.term)
}

// FindID rebinds one portable existing Value identity through this exact
// finalized Boundary authority. The sealed ID index is sorted and retains no
// map or replay builder.
func (v Values) FindID(id identity.ContentID) (Value, bool) {
	if !v.live() || !id.Available() {
		return Value{}, false
	}
	rows := v.component.authority.valueTable.ids
	index := sort.Search(len(rows), func(index int) bool { return bytes.Compare(rows[index].id[:], id[:]) >= 0 })
	if index >= len(rows) || rows[index].id != id || uint64(rows[index].ordinal) >= uint64(len(v.component.authority.valueTable.rows)) {
		return Value{}, false
	}
	return Value{component: v.component, ordinal: rows[index].ordinal}, true
}

// Compare orders two values from this exact Boundary.
func (v Values) Compare(left, right Value) (int, bool) {
	if !v.valid(left) || !v.valid(right) {
		return 0, false
	}
	if left.ordinal < right.ordinal {
		return -1, true
	}
	if left.ordinal > right.ordinal {
		return 1, true
	}
	return 0, true
}

// Of resolves one existing Program occurrence through an exact Project Shard.
func (v Values) Of(shard linkproject.Shard, term keyspace.Term) (Value, bool) {
	if !v.live() || term == 0 {
		return Value{}, false
	}
	mounts := v.component.authority.project.Mounts()
	mountIndex, ok := mounts.Index(shard)
	if !ok || mountIndex < 0 || mountIndex >= mounts.Count() {
		return Value{}, false
	}
	ordinal, ok := v.component.authority.valueTable.index.Lookup(radix.Index(mountIndex+1), uint32(term))
	if !ok || uint64(ordinal) >= uint64(len(v.component.authority.valueTable.rows)) {
		return Value{}, false
	}
	value := Value{component: v.component, ordinal: ordinal}
	row := v.component.authority.valueTable.rows[ordinal]
	return value, row.shard == uint32(mountIndex+1) && row.term == term
}

// ForMountedSpan rebinds one reusable Program span identity through an exact
// Link mount. It is the artifact substitution lane: neither a Program handle
// nor an authored Term is accepted or reconstructed.
func (v Values) ForMountedSpan(moduleKey, spanContext identity.ContentID) (Value, bool) {
	if !v.live() || !moduleKey.Available() || !spanContext.Available() {
		return Value{}, false
	}
	mount := v.component.authority.valueTable.mounts[moduleKey]
	if mount == 0 || uint64(mount) > uint64(len(v.component.authority.valueTable.relations)) {
		return Value{}, false
	}
	ordinal, ok := v.component.authority.valueTable.spans[valueSpanKey{mount: mount, context: spanContext}]
	if !ok || uint64(ordinal) >= uint64(len(v.component.authority.valueTable.rows)) {
		return Value{}, false
	}
	row := v.component.authority.valueTable.rows[ordinal]
	value := Value{component: v.component, ordinal: ordinal}
	return value, row.shard == mount && v.valid(value)
}

// ForMountedSemantic rebinds one exact reusable Program semantic occurrence
// through a concrete ModuleKey. The inverse was sealed while Link owned the
// live Program proof; callers cannot provide a raw Term or fabricate a
// Value-to-occurrence join.
func (v Values) ForMountedSemantic(moduleKey, occurrenceID identity.ContentID) (Value, bool) {
	if !v.live() || !moduleKey.Available() || !occurrenceID.Available() {
		return Value{}, false
	}
	mount := v.component.authority.valueTable.mounts[moduleKey]
	if mount == 0 || uint64(mount) > uint64(len(v.component.authority.valueTable.relations)) {
		return Value{}, false
	}
	ordinal, ok := v.component.authority.valueTable.semantic[valueSemanticKey{mount: mount, id: occurrenceID}]
	if !ok || uint64(ordinal) >= uint64(len(v.component.authority.valueTable.rows)) {
		return Value{}, false
	}
	row := v.component.authority.valueTable.rows[ordinal]
	value := Value{component: v.component, ordinal: ordinal}
	return value, row.shard == mount && v.valid(value)
}

// VisitMountedSemantics visits the complete mounted semantic Value directory
// exactly once. Order is deliberately unspecified: semantic IDs are opaque
// lookup keys, not an authored sequence. The callback receives only the
// existing ModuleKey and Boundary Value association sealed by this owner.
func (v Values) VisitMountedSemantics(visit func(moduleKey, occurrenceID identity.ContentID, value Value) bool) bool {
	if !v.live() || visit == nil {
		return false
	}
	table := v.component.authority.valueTable
	mounts := v.component.authority.project.Mounts()
	if mounts.Count() != len(table.relations) {
		return false
	}
	for key, ordinal := range table.semantic {
		if key.mount == 0 || uint64(key.mount) > uint64(mounts.Count()) || !key.id.Available() || uint64(ordinal) >= uint64(len(table.rows)) {
			return false
		}
		shard, shardOK := mounts.At(int(key.mount - 1))
		module, moduleOK := v.component.authority.project.ModuleKey(shard)
		row := table.rows[ordinal]
		value := Value{component: v.component, ordinal: ordinal}
		if !shardOK || !moduleOK || !module.Available() || table.mounts[module] != key.mount || row.shard != key.mount || !v.valid(value) || !visit(module, key.id, value) {
			return false
		}
	}
	return true
}
