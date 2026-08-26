package arrangement

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/mount/address"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

// CoordinateClass is the sealed ownership class of a physical layout
// coordinate.  The class is part of Layout's digest and cannot be changed
// after mount.
type CoordinateClass uint8

const (
	CoordinateClassInvalid CoordinateClass = iota
	// CoordinateClassNone is an ordinary relation scan or unkeyed vector.
	CoordinateClassNone
	// CoordinateClassDeclaredKey is a coordinate declared by KeySchema.
	CoordinateClassDeclaredKey
	// CoordinateClassStableCorrespondence is an owner-issued exact Join or
	// Project correspondence vector. Its source cells are stable coordinates.
	CoordinateClassStableCorrespondence
	// CoordinateClassLookupOnly is a physically indexed vector whose source
	// facts may ascend, such as a Merge alternative.
	CoordinateClassLookupOnly
)

// Available reports whether the class is one of the closed sealed values.
func (class CoordinateClass) Available() bool {
	return class >= CoordinateClassNone && class <= CoordinateClassLookupOnly
}

// Layout is one immutable physical realization of one logical Access.  The
// Handle is opaque and generation-fenced; key components and delivered
// columns remain logical IDs in their authored order.  Delivery shape and
// denominator are owned only by DeliveryRequirement, never by Layout or
// Access.
//
// An unkeyed Access may still carry keyColumns when the access is an exact
// correspondence or Merge lookup vector. This is the same Access/Layout/index
// vocabulary as ordinary keyed arrangements: the checked vector is promoted
// to the trie coordinate so a Reader can redeem it with Lookup. The logical
// Access keeps its zero KeyID, preserving the distinction between a relation
// key contract and an expression-owned physical correspondence.
type Layout struct {
	handle          Handle
	access          Access
	keyColumns      []model.ColumnID
	coordinateClass CoordinateClass
	digest          identity.ContentID
	// sealed is set only after the constructor has validated both logical and
	// physical vectors and issued the digest. Layout availability is therefore
	// a constant-time redemption of that owner proof.
	sealed bool
}

func newLayout(fence address.Fence, handle Handle, access Access, keyColumns []model.ColumnID) (Layout, bool) {
	coordinateClass := CoordinateClassNone
	if access.Key().Available() {
		coordinateClass = CoordinateClassDeclaredKey
	} else if len(keyColumns) != 0 {
		// The legacy constructor has no sealed expression proof with which to
		// classify an unkeyed physical coordinate. Require Derive to use the
		// class-bearing constructor instead of inferring immutability from shape.
		return Layout{}, false
	}
	return newLayoutWithClass(fence, handle, access, keyColumns, coordinateClass)
}

func newLayoutWithClass(fence address.Fence, handle Handle, access Access, keyColumns []model.ColumnID, coordinateClass CoordinateClass) (Layout, bool) {
	if !fence.Available() || !handle.ValidFor(fence) || !access.Available() {
		return Layout{}, false
	}
	if !coordinateClass.Available() {
		return Layout{}, false
	}
	keyColumns = append([]model.ColumnID(nil), keyColumns...)
	if access.Key().Available() {
		if coordinateClass != CoordinateClassDeclaredKey || len(keyColumns) == 0 {
			return Layout{}, false
		}
		seen := make(map[model.ColumnID]struct{}, len(keyColumns))
		for _, column := range keyColumns {
			if !column.Available() || column.Relation() != access.Relation() {
				return Layout{}, false
			}
			if _, exists := seen[column]; exists {
				return Layout{}, false
			}
			seen[column] = struct{}{}
		}
	} else if len(keyColumns) == 0 {
		if coordinateClass != CoordinateClassNone {
			return Layout{}, false
		}
	} else {
		// An unkeyed Access may carry a sealed physical correspondence or
		// Merge lookup vector. The coordinate is a checked subset of the
		// delivered row, so Lookup can redeem complete rows by that exact
		// vector without changing the logical Access.
		if coordinateClass != CoordinateClassStableCorrespondence && coordinateClass != CoordinateClassLookupOnly {
			return Layout{}, false
		}
		seen := make(map[model.ColumnID]struct{}, len(keyColumns))
		for _, column := range keyColumns {
			if !column.Available() || column.Relation() != access.Relation() {
				return Layout{}, false
			}
			found := false
			for _, delivered := range access.columns {
				if delivered == column {
					found = true
					break
				}
			}
			if !found {
				return Layout{}, false
			}
			if _, duplicate := seen[column]; duplicate {
				return Layout{}, false
			}
			seen[column] = struct{}{}
		}
		if coordinateClass == CoordinateClassStableCorrespondence && !sameColumns(access.columns, keyColumns) {
			return Layout{}, false
		}
	}
	layout := Layout{
		handle:          handle,
		access:          cloneAccess(access),
		keyColumns:      keyColumns,
		coordinateClass: coordinateClass,
	}
	value, ok := identity.DeriveContentID("analysis/relation/mount/arrangement/layout/v1", layoutDigestParts(layout)...)
	if !ok {
		return Layout{}, false
	}
	layout.digest = value
	layout.sealed = true
	return layout, true
}

