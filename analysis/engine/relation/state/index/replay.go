package index

import (
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

// ReplayRowIDs visits the live postings for the authenticated relation rows
// named by ids.  The ids must be in the mounted owner-directory order.  The
// physical walk is bounded to the selected posting intervals; it never
// enumerates the relation root to discover which rows were requested.
//
// Each requested row is redeemed through the exact immutable RowID posting
// directory. A requested row with no live posting is an authenticated empty
// result. All input identities are authenticated before the first callback,
// so a later malformed/foreign identity cannot produce a partial replay.
func (borrowed Borrowed) ReplayRowIDs(ids []model.RowID, visit func(Match) bool) (completed, valid bool) {
	if !borrowed.Available() || visit == nil {
		return false, false
	}
	if len(ids) == 0 {
		return true, true
	}
	priorOrdinal := -1
	for _, id := range ids {
		ordinal, ok := replayRowOrdinal(borrowed.state.mounted, borrowed.state.relation, id)
		if !ok || priorOrdinal >= ordinal {
			return false, false
		}
		priorOrdinal = ordinal
	}

	for _, id := range ids {
		completed, valid := borrowed.LookupRow(id, visit)
		if !valid {
			return false, false
		}
		if !completed {
			return false, true
		}
	}
	return true, true
}

// ReplayRowIDs is the direct immutable-version replay entry point.
func (version Version) ReplayRowIDs(ids []model.RowID, visit func(Match) bool) (completed, valid bool) {
	borrowed, ok := version.Borrow()
	if !ok {
		return false, false
	}
	return borrowed.ReplayRowIDs(ids, visit)
}

func replayRowOrdinal(mounted witness.Mounted, relation model.RelationID, id model.RowID) (int, bool) {
	if !relation.Available() || !id.Available() || id.Relation() != relation {
		return 0, false
	}
	ordinal, ok := mounted.RowIndex(relation, id)
	if !ok || ordinal < 0 {
		return 0, false
	}
	redeemed, ok := mounted.RowAt(relation, ordinal)
	return ordinal, ok && redeemed == id
}
