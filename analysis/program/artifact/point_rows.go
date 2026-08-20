package artifact

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/causal"
	"github.com/wippyai/go-lua/analysis/schema/program"
)

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
	arm       causal.BoundaryArmKind
	guarded   bool
	truth     bool
	hasReset  bool
	mu        identity.ContentID
	hasMu     bool
}

func (edge EnvironmentEdge) ID() identity.ContentID      { return edge.id }
func (edge EnvironmentEdge) From() identity.ContentID    { return edge.from }
func (edge EnvironmentEdge) To() identity.ContentID      { return edge.to }
func (edge EnvironmentEdge) RouteID() identity.ContentID { return edge.route }
func (edge EnvironmentEdge) Arm() causal.BoundaryArmKind { return edge.arm }

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
	id      identity.ContentID
	parent  identity.ContentID
	cyclic  bool
	members []identity.ContentID
}

func (region Region) ID() identity.ContentID { return region.id }
func (region Region) Head() identity.ContentID {
	if !region.id.Available() || len(region.members) == 0 {
		return identity.ContentID{}
	}
	return region.members[0]
}
func (region Region) ParentID() identity.ContentID { return region.parent }
func (region Region) Cyclic() bool                 { return region.cyclic }
func (region Region) MemberCount() int             { return len(region.members) }
func (region Region) MemberAt(index int) (identity.ContentID, bool) {
	if !region.id.Available() || index < 0 || index >= len(region.members) {
		return identity.ContentID{}, false
	}
	return region.members[index], true
}

func (artifact *Artifact) PointCount() int {
	if !artifact.Available() {
		return 0
	}
	count, published := coldCount(artifact, programschema.PointFamily())
	if !published {
		return 0
	}
	return count
}

func (artifact *Artifact) EnvironmentEdgeCount() int {
	if !artifact.Available() {
		return 0
	}
	count, published := coldCount(artifact, programschema.EnvironmentEdgeFamily())
	if !published {
		return 0
	}
	return count
}

func (artifact *Artifact) RegionCount() int {
	if !artifact.Available() {
		return 0
	}
	count, published := coldCount(artifact, programschema.RegionFamily())
	if !published {
		return 0
	}
	return count
}

func (artifact *Artifact) WTOEventCount() int {
	if !artifact.Available() {
		return 0
	}
	count, published := coldCount(artifact, programschema.WTOEventFamily())
	if !published {
		return 0
	}
	return count
}

func (artifact *Artifact) PointAt(index int) (Point, bool) {
	if !artifact.Available() {
		return Point{}, false
	}
	return artifact.pointRowAt(index)
}

// pointRowAt reads one point out of the sealed publication and rejoins it
// with the decision span it names. Decisions are a dense plane there, so the
// ordered geometry a caller receives is assembled at the read site rather
// than retained a second time beside the publication.
func (artifact *Artifact) pointRowAt(index int) (Point, bool) {
	sealed, held := coldRow(artifact, programschema.PointFamily(), index)
	offset, count, spanOK := sealed.DecisionSpan()
	if !held || !spanOK {
		return Point{}, false
	}
	row := Point{id: sealed.ID(), initial: sealed.Initial()}
	if count != 0 {
		row.decisions = make([]identity.ContentID, 0, count)
	}
	for position := uint32(0); position < count; position++ {
		decision, decisionHeld := coldRow(artifact, programschema.PointDecisionFamily(), int(offset+position))
		if !decisionHeld {
			return Point{}, false
		}
		row.decisions = append(row.decisions, decision.ID())
	}
	return row, row.Available()
}

func (artifact *Artifact) EnvironmentEdgeAt(index int) (EnvironmentEdge, bool) {
	if !artifact.Available() {
		return EnvironmentEdge{}, false
	}
	return artifact.environmentEdgeRowAt(index)
}

// environmentEdgeRowAt reads one final route out of the sealed publication and
// rejoins it with the reset span it names. Witnesses are a dense plane there,
// so the ordered geometry a caller receives is assembled at the read site
// rather than retained a second time beside the publication.
func (artifact *Artifact) environmentEdgeRowAt(index int) (EnvironmentEdge, bool) {
	sealed, held := coldRow(artifact, programschema.EnvironmentEdgeFamily(), index)
	offset, count, spanOK := sealed.ResetSpan()
	if !held || !spanOK {
		return EnvironmentEdge{}, false
	}
	guard, guarded := sealed.GuardID()
	decision, _ := sealed.DecisionID()
	condition, _ := sealed.ConditionValueSpanID()
	truth, _ := sealed.Truth()
	mu, hasMu := sealed.MuPathID()
	reset, hasReset := sealed.ResetDigest()
	row := EnvironmentEdge{
		id: sealed.ID(), from: sealed.From(), to: sealed.To(), route: sealed.RouteID(),
		guard: guard, decision: decision, condition: condition, reset: reset,
		component: sealed.ComponentID(), arm: causal.BoundaryArmKind(sealed.Arm()),
		guarded: guarded, truth: truth, hasReset: hasReset, mu: mu, hasMu: hasMu,
	}
	if count != 0 {
		row.resets = make([]identity.ContentID, 0, count)
	}
	for position := uint32(0); position < count; position++ {
		witness, witnessHeld := coldRow(artifact, programschema.EnvironmentResetFamily(), int(offset+position))
		if !witnessHeld {
			return EnvironmentEdge{}, false
		}
		row.resets = append(row.resets, witness.ID())
	}
	return row, row.Available()
}

func (artifact *Artifact) RegionAt(index int) (Region, bool) {
	if !artifact.Available() {
		return Region{}, false
	}
	return artifact.regionRowAt(index)
}

// regionRowAt reads one region out of the sealed publication and rejoins it
// with the member span it names. Members are a dense plane there, so the
// ordered geometry a caller receives is assembled at the read site rather
// than retained a second time beside the publication.
func (artifact *Artifact) regionRowAt(index int) (Region, bool) {
	sealed, held := coldRow(artifact, programschema.RegionFamily(), index)
	offset, count, spanOK := sealed.MemberSpan()
	if !held || !spanOK {
		return Region{}, false
	}
	row := Region{id: sealed.ID(), parent: sealed.ParentID(), cyclic: sealed.Cyclic(), members: make([]identity.ContentID, 0, count)}
	for position := uint32(0); position < count; position++ {
		member, memberHeld := coldRow(artifact, programschema.RegionMemberFamily(), int(offset+position))
		if !memberHeld {
			return Region{}, false
		}
		row.members = append(row.members, member.ID())
	}
	return row, sealed.Available()
}

func (artifact *Artifact) WTOEventAt(index int) (WTOEvent, bool) {
	if !artifact.Available() {
		return WTOEvent{}, false
	}
	return artifact.wtoEventRowAt(index)
}

// wtoEventRowAt reads one order bracket out of the sealed publication. The
// row is flat there, so the read is a change of vocabulary and no plane is
// retained beside the publication.
func (artifact *Artifact) wtoEventRowAt(index int) (WTOEvent, bool) {
	sealed, held := coldRow(artifact, programschema.WTOEventFamily(), index)
	if !held {
		return WTOEvent{}, false
	}
	row := WTOEvent{kind: WTOEventKind(sealed.Kind()), region: sealed.RegionID(), point: sealed.PointID()}
	return row, row.Available()
}

// Artifact is immutable after Compile succeeds. The sealed scalar is written
// only after complete deep validation; all hot availability checks are O(1).
