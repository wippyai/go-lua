// Package artifactingress owns the compiler boundary between reusable Program
// artifacts and the analysis engine. Snapshot is a borrowed read-only view:
// after Lower succeeds, its artifact pointer keeps the already sealed,
// immutable ProgramArtifact alive for the duration of the same process.
package artifactingress

import (
	"github.com/wippyai/go-lua/analysis/internal/programartifact"
	"github.com/wippyai/go-lua/program/keyspace"
)

// StructuralArm is the neutral structural-edge arm vocabulary.
type StructuralArm uint8

const (
	StructuralArmInvalid StructuralArm = iota
	StructuralArmLocal
	StructuralArmResume
	StructuralArmTrue
	StructuralArmFalse
	StructuralArmTail
	StructuralArmThrow
	StructuralArmYield
	StructuralArmCancel
)

func (arm StructuralArm) Valid() bool { return arm >= StructuralArmLocal && arm <= StructuralArmCancel }

// EventKind is the neutral bracket-stream vocabulary.
type EventKind uint8

const (
	EventInvalid EventKind = iota
	EventEnter
	EventPoint
	EventExit
)

// Snapshot is the smallest immutable ingress receipt. It retains only the
// sealed artifact pointer; all row accessors borrow the artifact's immutable
// storage by index. The pointer also gives Go's GC the lifetime proof needed
// for the borrowed rows, so no source Program or mutable owner is retained.
type Snapshot struct{ artifact *programartifact.Artifact }

// Available is intentionally only an authority fence. ProgramArtifact seal
// has already checked every row and cross-plane reference before publishing
// the artifact, so ingress must not repeat that validation.
func (snapshot *Snapshot) Available() bool {
	return snapshot != nil && artifactAuthority(snapshot.artifact)
}

func artifactAuthority(artifact *programartifact.Artifact) bool {
	if artifact == nil || !artifact.Available() {
		return false
	}
	key := artifact.CompileKey()
	return artifact.ID().Available() && key.ProgramID().Available() && key.SchemaDigest().Available() && artifact.PointCount() != 0
}

func (snapshot *Snapshot) ArtifactID() keyspace.ContentID {
	if !snapshot.Available() {
		return keyspace.ContentID{}
	}
	return snapshot.artifact.ID()
}
func (snapshot *Snapshot) ProgramID() keyspace.ContentID {
	if !snapshot.Available() {
		return keyspace.ContentID{}
	}
	return snapshot.artifact.CompileKey().ProgramID()
}
func (snapshot *Snapshot) SchemaID() keyspace.ContentID {
	if !snapshot.Available() {
		return keyspace.ContentID{}
	}
	return snapshot.artifact.CompileKey().SchemaDigest()
}
func (snapshot *Snapshot) PointCount() int {
	if !snapshot.Available() {
		return 0
	}
	return snapshot.artifact.PointCount()
}
func (snapshot *Snapshot) PointAt(index int) (Point, bool) {
	if !snapshot.Available() || index < 0 || index >= snapshot.artifact.PointCount() {
		return Point{}, false
	}
	row, ok := snapshot.artifact.PointAt(index)
	return Point{Point: row}, ok
}
func (snapshot *Snapshot) StructuralEdgeCount() int {
	if !snapshot.Available() {
		return 0
	}
	return snapshot.artifact.EnvironmentEdgeCount()
}
func (snapshot *Snapshot) StructuralEdgeAt(index int) (StructuralEdge, bool) {
	if !snapshot.Available() || index < 0 || index >= snapshot.artifact.EnvironmentEdgeCount() {
		return StructuralEdge{}, false
	}
	row, ok := snapshot.artifact.EnvironmentEdgeAt(index)
	return StructuralEdge{EnvironmentEdge: row}, ok
}
func (snapshot *Snapshot) LocalTransferCount() int {
	if !snapshot.Available() {
		return 0
	}
	return snapshot.artifact.LocalTransferCount()
}
func (snapshot *Snapshot) LocalTransferAt(index int) (LocalTransfer, bool) {
	if !snapshot.Available() || index < 0 || index >= snapshot.artifact.LocalTransferCount() {
		return LocalTransfer{}, false
	}
	row, ok := snapshot.artifact.LocalTransferAt(index)
	return LocalTransfer{LocalTransfer: row}, ok
}
func (snapshot *Snapshot) RegionCount() int {
	if !snapshot.Available() {
		return 0
	}
	return snapshot.artifact.RegionCount()
}
func (snapshot *Snapshot) RegionAt(index int) (Region, bool) {
	if !snapshot.Available() || index < 0 || index >= snapshot.artifact.RegionCount() {
		return Region{}, false
	}
	row, ok := snapshot.artifact.RegionAt(index)
	return Region{Region: row}, ok
}
func (snapshot *Snapshot) EventCount() int {
	if !snapshot.Available() {
		return 0
	}
	return snapshot.artifact.WTOEventCount()
}
func (snapshot *Snapshot) EventAt(index int) (Event, bool) {
	if !snapshot.Available() || index < 0 || index >= snapshot.artifact.WTOEventCount() {
		return Event{}, false
	}
	row, ok := snapshot.artifact.WTOEventAt(index)
	return Event{WTOEvent: row}, ok
}

