package accessgeometry

import "github.com/wippyai/go-lua/analysis/program/keyspace"

// TableFields is the typed normalized-key projection over authored
// TableField terms.
type TableFields struct{ result *Result }

func (view TableFields) Count() int {
	if view.result == nil || !view.result.available() || len(view.result.tableFields.keys) == 0 {
		return 0
	}
	return len(view.result.tableFields.keys) - 1
}

func (view TableFields) At(index int) (keyspace.Term, bool) {
	if view.result == nil || !view.result.available() || index < 0 || uint64(index+1) >= uint64(len(view.result.tableFields.keys)) {
		return 0, false
	}
	return keyspace.MakeTerm(keyspace.FamilyTableField, uint32(index+1)), true
}

// Get returns the Source-owned normalized key for one TableField. A valid
// dynamic FieldKey or non-storable nil/NaN exact field returns a zero Key with
// ok=true.
func (view TableFields) Get(field keyspace.Term) (keyspace.Key, bool) {
	if view.result == nil {
		return 0, false
	}
	return view.key(field, keyspace.FamilyTableField, view.result.tableFields.keys)
}

// ExactLenses is the typed normalized-key projection over authored exact Lens
// terms.
type ExactLenses struct{ result *Result }

func (view ExactLenses) Count() int {
	if view.result == nil || !view.result.available() || len(view.result.exactLenses.keys) == 0 {
		return 0
	}
	return len(view.result.exactLenses.keys) - 1
}

func (view ExactLenses) At(index int) (keyspace.Term, bool) {
	if view.result == nil || !view.result.available() || index < 0 || uint64(index+1) >= uint64(len(view.result.exactLenses.keys)) {
		return 0, false
	}
	return keyspace.MakeTerm(keyspace.FamilyLensExact, uint32(index+1)), true
}

// Get is an O(1) lookup into the Source-owned normalized exact-key plane.
func (view ExactLenses) Get(lens keyspace.Term) (keyspace.Key, bool) {
	if view.result == nil {
		return 0, false
	}
	return view.key(lens, keyspace.FamilyLensExact, view.result.exactLenses.keys)
}

// DynamicLenses is the typed dense zero-key projection over authored dynamic
// Lens terms.
type DynamicLenses struct{ result *Result }

func (view DynamicLenses) Count() int {
	if view.result == nil || !view.result.available() || len(view.result.dynamicLenses.keys) == 0 {
		return 0
	}
	return len(view.result.dynamicLenses.keys) - 1
}

func (view DynamicLenses) At(index int) (keyspace.Term, bool) {
	if view.result == nil || !view.result.available() || index < 0 || uint64(index+1) >= uint64(len(view.result.dynamicLenses.keys)) {
		return 0, false
	}
	return keyspace.MakeTerm(keyspace.FamilyLensKey, uint32(index+1)), true
}

// Get returns zero for every valid dynamic Lens. The bool distinguishes a
// valid dynamic row from an invalid term.
func (view DynamicLenses) Get(lens keyspace.Term) (keyspace.Key, bool) {
	if view.result == nil {
		return 0, false
	}
	return view.key(lens, keyspace.FamilyLensKey, view.result.dynamicLenses.keys)
}

func (view TableFields) key(term keyspace.Term, family keyspace.Family, keys []keyspace.Key) (keyspace.Key, bool) {
	if view.result == nil || !view.result.available() || keyspace.TermFamily(term) != family {
		return 0, false
	}
	ordinal := keyspace.TermOrdinal(term)
	if ordinal == 0 || uint64(ordinal) >= uint64(len(keys)) {
		return 0, false
	}
	return keys[ordinal], true
}

func (view ExactLenses) key(term keyspace.Term, family keyspace.Family, keys []keyspace.Key) (keyspace.Key, bool) {
	if view.result == nil || !view.result.available() || keyspace.TermFamily(term) != family {
		return 0, false
	}
	ordinal := keyspace.TermOrdinal(term)
	if ordinal == 0 || uint64(ordinal) >= uint64(len(keys)) {
		return 0, false
	}
	return keys[ordinal], true
}

func (view DynamicLenses) key(term keyspace.Term, family keyspace.Family, keys []keyspace.Key) (keyspace.Key, bool) {
	if view.result == nil || !view.result.available() || keyspace.TermFamily(term) != family {
		return 0, false
	}
	ordinal := keyspace.TermOrdinal(term)
	if ordinal == 0 || uint64(ordinal) >= uint64(len(keys)) {
		return 0, false
	}
	return keys[ordinal], true
}

