// Package store owns the lifetime part of Placement's storage census.
//
// Heap containment and Program storage are different edges.  An object write
// is a graph edge whose child inherits the destination allocation's current
// placement; a write to a Program cell is a lifetime edge whose consequence
// depends on the cell's owner (frame, module, retained closure, Link-global,
// or external).
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
	// LifetimeClosure is retained by a closure environment. It outlives the
	// introducing frame but is not module-entry state, so it demands an owned
	// heap placement without claiming module ownership.
	LifetimeClosure
)

// Valid reports whether the lifetime is a closed Placement storage class.
func (lifetime Lifetime) Valid() bool {
	return lifetime >= LifetimeFrame && lifetime <= LifetimeClosure
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
	case LifetimeClosure:
		return "closure"
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
	case lifecycle.StorageLifetimeClosure:
		return LifetimeClosure
	default:
		return LifetimeInvalid
	}
}

// Demand is the least Placement required by a storage destination.  The
// second result says whether a transition is required and the third says
// whether lifetime was a valid authenticated storage class.  Frame storage
// has no escape demand and therefore returns (Bottom, false, true).  An
// invalid lifetime returns (Bottom, false, false); it must never be turned
// into Placement.Unknown, because Unknown is a real authenticated semantic
// state, not a malformed-input sentinel.
func Demand(lifetime Lifetime) (placement.Placement, bool, bool) {
	switch lifetime {
	case LifetimeFrame:
		return placement.Bottom, false, true
	case LifetimeModule:
		return placement.OwnedHeap, true, true
	case LifetimeClosure:
		return placement.OwnedHeap, true, true
	case LifetimeGlobal:
		return placement.SharedHeap, true, true
	case LifetimeExternal, LifetimeUnknown:
		return placement.Unknown, true, true
	default:
		return placement.Bottom, false, false
	}
}

// Apply applies one Program-cell lifetime demand to the current source
// placement.  It is monotone and never promotes a proven frame-local write.
// Unknown destination evidence is conservative Unknown; it is not silently
// converted to Frame or Stack.  The boolean reports whether both inputs were
// valid and the transition was produced.  On failure the Placement result is
// only a Bottom sentinel and must be ignored.
func Apply(current placement.Fact, lifetime Lifetime) (placement.Fact, bool) {
	if !validFact(current) || !lifetime.Valid() {
		return placement.BottomFact(), false
	}
	demand, forced, demandOK := Demand(lifetime)
	if !demandOK {
		return placement.BottomFact(), false
	}
	if !forced {
		return current, true
	}
	switch demand {
	case placement.OwnedHeap:
		return placement.RetainAtClassChecked(current, placement.OwnedHeap)
	case placement.SharedHeap:
		return placement.RetainAtClassChecked(current, placement.SharedHeap)
	case placement.Unknown:
		return placement.UnknownFact(), true
	default:
		return placement.BottomFact(), false
	}
}

// ObjectStore propagates an object payload through Heap containment.  The
// destination allocation's current placement is the parent demand; the
// source's current placement is joined so this operation cannot lower a
// demand established by another path.  It is intentionally independent from
// Apply: ordinary object writes do not imply a Program-cell lifetime escape.
// The boolean reports whether both Placement inputs were valid.  Invalid
// inputs are refused rather than widened to Unknown.
func ObjectStore(destination, source placement.Fact) (placement.Fact, bool) {
	if !validFact(destination) || !validFact(source) {
		return placement.BottomFact(), false
	}
	return placement.ThroughContainerChecked(source, destination)
}

func validFact(value placement.Fact) bool {
	return value.Valid() && value.Class != placement.Bottom && value.RetainEscape != placement.EvidenceAbsent
}

func validPlacement(value placement.Placement) bool {
	switch value {
	case placement.Bottom, placement.Stack, placement.OwnedHeap, placement.SharedHeap, placement.Unknown:
		return true
	default:
		return false
	}
}
