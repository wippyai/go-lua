// Package ingress lowers a sealed ProgramArtifact into closed immutable
// columns exactly once per ContentID. After Lower succeeds the snapshot
// retains no owner pointer and cannot reopen artifact interiors.
package ingress

import (
	"github.com/wippyai/go-lua/analysis/identity"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/schema"
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

// Snapshot is the sealed ingress receipt: closed identity columns projected
// once from a ProgramArtifact. Accessors read these columns; they never hold
// or reopen the owner.
type Snapshot struct {
	artifactID identity.ContentID
	programID  identity.ContentID
	schemaID   identity.ContentID
	vocabulary structure.Table
	points     []Point
	edges      []StructuralEdge
	transfers  []LocalTransfer
	regions    []Region
	events     []Event
	placements []RulePlacement
	bodies     []BodyTransport
	boundaries []FunctionBoundary
}

func (snapshot *Snapshot) Available() bool {
	return snapshot != nil && snapshot.artifactID.Available() && snapshot.programID.Available() && snapshot.schemaID.Available() && vocabularyAuthority(snapshot.vocabulary)
}

func vocabularyAuthority(vocabulary structure.Table) bool {
	return vocabulary.Count(structure.CategoryArm) != 0 &&
		vocabulary.Count(structure.CategoryEvent) != 0 &&
		vocabulary.Count(structure.CategoryOutcome) != 0
}

func artifactAuthority(artifact *programartifact.Artifact) bool {
	return artifact != nil && artifact.Available()
}

func (snapshot *Snapshot) ArtifactID() identity.ContentID {
	if !snapshot.Available() {
		return identity.ContentID{}
	}
	return snapshot.artifactID
}
func (snapshot *Snapshot) ProgramID() identity.ContentID {
	if !snapshot.Available() {
		return identity.ContentID{}
	}
	return snapshot.programID
}
func (snapshot *Snapshot) SchemaID() identity.ContentID {
	if !snapshot.Available() {
		return identity.ContentID{}
	}
	return snapshot.schemaID
}
func (snapshot *Snapshot) PointCount() int {
	if !snapshot.Available() {
		return 0
	}
	return len(snapshot.points)
}
func (snapshot *Snapshot) PointAt(index int) (Point, bool) {
	if !snapshot.Available() || index < 0 || index >= len(snapshot.points) {
		return Point{}, false
	}
	return snapshot.points[index], true
}
func (snapshot *Snapshot) StructuralEdgeCount() int {
	if !snapshot.Available() {
		return 0
	}
	return len(snapshot.edges)
}
func (snapshot *Snapshot) StructuralEdgeAt(index int) (StructuralEdge, bool) {
	if !snapshot.Available() || index < 0 || index >= len(snapshot.edges) {
		return StructuralEdge{}, false
	}
	return snapshot.edges[index], true
}
func (snapshot *Snapshot) LocalTransferCount() int {
	if !snapshot.Available() {
		return 0
	}
	return len(snapshot.transfers)
}
func (snapshot *Snapshot) LocalTransferAt(index int) (LocalTransfer, bool) {
	if !snapshot.Available() || index < 0 || index >= len(snapshot.transfers) {
		return LocalTransfer{}, false
	}
	return snapshot.transfers[index], true
}
func (snapshot *Snapshot) RegionCount() int {
	if !snapshot.Available() {
		return 0
	}
	return len(snapshot.regions)
}
func (snapshot *Snapshot) RegionAt(index int) (Region, bool) {
	if !snapshot.Available() || index < 0 || index >= len(snapshot.regions) {
		return Region{}, false
	}
	return snapshot.regions[index], true
}
func (snapshot *Snapshot) EventCount() int {
	if !snapshot.Available() {
		return 0
	}
	return len(snapshot.events)
}
func (snapshot *Snapshot) EventAt(index int) (Event, bool) {
	if !snapshot.Available() || index < 0 || index >= len(snapshot.events) {
		return Event{}, false
	}
	return snapshot.events[index], true
}
func (snapshot *Snapshot) RulePlacementCount() int {
	if !snapshot.Available() {
		return 0
	}
	return len(snapshot.placements)
}
func (snapshot *Snapshot) RulePlacementAt(index int) (RulePlacement, bool) {
	if !snapshot.Available() || index < 0 || index >= len(snapshot.placements) {
		return RulePlacement{}, false
	}
	return snapshot.placements[index], true
}
func (snapshot *Snapshot) BodyTransportCount() int {
	if !snapshot.Available() {
		return 0
	}
	return len(snapshot.bodies)
}
func (snapshot *Snapshot) BodyTransportAt(index int) (BodyTransport, bool) {
	if !snapshot.Available() || index < 0 || index >= len(snapshot.bodies) {
		return BodyTransport{}, false
	}
	return snapshot.bodies[index], true
}
func (snapshot *Snapshot) FunctionBoundaryCount() int {
	if !snapshot.Available() {
		return 0
	}
	return len(snapshot.boundaries)
}
func (snapshot *Snapshot) FunctionBoundaryAt(index int) (FunctionBoundary, bool) {
	if !snapshot.Available() || index < 0 || index >= len(snapshot.boundaries) {
		return FunctionBoundary{}, false
	}
	return snapshot.boundaries[index], true
}

type Point struct {
	id        identity.ContentID
	initial   bool
	decisions []identity.ContentID
}

func (row Point) ID() identity.ContentID { return row.id }
func (row Point) Initial() bool          { return row.initial && row.id.Available() }
func (row Point) DecisionCount() int     { return len(row.decisions) }
func (row Point) DecisionAt(index int) (identity.ContentID, bool) {
	if index < 0 || index >= len(row.decisions) {
		return identity.ContentID{}, false
	}
	return row.decisions[index], true
}

type StructuralEdge struct {
	id, from, to, route  identity.ContentID
	guard, decision      identity.ContentID
	component, mu, reset identity.ContentID
	arm                  StructuralArm
	guarded, truth       bool
	hasReset             bool
	resets               []identity.ContentID
}

func (row StructuralEdge) ID() identity.ContentID          { return row.id }
func (row StructuralEdge) From() identity.ContentID        { return row.from }
func (row StructuralEdge) To() identity.ContentID          { return row.to }
func (row StructuralEdge) RouteID() identity.ContentID     { return row.route }
func (row StructuralEdge) ComponentID() identity.ContentID { return row.component }
func (row StructuralEdge) Arm() StructuralArm              { return row.arm }
func (row StructuralEdge) GuardID() (identity.ContentID, bool) {
	return row.guard, row.guarded && row.guard.Available()
}
func (row StructuralEdge) DecisionID() (identity.ContentID, bool) {
	return row.decision, row.guarded && row.decision.Available()
}
func (row StructuralEdge) Truth() (bool, bool) {
	return row.truth, row.guarded
}
func (row StructuralEdge) MuPathID() (identity.ContentID, bool) {
	return row.mu, row.mu.Available()
}
func (row StructuralEdge) ResetDigest() (identity.ContentID, bool) {
	return row.reset, row.hasReset && row.reset.Available()
}
func (row StructuralEdge) ResetCount() int { return len(row.resets) }
func (row StructuralEdge) ResetAt(index int) (identity.ContentID, bool) {
	if index < 0 || index >= len(row.resets) {
		return identity.ContentID{}, false
	}
	return row.resets[index], true
}

type LocalTransfer struct {
	id, from, to identity.ContentID
	full         bool
	writes       []schema.Key
}

func (row LocalTransfer) ID() identity.ContentID   { return row.id }
func (row LocalTransfer) From() identity.ContentID { return row.from }
func (row LocalTransfer) To() identity.ContentID   { return row.to }
func (row LocalTransfer) Full() bool               { return row.full }
func (row LocalTransfer) WritesCount() int         { return len(row.writes) }
func (row LocalTransfer) WritesAt(index int) (schema.Key, bool) {
	if index < 0 || index >= len(row.writes) {
		return "", false
	}
	return row.writes[index], true
}

type Region struct {
	id, head, parent identity.ContentID
	cyclic           bool
	members          []identity.ContentID
}

func (row Region) ID() identity.ContentID       { return row.id }
func (row Region) Head() identity.ContentID     { return row.head }
func (row Region) ParentID() identity.ContentID { return row.parent }
func (row Region) Cyclic() bool                 { return row.cyclic }
func (row Region) MemberCount() int             { return len(row.members) }
func (row Region) MemberAt(index int) (identity.ContentID, bool) {
	if index < 0 || index >= len(row.members) {
		return identity.ContentID{}, false
	}
	return row.members[index], true
}

type Event struct {
	kind   EventKind
	region identity.ContentID
	point  identity.ContentID
}

func (row Event) Kind() EventKind              { return row.kind }
func (row Event) RegionID() identity.ContentID { return row.region }
func (row Event) PointID() identity.ContentID  { return row.point }

type RulePlacement struct {
	key         schema.Key
	stage       uint8
	point       identity.ContentID
	input       identity.ContentID
	occurrence  identity.ContentID
	predecessor identity.ContentID
}

func (row RulePlacement) Key() schema.Key                  { return row.key }
func (row RulePlacement) Stage() uint8                     { return row.stage }
func (row RulePlacement) PointID() identity.ContentID      { return row.point }
func (row RulePlacement) InputPointID() identity.ContentID { return row.input }
func (row RulePlacement) OccurrenceID() identity.ContentID { return row.occurrence }
func (row RulePlacement) PredecessorRouteID() identity.ContentID {
	return row.predecessor
}

type BodyTransport struct {
	id, context, entry identity.ContentID
	function, formal   identity.ContentID
	callable           bool
	entries            []identity.ContentID
	exits              []identity.ContentID
}

func (row BodyTransport) BodyID() identity.ContentID          { return row.id }
func (row BodyTransport) ContextID() identity.ContentID       { return row.context }
func (row BodyTransport) SemanticEntryID() identity.ContentID { return row.entry }
func (row BodyTransport) Callable() bool                      { return row.callable }
func (row BodyTransport) FunctionID() identity.ContentID      { return row.function }
func (row BodyTransport) CallFormalID() identity.ContentID    { return row.formal }
func (row BodyTransport) EntryCount() int                     { return len(row.entries) }
func (row BodyTransport) EntryAt(index int) (identity.ContentID, bool) {
	if index < 0 || index >= len(row.entries) {
		return identity.ContentID{}, false
	}
	return row.entries[index], true
}
func (row BodyTransport) ExitCount() int { return len(row.exits) }
func (row BodyTransport) ExitAt(index int) (identity.ContentID, bool) {
	if index < 0 || index >= len(row.exits) {
		return identity.ContentID{}, false
	}
	return row.exits[index], true
}

type FunctionBoundary struct {
	id, body, bodyContext, entry, formal identity.ContentID
}

func (row FunctionBoundary) ID() identity.ContentID            { return row.id }
func (row FunctionBoundary) BodyID() identity.ContentID        { return row.body }
func (row FunctionBoundary) BodyContextID() identity.ContentID { return row.bodyContext }
func (row FunctionBoundary) EntryID() identity.ContentID       { return row.entry }
func (row FunctionBoundary) CallFormalID() identity.ContentID  { return row.formal }

func copyIDs(count int, at func(int) (identity.ContentID, bool)) ([]identity.ContentID, bool) {
	if count < 0 {
		return nil, false
	}
	if count == 0 {
		return nil, true
	}
	ids := make([]identity.ContentID, count)
	for index := 0; index < count; index++ {
		id, ok := at(index)
		if !ok || !id.Available() {
			return nil, false
		}
		ids[index] = id
	}
	return ids, true
}

func projectArm(vocabulary structure.Table, ordinal uint16) (StructuralArm, bool) {
	member, ok := vocabulary.At(structure.CategoryArm, ordinal)
	if !ok {
		return StructuralArmInvalid, false
	}
	arm := StructuralArm(member.Ordinal())
	return arm, arm.Valid()
}

func projectEvent(vocabulary structure.Table, ordinal uint16) (EventKind, bool) {
	member, ok := vocabulary.At(structure.CategoryEvent, ordinal)
	if !ok {
		return EventInvalid, false
	}
	kind := EventKind(member.Ordinal())
	return kind, kind != EventInvalid
}

func acceptedOutcome(vocabulary structure.Table, kind programartifact.OutcomeKind) bool {
	member, ok := vocabulary.At(structure.CategoryOutcome, uint16(kind))
	return ok && member.Accepted()
}

func lowerBodyExits(artifact *programartifact.Artifact, vocabulary structure.Table, bodyIndex int, body programartifact.BodyRow) ([]identity.ContentID, bool) {
	seen := make(map[identity.ContentID]struct{})
	var exits []identity.ContentID
	for outcomeIndex := 0; outcomeIndex < body.OutcomeCount(); outcomeIndex++ {
		outcome, ok := artifact.BodyOutcomeAt(bodyIndex, outcomeIndex)
		if !ok {
			return nil, false
		}
		if !acceptedOutcome(vocabulary, outcome.Kind()) {
			continue
		}
		for pointIndex := 0; pointIndex < outcome.PointCount(); pointIndex++ {
			point, ok := outcome.PointAt(pointIndex)
			if !ok || !point.Available() {
				return nil, false
			}
			if _, duplicate := seen[point]; duplicate {
				continue
			}
			seen[point] = struct{}{}
			exits = append(exits, point)
		}
	}
	return exits, true
}

// Lower projects one sealed artifact through the sealed structural vocabulary
// into closed columns. The returned snapshot retains no owner pointer.
func Lower(artifact *programartifact.Artifact, vocabulary structure.Table) (*Snapshot, bool) {
	if !artifactAuthority(artifact) || !vocabularyAuthority(vocabulary) {
		return nil, false
	}
	snapshot := &Snapshot{
		artifactID: artifact.ID(),
		programID:  artifact.CompileKey().ProgramID(),
		schemaID:   artifact.CompileKey().SchemaDigest(),
		vocabulary: vocabulary,
	}
	snapshot.points = make([]Point, 0, artifact.PointCount())
	for index := 0; index < artifact.PointCount(); index++ {
		row, ok := artifact.PointAt(index)
		if !ok || !row.ID().Available() {
			return nil, false
		}
		initial, initialOK := row.Initial()
		decisions, decisionsOK := copyIDs(row.DecisionCount(), row.DecisionAt)
		if !initialOK || !decisionsOK {
			return nil, false
		}
		snapshot.points = append(snapshot.points, Point{id: row.ID(), initial: initial, decisions: decisions})
	}
	snapshot.edges = make([]StructuralEdge, 0, artifact.EnvironmentEdgeCount())
	for index := 0; index < artifact.EnvironmentEdgeCount(); index++ {
		row, ok := artifact.EnvironmentEdgeAt(index)
		if !ok {
			return nil, false
		}
		arm, armOK := projectArm(vocabulary, uint16(row.Arm()))
		if !armOK {
			return nil, false
		}
		guard, guarded := row.GuardID()
		decision, decisionOK := row.DecisionID()
		truth, truthOK := row.Truth()
		mu, hasMu := row.MuPathID()
		reset, hasReset := row.ResetDigest()
		if guarded != decisionOK || guarded != truthOK || hasMu != hasReset {
			return nil, false
		}
		resets, resetsOK := copyIDs(row.ResetCount(), row.ResetAt)
		if !resetsOK {
			return nil, false
		}
		snapshot.edges = append(snapshot.edges, StructuralEdge{
			id: row.ID(), from: row.From(), to: row.To(), route: row.RouteID(),
			guard: guard, decision: decision, component: row.ComponentID(), mu: mu, reset: reset,
			arm: arm, guarded: guarded, truth: truth, hasReset: hasReset, resets: resets,
		})
	}
	snapshot.transfers = make([]LocalTransfer, 0, artifact.LocalTransferCount())
	for index := 0; index < artifact.LocalTransferCount(); index++ {
		row, ok := artifact.LocalTransferAt(index)
		if !ok {
			return nil, false
		}
		writes := make([]schema.Key, 0, row.WritesCount())
		for inner := 0; inner < row.WritesCount(); inner++ {
			write, writeOK := row.WritesAt(inner)
			if !writeOK || !write.Available() {
				return nil, false
			}
			writes = append(writes, write)
		}
		snapshot.transfers = append(snapshot.transfers, LocalTransfer{
			id: row.ID(), from: row.From(), to: row.To(), full: row.FullEnvironment(), writes: writes,
		})
	}
	snapshot.regions = make([]Region, 0, artifact.RegionCount())
	for index := 0; index < artifact.RegionCount(); index++ {
		row, ok := artifact.RegionAt(index)
		if !ok {
			return nil, false
		}
		members, membersOK := copyIDs(row.MemberCount(), row.MemberAt)
		if !membersOK {
			return nil, false
		}
		snapshot.regions = append(snapshot.regions, Region{
			id: row.ID(), head: row.Head(), parent: row.ParentID(), cyclic: row.Cyclic(), members: members,
		})
	}
	snapshot.events = make([]Event, 0, artifact.WTOEventCount())
	for index := 0; index < artifact.WTOEventCount(); index++ {
		row, ok := artifact.WTOEventAt(index)
		if !ok {
			return nil, false
		}
		kind, kindOK := projectEvent(vocabulary, uint16(row.Kind()))
		if !kindOK {
			return nil, false
		}
		snapshot.events = append(snapshot.events, Event{kind: kind, region: row.RegionID(), point: row.PointID()})
	}
	snapshot.placements = make([]RulePlacement, 0, artifact.RulePlacementCount())
	for index := 0; index < artifact.RulePlacementCount(); index++ {
		row, ok := artifact.RulePlacementAt(index)
		if !ok || !row.Key().Available() {
			return nil, false
		}
		point, _ := row.PointAt(0)
		input, _ := row.InputPoint()
		route, _ := row.PredecessorRouteID()
		snapshot.placements = append(snapshot.placements, RulePlacement{
			key: row.Key(), stage: uint8(row.Stage()), point: point, input: input,
			occurrence: row.ID(), predecessor: route,
		})
	}
	snapshot.bodies = make([]BodyTransport, 0, artifact.BodyCount())
	for index := 0; index < artifact.BodyCount(); index++ {
		row, ok := artifact.BodyAt(index)
		if !ok {
			return nil, false
		}
		entries, entriesOK := copyIDs(row.EntryPointCount(), row.EntryPointAt)
		exits, exitsOK := lowerBodyExits(artifact, vocabulary, index, row)
		if !entriesOK || !exitsOK {
			return nil, false
		}
		function, _ := row.FunctionContextID()
		formal, _ := row.CallFormalID()
		snapshot.bodies = append(snapshot.bodies, BodyTransport{
			id: row.ID(), context: row.ContextID(), entry: row.EntryID(),
			function: function, formal: formal, callable: row.Callable(),
			entries: entries, exits: exits,
		})
	}
	snapshot.boundaries = make([]FunctionBoundary, 0, artifact.FunctionBoundaryCount())
	for index := 0; index < artifact.FunctionBoundaryCount(); index++ {
		row, ok := artifact.FunctionBoundaryAt(index)
		if !ok {
			return nil, false
		}
		snapshot.boundaries = append(snapshot.boundaries, FunctionBoundary{
			id: row.ID(), body: row.BodyID(), bodyContext: row.BodyContextID(),
			entry: row.EntryID(), formal: row.CallFormalID(),
		})
	}
	return snapshot, snapshot.Available()
}
