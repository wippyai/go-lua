// Package ingress lowers a sealed ProgramArtifact into closed immutable
// columns exactly once per ContentID. After Lower succeeds the snapshot
// retains no owner pointer and cannot reopen artifact interiors.
package ingress

import (
	"github.com/wippyai/go-lua/analysis/identity"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	programsource "github.com/wippyai/go-lua/analysis/program/source"
	staticquery "github.com/wippyai/go-lua/analysis/program/static/query"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/cold"
	schemadiag "github.com/wippyai/go-lua/analysis/schema/diagnostic"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	snapshotstore "github.com/wippyai/go-lua/analysis/snapshot"
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
	artifactID      identity.ContentID
	programID       identity.ContentID
	schemaID        identity.ContentID
	frozen          snapshotstore.Frozen
	coldCatalog     identity.ContentID
	vocabulary      structure.Table
	transfers       []LocalTransfer
	placements      []RulePlacement
	bodies          []BodyTransport
	boundaries      []FunctionBoundary
	occurrences     []Occurrence
	observations    []DiagnosticObservation
	outcomes        []Outcome
	staticTypeNodes []StaticTypeNode
	staticInputs    []StaticInput
}

func (snapshot *Snapshot) Available() bool {
	return snapshot != nil && snapshot.artifactID.Available() && snapshot.programID.Available() && snapshot.schemaID.Available() && snapshot.frozen.Published() && snapshot.coldCatalog.Available() && vocabularyAuthority(snapshot.vocabulary)
}

func vocabularyAuthority(vocabulary structure.Table) bool {
	return vocabulary.Count(structure.CategoryArm) != 0 &&
		vocabulary.Count(structure.CategoryEvent) != 0 &&
		vocabulary.Count(structure.CategoryOutcome) != 0
}

// coldView is the address a snapshot reads its cold families at. It travels
// inside the rows that name a child span, so a span is rejoined where it is
// read and the snapshot never holds a second copy of a published plane.
type coldView struct {
	frozen  *snapshotstore.Frozen
	catalog identity.ContentID
}

func (snapshot *Snapshot) coldView() coldView {
	if snapshot == nil {
		return coldView{}
	}
	return coldView{frozen: &snapshot.frozen, catalog: snapshot.coldCatalog}
}

func coldCount[V cold.Row](view coldView, family cold.Family[V]) (int, bool) {
	if view.frozen == nil {
		return 0, false
	}
	return family.Count(view.frozen, view.catalog)
}

