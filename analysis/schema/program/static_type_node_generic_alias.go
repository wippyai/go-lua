package programschema

import "github.com/wippyai/go-lua/analysis/identity"

// GenericArgument and AliasParameter are separate families because a generic
// application argument is not an alias declaration parameter, even though
// both relations carry one ordered child edge.
type StaticTypeNodeGenericArgument struct {
	parent, child identity.ContentID
	position      uint32
}
type StaticTypeNodeAliasParameter struct {
	parent, child identity.ContentID
	position      uint32
}

func NewStaticTypeNodeGenericArgument(parent, child identity.ContentID, position uint32) (StaticTypeNodeGenericArgument, bool) {
	row := StaticTypeNodeGenericArgument{parent, child, position}
	return row, newStaticNodeChild(parent, child, position)
}
func NewStaticTypeNodeAliasParameter(parent, child identity.ContentID, position uint32) (StaticTypeNodeAliasParameter, bool) {
	row := StaticTypeNodeAliasParameter{parent, child, position}
	return row, newStaticNodeChild(parent, child, position)
}

func (row StaticTypeNodeGenericArgument) Available() bool {
	return newStaticNodeChild(row.parent, row.child, row.position)
}
func (row StaticTypeNodeAliasParameter) Available() bool {
	return newStaticNodeChild(row.parent, row.child, row.position)
}
func (row StaticTypeNodeGenericArgument) ParentID() identity.ContentID {
	if !childParent(row) {
		return identity.ContentID{}
	}
	return row.parent
}
func (row StaticTypeNodeGenericArgument) ChildID() identity.ContentID {
	if !childParent(row) {
		return identity.ContentID{}
	}
	return row.child
}
func (row StaticTypeNodeGenericArgument) Position() uint32 {
	if !childParent(row) {
		return 0
	}
	return row.position
}
func (row StaticTypeNodeAliasParameter) ParentID() identity.ContentID {
	if !childParent(row) {
		return identity.ContentID{}
	}
	return row.parent
}
func (row StaticTypeNodeAliasParameter) ChildID() identity.ContentID {
	if !childParent(row) {
		return identity.ContentID{}
	}
	return row.child
}
func (row StaticTypeNodeAliasParameter) Position() uint32 {
	if !childParent(row) {
		return 0
	}
	return row.position
}
