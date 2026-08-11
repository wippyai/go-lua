// Package ownership owns only normative owner/borrow/move/share/send/lifetime
// duties. It does not decide reachability, retention, residence, or physical
// allocation; those remain facts owned by their respective Factors.
package ownership

import (
	"github.com/wippyai/go-lua/analysis/domain/heap"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	linkmodule "github.com/wippyai/go-lua/program/link/module"
)

// Role is the closed ownership-obligation vocabulary. It has no resource
// state, holder count, residence, or physical allocation meaning.
type Role uint8

const (
	RoleInvalid Role = iota
	Owner
	Borrow
	Move
	Share
	Send
	Lifetime
)

func (role Role) Valid() bool { return role >= Owner && role <= Lifetime }

// Schema holds one homogeneous Ownership Factor family. Its coordinate is
// exactly (Link AnalysisRoot, Ownership Origin, Role).
type Schema struct{ owner *schema }

type schema struct {
	source      *link.Link
	heap        heap.Schema
	linkID      keyspace.ContentID
	origins     []originRow
	originIndex map[keyspace.ContentID]uint32
	coordinates []coordinate
	roots       []heap.Key
	rootIndex   map[keyspace.ContentID]uint32
}

type coordinate struct {
	root   linkmodule.AnalysisRoot
	origin Origin
	role   Role
}

// Coordinate is one sealed (analysis-root, structural-origin, duty) key. It
// cannot be constructed outside its Schema.
type Coordinate struct {
	owner *schema
	index uint32
}

// Root is one sealed source allocation alternative. Recent/Summary remains in
// the homogeneous Ownership Value relation, never in Coordinate.
type Root struct {
	owner *schema
	index uint32
}

// NewSchema derives Ownership's closed source-duty range from typed Link
// topology and Heap's sole allocation-key authority. Link has no ownerful
// source union, and Ownership never recreates an allocation coordinate.
func NewSchema(source *link.Link, heapSchema heap.Schema) (Schema, bool) {
	if source == nil || !source.ContentID().Available() || !heapSchema.Valid() || heapSchema.Link() != source || heapSchema.LinkContentID() != source.ContentID() {
		return Schema{}, false
	}
	roots, rootIndex, ok := collectRoots(heapSchema)
	if !ok {
		return Schema{}, false
	}
	origins, originIndex, ok := buildOrigins(source, heapSchema)
	if !ok {
		return Schema{}, false
	}
	owner := &schema{source: source, heap: heapSchema, linkID: source.ContentID(), origins: origins, originIndex: originIndex, roots: roots, rootIndex: rootIndex}
	coordinates := make([]coordinate, 0, len(origins))
	seen := make(map[coordinate]struct{})
	for originIndex := range origins {
		origin := Origin{owner: owner, index: uint32(originIndex + 1)}
		roles, roleCount, ok := rolesFor(Schema{owner: owner}, source, origin)
		if !ok {
			return Schema{}, false
		}
		for rootIndex := 0; rootIndex < len(origins[originIndex].roots); rootIndex++ {
			root := origins[originIndex].roots[rootIndex]
			if _, valid := source.Module().Roots().ID(root); !valid {
				return Schema{}, false
			}
			for roleIndex := 0; roleIndex < roleCount; roleIndex++ {
				item := coordinate{root: root, origin: origin, role: roles[roleIndex]}
				if !item.role.Valid() {
					return Schema{}, false
				}
				if _, duplicate := seen[item]; duplicate {
					return Schema{}, false
				}
				seen[item] = struct{}{}
				coordinates = append(coordinates, item)
			}
		}
	}
	owner.coordinates = coordinates
	return Schema{owner: owner}, true
}

func (schema Schema) Valid() bool {
	return schema.owner != nil && schema.owner.source != nil && schema.owner.heap.Valid() && schema.owner.linkID.Available() &&
		schema.owner.heap.Link() == schema.owner.source && schema.owner.source.ContentID() == schema.owner.linkID && schema.owner.heap.LinkContentID() == schema.owner.linkID
}

