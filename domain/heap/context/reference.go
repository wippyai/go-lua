package context

import (
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
	"github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/materialization"
)

// OriginKind tags the owner-issued lineage-origin variant. This first
// vertical has one variant only; future variants must be added as typed
// owner-issued constructors rather than caller-provided IDs or an opaque
// fallback.
type OriginKind uint8

const (
	OriginInvalid OriginKind = iota
	OriginExecutionContext
)

// Valid reports whether kind is an admitted lineage-origin tag.
func (kind OriginKind) Valid() bool { return kind == OriginExecutionContext }

// Origin is the immutable lineage-origin value for an Allocation. It is
// tagged so this context-origin vertical can grow without making a raw ID,
// Unknown value, or caller-shaped identity part of the relation.
type Origin struct {
	owner   *schemaOwner
	kind    OriginKind
	context executioncontext.Context
}

func (origin Origin) valid() bool {
	return origin.owner != nil && origin.kind.Valid() && sameContextInOwner(origin.owner, origin.context)
}

// Valid reports whether Origin is an authenticated immutable lineage value.
func (origin Origin) Valid() bool { return origin.valid() }

// Kind reports the tagged origin variant.
func (origin Origin) Kind() OriginKind {
	if !origin.valid() {
		return OriginInvalid
	}
	return origin.kind
}

// Context returns the typed execution context for an
// OriginExecutionContext origin.
func (origin Origin) Context() executioncontext.Context {
	if !origin.valid() || origin.kind != OriginExecutionContext {
		return executioncontext.Context{}
	}
	return origin.context
}

// Equal compares immutable origins under their exact contextual schema.
func (origin Origin) Equal(other Origin) bool {
	return origin.valid() && other.valid() && origin.owner == other.owner && origin.kind == other.kind && origin.context.ID() == other.context.ID()
}

// Allocation is the immutable identity of one contextual allocation lineage.
// Its canonical identity is exactly (Schema owner, tagged Origin, Heap key
// identity). A Share or Move creates another binding to this same Allocation;
// Copy creates a new Allocation.
type Allocation struct {
	owner  *schemaOwner
	origin Origin
	key    heap.Key
}

func (allocation Allocation) valid() bool {
	return allocation.owner != nil && allocation.owner.heap.Valid() && allocation.owner.heap.OwnsKey(allocation.key) &&
		allocation.key.Kind() == heap.RootAllocation && allocation.origin.valid() && allocation.origin.owner == allocation.owner
}

// Valid reports whether Allocation is an owner-issued immutable lineage
// identity.
func (allocation Allocation) Valid() bool { return allocation.valid() }

// Key returns the exact Heap allocation coordinate.
func (allocation Allocation) Key() heap.Key {
	if !allocation.valid() {
		return heap.Key{}
	}
	return allocation.key
}

// Origin returns the immutable tagged lineage origin.
func (allocation Allocation) Origin() Origin {
	if !allocation.valid() {
		return Origin{}
	}
	return allocation.origin
}

// Equal compares Allocation identity without considering its current holder
// or materialization role.
func (allocation Allocation) Equal(other Allocation) bool {
	if !allocation.valid() || !other.valid() || allocation.owner != other.owner || !allocation.origin.Equal(other.origin) {
		return false
	}
	leftID, leftOK := allocation.owner.heap.KeyID(allocation.key)
	rightID, rightOK := other.owner.heap.KeyID(other.key)
	return leftOK && rightOK && leftID == rightID
}

// Reference is the current holder binding of one Allocation. Its canonical
// identity is exactly (Schema owner, holder Context, Allocation, role). Event
// kind, transition route, and copy provenance are not part of Reference
// identity. A Share followed by a Move that arrives at the same holder has
// the same current Reference as a direct transfer.
type Reference struct {
	owner      *schemaOwner
	allocation Allocation
	holder     executioncontext.Context
	role       materialization.Role
}

func (reference Reference) valid() bool {
	if reference.owner == nil || !reference.owner.heap.Valid() || !reference.owner.directory.Available() ||
		!reference.allocation.valid() || reference.allocation.owner != reference.owner || !reference.role.Valid() ||
		!sameContextInOwner(reference.owner, reference.holder) {
		return false
	}
	_, referenceOK := reference.owner.heap.Reference(reference.allocation.key, reference.role)
	return referenceOK
}

func sameContextInOwner(owner *schemaOwner, row executioncontext.Context) bool {
	if owner == nil || !row.Available() || row.LinkID() != owner.directory.LinkID() {
		return false
	}
	canonical, ok := owner.directory.Context(row.ID())
	return ok && sameContext(canonical, row)
}

// Valid reports whether Reference is an authenticated current binding.
func (reference Reference) Valid() bool { return reference.valid() }

// Allocation returns the immutable allocation identity carried by this
// binding. Holder and Role remain properties of Reference.
func (reference Reference) Allocation() Allocation {
	if !reference.valid() {
		return Allocation{}
	}
	return reference.allocation
}

// Key returns the exact allocation coordinate retained by this binding.
func (reference Reference) Key() heap.Key {
	if !reference.valid() {
		return heap.Key{}
	}
	return reference.allocation.key
}

// Holder returns the current typed execution context.
func (reference Reference) Holder() executioncontext.Context {
	if !reference.valid() {
		return executioncontext.Context{}
	}
	return reference.holder
}

// Origin returns the immutable tagged lineage origin through Allocation.
func (reference Reference) Origin() Origin {
	if !reference.valid() {
		return Origin{}
	}
	return reference.allocation.origin
}

// Role returns the exact Heap materialization role carried by this binding.
func (reference Reference) Role() materialization.Role {
	if !reference.valid() {
		return materialization.Invalid
	}
	return reference.role
}

// HeapReference projects the allocation-only Heap Reference while leaving
// holder and lineage ownership in this package.
func (reference Reference) HeapReference() (heap.Reference, bool) {
	if !reference.valid() {
		return heap.Reference{}, false
	}
	return reference.owner.heap.Reference(reference.allocation.key, reference.role)
}

// FencedTo reports whether this binding belongs to the exact Schema issuer.
func (reference Reference) FencedTo(schema Schema) bool {
	return reference.valid() && schema.Valid() && reference.owner == schema.owner
}

// Equal is canonical current-reference equality. It excludes operation event
// kind and history, but includes Allocation identity, holder, and role.
func (reference Reference) Equal(other Reference) bool {
	return reference.valid() && other.valid() && reference.owner == other.owner && reference.holder.ID() == other.holder.ID() &&
		reference.role == other.role && reference.allocation.Equal(other.allocation)
}
