package flow

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/internal/framing"
)

// StorageAssignmentID returns the canonical owner-neutral identity of one
// executable authored assignment. The complete equation is Flow-owned:
// authored Assign and Values rows, executable admission, Body boundary,
// assignment path, and destination width are all sealed in this view.
func (view View) StorageAssignmentID(term keyspace.Term) (identity.ContentID, bool) {
	if !view.available() || keyspace.TermFamily(term) != keyspace.FamilyAssign {
		return identity.ContentID{}, false
	}
	assigns := view.Authored().Storage().Assigns()
	owner, values, related := assigns.Get(term)
	width, widthOK := assigns.WriteCount(term)
	body, bodyOK := view.FunctionBoundaries().ForBody(owner)
	bodyPath, bodyPathOK := view.BodyPath(owner)
	assignmentPath, assignmentPathOK := view.SemanticTermPath(term)
	if !related || !widthOK || width <= 0 || !view.Executable().Contains(term) {
		return identity.ContentID{}, false
	}
	if _, _, valuesOK := view.Authored().Values().Get(values); !valuesOK {
		return identity.ContentID{}, false
	}
	if !bodyOK || !body.Available() || !bodyPathOK || !bodyPath.Available() || !assignmentPathOK || !assignmentPath.Available() {
		return identity.ContentID{}, false
	}
	id := flowSemanticID("program/transformer/storage-assignment", func(writer *framing.Writer) bool {
		return writer.Bytes(bodyPath[:]) == nil && writer.Bytes(assignmentPath[:]) == nil && writer.Count(uint64(width)) == nil
	})
	return id, id.Available()
}
