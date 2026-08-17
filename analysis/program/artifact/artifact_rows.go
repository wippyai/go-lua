package artifact

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
)

type Artifact struct {
	key                    CompileKey
	id                     identity.ContentID
	sealed                 identity.ContentID
	counts                 denominator.CountRows
	pointAttachments       []PointAttachmentRow
	points                 []Point
	environment            []EnvironmentEdge
	localTransfers         []LocalTransfer
	regions                []Region
	events                 []WTOEvent
	values                 []ValuesRow
	calls                  []CallRow
	callOperands           []CallOperandRow
	callArguments          []CallArgumentRow
	callTypeArguments      []CallTypeArgumentRow
	bodies                 []BodyRow
	functionBoundaries     []FunctionBoundaryRow
	callTargets            []CallTargetRow
	boundaries             []BoundaryRow
	outcomes               []OutcomeRow
	returnValues           []ReturnValue
	occurrences            []OccurrenceRow
	exactScalarSummaries   []ExactScalarSummaryRow
	arithmeticSummaries    []ArithmeticSummaryRow
	unarySummaries         []UnarySummaryRow
	heapAllocations        []HeapAllocationRow
	heapIndexes            []HeapIndexRow
	occurrenceByID         map[occurrenceLookup]uint32
	ruleOccurrences        map[RuleRole][]RuleOccurrence
	diagnosticObservations []DiagnosticObservationRow
	staticTypeArguments    []StaticTypeArgumentRow
	staticTypeValues       []StaticTypeValueRow
	staticTypeNodes        []StaticTypeNodeRow
	staticExpressions      []StaticExpressionRow
	staticInputs           []StaticInputRow
	occurrenceByKind       map[OccurrenceKind][]uint32
	functionBoundaryByBody map[identity.ContentID]uint32
}

// HeapFieldRow is the immutable scalar constructor geometry captured while
// the Program proof is live.  Terms are cold source coordinates solely for
// Link substitution; no Program, Flow, or domain value escapes.
type HeapFieldRow struct {
	id                   identity.ContentID
	kind                 flowkind.FieldKind
	fieldSpan            identity.ContentID
	selectorSpan         identity.ContentID
	valuesSpan           identity.ContentID
	valuesID             identity.ContentID
	width                int
	finalOpen            bool
	sharesFirstValueCell bool
	normalized           keyspace.Key
	normalizedOK         bool
}

func (row HeapFieldRow) Available() bool {
	return row.id.Available() && row.kind >= flowkind.FieldList && row.kind <= flowkind.FieldKey && row.fieldSpan.Available() && row.valuesSpan.Available() && row.valuesID.Available() && row.width >= 0 && (row.kind == flowkind.FieldKey) == row.selectorSpan.Available() && (row.kind == flowkind.FieldKey || !row.sharesFirstValueCell)
}
func (row HeapFieldRow) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}
func (row HeapFieldRow) Kind() flowkind.FieldKind {
	if !row.Available() {
		return 0
	}
	return row.kind
}
func (row HeapFieldRow) FieldSpan() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.fieldSpan
}
func (row HeapFieldRow) SelectorSpan() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.selectorSpan
}
func (row HeapFieldRow) Values() (identity.ContentID, int, bool, bool) {
	if !row.Available() {
		return identity.ContentID{}, 0, false, false
	}
	return row.valuesSpan, row.width, row.finalOpen, true
}

// Width returns the exact authored Values member count copied into this
// field's constructor geometry.
func (row HeapFieldRow) Width() int {
	if !row.Available() {
		return 0
	}
	return row.width
}

// FinalOpen reports whether this field's Values row has an open tail.
func (row HeapFieldRow) FinalOpen() bool {
	return row.Available() && row.finalOpen
}
func (row HeapFieldRow) ValuesID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.valuesID
}

// SharesFirstValueCell preserves the exact dynamic FieldKey relation needed
// by Heap's closed-constructor descriptor without retaining raw read terms.
func (row HeapFieldRow) SharesFirstValueCell() bool {
	return row.Available() && row.sharesFirstValueCell
}
func (row HeapFieldRow) NormalizedKey() (keyspace.Key, bool) {
	if !row.Available() {
		return 0, false
	}
	return row.normalized, row.normalizedOK
}

