package index

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/geometry"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

// Borrowed is a no-copy read handle over one exact immutable trie root. It
// owns no cursor or result buffer; callers provide only the callback.
type Borrowed struct {
	state *versionState
	fence binding.Fence
}

// Available reports whether this handle still names its complete immutable
// publication root and exact mounted runtime fence.
func (borrowed Borrowed) Available() bool {
	// Borrow authenticates the complete immutable publication once. The state
	// pointer and fence are never mutated afterwards, so the hot read path only
	// needs this constant-time identity check; re-running the cold structural
	// validator here would allocate through defensive catalogue projections.
	return borrowed.state != nil && borrowed.fence.Available() && borrowed.state.fence.Same(borrowed.fence)
}

// Fence returns the exact semantic runtime fence captured on borrow.
func (borrowed Borrowed) Fence() binding.Fence {
	if !borrowed.Available() {
		return binding.Fence{}
	}
	return borrowed.fence
}

// Layout returns the exact physical layout captured on borrow.
func (borrowed Borrowed) Layout() (arrangement.Layout, bool) {
	if !borrowed.Available() {
		return arrangement.Layout{}, false
	}
	return borrowed.state.layout, true
}

// Match is one indexed posting. All fields are borrowed immutable values from
// the authenticated mounted row directory; domain payloads remain opaque.
type Match struct {
	posting *posting
}

// Key returns the mounted scalar row coordinate.
func (match Match) Key() geometry.Key {
	if match.posting == nil {
		return 0
	}
	return match.posting.key
}

// Relation returns the owner-issued relation identity of the posting.
func (match Match) Relation() model.RelationID {
	if match.posting == nil {
		return model.RelationID{}
	}
	return match.posting.relation
}

// Row returns the exact owner-issued logical row identity redeemed from the
// mounted relation-local directory during construction.
func (match Match) Row() model.RowID {
	if match.posting == nil {
		return model.RowID{}
	}
	return match.posting.row
}

// Region returns the exact support partition represented by this posting.
func (match Match) Region() support.Mask {
	if match.posting == nil {
		return support.Mask{}
	}
	return match.posting.region
}

// Lookup visits every posting whose mounted owner-semantic ValueToken tuple
// equals values. Opaque bytes only order representative groups; they are not
// the key relation. Results are deterministic by geometry key and canonical
// support identity. The warm path performs no allocation.
func (borrowed Borrowed) Lookup(values []binding.ValueToken, visit func(Match) bool) (completed, valid bool) {
	if !borrowed.Available() || visit == nil || len(values) != borrowed.state.width {
		return false, false
	}
	for _, value := range values {
		// A same-fence token is not sufficient admission: its TypeID must
		// be one of the mounted key equality authorities. This makes foreign
		// types, decode-only values, and stale/fenced handles refuse rather
		// than silently produce an empty result.
		if !indexedTokenValid(borrowed.state.mounted, value) {
			return false, false
		}
	}
	node := borrowed.state.root
	for _, value := range values {
		position := findEdge(node.children, value, borrowed.state.mounted)
		if position < 0 {
			return true, true
		}
		node = node.children[position].child
	}
	for position := range node.postings {
		if !visit(Match{posting: &node.postings[position]}) {
			return false, true
		}
	}
	return true, true
}

// LookupRow redeems one mounted owner row through the exact immutable RowID
// posting directory. RowIndex/RowAt authenticate the owner-issued logical
// coordinate; the index then addresses its own posting group directly. The
// lookup never converts that coordinate to geometry.Key, walks the semantic
// trie, or scans a relation. A mounted row with no live posting is an
// authenticated empty result, not an invalid handle.
func (borrowed Borrowed) LookupRow(row model.RowID, visit func(Match) bool) (completed, valid bool) {
	if !borrowed.Available() || visit == nil || !row.Available() || row.Relation() != borrowed.state.relation {
		return false, false
	}
	coordinate, ok := replayRowOrdinal(borrowed.state.mounted, borrowed.state.relation, row)
	if !ok || coordinate < 0 {
		return false, false
	}
	node := findRowPostingNode(borrowed.state.rowPostings.root, coordinate)
	if node == nil || node.group.row != row {
		return true, true
	}
	for postingPosition := range node.group.postings {
		posting := node.group.postings[postingPosition]
		if posting.relation != borrowed.state.relation || posting.row != row {
			return false, false
		}
		if !visit(Match{posting: &node.group.postings[postingPosition]}) {
			return false, true
		}
	}
	return true, true
}

// Scan visits every posting in canonical trie order. Its output is the
// arrangement's scan surface for extensional equivalence laws.
func (borrowed Borrowed) Scan(visit func(Match) bool) (completed, valid bool) {
	if !borrowed.Available() || visit == nil {
		return false, false
	}
	return borrowed.scanNode(borrowed.state.root, visit)
}

// Lookup is the direct immutable-version lookup entry point.
func (version Version) Lookup(values []binding.ValueToken, visit func(Match) bool) (completed, valid bool) {
	borrowed, ok := version.Borrow()
	if !ok {
		return false, false
	}
	return borrowed.Lookup(values, visit)
}

// LookupRow is the direct immutable-version row lookup entry point.
func (version Version) LookupRow(row model.RowID, visit func(Match) bool) (completed, valid bool) {
	borrowed, ok := version.Borrow()
	if !ok {
		return false, false
	}
	return borrowed.LookupRow(row, visit)
}

// Scan is the direct immutable-version scan entry point.
func (version Version) Scan(visit func(Match) bool) (completed, valid bool) {
	borrowed, ok := version.Borrow()
	if !ok {
		return false, false
	}
	return borrowed.Scan(visit)
}

func (borrowed Borrowed) scanNode(node *trieNode, visit func(Match) bool) (bool, bool) {
	if node == nil {
		return false, false
	}
	if len(node.postings) != 0 {
		for position := range node.postings {
			if !visit(Match{posting: &node.postings[position]}) {
				return false, true
			}
		}
		return true, true
	}
	for _, edge := range node.children {
		completed, valid := borrowed.scanNode(edge.child, visit)
		if !completed || !valid {
			return completed, valid
		}
	}
	return true, true
}

func findEdge(edges []trieEdge, token binding.ValueToken, mounted witness.Mounted) int {
	// Opaque handles are only a deterministic physical ordering. Semantic
	// equality is owned by the mounted type authority, so an equivalent query
	// token issued with another encoding must redeem the representative edge.
	// This visits trie edges at one coordinate depth only; it never scans
	// postings or invokes a relation-wide Reader scan.
	for index := range edges {
		if semanticValueEqual(mounted, edges[index].token, token) {
			return index
		}
	}
	return -1
}
