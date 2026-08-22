// Package contextfiber owns the engine's dense execution-context/point
// coordinate plane.  It is deliberately a consumer of the sealed scalar
// executioncontext.Directory: it does not admit contexts, invent a default
// context, or retain any construction-side root information.
package contextfiber

import (
	"math"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
)

const directoryOwnerDomain = "analysis/engine/internal/contextfiber/directory/v1"

// ContextOrdinal is the zero-based position of a Context in Directory's
// canonical ContextAt sequence.
//
// The type is unsigned because negative coordinates are never members of the
// fiber plane.  An unavailable value is represented by the bool returned from
// the lookup/coordinate methods rather than by reserving an ordinal: ordinal
// zero is a real coordinate.
type ContextOrdinal uint64

// PointOrdinal is the zero-based position of a point in every execution
// context's fixed point shape.
type PointOrdinal uint64

// FiberOrdinal is the checked row number in the flattened context-major
// plane.  It is zero-based, so (ContextOrdinal(0), PointOrdinal(0)) maps to
// FiberOrdinal(0).
type FiberOrdinal uint64

// Index is an immutable, bounded mapping from a sealed execution-context
// directory and one positive point shape to a dense context-major fiber plane.
// The plane is only an address shape: it does not assert that every
// ContextOrdinal×PointOrdinal pair is executable. Runtime eligibility remains
// a separate mounted-point/context admission check.
//
// owner is a canonical digest of the Link and the directory's ordered Context
// identities.  generation is an exact engine revision fence; pointCount is
// part of that fence because the same ContextOrdinal has a different flattened
// meaning under a different point shape.  No caller-provided owner token can
// be installed: New derives owner only after validating the sealed Directory.
//
// available is the shape verdict New reached over the sealed Directory.  The
// value is immutable, so the verdict is decided exactly once where the shape
// can still be malformed; the zero Index carries the false verdict.
type Index struct {
	owner      identity.ContentID
	generation identity.Generation
	contexts   []identity.ContentID
	points     uint64
	fibers     uint64
	available  bool
}

// New seals one engine-local index over directory and the fixed positive point
// count.  The supplied generation is the exact revision that owns this shape;
// an unavailable generation is refused rather than treated as an initial or
// default revision.
//
// The product contextCount*pointCount is checked before publication.  No
// backing slice is allocated for the product, so a large but representable
// shape remains bounded by the directory's context inventory.
func New(directory executioncontext.Directory, pointCount int, generation identity.Generation) (Index, bool) {
	if !directory.Available() || pointCount <= 0 || !generation.Available() {
		return Index{}, false
	}
	contextCount := directory.ContextCount()
	if contextCount <= 0 {
		return Index{}, false
	}

	contexts := make([]identity.ContentID, contextCount)
	for ordinal := range contexts {
		row, ok := directory.ContextAt(ordinal)
		if !ok || !row.Available() || row.LinkID() != directory.LinkID() || !row.ID().Available() {
			return Index{}, false
		}
		contexts[ordinal] = row.ID()
		if ordinal > 0 && contexts[ordinal-1] == contexts[ordinal] {
			// Directory.Seal already rejects duplicates.  Retaining this check
			// keeps the consumer closed if a malformed value ever crosses the
			// schema boundary.
			return Index{}, false
		}
	}

	product, ok := checkedProduct(uint64(contextCount), uint64(pointCount))
	if !ok {
		return Index{}, false
	}
	owner, ok := directoryOwner(directory, contexts)
	if !ok {
		return Index{}, false
	}
	index := Index{
		owner:      owner,
		generation: generation,
		contexts:   contexts,
		points:     uint64(pointCount),
		fibers:     product,
	}
	index.available = index.completeShape()
	return index, index.available
}

// Available reports whether this index is a complete published shape.  The
// verdict is sealed by New; the zero Index is unavailable and never names even
// the first fiber.
func (index Index) Available() bool { return index.available }

func (index Index) completeShape() bool {
	if !index.owner.Available() || !index.generation.Available() || len(index.contexts) == 0 || index.points == 0 || index.fibers == 0 {
		return false
	}
	product, ok := checkedProduct(uint64(len(index.contexts)), index.points)
	return ok && product == index.fibers
}

// Generation returns the exact engine revision fenced into index.
func (index Index) Generation() identity.Generation {
	if !index.Available() {
		return 0
	}
	return index.generation
}

// ContextCount reports the number of canonical contexts in this plane.
func (index Index) ContextCount() int {
	if !index.Available() {
		return 0
	}
	return len(index.contexts)
}