// HeapAllocationRow is one allocation template plus its ordered field
// geometry. It is neutral source data consumed by Heap at Link binding time.
type HeapAllocationRow struct {
	id       identity.ContentID
	role     flow.AllocationRole
	form     flow.AllocationForm
	rootSpan identity.ContentID
	fields   []HeapFieldRow
}

func (row HeapAllocationRow) Available() bool {
	if !row.id.Available() || !row.role.Valid() || !row.form.Valid() || !row.rootSpan.Available() {
		return false
	}
	for _, field := range row.fields {
		if !field.Available() {
			return false
		}
	}
	return true
}
func (row HeapAllocationRow) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}

func (row HeapAllocationRow) Role() flow.AllocationRole {
	if !row.Available() {
		return flow.AllocationInvalid
	}
	return row.role
}

func (row HeapAllocationRow) Form() flow.AllocationForm {
	if !row.Available() {
		return flow.AllocationFormInvalid
	}
	return row.form
}
func (row HeapAllocationRow) RootSpan() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.rootSpan
}
func (row HeapAllocationRow) FieldCount() int {
	if !row.Available() {
		return 0
	}
	return len(row.fields)
}
func (row HeapAllocationRow) FieldAt(index int) (HeapFieldRow, bool) {
	if !row.Available() || index < 0 || index >= len(row.fields) {
		return HeapFieldRow{}, false
	}
	return row.fields[index], true
}

// HeapIndexRow is one scalar IndexRead/IndexWrite candidate. A false Read
// denotes a write and therefore carries Values and its exact position.
type HeapIndexRow struct {
	id         identity.ContentID
	read       bool
	baseSpan   identity.ContentID
	resultSpan identity.ContentID
	keySpan    identity.ContentID
	lensKind   uint8
	exactKey   keyspace.Key
	valuesSpan identity.ContentID
	valuesID   identity.ContentID
	position   int
}

func (row HeapIndexRow) Available() bool {
	if !row.id.Available() || !row.baseSpan.Available() || row.lensKind == 0 || row.lensKind > 2 {
		return false
	}
	if row.lensKind == 1 && row.exactKey == 0 || row.lensKind == 2 && !row.keySpan.Available() {
		return false
	}
	return row.read && row.resultSpan.Available() && !row.valuesSpan.Available() && !row.valuesID.Available() && row.position == -1 || !row.read && !row.resultSpan.Available() && row.valuesSpan.Available() && row.valuesID.Available() && row.position >= 0
}
func (row HeapIndexRow) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}
func (row HeapIndexRow) Read() bool { return row.Available() && row.read }
func (row HeapIndexRow) BaseSpan() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.baseSpan
}
func (row HeapIndexRow) ResultSpan() identity.ContentID {
	if !row.Available() || !row.read {
		return identity.ContentID{}
	}
	return row.resultSpan
}
func (row HeapIndexRow) DynamicKeySpan() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.keySpan
}
func (row HeapIndexRow) ExactKey() (keyspace.Key, bool) {
	if !row.Available() || row.lensKind != 1 {
		return 0, false
	}
	return row.exactKey, true
}
func (row HeapIndexRow) Values() (identity.ContentID, int, bool) {
	if !row.Available() || row.read {
		return identity.ContentID{}, 0, false
	}
	return row.valuesSpan, row.position, true
}
func (row HeapIndexRow) ValuesID() identity.ContentID {
	if !row.Available() || row.read {
		return identity.ContentID{}
	}
	return row.valuesID
}

func (artifact *Artifact) Available() bool {
	return artifact != nil && artifact.key.Available() && artifact.id.Available() && artifact.counts.Available() && artifact.sealed == artifact.id
}

func (artifact *Artifact) CompileKey() CompileKey {
	if !artifact.Available() {
		return CompileKey{}
	}
	return artifact.key
}

func (artifact *Artifact) ID() identity.ContentID {
	if !artifact.Available() {
		return identity.ContentID{}
	}
	return artifact.id
}

