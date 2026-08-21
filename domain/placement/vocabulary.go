package placement

import internal "github.com/wippyai/go-lua/internal/hash"

// Escape is the canonical, name-frozen escape disposition.
type Escape uint8

const (
	None Escape = iota
	Borrow
	Retain
	Store
	Send
	Export
	Opaque
	Return
)

// ValidManifest reports whether e belongs to the stable manifest vocabulary.
// Return is encoded there as Export plus ThroughReturn.
func (e Escape) ValidManifest() bool {
	return e >= None && e <= Opaque
}

// Name returns the stable manifest spelling. Return is included for
// placement-policy diagnostics even though it has no standalone manifest wire
// spelling. It is deliberately not String: the displaced integer enums
// historically rendered numerically through fmt, and compatibility includes
// retaining that incidental debug form.
func (e Escape) Name() string {
	switch e {
	case None:
		return "none"
	case Borrow:
		return "borrow"
	case Retain:
		return "retain"
	case Store:
		return "store"
	case Send:
		return "send"
	case Export:
		return "export"
	case Opaque:
		return "opaque"
	case Return:
		return "return"
	default:
		return "escape(invalid)"
	}
}

// Placement returns the analysis placement required by this escape. None and
// Borrow do not force a transition.
func (e Escape) Placement() (Placement, bool) {
	switch e {
	case Retain, Store, Return:
		return OwnedHeap, true
	case Send, Export, Opaque:
		return SharedHeap, true
	default:
		return Bottom, false
	}
}

// Placement is the canonical, name-frozen allocation/runtime placement.
//
// Bottom through Unknown retain Placement's historical numeric layout so
// lattice hashes remain stable.  Compiled artifacts never serialize these
// numbers directly; equation owns an explicit frozen ordinal codec.
type Placement uint8

const (
	Bottom Placement = iota
	Stack
	OwnedHeap
	SharedHeap
	Unknown
	Interpreter
	Register
)

// IsBottom reports whether the placement is unreachable.
func (p Placement) IsBottom() bool { return p == Bottom }

// IsTop reports whether the placement is the conservative analysis top.
func (p Placement) IsTop() bool { return p == Unknown }

// Hash is stable and consistent with placement lattice equality.
func (p Placement) Hash() uint64 {
	return internal.MixHash(internal.FnvString("placement"), uint64(p))
}

// Covers reports whether p is at least as conservative as other in the
// analysis placement chain. JIT-only values do not cover analysis placements.
func (p Placement) Covers(other Placement) bool {
	rank := func(value Placement) (int, bool) {
		switch value {
		case Bottom:
			return 0, true
		case Stack:
			return 1, true
		case OwnedHeap:
			return 2, true
		case SharedHeap:
			return 3, true
		case Unknown:
			return 4, true
		default:
			return 0, false
		}
	}
	left, leftOK := rank(p)
	right, rightOK := rank(other)
	return leftOK && rightOK && left >= right
}

// String returns the stable rendered spelling used by placement/native
// fixtures.  JIT-only variants have explicit spellings for diagnostics; they
// are not analysis placement wire values.
func (p Placement) String() string {
	switch p {
	case Bottom:
		return "bottom"
	case Stack:
		return "stack"
	case OwnedHeap:
		return "owned-heap"
	case SharedHeap:
		return "shared-heap"
	case Unknown:
		return "unknown"
	case Interpreter:
		return "interpreter"
	case Register:
		return "register"
	default:
		return "placement(invalid)"
	}
}

// Consequence is the manifest projection of an escape onto caller placement.
type Consequence string

const (
	Keep              Consequence = "keep"
	ConsequenceOwned  Consequence = "owned-heap"
	ConsequenceShared Consequence = "shared-heap"
)

// Valid reports whether c is a serialized manifest consequence.
func (c Consequence) Valid() bool {
	return c == Keep || c == ConsequenceOwned || c == ConsequenceShared
}

// Event is the typed in-memory form of a placement/event fact label.
type Event string

const (
	EventOwned       Event = "owned"
	EventShared      Event = "shared"
	EventSealed      Event = "sealed"
	EventEnvironment Event = "environment"
	EventSuspended   Event = "suspended"
)

// ParseEvent admits exactly the frozen placement/event wire labels.
func ParseEvent(wire string) (Event, bool) {
	event := Event(wire)
	switch event {
	case EventOwned, EventShared, EventSealed, EventEnvironment, EventSuspended:
		return event, true
	default:
		return "", false
	}
}
