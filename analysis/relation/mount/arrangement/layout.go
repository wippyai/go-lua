package arrangement

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/mount/address"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

// Layout is one immutable physical realization of one logical Access.  The
// Handle is opaque and generation-fenced; key components and delivered
// columns remain logical IDs in their authored order.  Delivery shape and
// denominator are owned only by DeliveryRequirement, never by Layout or
// Access.
type Layout struct {
	handle     Handle
	access     Access
	keyColumns []model.ColumnID
	digest     identity.ContentID
}

func newLayout(fence address.Fence, handle Handle, access Access, keyColumns []model.ColumnID) (Layout, bool) {
	if !fence.Available() || !handle.ValidFor(fence) || !access.Available() {
		return Layout{}, false
	}
	keyColumns = append([]model.ColumnID(nil), keyColumns...)
	if access.Key().Available() {
		if len(keyColumns) == 0 {
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
	} else if len(keyColumns) != 0 {
		return Layout{}, false
	}
	layout := Layout{
		handle:     handle,
		access:     cloneAccess(access),
		keyColumns: keyColumns,
	}
	value, ok := identity.DeriveContentID("analysis/relation/mount/arrangement/layout/v1", layoutDigestParts(layout)...)
	if !ok {
		return Layout{}, false
	}
	layout.digest = value
	return layout, true
}

// Available reports whether this layout carries a complete fenced physical
// realization and immutable logical vectors.
func (layout Layout) Available() bool {
	return layout.digest.Available() && layout.access.Available() && layout.handle.Available() && layout.handle.ValidFor(layout.handle.Fence())
}

// Handle returns the opaque physical coordinate for runtime registry lookup.
func (layout Layout) Handle() Handle { return layout.handle }

// Access returns the logical requirement realized by this layout.
func (layout Layout) Access() Access { return cloneAccess(layout.access) }

// KeyColumns returns the exact checked key component order. It is nil for
// relation scans and unkeyed vectors.
func (layout Layout) KeyColumns() []model.ColumnID {
	return append([]model.ColumnID(nil), layout.keyColumns...)
}

// Columns returns the delivered/access vector in authored order.
func (layout Layout) Columns() []model.ColumnID {
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
	if !layout.Available() || !other.Available() || layout.digest != other.digest || layout.handle != other.handle || !layout.access.Equal(other.access) || len(layout.keyColumns) != len(other.keyColumns) {
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
	parts := make([][]byte, 0, 2+len(layout.keyColumns))
	parts = append(parts, accessDigest(layout.access), handleDigest(layout.handle))
	for _, column := range layout.keyColumns {
		parts = append(parts, nominalBytes(column.Relation().Owner().Content(), column.Content()))
	}
	return parts
}
