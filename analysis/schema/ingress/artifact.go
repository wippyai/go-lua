// Package ingress lowers a sealed ProgramArtifact into engine template rows
// exactly once per ContentID. Snapshot is a borrowed read-only view: after
// Lower succeeds, its artifact pointer keeps the already sealed, immutable
// ProgramArtifact alive for the duration of the same process.
package ingress

import (
	"github.com/wippyai/go-lua/analysis/identity"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

// StructuralArm and EventKind are the boundary spellings of two sealed
// structural vocabularies. Their ordinals are the vocabulary's declared
// ordinals, pinned there by the declaration surface's own laws, so a projection
// below is a lookup of the sealed member rather than a translation between two
// catalogs.

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

// Snapshot is the smallest immutable ingress receipt. It retains the sealed
// artifact pointer and the sealed structural vocabulary it projects rows
// through; all row accessors borrow the artifact's immutable storage by index,
// and the vocabulary is the declaration table's own immutable projection rather
// than ingress-owned row storage. The artifact pointer also gives Go's GC the
// lifetime proof needed for the borrowed rows, so no source Program or mutable
// owner is retained.
type Snapshot struct {
	artifact   *programartifact.Artifact
	vocabulary structure.Table
}

// Available is intentionally only the borrow fence. Lower admitted the sealed
// artifact and the structural vocabulary once, and ProgramArtifact seal had
// already checked every row and cross-plane reference before publishing the
// artifact, so an accessor reads borrowed storage behind this fence instead of
// re-deriving either.
func (snapshot *Snapshot) Available() bool {
	return snapshot != nil && snapshot.artifact != nil && vocabularyAuthority(snapshot.vocabulary)
}

// vocabularyAuthority is the second half of the fence: ingress projects arms,
// events, and body outcomes, so a vocabulary missing any of those catalogs is
// no authority for this boundary. Density and totality within a catalog are the
// declaration surface's laws, already stated at seal.
func vocabularyAuthority(vocabulary structure.Table) bool {
	return vocabulary.Count(structure.CategoryArm) != 0 &&
		vocabulary.Count(structure.CategoryEvent) != 0 &&
		vocabulary.Count(structure.CategoryOutcome) != 0
}

// artifactAuthority is the construction fence: an artifact is an authority for
// this boundary exactly when it carries its own seal proof. Its identity,
// compile key, and point population are established by that seal and are not
// re-derived here.
func artifactAuthority(artifact *programartifact.Artifact) bool {
	return artifact != nil && artifact.Available()
}

func (snapshot *Snapshot) ArtifactID() identity.ContentID {
	if !snapshot.Available() {
		return identity.ContentID{}
	}
	return snapshot.artifact.ID()
}
func (snapshot *Snapshot) ProgramID() identity.ContentID {
	if !snapshot.Available() {
		return identity.ContentID{}
	}
	return snapshot.artifact.CompileKey().ProgramID()
}
func (snapshot *Snapshot) SchemaID() identity.ContentID {
	if !snapshot.Available() {
		return identity.ContentID{}
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
	return StructuralEdge{EnvironmentEdge: row, vocabulary: &snapshot.vocabulary}, ok
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
	return Event{WTOEvent: row, vocabulary: &snapshot.vocabulary}, ok
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
	return BodyTransport{BodyRow: row, artifact: snapshot.artifact, vocabulary: &snapshot.vocabulary, bodyIndex: index}, ok
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
	vocabulary *structure.Table
}

// Arm resolves the artifact row's arm ordinal against the sealed arm
// vocabulary. An ordinal outside the declared catalog names no member, so it
// yields the invalid arm rather than a member this boundary invented.
func (row StructuralEdge) Arm() StructuralArm {
	if row.vocabulary == nil {
		return StructuralArmInvalid
	}
	member, ok := row.vocabulary.At(structure.CategoryArm, uint16(row.EnvironmentEdge.Arm()))
	if !ok {
		return StructuralArmInvalid
	}
	return StructuralArm(member.Ordinal())
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

type Event struct {
	programartifact.WTOEvent
	vocabulary *structure.Table
}

// Kind resolves the artifact row's bracket ordinal against the sealed event
// vocabulary.
func (row Event) Kind() EventKind {
	if row.vocabulary == nil {
		return EventInvalid
	}
	member, ok := row.vocabulary.At(structure.CategoryEvent, uint16(row.WTOEvent.Kind()))
	if !ok {
		return EventInvalid
	}
	return EventKind(member.Ordinal())
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
func (row RulePlacement) PointID() identity.ContentID {
	point, ok := row.RuleOccurrenceRow.PointAt(0)
	if !ok {
		return identity.ContentID{}
	}
	return point
}
func (row RulePlacement) InputPointID() identity.ContentID {
	point, _ := row.RuleOccurrenceRow.InputPoint()
	return point
}
func (row RulePlacement) OccurrenceID() identity.ContentID { return row.RuleOccurrenceRow.ID() }
func (row RulePlacement) PredecessorRouteID() identity.ContentID {
	route, _ := row.RuleOccurrenceRow.PredecessorRouteID()
	return route
}

// BodyTransport is a borrowed body view. Entry points come directly from the
// Body row; exits are the ingress projection of accepted Outcome point sets
// and are deduplicated on demand in their original artifact order. Which
// outcomes are accepted is the sealed vocabulary's declared property, read row
// by row.
type BodyTransport struct {
	programartifact.BodyRow
	artifact   *programartifact.Artifact
	vocabulary *structure.Table
	bodyIndex  int
}

// accepted resolves one artifact outcome ordinal against the sealed outcome
// vocabulary and reads the member's declared admission into this projection.
func (row BodyTransport) accepted(kind programartifact.OutcomeKind) bool {
	if row.vocabulary == nil {
		return false
	}
	member, ok := row.vocabulary.At(structure.CategoryOutcome, uint16(kind))
	return ok && member.Accepted()
}

func (row BodyTransport) BodyID() identity.ContentID          { return row.BodyRow.ID() }
func (row BodyTransport) ContextID() identity.ContentID       { return row.BodyRow.ContextID() }
func (row BodyTransport) SemanticEntryID() identity.ContentID { return row.BodyRow.EntryID() }
func (row BodyTransport) Callable() bool                      { return row.BodyRow.Callable() }
func (row BodyTransport) FunctionID() identity.ContentID {
	id, _ := row.BodyRow.FunctionContextID()
	return id
}
func (row BodyTransport) CallFormalID() identity.ContentID {
	id, _ := row.BodyRow.CallFormalID()
	return id
}
func (row BodyTransport) EntryCount() int { return row.BodyRow.EntryPointCount() }
func (row BodyTransport) EntryAt(index int) (identity.ContentID, bool) {
	return row.BodyRow.EntryPointAt(index)
}
func (row BodyTransport) ExitCount() int {
	count, ok := row.exitCount()
	if !ok {
		return 0
	}
	return count
}
func (row BodyTransport) ExitAt(index int) (identity.ContentID, bool) {
	if index < 0 || row.artifact == nil || !row.BodyRow.Available() {
		return identity.ContentID{}, false
	}
	seen := make(map[identity.ContentID]struct{})
	next := 0
	for outcomeIndex := 0; outcomeIndex < row.BodyRow.OutcomeCount(); outcomeIndex++ {
		outcome, ok := row.artifact.BodyOutcomeAt(row.bodyIndex, outcomeIndex)
		if !ok {
			return identity.ContentID{}, false
		}
		if !row.accepted(outcome.Kind()) {
			continue
		}
		for pointIndex := 0; pointIndex < outcome.PointCount(); pointIndex++ {
			point, ok := outcome.PointAt(pointIndex)
			if !ok {
				return identity.ContentID{}, false
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
	return identity.ContentID{}, false
}

func (row BodyTransport) exitCount() (int, bool) {
	if row.artifact == nil || !row.BodyRow.Available() {
		return 0, false
	}
	seen := make(map[identity.ContentID]struct{})
	count := 0
	for outcomeIndex := 0; outcomeIndex < row.BodyRow.OutcomeCount(); outcomeIndex++ {
		outcome, ok := row.artifact.BodyOutcomeAt(row.bodyIndex, outcomeIndex)
		if !ok {
			return 0, false
		}
		if !row.accepted(outcome.Kind()) {
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

// Lower only admits an already sealed artifact, the sealed structural
// vocabulary its rows are projected through, and the closed ingress role
// catalog. All row/reference validation belongs to ProgramArtifact sealing;
// this boundary adds no second owned representation of those rows. The
// vocabulary is handed in by the composition rather than reached for here, so
// the boundary reads one sealed declaration and never a catalog of its own.
func Lower(artifact *programartifact.Artifact, vocabulary structure.Table) (*Snapshot, bool) {
	if !artifactAuthority(artifact) || !vocabularyAuthority(vocabulary) {
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
	return &Snapshot{artifact: artifact, vocabulary: vocabulary}, true
}
