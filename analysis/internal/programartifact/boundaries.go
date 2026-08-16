package programartifact

import "github.com/wippyai/go-lua/program/keyspace"

// BoundaryKind is the small domain-neutral structural boundary vocabulary
// copied while the Program proof is live. Consumers bind these rows to their
// own mounted Link authority; no Term, Flow, or Program handle escapes.
type BoundaryKind uint8

const (
	BoundaryInvalid BoundaryKind = iota
	BoundaryCapture
	BoundaryStore
	BoundaryReturn
)

// BoundaryRow is one reusable Program boundary. Owner is the parent-issued
// Function/Body identity and Position is meaningful only for captures.
type BoundaryRow struct {
	kind     BoundaryKind
	id       keyspace.ContentID
	owner    keyspace.ContentID
	position uint32
	eligible bool
}

func (row BoundaryRow) Available() bool {
	return row.kind >= BoundaryCapture && row.kind <= BoundaryReturn && row.id.Available() && row.owner.Available() && (row.kind == BoundaryCapture || row.position == 0)
}
func (row BoundaryRow) Kind() BoundaryKind {
	if !row.Available() {
		return BoundaryInvalid
	}
	return row.kind
}
func (row BoundaryRow) ID() keyspace.ContentID {
	if !row.Available() {
		return keyspace.ContentID{}
	}
	return row.id
}
func (row BoundaryRow) OwnerID() keyspace.ContentID {
	if !row.Available() {
		return keyspace.ContentID{}
	}
	return row.owner
}
func (row BoundaryRow) Position() (int, bool) {
	return int(row.position), row.Available() && row.kind == BoundaryCapture
}
func (row BoundaryRow) Eligible() bool { return row.Available() && row.eligible }
