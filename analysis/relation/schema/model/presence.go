package model

// PresenceKind is the closed logical presence vocabulary.  Presence is not a
// domain lattice value and never carries a semantic payload.
type PresenceKind uint8

const (
	// InvalidPresence is the unavailable zero value.
	InvalidPresence PresenceKind = iota
	// Present means an authenticated semantic cell exists.
	Present
	// ProvenAbsent means authenticated evidence proves no cell exists.
	ProvenAbsent
	// UnprovenMissing means no cell is currently known, without proof of
	// absence.
	UnprovenMissing
	// AuthenticatedOpaque means a cell is authenticated but its semantic value
	// is intentionally opaque to this layer.
	AuthenticatedOpaque
	// Refused means evaluation refused with a stable refusal identity.
	Refused
)

// String returns the canonical status label.
func (kind PresenceKind) String() string {
	switch kind {
	case Present:
		return "Present"
	case ProvenAbsent:
		return "ProvenAbsent"
	case UnprovenMissing:
		return "UnprovenMissing"
	case AuthenticatedOpaque:
		return "AuthenticatedOpaque"
	case Refused:
		return "Refused"
	default:
		return "InvalidPresence"
	}
}

// Presence is an immutable logical status.  Only Refused carries a reason;
// all other statuses have an unavailable reason by construction.
type Presence struct {
	kind   PresenceKind
	reason RefusalID
}

// NewPresence constructs a non-refusal status.  Refused must be made with
// NewRefused so a valid reason identity is always attached.
func NewPresence(kind PresenceKind) (Presence, bool) {
	switch kind {
	case Present, ProvenAbsent, UnprovenMissing, AuthenticatedOpaque:
		return Presence{kind: kind}, true
	default:
		return Presence{}, false
	}
}

// NewRefused constructs a refusal only when reason is a valid owner-issued
// identity.  A zero or foreign/unavailable reason fails closed.
func NewRefused(reason RefusalID) (Presence, bool) {
	if !reason.Available() {
		return Presence{}, false
	}
	return Presence{kind: Refused, reason: reason}, true
}

// Kind returns the closed status kind.
func (presence Presence) Kind() PresenceKind { return presence.kind }

// Available reports whether presence is a complete status.  Refused additionally
// requires a valid reason identity.
func (presence Presence) Available() bool {
	switch presence.kind {
	case Present, ProvenAbsent, UnprovenMissing, AuthenticatedOpaque:
		return !presence.reason.Available()
	case Refused:
		return presence.reason.Available()
	default:
		return false
	}
}

// Is reports whether presence has kind.  Invalid and incomplete statuses
// never compare as a valid logical status.
func (presence Presence) Is(kind PresenceKind) bool {
	return presence.Available() && presence.kind == kind
}

// Reason returns the refusal identity when and only when kind is Refused.
func (presence Presence) Reason() (RefusalID, bool) {
	if !presence.Is(Refused) {
		return RefusalID{}, false
	}
	return presence.reason, true
}
