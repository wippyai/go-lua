// Package escape owns only the finite may-reachability relation from a
// sealed allocation alternative to an exact structural boundary.  It owns no
// duty, residence, continuation generation, or heap-shape conclusion.
package escape

import (
	"github.com/wippyai/go-lua/analysis/domain/heap"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	linkhost "github.com/wippyai/go-lua/program/link/host"
	linkmodule "github.com/wippyai/go-lua/program/link/module"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	"github.com/wippyai/go-lua/program/target"
)

// BoundaryKind names the sealed Link relation at which a root may become
// observable.  It is structural vocabulary, not an execution judgment.
type BoundaryKind uint8

const (
	BoundaryInvalid BoundaryKind = iota
	BoundaryCapture
	BoundaryStore
	BoundaryReturn
	BoundaryGlobal
	BoundaryCall
	BoundaryInput
	BoundaryOutcome
	BoundaryCallback
	BoundaryModule
	BoundaryTransfer
	BoundaryResume
	BoundarySuspension
)

func (kind BoundaryKind) Valid() bool {
	return kind >= BoundaryCapture && kind <= BoundarySuspension
}

// Schema is the one immutable Escape Factor family for one Link.  Its
// coordinates are the existing Link boundary rows.  Allocation alternatives
// are deliberately not coordinates: they are the finite relation carried by
// Value, so no allocation×boundary product can be admitted as a key.
type Schema struct{ owner *schema }

type schema struct {
	source      *link.Link
	heap        heap.Schema
	linkID      keyspace.ContentID
	coordinates []boundary
	transfers   map[target.TransferID]uint32
	roots       []heap.Key
	rootIndex   map[keyspace.ContentID]uint32
}

type boundary struct {
	kind       BoundaryKind
	program    programBoundary
	global     linkhost.GlobalBinding
	call       linkproject.Application
	input      targetPort
	outcome    targetPort
	callback   target.CallbackID
	module     linkmodule.ModuleCacheEntry
	transfer   target.TransferID
	resume     target.ResumeID
	suspension targetPort
}

// targetPort is one static Contract descriptor.  Target owns the operation
// and its local ordinal; an Application is deliberately not part of Escape's
// coordinate space.  Dynamic application data enters only through a named
// activation port when a Rule is instantiated.
type targetPort struct {
	operation target.Operation
	port      uint32
}

// Coordinate is an exact schema-issued boundary capability.  Its private
// dense index cannot be combined with a caller-supplied allocation or role.
type Coordinate struct {
	owner *schema
	index uint32
}

// Root is an exact schema-issued allocation alternative source.  A Value
// refers to this finite support by index and records whether the alternative
// is recent or materialized-summary; it never stores a Link allocation handle.
type Root struct {
	owner *schema
	index uint32
}

// NewSchema cold-seals the complete existing Link boundary range and Heap
// allocation-key range. No supplemental runtime admission can create a root.
func NewSchema(source *link.Link, heapSchema heap.Schema) (Schema, bool) {
	// ContentID is the cold/replay identity; it is not a live ownership
	// capability.  The Heap schema must have been sealed from this exact Link
	// before its allocation keys can enter Escape's source universe.
	if source == nil || !source.ContentID().Available() || !heapSchema.Valid() || heapSchema.Link() != source || heapSchema.LinkContentID() != source.ContentID() {
		return Schema{}, false
	}
	roots, rootIndex, ok := collectRoots(heapSchema)
	if !ok {
		return Schema{}, false
	}
	coordinates, ok := collectBoundaries(source)
	if !ok {
		return Schema{}, false
	}
	transfers := make(map[target.TransferID]uint32)
	for index, boundary := range coordinates {
		if boundary.kind != BoundaryTransfer {
			continue
		}
		if _, duplicate := transfers[boundary.transfer]; duplicate {
			return Schema{}, false
		}
		transfers[boundary.transfer] = uint32(index + 1)
	}
	return Schema{owner: &schema{
		source: source, heap: heapSchema, linkID: source.ContentID(), coordinates: coordinates,
		transfers: transfers, roots: roots, rootIndex: rootIndex,
	}}, true
}

func (schema Schema) Valid() bool {
	return schema.owner != nil && schema.owner.source != nil &&
		schema.owner.heap.Valid() && schema.owner.heap.Link() == schema.owner.source && schema.owner.linkID.Available() && schema.owner.source.ContentID() == schema.owner.linkID &&
		schema.owner.heap.LinkContentID() == schema.owner.linkID
}

func (schema Schema) LinkContentID() (keyspace.ContentID, bool) {
	if !schema.Valid() {
		return keyspace.ContentID{}, false
	}
	return schema.owner.linkID, true
}

// Link returns Escape's one sealed structural authority.  It is a cold
// declaration/query projection only: recurrent Escape Values retain neither
// this pointer nor a Link identity.  Cross-factor Rules use pointer identity
// here to reject independently resealed same-content Links before binding.
func (schema Schema) Link() *link.Link {
	if !schema.Valid() {
		return nil
	}
	return schema.owner.source
}

// CoordinateCount and CoordinateAt are the complete, deterministic K range.
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

func (schema Schema) boundary(coordinate Coordinate) (boundary, bool) {
	if !schema.Valid() || !coordinate.valid() || coordinate.owner != schema.owner {
		return boundary{}, false
	}
	return schema.owner.coordinates[coordinate.index-1], true
}