func (snapshot *Snapshot) RulePlacementCount() int {
	if !snapshot.Available() {
		return 0
	}
	count := 0
	for index := 0; index < programartifact.MountedRuleRoleCount(); index++ {
		role, ok := programartifact.MountedRuleRoleAt(index)
		if !ok {
			return 0
		}
		count += snapshot.artifact.RuleOccurrenceCount(role)
	}
	return count
}
func (snapshot *Snapshot) RulePlacementAt(index int) (RulePlacement, bool) {
	if !snapshot.Available() || index < 0 {
		return RulePlacement{}, false
	}
	for roleIndex := 0; roleIndex < programartifact.MountedRuleRoleCount(); roleIndex++ {
		role, ok := programartifact.MountedRuleRoleAt(roleIndex)
		if !ok {
			return RulePlacement{}, false
		}
		count := snapshot.artifact.RuleOccurrenceCount(role)
		if index < count {
			row, ok := snapshot.artifact.RuleOccurrenceAt(role, index)
			return RulePlacement{RuleOccurrenceRow: row}, ok
		}
		index -= count
	}
	return RulePlacement{}, false
}
func (snapshot *Snapshot) BodyTransportCount() int {
	if !snapshot.Available() {
		return 0
	}
	return snapshot.artifact.BodyCount()
}
func (snapshot *Snapshot) BodyTransportAt(index int) (BodyTransport, bool) {
	if !snapshot.Available() || index < 0 || index >= snapshot.artifact.BodyCount() {
		return BodyTransport{}, false
	}
	row, ok := snapshot.artifact.BodyAt(index)
	return BodyTransport{BodyRow: row, artifact: snapshot.artifact, bodyIndex: index}, ok
}
func (snapshot *Snapshot) FunctionBoundaryCount() int {
	if !snapshot.Available() {
		return 0
	}
	return snapshot.artifact.FunctionBoundaryCount()
}
func (snapshot *Snapshot) FunctionBoundaryAt(index int) (FunctionBoundary, bool) {
	if !snapshot.Available() || index < 0 || index >= snapshot.artifact.FunctionBoundaryCount() {
		return FunctionBoundary{}, false
	}
	row, ok := snapshot.artifact.FunctionBoundaryAt(index)
	return FunctionBoundary{FunctionBoundaryRow: row}, ok
}

// Point, StructuralEdge, LocalTransfer, Region, Event, and RulePlacement
// embed immutable artifact rows by value. These are shallow descriptor copies
// (their private variable-length storage remains owned by Artifact), not new
// ingress-owned row storage.
type Point struct{ programartifact.Point }

