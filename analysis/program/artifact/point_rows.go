package artifact

import "github.com/wippyai/go-lua/analysis/identity"

// Point is an exact parent-issued LocalWTO phase vertex path. Its ordered
// decision IDs and initial disposition are copied from canonical Flow point
// sites; Link never has to reopen Program to recover point geometry.
type Point struct {
	id        identity.ContentID
	decisions []identity.ContentID
	initial   bool
}

func (point Point) ID() identity.ContentID { return point.id }
func (point Point) Available() bool        { return point.id.Available() }
func (point Point) DecisionCount() int {
	if !point.Available() {
		return 0
	}
	return len(point.decisions)
}
func (point Point) DecisionAt(index int) (identity.ContentID, bool) {
	if !point.Available() || index < 0 || index >= len(point.decisions) {
		return identity.ContentID{}, false
	}
	return point.decisions[index], true
}
func (point Point) Initial() (bool, bool) { return point.initial, point.Available() }

// RouteKind is the artifact-owned closed route-arm vocabulary.
type RouteKind uint8

const (
	RouteInvalid RouteKind = iota
	RouteLocal
	RouteResume
	RouteSelectTrue
	RouteSelectFalse
	RouteTail
	RouteThrow
	RouteYield
	RouteCancel
)

func (kind RouteKind) Valid() bool { return kind >= RouteLocal && kind <= RouteCancel }

// EnvironmentEdge is a scalar copy of one exact canonical Flow final route.
// Its ID is the stable route-occurrence identity. RouteID is the parent
// final-route semantic ID and is not a second artifact identity.
type EnvironmentEdge struct {
	id        identity.ContentID
	from      identity.ContentID
	to        identity.ContentID
	route     identity.ContentID
	guard     identity.ContentID
	decision  identity.ContentID
	condition identity.ContentID
	reset     identity.ContentID
	resets    []identity.ContentID
	component identity.ContentID
	arm       RouteKind
	guarded   bool
	truth     bool
	hasReset  bool
	mu        identity.ContentID
	hasMu     bool
}

// LocalTransfer is a Program-artifact-owned acyclic stage transport. It is
// distinct from EnvironmentEdge and therefore never owns a guard, recurrence
// component, Mu, or reset witness. Full transports carry the complete
// environment. Factor transports carry exactly the factors identified by a
// canonical list of artifact-owned rule roles; consumers resolve those roles
// only through their sealed schema directory.
type LocalTransfer struct {
	id    identity.ContentID
	from  identity.ContentID
	to    identity.ContentID
	full  bool
	roles []RuleRole
}

func (edge LocalTransfer) ID() identity.ContentID   { return edge.id }
func (edge LocalTransfer) From() identity.ContentID { return edge.from }
func (edge LocalTransfer) To() identity.ContentID   { return edge.to }
func (edge LocalTransfer) FullEnvironment() bool    { return edge.Available() && edge.full }

func (edge LocalTransfer) FactorRoleCount() int {
	if !edge.Available() || edge.full {
		return 0
	}
	return len(edge.roles)
}
func (edge LocalTransfer) FactorRoleAt(index int) (RuleRole, bool) {
	if !edge.Available() || edge.full || index < 0 || index >= len(edge.roles) {
		return RuleRoleInvalid, false
	}
	return edge.roles[index], true
}
func (edge LocalTransfer) Available() bool {
	if !edge.id.Available() || !edge.from.Available() || !edge.to.Available() || edge.from == edge.to || edge.full == (len(edge.roles) != 0) {
		return false
	}
	for index, role := range edge.roles {
		if !role.valid() || index != 0 && edge.roles[index-1] >= role {
			return false
		}
	}
	return true
}

func (edge EnvironmentEdge) ID() identity.ContentID      { return edge.id }
func (edge EnvironmentEdge) From() identity.ContentID    { return edge.from }
func (edge EnvironmentEdge) To() identity.ContentID      { return edge.to }
func (edge EnvironmentEdge) RouteID() identity.ContentID { return edge.route }
func (edge EnvironmentEdge) Arm() RouteKind              { return edge.arm }

// DecisionID is the parent-issued decision Site identity for a guarded
// route. It is distinct from GuardID: GuardID authenticates the guard proof,
// while DecisionID is the coordinate needed to issue the exact Link-local
// guard expression. Unguarded routes deliberately have neither identity.
func (edge EnvironmentEdge) DecisionID() (identity.ContentID, bool) {
	return edge.decision, edge.Available() && edge.guarded
}

