package programschema

import "github.com/wippyai/go-lua/analysis/identity"

// Environment arm ordinals retain the historical Flow boundary-arm ordinals.
// They are kept as plain bytes: the cold column preserves the value and shape
// but does not import or reinterpret the Program vocabulary.
const (
	EnvironmentArmLocal  uint8 = 1
	EnvironmentArmCancel uint8 = 8
)

// EnvironmentReset is one reset witness member. Its position is its ordinal
// in EnvironmentResetFamily and the parent edge names the half-open span it
// owns, so no edge retains a slice header.
type EnvironmentReset struct{ id identity.ContentID }

// NewEnvironmentReset copies one canonical reset witness identity.
func NewEnvironmentReset(id identity.ContentID) (EnvironmentReset, bool) {
	row := EnvironmentReset{id: id}
	return row, row.Available()
}

func (row EnvironmentReset) Available() bool { return row.id.Available() }

func (row EnvironmentReset) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}

// EnvironmentEdge is one canonical final route. Its reset witnesses are a
// span in EnvironmentResetFamily, preserving the canonical witness order
// while making this row flat and copy-safe.
type EnvironmentEdge struct {
	id identity.ContentID
	// from and to are the route's endpoints and this row's identity.
	// departure is where its state actually leaves once those endpoints carry
	// stages. Both are published: a reader that authenticates the route needs
	// the endpoints, and a reader that moves state along it needs departure.
	from        identity.ContentID
	departure   identity.ContentID
	to          identity.ContentID
	route       identity.ContentID
	guard       identity.ContentID
	decision    identity.ContentID
	condition   identity.ContentID
	reset       identity.ContentID
	component   identity.ContentID
	mu          identity.ContentID
	resetOffset uint32
	resetCount  uint32
	arm         uint8
	guarded     bool
	truth       bool
	hasReset    bool
	hasMu       bool
}

// NewEnvironmentEdge copies one canonical EnvironmentEdge row and replaces
// its nested reset slice with a dense EnvironmentResetFamily span.
func NewEnvironmentEdge(
	id, from, departure, to, route, guard, decision, condition, reset, component, mu identity.ContentID,
	resetOffset, resetCount uint32, arm uint8, guarded, truth, hasReset, hasMu bool,
) (EnvironmentEdge, bool) {
	if !departure.Available() {
		departure = from
	}
	row := EnvironmentEdge{
		id: id, from: from, departure: departure, to: to, route: route, guard: guard, decision: decision,
		condition: condition, reset: reset, component: component, mu: mu,
		resetOffset: resetOffset, resetCount: resetCount, arm: arm,
		guarded: guarded, truth: truth, hasReset: hasReset, hasMu: hasMu,
	}
	return row, row.Available()
}

func (row EnvironmentEdge) Available() bool {
	if !row.id.Available() || !row.from.Available() || !row.departure.Available() ||
		!row.to.Available() || !row.route.Available() ||
		row.arm < EnvironmentArmLocal || row.arm > EnvironmentArmCancel ||
		uint64(row.resetOffset)+uint64(row.resetCount) > uint64(^uint32(0)) {
		return false
	}
	if row.guarded {
		if !row.guard.Available() {
			return false
		}
	} else if row.guard.Available() || row.truth {
		return false
	}
	if row.condition.Available() && !row.guarded {
		return false
	}
	if row.hasMu != row.mu.Available() || row.hasReset != row.reset.Available() || row.hasMu != row.hasReset {
		return false
	}
	if !row.component.Available() {
		return !row.hasMu && !row.hasReset && row.resetCount == 0
	}
	if !row.hasMu {
		return !row.hasReset && row.resetCount == 0
	}
	return row.hasReset
}

func (row EnvironmentEdge) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}

func (row EnvironmentEdge) From() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.from
}

// Departure is the point the state travelling this route leaves from.
func (row EnvironmentEdge) Departure() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.departure
}

func (row EnvironmentEdge) To() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.to
}

func (row EnvironmentEdge) RouteID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.route
}

func (row EnvironmentEdge) Arm() uint8 {
	if !row.Available() {
		return 0
	}
	return row.arm
}

func (row EnvironmentEdge) GuardID() (identity.ContentID, bool) {
	return row.guard, row.Available() && row.guarded
}

func (row EnvironmentEdge) DecisionID() (identity.ContentID, bool) {
	return row.decision, row.Available() && row.guarded
}

func (row EnvironmentEdge) ConditionValueSpanID() (identity.ContentID, bool) {
	return row.condition, row.Available() && row.guarded && row.condition.Available()
}

func (row EnvironmentEdge) Truth() (bool, bool) {
	return row.truth, row.Available() && row.guarded
}

func (row EnvironmentEdge) ResetDigest() (identity.ContentID, bool) {
	return row.reset, row.Available() && row.hasReset
}

func (row EnvironmentEdge) MuPathID() (identity.ContentID, bool) {
	return row.mu, row.Available() && row.hasMu
}

func (row EnvironmentEdge) ComponentID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.component
}

func (row EnvironmentEdge) ResetSpan() (offset, count uint32, ok bool) {
	return row.resetOffset, row.resetCount, row.Available()
}

func (row EnvironmentEdge) ResetCount() int {
	if !row.Available() {
		return 0
	}
	return int(row.resetCount)
}
