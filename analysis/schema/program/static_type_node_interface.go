package programschema

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// InterfaceExtend is the ordered parent edge for one inherited interface.
// Interface members live in their own metadata family below.
type StaticTypeNodeInterfaceExtend struct {
	parent, child identity.ContentID
	position      uint32
}

func NewStaticTypeNodeInterfaceExtend(parent, child identity.ContentID, position uint32) (StaticTypeNodeInterfaceExtend, bool) {
	row := StaticTypeNodeInterfaceExtend{parent, child, position}
	return row, newStaticNodeChild(parent, child, position)
}

func (row StaticTypeNodeInterfaceExtend) Available() bool {
	return newStaticNodeChild(row.parent, row.child, row.position)
}
func (row StaticTypeNodeInterfaceExtend) ParentID() identity.ContentID {
	if !childParent(row) {
		return identity.ContentID{}
	}
	return row.parent
}
func (row StaticTypeNodeInterfaceExtend) ChildID() identity.ContentID {
	if !childParent(row) {
		return identity.ContentID{}
	}
	return row.child
}
func (row StaticTypeNodeInterfaceExtend) Position() uint32 {
	if !childParent(row) {
		return 0
	}
	return row.position
}

func (row StaticTypeNodeInterfaceMember) ParentID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.parent
}
func (row StaticTypeNodeInterfaceMember) ChildID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.child
}
func (row StaticTypeNodeInterfaceMember) Key() keyspace.Key {
	if !row.Available() {
		return 0
	}
	return row.key
}
func (row StaticTypeNodeInterfaceMember) Text() string {
	if !row.Available() {
		return ""
	}
	return row.text
}
func (row StaticTypeNodeInterfaceMember) Optional() bool { return row.Available() && row.optional }
func (row StaticTypeNodeInterfaceMember) Readonly() bool { return row.Available() && row.readonly }
func (row StaticTypeNodeInterfaceMember) KindCode() uint8 {
	if !row.Available() {
		return 0
	}
	return row.kind
}
func (row StaticTypeNodeInterfaceMember) Position() uint32 {
	if !row.Available() {
		return 0
	}
	return row.position
}