// Available reports whether this layout carries a complete fenced physical
// realization and immutable logical vectors.
func (layout Layout) Available() bool {
	return layout.sealed && layout.digest.Available() && layout.handle.Available() && layout.handle.ValidFor(layout.handle.Fence()) && layout.access.Available() && layout.coordinateClass.Available()
}

// Handle returns the opaque physical coordinate for runtime registry lookup.
func (layout Layout) Handle() Handle { return layout.handle }

// Access returns the logical requirement realized by this layout.
func (layout Layout) Access() Access {
	if !layout.Available() {
		return Access{}
	}
	return cloneAccess(layout.access)
}

// KeyColumns returns the exact checked physical key component order. It is nil
// for relation scans and ordinary unkeyed vectors. A correspondence vector is
// intentionally represented by the same coordinate field even though
// Access.Key remains unavailable; a Merge vector may be a checked subset of
// the delivered Access.Columns.
func (layout Layout) KeyColumns() []model.ColumnID {
	if !layout.Available() {
		return nil
	}
	return append([]model.ColumnID(nil), layout.keyColumns...)
}

// KeyWidth returns the sealed key-vector arity without projecting the
// defensive column slice. Runtime operators use this scalar to redeem a
// mounted plan on their hot path; key ownership and ordering remain private
// to the immutable Layout.
func (layout Layout) KeyWidth() int {
	if !layout.Available() {
		return 0
	}
	return len(layout.keyColumns)
}

// CoordinateClass returns the sealed ownership class of this layout. An
// unavailable layout returns CoordinateClassInvalid.
func (layout Layout) CoordinateClass() CoordinateClass {
	if !layout.Available() {
		return CoordinateClassInvalid
	}
	return layout.coordinateClass
}

// Columns returns the delivered/access vector in authored order.
func (layout Layout) Columns() []model.ColumnID {
	if !layout.Available() {
		return nil
	}
	return layout.access.Columns()
}

// ValidFor reports whether this immutable layout belongs to the exact fence.
func (layout Layout) ValidFor(fence address.Fence) bool {
	return layout.Available() && fence.Available() && layout.handle.ValidFor(fence)
}

// Digest returns the deterministic physical layout identity.
func (layout Layout) Digest() identity.ContentID { return layout.digest }

// Equal compares complete logical and physical layout content.
func (layout Layout) Equal(other Layout) bool {
	if !layout.Available() || !other.Available() || layout.digest != other.digest || layout.handle != other.handle || layout.coordinateClass != other.coordinateClass || !layout.access.Equal(other.access) || len(layout.keyColumns) != len(other.keyColumns) {
		return false
	}
	for index := range layout.keyColumns {
		if layout.keyColumns[index] != other.keyColumns[index] {
			return false
		}
	}
	return true
}

func layoutDigestParts(layout Layout) [][]byte {
	parts := make([][]byte, 0, 3+len(layout.keyColumns))
	parts = append(parts, accessDigest(layout.access), handleDigest(layout.handle), []byte{byte(layout.coordinateClass)})
	for _, column := range layout.keyColumns {
		parts = append(parts, nominalBytes(column.Relation().Owner().Content(), column.Content()))
	}
	return parts
}