func (row Point) Initial() bool {
	initial, ok := row.Point.Initial()
	return ok && initial
}

type StructuralEdge struct {
	programartifact.EnvironmentEdge
}

func (row StructuralEdge) Arm() StructuralArm {
	arm, ok := structuralArm(row.EnvironmentEdge.Arm())
	if !ok {
		return StructuralArmInvalid
	}
	return arm
}

type LocalTransfer struct{ programartifact.LocalTransfer }

func (row LocalTransfer) Full() bool { return row.LocalTransfer.FullEnvironment() }
func (row LocalTransfer) TagCount() int {
	return row.LocalTransfer.FactorRoleCount()
}
func (row LocalTransfer) TagAt(index int) (uint8, bool) {
	role, ok := row.LocalTransfer.FactorRoleAt(index)
	if !ok {
		return 0, false
	}
	return uint8(role), true
}

type Region struct{ programartifact.Region }

type Event struct{ programartifact.WTOEvent }

func (row Event) Kind() EventKind {
	kind, ok := eventKind(row.WTOEvent.Kind())
	if !ok {
		return EventInvalid
	}
	return kind
}

// RulePlacement is a role-specific projection over the artifact's canonical
// occurrence catalog. It stores no copied placement or point list.
type RulePlacement struct {
	programartifact.RuleOccurrenceRow
}

// FunctionBoundary is a borrowed view over ProgramArtifact's neutral formal
// interface. It adds no row storage and exposes no live Program/Flow owner.
type FunctionBoundary struct {
	programartifact.FunctionBoundaryRow
}

func (row RulePlacement) Tag() uint8 { return uint8(row.RuleOccurrenceRow.Role()) }
func (row RulePlacement) Stage() uint8 {
	return uint8(row.RuleOccurrenceRow.Stage())
}
func (row RulePlacement) PointID() keyspace.ContentID {
	point, ok := row.RuleOccurrenceRow.PointAt(0)
	if !ok {
		return keyspace.ContentID{}
	}
	return point
}
func (row RulePlacement) InputPointID() keyspace.ContentID {
	point, _ := row.RuleOccurrenceRow.InputPoint()
	return point
}
func (row RulePlacement) OccurrenceID() keyspace.ContentID { return row.RuleOccurrenceRow.ID() }
func (row RulePlacement) PredecessorRouteID() keyspace.ContentID {
	route, _ := row.RuleOccurrenceRow.PredecessorRouteID()
	return route
}

// BodyTransport is a borrowed body view. Entry points come directly from the
// Body row; exits are the ingress projection of accepted Outcome point sets
// and are deduplicated on demand in their original artifact order.
type BodyTransport struct {
	programartifact.BodyRow
	artifact  *programartifact.Artifact
	bodyIndex int
}

func (row BodyTransport) BodyID() keyspace.ContentID          { return row.BodyRow.ID() }
func (row BodyTransport) ContextID() keyspace.ContentID       { return row.BodyRow.ContextID() }
func (row BodyTransport) SemanticEntryID() keyspace.ContentID { return row.BodyRow.EntryID() }
func (row BodyTransport) Callable() bool                      { return row.BodyRow.Callable() }
func (row BodyTransport) FunctionID() keyspace.ContentID {
	id, _ := row.BodyRow.FunctionContextID()
	return id
}
func (row BodyTransport) CallFormalID() keyspace.ContentID {
	id, _ := row.BodyRow.CallFormalID()
	return id
}
func (row BodyTransport) EntryCount() int { return row.BodyRow.EntryPointCount() }
func (row BodyTransport) EntryAt(index int) (keyspace.ContentID, bool) {
	return row.BodyRow.EntryPointAt(index)
}
func (row BodyTransport) ExitCount() int {
	count, ok := row.exitCount()
	if !ok {
		return 0
	}
	return count
}
func (row BodyTransport) ExitAt(index int) (keyspace.ContentID, bool) {
	if index < 0 || row.artifact == nil || !row.BodyRow.Available() {
		return keyspace.ContentID{}, false
	}
	seen := make(map[keyspace.ContentID]struct{})
	next := 0
	for outcomeIndex := 0; outcomeIndex < row.BodyRow.OutcomeCount(); outcomeIndex++ {
		outcome, ok := row.artifact.BodyOutcomeAt(row.bodyIndex, outcomeIndex)
		if !ok {
			return keyspace.ContentID{}, false
		}
		if !acceptedOutcome(outcome.Kind()) {
			continue
		}
		for pointIndex := 0; pointIndex < outcome.PointCount(); pointIndex++ {
			point, ok := outcome.PointAt(pointIndex)
			if !ok {
				return keyspace.ContentID{}, false
			}
			if _, duplicate := seen[point]; duplicate {
				continue
			}
			seen[point] = struct{}{}
			if next == index {
				return point, true
			}
			next++
		}
	}
	return keyspace.ContentID{}, false
}

