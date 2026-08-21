package staticnode

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func (row StaticTypeNodeRecordField) ParentID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.parent
}
func (row StaticTypeNodeRecordField) ChildID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.child
}
func (row StaticTypeNodeRecordField) Key() keyspace.Key {
	if !row.Available() {
		return 0
	}
	return row.key
}
func (row StaticTypeNodeRecordField) Text() string {
	if !row.Available() {
		return ""
	}
	return row.text
}
func (row StaticTypeNodeRecordField) Optional() bool { return row.Available() && row.optional }
func (row StaticTypeNodeRecordField) Readonly() bool { return row.Available() && row.readonly }
func (row StaticTypeNodeRecordField) Position() uint32 {
	if !row.Available() {
		return 0
	}
	return row.position
}
