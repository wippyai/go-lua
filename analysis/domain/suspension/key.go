// Package suspension owns one Link-scoped continuation-generation family. It
// has no module cache coordinate or cache-state relation.
package suspension

import (
	"github.com/wippyai/go-lua/program/keyspace"
	linkmodule "github.com/wippyai/go-lua/program/link/module"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	"github.com/wippyai/go-lua/program/target"
)

// Key is a Schema-issued coordinate for one typed Link continuation-generation
// source. Operation suspensions and module-init generations retain distinct
// source projections but one lifecycle owner.
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

func (key Key) OccurrenceID() (keyspace.ContentID, bool) {
	_, support, ok := key.support()
	if !ok {
		return keyspace.ContentID{}, false
	}
	return support.id, true
}

func (key Key) Operation() (linkproject.Application, target.Operation, int, bool) {
	_, support, ok := key.support()
	if !ok || support.generation.kind != generationSourceOperation {
		return linkproject.Application{}, 0, 0, false
	}
	return support.generation.application, support.generation.operation, int(support.generation.suspension), true
}

// ModuleInitGeneration projects the Link-native module-cache initialization
// generation source. It is false for ordinary operation suspension keys.
func (key Key) ModuleInitGeneration() (linkmodule.ModuleInitGeneration, bool) {
	_, support, ok := key.support()
	if !ok || support.generation.kind != generationSourceModuleInit {
		return linkmodule.ModuleInitGeneration{}, false
	}
	return support.generation.moduleInit, true
}
