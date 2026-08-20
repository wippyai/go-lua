package programschema

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// InterfaceMember, TypeFunctionParameter, and RecordField carry metadata
// beside their typed child. Their key/text/optionality/readonly values remain
// in the same dense relation as the child they describe.
type StaticTypeNodeInterfaceMember struct {
	parent, child      identity.ContentID
	key                keyspace.Key
	text               string
	optional, readonly bool
	kind               uint8
	position           uint32
}
type StaticTypeNodeTypeFunctionParameter struct {
	parent, child identity.ContentID
	key           keyspace.Key
	text          string
	position      uint32
}
type StaticTypeNodeRecordField struct {
	parent, child      identity.ContentID
	key                keyspace.Key
	text               string
	optional, readonly bool
	position           uint32
}
type StaticTypeNodeReferenceSourceKey struct {
	parent   identity.ContentID
	key      keyspace.Key
	position uint32
}
type StaticTypeNodeReferenceCanonicalKey struct {
	parent   identity.ContentID
	key      keyspace.Key
	position uint32
}

func NewStaticTypeNodeInterfaceMember(parent, child identity.ContentID, key keyspace.Key, text string, optional, readonly bool, kind uint8, position uint32) (StaticTypeNodeInterfaceMember, bool) {
	row := StaticTypeNodeInterfaceMember{parent: parent, child: child, key: key, text: text, optional: optional, readonly: readonly, kind: kind, position: position}
	return row, parent.Available() && child.Available() && key != 0
}
func NewStaticTypeNodeTypeFunctionParameter(parent, child identity.ContentID, key keyspace.Key, text string, position uint32) (StaticTypeNodeTypeFunctionParameter, bool) {
	row := StaticTypeNodeTypeFunctionParameter{parent: parent, child: child, key: key, text: text, position: position}
	return row, parent.Available() && child.Available() && key != 0
}
func NewStaticTypeNodeRecordField(parent, child identity.ContentID, key keyspace.Key, text string, optional, readonly bool, position uint32) (StaticTypeNodeRecordField, bool) {
	row := StaticTypeNodeRecordField{parent: parent, child: child, key: key, text: text, optional: optional, readonly: readonly, position: position}
	return row, parent.Available() && child.Available() && key != 0
}
func NewStaticTypeNodeReferenceSourceKey(parent identity.ContentID, key keyspace.Key, position uint32) (StaticTypeNodeReferenceSourceKey, bool) {
	row := StaticTypeNodeReferenceSourceKey{parent: parent, key: key, position: position}
	return row, parent.Available() && key != 0
}
func NewStaticTypeNodeReferenceCanonicalKey(parent identity.ContentID, key keyspace.Key, position uint32) (StaticTypeNodeReferenceCanonicalKey, bool) {
	row := StaticTypeNodeReferenceCanonicalKey{parent: parent, key: key, position: position}
	return row, parent.Available() && key != 0
}

func (row StaticTypeNodeInterfaceMember) Available() bool {
	return row.parent.Available() && row.child.Available() && row.key != 0
}
func (row StaticTypeNodeTypeFunctionParameter) Available() bool {
	return row.parent.Available() && row.child.Available() && row.key != 0
}
func (row StaticTypeNodeRecordField) Available() bool {
	return row.parent.Available() && row.child.Available() && row.key != 0
}
func (row StaticTypeNodeReferenceSourceKey) Available() bool {
	return row.parent.Available() && row.key != 0
}
func (row StaticTypeNodeReferenceCanonicalKey) Available() bool {
	return row.parent.Available() && row.key != 0
}
