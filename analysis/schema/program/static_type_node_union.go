package programschema

import "github.com/wippyai/go-lua/analysis/identity"

// UnionMember and IntersectionMember are separate families even though both
// carry one ordered child edge. Their parent spans therefore cannot be
// accidentally joined through a shared child vocabulary.
type StaticTypeNodeUnionMember struct {
	parent, child identity.ContentID
	position      uint32
}
type StaticTypeNodeIntersectionMember struct {
	parent, child identity.ContentID
	position      uint32
}

func NewStaticTypeNodeUnionMember(parent, child identity.ContentID, position uint32) (StaticTypeNodeUnionMember, bool) {
	row := StaticTypeNodeUnionMember{parent, child, position}
	return row, newStaticNodeChild(parent, child, position)
}
func NewStaticTypeNodeIntersectionMember(parent, child identity.ContentID, position uint32) (StaticTypeNodeIntersectionMember, bool) {
	row := StaticTypeNodeIntersectionMember{parent, child, position}
	return row, newStaticNodeChild(parent, child, position)
}

func (row StaticTypeNodeUnionMember) Available() bool {
	return newStaticNodeChild(row.parent, row.child, row.position)
}
func (row StaticTypeNodeIntersectionMember) Available() bool {
	return newStaticNodeChild(row.parent, row.child, row.position)
}
func (row StaticTypeNodeUnionMember) ParentID() identity.ContentID {
	if !childParent(row) {
		return identity.ContentID{}
	}
	return row.parent
}
func (row StaticTypeNodeUnionMember) ChildID() identity.ContentID {
	if !childParent(row) {
		return identity.ContentID{}
	}
	return row.child
}
func (row StaticTypeNodeUnionMember) Position() uint32 {
	if !childParent(row) {
		return 0
	}
	return row.position
}
func (row StaticTypeNodeIntersectionMember) ParentID() identity.ContentID {
	if !childParent(row) {
		return identity.ContentID{}
	}
	return row.parent
}
func (row StaticTypeNodeIntersectionMember) ChildID() identity.ContentID {
	if !childParent(row) {
		return identity.ContentID{}
	}
	return row.child
}
func (row StaticTypeNodeIntersectionMember) Position() uint32 {
	if !childParent(row) {
		return 0
	}
	return row.position
}