// CountRows returns the immutable Program denominator rows frozen into this
// artifact. The rows are keyed by schema EntryID and contain no owner payload.
func (artifact *Artifact) CountRows() denominator.CountRows {
	if !artifact.Available() {
		return denominator.CountRows{}
	}
	return artifact.counts
}

func (artifact *Artifact) PointCount() int {
	if !artifact.Available() {
		return 0
	}
	return len(artifact.points)
}

// PointAttachmentCount returns the exact immutable Site-to-LocalWTO point
// column copied during artifact compilation.
func (artifact *Artifact) PointAttachmentCount() int {
	if !artifact.Available() {
		return 0
	}
	return len(artifact.pointAttachments)
}

// PointAttachmentAt returns one ordered immutable point-attachment row.
func (artifact *Artifact) PointAttachmentAt(index int) (PointAttachmentRow, bool) {
	if !artifact.Available() || index < 0 || index >= len(artifact.pointAttachments) {
		return PointAttachmentRow{}, false
	}
	return artifact.pointAttachments[index], true
}

func (artifact *Artifact) EnvironmentEdgeCount() int {
	if !artifact.Available() {
		return 0
	}
	return len(artifact.environment)
}

func (artifact *Artifact) LocalTransferCount() int {
	if !artifact.Available() {
		return 0
	}
	return len(artifact.localTransfers)
}

func (artifact *Artifact) DiagnosticObservationCount() int {
	if !artifact.Available() {
		return 0
	}
	return len(artifact.diagnosticObservations)
}

func (artifact *Artifact) DiagnosticObservationAt(index int) (DiagnosticObservationRow, bool) {
	if !artifact.Available() || index < 0 || index >= len(artifact.diagnosticObservations) {
		return DiagnosticObservationRow{}, false
	}
	return artifact.diagnosticObservations[index], true
}

// StaticTypeArgumentCount and StaticTypeArgumentAt expose the closed
// Program-owned type-argument formal plane to mounted Static authorities.
func (artifact *Artifact) StaticTypeArgumentCount() int {
	if !artifact.Available() {
		return 0
	}
	return len(artifact.staticTypeArguments)
}
func (artifact *Artifact) StaticTypeArgumentAt(index int) (StaticTypeArgumentRow, bool) {
	if !artifact.Available() || index < 0 || index >= len(artifact.staticTypeArguments) {
		return StaticTypeArgumentRow{}, false
	}
	return artifact.staticTypeArguments[index], true
}

// StaticTypeValueCount and StaticTypeValueAt expose executable TypeValue
// source rows without exporting the authored source coordinate.
func (artifact *Artifact) StaticTypeValueCount() int {
	if !artifact.Available() {
		return 0
	}
	return len(artifact.staticTypeValues)
}
func (artifact *Artifact) StaticTypeValueAt(index int) (StaticTypeValueRow, bool) {
	if !artifact.Available() || index < 0 || index >= len(artifact.staticTypeValues) {
		return StaticTypeValueRow{}, false
	}
	return artifact.staticTypeValues[index], true
}

func (artifact *Artifact) StaticTypeNodeCount() int {
	if !artifact.Available() {
		return 0
	}
	return len(artifact.staticTypeNodes)
}
func (artifact *Artifact) StaticTypeNodeAt(index int) (StaticTypeNodeRow, bool) {
	if !artifact.Available() || index < 0 || index >= len(artifact.staticTypeNodes) {
		return StaticTypeNodeRow{}, false
	}
	return artifact.staticTypeNodes[index], true
}
func (artifact *Artifact) StaticExpressionCount() int {
	if !artifact.Available() {
		return 0
	}
	return len(artifact.staticExpressions)
}
func (artifact *Artifact) StaticExpressionAt(index int) (StaticExpressionRow, bool) {
	if !artifact.Available() || index < 0 || index >= len(artifact.staticExpressions) {
		return StaticExpressionRow{}, false
	}
	return artifact.staticExpressions[index], true
}