func (row BodyTransport) exitCount() (int, bool) {
	if row.artifact == nil || !row.BodyRow.Available() {
		return 0, false
	}
	seen := make(map[keyspace.ContentID]struct{})
	count := 0
	for outcomeIndex := 0; outcomeIndex < row.BodyRow.OutcomeCount(); outcomeIndex++ {
		outcome, ok := row.artifact.BodyOutcomeAt(row.bodyIndex, outcomeIndex)
		if !ok {
			return 0, false
		}
		if !acceptedOutcome(outcome.Kind()) {
			continue
		}
		for pointIndex := 0; pointIndex < outcome.PointCount(); pointIndex++ {
			point, ok := outcome.PointAt(pointIndex)
			if !ok {
				return 0, false
			}
			if _, duplicate := seen[point]; duplicate {
				continue
			}
			seen[point] = struct{}{}
			count++
		}
	}
	return count, true
}

// Lower only admits an already sealed artifact and the closed ingress role
// catalog. All row/reference validation belongs to ProgramArtifact sealing;
// this boundary adds no second owned representation of those rows.
func Lower(artifact *programartifact.Artifact) (*Snapshot, bool) {
	if !artifactAuthority(artifact) {
		return nil, false
	}
	for index := 0; index < programartifact.MountedRuleRoleCount(); index++ {
		role, ok := programartifact.MountedRuleRoleAt(index)
		if !ok {
			return nil, false
		}
		if !artifact.RuleRoleSupported(role) {
			return nil, false
		}
	}
	return &Snapshot{artifact: artifact}, true
}

func structuralArm(arm programartifact.RouteKind) (StructuralArm, bool) {
	switch arm {
	case programartifact.RouteLocal:
		return StructuralArmLocal, true
	case programartifact.RouteResume:
		return StructuralArmResume, true
	case programartifact.RouteSelectTrue:
		return StructuralArmTrue, true
	case programartifact.RouteSelectFalse:
		return StructuralArmFalse, true
	case programartifact.RouteTail:
		return StructuralArmTail, true
	case programartifact.RouteThrow:
		return StructuralArmThrow, true
	case programartifact.RouteYield:
		return StructuralArmYield, true
	case programartifact.RouteCancel:
		return StructuralArmCancel, true
	default:
		return StructuralArmInvalid, false
	}
}

func eventKind(kind programartifact.WTOEventKind) (EventKind, bool) {
	switch kind {
	case programartifact.WTOEventEnter:
		return EventEnter, true
	case programartifact.WTOEventPoint:
		return EventPoint, true
	case programartifact.WTOEventExit:
		return EventExit, true
	default:
		return EventInvalid, false
	}
}

func acceptedOutcome(kind programartifact.OutcomeKind) bool {
	switch kind {
	case programartifact.OutcomeNormal, programartifact.OutcomeReturn, programartifact.OutcomeThrow, programartifact.OutcomeYield, programartifact.OutcomeCancel:
		return true
	default:
		return false
	}
}