// IndexAccesses is the route-free candidate indexed-access projection. Reads
// and Writes are independent typed planes and are both retained in canonical
// candidate order.
type IndexAccesses struct{ result *Result }

func (view IndexAccesses) Reads() IndexReads   { return IndexReads{result: view.result} }
func (view IndexAccesses) Writes() IndexWrites { return IndexWrites{result: view.result} }

// IndexReads is the typed candidate IndexGet Read projection.
type IndexReads struct{ result *Result }

func (view IndexReads) Count() int {
	if !view.shapeValid() {
		return 0
	}
	return view.result.indexAccesses.readCount
}

// At enumerates dense candidate ordinals, not authored sparse Read ordinals.
func (view IndexReads) At(index int) (keyspace.Term, bool) {
	access, ok := view.accessAt(index)
	if !ok {
		return 0, false
	}
	return access.Read, true
}

func (view IndexReads) Contains(read keyspace.Term) bool {
	_, ok := view.Slot(read)
	return ok
}

// Slot returns the zero-based dense candidate ordinal for an authored Read.
func (view IndexReads) Slot(read keyspace.Term) (int, bool) {
	access, index, ok := view.access(read)
	return index, ok && access.Read == read
}

// Get returns the evaluated Base, the raw authored Lens key term, and the
// authored Lens identity. For an exact Lens, callers query its normalized key
// in Result.ExactLenses() in O(1); this route row does not duplicate that key.
func (view IndexReads) Get(read keyspace.Term) (base keyspace.Term, keyTerm keyspace.Term, lens keyspace.Term, ok bool) {
	access, _, ok := view.access(read)
	if !ok {
		return 0, 0, 0, false
	}
	return access.Base, access.KeyTerm, access.Lens, true
}

// IndexWrites is the typed candidate IndexSet Write projection.
type IndexWrites struct{ result *Result }

func (view IndexWrites) Count() int {
	if !view.shapeValid() {
		return 0
	}
	return view.result.indexAccesses.writeCount
}

// At enumerates dense candidate ordinals, not authored sparse Write ordinals.
func (view IndexWrites) At(index int) (keyspace.Term, bool) {
	access, ok := view.accessAt(index)
	if !ok {
		return 0, false
	}
	return access.Write, true
}

func (view IndexWrites) Contains(write keyspace.Term) bool {
	_, ok := view.Slot(write)
	return ok
}

// Slot returns the zero-based dense candidate ordinal for an authored Write.
func (view IndexWrites) Slot(write keyspace.Term) (int, bool) {
	access, index, ok := view.access(write)
	return index, ok && access.Write == write
}

// Get returns the evaluated Base, raw authored Lens key term, Assign Values,
// relative Write position, and authored Lens identity. The normalized exact
// key is queried separately from Result.ExactLenses() in O(1).
func (view IndexWrites) Get(write keyspace.Term) (base keyspace.Term, keyTerm keyspace.Term, values keyspace.Term, position int, lens keyspace.Term, ok bool) {
	access, _, ok := view.access(write)
	if !ok {
		return 0, 0, 0, 0, 0, false
	}
	return access.Base, access.KeyTerm, access.Values, access.Position, access.Lens, true
}

func (view IndexReads) shapeValid() bool {
	return indexAccessesShapeValid(view.result)
}

func (view IndexWrites) shapeValid() bool {
	return indexAccessesShapeValid(view.result)
}

func indexAccessesShapeValid(result *Result) bool {
	if result == nil || !result.available() || result.indexAccesses.readCount < 0 || result.indexAccesses.writeCount < 0 {
		return false
	}
	if result.indexAccesses.readCount > len(result.indexAccesses.accesses) || result.indexAccesses.writeCount > len(result.indexAccesses.accesses)-result.indexAccesses.readCount {
		return false
	}
	return result.indexAccesses.readCount+result.indexAccesses.writeCount == len(result.indexAccesses.accesses) &&
		len(result.indexAccesses.reads) > result.indexAccesses.readCount &&
		len(result.indexAccesses.writes) > result.indexAccesses.writeCount
}

func (view IndexReads) accessAt(index int) (indexAccess, bool) {
	if !view.shapeValid() || index < 0 || index >= view.result.indexAccesses.readCount {
		return indexAccess{}, false
	}
	return indexAccessAt(view.result, index)
}