func (schema Schema) LinkContentID() (keyspace.ContentID, bool) {
	if !schema.Valid() {
		return keyspace.ContentID{}, false
	}
	return schema.owner.linkID, true
}

// Link returns Ownership's exact immutable structural authority. The content
// ID remains the portable replay identity; this pointer is the live
// construction/rule fence that prevents a same-content independently sealed
// Link from supplying coordinates to this schema.
func (schema Schema) Link() *link.Link {
	if !schema.Valid() {
		return nil
	}
	return schema.owner.source
}

func (schema Schema) CoordinateCount() int {
	if !schema.Valid() {
		return 0
	}
	return len(schema.owner.coordinates)
}

func (schema Schema) CoordinateAt(index int) (Coordinate, bool) {
	if !schema.Valid() || index < 0 || index >= len(schema.owner.coordinates) {
		return Coordinate{}, false
	}
	return Coordinate{owner: schema.owner, index: uint32(index + 1)}, true
}

func (coordinate Coordinate) valid() bool {
	return coordinate.owner != nil && coordinate.index != 0 && uint64(coordinate.index) <= uint64(len(coordinate.owner.coordinates))
}

func (coordinate Coordinate) Valid() bool { return coordinate.valid() }

func (schema Schema) declaration(value Coordinate) (coordinate, bool) {
	if !schema.Valid() || !value.valid() || value.owner != schema.owner {
		return coordinate{}, false
	}
	return schema.owner.coordinates[value.index-1], true
}

func (schema Schema) AnalysisRoot(value Coordinate) (linkmodule.AnalysisRoot, bool) {
	item, ok := schema.declaration(value)
	return item.root, ok
}

func (schema Schema) Origin(value Coordinate) (Origin, bool) {
	item, ok := schema.declaration(value)
	return item.origin, ok
}

func (schema Schema) Role(value Coordinate) (Role, bool) {
	item, ok := schema.declaration(value)
	return item.role, ok
}

func (schema Schema) RootCount() int {
	if !schema.Valid() {
		return 0
	}
	return len(schema.owner.roots)
}

func (schema Schema) RootAt(index int) (Root, bool) {
	if !schema.Valid() || index < 0 || index >= len(schema.owner.roots) {
		return Root{}, false
	}
	return Root{owner: schema.owner, index: uint32(index + 1)}, true
}

func (root Root) valid() bool {
	return root.owner != nil && root.index != 0 && uint64(root.index) <= uint64(len(root.owner.roots))
}

func (root Root) Valid() bool { return root.valid() }

func (schema Schema) RootFor(raw heap.Key) (Root, bool) {
	if !schema.Valid() || !schema.owner.heap.OwnsKey(raw) || raw.Kind() != heap.RootAllocation {
		return Root{}, false
	}
	id, ok := raw.ContentID()
	if !ok {
		return Root{}, false
	}
	index, ok := schema.owner.rootIndex[id]
	if !ok {
		return Root{}, false
	}
	return Root{owner: schema.owner, index: index}, true
}

func (schema Schema) HeapKey(root Root) (heap.Key, bool) {
	if !schema.Valid() || !root.valid() || root.owner != schema.owner {
		return heap.Key{}, false
	}
	return schema.owner.roots[root.index-1], true
}

func collectRoots(source heap.Schema) ([]heap.Key, map[keyspace.ContentID]uint32, bool) {
	if !source.Valid() {
		return nil, nil, false
	}
	roots := make([]heap.Key, 0, source.KeyCount())
	indices := make(map[keyspace.ContentID]uint32)
	for index := 0; index < source.KeyCount(); index++ {
		root, ok := source.KeyAt(index)
		if !ok {
			return nil, nil, false
		}
		if root.Kind() != heap.RootAllocation {
			continue
		}
		id, ok := root.ContentID()
		if !ok || uint64(len(roots)) >= uint64(^uint32(0)) {
			return nil, nil, false
		}
		if _, duplicate := indices[id]; duplicate {
			return nil, nil, false
		}
		roots = append(roots, root)
		indices[id] = uint32(len(roots))
	}
	return roots, indices, true
}
