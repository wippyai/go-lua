package read

import (
	"github.com/wippyai/go-lua/analysis/engine/relation/state/index"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

// ReplayRowIDs visits the exact filtered subsequence of this Reader's Scan
// for the authenticated relation rows named by ids.  RowIDs are required in
// mounted owner-directory order; the state index performs a bounded posting
// walk and rowsFor preserves every common cofiber, scope, lineage, cell, and
// multiplicity.  Empty input and live rows with no current posting complete
// successfully with no callbacks.
//
// The complete id vector is authenticated before the first callback.  This
// makes foreign, stale, duplicate, and unsorted input a refusal rather than
// a partial replay.  No relation scan, fallback row, or second row wrapper is
// introduced by this seam.
func (handle Reader) ReplayRowIDs(ids []model.RowID, visit func(Row) bool) (completed, valid bool) {
	if !handle.Available() || visit == nil {
		return false, false
	}
	return handle.value.replayRowIDs(ids, visit)
}

func (value *reader) replayRowIDs(ids []model.RowID, visit func(Row) bool) (completed, valid bool) {
	if !value.available() || visit == nil {
		return false, false
	}
	relation := value.layout.Access().Relation()
	if !relation.Available() {
		return false, false
	}
	if len(ids) == 0 {
		return true, true
	}
	// The index's bounded RowID inverse is intentionally limited to unkeyed
	// relation accesses. Refuse keyed arrangements instead of materializing a
	// private key inverse or falling back to Reader.Scan.
	if value.layout.KeyWidth() != 0 || len(value.layout.KeyColumns()) != 0 {
		return false, false
	}
	priorOrdinal := -1
	for _, id := range ids {
		ordinal, ok := replayRowOrdinal(value.mounted, relation, id)
		if !ok || priorOrdinal >= ordinal {
			return false, false
		}
		priorOrdinal = ordinal
	}

	malformed := false
	completed, valid = value.index.ReplayRowIDs(ids, func(match index.Match) bool {
		if match.Relation() != relation {
			malformed = true
			return false
		}
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

func replayRowOrdinal(mounted witness.Mounted, relation model.RelationID, id model.RowID) (int, bool) {
	if !mounted.Available() || !relation.Available() || !id.Available() || id.Relation() != relation {
		return 0, false
	}
	ordinal, ok := mounted.RowIndex(relation, id)
	if !ok || ordinal < 0 {
		return 0, false
	}
	redeemed, ok := mounted.RowAt(relation, ordinal)
	return ordinal, ok && redeemed == id
}
