package accessgeometry

import (
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// Count reports the dense Read denominator. A zero slot is an explicit
// absent selection.
func (view ExactReads) Count() int {
	if view.result == nil || !view.result.available() || len(view.result.selectorReadSlots) == 0 {
		return 0
	}
	return len(view.result.selectorReadSlots) - 1
}

// Get returns an external Read's root and exact suffix depth in O(1).
func (view ExactReads) Get(read keyspace.Term) (root keyspace.Term, depth int, ok bool) {
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
func (view ExactReads) PathCursor(read keyspace.Term) (ExactReadPath, bool) {
	rowIndex, row, ok := view.readRow(read)
	if !ok || !row.external || !validCellTerm(row.root) ||
		(row.depth == 0 && (row.parent != 0 || row.suffix != 0)) ||
		(row.depth != 0 && (row.parent == 0 || row.parent >= rowIndex || row.suffix == 0)) {
		return ExactReadPath{}, false
	}
	return ExactReadPath{
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
func (path ExactReadPath) Segment() (keyspace.Key, ExactReadPath, bool) {
	key, current, remaining, ok := path.result.nextSegment(path.current, path.root, path.remaining, false)
	if !ok {
		return 0, ExactReadPath{}, false
	}
	return key, ExactReadPath{
		result:    path.result,
		current:   current,
		root:      path.root,
		remaining: remaining,
	}, true
}

// Count reports the dense TypePublication denominator.
func (view TypePublications) Count() int {
	if view.result == nil || !view.result.available() || len(view.result.publicationSlots) == 0 {
		return 0
	}
	return len(view.result.publicationSlots) - 1
}

// Get returns one publication's root Cell, Assign owner Body, and exact
// dotted/name depth. Lexical visibility is intentionally not checked here.
func (view TypePublications) Get(publication keyspace.Term) (root, owner keyspace.Term, depth int, ok bool) {
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
func (view TypePublications) PathCursor(publication keyspace.Term) (PublicationPath, bool) {
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
func (view DirectCalls) Get(call keyspace.Term) (read keyspace.Term, form uint8, ok bool) {
	if view.result == nil || !view.result.available() || keyspace.TermFamily(call) != keyspace.FamilyCall {
		return 0, 0, false
	}
	ordinal := keyspace.TermOrdinal(call)
	if ordinal == 0 || uint64(ordinal) >= uint64(len(view.result.directCalls)) {
		return 0, 0, false
	}
	row := view.result.directCalls[ordinal]
	if row.read == 0 || (row.form != selectorCallPlain && row.form != selectorCallMethod) || !view.result.validRead(row.read) {
		return 0, 0, false
	}
	return row.read, row.form, true
}

func (view ExactReads) readRow(read keyspace.Term) (uint32, selectorRow, bool) {
	if view.result == nil || !view.result.available() || keyspace.TermFamily(read) != keyspace.FamilyRead {
		return 0, selectorRow{}, false
	}
	ordinal := keyspace.TermOrdinal(read)
	if ordinal == 0 || uint64(ordinal) >= uint64(len(view.result.selectorReadSlots)) {
		return 0, selectorRow{}, false
	}
	rowIndex := view.result.selectorReadSlots[ordinal]
	row, ok := view.result.row(rowIndex)
	if !ok || !view.result.validSelectorRow(rowIndex, row) {
		return 0, selectorRow{}, false
	}
	rowRead, ok := view.result.rowRead(rowIndex)
	if !ok || rowRead != read {
		return 0, selectorRow{}, false
	}
	return rowIndex, row, true
}

func (view TypePublications) publicationRow(publication keyspace.Term) (uint32, selectorRow, bool) {
	if view.result == nil || !view.result.available() || keyspace.TermFamily(publication) != keyspace.FamilyTypePublication {
		return 0, selectorRow{}, false
	}
	ordinal := keyspace.TermOrdinal(publication)
	if ordinal == 0 || uint64(ordinal) >= uint64(len(view.result.publicationSlots)) {
		return 0, selectorRow{}, false
	}
	rowIndex := view.result.publicationSlots[ordinal]
	row, ok := view.result.row(rowIndex)
	if !ok || !view.result.validPublicationRow(rowIndex, row) || uint64(ordinal) >= uint64(len(view.result.publicationOwners)) || !validBodyTerm(view.result.publicationOwners[ordinal]) {
		return 0, selectorRow{}, false
	}
	return rowIndex, row, true
}

func (r *Result) row(index uint32) (selectorRow, bool) {
	if r == nil || index == 0 || uint64(index) > uint64(len(r.selectorRows)) {
		return selectorRow{}, false
	}
	return r.selectorRows[index-1], true
}

func (r *Result) rowRead(index uint32) (keyspace.Term, bool) {
	if r == nil || index == 0 || uint64(index) > uint64(len(r.selectorRowReads)) {
		return 0, false
	}
	return r.selectorRowReads[index-1], true
}

func validCellTerm(term keyspace.Term) bool {
	return keyspace.TermFamily(term) == keyspace.FamilyCell && keyspace.TermOrdinal(term) != 0
}

func validBodyTerm(term keyspace.Term) bool {
	return keyspace.TermFamily(term) == keyspace.FamilyBody && keyspace.TermOrdinal(term) != 0
}

func (r *Result) validReadRow(index uint32, row selectorRow) bool {
	if r == nil || index == 0 || r.publicationStart == 0 || index >= r.publicationStart || row.plane != selectorPlaneRead || !validCellTerm(row.root) {
		return false
	}
	read, ok := r.rowRead(index)
	if !ok || keyspace.TermFamily(read) != keyspace.FamilyRead || keyspace.TermOrdinal(read) == 0 {
		return false
	}
	ordinal := keyspace.TermOrdinal(read)
	return uint64(ordinal) < uint64(len(r.selectorReadSlots)) && r.selectorReadSlots[ordinal] == index
}

func (r *Result) validSelectorRow(index uint32, row selectorRow) bool {
	return r.validReadRow(index, row) && row.external
}

func (r *Result) validPublicationRow(index uint32, row selectorRow) bool {
	if r == nil || index == 0 || r.publicationStart == 0 || index < r.publicationStart || row.plane != selectorPlanePublication || !row.typePath || !validCellTerm(row.root) || row.depth == 0 || row.parent == 0 || row.parent >= index || row.suffix == 0 {
		return false
	}
	publicationOrdinal := uint64(index-r.publicationStart) + 1
	if publicationOrdinal >= uint64(len(r.publicationSlots)) || r.publicationSlots[publicationOrdinal] != index || publicationOrdinal >= uint64(len(r.publicationOwners)) || !validBodyTerm(r.publicationOwners[publicationOrdinal]) {
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
	if ordinal == 0 || uint64(ordinal) >= uint64(len(r.selectorReadSlots)) {
		return false
	}
	rowIndex := r.selectorReadSlots[ordinal]
	row, ok := r.row(rowIndex)
	if !ok || !r.validSelectorRow(rowIndex, row) {
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
		if row.plane == selectorPlanePublication {
			if !r.validPublicationRow(current, row) {
				return 0, 0, 0, false
			}
		} else if row.plane != selectorPlaneRead || !r.validReadRow(current, row) {
			return 0, 0, 0, false
		}
	} else if !r.validSelectorRow(current, row) {
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
	} else if !r.validSelectorRow(row.parent, parent) {
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
