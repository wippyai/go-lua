// Package ingress lowers a sealed ProgramArtifact into closed immutable
// columns exactly once per ContentID. After Lower succeeds the snapshot
// retains no owner pointer and cannot reopen artifact interiors.
package ingress

import (
	"github.com/wippyai/go-lua/analysis/identity"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/schema/program"
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
	artifactID  identity.ContentID
	programID   identity.ContentID
	schemaID    identity.ContentID
	frozen      snapshotstore.Frozen
	coldCatalog identity.ContentID
	vocabulary  structure.Table
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

func coldCount[V programschema.Row](view coldView, family programschema.Family[V]) (int, bool) {
	if view.frozen == nil {
		return 0, false
	}
	return family.Count(view.frozen, view.catalog)
}

func coldRow[V programschema.Row](view coldView, family programschema.Family[V], index int) (V, bool) {
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

// Frozen returns this snapshot's neutral compiled publication. The publication
// carries no mount identity; a mount row adds its module key at the boundary
// that owns placement.
func (snapshot *Snapshot) Frozen() snapshotstore.Frozen {
	if !snapshot.Available() {
		return snapshotstore.Frozen{}
	}
	return snapshot.frozen
}

// Program returns the one canonical compiled Program named by this ingress
// publication. Families already owned by Program are consumed through this
// value rather than copied into another ingress row vocabulary.
func (snapshot *Snapshot) Program() programschema.Program {
	if !snapshot.Available() {
		return programschema.Program{}
	}
	return programschema.Program{
		Frozen: snapshot.frozen, ArtifactID: snapshot.artifactID,
		ProgramID: snapshot.programID, SchemaID: snapshot.schemaID,
	}
}
func (snapshot *Snapshot) PointCount() int {
	if !snapshot.Available() {
		return 0
	}
	count, published := coldCount(snapshot.coldView(), programschema.PointFamily())
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
	row, held := coldRow(view, programschema.PointFamily(), index)
	if !held {
		return Point{}, false
	}
	return Point{row: row, view: view}, true
}
func (snapshot *Snapshot) StructuralEdgeCount() int {
	if !snapshot.Available() {
		return 0
	}
	count, published := coldCount(snapshot.coldView(), programschema.EnvironmentEdgeFamily())
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
	row, held := coldRow(view, programschema.EnvironmentEdgeFamily(), index)
	if !held {
		return StructuralEdge{}, false
	}
	arm, armOK := projectArm(snapshot.vocabulary, uint16(row.Arm()))
	if !armOK {
		return StructuralEdge{}, false
	}
	return StructuralEdge{row: row, view: view, arm: arm}, true
}
func (snapshot *Snapshot) RegionCount() int {
	if !snapshot.Available() {
		return 0
	}
	count, published := coldCount(snapshot.coldView(), programschema.RegionFamily())
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
	row, held := coldRow(view, programschema.RegionFamily(), index)
	if !held {
		return Region{}, false
	}
	return Region{row: row, view: view}, true
}
func (snapshot *Snapshot) EventCount() int {
	if !snapshot.Available() {
		return 0
	}
	count, published := coldCount(snapshot.coldView(), programschema.WTOEventFamily())
	if !published {
		return 0
	}
	return count
}
func (snapshot *Snapshot) EventAt(index int) (Event, bool) {
	if !snapshot.Available() {
		return Event{}, false
	}
	row, held := coldRow(snapshot.coldView(), programschema.WTOEventFamily(), index)
	if !held {
		return Event{}, false
	}
	kind, kindOK := projectEvent(snapshot.vocabulary, uint16(row.Kind()))
	if !kindOK {
		return Event{}, false
	}
	return Event{row: row, kind: kind}, true
}
func (snapshot *Snapshot) CallArgumentCount() int {
	if !snapshot.Available() {
		return 0
	}
	count, published := coldCount(snapshot.coldView(), programschema.CallArgumentFamily())
	if !published {
		return 0
	}
	return count
}
func (snapshot *Snapshot) CallArgumentAt(index int) (CallArgument, bool) {
	if !snapshot.Available() {
		return CallArgument{}, false
	}
	row, held := coldRow(snapshot.coldView(), programschema.CallArgumentFamily(), index)
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
	count, published := coldCount(snapshot.coldView(), programschema.CallFamily())
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
	row, held := coldRow(view, programschema.CallFamily(), index)
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
	count, published := coldCount(snapshot.coldView(), programschema.HeapAllocationFamily())
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
	row, held := coldRow(view, programschema.HeapAllocationFamily(), index)
	if !held {
		return HeapAllocation{}, false
	}
	return HeapAllocation{row: row, view: view}, true
}
func (snapshot *Snapshot) ValuesCount() int {
	if !snapshot.Available() {
		return 0
	}
	count, published := coldCount(snapshot.coldView(), programschema.ValuesFamily())
	if !published {
		return 0
	}
	return count
}
func (snapshot *Snapshot) StaticTypeValueCount() int {
	if !snapshot.Available() {
		return 0
	}
	count, published := coldCount(snapshot.coldView(), programschema.StaticTypeValueFamily())
	if !published {
		return 0
	}
	return count
}
func (snapshot *Snapshot) StaticTypeValueAt(index int) (StaticTypeValue, bool) {
	if !snapshot.Available() {
		return StaticTypeValue{}, false
	}
	row, held := coldRow(snapshot.coldView(), programschema.StaticTypeValueFamily(), index)
	if !held {
		return StaticTypeValue{}, false
	}
	return StaticTypeValue{row: row}, true
}
func (snapshot *Snapshot) StaticTypeArgumentCount() int {
	if !snapshot.Available() {
		return 0
	}
	count, published := coldCount(snapshot.coldView(), programschema.CallTypeArgumentFamily())
	if !published {
		return 0
	}
	return count
}
func (snapshot *Snapshot) StaticTypeArgumentAt(index int) (StaticTypeArgument, bool) {
	if !snapshot.Available() {
		return StaticTypeArgument{}, false
	}
	row, held := coldRow(snapshot.coldView(), programschema.CallTypeArgumentFamily(), index)
	if !held {
		return StaticTypeArgument{}, false
	}
	return StaticTypeArgument{row: row}, true
}
func (snapshot *Snapshot) StaticExpressionCount() int {
	if !snapshot.Available() {
		return 0
	}
	count, published := coldCount(snapshot.coldView(), programschema.StaticExpressionFamily())
	if !published {
		return 0
	}
	return count
}
func (snapshot *Snapshot) StaticExpressionAt(index int) (StaticExpression, bool) {
	if !snapshot.Available() {
		return StaticExpression{}, false
	}
	row, held := coldRow(snapshot.coldView(), programschema.StaticExpressionFamily(), index)
	if !held {
		return StaticExpression{}, false
	}
	return StaticExpression{row: row}, true
}
func (snapshot *Snapshot) ValuesAt(index int) (Values, bool) {
	if !snapshot.Available() {
		return Values{}, false
	}
	view := snapshot.coldView()
	row, held := coldRow(view, programschema.ValuesFamily(), index)
	if !held {
		return Values{}, false
	}
	return Values{row: row, view: view}, true
}
func (snapshot *Snapshot) HeapIndexCount() int {
	if !snapshot.Available() {
		return 0
	}
	count, published := coldCount(snapshot.coldView(), programschema.HeapIndexFamily())
	if !published {
		return 0
	}
	return count
}
func (snapshot *Snapshot) HeapIndexAt(index int) (HeapIndex, bool) {
	if !snapshot.Available() {
		return HeapIndex{}, false
	}
	row, held := coldRow(snapshot.coldView(), programschema.HeapIndexFamily(), index)
	if !held {
		return HeapIndex{}, false
	}
	return HeapIndex{row: row}, true
}

type Point struct {
	row  programschema.Point
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
	decision, held := coldRow(row.view, programschema.PointDecisionFamily(), int(offset)+index)
	if !held {
		return identity.ContentID{}, false
	}
	return decision.ID(), true
}

type StructuralEdge struct {
	row  programschema.EnvironmentEdge
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
	witness, held := coldRow(row.view, programschema.EnvironmentResetFamily(), int(offset)+index)
	if !held {
		return identity.ContentID{}, false
	}
	return witness.ID(), true
}

type Region struct {
	row  programschema.Region
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
	member, held := coldRow(row.view, programschema.RegionMemberFamily(), int(offset)+index)
	if !held {
		return identity.ContentID{}, false
	}
	return member.ID(), true
}

// Event is one order bracket read out of the published plane. Its kind is the
// sealed structural vocabulary's member, projected at the read site.
type Event struct {
	row  programschema.WTOEvent
	kind EventKind
}

func (row Event) Kind() EventKind              { return row.kind }
func (row Event) RegionID() identity.ContentID { return row.row.RegionID() }
func (row Event) PointID() identity.ContentID  { return row.row.PointID() }

type CallOperand struct{ row programschema.CallOperand }

func (row CallOperand) ID() identity.ContentID      { return row.row.ID() }
func (row CallOperand) CallID() identity.ContentID  { return row.row.CallID() }
func (row CallOperand) ValueID() identity.ContentID { return row.row.ValueID() }
func (row CallOperand) SpanID() identity.ContentID  { return row.row.SpanID() }
func (row CallOperand) Callee() bool                { return row.row.Kind() == programschema.CallOperandCallee }

type CallArgument struct{ row programschema.CallArgument }

func (row CallArgument) Available() bool              { return row.row.Available() }
func (row CallArgument) ID() identity.ContentID       { return row.row.ID() }
func (row CallArgument) CallID() identity.ContentID   { return row.row.CallID() }
func (row CallArgument) ValuesID() identity.ContentID { return row.row.ValuesID() }
func (row CallArgument) MemberID() identity.ContentID { return row.row.MemberID() }
func (row CallArgument) ValueID() identity.ContentID  { return row.row.MemberID() }
func (row CallArgument) SpanID() identity.ContentID   { return row.row.SpanID() }
func (row CallArgument) Index() uint32                { return row.row.Index() }

type Call struct {
	row  programschema.Call
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
	operand, held := coldRow(row.view, programschema.CallOperandFamily(), int(offset)+index)
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
	argument, held := coldRow(row.view, programschema.CallArgumentFamily(), int(offset)+index)
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

type HeapField struct{ row programschema.HeapField }

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
	row  programschema.HeapAllocation
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
	field, held := coldRow(row.view, programschema.HeapFieldFamily(), int(offset)+index)
	if !held {
		return HeapField{}, false
	}
	return HeapField{row: field}, true
}

type ValuesMember struct{ row programschema.ValuesMember }

func newValuesMember(id identity.ContentID) (ValuesMember, bool) {
	row, ok := programschema.NewValuesMember(id)
	if !ok {
		return ValuesMember{}, false
	}
	return ValuesMember{row: row}, true
}

func (row ValuesMember) ID() identity.ContentID { return row.row.ID() }

type ValuesTail struct{ row programschema.ValuesTail }

func (row ValuesTail) Present() bool          { return row.row.Present() }
func (row ValuesTail) ID() identity.ContentID { return row.row.ID() }
func (row ValuesTail) Kind() uint8            { return uint8(row.row.Kind()) }

type Values struct {
	row  programschema.Values
	view coldView
}

func (row Values) ID() identity.ContentID { return row.row.ID() }
func (row Values) MemberCount() int       { return row.row.MemberCount() }
func (row Values) MemberAt(index int) (ValuesMember, bool) {
	offset, count, spanOK := row.row.MemberSpan()
	if !spanOK || index < 0 || uint64(index) >= uint64(count) {
		return ValuesMember{}, false
	}
	member, held := coldRow(row.view, programschema.ValuesMemberFamily(), int(offset)+index)
	if !held {
		return ValuesMember{}, false
	}
	return ValuesMember{row: member}, true
}
func (row Values) Tail() (ValuesTail, bool) {
	tail, present := row.row.Tail()
	return ValuesTail{row: tail}, present
}

type StaticTypeValue struct{ row programschema.StaticTypeValue }

func (row StaticTypeValue) ID() identity.ContentID          { return row.row.ID() }
func (row StaticTypeValue) BodyPathID() identity.ContentID  { return row.row.BodyPathID() }
func (row StaticTypeValue) ReferenceID() identity.ContentID { return row.row.ReferenceID() }
func (row StaticTypeValue) RootID() identity.ContentID      { return row.row.RootID() }
func (row StaticTypeValue) Name() string                    { return row.row.Name() }
func (row StaticTypeValue) Available() bool                 { return row.row.Available() }

type StaticTypeArgument struct {
	row programschema.CallTypeArgument
}

func (row StaticTypeArgument) ID() identity.ContentID          { return row.row.ID() }
func (row StaticTypeArgument) CallID() identity.ContentID      { return row.row.CallID() }
func (row StaticTypeArgument) TypesID() identity.ContentID     { return row.row.TypesID() }
func (row StaticTypeArgument) ReferenceID() identity.ContentID { return row.row.ReferenceID() }
func (row StaticTypeArgument) Index() uint32                   { return row.row.Index() }
func (row StaticTypeArgument) Available() bool                 { return row.row.Available() }

type StaticExpression struct {
	row programschema.StaticExpression
}

func (row StaticExpression) ID() identity.ContentID          { return row.row.ID() }
func (row StaticExpression) ReferenceID() identity.ContentID { return row.row.ReferenceID() }
func (row StaticExpression) Owner() identity.ContentID       { return row.row.Owner() }
func (row StaticExpression) Available() bool                 { return row.row.Available() }

type HeapIndex struct{ row programschema.HeapIndex }

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
	program := artifact.Program()
	if !program.Available() {
		return nil, false
	}
	snapshot := &Snapshot{
		artifactID:  artifact.ID(),
		programID:   program.ProgramID,
		schemaID:    program.SchemaID,
		frozen:      frozen,
		coldCatalog: coldCatalog,
		vocabulary:  vocabulary,
	}
	// Every published route arm must name a member of the sealed structural
	// vocabulary. The admission is stated once here over the published plane;
	// the arm each reader receives is projected at the read site.
	edgeCount, edgesPublished := coldCount(snapshot.coldView(), programschema.EnvironmentEdgeFamily())
	if !edgesPublished {
		return nil, false
	}
	for index := 0; index < edgeCount; index++ {
		row, held := coldRow(snapshot.coldView(), programschema.EnvironmentEdgeFamily(), index)
		if !held {
			return nil, false
		}
		if _, armOK := projectArm(vocabulary, uint16(row.Arm())); !armOK {
			return nil, false
		}
	}
	// Every published order bracket must name a member of the sealed
	// structural vocabulary. The admission is stated once here over the
	// published plane; the kind each reader receives is projected at the read
	// site.
	eventCount, eventsPublished := coldCount(snapshot.coldView(), programschema.WTOEventFamily())
	if !eventsPublished {
		return nil, false
	}
	for index := 0; index < eventCount; index++ {
		row, held := coldRow(snapshot.coldView(), programschema.WTOEventFamily(), index)
		if !held {
			return nil, false
		}
		if _, kindOK := projectEvent(vocabulary, uint16(row.Kind())); !kindOK {
			return nil, false
		}
	}
	ruleCount, rulesPublished := program.RuleOccurrenceCount()
	occurrenceCount, occurrencesPublished := program.OccurrenceCount()
	occurrencePointCount, occurrencePointsPublished := programschema.OccurrencePointFamily().Count(&snapshot.frozen, snapshot.coldCatalog)
	occurrenceInputCount, occurrenceInputsPublished := programschema.OccurrenceInputFamily().Count(&snapshot.frozen, snapshot.coldCatalog)
	if !rulesPublished || !occurrencesPublished || !occurrencePointsPublished || !occurrenceInputsPublished {
		return nil, false
	}
	for index := 0; index < occurrenceCount; index++ {
		row, ok := program.OccurrenceAt(index)
		pointOffset, pointWidth, pointSpanOK := row.PointSpan()
		inputOffset, inputWidth, inputSpanOK := row.InputSpan()
		if !ok || !row.Available() || !pointSpanOK || !inputSpanOK || uint64(pointOffset)+uint64(pointWidth) > uint64(occurrencePointCount) || uint64(inputOffset)+uint64(inputWidth) > uint64(occurrenceInputCount) {
			return nil, false
		}
	}
	for index := 0; index < ruleCount; index++ {
		row, ok := program.RuleOccurrenceAt(index)
		if !ok || !row.Available() {
			return nil, false
		}
		parent, parentOK := row.Occurrence()
		if !parentOK || uint64(parent) >= uint64(occurrenceCount) {
			return nil, false
		}
	}
	// Every published call must name operand and argument spans the two child
	// planes actually hold. The admission is stated once here over the
	// published planes; each child row is rejoined at the read site.
	callView := snapshot.coldView()
	callCount, callsPublished := coldCount(callView, programschema.CallFamily())
	operandCount, operandsPublished := coldCount(callView, programschema.CallOperandFamily())
	argumentCount, argumentsPublished := coldCount(callView, programschema.CallArgumentFamily())
	if !callsPublished || !operandsPublished || !argumentsPublished {
		return nil, false
	}
	if _, typeArgumentsPublished := coldCount(callView, programschema.CallTypeArgumentFamily()); !typeArgumentsPublished {
		return nil, false
	}
	for index := 0; index < callCount; index++ {
		row, held := coldRow(callView, programschema.CallFamily(), index)
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
	return snapshot, snapshot.Available()
}
