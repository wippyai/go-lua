package staticnode

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

// StaticTypeNodeReferenceCanonicalKey is one segment of a reference's
// canonical path. It carries the segment's exact spelling as well as its key:
// the canonical path names a declaration outside this Program, so a consumer
// that has only the Program's rows must be able to read it by name.
type StaticTypeNodeReferenceCanonicalKey struct {
	parent   identity.ContentID
	key      keyspace.Key
	text     string
	position uint32
}

// namedStaticNodeMetadata holds for the metadata families whose row is
// addressed by its name. An interface member and a record field are reached
// through the key, so a row carrying none reaches nothing and is refused.
func namedStaticNodeMetadata(parent, child identity.ContentID, key keyspace.Key) bool {
	return parent.Available() && child.Available() && key != 0
}

// positionalStaticNodeMetadata holds for the metadata family whose row is
// addressed by its position. A type-function parameter is such a row:
// `(number) -> number` declares a parameter type and no parameter name, so the
// key is optional there and zero states the absent name rather than a value
// the producer failed to supply.
func positionalStaticNodeMetadata(parent, child identity.ContentID) bool {
	return parent.Available() && child.Available()
}

func NewStaticTypeNodeInterfaceMember(parent, child identity.ContentID, key keyspace.Key, text string, optional, readonly bool, kind uint8, position uint32) (StaticTypeNodeInterfaceMember, bool) {
	row := StaticTypeNodeInterfaceMember{parent: parent, child: child, key: key, text: text, optional: optional, readonly: readonly, kind: kind, position: position}
	return row, namedStaticNodeMetadata(parent, child, key)
}
func NewStaticTypeNodeTypeFunctionParameter(parent, child identity.ContentID, key keyspace.Key, text string, position uint32) (StaticTypeNodeTypeFunctionParameter, bool) {
	row := StaticTypeNodeTypeFunctionParameter{parent: parent, child: child, key: key, text: text, position: position}
	return row, positionalStaticNodeMetadata(parent, child)
}
func NewStaticTypeNodeRecordField(parent, child identity.ContentID, key keyspace.Key, text string, optional, readonly bool, position uint32) (StaticTypeNodeRecordField, bool) {
	row := StaticTypeNodeRecordField{parent: parent, child: child, key: key, text: text, optional: optional, readonly: readonly, position: position}
	return row, namedStaticNodeMetadata(parent, child, key)
}
func NewStaticTypeNodeReferenceSourceKey(parent identity.ContentID, key keyspace.Key, position uint32) (StaticTypeNodeReferenceSourceKey, bool) {
	row := StaticTypeNodeReferenceSourceKey{parent: parent, key: key, position: position}
	return row, parent.Available() && key != 0
}
func NewStaticTypeNodeReferenceCanonicalKey(parent identity.ContentID, key keyspace.Key, text string, position uint32) (StaticTypeNodeReferenceCanonicalKey, bool) {
	row := StaticTypeNodeReferenceCanonicalKey{parent: parent, key: key, text: text, position: position}
	return row, parent.Available() && key != 0 && text != ""
}

func (row StaticTypeNodeInterfaceMember) Available() bool {
	return namedStaticNodeMetadata(row.parent, row.child, row.key)
}
func (row StaticTypeNodeTypeFunctionParameter) Available() bool {
	return positionalStaticNodeMetadata(row.parent, row.child)
}
func (row StaticTypeNodeRecordField) Available() bool {
	return namedStaticNodeMetadata(row.parent, row.child, row.key)
}
func (row StaticTypeNodeReferenceSourceKey) Available() bool {
	return row.parent.Available() && row.key != 0
}
func (row StaticTypeNodeReferenceCanonicalKey) Available() bool {
	return row.parent.Available() && row.key != 0 && row.text != ""
}