func (artifact *Artifact) StaticExpressionByID(id identity.ContentID) (StaticExpressionRow, bool) {
	if artifact == nil || !id.Available() {
		return StaticExpressionRow{}, false
	}
	for _, row := range artifact.staticExpressions {
		if row.id == id {
			return row, true
		}
	}
	return StaticExpressionRow{}, false
}
func (artifact *Artifact) StaticInputCount() int {
	if !artifact.Available() {
		return 0
	}
	return len(artifact.staticInputs)
}
func (artifact *Artifact) StaticInputAt(index int) (StaticInputRow, bool) {
	if !artifact.Available() || index < 0 || index >= len(artifact.staticInputs) {
		return StaticInputRow{}, false
	}
	return artifact.staticInputs[index], true
}

func (artifact *Artifact) RegionCount() int {
	if !artifact.Available() {
		return 0
	}
	return len(artifact.regions)
}

func (artifact *Artifact) WTOEventCount() int {
	if !artifact.Available() {
		return 0
	}
	return len(artifact.events)
}

// ValuesCount returns the exact immutable Program Values denominator copied
// into this artifact.
func (artifact *Artifact) ValuesCount() int {
	if !artifact.Available() {
		return 0
	}
	return len(artifact.values)
}

// HeapAllocationCount and HeapAllocationAt expose the exact reusable
// allocation geometry for Link-local Heap substitution.
func (artifact *Artifact) HeapAllocationCount() int {
	if !artifact.Available() {
		return 0
	}
	return len(artifact.heapAllocations)
}
func (artifact *Artifact) HeapAllocationAt(index int) (HeapAllocationRow, bool) {
	if !artifact.Available() || index < 0 || index >= len(artifact.heapAllocations) {
		return HeapAllocationRow{}, false
	}
	return artifact.heapAllocations[index], true
}

// HeapIndexCount and HeapIndexAt expose the exact reusable access geometry
// for Link-local Heap substitution.
func (artifact *Artifact) HeapIndexCount() int {
	if !artifact.Available() {
		return 0
	}
	return len(artifact.heapIndexes)
}
func (artifact *Artifact) HeapIndexAt(index int) (HeapIndexRow, bool) {
	if !artifact.Available() || index < 0 || index >= len(artifact.heapIndexes) {
		return HeapIndexRow{}, false
	}
	return artifact.heapIndexes[index], true
}

// ValuesAt returns one immutable Values row in authored denominator order.
// It never exposes the backing slice.
func (artifact *Artifact) ValuesAt(index int) (ValuesRow, bool) {
	if !artifact.Available() || index < 0 || index >= len(artifact.values) {
		return ValuesRow{}, false
	}
	return artifact.values[index], true
}

// CallCount and CallAt expose the complete immutable authored-call
// denominator. Rows are retained in the same authored order as Flow.Calls;
// no executable-only compaction is performed at this boundary.
func (artifact *Artifact) CallCount() int {
	if !artifact.Available() {
		return 0
	}
	return len(artifact.calls)
}
func (artifact *Artifact) CallAt(index int) (CallRow, bool) {
	if !artifact.Available() || index < 0 || index >= len(artifact.calls) {
		return CallRow{}, false
	}
	return artifact.calls[index], artifact.calls[index].Available()
}
func (artifact *Artifact) CallOperandCount() int {
	if !artifact.Available() {
		return 0
	}
	return len(artifact.callOperands)
}
func (artifact *Artifact) CallOperandAt(index int) (CallOperandRow, bool) {
	if !artifact.Available() || index < 0 || index >= len(artifact.callOperands) {
		return CallOperandRow{}, false
	}
	return artifact.callOperands[index], artifact.callOperands[index].Available()
}
func (artifact *Artifact) CallArgumentCount() int {
	if !artifact.Available() {
		return 0
	}
	return len(artifact.callArguments)
}
func (artifact *Artifact) CallArgumentAt(index int) (CallArgumentRow, bool) {
	if !artifact.Available() || index < 0 || index >= len(artifact.callArguments) {
		return CallArgumentRow{}, false
	}
	return artifact.callArguments[index], artifact.callArguments[index].Available()
}
func (artifact *Artifact) CallTypeArgumentCount() int {
	if !artifact.Available() {
		return 0
	}
	return len(artifact.callTypeArguments)
}
func (artifact *Artifact) CallTypeArgumentAt(index int) (CallTypeArgumentRow, bool) {
	if !artifact.Available() || index < 0 || index >= len(artifact.callTypeArguments) {
		return CallTypeArgumentRow{}, false
	}
	return artifact.callTypeArguments[index], artifact.callTypeArguments[index].Available()
}

