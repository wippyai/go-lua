// Package store owns the lifetime part of Placement's storage census.
//
// Heap containment and Program storage are different edges.  An object write
// is a graph edge whose child inherits the destination allocation's current
// placement; a write to a Program cell is a lifetime edge whose consequence
// depends on the cell's owner (frame, module, Link-global, or external).
// Keeping these operations separate is important: treating every storage
// transfer as Store would promote ordinary local assignments and hide the
// actual escape boundary.
//
// This package intentionally owns no Program or Value identity. The mounted
// consumer feeds it Value's owner-issued StorageTransfer row, whose lifetime
// was derived from the neutral Program schema. Missing lifetime evidence is
// represented as LifetimeUnknown rather than guessed to be frame-local.
package store

import (
	"github.com/wippyai/go-lua/analysis/schema/program/lifecycle"
	"github.com/wippyai/go-lua/domain/placement"
)

// Lifetime is the retention boundary of a Program storage destination.  The
// values are semantic classes, not storage coordinates or compiler terms.
// A coordinate is supplied by the neutral Program schema; this enum only
// interprets the already-proven class.
type Lifetime uint8

const (
	LifetimeInvalid Lifetime = iota
	// LifetimeFrame is a lexical cell that dies with the current activation.
	LifetimeFrame
	// LifetimeModule is retained by one mounted module, outliving its current
	// frame but not necessarily being exported through the Link boundary.
	LifetimeModule
	// LifetimeGlobal is a Link/actor global retained beyond the module frame.
	LifetimeGlobal
	// LifetimeExternal is handed to an owner outside the analyzed storage
	// lifetime.  Its eventual owner is opaque to Placement.
	LifetimeExternal
	// LifetimeUnknown means that the neutral storage-lifetime proof was absent
	// or widened.  It is deliberately not treated as frame-local.
	LifetimeUnknown
)

// Valid reports whether the lifetime is a closed Placement storage class.
func (lifetime Lifetime) Valid() bool {
	return lifetime >= LifetimeFrame && lifetime <= LifetimeUnknown
}

// String returns the stable diagnostic spelling of a lifetime class.
func (lifetime Lifetime) String() string {
	switch lifetime {
	case LifetimeFrame:
		return "frame"
	case LifetimeModule:
		return "module"
	case LifetimeGlobal:
		return "global"
	case LifetimeExternal:
		return "external"
	case LifetimeUnknown:
		return "unknown"
	default:
		return "lifetime(invalid)"
	}
}

// FromProgram maps the neutral Program storage proof into this domain's
// interpretation. The conversion is explicit so Placement cannot silently
// treat an unavailable schema row as a frame-local default.
func FromProgram(lifetime lifecycle.StorageLifetime) Lifetime {
	switch lifetime {
	case lifecycle.StorageLifetimeFrame:
		return LifetimeFrame
	case lifecycle.StorageLifetimeModule:
		return LifetimeModule
	case lifecycle.StorageLifetimeGlobal:
		return LifetimeGlobal
	case lifecycle.StorageLifetimeExternal:
		return LifetimeExternal
	case lifecycle.StorageLifetimeUnknown:
		return LifetimeUnknown
	default:
		return LifetimeInvalid
	}
}

// Demand is the least Placement required by a storage destination.  Frame
// storage has no escape demand and therefore returns (Bottom, false).  The
// distinction matters: Bottom here means "no transition", never "the value
// is known to be stack-local".
func Demand(lifetime Lifetime) (placement.Placement, bool) {
	switch lifetime {
	case LifetimeFrame:
		return placement.Bottom, false
	case LifetimeModule:
		return placement.OwnedHeap, true
	case LifetimeGlobal:
		return placement.SharedHeap, true
	case LifetimeExternal, LifetimeUnknown:
		return placement.Unknown, true
	default:
		return placement.Unknown, true
	}
}

// Apply applies one Program-cell lifetime demand to the current source
// placement.  It is monotone and never promotes a proven frame-local write.
// Unknown destination evidence is conservative Unknown; it is not silently
// converted to Frame or Stack.
func Apply(current placement.Placement, lifetime Lifetime) placement.Placement {
	if !validPlacement(current) || !lifetime.Valid() {
		return placement.Unknown
	}
	demand, forced := Demand(lifetime)
	if !forced {
		return current
	}
	switch demand {
	case placement.OwnedHeap:
		return placement.Displace(current, placement.Retain)
	case placement.SharedHeap:
		return placement.Displace(current, placement.Send)
	case placement.Unknown:
		return placement.Unknown
	default:
		return placement.Unknown
	}
}

// ObjectStore propagates an object payload through Heap containment.  The
// destination allocation's current placement is the parent demand; the
// source's current placement is joined so this operation cannot lower a
// demand established by another path.  It is intentionally independent from
// Apply: ordinary object writes do not imply a Program-cell lifetime escape.
func ObjectStore(destination, source placement.Placement) placement.Placement {
	if !validPlacement(destination) || !validPlacement(source) {
		return placement.Unknown
	}
	return placement.Join(destination, source)
}

func validPlacement(value placement.Placement) bool {
	switch value {
	case placement.Bottom, placement.Stack, placement.OwnedHeap, placement.SharedHeap, placement.Unknown:
		return true
	default:
		return false
	}
}
