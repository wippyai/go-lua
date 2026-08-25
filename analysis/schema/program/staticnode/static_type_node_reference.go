package staticnode

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func (row StaticTypeNodeReferenceSourceKey) ParentID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.parent
}
func (row StaticTypeNodeReferenceSourceKey) Key() keyspace.Key {
	if !row.Available() {
		return 0
	}
	return row.key
}
func (row StaticTypeNodeReferenceSourceKey) Position() uint32 {
	if !row.Available() {
		return 0
	}
	return row.position
}
func (row StaticTypeNodeReferenceCanonicalKey) ParentID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.parent
}

// Text is the exact spelling of this canonical path segment.
func (row StaticTypeNodeReferenceCanonicalKey) Text() string {
	return row.text
}

func (row StaticTypeNodeReferenceCanonicalKey) Key() keyspace.Key {
	if !row.Available() {
		return 0
	}
	return row.key
}
func (row StaticTypeNodeReferenceCanonicalKey) Position() uint32 {
	if !row.Available() {
		return 0
	}
	return row.position
}