// CallOperandFor, CallArgumentFor, and CallTypeArgumentFor resolve one
// position in a Call's closed ordered child columns. They validate the row's
// private ranges before returning a child, so callers cannot splice a child
// from another call or mount.
func (artifact *Artifact) CallOperandFor(callIndex, childIndex int) (CallOperandRow, bool) {
	call, ok := artifact.CallAt(callIndex)
	if !ok || childIndex < 0 || childIndex >= call.OperandCount() {
		return CallOperandRow{}, false
	}
	return artifact.CallOperandAt(int(call.operandStart) + childIndex)
}
func (artifact *Artifact) CallArgumentFor(callIndex, childIndex int) (CallArgumentRow, bool) {
	call, ok := artifact.CallAt(callIndex)
	if !ok || childIndex < 0 || childIndex >= call.ArgumentCount() {
		return CallArgumentRow{}, false
	}
	return artifact.CallArgumentAt(int(call.argumentStart) + childIndex)
}
func (artifact *Artifact) CallTypeArgumentFor(callIndex, childIndex int) (CallTypeArgumentRow, bool) {
	call, ok := artifact.CallAt(callIndex)
	if !ok || childIndex < 0 || childIndex >= call.TypeArgumentCount() {
		return CallTypeArgumentRow{}, false
	}
	return artifact.CallTypeArgumentAt(int(call.typeArgumentStart) + childIndex)
}

// BodyCount returns Source's exact immutable Body denominator.
func (artifact *Artifact) BodyCount() int {
	if !artifact.Available() {
		return 0
	}
	return len(artifact.bodies)
}

func (artifact *Artifact) BodyAt(index int) (BodyRow, bool) {
	if !artifact.Available() || index < 0 || index >= len(artifact.bodies) {
		return BodyRow{}, false
	}
	return artifact.bodies[index], true
}

// FunctionBoundaryCount and FunctionBoundaryAt expose the sole neutral
// callable-interface denominator. The rows are content-addressed Program
// artifact data; Link supplies only mounted actual/callback substitutions.
func (artifact *Artifact) FunctionBoundaryCount() int {
	if !artifact.Available() {
		return 0
	}
	return len(artifact.functionBoundaries)
}
func (artifact *Artifact) FunctionBoundaryAt(index int) (FunctionBoundaryRow, bool) {
	if !artifact.Available() || index < 0 || index >= len(artifact.functionBoundaries) {
		return FunctionBoundaryRow{}, false
	}
	return artifact.functionBoundaries[index], true
}

// FunctionBoundaryForBody resolves the sole callable boundary owned by a
// Body. The inverse is sealed with the Artifact and keeps consumers from
// rebuilding a body-to-function map beside the canonical column.
func (artifact *Artifact) FunctionBoundaryForBody(bodyID identity.ContentID) (FunctionBoundaryRow, bool) {
	if artifact == nil || !artifact.Available() || !bodyID.Available() || artifact.functionBoundaryByBody == nil {
		return FunctionBoundaryRow{}, false
	}
	index, ok := artifact.functionBoundaryByBody[bodyID]
	if !ok || uint64(index) >= uint64(len(artifact.functionBoundaries)) {
		return FunctionBoundaryRow{}, false
	}
	row := artifact.functionBoundaries[index]
	return row, row.Available() && row.BodyID() == bodyID
}

// CallTargetCount is the exact closure-allocation denominator captured while
// the Program proof was live.  It is separate from BodyCount because only
// executable function bodies are Call targets.
func (artifact *Artifact) CallTargetCount() int {
	if !artifact.Available() {
		return 0
	}
	return len(artifact.callTargets)
}

// CallTargetAt returns one immutable allocation-to-body target proof.  It
// exposes IDs only; no Program, transformer coordinate, or allocation handle
// can escape the artifact boundary.
func (artifact *Artifact) CallTargetAt(index int) (CallTargetRow, bool) {
	if !artifact.Available() || index < 0 || index >= len(artifact.callTargets) {
		return CallTargetRow{}, false
	}
	return artifact.callTargets[index], true
}

