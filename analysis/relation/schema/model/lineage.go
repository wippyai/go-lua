package model

import "github.com/wippyai/go-lua/analysis/identity"

// LineageRef names an immutable support/proof sidecar.  It is deliberately
// separate from semantic value, presence, and scope; changing lineage cannot
// change those components or a logical cell's identity.
type LineageRef struct {
	issued
}

// IssueLineageRef adopts a non-zero owner-issued lineage token.
func IssueLineageRef(owner OwnerID, content identity.ContentID) (LineageRef, bool) {
	value, ok := issue(owner, content)
	if !ok {
		return LineageRef{}, false
	}
	return LineageRef{issued: value}, true
}

// Available reports whether ref names a lineage sidecar.
func (ref LineageRef) Available() bool {
	return ref.issued.Available()
}

// Owner returns the authority that issued ref.
func (ref LineageRef) Owner() OwnerID { return ref.issued.owner }

// Content returns the owner-issued lineage token.
func (ref LineageRef) Content() identity.ContentID { return ref.issued.content }
