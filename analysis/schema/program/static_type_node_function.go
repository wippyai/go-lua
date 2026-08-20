package programschema

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// TypeFunctionTypeParameter and TypeFunctionReturn are ordered child
// families. Named function parameters are metadata rows in this file's
// companion definitions in static_type_node_metadata.go.
type StaticTypeNodeTypeFunctionTypeParameter struct {
	parent, child identity.ContentID
	position      uint32
}
type StaticTypeNodeTypeFunctionReturn struct {
	parent, child identity.ContentID
	position      uint32
}

func NewStaticTypeNodeTypeFunctionTypeParameter(parent, child identity.ContentID, position uint32) (StaticTypeNodeTypeFunctionTypeParameter, bool) {
	row := StaticTypeNodeTypeFunctionTypeParameter{parent, child, position}
	return row, newStaticNodeChild(parent, child, position)
}
func NewStaticTypeNodeTypeFunctionReturn(parent, child identity.ContentID, position uint32) (StaticTypeNodeTypeFunctionReturn, bool) {
	row := StaticTypeNodeTypeFunctionReturn{parent, child, position}
	return row, newStaticNodeChild(parent, child, position)
}

func (row StaticTypeNodeTypeFunctionTypeParameter) Available() bool {
	return newStaticNodeChild(row.parent, row.child, row.position)
}
func (row StaticTypeNodeTypeFunctionReturn) Available() bool {
	return newStaticNodeChild(row.parent, row.child, row.position)
}
func (row StaticTypeNodeTypeFunctionTypeParameter) ParentID() identity.ContentID {
	if !childParent(row) {
		return identity.ContentID{}
	}
	return row.parent
}
func (row StaticTypeNodeTypeFunctionTypeParameter) ChildID() identity.ContentID {
	if !childParent(row) {
		return identity.ContentID{}
	}
	return row.child
}
func (row StaticTypeNodeTypeFunctionTypeParameter) Position() uint32 {
	if !childParent(row) {
		return 0
	}
	return row.position
}
func (row StaticTypeNodeTypeFunctionReturn) ParentID() identity.ContentID {
	if !childParent(row) {
		return identity.ContentID{}
	}
	return row.parent
}
func (row StaticTypeNodeTypeFunctionReturn) ChildID() identity.ContentID {
	if !childParent(row) {
		return identity.ContentID{}
	}
	return row.child
}
func (row StaticTypeNodeTypeFunctionReturn) Position() uint32 {
	if !childParent(row) {
		return 0
	}
	return row.position
}

func (row StaticTypeNodeTypeFunctionParameter) ParentID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.parent
}
func (row StaticTypeNodeTypeFunctionParameter) ChildID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.child
}
func (row StaticTypeNodeTypeFunctionParameter) Key() keyspace.Key {
	if !row.Available() {
		return 0
	}
	return row.key
}
func (row StaticTypeNodeTypeFunctionParameter) Text() string {
	if !row.Available() {
		return ""
	}
	return row.text
}
func (row StaticTypeNodeTypeFunctionParameter) Position() uint32 {
	if !row.Available() {
		return 0
	}
	return row.position
}