// BoundaryCount and BoundaryAt expose the exact Program boundary rows copied
// during compilation. Link consumers substitute their own mounted authority.
func (artifact *Artifact) BoundaryCount() int {
	if !artifact.Available() {
		return 0
	}
	return len(artifact.boundaries)
}
func (artifact *Artifact) BoundaryAt(index int) (BoundaryRow, bool) {
	if !artifact.Available() || index < 0 || index >= len(artifact.boundaries) {
		return BoundaryRow{}, false
	}
	return artifact.boundaries[index], true
}

// OutcomeCount returns the complete flattened Body Outcome denominator in
// parent Body/per-Body order.
func (artifact *Artifact) OutcomeCount() int {
	if !artifact.Available() {
		return 0
	}
	return len(artifact.outcomes)
}

func (artifact *Artifact) OutcomeAt(index int) (OutcomeRow, bool) {
	if !artifact.Available() || index < 0 || index >= len(artifact.outcomes) {
		return OutcomeRow{}, false
	}
	return artifact.outcomes[index], true
}

// BodyOutcomeAt indexes one Body's exact ordered Outcome range without
// exposing the backing range or slice.
func (artifact *Artifact) BodyOutcomeAt(bodyIndex, outcomeIndex int) (OutcomeRow, bool) {
	if !artifact.Available() || bodyIndex < 0 || bodyIndex >= len(artifact.bodies) || outcomeIndex < 0 {
		return OutcomeRow{}, false
	}
	body := artifact.bodies[bodyIndex]
	if !body.Available() || uint64(outcomeIndex) >= uint64(body.outcomeEnd-body.outcomeStart) {
		return OutcomeRow{}, false
	}
	index := uint64(body.outcomeStart) + uint64(outcomeIndex)
	if index >= uint64(len(artifact.outcomes)) {
		return OutcomeRow{}, false
	}
	return artifact.outcomes[index], true
}

// OutcomeReturnValueAt returns one ordered Values occurrence reference for a
// Return Outcome without exposing the flat backing plane.
func (artifact *Artifact) OutcomeReturnValueAt(outcomeIndex, valueIndex int) (ReturnValue, bool) {
	if !artifact.Available() || outcomeIndex < 0 || outcomeIndex >= len(artifact.outcomes) || valueIndex < 0 {
		return ReturnValue{}, false
	}
	outcome := artifact.outcomes[outcomeIndex]
	if !outcome.Available() || uint64(valueIndex) >= uint64(outcome.returnEnd-outcome.returnStart) {
		return ReturnValue{}, false
	}
	index := uint64(outcome.returnStart) + uint64(valueIndex)
	if index >= uint64(len(artifact.returnValues)) {
		return ReturnValue{}, false
	}
	return artifact.returnValues[index], true
}

func (artifact *Artifact) PointAt(index int) (Point, bool) {
	if !artifact.Available() || index < 0 || index >= len(artifact.points) {
		return Point{}, false
	}
	return artifact.points[index], true
}

func (artifact *Artifact) EnvironmentEdgeAt(index int) (EnvironmentEdge, bool) {
	if !artifact.Available() || index < 0 || index >= len(artifact.environment) {
		return EnvironmentEdge{}, false
	}
	return artifact.environment[index], true
}

func (artifact *Artifact) LocalTransferAt(index int) (LocalTransfer, bool) {
	if !artifact.Available() || index < 0 || index >= len(artifact.localTransfers) {
		return LocalTransfer{}, false
	}
	return artifact.localTransfers[index], true
}

func (artifact *Artifact) RegionAt(index int) (Region, bool) {
	if !artifact.Available() || index < 0 || index >= len(artifact.regions) {
		return Region{}, false
	}
	return artifact.regions[index], true
}

func (artifact *Artifact) WTOEventAt(index int) (WTOEvent, bool) {
	if !artifact.Available() || index < 0 || index >= len(artifact.events) {
		return WTOEvent{}, false
	}
	return artifact.events[index], true
}
