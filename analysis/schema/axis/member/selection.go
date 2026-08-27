package member

import "github.com/wippyai/go-lua/analysis/schema"

// Selection is one owner-issued operation that publishes the rows of a
// relation another rule then reads.
//
// Which rows a selection returns depends on the values the reads before it
// delivered, so the coordinate of a selected row does not exist until those
// cells are known. That is not a column vector anything could be paired
// against: it is an operation, and the rows it produces are published into a
// declared relation and stamped with a declared tag so the reading rule joins
// them like any other rows.
//
// The operation body is the owner's existing judgment. This row names it and
// says where its rows land; it carries no callback, no traversal and no
// engine handle.
type Selection struct {
	id schema.EntryID
	// Key is this member's own name within its axis catalog.
	Key schema.Key
	// Relation is the relation whose rows this operation publishes.
	Relation schema.Key
	// Tag is the projection over that relation this operation stamps each
	// published row with, so a reading rule correlates a returned row with the
	// source row it was selected for.
	Tag schema.Key
}

// ID returns the immutable identity issued to this selection by its owning
// axis. Construction rows intentionally return the unavailable zero value.
func (selection Selection) ID() schema.EntryID { return selection.id }

// Available reports whether the row names an operation, the relation it
// publishes into, and the tag it stamps.
func (selection Selection) Available() bool {
	return selection.Key.Available() && selection.Relation.Available() && selection.Tag.Available()
}

// SelectionRef names one selection member on an axis.
type SelectionRef struct {
	Axis   schema.EntryReference
	Member schema.Key
}

// Available reports whether the reference names a member of a declared axis.
func (reference SelectionRef) Available() bool {
	return axisReferenceAvailable(reference.Axis) && reference.Member.Available()
}

// Declared distinguishes an absent optional reference from a malformed one.
func (reference SelectionRef) Declared() bool {
	return reference.Axis.Declared() || reference.Member.Available()
}

// EntryReference returns the upward seal target of this reference.
func (reference SelectionRef) EntryReference() schema.EntryReference { return reference.Axis }

// AxisReference returns the axis that owns the named member.
func (reference SelectionRef) AxisReference() schema.EntryReference { return reference.Axis }

// ID returns the identity the owning axis issued for this selection member.
func (reference SelectionRef) ID() schema.EntryID {
	return IssueID(reference.Axis, reference.Member)
}