func (schema Schema) BoundaryKind(coordinate Coordinate) (BoundaryKind, bool) {
	boundary, ok := schema.boundary(coordinate)
	return boundary.kind, ok
}

func (schema Schema) Global(coordinate Coordinate) (linkhost.GlobalBinding, bool) {
	boundary, ok := schema.boundary(coordinate)
	return boundary.global, ok && boundary.kind == BoundaryGlobal
}
func (schema Schema) Call(coordinate Coordinate) (linkproject.Application, bool) {
	boundary, ok := schema.boundary(coordinate)
	return boundary.call, ok && boundary.kind == BoundaryCall
}
func (schema Schema) Module(coordinate Coordinate) (linkmodule.ModuleCacheEntry, bool) {
	boundary, ok := schema.boundary(coordinate)
	return boundary.module, ok && boundary.kind == BoundaryModule
}

// CoordinateForTransfer returns Escape's private coordinate for one sealed
// Contract transfer declaration.  The owning operation is recovered from the
// Contract; no Application or application-specific row is accepted.
func (schema Schema) CoordinateForTransfer(transfer target.TransferID) (Coordinate, bool) {
	if !schema.Valid() {
		return Coordinate{}, false
	}
	contract, ok := schema.owner.source.Boundary().Target()
	if !ok || contract == nil {
		return Coordinate{}, false
	}
	if _, ownerOK := contract.TransferOwner(transfer); !ownerOK {
		return Coordinate{}, false
	}
	index := schema.owner.transfers[transfer]
	if index == 0 || uint64(index) > uint64(len(schema.owner.coordinates)) {
		return Coordinate{}, false
	}
	coordinate := Coordinate{owner: schema.owner, index: index}
	return coordinate, coordinate.valid()
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
	appendRoot := func(root heap.Key) bool {
		if !source.OwnsKey(root) || root.Kind() != heap.RootAllocation {
			return false
		}
		id, ok := root.ContentID()
		if !ok {
			return false
		}
		if _, duplicate := indices[id]; duplicate || len(roots) == int(^uint32(0)) {
			return false
		}
		roots = append(roots, root)
		indices[id] = uint32(len(roots))
		return true
	}
	for index := 0; index < source.KeyCount(); index++ {
		root, ok := source.KeyAt(index)
		if !ok || root.Kind() != heap.RootAllocation {
			if !ok {
				return nil, nil, false
			}
			continue
		}
		if !appendRoot(root) {
			return nil, nil, false
		}
	}
	return roots, indices, true
}

func collectBoundaries(source *link.Link) ([]boundary, bool) {
	coordinates := make([]boundary, 0)
	for _, kind := range [...]BoundaryKind{BoundaryCapture, BoundaryStore, BoundaryReturn} {
		items, ok := collectProgramBoundaries(source, kind)
		if !ok {
			return nil, false
		}
		coordinates = append(coordinates, items...)
	}
	for index := 0; index < source.Host().Globals().Count(); index++ {
		global, ok := source.Host().Globals().At(index)
		if !ok {
			return nil, false
		}
		coordinates = append(coordinates, boundary{kind: BoundaryGlobal, global: global})
	}
	for index := 0; index < source.Project().Applications().Count(); index++ {
		application, ok := source.Project().Applications().At(index)
		if !ok {
			return nil, false
		}
		if _, _, ok := source.Project().Applications().Call(application); ok {
			coordinates = append(coordinates, boundary{kind: BoundaryCall, call: application})
		}
	}
	for index := 0; index < source.Module().Cache().EntryCount(); index++ {
		entry, ok := source.Module().Cache().EntryAt(index)
		if !ok {
			return nil, false
		}
		coordinates = append(coordinates, boundary{kind: BoundaryModule, module: entry})
	}
	contract, ok := source.Boundary().Target()
	if !ok || contract == nil {
		return nil, false
	}
	for opIndex := 0; opIndex < contract.OperationCount(); opIndex++ {
		operation, ok := contract.OperationAt(opIndex)
		if !ok {
			return nil, false
		}
		for port := 0; port < contract.ValueFormalCount(operation); port++ {
			coordinates = append(coordinates, boundary{kind: BoundaryInput, input: targetPort{operation: operation, port: uint32(port)}})
		}
		for port := 0; port < contract.OutcomeCount(operation); port++ {
			coordinates = append(coordinates, boundary{kind: BoundaryOutcome, outcome: targetPort{operation: operation, port: uint32(port)}})
		}
		for port := 0; port < contract.CallbackCount(operation); port++ {
			id, found := contract.CallbackAt(operation, port)
			if !found {
				return nil, false
			}
			coordinates = append(coordinates, boundary{kind: BoundaryCallback, callback: id})
		}
		for port := 0; port < contract.TransferCount(operation); port++ {
			id, found := contract.TransferIDAt(operation, port)
			if !found {
				return nil, false
			}
			coordinates = append(coordinates, boundary{kind: BoundaryTransfer, transfer: id})
		}
		for port := 0; port < contract.ResumeCount(operation); port++ {
			id, found := contract.ResumeIDAt(operation, port)
			if !found {
				return nil, false
			}
			coordinates = append(coordinates, boundary{kind: BoundaryResume, resume: id})
		}
		for port := 0; port < contract.SuspensionCount(operation); port++ {
			coordinates = append(coordinates, boundary{kind: BoundarySuspension, suspension: targetPort{operation: operation, port: uint32(port)}})
		}
	}
	return coordinates, true
}
