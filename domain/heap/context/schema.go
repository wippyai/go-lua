// Package context owns the contextual allocation-reference relation.
//
// Heap remains the authority for allocation coordinates and materialization
// roles.  This package only adds a sealed execution-context holder and an
// immutable lineage origin to those existing allocation references.  It does
// not add a second Heap coordinate system, a Placement coordinate system, or
// an identity input supplied by a caller.
package context

import (
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
	"github.com/wippyai/go-lua/domain/heap"
)

const schemaDomain = "domain/heap/context/schema/v1"

// Schema is the immutable authority for contextual allocation references. It
// is composed of exactly one Heap schema and one sealed execution-context
// directory.  The private owner pointer is intentional: two independently
// sealed, equal-content authorities are still different issuers for their
// owner-local Heap keys and contextual capabilities.
type Schema struct {
	owner *schemaOwner
}

type schemaOwner struct {
	heap      heap.Schema
	directory executioncontext.Directory
	id        identity.ContentID
}

// Seal composes one exact Heap authority with one exact Link-scoped
// execution-context directory.  The directory must belong to the Link that
// sealed the Heap; no context or transition is accepted as a raw identity.
func Seal(source heap.Schema, directory executioncontext.Directory) (Schema, bool) {
	if !source.Valid() || !directory.Available() || source.LinkContentID() != directory.LinkID() {
		return Schema{}, false
	}
	id, ok := contextualSchemaID(source, directory)
	if !ok {
		return Schema{}, false
	}
	owner := &schemaOwner{heap: source, directory: directory, id: id}
	schema := Schema{owner: owner}
	return schema, schema.Valid()
}

// contextualSchemaID is derived from the sealed typed rows in their
// Directory-owned canonical order.  The hash is an authority identity, not a
// caller-shaped reference identity and is never accepted as an operation
// input.
func contextualSchemaID(source heap.Schema, directory executioncontext.Directory) (identity.ContentID, bool) {
	heapID := source.ContentID()
	linkID := directory.LinkID()
	if !heapID.Available() || !linkID.Available() {
		return identity.ContentID{}, false
	}
	parts := make([][]byte, 0, 4+directory.ContextCount()*2+directory.RootCount()*3+directory.TransitionCount()*3)
	parts = append(parts, heapID[:], linkID[:])
	parts = append(parts, countPart(directory.ContextCount()), countPart(directory.RootCount()), countPart(directory.TransitionCount()))
	for index := 0; index < directory.ContextCount(); index++ {
		row, ok := directory.ContextAt(index)
		if !ok || !row.Available() {
			return identity.ContentID{}, false
		}
		id := row.ID()
		module := row.ModuleKey()
		actor := row.ActorID()
		representative := row.RepresentativeCacheInstanceID()
		if !id.Available() || !module.Available() || !actor.Available() || !representative.Available() {
			return identity.ContentID{}, false
		}
		parts = append(parts, []byte{'c'}, id[:], module[:], actor[:], representative[:])
	}
	for index := 0; index < directory.RootCount(); index++ {
		row, ok := directory.RootAt(index)
		if !ok || !row.Available() {
			return identity.ContentID{}, false
		}
		id := row.ID()
		root := row.AnalysisRootID()
		contextID := row.ContextID()
		if !id.Available() || !root.Available() || !contextID.Available() {
			return identity.ContentID{}, false
		}
		parts = append(parts, []byte{'r'}, id[:], root[:], contextID[:])
	}
	for index := 0; index < directory.TransitionCount(); index++ {
		row, ok := directory.TransitionAt(index)
		if !ok || !row.Available() {
			return identity.ContentID{}, false
		}
		id := row.ID()
		from := row.FromContextID()
		to := row.ToContextID()
		if !id.Available() || !from.Available() || !to.Available() {
			return identity.ContentID{}, false
		}
		parts = append(parts, []byte{'t'}, id[:], from[:], to[:])
	}
	return identity.DeriveContentID(schemaDomain, parts...)
}

func countPart(value int) []byte {
	var encoded [8]byte
	if value < 0 {
		return nil
	}
	binary.BigEndian.PutUint64(encoded[:], uint64(value))
	return encoded[:]
}

// Valid reports whether Schema is a complete contextual authority.
func (schema Schema) Valid() bool {
	return schema.owner != nil && schema.owner.heap.Valid() && schema.owner.directory.Available() &&
		schema.owner.id.Available() && schema.owner.heap.LinkContentID() == schema.owner.directory.LinkID()
}

