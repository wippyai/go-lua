package compiler

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/causal"
)

// pointDraft is an exact parent-issued LocalWTO phase vertex path. The base
// row owns the final decision vector for decisionScope; synthetic stage rows
// retain only that scope reference, so Link never has to reopen Program or
// copy point geometry to recover a stage's logical decisions.
type pointDraft struct {
	id            identity.ContentID
	decisionScope identity.ContentID
	decisions     []identity.ContentID
	initial       bool
}

func (point pointDraft) ID() identity.ContentID { return point.id }
func (point pointDraft) Available() bool {
	return point.id.Available() && point.decisionScope.Available()
}
func (point pointDraft) DecisionCount() int {
	if !point.Available() {
		return 0
	}
	return len(point.decisions)
}
func (point pointDraft) DecisionAt(index int) (identity.ContentID, bool) {
	if !point.Available() || index < 0 || index >= len(point.decisions) {
		return identity.ContentID{}, false
	}
	return point.decisions[index], true
}
func (point pointDraft) Initial() (bool, bool) { return point.initial, point.Available() }

// environmentEdgeDraft is a scalar copy of one exact canonical Flow final route.
// Its ID is the stable route-occurrence identity. RouteID is the parent
// final-route semantic ID and is not a second artifact identity.
type environmentEdgeDraft struct {
	id identity.ContentID
	// from and to are where the route comes from and goes to. They are the
	// edge's identity and never move, which is what lets the seal authenticate
	// a routed occurrence against the route that carries it.
	//
	// departure is where the state travelling this route actually leaves from:
	// the source's terminal stage once the source is staged, and the route's
	// own predecessor stage where one stands on it. Separating the two is what
	// lets a fact this transfer proves be staged on the transfer itself.
	from      identity.ContentID
	departure identity.ContentID
	to        identity.ContentID
	route     identity.ContentID
	guard     identity.ContentID
	decision  identity.ContentID
	condition identity.ContentID
	reset     identity.ContentID
	resets    []identity.ContentID
	component identity.ContentID
	arm       causal.BoundaryArmKind
	guarded   bool
	truth     bool
	hasReset  bool
	mu        identity.ContentID
	hasMu     bool
}

func (edge environmentEdgeDraft) ID() identity.ContentID { return edge.id }

// Departure is the point the transfer's state leaves from: From until the
// source carries stages, and the source's terminal stage afterwards.
func (edge environmentEdgeDraft) Departure() identity.ContentID {
	if edge.departure.Available() {
		return edge.departure
	}
	return edge.from
}
func (edge environmentEdgeDraft) From() identity.ContentID    { return edge.from }
func (edge environmentEdgeDraft) To() identity.ContentID      { return edge.to }
func (edge environmentEdgeDraft) RouteID() identity.ContentID { return edge.route }
func (edge environmentEdgeDraft) Arm() causal.BoundaryArmKind { return edge.arm }

// DecisionID is the parent-issued decision Site identity for a guarded
// route. It is distinct from GuardID: GuardID authenticates the guard proof,
// while DecisionID is the coordinate needed to issue the exact Link-local
// guard expression. Unguarded routes deliberately have neither identity.
func (edge environmentEdgeDraft) DecisionID() (identity.ContentID, bool) {
	return edge.decision, edge.Available() && edge.guarded
}

func (edge environmentEdgeDraft) GuardID() (identity.ContentID, bool) {
	return edge.guard, edge.Available() && edge.guarded
}

// ConditionValueSpanID is the generic Program-issued branch condition value
// identity. It is absent for non-Branch guards and carries no diagnostic or
// domain meaning.
func (edge environmentEdgeDraft) ConditionValueSpanID() (identity.ContentID, bool) {
	return edge.condition, edge.Available() && edge.guarded && edge.condition.Available()
}

func (edge environmentEdgeDraft) Truth() (bool, bool) {
	return edge.truth, edge.Available() && edge.guarded
}

func (edge environmentEdgeDraft) ResetDigest() (identity.ContentID, bool) {
	return edge.reset, edge.Available() && edge.hasReset
}

func (edge environmentEdgeDraft) HasResetWitness() bool {
	return edge.Available() && edge.hasReset
}

func (edge environmentEdgeDraft) ResetCount() int {
	if !edge.Available() {
		return 0
	}
	return len(edge.resets)
}

func (edge environmentEdgeDraft) ResetAt(index int) (identity.ContentID, bool) {
	if !edge.Available() || index < 0 || index >= len(edge.resets) {
		return identity.ContentID{}, false
	}
	return edge.resets[index], true
}

func (edge environmentEdgeDraft) ComponentID() identity.ContentID {
	if !edge.Available() {
		return identity.ContentID{}
	}
	return edge.component
}

func (edge environmentEdgeDraft) MuPathID() (identity.ContentID, bool) {
	return edge.mu, edge.Available() && edge.hasMu
}

func (edge environmentEdgeDraft) Available() bool {
	if !edge.id.Available() || !edge.from.Available() || !edge.to.Available() || !edge.route.Available() || edge.arm < causal.BoundaryLocal || edge.arm > causal.BoundaryCancel {
		return false
	}
	if edge.guarded {
		if !edge.guard.Available() {
			return false
		}
	} else if edge.guard.Available() || edge.truth {
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

// wtoEventKind brackets the exact parent-issued nested weak-topological order.
type wtoEventKind uint8

const (
	wtoEventInvalid wtoEventKind = iota
	wtoEventEnter
	wtoEventPoint
	wtoEventExit
)

func (kind wtoEventKind) Valid() bool { return kind >= wtoEventEnter && kind <= wtoEventExit }

type wtoEventDraft struct {
	kind   wtoEventKind
	region identity.ContentID
	point  identity.ContentID
}

func (event wtoEventDraft) Kind() wtoEventKind           { return event.kind }
func (event wtoEventDraft) RegionID() identity.ContentID { return event.region }
func (event wtoEventDraft) PointID() identity.ContentID  { return event.point }
func (event wtoEventDraft) Available() bool {
	if !event.kind.Valid() {
		return false
	}
	if event.kind == wtoEventPoint {
		return !event.region.Available() && event.point.Available()
	}
	return event.region.Available() && !event.point.Available()
}

// regionDraft is a zero-escape scalar copy of one parent-issued LocalWTO region.
type regionDraft struct {
	id      identity.ContentID
	parent  identity.ContentID
	cyclic  bool
	members []identity.ContentID
}

func (region regionDraft) ID() identity.ContentID { return region.id }
func (region regionDraft) Head() identity.ContentID {
	if !region.id.Available() || len(region.members) == 0 {
		return identity.ContentID{}
	}
	return region.members[0]
}
func (region regionDraft) ParentID() identity.ContentID { return region.parent }
func (region regionDraft) Cyclic() bool                 { return region.cyclic }
func (region regionDraft) MemberCount() int             { return len(region.members) }
func (region regionDraft) MemberAt(index int) (identity.ContentID, bool) {
	if !region.id.Available() || index < 0 || index >= len(region.members) {
		return identity.ContentID{}, false
	}
	return region.members[index], true
}
