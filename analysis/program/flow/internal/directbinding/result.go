package directbinding

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

type selectionPlane uint8

const (
	selectionPlaneInvalid selectionPlane = iota
	selectionPlaneRead
	selectionPlanePublication
)

// selectionRow is the one shared exact-selector row. Parent is a one-based
// row ordinal; zero denotes a root.  Read and owner identity are supplied by
// the dense Read/Publication planes rather than duplicated in every row.
type selectionRow struct {
	root     keyspace.Term
	parent   uint32
	suffix   keyspace.Key
	depth    uint32
	external bool
	plane    selectionPlane
	// typePath is a construction-time predicate, not a generic path kind:
	// every retained suffix so far was authored as FieldName. A normalized
	// FieldExact suffix may select a Read but clears this predicate, so Static
	// publication geometry can reject it without another chain representation.
	typePath bool
}

type directCallRow struct {
	read keyspace.Term
	form CallForm
}

// Result is the immutable direct-binding proof.  Each source relation has a
// dense one-based slot with zero as explicit absence; all exact suffixes share
// the compact parent-chain rows.
//
// No Source, Flow, Static, Module, Link, or artifact owner/payload survives
// sealing; only scalar lifecycle identity fences remain.
type Result struct {
	selections  []selectionRow
	rowReads    []keyspace.Term
	readSlots   []uint32
	publication []uint32
	// publicationStart is the first row ordinal in the publication plane;
	// every Read row is strictly before it.
	publicationStart uint32
	// Publication owner Bodies are the typed owner scalar returned by the
	// publication projection; they are not retained owner pointers.
	publicationOwners []keyspace.Term
	directCalls       []directCallRow
	// These scalar fences do not retain any owner graph. They make a sealed
	// result reject equal-cardinality views from a different owner lifecycle.
	sourceID identity.ContentID
	flowID   identity.ContentID
	staticID identity.ContentID
	moduleID identity.ContentID
}

// BindingSelections returns the external exact-Read view.
func (r *Result) BindingSelections() BindingSelections { return BindingSelections{result: r} }

// PublicationPaths returns the exact Static publication view.
func (r *Result) PublicationPaths() PublicationPaths { return PublicationPaths{result: r} }

// DirectCalls returns the exact direct-call view.
func (r *Result) DirectCalls() DirectCalls { return DirectCalls{result: r} }

// Matches reports whether r belongs to the exact Source preimage, authored
// Flow, Static, and Module views supplied to Seal.
func Matches(r *Result, sourceID, flowID, staticID, moduleID identity.ContentID) bool {
	return r != nil && r.available() && sourceID.Available() && flowID.Available() && staticID.Available() && moduleID.Available() &&
		r.sourceID == sourceID && r.flowID == flowID && r.staticID == staticID && r.moduleID == moduleID
}

func (r *Result) available() bool {
	return r != nil && r.sourceID.Available() && r.flowID.Available() && r.staticID.Available() && r.moduleID.Available()
}

// Count reports the dense Read denominator. A zero slot is an explicit
// absent selection.
func (view BindingSelections) Count() int {
	if view.result == nil || !view.result.available() || len(view.result.readSlots) == 0 {
		return 0
	}
	return len(view.result.readSlots) - 1
}

// Get returns an external Read's root and exact suffix depth in O(1).
func (view BindingSelections) Get(read keyspace.Term) (root keyspace.Term, depth int, ok bool) {
	_, row, ok := view.readRow(read)
	if !ok || !row.external {
		return 0, 0, false
	}
	return row.root, int(row.depth), true
}

// PathCursor returns an immutable exact-suffix cursor for one external Read.
// Segment follows the sealed parent chain from leaf to root, one existing
// selector-chain edge per O(1) call. This parent-chain orientation lets callers
// prove exact termination without copying the path.
func (view BindingSelections) PathCursor(read keyspace.Term) (BindingPath, bool) {
	rowIndex, row, ok := view.readRow(read)
	if !ok || !row.external || !validCellTerm(row.root) ||
		(row.depth == 0 && (row.parent != 0 || row.suffix != 0)) ||
		(row.depth != 0 && (row.parent == 0 || row.parent >= rowIndex || row.suffix == 0)) {
		return BindingPath{}, false
	}
	return BindingPath{
		result:    view.result,
		current:   rowIndex,
		root:      row.root,
		remaining: row.depth,
	}, true
}

// Segment returns the current leaf-to-root suffix and an immutable cursor for
// the remainder. A final successful Segment proves that its parent is the
// exact depth-zero root; the following Segment therefore fails closed. Every
// call is O(1) and allocation-free.
func (path BindingPath) Segment() (keyspace.Key, BindingPath, bool) {
	key, current, remaining, ok := path.result.nextSegment(path.current, path.root, path.remaining, false)
	if !ok {
		return 0, BindingPath{}, false
	}
	return key, BindingPath{
		result:    path.result,
		current:   current,
		root:      path.root,
		remaining: remaining,
	}, true
}