// ContentID identifies this sealed contextual authority. It is derived only
// from the exact Heap schema and exact typed Directory rows.
func (schema Schema) ContentID() identity.ContentID {
	if !schema.Valid() {
		return identity.ContentID{}
	}
	return schema.owner.id
}

// Heap returns the exact allocation-only Heap authority projected by Schema.
func (schema Schema) Heap() heap.Schema {
	if !schema.Valid() {
		return heap.Schema{}
	}
	return schema.owner.heap
}

// Directory returns the exact sealed execution-context directory projected by
// Schema. Directory is immutable; the returned value has no mutator.
func (schema Schema) Directory() executioncontext.Directory {
	if !schema.Valid() {
		return executioncontext.Directory{}
	}
	return schema.owner.directory
}

// OwnsSchema reports exact issuer identity. Equal-content schemas sealed in
// separate calls are intentionally not interchangeable for local handles.
func (schema Schema) OwnsSchema(other Schema) bool {
	return schema.Valid() && other.Valid() && schema.owner == other.owner
}

// OwnsKey admits allocation roots from this exact Heap authority. Boot roots
// are not contextual allocation references.
func (schema Schema) OwnsKey(key heap.Key) bool {
	return schema.Valid() && key.Kind() == heap.RootAllocation && schema.owner.heap.OwnsKey(key)
}

// OwnsAllocation reports whether an immutable Allocation was issued by this
// exact contextual authority.
func (schema Schema) OwnsAllocation(allocation Allocation) bool {
	return schema.Valid() && allocation.valid() && allocation.owner == schema.owner
}

// FreshCount returns the owner-issued Target fresh-allocation denominator.
func (schema Schema) FreshCount() int {
	if !schema.Valid() {
		return 0
	}
	return schema.owner.heap.FreshCount()
}

// FreshAt projects one owner-issued fresh allocation key. The returned key is
// still owned by Heap; this package does not mint a second allocation handle.
func (schema Schema) FreshAt(index int) (heap.Key, bool) {
	if !schema.Valid() {
		return heap.Key{}, false
	}
	_, key, ok := schema.owner.heap.FreshAt(index)
	return key, ok && schema.OwnsKey(key)
}

func (schema Schema) freshKey(key heap.Key) bool {
	if !schema.OwnsKey(key) {
		return false
	}
	_, _, _, fresh := key.FreshResultID()
	return fresh
}

// OwnsContext reports whether an authenticated Context row belongs to this
// exact sealed Directory. Context identity is checked through the canonical
// row, including its Link/module/actor/representative fields.
func (schema Schema) OwnsContext(row executioncontext.Context) bool {
	if !schema.Valid() || !row.Available() || row.LinkID() != schema.owner.directory.LinkID() {
		return false
	}
	canonical, ok := schema.owner.directory.Context(row.ID())
	return ok && sameContext(canonical, row)
}

func sameContext(left, right executioncontext.Context) bool {
	return left.Available() && right.Available() && left.ID() == right.ID() && left.LinkID() == right.LinkID() &&
		left.ModuleKey() == right.ModuleKey() && left.ActorID() == right.ActorID() &&
		left.RepresentativeCacheInstanceID() == right.RepresentativeCacheInstanceID()
}

// OwnsTransition reports whether an authenticated typed Transition is one of
// this exact Directory's admitted edges. No endpoint pair is accepted from a
// caller in place of the typed row.
func (schema Schema) OwnsTransition(row executioncontext.Transition) bool {
	if !schema.Valid() || !row.Available() || row.LinkID() != schema.owner.directory.LinkID() {
		return false
	}
	canonical, ok := schema.owner.directory.Transition(row.FromContextID(), row.ToContextID())
	return ok && canonical.ID() == row.ID() && canonical.LinkID() == row.LinkID()
}

func (owner *schemaOwner) ownsTransition(row executioncontext.Transition) bool {
	return Schema{owner: owner}.OwnsTransition(row)
}

// ContextAt returns a typed context from this exact Directory.
func (schema Schema) ContextAt(index int) (executioncontext.Context, bool) {
	if !schema.Valid() {
		return executioncontext.Context{}, false
	}
	return schema.owner.directory.ContextAt(index)
}

// TransitionAt returns a typed transition from this exact Directory.
func (schema Schema) TransitionAt(index int) (executioncontext.Transition, bool) {
	if !schema.Valid() {
		return executioncontext.Transition{}, false
	}
	return schema.owner.directory.TransitionAt(index)
}
