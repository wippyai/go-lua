// Package module owns one Link-scoped abstract cache family. It owns no
// loading policy, Heap cell, or Suspension lifecycle.
package module

import (
	"github.com/wippyai/go-lua/program/keyspace"
	linkmodule "github.com/wippyai/go-lua/program/link/module"
)

// Key is a Schema-issued coordinate in the one Module Factor family. The
// opaque Link coordinate and its stable content identity remain cold schema
// authority; no Actor/Shard/cache tuple can be assembled here.
type Key struct {
	owner *schema
	index uint32
}

func (key Key) support() (*schema, *keySupport, bool) {
	if key.owner == nil || uint64(key.index) >= uint64(len(key.owner.keys)) {
		return nil, nil, false
	}
	return key.owner, &key.owner.keys[key.index], true
}

func (key Key) Valid() bool {
	_, _, ok := key.support()
	return ok
}

func (key Key) LinkContentID() (keyspace.ContentID, bool) {
	owner, _, ok := key.support()
	if !ok {
		return keyspace.ContentID{}, false
	}
	return owner.linkID, true
}

func (key Key) CoordinateID() (keyspace.ContentID, bool) {
	_, support, ok := key.support()
	if !ok {
		return keyspace.ContentID{}, false
	}
	return support.id, true
}

func (key Key) Coordinate() (linkmodule.ModuleCoordinate, bool) {
	_, support, ok := key.support()
	if !ok {
		return linkmodule.ModuleCoordinate{}, false
	}
	return support.coordinate, true
}
