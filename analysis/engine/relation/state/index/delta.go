package index

import (
	"bytes"
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/geometry"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/store"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

// Delta is the immutable arrangement difference between exact source-store
// roots. An empty delta may still carry a lineage-only source successor; in
// that case the trie root is shared and no semantic wake is required.
type Delta struct {
	base       Version
	next       Version
	changedKey []geometry.Key
	sealed     bool
}

// Available authenticates exact source ancestry, layout/fence identity, and
// deterministic changed-key order.
func (delta Delta) Available() bool {
	if delta.sealed {
		return true
	}
	return delta.valid()
}

func (delta Delta) valid() bool {
	if !delta.base.Available() || !delta.next.Available() || !delta.next.SuccessorOf(delta.base) || !delta.base.Layout().Equal(delta.next.Layout()) || !delta.base.Fence().Same(delta.next.Fence()) {
		return false
	}
	for position, key := range delta.changedKey {
		if position > 0 && delta.changedKey[position-1] >= key {
			return false
		}
	}
	return true
}

func sealDelta(delta Delta) Delta {
	if delta.valid() {
		delta.sealed = true
	}
	return delta
}

// Empty reports whether no indexed posting changed. Lineage-only and
// irrelevant-column semantic source changes share this result by design.
func (delta Delta) Empty() bool {
	return delta.Available() && delta.base.state.root == delta.next.state.root
}

// Len reports how many geometry keys were re-evaluated.
func (delta Delta) Len() int {
	if !delta.Available() {
		return 0
	}
	return len(delta.changedKey)
}

// KeyAt returns one deterministic re-evaluated geometry key.
func (delta Delta) KeyAt(position int) (geometry.Key, bool) {
	if !delta.Available() || position < 0 || position >= len(delta.changedKey) {
		return 0, false
	}
	return delta.changedKey[position], true
}

// Base returns the exact predecessor index root.
func (delta Delta) Base() Version {
	if !delta.Available() {
		return Version{}
	}
	return delta.base
}

// Next returns the exact successor index root.
func (delta Delta) Next() Version {
	if !delta.Available() {
		return Version{}
	}
	return delta.next
}

// Next advances one index over an exact aggregate store delta. Semantic
// column contents come through the canonical store change stream; no source
// scan or caller row materialization is used. Only geometry keys touched by
// semantic entries to this layout are read again. Lineage-only entries advance
// the source fence while sharing the trie root and never wake the index.
func (version Version) Next(sourceDelta store.Delta, scratch *store.ReadScratch) (Version, Delta, bool) {
	if !version.Available() || !sourceDelta.Available() || !version.state.store.Same(sourceDelta.Base()) || scratch == nil || !scratch.Available() {
		return Version{}, Delta{}, false
	}
	nextStore := sourceDelta.Next()
	if !nextStore.Available() || !nextStore.Fence().Same(version.state.fence) || nextStore.MountedDigest() != version.state.store.MountedDigest() || nextStore.ArrangementDigest() != version.state.store.ArrangementDigest() {
		return Version{}, Delta{}, false
	}
	relevant, keys, ok := relevantDeltaKeys(version, sourceDelta)
	if !ok {
		return Version{}, Delta{}, false
	}
	if !relevant {
		next := version.successor(nextStore, version.state.root, version.state.rowPostings)
		if !next.Available() {
			return Version{}, Delta{}, false
		}
		delta := sealDelta(Delta{base: version, next: next})
		if !delta.Available() {
			return Version{}, Delta{}, false
		}
		return next, delta, true
	}
	root := version.state.root
	rowPostings := version.state.rowPostings
	for _, key := range keys {
		oldRows := rowsAtKey(root, version.state.width, key)
		newRows, readOK := rowsForKey(nextStore, version.state.mounted, version.state.layout, version.state.within, scratch, version.state.fence, key)
		if !readOK {
			return Version{}, Delta{}, false
		}
		newRows, readOK = canonicalRowsWithEquality(version.state.mounted, newRows)
		if !readOK {
			return Version{}, Delta{}, false
		}
		if rowsEqual(version.state.mounted, oldRows, newRows) {
			continue
		}
		for _, old := range oldRows {
			var removed bool
			root, removed = removePosting(root, old.values, postingFromRow(old), version.state.mounted)
			if !removed {
				return Version{}, Delta{}, false
			}
			rowPostings, removed = removeRowPosting(rowPostings, postingFromRow(old), version.state.mounted, version.state.relation)
			if !removed {
				return Version{}, Delta{}, false
			}
		}
		for _, value := range newRows {
			var inserted bool
			root, inserted = insertPosting(root, value.values, postingFromRow(value), version.state.mounted)
			if !inserted {
				return Version{}, Delta{}, false
			}
			rowPostings, inserted = insertRowPosting(rowPostings, postingFromRow(value), version.state.mounted, version.state.relation)
			if !inserted {
				return Version{}, Delta{}, false
			}
		}
	}
	next := version.successor(nextStore, root, rowPostings)
	if !next.Available() {
		return Version{}, Delta{}, false
	}
	delta := sealDelta(Delta{base: version, next: next, changedKey: append([]geometry.Key(nil), keys...)})
	if !delta.Available() {
		return Version{}, Delta{}, false
	}
	return next, delta, true
}

func (version Version) successor(nextStore store.Version, root *trieNode, rowPostings *rowPostingDirectory) Version {
	if !version.Available() || !nextStore.Available() || root == nil || rowPostings == nil {
		return Version{}
	}
	return sealVersion(Version{state: &versionState{
		parent: version.state, mounted: version.state.mounted, store: nextStore,
		layout: version.state.layout, fence: version.state.fence, within: version.state.within,
		relation: version.state.relation,
		manager:  version.state.manager, root: root, rowPostings: rowPostings, rowPostingSealed: true, rows: countPostings(root),
		width: version.state.width,
	}})
}

func relevantDeltaKeys(version Version, sourceDelta store.Delta) (bool, []geometry.Key, bool) {
	relevantIDs := version.state.layout.KeyColumns()
	if len(relevantIDs) == 0 {
		relevantIDs = version.state.layout.Columns()
	}
	// A zero-width relation layout is the owner directory itself. It has no
	// semantic columns to name, but its membership changes when any committed
	// semantic column of that relation changes. Keep this dependency derived
	// from the issued relation identity; never wake it for a foreign relation.
	relationDirectory := len(version.state.layout.KeyColumns()) == 0 && len(version.state.layout.Columns()) == 0
	keySet := make(map[geometry.Key]struct{})
	relevant := false
	for _, change := range sourceDelta.Changes() {
		id := change.ColumnID()
		wanted := relationDirectory && id.Relation() == version.state.relation
		if !wanted {
			for _, candidate := range relevantIDs {
				if candidate == id {
					wanted = true
					break
				}
			}
		}
		if !wanted {
			continue
		}
		if !change.Available() || change.Empty() {
			return false, nil, false
		}
		semanticEntry := false
		for position := 0; position < change.Len(); position++ {
			entry, entryOK := change.At(position)
			if !entryOK || !entry.Region().Valid() || support.Empty(entry.Region()) || entry.Region().Manager() != version.state.manager {
				return false, nil, false
			}
			if !entry.SemanticChanged() {
				continue
			}
			semanticEntry = true
			keySet[entry.Key()] = struct{}{}
		}
		if !semanticEntry {
			continue
		}
		relevant = true
	}
	keys := make([]geometry.Key, 0, len(keySet))
	for key := range keySet {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(left, right int) bool { return keys[left] < keys[right] })
	return relevant, keys, true
}

func rowsForKey(state store.Version, mounted witness.Mounted, layout arrangement.Layout, within support.Mask, scratch *store.ReadScratch, fence binding.Fence, key geometry.Key) ([]row, bool) {
	relation := layout.Access().Relation()
	keyColumns := layout.KeyColumns()
	if len(keyColumns) == 0 && len(layout.Columns()) == 0 {
		// The relation directory has no payload tuple to read. Reuse the same
		// owner-column membership proof as cold materialization, then retain
		// only this changed coordinate. The mounted directory supplies the
		// logical RowID; no row identity or payload is synthesized here.
		all, ok := materializeRelationDirectory(mounted, state, relation, within, scratch)
		if !ok {
			return nil, false
		}
		rows := make([]row, 0, 1)
		for _, value := range all {
			if value.key == key {
				rows = append(rows, value)
			}
		}
		return rows, true
	}
	if len(keyColumns) != 0 {
		parts := make([][]scanPart, len(keyColumns))
		for position, id := range keyColumns {
			value, ok := readColumnAt(state, id, key, within, scratch, fence)
			if !ok {
				return nil, false
			}
			parts[position] = value
		}
		return joinKeyAt(parts, key, within.Manager(), mounted, relation)
	}
	return unkeyedAt(state, layout.Columns(), key, within, scratch, fence, mounted, relation)
}

func readColumnAt(state store.Version, id model.ColumnID, key geometry.Key, within support.Mask, scratch *store.ReadScratch, fence binding.Fence) ([]scanPart, bool) {
	parts := make([]scanPart, 0, 4)
	projectionValid := true
	completed, valid := state.Read(id, key, within, scratch, func(part store.ReadPart) bool {
		if !part.Region().Valid() || support.Empty(part.Region()) || part.Region().Manager() != within.Manager() || !part.Presence().Available() {
			projectionValid = false
			return false
		}
		value := part.Value()
		if value.Available() && !value.ValidFor(fence) {
			projectionValid = false
			return false
		}
		parts = append(parts, scanPart{key: key, region: part.Region(), value: value})
		return true
	})
	return parts, completed && valid && projectionValid
}

func joinKeyAt(parts [][]scanPart, key geometry.Key, manager *guard.Manager, mounted witness.Mounted, relation model.RelationID) ([]row, bool) {
	if len(parts) == 0 || manager == nil {
		return nil, false
	}
	logical, rowOK := mountedRowAt(mounted, relation, key)
	if !rowOK {
		return nil, false
	}
	rows := make([]row, 0)
	var visit func(int, support.Mask, []binding.ValueToken) bool
	visit = func(position int, region support.Mask, values []binding.ValueToken) bool {
		if position == len(parts) {
			rows = append(rows, row{key: key, relation: relation, logical: logical, region: region, values: append([]binding.ValueToken(nil), values...)})
			return true
		}
		for _, part := range parts[position] {
			if !part.value.Available() {
				continue
			}
			overlap, ok := support.Intersect(region, part.region)
			if !ok {
				return false
			}
			if support.Empty(overlap) {
				continue
			}
			if !visit(position+1, overlap, append(values, part.value)) {
				return false
			}
		}
		return true
	}
	// Start with each first-column partition; no value/default is selected if
	// its semantic cell is explicit absence or unproven missing.
	for _, first := range parts[0] {
		if !first.value.Available() {
			continue
		}
		if !visit(1, first.region, []binding.ValueToken{first.value}) {
			return nil, false
		}
	}
	canonical, ok := canonicalRowsWithEquality(mounted, rows)
	if !ok {
		return nil, false
	}
	return canonical, true
}

func unkeyedAt(state store.Version, columns []model.ColumnID, key geometry.Key, within support.Mask, scratch *store.ReadScratch, fence binding.Fence, mounted witness.Mounted, relation model.RelationID) ([]row, bool) {
	var region support.Mask
	for _, id := range columns {
		parts, ok := readColumnAt(state, id, key, within, scratch, fence)
		if !ok {
			return nil, false
		}
		for _, part := range parts {
			if !part.region.Valid() || support.Empty(part.region) {
				continue
			}
			if !region.Valid() {
				region = part.region
				continue
			}
			merged, mergeOK := support.Union(region, part.region)
			if !mergeOK {
				return nil, false
			}
			region = merged
		}
	}
	if !region.Valid() || support.Empty(region) || region.Manager() != within.Manager() {
		return nil, true
	}
	logical, rowOK := mountedRowAt(mounted, relation, key)
	if !rowOK {
		return nil, false
	}
	return []row{{key: key, relation: relation, logical: logical, region: region}}, true
}

func rowsAtKey(root *trieNode, width int, key geometry.Key) []row {
	rows := make([]row, 0)
	collectRowsAtKey(root, width, key, nil, &rows)
	return rows
}

func collectRowsAtKey(node *trieNode, width int, key geometry.Key, values []binding.ValueToken, rows *[]row) {
	if node == nil {
		return
	}
	if len(node.postings) != 0 {
		for _, value := range node.postings {
			if value.key == key {
				*rows = append(*rows, row{key: key, relation: value.relation, logical: value.row, region: value.region, values: append([]binding.ValueToken(nil), values...)})
			}
		}
		return
	}
	if width == 0 {
		return
	}
	for _, edge := range node.children {
		collectRowsAtKey(edge.child, width-1, key, append(values, edge.token), rows)
	}
}

func rowsEqual(mounted witness.Mounted, left, right []row) bool {
	if !mounted.Available() || len(left) != len(right) {
		return false
	}
	// Representative selection is deterministic for each cold materialized
	// input, but an equivalent successor may carry a different opaque encoding
	// for every semantic group. Compare the small changed-key row sets as
	// semantic sets rather than relying on representative order.
	for _, candidate := range left {
		found := false
		for _, replacement := range right {
			if semanticRowEqual(mounted, candidate, replacement) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func postingFromRow(value row) posting {
	regionID, _ := value.region.Identity()
	return posting{key: value.key, relation: value.relation, row: value.logical, region: value.region, regionID: regionID}
}

func postingEqual(left, right posting) bool {
	return left.key == right.key && left.relation == right.relation && left.row == right.row && left.region.Equal(right.region)
}

func insertPosting(node *trieNode, values []binding.ValueToken, value posting, mounted witness.Mounted) (*trieNode, bool) {
	if node == nil || !mounted.Available() {
		return nil, false
	}
	if len(values) != 0 && !indexedTokenValid(mounted, values[0]) {
		return nil, false
	}
	copyOf := &trieNode{children: append([]trieEdge(nil), node.children...), postings: append([]posting(nil), node.postings...)}
	if len(values) == 0 {
		position := postingPosition(copyOf.postings, value)
		if position < len(copyOf.postings) && postingEqual(copyOf.postings[position], value) {
			return node, false
		}
		copyOf.postings = append(copyOf.postings, posting{})
		copy(copyOf.postings[position+1:], copyOf.postings[position:])
		copyOf.postings[position] = value
		return copyOf, true
	}
	position, found := edgePosition(copyOf.children, values[0], mounted)
	if found {
		child, inserted := insertPosting(copyOf.children[position].child, values[1:], value, mounted)
		if !inserted {
			return node, false
		}
		copyOf.children[position].child = child
		return copyOf, true
	}
	child, inserted := insertPosting(&trieNode{}, values[1:], value, mounted)
	if !inserted {
		return node, false
	}
	copyOf.children = append(copyOf.children, trieEdge{})
	copy(copyOf.children[position+1:], copyOf.children[position:])
	copyOf.children[position] = trieEdge{token: values[0], child: child}
	return copyOf, true
}

func removePosting(node *trieNode, values []binding.ValueToken, value posting, mounted witness.Mounted) (*trieNode, bool) {
	if node == nil || !mounted.Available() {
		return nil, false
	}
	if len(values) != 0 && !indexedTokenValid(mounted, values[0]) {
		return node, false
	}
	copyOf := &trieNode{children: append([]trieEdge(nil), node.children...), postings: append([]posting(nil), node.postings...)}
	if len(values) == 0 {
		position := postingPosition(copyOf.postings, value)
		if position >= len(copyOf.postings) || !postingEqual(copyOf.postings[position], value) {
			return node, false
		}
		copyOf.postings = append(copyOf.postings[:position], copyOf.postings[position+1:]...)
		return copyOf, true
	}
	position, found := edgePosition(copyOf.children, values[0], mounted)
	if !found {
		return node, false
	}
	child, removed := removePosting(copyOf.children[position].child, values[1:], value, mounted)
	if !removed {
		return node, false
	}
	if child == nil || (len(child.children) == 0 && len(child.postings) == 0) {
		copyOf.children = append(copyOf.children[:position], copyOf.children[position+1:]...)
	} else {
		copyOf.children[position].child = child
	}
	return copyOf, true
}

func edgePosition(edges []trieEdge, token binding.ValueToken, mounted witness.Mounted) (int, bool) {
	// Resolve semantic aliases before using opaque bytes as the deterministic
	// insertion order. The edge vector is one trie depth, never a posting or
	// relation scan; all semantic identity comes from the mounted authority.
	if !mounted.Available() {
		return 0, false
	}
	for index := range edges {
		if semanticValueEqual(mounted, edges[index].token, token) {
			return index, true
		}
	}
	left, right := 0, len(edges)
	for left < right {
		middle := left + (right-left)/2
		compared := compareValue(edges[middle].token, token)
		if compared < 0 {
			left = middle + 1
		} else {
			right = middle
		}
	}
	return left, false
}

func postingPosition(postings []posting, value posting) int {
	return sort.Search(len(postings), func(position int) bool {
		candidate := postings[position]
		if candidate.key != value.key {
			return candidate.key > value.key
		}
		if candidate.relation != value.relation {
			leftOwner, rightOwner := candidate.relation.Owner().Content(), value.relation.Owner().Content()
			if compared := bytes.Compare(leftOwner[:], rightOwner[:]); compared != 0 {
				return compared > 0
			}
			leftRelation, rightRelation := candidate.relation.Content(), value.relation.Content()
			return bytes.Compare(leftRelation[:], rightRelation[:]) >= 0
		}
		if candidate.row != value.row {
			leftOwner, rightOwner := candidate.row.Relation().Owner().Content(), value.row.Relation().Owner().Content()
			if compared := bytes.Compare(leftOwner[:], rightOwner[:]); compared != 0 {
				return compared > 0
			}
			leftRow, rightRow := candidate.row.Content(), value.row.Content()
			if compared := bytes.Compare(leftRow[:], rightRow[:]); compared != 0 {
				return compared > 0
			}
		}
		return bytes.Compare(candidate.regionID[:], value.regionID[:]) >= 0
	})
}

func countPostings(node *trieNode) uint64 {
	if node == nil {
		return 0
	}
	count := uint64(len(node.postings))
	for _, edge := range node.children {
		count += countPostings(edge.child)
	}
	return count
}
