package read

import (
	"github.com/wippyai/go-lua/analysis/engine/relation/state/index"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

func (handle Reader) Scan(visit func(Row) bool) (completed, valid bool) {
	if !handle.Available() {
		return false, false
	}
	return handle.value.Scan(visit)
}

func (handle Reader) Lookup(tuple Tuple, visit func(Row) bool) (completed, valid bool) {
	if !handle.Available() {
		return false, false
	}
	return handle.value.Lookup(tuple, visit)
}

// LookupRowID visits every live common-cofiber row owned by id in this exact
// committed reader.  RowID admission is deliberately directory-backed: the
// mounted RowIndex/RowAt round-trip authenticates the relation-local logical
// identity before the immutable index posting is read.  A valid directory
// member with no posting is an ordinary completed empty result; an unknown,
// foreign, or stale identity refuses the read.
func (handle Reader) LookupRowID(id model.RowID, visit func(Row) bool) (completed, valid bool) {
	if !handle.Available() {
		return false, false
	}
	return handle.value.LookupRowID(id, visit)
}

func (value *reader) Scan(visit func(Row) bool) (completed, valid bool) {
	if !value.available() || visit == nil {
		return false, false
	}
	malformed := false
	completed, valid = value.index.Scan(func(match index.Match) bool {
		candidates, ok := value.rowsFor(match)
		if !ok {
			malformed = true
			return false
		}
		for _, candidate := range candidates {
			if !visit(candidate) {
				return false
			}
		}
		return true
	})
	if malformed {
		return false, false
	}
	return completed, valid
}

func (value *reader) Lookup(tuple Tuple, visit func(Row) bool) (completed, valid bool) {
	if !value.available() || visit == nil || !tuple.Available() || !tuple.fence.Same(value.fence) || tuple.owner == nil || !tuple.owner.root.Same(value.root) || len(tuple.values) != len(value.types) {
		return false, false
	}
	malformed := false
	completed, valid = value.index.Lookup(tuple.values, func(match index.Match) bool {
		candidates, ok := value.rowsFor(match)
		if !ok {
			malformed = true
			return false
		}
		for _, candidate := range candidates {
			if !visit(candidate) {
				return false
			}
		}
		return true
	})
	if malformed {
		return false, false
	}
	return completed, valid
}

func (value *reader) LookupRowID(id model.RowID, visit func(Row) bool) (completed, valid bool) {
	if !value.available() || visit == nil || !id.Available() {
		return false, false
	}
	relation := value.layout.Access().Relation()
	if !relation.Available() || id.Relation() != relation {
		return false, false
	}
	// The logical identity is authenticated by the mounted relation directory,
	// never by converting an arbitrary RowID or geometry coordinate.  Redeem
	// both directions so a stale/corrupt directory cannot become an empty
	// successful lookup.
	position, ok := value.mounted.RowIndex(relation, id)
	if !ok || position < 0 {
		return false, false
	}
	redeemed, ok := value.mounted.RowAt(relation, position)
	if !ok || redeemed != id || redeemed.Relation() != relation {
		return false, false
	}
	malformed := false
	completed, valid = value.index.LookupRow(id, func(match index.Match) bool {
		// The index is already bound to this layout/relation.  Keep this check
		// explicit at the logical-row seam so an impossible foreign posting
		// fails closed instead of being mistaken for an empty result.
		if match.Relation() != relation {
			malformed = true
			return false
		}
		if match.Row() != id {
			return true
		}
		// rowsFor rechecks the mounted RowAt/RowIndex inverse for the exact
		// posting and reads the store's full common cofiber.  In particular,
		// do not emit the first cell/row and do not coalesce support fibers.
		candidates, candidatesOK := value.rowsFor(match)
		if !candidatesOK {
			malformed = true
			return false
		}
		for _, candidate := range candidates {
			if !visit(candidate) {
				return false
			}
		}
		return true
	})
	if malformed {
		return false, false
	}
	return completed, valid
}