func (edge EnvironmentEdge) GuardID() (identity.ContentID, bool) {
	return edge.guard, edge.Available() && edge.guarded
}

// ConditionValueSpanID is the generic Program-issued branch condition value
// identity. It is absent for non-Branch guards and carries no diagnostic or
// domain meaning.
func (edge EnvironmentEdge) ConditionValueSpanID() (identity.ContentID, bool) {
	return edge.condition, edge.Available() && edge.guarded && edge.condition.Available()
}

func (edge EnvironmentEdge) Truth() (bool, bool) {
	return edge.truth, edge.Available() && edge.guarded
}

func (edge EnvironmentEdge) ResetDigest() (identity.ContentID, bool) {
	return edge.reset, edge.Available() && edge.hasReset
}

func (edge EnvironmentEdge) HasResetWitness() bool {
	return edge.Available() && edge.hasReset
}

func (edge EnvironmentEdge) ResetCount() int {
	if !edge.Available() {
		return 0
	}
	return len(edge.resets)
}

func (edge EnvironmentEdge) ResetAt(index int) (identity.ContentID, bool) {
	if !edge.Available() || index < 0 || index >= len(edge.resets) {
		return identity.ContentID{}, false
	}
	return edge.resets[index], true
}

func (edge EnvironmentEdge) ComponentID() identity.ContentID {
	if !edge.Available() {
		return identity.ContentID{}
	}
	return edge.component
}

func (edge EnvironmentEdge) MuPathID() (identity.ContentID, bool) {
	return edge.mu, edge.Available() && edge.hasMu
}

func (edge EnvironmentEdge) Available() bool {
	if !edge.id.Available() || !edge.from.Available() || !edge.to.Available() || !edge.route.Available() || !edge.arm.Valid() {
		return false
	}
	if !((edge.guarded && edge.guard.Available()) || (!edge.guarded && !edge.guard.Available() && !edge.truth)) {
		return false
	}
	if edge.condition.Available() && !edge.guarded {
		return false
	}
	if edge.hasMu != edge.mu.Available() || edge.hasReset != edge.reset.Available() || edge.hasMu != edge.hasReset {
		return false
	}
	if !edge.component.Available() {
		return !edge.hasMu && !edge.hasReset && len(edge.resets) == 0
	}
	if !edge.hasMu {
		return !edge.hasReset && len(edge.resets) == 0
	}
	return edge.hasReset
}

// WTOEventKind brackets the exact parent-issued nested weak-topological order.
type WTOEventKind uint8

const (
	WTOEventInvalid WTOEventKind = iota
	WTOEventEnter
	WTOEventPoint
	WTOEventExit
)

func (kind WTOEventKind) Valid() bool { return kind >= WTOEventEnter && kind <= WTOEventExit }

type WTOEvent struct {
	kind   WTOEventKind
	region identity.ContentID
	point  identity.ContentID
}

func (event WTOEvent) Kind() WTOEventKind           { return event.kind }
func (event WTOEvent) RegionID() identity.ContentID { return event.region }
func (event WTOEvent) PointID() identity.ContentID  { return event.point }
func (event WTOEvent) Available() bool {
	if !event.kind.Valid() {
		return false
	}
	if event.kind == WTOEventPoint {
		return !event.region.Available() && event.point.Available()
	}
	return event.region.Available() && !event.point.Available()
}

// Region is a zero-escape scalar copy of one parent-issued LocalWTO region.
type Region struct {
	id         identity.ContentID
	head       identity.ContentID
	sourceHead identity.ContentID
	parent     identity.ContentID
	cyclic     bool
	members    []identity.ContentID
}

func (region Region) ID() identity.ContentID         { return region.id }
func (region Region) Head() identity.ContentID       { return region.head }
func (region Region) SourceHead() identity.ContentID { return region.sourceHead }
func (region Region) ParentID() identity.ContentID   { return region.parent }
func (region Region) Cyclic() bool                   { return region.cyclic }
func (region Region) MemberCount() int               { return len(region.members) }
func (region Region) MemberAt(index int) (identity.ContentID, bool) {
	if !region.id.Available() || index < 0 || index >= len(region.members) {
		return identity.ContentID{}, false
	}
	return region.members[index], true
}

// Artifact is immutable after Compile succeeds. The sealed scalar is written
// only after complete deep validation; all hot availability checks are O(1).