// PointCount reports the fixed point width shared by every context in this
// plane.
func (index Index) PointCount() int {
	if !index.Available() || index.points > uint64(math.MaxInt) {
		return 0
	}
	return int(index.points)
}

// FiberCount reports the total flattened cardinality without narrowing it to
// int.  A zero result means the index is unavailable; zero is never a valid
// count for an available index because both dimensions are positive.
func (index Index) FiberCount() FiberOrdinal {
	if !index.Available() {
		return 0
	}
	return FiberOrdinal(index.fibers)
}

// ContextOrdinal resolves a sealed Context identity to the ordinal assigned
// by Directory.ContextAt ordering.  Unknown, unavailable, and foreign context
// identities are refused.
func (index Index) ContextOrdinal(contextID identity.ContentID) (ContextOrdinal, bool) {
	if !index.Available() || !contextID.Available() {
		return 0, false
	}
	for ordinal, candidate := range index.contexts {
		if candidate == contextID {
			return ContextOrdinal(ordinal), true
		}
	}
	return 0, false
}

// ContextID resolves one canonical ContextOrdinal back to its Context
// identity.  It is the inverse of ContextOrdinal over the directory's sealed
// context set.
func (index Index) ContextID(ordinal ContextOrdinal) (identity.ContentID, bool) {
	if !index.Available() || uint64(ordinal) >= uint64(len(index.contexts)) {
		return identity.ContentID{}, false
	}
	return index.contexts[ordinal], true
}

// Flatten maps a bounded (ContextOrdinal, PointOrdinal) tuple to one
// context-major FiberOrdinal.  Both multiplication and addition are checked,
// even though New has already checked the complete shape product, so this law
// remains explicit at the hot coordinate boundary.
func (index Index) Flatten(context ContextOrdinal, point PointOrdinal) (FiberOrdinal, bool) {
	if !index.Available() || uint64(context) >= uint64(len(index.contexts)) || uint64(point) >= index.points {
		return 0, false
	}
	base, ok := checkedProduct(uint64(context), index.points)
	if !ok || base > math.MaxUint64-uint64(point) {
		return 0, false
	}
	flat := base + uint64(point)
	if flat >= index.fibers {
		return 0, false
	}
	return FiberOrdinal(flat), true
}

// Unflatten maps one bounded FiberOrdinal back to its exact context-major
// tuple.  The returned ordinals are valid precisely when the bool is true.
func (index Index) Unflatten(fiber FiberOrdinal) (ContextOrdinal, PointOrdinal, bool) {
	if !index.Available() || uint64(fiber) >= index.fibers {
		return 0, 0, false
	}
	flat := uint64(fiber)
	return ContextOrdinal(flat / index.points), PointOrdinal(flat % index.points), true
}

// OwnedBy proves that index belongs to exactly the supplied sealed directory,
// point shape, and generation.  The directory owner digest is canonical, so a
// permutation of the same sealed rows remains the same owner, while a foreign
// Link/context set or a different point shape fails closed.
func (index Index) OwnedBy(directory executioncontext.Directory, pointCount int, generation identity.Generation) bool {
	if !index.Available() || !directory.Available() || pointCount <= 0 || !generation.Available() || generation != index.generation || uint64(pointCount) != index.points {
		return false
	}
	contexts := make([]identity.ContentID, directory.ContextCount())
	if len(contexts) == 0 {
		return false
	}
	for ordinal := range contexts {
		row, ok := directory.ContextAt(ordinal)
		if !ok || !row.Available() || row.LinkID() != directory.LinkID() {
			return false
		}
		contexts[ordinal] = row.ID()
	}
	owner, ok := directoryOwner(directory, contexts)
	if !ok || owner != index.owner || len(contexts) != len(index.contexts) {
		return false
	}
	product, ok := checkedProduct(uint64(len(contexts)), uint64(pointCount))
	return ok && product == index.fibers
}

func checkedProduct(left, right uint64) (uint64, bool) {
	if left != 0 && right > math.MaxUint64/left {
		return 0, false
	}
	return left * right, true
}

func directoryOwner(directory executioncontext.Directory, contexts []identity.ContentID) (identity.ContentID, bool) {
	if !directory.Available() || !directory.LinkID().Available() || len(contexts) == 0 || len(contexts) != directory.ContextCount() {
		return identity.ContentID{}, false
	}
	parts := make([][]byte, 0, len(contexts)+1)
	linkID := directory.LinkID()
	parts = append(parts, linkID[:])
	for _, contextID := range contexts {
		if !contextID.Available() {
			return identity.ContentID{}, false
		}
		parts = append(parts, contextID[:])
	}
	return identity.DeriveContentID(directoryOwnerDomain, parts...)
}