func coldRow[V cold.Row](view coldView, family cold.Family[V], index int) (V, bool) {
	var absent V
	if view.frozen == nil {
		return absent, false
	}
	return family.At(view.frozen, view.catalog, index)
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

// ColdProgram authenticates this ingress snapshot as the same published cold
// Program mounted under module. Summary consumers use this row rather than
// reopening ingress or retaining a second summary projection.
func (snapshot *Snapshot) ColdProgram(module identity.ContentID) (cold.Program, bool) {
	if !snapshot.Available() || !module.Available() {
		return cold.Program{}, false
	}
	program := cold.Program{
		Frozen: snapshot.frozen, ModuleKey: module,
		ArtifactID: snapshot.artifactID, ProgramID: snapshot.programID, SchemaID: snapshot.schemaID,
	}
	return program, program.Available()
}
func (snapshot *Snapshot) PointCount() int {
	if !snapshot.Available() {
		return 0
	}
	count, published := coldCount(snapshot.coldView(), cold.PointFamily())
	if !published {
		return 0
	}
	return count
}
func (snapshot *Snapshot) PointAt(index int) (Point, bool) {
	if !snapshot.Available() {
		return Point{}, false
	}
	view := snapshot.coldView()
	row, held := coldRow(view, cold.PointFamily(), index)
	if !held {
		return Point{}, false
	}
	return Point{row: row, view: view}, true
}
func (snapshot *Snapshot) StructuralEdgeCount() int {
	if !snapshot.Available() {
		return 0
	}
	count, published := coldCount(snapshot.coldView(), cold.EnvironmentEdgeFamily())
	if !published {
		return 0
	}
	return count
}
func (snapshot *Snapshot) StructuralEdgeAt(index int) (StructuralEdge, bool) {
	if !snapshot.Available() {
		return StructuralEdge{}, false
	}
	view := snapshot.coldView()
	row, held := coldRow(view, cold.EnvironmentEdgeFamily(), index)
	if !held {
		return StructuralEdge{}, false
	}
	arm, armOK := projectArm(snapshot.vocabulary, uint16(row.Arm()))
	if !armOK {
		return StructuralEdge{}, false
	}
	return StructuralEdge{row: row, view: view, arm: arm}, true
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
	count, published := coldCount(snapshot.coldView(), cold.RegionFamily())
	if !published {
		return 0
	}
	return count
}
func (snapshot *Snapshot) RegionAt(index int) (Region, bool) {
	if !snapshot.Available() {
		return Region{}, false
	}
	view := snapshot.coldView()
	row, held := coldRow(view, cold.RegionFamily(), index)
	if !held {
		return Region{}, false
	}
	return Region{row: row, view: view}, true
}
func (snapshot *Snapshot) EventCount() int {
	if !snapshot.Available() {
		return 0
	}
	count, published := coldCount(snapshot.coldView(), cold.WTOEventFamily())
	if !published {
		return 0
	}
	return count
}
func (snapshot *Snapshot) EventAt(index int) (Event, bool) {
	if !snapshot.Available() {
		return Event{}, false
	}
	row, held := coldRow(snapshot.coldView(), cold.WTOEventFamily(), index)
	if !held {
		return Event{}, false
	}
	kind, kindOK := projectEvent(snapshot.vocabulary, uint16(row.Kind()))
	if !kindOK {
		return Event{}, false
	}
	return Event{row: row, kind: kind}, true
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
func (snapshot *Snapshot) CallArgumentCount() int {
	if !snapshot.Available() {
		return 0
	}
	count, published := coldCount(snapshot.coldView(), cold.CallArgumentFamily())
	if !published {
		return 0
	}
	return count
}
func (snapshot *Snapshot) CallArgumentAt(index int) (CallArgument, bool) {
	if !snapshot.Available() {
		return CallArgument{}, false
	}
	row, held := coldRow(snapshot.coldView(), cold.CallArgumentFamily(), index)
	if !held {
		return CallArgument{}, false
	}
	return CallArgument{row: row}, true
}
func (snapshot *Snapshot) CallArgumentForID(id identity.ContentID) (CallArgument, bool) {
	if !snapshot.Available() || !id.Available() {
		return CallArgument{}, false
	}
	for index := 0; index < snapshot.CallArgumentCount(); index++ {
		row, held := snapshot.CallArgumentAt(index)
		if held && row.ID() == id {
			return row, row.Available()
		}
	}
	return CallArgument{}, false
}
func (snapshot *Snapshot) CallCount() int {
	if !snapshot.Available() {
		return 0
	}
	count, published := coldCount(snapshot.coldView(), cold.CallFamily())
	if !published {
		return 0
	}
	return count
}
func (snapshot *Snapshot) CallAt(index int) (Call, bool) {
	if !snapshot.Available() {
		return Call{}, false
	}
	view := snapshot.coldView()
	row, held := coldRow(view, cold.CallFamily(), index)
	if !held {
		return Call{}, false
	}
	return Call{row: row, view: view}, true
}

// CallForID resolves one parent-issued Call identity through the sealed
// ingress directory. Consumers must use this owner query instead of
// rebuilding a private Call index from the snapshot rows.
func (snapshot *Snapshot) CallForID(id identity.ContentID) (Call, bool) {
	if !snapshot.Available() || !id.Available() {
		return Call{}, false
	}
	var found Call
	foundOne := false
	for index := 0; index < snapshot.CallCount(); index++ {
		row, held := snapshot.CallAt(index)
		if !held || row.ID() != id {
			continue
		}
		if foundOne {
			return Call{}, false
		}
		found, foundOne = row, true
	}
	return found, foundOne
}
func (snapshot *Snapshot) HeapAllocationCount() int {
	if !snapshot.Available() {
		return 0
	}
	count, published := coldCount(snapshot.coldView(), cold.HeapAllocationFamily())
	if !published {
		return 0
	}
	return count
}
func (snapshot *Snapshot) HeapAllocationAt(index int) (HeapAllocation, bool) {
	if !snapshot.Available() {
		return HeapAllocation{}, false
	}
	view := snapshot.coldView()
	row, held := coldRow(view, cold.HeapAllocationFamily(), index)
	if !held {
		return HeapAllocation{}, false
	}
	return HeapAllocation{row: row, view: view}, true
}
func (snapshot *Snapshot) ValuesCount() int {
	if !snapshot.Available() {
		return 0
	}
	count, published := coldCount(snapshot.coldView(), cold.ValuesFamily())
	if !published {
		return 0
	}
	return count
}
func (snapshot *Snapshot) StaticTypeValueCount() int {
	if !snapshot.Available() {
		return 0
	}
	count, published := coldCount(snapshot.coldView(), cold.StaticTypeValueFamily())
	if !published {
		return 0
	}
	return count
}
func (snapshot *Snapshot) StaticTypeValueAt(index int) (StaticTypeValue, bool) {
	if !snapshot.Available() {
		return StaticTypeValue{}, false
	}
	row, held := coldRow(snapshot.coldView(), cold.StaticTypeValueFamily(), index)
	if !held {
		return StaticTypeValue{}, false
	}
	return StaticTypeValue{row: row}, true
}
func (snapshot *Snapshot) StaticTypeNodeCount() int {
	if !snapshot.Available() {
		return 0
	}
	return len(snapshot.staticTypeNodes)
}
func (snapshot *Snapshot) StaticTypeNodeAt(index int) (StaticTypeNode, bool) {
	if !snapshot.Available() || index < 0 || index >= len(snapshot.staticTypeNodes) {
		return StaticTypeNode{}, false
	}
	return snapshot.staticTypeNodes[index], true
}
func (snapshot *Snapshot) StaticTypeNodeForID(id identity.ContentID) (StaticTypeNode, bool) {
	if !snapshot.Available() || !id.Available() {
		return StaticTypeNode{}, false
	}
	for _, row := range snapshot.staticTypeNodes {
		if row.ID() == id {
			return row, row.Available()
		}
	}
	return StaticTypeNode{}, false
}
func (snapshot *Snapshot) StaticTypeArgumentCount() int {
	if !snapshot.Available() {
		return 0
	}
	count, published := coldCount(snapshot.coldView(), cold.CallTypeArgumentFamily())
	if !published {
		return 0
	}
	return count
}
func (snapshot *Snapshot) StaticTypeArgumentAt(index int) (StaticTypeArgument, bool) {
	if !snapshot.Available() {
		return StaticTypeArgument{}, false
	}
	row, held := coldRow(snapshot.coldView(), cold.CallTypeArgumentFamily(), index)
	if !held {
		return StaticTypeArgument{}, false
	}
	return StaticTypeArgument{row: row}, true
}
func (snapshot *Snapshot) StaticExpressionCount() int {
	if !snapshot.Available() {
		return 0
	}
	count, published := coldCount(snapshot.coldView(), cold.StaticExpressionFamily())
	if !published {
		return 0
	}
	return count
}
func (snapshot *Snapshot) StaticExpressionAt(index int) (StaticExpression, bool) {
	if !snapshot.Available() {
		return StaticExpression{}, false
	}
	row, held := coldRow(snapshot.coldView(), cold.StaticExpressionFamily(), index)
	if !held {
		return StaticExpression{}, false
	}
	return StaticExpression{row: row}, true
}
func (snapshot *Snapshot) StaticInputCount() int {
	if !snapshot.Available() {
		return 0
	}
	return len(snapshot.staticInputs)
}
func (snapshot *Snapshot) StaticInputAt(index int) (StaticInput, bool) {
	if !snapshot.Available() || index < 0 || index >= len(snapshot.staticInputs) {
		return StaticInput{}, false
	}
	return snapshot.staticInputs[index], true
}
func (snapshot *Snapshot) ValuesAt(index int) (Values, bool) {
	if !snapshot.Available() {
		return Values{}, false
	}
	view := snapshot.coldView()
	row, held := coldRow(view, cold.ValuesFamily(), index)
	if !held {
		return Values{}, false
	}
	return Values{row: row, view: view}, true
}
func (snapshot *Snapshot) OutcomeCount() int {
	if !snapshot.Available() {
		return 0
	}
	return len(snapshot.outcomes)
}
func (snapshot *Snapshot) OutcomeAt(index int) (Outcome, bool) {
	if !snapshot.Available() || index < 0 || index >= len(snapshot.outcomes) {
		return Outcome{}, false
	}
	return snapshot.outcomes[index], true
}
func (snapshot *Snapshot) OutcomeReturnValueAt(outcomeIndex, valueIndex int) (ValuesMember, bool) {
	row, ok := snapshot.OutcomeAt(outcomeIndex)
	if !ok || valueIndex < 0 || valueIndex >= len(row.returns) {
		return ValuesMember{}, false
	}
	return newValuesMember(row.returns[valueIndex])
}
func (snapshot *Snapshot) DiagnosticObservationCount() int {
	if !snapshot.Available() {
		return 0
	}
	return len(snapshot.observations)
}
func (snapshot *Snapshot) DiagnosticObservationAt(index int) (DiagnosticObservation, bool) {
	if !snapshot.Available() || index < 0 || index >= len(snapshot.observations) {
		return DiagnosticObservation{}, false
	}
	return snapshot.observations[index], true
}
func (snapshot *Snapshot) DiagnosticObservationForID(id identity.ContentID) (DiagnosticObservation, bool) {
	if !snapshot.Available() || !id.Available() {
		return DiagnosticObservation{}, false
	}
	for _, row := range snapshot.observations {
		if row.ID() == id {
			return row, row.Available()
		}
	}
	return DiagnosticObservation{}, false
}
func (snapshot *Snapshot) HeapIndexCount() int {
	if !snapshot.Available() {
		return 0
	}
	count, published := coldCount(snapshot.coldView(), cold.HeapIndexFamily())
	if !published {
		return 0
	}
	return count
}
func (snapshot *Snapshot) HeapIndexAt(index int) (HeapIndex, bool) {
	if !snapshot.Available() {
		return HeapIndex{}, false
	}
	row, held := coldRow(snapshot.coldView(), cold.HeapIndexFamily(), index)
	if !held {
		return HeapIndex{}, false
	}
	return HeapIndex{row: row}, true
}
func (snapshot *Snapshot) OccurrenceCount() int {
	if !snapshot.Available() {
		return 0
	}
	return len(snapshot.occurrences)
}
func (snapshot *Snapshot) OccurrenceAt(index int) (Occurrence, bool) {
	if !snapshot.Available() || index < 0 || index >= len(snapshot.occurrences) {
		return Occurrence{}, false
	}
	return snapshot.occurrences[index], true
}
func (snapshot *Snapshot) OccurrenceKindCount(kind uint8) int {
	if !snapshot.Available() {
		return 0
	}
	count := 0
	for _, row := range snapshot.occurrences {
		if row.kind == kind {
			count++
		}
	}
	return count
}
func (snapshot *Snapshot) OccurrenceKindAt(kind uint8, index int) (Occurrence, bool) {
	if !snapshot.Available() || index < 0 {
		return Occurrence{}, false
	}
	seen := 0
	for _, row := range snapshot.occurrences {
		if row.kind != kind {
			continue
		}
		if seen == index {
			return row, true
		}
		seen++
	}
	return Occurrence{}, false
}
func (snapshot *Snapshot) BodyCount() int { return snapshot.BodyTransportCount() }
func (snapshot *Snapshot) BodyAt(index int) (BodyTransport, bool) {
	return snapshot.BodyTransportAt(index)
}
func (snapshot *Snapshot) FunctionBoundaryForBody(bodyID identity.ContentID) (FunctionBoundary, bool) {
	if !snapshot.Available() || !bodyID.Available() {
		return FunctionBoundary{}, false
	}
	for _, row := range snapshot.boundaries {
		if row.body == bodyID {
			return row, true
		}
	}
	return FunctionBoundary{}, false
}
func (snapshot *Snapshot) OccurrenceForID(kind uint8, id identity.ContentID) (Occurrence, bool) {
	if !snapshot.Available() || !id.Available() {
		return Occurrence{}, false
	}
	for _, row := range snapshot.occurrences {
		if row.kind == kind && row.id == id {
			return row, true
		}
	}
	return Occurrence{}, false
}

type Point struct {
	row  cold.Point
	view coldView
}

func (row Point) ID() identity.ContentID { return row.row.ID() }
func (row Point) Initial() bool          { return row.row.Initial() }
func (row Point) DecisionCount() int     { return row.row.DecisionCount() }
func (row Point) DecisionAt(index int) (identity.ContentID, bool) {
	offset, count, spanOK := row.row.DecisionSpan()
	if !spanOK || index < 0 || uint64(index) >= uint64(count) {
		return identity.ContentID{}, false
	}
	decision, held := coldRow(row.view, cold.PointDecisionFamily(), int(offset)+index)
	if !held {
		return identity.ContentID{}, false
	}
	return decision.ID(), true
}

type StructuralEdge struct {
	row  cold.EnvironmentEdge
	view coldView
	arm  StructuralArm
}

func (row StructuralEdge) ID() identity.ContentID          { return row.row.ID() }
func (row StructuralEdge) From() identity.ContentID        { return row.row.From() }
func (row StructuralEdge) To() identity.ContentID          { return row.row.To() }
func (row StructuralEdge) RouteID() identity.ContentID     { return row.row.RouteID() }
func (row StructuralEdge) ComponentID() identity.ContentID { return row.row.ComponentID() }
func (row StructuralEdge) Arm() StructuralArm              { return row.arm }
func (row StructuralEdge) GuardID() (identity.ContentID, bool) {
	guard, guarded := row.row.GuardID()
	return guard, guarded && guard.Available()
}
func (row StructuralEdge) DecisionID() (identity.ContentID, bool) {
	decision, guarded := row.row.DecisionID()
	return decision, guarded && decision.Available()
}
func (row StructuralEdge) Truth() (bool, bool) { return row.row.Truth() }
func (row StructuralEdge) MuPathID() (identity.ContentID, bool) {
	mu, _ := row.row.MuPathID()
	return mu, mu.Available()
}
func (row StructuralEdge) ResetDigest() (identity.ContentID, bool) {
	reset, hasReset := row.row.ResetDigest()
	return reset, hasReset && reset.Available()
}
func (row StructuralEdge) ResetCount() int { return row.row.ResetCount() }
func (row StructuralEdge) ResetAt(index int) (identity.ContentID, bool) {
	offset, count, spanOK := row.row.ResetSpan()
	if !spanOK || index < 0 || uint64(index) >= uint64(count) {
		return identity.ContentID{}, false
	}
	witness, held := coldRow(row.view, cold.EnvironmentResetFamily(), int(offset)+index)
	if !held {
		return identity.ContentID{}, false
	}
	return witness.ID(), true
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
func (row LocalTransfer) Available() bool {
	return row.id.Available() && row.from.Available() && row.to.Available()
}
func (row LocalTransfer) WritesCount() int { return len(row.writes) }
func (row LocalTransfer) WritesAt(index int) (schema.Key, bool) {
	if index < 0 || index >= len(row.writes) {
		return "", false
	}
	return row.writes[index], true
}

type Region struct {
	row  cold.Region
	view coldView
}

func (row Region) ID() identity.ContentID { return row.row.ID() }
func (row Region) Head() identity.ContentID {
	head, held := row.MemberAt(0)
	if !held {
		return identity.ContentID{}
	}
	return head
}
func (row Region) ParentID() identity.ContentID { return row.row.ParentID() }
func (row Region) Cyclic() bool                 { return row.row.Cyclic() }
func (row Region) MemberCount() int             { return row.row.MemberCount() }
func (row Region) MemberAt(index int) (identity.ContentID, bool) {
	offset, count, spanOK := row.row.MemberSpan()
	if !spanOK || index < 0 || uint64(index) >= uint64(count) {
		return identity.ContentID{}, false
	}
	member, held := coldRow(row.view, cold.RegionMemberFamily(), int(offset)+index)
	if !held {
		return identity.ContentID{}, false
	}
	return member.ID(), true
}

// Event is one order bracket read out of the published plane. Its kind is the
// sealed structural vocabulary's member, projected at the read site.
type Event struct {
	row  cold.WTOEvent
	kind EventKind
}

func (row Event) Kind() EventKind              { return row.kind }
func (row Event) RegionID() identity.ContentID { return row.row.RegionID() }
func (row Event) PointID() identity.ContentID  { return row.row.PointID() }

type RulePlacement struct {
	key         schema.Key
	stage       uint8
	point       identity.ContentID
	input       identity.ContentID
	occurrence  identity.ContentID
	predecessor identity.ContentID
	output      identity.ContentID
	hasOutput   bool
	spanResult  bool
	inputKind   uint8
}

func (row RulePlacement) Key() schema.Key                  { return row.key }
func (row RulePlacement) Stage() uint8                     { return row.stage }
func (row RulePlacement) PointID() identity.ContentID      { return row.point }
func (row RulePlacement) InputPointID() identity.ContentID { return row.input }
func (row RulePlacement) OccurrenceID() identity.ContentID { return row.occurrence }
func (row RulePlacement) PredecessorRouteID() identity.ContentID {
	return row.predecessor
}
func (row RulePlacement) OutputSemanticID() (identity.ContentID, bool) {
	return row.output, row.hasOutput && row.output.Available()
}
func (row RulePlacement) SpanResult() bool { return row.spanResult }
func (row RulePlacement) InputKind() uint8 { return row.inputKind }

type BodyRoot struct {
	id     identity.ContentID
	family keyspace.Family
}

func (row BodyRoot) Available() bool {
	return row.id.Available() && row.family != keyspace.FamilyInvalid
}
func (row BodyRoot) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}
func (row BodyRoot) Family() keyspace.Family {
	if !row.Available() {
		return keyspace.FamilyInvalid
	}
	return row.family
}

type BodyTransport struct {
	id, context, entry identity.ContentID
	function, formal   identity.ContentID
	callable           bool
	entries            []identity.ContentID
	exits              []identity.ContentID
	roots              []BodyRoot
}

func (row BodyTransport) ID() identity.ContentID              { return row.id }
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
func (row BodyTransport) RootCount() int { return len(row.roots) }
func (row BodyTransport) RootAt(index int) (BodyRoot, bool) {
	if index < 0 || index >= len(row.roots) {
		return BodyRoot{}, false
	}
	return row.roots[index], row.roots[index].Available()
}

type FunctionFormal struct {
	id, cell, storage identity.ContentID
}

func (row FunctionFormal) Available() bool {
	return row.id.Available() && row.cell.Available() && row.storage.Available()
}
func (row FunctionFormal) ID() identity.ContentID            { return row.id }
func (row FunctionFormal) CellID() identity.ContentID        { return row.cell }
func (row FunctionFormal) StorageCellID() identity.ContentID { return row.storage }

type FunctionVararg struct {
	id, cell identity.ContentID
}

func (row FunctionVararg) Available() bool {
	return row.id.Available() && row.cell.Available()
}
func (row FunctionVararg) ID() identity.ContentID     { return row.id }
func (row FunctionVararg) CellID() identity.ContentID { return row.cell }

type FunctionCapture struct {
	id, inner, outer, innerBody, outerBody identity.ContentID
}

func (row FunctionCapture) Available() bool {
	return row.id.Available() && row.inner.Available() && row.outer.Available() &&
		row.innerBody.Available() && row.outerBody.Available() &&
		row.inner != row.outer && row.innerBody != row.outerBody
}
func (row FunctionCapture) ID() identity.ContentID          { return row.id }
func (row FunctionCapture) InnerCellID() identity.ContentID { return row.inner }
func (row FunctionCapture) OuterCellID() identity.ContentID { return row.outer }
func (row FunctionCapture) InnerBodyID() identity.ContentID { return row.innerBody }
func (row FunctionCapture) OuterBodyID() identity.ContentID { return row.outerBody }

type FunctionBoundary struct {
	id, body, bodyContext, entry, formal identity.ContentID
	formals                              []FunctionFormal
	vararg                               FunctionVararg
	hasVararg                            bool
	captures                             []FunctionCapture
	outcomes                             []identity.ContentID
}

func (row FunctionBoundary) Available() bool {
	if !row.id.Available() || !row.body.Available() || !row.bodyContext.Available() ||
		!row.entry.Available() || !row.formal.Available() || row.hasVararg != row.vararg.Available() || len(row.outcomes) == 0 {
		return false
	}
	for _, port := range row.formals {
		if !port.Available() {
			return false
		}
	}
	for _, capture := range row.captures {
		if !capture.Available() || capture.innerBody != row.body {
			return false
		}
	}
	for _, outcome := range row.outcomes {
		if !outcome.Available() {
			return false
		}
	}
	return true
}

type CallOperand struct{ row cold.CallOperand }

func (row CallOperand) ID() identity.ContentID      { return row.row.ID() }
func (row CallOperand) CallID() identity.ContentID  { return row.row.CallID() }
func (row CallOperand) ValueID() identity.ContentID { return row.row.ValueID() }
func (row CallOperand) SpanID() identity.ContentID  { return row.row.SpanID() }
func (row CallOperand) Callee() bool                { return row.row.Kind() == cold.CallOperandCallee }

type CallArgument struct{ row cold.CallArgument }

func (row CallArgument) Available() bool              { return row.row.Available() }
func (row CallArgument) ID() identity.ContentID       { return row.row.ID() }
func (row CallArgument) CallID() identity.ContentID   { return row.row.CallID() }
func (row CallArgument) ValuesID() identity.ContentID { return row.row.ValuesID() }
func (row CallArgument) MemberID() identity.ContentID { return row.row.MemberID() }
func (row CallArgument) ValueID() identity.ContentID  { return row.row.MemberID() }
func (row CallArgument) SpanID() identity.ContentID   { return row.row.SpanID() }
func (row CallArgument) Index() uint32                { return row.row.Index() }

type Call struct {
	row  cold.Call
	view coldView
}

func (row Call) ID() identity.ContentID              { return row.row.ID() }
func (row Call) BodyID() identity.ContentID          { return row.row.BodyID() }
func (row Call) SpanID() identity.ContentID          { return row.row.SpanID() }
func (row Call) CalleeID() identity.ContentID        { return row.row.CalleeID() }
func (row Call) FormalID() identity.ContentID        { return row.row.FormalID() }
func (row Call) ValuesID() identity.ContentID        { return row.row.ValuesID() }
func (row Call) TypeArgumentsID() identity.ContentID { return row.row.TypeArgumentsID() }
func (row Call) Form() uint8                         { return uint8(row.row.Form()) }
func (row Call) DirectTargetBody() (identity.ContentID, bool) {
	return row.row.DirectTargetBody()
}
func (row Call) ReceiverID() (identity.ContentID, bool) { return row.row.ReceiverID() }
func (row Call) TailID() (identity.ContentID, bool)     { return row.row.TailID() }
func (row Call) OperandCount() int                      { return row.row.OperandCount() }
func (row Call) ArgumentCount() int                     { return row.row.ArgumentCount() }
func (row Call) OperandAt(index int) (CallOperand, bool) {
	offset, count, spanOK := row.row.OperandSpan()
	if !spanOK || index < 0 || uint64(index) >= uint64(count) {
		return CallOperand{}, false
	}
	operand, held := coldRow(row.view, cold.CallOperandFamily(), int(offset)+index)
	if !held {
		return CallOperand{}, false
	}
	return CallOperand{row: operand}, true
}
func (row Call) ArgumentAt(index int) (CallArgument, bool) {
	offset, count, spanOK := row.row.ArgumentSpan()
	if !spanOK || index < 0 || uint64(index) >= uint64(count) {
		return CallArgument{}, false
	}
	argument, held := coldRow(row.view, cold.CallArgumentFamily(), int(offset)+index)
	if !held {
		return CallArgument{}, false
	}
	return CallArgument{row: argument}, true
}
func (snapshot *Snapshot) CallArgumentFor(callIndex, childIndex int) (CallArgument, bool) {
	call, ok := snapshot.CallAt(callIndex)
	if !ok {
		return CallArgument{}, false
	}
	return call.ArgumentAt(childIndex)
}

// HeapField, HeapAllocation, Values, ValuesMember, ValuesTail and HeapIndex
// are views over cold families the compiled program already publishes. A row
// here is the sealed cold row plus the address it was read at, so a field or
// member span is rejoined at the read site and the plane is declared once.

type HeapField struct{ row cold.HeapField }

func (row HeapField) Available() bool                  { return row.row.Available() }
func (row HeapField) ID() identity.ContentID           { return row.row.ID() }
func (row HeapField) ValuesID() identity.ContentID     { return row.row.ValuesID() }
func (row HeapField) Kind() uint8                      { return row.row.Kind() }
func (row HeapField) SharesFirstValueCell() bool       { return row.row.SharesFirstValueCell() }
func (row HeapField) SelectorSpan() identity.ContentID { return row.row.SelectorSpan() }
func (row HeapField) Values() (identity.ContentID, int, bool, bool) {
	return row.row.Values()
}
func (row HeapField) NormalizedKey() (uint64, bool) { return row.row.NormalizedKey() }

type HeapAllocation struct {
	row  cold.HeapAllocation
	view coldView
}

func (row HeapAllocation) Available() bool              { return row.row.Available() }
func (row HeapAllocation) ID() identity.ContentID       { return row.row.ID() }
func (row HeapAllocation) Role() uint8                  { return row.row.Role() }
func (row HeapAllocation) Form() uint8                  { return row.row.Form() }
func (row HeapAllocation) RootSpan() identity.ContentID { return row.row.RootSpan() }
func (row HeapAllocation) FieldCount() int              { return row.row.FieldCount() }
func (row HeapAllocation) FieldAt(index int) (HeapField, bool) {
	offset, count, spanOK := row.row.FieldSpan()
	if !spanOK || index < 0 || uint64(index) >= uint64(count) {
		return HeapField{}, false
	}
	field, held := coldRow(row.view, cold.HeapFieldFamily(), int(offset)+index)
	if !held {
		return HeapField{}, false
	}
	return HeapField{row: field}, true
}

type ValuesMember struct{ row cold.ValuesMember }

func newValuesMember(id identity.ContentID) (ValuesMember, bool) {
	row, ok := cold.NewValuesMember(id)
	if !ok {
		return ValuesMember{}, false
	}
	return ValuesMember{row: row}, true
}

func (row ValuesMember) ID() identity.ContentID { return row.row.ID() }

type ValuesTail struct{ row cold.ValuesTail }

func (row ValuesTail) Present() bool          { return row.row.Present() }
func (row ValuesTail) ID() identity.ContentID { return row.row.ID() }
func (row ValuesTail) Kind() uint8            { return uint8(row.row.Kind()) }

type Values struct {
	row  cold.Values
	view coldView
}

func (row Values) ID() identity.ContentID { return row.row.ID() }
func (row Values) MemberCount() int       { return row.row.MemberCount() }
func (row Values) MemberAt(index int) (ValuesMember, bool) {
	offset, count, spanOK := row.row.MemberSpan()
	if !spanOK || index < 0 || uint64(index) >= uint64(count) {
		return ValuesMember{}, false
	}
	member, held := coldRow(row.view, cold.ValuesMemberFamily(), int(offset)+index)
	if !held {
		return ValuesMember{}, false
	}
	return ValuesMember{row: member}, true
}
func (row Values) Tail() (ValuesTail, bool) {
	tail, present := row.row.Tail()
	return ValuesTail{row: tail}, present
}

type StaticTypeValue struct{ row cold.StaticTypeValue }

func (row StaticTypeValue) ID() identity.ContentID          { return row.row.ID() }
func (row StaticTypeValue) BodyPathID() identity.ContentID  { return row.row.BodyPathID() }
func (row StaticTypeValue) ReferenceID() identity.ContentID { return row.row.ReferenceID() }
func (row StaticTypeValue) RootID() identity.ContentID      { return row.row.RootID() }
func (row StaticTypeValue) Name() string                    { return row.row.Name() }
func (row StaticTypeValue) Available() bool                 { return row.row.Available() }

type StaticTypeNode struct {
	id      identity.ContentID
	owner   identity.ContentID
	kind    uint8
	literal uint8
}

func (row StaticTypeNode) ID() identity.ContentID    { return row.id }
func (row StaticTypeNode) Owner() identity.ContentID { return row.owner }
func (row StaticTypeNode) Kind() uint8               { return row.kind }
func (row StaticTypeNode) LiteralKind() uint8        { return row.literal }
func (row StaticTypeNode) Available() bool {
	return row.id.Available() && row.owner.Available()
}

type StaticTypeArgument struct{ row cold.CallTypeArgument }

func (row StaticTypeArgument) ID() identity.ContentID          { return row.row.ID() }
func (row StaticTypeArgument) CallID() identity.ContentID      { return row.row.CallID() }
func (row StaticTypeArgument) TypesID() identity.ContentID     { return row.row.TypesID() }
func (row StaticTypeArgument) ReferenceID() identity.ContentID { return row.row.ReferenceID() }
func (row StaticTypeArgument) Index() uint32                   { return row.row.Index() }
func (row StaticTypeArgument) Available() bool                 { return row.row.Available() }

type StaticExpression struct{ row cold.StaticExpression }

func (row StaticExpression) ID() identity.ContentID          { return row.row.ID() }
func (row StaticExpression) ReferenceID() identity.ContentID { return row.row.ReferenceID() }
func (row StaticExpression) Owner() identity.ContentID       { return row.row.Owner() }
func (row StaticExpression) Available() bool                 { return row.row.Available() }

type StaticInput struct {
	id, owner, expression, source, target, operand, frontier identity.ContentID
	operandReference, operandSubject, operandBody            identity.ContentID
	literal                                                  keyspace.LiteralValue
	kind, operandKind                                        uint8
	cursor                                                   uint32
}

func (row StaticInput) ID() identity.ContentID                 { return row.id }
func (row StaticInput) Owner() identity.ContentID              { return row.owner }
func (row StaticInput) Kind() uint8                            { return row.kind }
func (row StaticInput) ExpressionID() identity.ContentID       { return row.expression }
func (row StaticInput) SourceID() identity.ContentID           { return row.source }
func (row StaticInput) TargetID() identity.ContentID           { return row.target }
func (row StaticInput) OperandID() identity.ContentID          { return row.operand }
func (row StaticInput) FrontierID() identity.ContentID         { return row.frontier }
func (row StaticInput) Cursor() uint32                         { return row.cursor }
func (row StaticInput) OperandKind() uint8                     { return row.operandKind }
func (row StaticInput) OperandLiteral() keyspace.LiteralValue  { return row.literal }
func (row StaticInput) OperandReferenceID() identity.ContentID { return row.operandReference }
func (row StaticInput) OperandSubjectID() identity.ContentID   { return row.operandSubject }
func (row StaticInput) OperandBodyPathID() identity.ContentID  { return row.operandBody }
func (row StaticInput) Available() bool {
	if !row.id.Available() || !row.owner.Available() || !row.expression.Available() || !row.source.Available() || !row.target.Available() || !row.operand.Available() || !row.frontier.Available() || row.kind == uint8(programartifact.StaticInputInvalid) || row.operandKind == uint8(staticquery.StaticOperandInvalid) {
		return false
	}
	switch staticquery.StaticOperandKind(row.operandKind) {
	case staticquery.StaticOperandKnown:
		return row.operandSubject == (identity.ContentID{}) && row.operandReference == (identity.ContentID{})
	case staticquery.StaticOperandRuntimeSubject:
		return row.operandSubject.Available() && row.operandBody.Available() && row.operandReference == (identity.ContentID{})
	case staticquery.StaticOperandTypeValue:
		return row.operandReference.Available() && row.operandBody.Available() && row.operandSubject == (identity.ContentID{})
	default:
		return false
	}
}

type Occurrence struct {
	kind    uint8
	id      identity.ContentID
	body    identity.ContentID
	code    uint64
	points  []identity.ContentID
	inputs  []identity.ContentID
	family  keyspace.Family
	literal keyspace.LiteralValue
	hasLit  bool
}

func (row Occurrence) ID() identity.ContentID { return row.id }
func (row Occurrence) Code() uint64           { return row.code }
func (row Occurrence) Kind() uint8            { return row.kind }
func (row Occurrence) BodyID() (identity.ContentID, bool) {
	return row.body, row.body.Available()
}
func (row Occurrence) PointCount() int { return len(row.points) }
func (row Occurrence) PointAt(index int) (identity.ContentID, bool) {
	if index < 0 || index >= len(row.points) {
		return identity.ContentID{}, false
	}
	return row.points[index], true
}
func (row Occurrence) InputCount() int { return len(row.inputs) }
func (row Occurrence) InputAt(index int) (identity.ContentID, bool) {
	if index < 0 || index >= len(row.inputs) {
		return identity.ContentID{}, false
	}
	return row.inputs[index], true
}
func (row Occurrence) Literal() (keyspace.Family, keyspace.LiteralValue, bool) {
	return row.family, row.literal, row.hasLit
}
func (row Occurrence) ValueSourceSpanID() (identity.ContentID, bool) {
	if row.kind != uint8(programartifact.OccurrenceValueSource) || len(row.inputs) != 1 {
		return identity.ContentID{}, false
	}
	return row.inputs[0], row.inputs[0].Available()
}
func (row Occurrence) BinaryArithmetic() (identity.ContentID, identity.ContentID, flowkind.BinaryOp, bool) {
	if row.kind != uint8(programartifact.OccurrenceBinaryArithmetic) || len(row.inputs) != 2 {
		return identity.ContentID{}, identity.ContentID{}, 0, false
	}
	return row.inputs[0], row.inputs[1], flowkind.BinaryOp(row.code), true
}
func (row Occurrence) BinaryEquality() (identity.ContentID, identity.ContentID, flowkind.BinaryOp, bool) {
	if row.kind != uint8(programartifact.OccurrenceBinaryEquality) || len(row.inputs) < 2 {
		return identity.ContentID{}, identity.ContentID{}, 0, false
	}
	return row.inputs[0], row.inputs[1], flowkind.BinaryOp(row.code), true
}
func (row Occurrence) BinaryOrder() (identity.ContentID, identity.ContentID, flowkind.BinaryOp, bool) {
	if row.kind != uint8(programartifact.OccurrenceBinaryOrder) || len(row.inputs) != 2 {
		return identity.ContentID{}, identity.ContentID{}, 0, false
	}
	return row.inputs[0], row.inputs[1], flowkind.BinaryOp(row.code), true
}
func (row Occurrence) BinaryPresenceRefinement() (identity.ContentID, identity.ContentID, identity.ContentID, identity.ContentID, bool, bool) {
	if row.kind != uint8(programartifact.OccurrenceBinaryPresenceRefinement) || len(row.inputs) != 4 {
		return identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, false, false
	}
	return row.inputs[0], row.inputs[1], row.inputs[2], row.inputs[3], row.code == 1, true
}

func (row Occurrence) OperationPredicateRefinement() (identity.ContentID, identity.ContentID, identity.ContentID, identity.ContentID, uint8, bool, bool) {
	if row.kind != uint8(programartifact.OccurrenceOperationPredicateRefinement) || len(row.inputs) != 4 {
		return identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, 0, false, false
	}
	return row.inputs[0], row.inputs[1], row.inputs[2], row.inputs[3], uint8(row.code & 0xff), row.code&(1<<8) != 0, true
}

type HeapIndex struct{ row cold.HeapIndex }

func (row HeapIndex) Available() bool                    { return row.row.Available() }
func (row HeapIndex) ID() identity.ContentID             { return row.row.ID() }
func (row HeapIndex) BaseSpan() identity.ContentID       { return row.row.BaseSpan() }
func (row HeapIndex) ResultSpan() identity.ContentID     { return row.row.ResultSpan() }
func (row HeapIndex) DynamicKeySpan() identity.ContentID { return row.row.DynamicKeySpan() }
func (row HeapIndex) ExactKey() (uint64, bool)           { return row.row.ExactKey() }
func (row HeapIndex) Read() bool                         { return row.row.Read() }
func (row HeapIndex) ValuesID() identity.ContentID       { return row.row.ValuesID() }
func (row HeapIndex) Values() (identity.ContentID, int, bool) {
	return row.row.Values()
}

type diagnosticBranch struct {
	decision, value identity.ContentID
	points          []identity.ContentID
}

func (payload diagnosticBranch) DecisionPathID() identity.ContentID { return payload.decision }
func (payload diagnosticBranch) ValueSpanID() identity.ContentID    { return payload.value }
func (payload diagnosticBranch) EvidencePoints() ([]identity.ContentID, bool) {
	if len(payload.points) == 0 {
		return nil, false
	}
	return append([]identity.ContentID(nil), payload.points...), true
}

type diagnosticUnresolvedType struct {
	reference, root identity.ContentID
	path            []string
	name            string
}

func (payload diagnosticUnresolvedType) StaticReferenceID() identity.ContentID {
	return payload.reference
}
func (payload diagnosticUnresolvedType) RootID() identity.ContentID { return payload.root }
func (payload diagnosticUnresolvedType) Path() ([]string, bool) {
	if len(payload.path) == 0 {
		return nil, false
	}
	return append([]string(nil), payload.path...), true
}
func (payload diagnosticUnresolvedType) Name() (string, bool) {
	return payload.name, payload.name != ""
}

type diagnosticUnresolvedValue struct {
	read, cell identity.ContentID
	name       string
}

func (payload diagnosticUnresolvedValue) ReadID() identity.ContentID { return payload.read }
func (payload diagnosticUnresolvedValue) CellID() identity.ContentID { return payload.cell }
func (payload diagnosticUnresolvedValue) Name() (string, bool) {
	return payload.name, payload.name != ""
}

type diagnosticConformance struct {
	call, argument, declared, span identity.ContentID
	site                           schemadiag.Site
	position                       uint32
	points                         []identity.ContentID
}

func (payload diagnosticConformance) CallID() identity.ContentID { return payload.call }
func (payload diagnosticConformance) ArgumentID() identity.ContentID {
	return payload.argument
}
func (payload diagnosticConformance) DeclaredStaticTypeID() identity.ContentID {
	return payload.declared
}
func (payload diagnosticConformance) SpanID() identity.ContentID { return payload.span }
func (payload diagnosticConformance) Site() schemadiag.Site      { return payload.site }
func (payload diagnosticConformance) Position() (uint32, bool) {
	return payload.position, payload.call.Available()
}
func (payload diagnosticConformance) EvidencePoints() ([]identity.ContentID, bool) {
	if len(payload.points) == 0 {
		return nil, false
	}
	return append([]identity.ContentID(nil), payload.points...), true
}

type DiagnosticObservation struct {
	id         identity.ContentID
	kind       structure.DiagnosticObservationKind
	location   programsource.Span
	branch     diagnosticBranch
	unresolved diagnosticUnresolvedType
	value      diagnosticUnresolvedValue
	conform    diagnosticConformance
}

func (row DiagnosticObservation) ID() identity.ContentID { return row.id }
func (row DiagnosticObservation) Kind() structure.DiagnosticObservationKind {
	return row.kind
}
func (row DiagnosticObservation) Location() (programsource.Span, bool) {
	if row.location.File == "" || row.location.StartLine == 0 || row.location.StartCol == 0 {
		return programsource.Span{}, false
	}
	return row.location, true
}
func (row DiagnosticObservation) Available() bool {
	if !row.id.Available() || !row.kind.Available() {
		return false
	}
	if _, ok := row.Location(); !ok {
		return false
	}
	switch row.kind {
	case structure.DiagnosticObservationBranchCondition:
		return row.branch.decision.Available() && row.branch.value.Available() && len(row.branch.points) != 0
	case structure.DiagnosticObservationTypeReferenceUnresolved:
		return row.unresolved.reference.Available() && len(row.unresolved.path) != 0
	case structure.DiagnosticObservationValueReferenceUnresolved:
		return row.value.read.Available() && row.value.cell.Available() && row.value.name != ""
	case structure.DiagnosticObservationTypeConformance:
		return row.conform.call.Available() && row.conform.argument.Available() &&
			row.conform.declared.Available() && row.conform.span.Available() && len(row.conform.points) != 0
	default:
		return false
	}
}
func (row DiagnosticObservation) BranchCondition() (diagnosticBranch, bool) {
	if !row.Available() || row.kind != structure.DiagnosticObservationBranchCondition {
		return diagnosticBranch{}, false
	}
	return row.branch, true
}
func (row DiagnosticObservation) UnresolvedTypeReference() (diagnosticUnresolvedType, bool) {
	if !row.Available() || row.kind != structure.DiagnosticObservationTypeReferenceUnresolved {
		return diagnosticUnresolvedType{}, false
	}
	return row.unresolved, true
}
func (row DiagnosticObservation) UnresolvedValueReference() (diagnosticUnresolvedValue, bool) {
	if !row.Available() || row.kind != structure.DiagnosticObservationValueReferenceUnresolved {
		return diagnosticUnresolvedValue{}, false
	}
	return row.value, true
}
func (row DiagnosticObservation) TypeConformance() (diagnosticConformance, bool) {
	if !row.Available() || row.kind != structure.DiagnosticObservationTypeConformance {
		return diagnosticConformance{}, false
	}
	return row.conform, true
}

type Outcome struct {
	id, body identity.ContentID
	kind     uint8
	returns  []identity.ContentID
}

func (row Outcome) ID() identity.ContentID     { return row.id }
func (row Outcome) BodyID() identity.ContentID { return row.body }
func (row Outcome) Kind() uint8                { return row.kind }
func (row Outcome) ReturnValueCount() int      { return len(row.returns) }

func (row FunctionBoundary) ID() identity.ContentID            { return row.id }
func (row FunctionBoundary) BodyID() identity.ContentID        { return row.body }
func (row FunctionBoundary) BodyContextID() identity.ContentID { return row.bodyContext }
func (row FunctionBoundary) EntryID() identity.ContentID       { return row.entry }
func (row FunctionBoundary) CallFormalID() identity.ContentID  { return row.formal }
func (row FunctionBoundary) FormalCount() int                  { return len(row.formals) }
func (row FunctionBoundary) FormalAt(index int) (FunctionFormal, bool) {
	if index < 0 || index >= len(row.formals) {
		return FunctionFormal{}, false
	}
	return row.formals[index], true
}
func (row FunctionBoundary) Vararg() (FunctionVararg, bool) {
	return row.vararg, row.hasVararg && row.vararg.Available()
}
func (row FunctionBoundary) CaptureCount() int { return len(row.captures) }
func (row FunctionBoundary) CaptureAt(index int) (FunctionCapture, bool) {
	if index < 0 || index >= len(row.captures) {
		return FunctionCapture{}, false
	}
	return row.captures[index], true
}
func (row FunctionBoundary) OutcomeCount() int { return len(row.outcomes) }
func (row FunctionBoundary) OutcomeAt(index int) (identity.ContentID, bool) {
	if index < 0 || index >= len(row.outcomes) {
		return identity.ContentID{}, false
	}
	return row.outcomes[index], true
}

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
	frozen, coldCatalog, coldPublished := artifact.ColdPublication()
	if !coldPublished {
		return nil, false
	}
	snapshot := &Snapshot{
		artifactID:  artifact.ID(),
		programID:   artifact.CompileKey().ProgramID(),
		schemaID:    artifact.CompileKey().SchemaDigest(),
		frozen:      frozen,
		coldCatalog: coldCatalog,
		vocabulary:  vocabulary,
	}
	// Every published route arm must name a member of the sealed structural
	// vocabulary. The admission is stated once here over the published plane;
	// the arm each reader receives is projected at the read site.
	edgeCount, edgesPublished := coldCount(snapshot.coldView(), cold.EnvironmentEdgeFamily())
	if !edgesPublished {
		return nil, false
	}
	for index := 0; index < edgeCount; index++ {
		row, held := coldRow(snapshot.coldView(), cold.EnvironmentEdgeFamily(), index)
		if !held {
			return nil, false
		}
		if _, armOK := projectArm(vocabulary, uint16(row.Arm())); !armOK {
			return nil, false
		}
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
	// Every published order bracket must name a member of the sealed
	// structural vocabulary. The admission is stated once here over the
	// published plane; the kind each reader receives is projected at the read
	// site.
	eventCount, eventsPublished := coldCount(snapshot.coldView(), cold.WTOEventFamily())
	if !eventsPublished {
		return nil, false
	}
	for index := 0; index < eventCount; index++ {
		row, held := coldRow(snapshot.coldView(), cold.WTOEventFamily(), index)
		if !held {
			return nil, false
		}
		if _, kindOK := projectEvent(vocabulary, uint16(row.Kind())); !kindOK {
			return nil, false
		}
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
		output, hasOutput := row.OutputSemanticID()
		snapshot.placements = append(snapshot.placements, RulePlacement{
			key: row.Key(), stage: uint8(row.Stage()), point: point, input: input,
			occurrence: row.ID(), predecessor: route, output: output, hasOutput: hasOutput,
			spanResult: programartifact.SpanResultOccurrence(row.OccurrenceKind()),
			inputKind:  uint8(row.InputKind()),
		})
	}
	snapshot.bodies = make([]BodyTransport, 0, artifact.BodyCount())
	bodyIndexByID := make(map[identity.ContentID]int, artifact.BodyCount())
	for index := 0; index < artifact.BodyCount(); index++ {
		row, ok := artifact.BodyAt(index)
		if !ok {
			return nil, false
		}
		bodyIndexByID[row.ID()] = index
		entries, entriesOK := copyIDs(row.EntryPointCount(), row.EntryPointAt)
		exits, exitsOK := lowerBodyExits(artifact, vocabulary, index, row)
		if !entriesOK || !exitsOK {
			return nil, false
		}
		roots := make([]BodyRoot, 0, row.RootCount())
		for rootIndex := 0; rootIndex < row.RootCount(); rootIndex++ {
			root, rootOK := row.RootAt(rootIndex)
			if !rootOK || !root.Available() {
				return nil, false
			}
			roots = append(roots, BodyRoot{id: root.ID(), family: root.Family()})
		}
		function, _ := row.FunctionContextID()
		formal, _ := row.CallFormalID()
		snapshot.bodies = append(snapshot.bodies, BodyTransport{
			id: row.ID(), context: row.ContextID(), entry: row.EntryID(),
			function: function, formal: formal, callable: row.Callable(),
			entries: entries, exits: exits, roots: roots,
		})
	}
	snapshot.boundaries = make([]FunctionBoundary, 0, artifact.FunctionBoundaryCount())
	for index := 0; index < artifact.FunctionBoundaryCount(); index++ {
		row, ok := artifact.FunctionBoundaryAt(index)
		if !ok || !row.Available() {
			return nil, false
		}
		ports := make([]FunctionFormal, 0, row.FormalCount())
		for formalIndex := 0; formalIndex < row.FormalCount(); formalIndex++ {
			port, portOK := row.FormalAt(formalIndex)
			if !portOK || !port.Available() {
				return nil, false
			}
			ports = append(ports, FunctionFormal{id: port.ID(), cell: port.CellID(), storage: port.StorageCellID()})
		}
		captures := make([]FunctionCapture, 0, row.CaptureCount())
		for captureIndex := 0; captureIndex < row.CaptureCount(); captureIndex++ {
			capture, captureOK := row.CaptureAt(captureIndex)
			if !captureOK || !capture.Available() {
				return nil, false
			}
			captures = append(captures, FunctionCapture{
				id: capture.ID(), inner: capture.InnerCellID(), outer: capture.OuterCellID(),
				innerBody: capture.InnerBodyID(), outerBody: capture.OuterBodyID(),
			})
		}
		bodyIndex, bodyKnown := bodyIndexByID[row.BodyID()]
		bodyRow, bodyOK := artifact.BodyAt(bodyIndex)
		if !bodyKnown || !bodyOK {
			return nil, false
		}
		outcomes := make([]identity.ContentID, 0, bodyRow.OutcomeCount())
		for outcomeIndex := 0; outcomeIndex < bodyRow.OutcomeCount(); outcomeIndex++ {
			outcome, outcomeOK := artifact.BodyOutcomeAt(bodyIndex, outcomeIndex)
			if !outcomeOK || !outcome.Available() {
				return nil, false
			}
			outcomes = append(outcomes, outcome.ID())
		}
		vararg, hasVararg := row.Vararg()
		copiedVararg := FunctionVararg{}
		if hasVararg {
			copiedVararg = FunctionVararg{id: vararg.ID(), cell: vararg.CellID()}
		}
		snapshot.boundaries = append(snapshot.boundaries, FunctionBoundary{
			id: row.ID(), body: row.BodyID(), bodyContext: row.BodyContextID(),
			entry: row.EntryID(), formal: row.CallFormalID(), formals: ports,
			vararg: copiedVararg, hasVararg: hasVararg, captures: captures, outcomes: outcomes,
		})
	}
	// Every published call must name operand and argument spans the two child
	// planes actually hold. The admission is stated once here over the
	// published planes; each child row is rejoined at the read site.
	callView := snapshot.coldView()
	callCount, callsPublished := coldCount(callView, cold.CallFamily())
	operandCount, operandsPublished := coldCount(callView, cold.CallOperandFamily())
	argumentCount, argumentsPublished := coldCount(callView, cold.CallArgumentFamily())
	if !callsPublished || !operandsPublished || !argumentsPublished {
		return nil, false
	}
	if _, typeArgumentsPublished := coldCount(callView, cold.CallTypeArgumentFamily()); !typeArgumentsPublished {
		return nil, false
	}
	for index := 0; index < callCount; index++ {
		row, held := coldRow(callView, cold.CallFamily(), index)
		if !held {
			return nil, false
		}
		operandOffset, operandWidth, operandSpanOK := row.OperandSpan()
		argumentOffset, argumentWidth, argumentSpanOK := row.ArgumentSpan()
		if !operandSpanOK || !argumentSpanOK ||
			uint64(operandOffset)+uint64(operandWidth) > uint64(operandCount) ||
			uint64(argumentOffset)+uint64(argumentWidth) > uint64(argumentCount) {
			return nil, false
		}
	}
	snapshot.occurrences = make([]Occurrence, 0, artifact.OccurrenceCount())
	for index := 0; index < artifact.OccurrenceCount(); index++ {
		row, ok := artifact.OccurrenceAt(index)
		if !ok || !row.ID().Available() {
			return nil, false
		}
		inputs, inputsOK := copyIDs(row.InputCount(), row.InputAt)
		points, pointsOK := copyIDs(row.PointCount(), row.PointAt)
		if !inputsOK || !pointsOK {
			return nil, false
		}
		body, _ := row.BodyID()
		family, literal, hasLit := row.Literal()
		snapshot.occurrences = append(snapshot.occurrences, Occurrence{
			kind: uint8(row.Kind()), id: row.ID(), body: body, code: row.Code(), points: points, inputs: inputs,
			family: family, literal: literal, hasLit: hasLit,
		})
	}
	snapshot.staticTypeNodes = make([]StaticTypeNode, 0, artifact.StaticTypeNodeCount())
	for index := 0; index < artifact.StaticTypeNodeCount(); index++ {
		row, ok := artifact.StaticTypeNodeAt(index)
		if !ok || !row.Available() {
			return nil, false
		}
		snapshot.staticTypeNodes = append(snapshot.staticTypeNodes, StaticTypeNode{
			id: row.ID(), owner: row.Owner(), kind: uint8(row.Kind()), literal: row.LiteralKind(),
		})
	}
	snapshot.staticInputs = make([]StaticInput, 0, artifact.StaticInputCount())
	for index := 0; index < artifact.StaticInputCount(); index++ {
		row, ok := artifact.StaticInputAt(index)
		if !ok || !row.Available() {
			return nil, false
		}
		snapshot.staticInputs = append(snapshot.staticInputs, StaticInput{
			id: row.ID(), owner: row.Owner(), expression: row.ExpressionID(), source: row.SourceID(),
			target: row.TargetID(), operand: row.OperandID(), frontier: row.FrontierID(),
			operandReference: row.OperandReferenceID(), operandSubject: row.OperandSubjectID(),
			operandBody: row.OperandBodyPathID(), literal: row.OperandLiteral(),
			kind: uint8(row.Kind()), operandKind: uint8(row.OperandKind()), cursor: row.Cursor(),
		})
	}
	snapshot.outcomes = make([]Outcome, 0, artifact.OutcomeCount())
	for index := 0; index < artifact.OutcomeCount(); index++ {
		row, ok := artifact.OutcomeAt(index)
		if !ok {
			return nil, false
		}
		returns := make([]identity.ContentID, 0, row.ReturnValueCount())
		for valueIndex := 0; valueIndex < row.ReturnValueCount(); valueIndex++ {
			value, valueOK := artifact.OutcomeReturnValueAt(index, valueIndex)
			if !valueOK {
				return nil, false
			}
			returns = append(returns, value.ID())
		}
		snapshot.outcomes = append(snapshot.outcomes, Outcome{id: row.ID(), body: row.BodyID(), kind: uint8(row.Kind()), returns: returns})
	}
	snapshot.observations = make([]DiagnosticObservation, 0, artifact.DiagnosticObservationCount())
	for index := 0; index < artifact.DiagnosticObservationCount(); index++ {
		row, ok := artifact.DiagnosticObservationAt(index)
		if !ok || !row.Available() {
			return nil, false
		}
		lowered, loweredOK := lowerDiagnosticObservation(row)
		if !loweredOK {
			return nil, false
		}
		snapshot.observations = append(snapshot.observations, lowered)
	}
	return snapshot, snapshot.Available()
}

func lowerDiagnosticObservation(row programartifact.DiagnosticObservationRow) (DiagnosticObservation, bool) {
	if !row.Available() {
		return DiagnosticObservation{}, false
	}
	location, locationOK := row.Location()
	if !locationOK {
		return DiagnosticObservation{}, false
	}
	observation := DiagnosticObservation{id: row.ID(), kind: row.Kind(), location: location}
	switch row.Kind() {
	case structure.DiagnosticObservationBranchCondition:
		branch, branchOK := row.BranchCondition()
		points, pointsOK := branch.EvidencePoints()
		if !branchOK || !pointsOK {
			return DiagnosticObservation{}, false
		}
		observation.branch = diagnosticBranch{decision: branch.DecisionPathID(), value: branch.ValueSpanID(), points: append([]identity.ContentID(nil), points...)}
	case structure.DiagnosticObservationTypeReferenceUnresolved:
		unresolved, unresolvedOK := row.UnresolvedTypeReference()
		path, pathOK := unresolved.Path()
		name, nameOK := unresolved.Name()
		if !unresolvedOK || !pathOK || !nameOK {
			return DiagnosticObservation{}, false
		}
		observation.unresolved = diagnosticUnresolvedType{reference: unresolved.StaticReferenceID(), root: unresolved.RootID(), path: append([]string(nil), path...), name: name}
	case structure.DiagnosticObservationValueReferenceUnresolved:
		unresolved, unresolvedOK := row.UnresolvedValueReference()
		name, nameOK := unresolved.Name()
		if !unresolvedOK || !nameOK {
			return DiagnosticObservation{}, false
		}
		observation.value = diagnosticUnresolvedValue{read: unresolved.ReadID(), cell: unresolved.CellID(), name: name}
	case structure.DiagnosticObservationTypeConformance:
		conformance, conformanceOK := row.TypeConformance()
		if !conformanceOK {
			return DiagnosticObservation{}, false
		}
		points, pointsOK := conformance.EvidencePoints()
		position, positionOK := conformance.Position()
		if !pointsOK || !positionOK {
			return DiagnosticObservation{}, false
		}
		observation.conform = diagnosticConformance{
			call: conformance.CallID(), argument: conformance.ArgumentID(),
			declared: conformance.DeclaredStaticTypeID(), span: conformance.SpanID(),
			site: conformance.Site(), position: position, points: append([]identity.ContentID(nil), points...),
		}
	default:
		return DiagnosticObservation{}, false
	}
	return observation, observation.Available()
}