// Count reports the dense TypePublication denominator.
func (view PublicationPaths) Count() int {
	if view.result == nil || !view.result.available() || len(view.result.publication) == 0 {
		return 0
	}
	return len(view.result.publication) - 1
}

// Get returns one publication's root Cell, Assign owner Body, and exact
// dotted/name depth. Lexical visibility is intentionally not checked here.
func (view PublicationPaths) Get(publication keyspace.Term) (root, owner keyspace.Term, depth int, ok bool) {
	_, row, ok := view.publicationRow(publication)
	if !ok || !row.typePath || row.root == 0 || row.depth == 0 || uint64(keyspace.TermOrdinal(publication)) >= uint64(len(view.result.publicationOwners)) {
		return 0, 0, 0, false
	}
	owner = view.result.publicationOwners[keyspace.TermOrdinal(publication)]
	if owner == 0 {
		return 0, 0, 0, false
	}
	return row.root, owner, int(row.depth), true
}

// PathCursor returns an immutable exact-suffix cursor for one publication.
// Segment follows the sealed parent chain from leaf to root, allowing Static
// to prove every canonical segment and exact termination without retaining a
// duplicate path.
func (view PublicationPaths) PathCursor(publication keyspace.Term) (PublicationPath, bool) {
	rowIndex, row, ok := view.publicationRow(publication)
	if !ok || !row.typePath || row.root == 0 || row.depth == 0 {
		return PublicationPath{}, false
	}
	return PublicationPath{
		result:    view.result,
		current:   rowIndex,
		root:      row.root,
		remaining: row.depth,
	}, true
}

// Segment returns the current leaf-to-root publication suffix and an immutable
// cursor for the remainder. The final successful Segment validates the exact
// depth-zero root, and the following Segment fails closed. Every call is O(1)
// and allocation-free.
func (path PublicationPath) Segment() (keyspace.Key, PublicationPath, bool) {
	key, current, remaining, ok := path.result.nextSegment(path.current, path.root, path.remaining, true)
	if !ok {
		return 0, PublicationPath{}, false
	}
	return key, PublicationPath{
		result:    path.result,
		current:   current,
		root:      path.root,
		remaining: remaining,
	}, true
}

// Count reports the dense Call denominator.
func (view DirectCalls) Count() int {
	if view.result == nil || !view.result.available() || len(view.result.directCalls) == 0 {
		return 0
	}
	return len(view.result.directCalls) - 1
}

// Get returns the exact selected Read and authored plain/method form in O(1).
func (view DirectCalls) Get(call keyspace.Term) (read keyspace.Term, form CallForm, ok bool) {
	if view.result == nil || !view.result.available() || keyspace.TermFamily(call) != keyspace.FamilyCall {
		return 0, 0, false
	}
	ordinal := keyspace.TermOrdinal(call)
	if ordinal == 0 || uint64(ordinal) >= uint64(len(view.result.directCalls)) {
		return 0, 0, false
	}
	row := view.result.directCalls[ordinal]
	if row.read == 0 || !row.form.valid() || !view.result.validRead(row.read) {
		return 0, 0, false
	}
	return row.read, row.form, true
}

func (view BindingSelections) readRow(read keyspace.Term) (uint32, selectionRow, bool) {
	if view.result == nil || !view.result.available() || keyspace.TermFamily(read) != keyspace.FamilyRead {
		return 0, selectionRow{}, false
	}
	ordinal := keyspace.TermOrdinal(read)
	if ordinal == 0 || uint64(ordinal) >= uint64(len(view.result.readSlots)) {
		return 0, selectionRow{}, false
	}
	rowIndex := view.result.readSlots[ordinal]
	row, ok := view.result.row(rowIndex)
	if !ok || !view.result.validBindingRow(rowIndex, row) {
		return 0, selectionRow{}, false
	}
	rowRead, ok := view.result.rowRead(rowIndex)
	if !ok || rowRead != read {
		return 0, selectionRow{}, false
	}
	return rowIndex, row, true
}

func (view PublicationPaths) publicationRow(publication keyspace.Term) (uint32, selectionRow, bool) {
	if view.result == nil || !view.result.available() || keyspace.TermFamily(publication) != keyspace.FamilyTypePublication {
		return 0, selectionRow{}, false
	}
	ordinal := keyspace.TermOrdinal(publication)
	if ordinal == 0 || uint64(ordinal) >= uint64(len(view.result.publication)) {
		return 0, selectionRow{}, false
	}
	rowIndex := view.result.publication[ordinal]
	row, ok := view.result.row(rowIndex)
	if !ok || !view.result.validPublicationRow(rowIndex, row) || uint64(ordinal) >= uint64(len(view.result.publicationOwners)) || !validBodyTerm(view.result.publicationOwners[ordinal]) {
		return 0, selectionRow{}, false
	}
	return rowIndex, row, true
}

func (r *Result) row(index uint32) (selectionRow, bool) {
	if r == nil || index == 0 || uint64(index) > uint64(len(r.selections)) {
		return selectionRow{}, false
	}
	return r.selections[index-1], true
}

