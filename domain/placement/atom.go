package placement

import (
	"github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/materialization"
	runtimekind "github.com/wippyai/go-lua/domain/runtimekind"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// AtomClass is the Placement-relevant shape of one Value atom.  It is a
// deliberately small projection: Value remains the authority for the atom,
// its exact reference, and its runtime-kind set, while Placement only needs
// to know whether the atom names one local allocation root, a non-local root,
// an opaque reference alternative, or no reference at all.
//
// The distinction between AtomClassRoot and AtomClassOpaque is important.  An
// exact Boot/callable/endpoint handle has no local Placement allocation route;
// an opaque reference may denote an allocation that Value cannot name and must
// therefore be widened by the consuming policy.
type AtomClass uint8

const (
	AtomClassInvalid AtomClass = iota
	// AtomClassScalar includes nil and exact scalar/literal alternatives. They
	// carry no local Heap allocation route.
	AtomClassScalar
	// AtomClassAllocation names one exact Heap RootAllocation key.
	AtomClassAllocation
	// AtomClassBoot names an exact actor-local Heap RootBoot handle.
	AtomClassBoot
	// AtomClassRoot names an exact non-allocation structural root, such as a
	// host endpoint or callable. It is valid but has no local route.
	AtomClassRoot
	// AtomClassOpaque is a reference-kind alternative without an exact root.
	AtomClassOpaque
)

// Valid reports whether c is one of the closed atom classes.
func (c AtomClass) Valid() bool { return c >= AtomClassScalar && c <= AtomClassOpaque }

// AtomClassification is the zero-copy result of ClassifyAtom. Key and Role
// are present only for AtomClassAllocation; all other classes keep their zero
// values.
// Returning this compact value is intentional: the hot Value-to-Placement
// walks can classify atoms without creating a slice, map, or heap object.
type AtomClassification struct {
	Class AtomClass
	Key   heap.Key
	Role  materialization.Role
}

// Valid reports whether the classification and any attached exact key are
// internally consistent.
func (classification AtomClassification) Valid() bool {
	switch classification.Class {
	case AtomClassScalar, AtomClassBoot, AtomClassRoot, AtomClassOpaque:
		return !classification.Key.Valid() && classification.Role == materialization.Invalid
	case AtomClassAllocation:
		return classification.Key.Valid() && classification.Key.Kind() == heap.RootAllocation && classification.Role.Valid()
	default:
		return false
	}
}

// ClassifyAtom authenticates and classifies one exact Value atom for a
// Placement consumer. It performs no allocation and returns no borrowed
// memory. The caller must still authenticate an allocation key against its
// own Placement/Heap schema before routing it; this helper only centralizes
// the Value-side atom distinction shared by all Placement consumers.
//
// A malformed or foreign atom/reference is rejected. Exact non-allocation
// roots (including Boot roots) are not opaque: they have no local allocation
// route. Opaque reference-kind alternatives are the only exact atom class
// that requests conservative widening at a route boundary.
func ClassifyAtom(values *valuedomain.Schema, atom valuedomain.Atom) (AtomClassification, bool) {
	if values == nil || !values.Valid() || !values.OwnsAtom(atom) {
		return AtomClassification{}, false
	}

	reference, role, referenceOK := atom.Reference()
	if referenceOK {
		if !values.OwnsReference(reference) {
			return AtomClassification{}, false
		}
		if key, keyOK := reference.AllocationKey(); keyOK {
			if !key.Valid() || key.Kind() != heap.RootAllocation {
				return AtomClassification{}, false
			}
			return AtomClassification{Class: AtomClassAllocation, Key: key, Role: role}, true
		}
		if bootID, bootOK := reference.BootRootID(); bootOK {
			if !bootID.Available() {
				return AtomClassification{}, false
			}
			return AtomClassification{Class: AtomClassBoot}, true
		}
		return AtomClassification{Class: AtomClassRoot}, true
	}

	if atom.RuntimeKinds()&runtimekind.Reference != 0 {
		return AtomClassification{Class: AtomClassOpaque}, true
	}
	return AtomClassification{Class: AtomClassScalar}, true
}
