package index

import (
	"bytes"
	"sort"

	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/geometry"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/store"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

// Version is one immutable canonical trie root for one mounted Layout. Its
// source store version is retained only as an exact replacement fence; value
// payloads remain opaque binding.ValueTokens throughout the index.
type Version struct {
	state *versionState
}

type versionState struct {
	parent   *versionState
	mounted  witness.Mounted
	store    store.Version
	layout   arrangement.Layout
	relation model.RelationID
	fence    binding.Fence
	within   support.Mask
	manager  *guard.Manager
	root     *trieNode
	// rowPostings is the exact immutable inverse of root's postings in the
	// mounted relation directory.  It is an index-owned posting directory,
	// not a read cache: each group is addressed by the owner-issued RowID
	// coordinate and retains canonical posting order for every live posting
	// for that row. Keeping the inverse here lets keyed layouts redeem a RowID
	// without manufacturing a geometry key or traversing the relation trie.
	rowPostings      *rowPostingDirectory
	rowPostingSealed bool
	rows             uint64
	width            int
	sealed           bool
}

type rowPostingGroup struct {
	coordinate int
	row        model.RowID
	postings   []posting
}

type trieNode struct {
	children []trieEdge
	postings []posting
}

type trieEdge struct {
	token binding.ValueToken
	child *trieNode
}

type posting struct {
	key      geometry.Key
	relation model.RelationID
	row      model.RowID
	region   support.Mask
	regionID guard.FormulaID
}

// row is an index-private materialized posting. It is produced only from a
// committed column scan; no public constructor admits caller-authored rows.
type row struct {
	key      geometry.Key
	relation model.RelationID
	logical  model.RowID
	region   support.Mask
	values   []binding.ValueToken
}

func (value row) Available() bool {
	if !value.relation.Available() || !value.logical.Available() || value.logical.Relation() != value.relation || !value.region.Valid() || support.Empty(value.region) {
		return false
	}
	for _, token := range value.values {
		if !token.Available() {
			return false
		}
	}
	return true
}

func semanticRowEqual(mounted witness.Mounted, value, other row) bool {
	if !mounted.Available() || !value.Available() || !other.Available() || value.key != other.key || value.relation != other.relation || value.logical != other.logical || !value.region.Equal(other.region) || len(value.values) != len(other.values) {
		return false
	}
	for position := range value.values {
		if !semanticValueEqual(mounted, value.values[position], other.values[position]) {
			return false
		}
	}
	return true
}

type scanPart struct {
	key    geometry.Key
	region support.Mask
	value  binding.ValueToken
}

// New materializes one arrangement from the exact mounted capability and
// immutable aggregate state. Rows are discovered only through committed
// internal column scans; callers cannot provide a row/materialization list.
//
// Keyed layouts read every owner-declared KeyColumn and intersect their
// support partitions at identical geometry keys, retaining the authored key
// component order. Unkeyed layouts use only the delivered Access columns and
// union their committed support partitions. An empty delivered vector yields
// an empty immutable root; it is never treated as a relation-wide default.
func New(mounted witness.Mounted, state store.Version, layout arrangement.Layout, within support.Mask, scratch *store.ReadScratch) (Version, bool) {
	if !validIngress(mounted, state, layout, within, scratch) {
		return Version{}, false
	}
	rows, ok := materialize(mounted, state, layout, within, scratch)
	if !ok {
		return Version{}, false
	}
	// Equality witnesses do not promise an order or a hash.  Build the cold
	// trie from deterministic opaque representatives, but partition every
	// coordinate with the mounted owner equality authority.
	root := buildTrieWithMounted(rows, layout.KeyWidth(), 0, mounted)
	if root == nil {
		return Version{}, false
	}
	rowPostings, rowPostingsOK := buildRowPostingDirectory(root, layout.KeyWidth(), mounted, layout.Access().Relation())
	if !rowPostingsOK {
		return Version{}, false
	}
	version := sealVersion(Version{state: &versionState{
		mounted: mounted, store: state, layout: layout, fence: mounted.RuntimeFence(),
		relation: layout.Access().Relation(),
		within:   within, manager: within.Manager(), root: root, rowPostings: rowPostings, rowPostingSealed: true, rows: uint64(len(rows)),
		width: layout.KeyWidth(),
	}})
	if !version.Available() {
		return Version{}, false
	}
	return version, true
}

func validIngress(mounted witness.Mounted, state store.Version, layout arrangement.Layout, within support.Mask, scratch *store.ReadScratch) bool {
	if !mounted.Available() || !state.Available() || !layout.Available() || !within.Valid() || scratch == nil || !scratch.Available() || within.Manager() == nil {
		return false
	}
	if scratch.Manager() != within.Manager() {
		return false
	}
	fence := mounted.RuntimeFence()
	if !fence.Available() || !state.Fence().Same(fence) || state.MountedDigest() != mounted.Digest() || state.ArrangementDigest() != mounted.Arrangement().Digest() || !layout.ValidFor(mounted.Fence()) {
		return false
	}
	if !mounted.Arrangement().Available() {
		return false
	}
	belongs := false
	for _, candidate := range mounted.Arrangement().Layouts() {
		if candidate.Equal(layout) {
			belongs = true
			break
		}
	}
	if !belongs {
		return false
	}
	keyColumns := layout.KeyColumns()
	delivered := layout.Columns()
	availableIDs := state.ColumnIDs()
	required := append(keyColumns, delivered...)
	for _, id := range required {
		present := false
		for _, available := range availableIDs {
			if available == id {
				present = true
				break
			}
		}
		if !present || id.Relation() != layout.Access().Relation() {
			return false
		}
	}
	for _, id := range keyColumns {
		column, columnOK := state.Column(id)
		if !columnOK || !column.Available() {
			return false
		}
	}
	return true
}

// Available reports whether this value owns a complete immutable trie and
// exact mounted/source fences.
func (version Version) Available() bool {
	if version.state != nil && version.state.sealed {
		return true
	}
	return version.valid()
}

func (version Version) valid() bool {
	if version.state == nil || !version.state.mounted.Available() || !version.state.store.Available() || !version.state.layout.Available() || !version.state.relation.Available() || version.state.relation != version.state.layout.Access().Relation() || !version.state.fence.Available() || !version.state.within.Valid() || version.state.root == nil || version.state.within.Manager() != version.state.manager || !version.state.store.Fence().Same(version.state.fence) || !version.state.layout.ValidFor(version.state.mounted.Fence()) {
		return false
	}
	return version.state.rowPostings != nil && version.state.rowPostingSealed && trieValid(version.state.root, version.state.width, version.state.manager, version.state.mounted, version.state.relation)
}

// sealVersion validates the complete immutable trie once at construction.
// Runtime readers use the private proof bit and do not walk every posting.
func sealVersion(version Version) Version {
	if version.state == nil || !version.valid() {
		return Version{}
	}
	version.state.sealed = true
	return version
}

// Digest returns the immutable physical layout identity owned by this index.
// The aggregate store consumes this neutral identity contract without
// importing index, avoiding a store↔index package cycle. Dynamic posting-root
// identity remains guarded by Same/SuccessorOf and the source store delta.
func (version Version) Digest() identity.ContentID {
	if !version.Available() {
		return identity.ContentID{}
	}
	return version.state.layout.Digest()
}

// Same reports exact immutable publication-root identity.
func (version Version) Same(other Version) bool {
	return version.Available() && other.Available() && version.state == other.state
}

// SuccessorOf proves direct immutable ancestry, including the exact mounted
// layout and source runtime fence.
func (version Version) SuccessorOf(base Version) bool {
	return version.Available() && base.Available() && !version.Same(base) && version.state.parent == base.state && version.state.layout.Equal(base.state.layout) && version.Fence().Same(base.Fence()) && version.state.store.SuccessorOf(base.state.store)
}

// Layout returns the exact mounted physical layout.
func (version Version) Layout() arrangement.Layout {
	if !version.Available() {
		return arrangement.Layout{}
	}
	return version.state.layout
}

// Fence returns the exact mounted semantic runtime fence.
func (version Version) Fence() binding.Fence {
	if !version.Available() {
		return binding.Fence{}
	}
	return version.state.fence
}

// Source returns the exact immutable aggregate root used to build this index.
func (version Version) Source() store.Version {
	if !version.Available() {
		return store.Version{}
	}
	return version.state.store
}

// RowCount reports the number of canonical physical postings.
func (version Version) RowCount() int {
	if !version.Available() || version.state.rows > uint64(^uint(0)>>1) {
		return 0
	}
	return int(version.state.rows)
}

// Borrow opens a no-copy lookup handle over this exact immutable root.
func (version Version) Borrow() (Borrowed, bool) {
	if !version.Available() {
		return Borrowed{}, false
	}
	return Borrowed{state: version.state, fence: version.state.fence}, true
}

// canonicalRowsWithEquality canonicalizes every indexed tuple component to a
// mounted owner-equality representative before ordering/building the trie.
// ValueToken.Opaque remains an authentication payload and a deterministic
// tie-breaker only; it is never the semantic key relation. This cold pass is
// intentionally outside Reader.Lookup, so a query issued from an equivalent
// but differently encoded token reaches the same trie edge without scanning
// postings or introducing a second equality registry.
func canonicalRowsWithEquality(mounted witness.Mounted, rows []row) ([]row, bool) {
	if !mounted.Available() {
		return nil, false
	}
	for position := range rows {
		if !rows[position].Available() {
			return nil, false
		}
		rows[position] = cloneRow(rows[position])
	}
	// Sort before choosing representatives.  This makes the representative
	// independent of scan/map iteration order: the first token encountered in
	// a semantic class is its least opaque ordering representative.  Opaque
	// bytes are used only for this deterministic presentation order, never as
	// the equality relation.
	sort.SliceStable(rows, func(left, right int) bool { return rowLess(rows[left], rows[right]) })
	representatives := make([][]binding.ValueToken, 0)
	for position := range rows {
		for valueIndex, token := range rows[position].values {
			if !indexedTokenValid(mounted, token) {
				return nil, false
			}
			for len(representatives) <= valueIndex {
				representatives = append(representatives, nil)
			}
			found := false
			for _, representative := range representatives[valueIndex] {
				if semanticValueEqual(mounted, representative, token) {
					rows[position].values[valueIndex] = representative
					found = true
					break
				}
			}
			if !found {
				representatives[valueIndex] = append(representatives[valueIndex], token)
			}
		}
	}
	// Re-sort after canonicalization.  Equal owner values now have the same
	// physical representative, while all remaining tie-breakers stay
	// deterministic.
	sort.SliceStable(rows, func(left, right int) bool { return rowLess(rows[left], rows[right]) })
	result := rows[:0]
	for _, value := range rows {
		// A keyed relation has one logical row identity for each declared key
		// tuple.  The identity is issued by the relation owner and is carried
		// through every physical arrangement; it is not selected by this
		// index, by scope order, or by the first posting encountered.  Equal
		// tuples with different RowIDs therefore prove an invalid owner
		// publication and must refuse at the state boundary.  The same tuple
		// may occur in several guard fibers, but those occurrences must retain
		// the same RowID.  Unkeyed arrangements have no tuple identity and are
		// intentionally not subject to this check.
		if len(value.values) != 0 && len(result) != 0 {
			prior := result[len(result)-1]
			if prior.relation == value.relation && compareValues(prior.values, value.values) == 0 && prior.logical != value.logical {
				return nil, false
			}
		}
		if len(result) == 0 || !semanticRowEqual(mounted, result[len(result)-1], value) {
			result = append(result, value)
		}
	}
	return result, true
}

func cloneRow(value row) row {
	return row{key: value.key, relation: value.relation, logical: value.logical, region: value.region, values: append([]binding.ValueToken(nil), value.values...)}
}

func rowLess(left, right row) bool {
	if compared := compareValues(left.values, right.values); compared != 0 {
		return compared < 0
	}
	if left.key != right.key {
		return left.key < right.key
	}
	leftRelationOwner, rightRelationOwner := left.relation.Owner().Content(), right.relation.Owner().Content()
	if compared := bytes.Compare(leftRelationOwner[:], rightRelationOwner[:]); compared != 0 {
		return compared < 0
	}
	leftRelation, rightRelation := left.relation.Content(), right.relation.Content()
	if compared := bytes.Compare(leftRelation[:], rightRelation[:]); compared != 0 {
		return compared < 0
	}
	leftLogicalOwner, rightLogicalOwner := left.logical.Relation().Owner().Content(), right.logical.Relation().Owner().Content()
	if compared := bytes.Compare(leftLogicalOwner[:], rightLogicalOwner[:]); compared != 0 {
		return compared < 0
	}
	leftLogical, rightLogical := left.logical.Content(), right.logical.Content()
	if compared := bytes.Compare(leftLogical[:], rightLogical[:]); compared != 0 {
		return compared < 0
	}
	leftID, leftOK := left.region.Identity()
	rightID, rightOK := right.region.Identity()
	if leftOK != rightOK {
		return leftOK
	}
	return leftOK && bytes.Compare(leftID[:], rightID[:]) < 0
}

func scanPartLess(left, right scanPart) bool {
	leftID, leftOK := left.region.Identity()
	rightID, rightOK := right.region.Identity()
	if leftOK != rightOK {
		return leftOK
	}
	return leftOK && bytes.Compare(leftID[:], rightID[:]) < 0
}

func compareValues(left, right []binding.ValueToken) int {
	for position := 0; position < len(left) && position < len(right); position++ {
		if compared := compareValue(left[position], right[position]); compared != 0 {
			return compared
		}
	}
	if len(left) < len(right) {
		return -1
	}
	if len(left) > len(right) {
		return 1
	}
	return 0
}

func compareValue(left, right binding.ValueToken) int {
	leftType, rightType := left.Type(), right.Type()
	if compared := compareNominal(leftType.Owner().Content(), leftType.Content(), rightType.Owner().Content(), rightType.Content()); compared != 0 {
		return compared
	}
	leftOpaque, rightOpaque := left.Opaque(), right.Opaque()
	return bytes.Compare(leftOpaque[:], rightOpaque[:])
}

func compareNominal(leftOwner, leftContent, rightOwner, rightContent identity.ContentID) int {
	if compared := bytes.Compare(leftOwner[:], rightOwner[:]); compared != 0 {
		return compared
	}
	return bytes.Compare(leftContent[:], rightContent[:])
}

func buildTrieWithMounted(rows []row, width, depth int, mounted witness.Mounted) *trieNode {
	if !mounted.Available() {
		return nil
	}
	return buildTrieOrdered(rows, width, depth, mounted)
}

type trieRowGroup struct {
	representative binding.ValueToken
	rows           []row
}

// buildTrieOrdered uses opaque order only to order canonical representative
// groups.  Since ValueEquality exposes no lawful order/hash, group admission
// and lookup are linear in the number of semantic groups at a depth.  This is
// the smallest truthful representation; claiming logarithmic lookup from
// opaque bytes would silently turn physical encoding into semantic equality.
func buildTrieOrdered(rows []row, width, depth int, mounted witness.Mounted) *trieNode {
	node := &trieNode{}
	if !mounted.Available() || width < 0 || depth < 0 || depth > width {
		return nil
	}
	if len(rows) == 0 {
		return node
	}
	if depth == width {
		node.postings = make([]posting, 0, len(rows))
		for _, value := range rows {
			regionID, _ := value.region.Identity()
			node.postings = append(node.postings, posting{key: value.key, relation: value.relation, row: value.logical, region: value.region, regionID: regionID})
		}
		return node
	}
	// The materializer normally supplies canonical rows already, but sorting
	// here keeps direct cold rebuilds deterministic as well.
	sort.SliceStable(rows, func(left, right int) bool { return rowLess(rows[left], rows[right]) })
	groups := make([]trieRowGroup, 0)
	for _, value := range rows {
		if depth >= len(value.values) {
			return nil
		}
		token := value.values[depth]
		if !indexedTokenValid(mounted, token) {
			return nil
		}
		groupIndex := -1
		for index := range groups {
			if semanticValueEqual(mounted, groups[index].representative, token) {
				groupIndex = index
				break
			}
		}
		if groupIndex < 0 {
			groups = append(groups, trieRowGroup{representative: token, rows: []row{value}})
			continue
		}
		groups[groupIndex].rows = append(groups[groupIndex].rows, value)
	}
	sort.SliceStable(groups, func(left, right int) bool {
		return compareValue(groups[left].representative, groups[right].representative) < 0
	})
	for position, group := range groups {
		if position > 0 && compareValue(groups[position-1].representative, group.representative) >= 0 {
			// A lawful owner equality is reflexive, so this can only be a
			// malformed/ambiguous representative set. Refuse instead of
			// allowing opaque collision to select a semantic bucket.
			return nil
		}
		child := buildTrieOrdered(group.rows, width, depth+1, mounted)
		if child == nil {
			return nil
		}
		node.children = append(node.children, trieEdge{token: group.representative, child: child})
	}
	return node
}

func trieValid(node *trieNode, width int, manager *guard.Manager, mounted witness.Mounted, relation model.RelationID) bool {
	if node == nil || width < 0 {
		return false
	}
	if len(node.postings) != 0 {
		if len(node.children) != 0 || !postingsValid(node.postings, manager, mounted, relation) {
			return false
		}
	}
	if width == 0 && len(node.children) != 0 {
		return false
	}
	for position, edge := range node.children {
		if !indexedTokenValid(mounted, edge.token) || edge.child == nil || !trieValid(edge.child, width-1, manager, mounted, relation) {
			return false
		}
		if position > 0 && compareValue(node.children[position-1].token, edge.token) >= 0 {
			return false
		}
		for prior := 0; prior < position; prior++ {
			if semanticValueEqual(mounted, node.children[prior].token, edge.token) {
				// One semantic tuple coordinate must have one canonical trie
				// edge. Equality is not assumed to follow opaque ordering.
				return false
			}
		}
	}
	return true
}

func indexedTokenValid(mounted witness.Mounted, token binding.ValueToken) bool {
	if !mounted.Available() || !token.ValidFor(mounted.RuntimeFence()) {
		return false
	}
	equality, ok := mounted.Equality(token.Type())
	return ok && equality != nil && equality.Type() == token.Type()
}

func semanticValueEqual(mounted witness.Mounted, left, right binding.ValueToken) bool {
	if !mounted.Available() || !left.Available() || !right.Available() || left.Type() != right.Type() || !left.ValidFor(mounted.RuntimeFence()) || !right.ValidFor(mounted.RuntimeFence()) {
		return false
	}
	equality, ok := mounted.Equality(left.Type())
	// The mounted authority is the sole semantic relation. In particular,
	// equal opaque bytes do not bypass an owner witness, and a witness for a
	// different TypeID cannot widen the comparison domain.
	return ok && equality != nil && equality.Type() == left.Type() && equality.Equal(left, right)
}

func postingsValid(postings []posting, manager *guard.Manager, mounted witness.Mounted, relation model.RelationID) bool {
	for position, value := range postings {
		if value.relation != relation || !value.relation.Available() || !value.row.Available() || value.row.Relation() != value.relation || !value.region.Valid() || support.Empty(value.region) || value.region.Manager() != manager {
			return false
		}
		logical, rowOK := mountedRowAt(mounted, relation, value.key)
		if !rowOK || logical != value.row {
			return false
		}
		identityValue, ok := value.region.Identity()
		if !ok || identityValue != value.regionID {
			return false
		}
		if position > 0 {
			prior := postings[position-1]
			if prior.key > value.key || (prior.key == value.key && (prior.relation != value.relation || prior.row != value.row || bytes.Compare(prior.regionID[:], value.regionID[:]) >= 0)) {
				return false
			}
		}
	}
	return true
}
