package index

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/relation/state/geometry"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

// ReplayRowIDs visits the live postings for the authenticated relation rows
// named by ids.  The ids must be in the mounted owner-directory order.  The
// physical walk is bounded to the selected posting intervals; it never
// enumerates the relation root to discover which rows were requested.
//
// An unkeyed relation access is the only layout that can redeem a RowID
// without first reconstructing a semantic key tuple. Keyed layouts refuse
// here rather than adding a second RowID inverse or silently falling back to
// a trie scan. A requested row with no live posting is an authenticated
// empty result. All input identities are authenticated before the first
// callback, so a later malformed/foreign identity cannot produce a partial
// replay.
func (borrowed Borrowed) ReplayRowIDs(ids []model.RowID, visit func(Match) bool) (completed, valid bool) {
	if !borrowed.Available() || visit == nil {
		return false, false
	}
	if len(ids) == 0 {
		return true, true
	}
	// An unkeyed posting is addressed by the mounted row-directory coordinate.
	// A keyed trie has no owner-issued inverse from that coordinate to its
	// semantic key tuple; refusing it is the only sound bounded operation.
	if borrowed.state.width != 0 || borrowed.state.root == nil || len(borrowed.state.root.children) != 0 {
		return false, false
	}

	priorOrdinal := -1
	for _, id := range ids {
		ordinal, ok := replayRowOrdinal(borrowed.state.mounted, borrowed.state.relation, id)
		if !ok || priorOrdinal >= ordinal {
			return false, false
		}
		priorOrdinal = ordinal
	}

	postings := borrowed.state.root.postings
	cursor := 0
	for _, id := range ids {
		ordinal, ok := replayRowOrdinal(borrowed.state.mounted, borrowed.state.relation, id)
		if !ok {
			return false, false
		}
		key := geometry.Key(ordinal)
		postingPosition := cursor + sort.Search(len(postings)-cursor, func(candidate int) bool {
			return postings[cursor+candidate].key >= key
		})
		for postingPosition < len(postings) && postings[postingPosition].key == key {
			posting := postings[postingPosition]
			if posting.relation != borrowed.state.relation || posting.row != id || posting.key != key {
				return false, false
			}
			if !visit(Match{posting: &postings[postingPosition]}) {
				return false, true
			}
			postingPosition++
		}
		cursor = postingPosition
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