func (view IndexWrites) accessAt(index int) (indexAccess, bool) {
	if !view.shapeValid() || index < 0 || index >= view.result.indexAccesses.writeCount {
		return indexAccess{}, false
	}
	return indexAccessAt(view.result, view.result.indexAccesses.readCount+index)
}

func indexAccessAt(result *Result, index int) (indexAccess, bool) {
	if !indexAccessesShapeValid(result) || index < 0 || index >= len(result.indexAccesses.accesses) {
		return indexAccess{}, false
	}
	access := result.indexAccesses.accesses[index]
	if access.Read == 0 && access.Write == 0 || access.Read != 0 && access.Write != 0 {
		return indexAccess{}, false
	}
	if access.Lens == 0 || access.Base == 0 || access.KeyTerm == 0 || access.Position < -1 {
		return indexAccess{}, false
	}
	keyFamily := keyspace.TermFamily(access.KeyTerm)
	if keyFamily <= keyspace.FamilyInvalid || keyFamily >= keyspace.FamilyCount || keyspace.TermOrdinal(access.KeyTerm) == 0 {
		return indexAccess{}, false
	}
	if access.Read != 0 {
		if index >= result.indexAccesses.readCount || keyspace.TermFamily(access.Read) != keyspace.FamilyRead || access.Values != 0 || access.Position != -1 {
			return indexAccess{}, false
		}
		ordinal := keyspace.TermOrdinal(access.Read)
		if ordinal == 0 || uint64(ordinal) >= uint64(len(result.indexAccesses.reads)) || result.indexAccesses.reads[ordinal] != uint32(index+1) {
			return indexAccess{}, false
		}
	} else {
		if index < result.indexAccesses.readCount || keyspace.TermFamily(access.Write) != keyspace.FamilyWrite || access.Values == 0 || access.Position < 0 {
			return indexAccess{}, false
		}
		ordinal := keyspace.TermOrdinal(access.Write)
		if ordinal == 0 || uint64(ordinal) >= uint64(len(result.indexAccesses.writes)) || result.indexAccesses.writes[ordinal] != uint32(index+1) {
			return indexAccess{}, false
		}
	}
	lf := keyspace.TermFamily(access.Lens)
	if lf == keyspace.FamilyLensExact {
		if _, ok := result.ExactLenses().Get(access.Lens); !ok {
			return indexAccess{}, false
		}
	} else if lf == keyspace.FamilyLensKey {
		if _, ok := result.DynamicLenses().Get(access.Lens); !ok {
			return indexAccess{}, false
		}
	} else {
		return indexAccess{}, false
	}
	return access, true
}

func (view IndexReads) access(read keyspace.Term) (indexAccess, int, bool) {
	if !view.shapeValid() || keyspace.TermFamily(read) != keyspace.FamilyRead {
		return indexAccess{}, 0, false
	}
	ordinal := keyspace.TermOrdinal(read)
	if ordinal == 0 || uint64(ordinal) >= uint64(len(view.result.indexAccesses.reads)) {
		return indexAccess{}, 0, false
	}
	slot := view.result.indexAccesses.reads[ordinal]
	if slot == 0 || uint64(slot) > uint64(view.result.indexAccesses.readCount) {
		return indexAccess{}, 0, false
	}
	access, ok := indexAccessAt(view.result, int(slot-1))
	return access, int(slot - 1), ok && access.Read == read
}

func (view IndexWrites) access(write keyspace.Term) (indexAccess, int, bool) {
	if !view.shapeValid() || keyspace.TermFamily(write) != keyspace.FamilyWrite {
		return indexAccess{}, 0, false
	}
	ordinal := keyspace.TermOrdinal(write)
	if ordinal == 0 || uint64(ordinal) >= uint64(len(view.result.indexAccesses.writes)) {
		return indexAccess{}, 0, false
	}
	slot := view.result.indexAccesses.writes[ordinal]
	if slot == 0 || slot <= uint32(view.result.indexAccesses.readCount) || uint64(slot) > uint64(len(view.result.indexAccesses.accesses)) {
		return indexAccess{}, 0, false
	}
	access, ok := indexAccessAt(view.result, int(slot-1))
	return access, int(slot-1) - view.result.indexAccesses.readCount, ok && access.Write == write
}