func (r *Result) rowRead(index uint32) (keyspace.Term, bool) {
	if r == nil || index == 0 || uint64(index) > uint64(len(r.rowReads)) {
		return 0, false
	}
	return r.rowReads[index-1], true
}

func validCellTerm(term keyspace.Term) bool {
	return keyspace.TermFamily(term) == keyspace.FamilyCell && keyspace.TermOrdinal(term) != 0
}

func validBodyTerm(term keyspace.Term) bool {
	return keyspace.TermFamily(term) == keyspace.FamilyBody && keyspace.TermOrdinal(term) != 0
}

func (r *Result) validReadRow(index uint32, row selectionRow) bool {
	if r == nil || index == 0 || r.publicationStart == 0 || index >= r.publicationStart || row.plane != selectionPlaneRead || !validCellTerm(row.root) {
		return false
	}
	read, ok := r.rowRead(index)
	if !ok || keyspace.TermFamily(read) != keyspace.FamilyRead || keyspace.TermOrdinal(read) == 0 {
		return false
	}
	ordinal := keyspace.TermOrdinal(read)
	return uint64(ordinal) < uint64(len(r.readSlots)) && r.readSlots[ordinal] == index
}

func (r *Result) validBindingRow(index uint32, row selectionRow) bool {
	return r.validReadRow(index, row) && row.external
}

func (r *Result) validPublicationRow(index uint32, row selectionRow) bool {
	if r == nil || index == 0 || r.publicationStart == 0 || index < r.publicationStart || row.plane != selectionPlanePublication || !row.typePath || !validCellTerm(row.root) || row.depth == 0 || row.parent == 0 || row.parent >= index || row.suffix == 0 {
		return false
	}
	publicationOrdinal := uint64(index-r.publicationStart) + 1
	if publicationOrdinal >= uint64(len(r.publication)) || r.publication[publicationOrdinal] != index || publicationOrdinal >= uint64(len(r.publicationOwners)) || !validBodyTerm(r.publicationOwners[publicationOrdinal]) {
		return false
	}
	parent, parentOK := r.row(row.parent)
	if !parentOK || !r.validReadRow(row.parent, parent) || !parent.typePath || parent.root != row.root || parent.depth+1 != row.depth {
		return false
	}
	read, ok := r.rowRead(index)
	return ok && read == 0
}

func (r *Result) validRead(read keyspace.Term) bool {
	if r == nil || !r.available() || keyspace.TermFamily(read) != keyspace.FamilyRead {
		return false
	}
	ordinal := keyspace.TermOrdinal(read)
	if ordinal == 0 || uint64(ordinal) >= uint64(len(r.readSlots)) {
		return false
	}
	rowIndex := r.readSlots[ordinal]
	row, ok := r.row(rowIndex)
	if !ok || !r.validBindingRow(rowIndex, row) {
		return false
	}
	rowRead, ok := r.rowRead(rowIndex)
	return ok && rowRead == read
}

// nextSegment consumes one row in a sealed selector chain. remaining is the
// depth expected at current; requiring each parent to decrement it proves
// both acyclicity and the exact root termination without a second path store.
// requireTypePath is selected only by the typed PublicationPath projection;
// it is a construction predicate, not Static knowledge retained in Result.
func (r *Result) nextSegment(current uint32, root keyspace.Term, remaining uint32, requireTypePath bool) (keyspace.Key, uint32, uint32, bool) {
	if r == nil || !r.available() || current == 0 || root == 0 || remaining == 0 {
		return 0, 0, 0, false
	}
	row, ok := r.row(current)
	if !ok || row.root != root || row.depth != remaining || row.suffix == 0 || !validCellTerm(row.root) || (requireTypePath && !row.typePath) {
		return 0, 0, 0, false
	}
	if requireTypePath {
		if row.plane == selectionPlanePublication {
			if !r.validPublicationRow(current, row) {
				return 0, 0, 0, false
			}
		} else if row.plane != selectionPlaneRead || !r.validReadRow(current, row) {
			return 0, 0, 0, false
		}
	} else if !r.validBindingRow(current, row) {
		return 0, 0, 0, false
	}
	if row.parent == 0 || row.parent >= current {
		return 0, 0, 0, false
	}
	parent, ok := r.row(row.parent)
	if !ok || parent.root != root || parent.depth+1 != row.depth || (requireTypePath && !parent.typePath) {
		return 0, 0, 0, false
	}
	if parent.depth == 0 {
		if parent.parent != 0 || parent.suffix != 0 {
			return 0, 0, 0, false
		}
	} else if parent.parent == 0 || parent.parent >= row.parent || parent.suffix == 0 {
		return 0, 0, 0, false
	}
	if requireTypePath {
		if !r.validReadRow(row.parent, parent) {
			return 0, 0, 0, false
		}
	} else if !r.validBindingRow(row.parent, parent) {
		return 0, 0, 0, false
	}
	if remaining == 1 {
		if parent.depth != 0 || parent.parent != 0 || parent.suffix != 0 {
			return 0, 0, 0, false
		}
		return row.suffix, 0, 0, true
	}
	return row.suffix, row.parent, remaining - 1, true
}
