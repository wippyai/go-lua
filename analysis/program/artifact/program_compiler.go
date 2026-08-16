// Package programartifact owns the immutable, reusable analyzer artifact for
// one sealed Program. It retains no Link, engine, schema, runtime, callback,
// raw Term, or domain authority.
package artifact

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/artifact/internal/grammar"
	"github.com/wippyai/go-lua/analysis/program/flow"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

const (
	// GrammarABIVersion is the cold schema/artifact contract shared with the
	// sole Program schema issuer. It is data in every CompileKey, not an
	// ambient package assumption.
	GrammarABIVersion = grammar.ABIVersion

	artifactFormat             = uint64(32)
	pointGeometryLawVersion    = uint64(1)
	pointAttachmentLawVersion  = uint64(1)
	compilerLawVersion         = uint64(2)
	operatorLawVersion         = uint64(1)
	substitutionLawVersion     = uint64(1)
	summaryLawVersion          = uint64(1)
	wtoLawVersion              = uint64(1)
	routeLawVersion            = uint64(3)
	valuesLawVersion           = uint64(1)
	bodyOutcomeLawVersion      = uint64(4)
	functionBoundaryLawVersion = uint64(2)
	// Heap allocation geometry now commits the parent-issued
	// SharesFirstValueCell relation. Keep this generic occurrence law separate
	// from Pack's receipt law: changing Heap rows must invalidate only the
	// reusable occurrence/artifact identity contract.
	occurrenceLawVersion = uint64(11)
	// v3 records the closed DiagnosticObservation union, including detached
	// unresolved-reference proof and exact branch payload masks.
	diagnosticLawVersion  = uint64(4)
	packReceiptLawVersion = uint64(2)

	compileKeyDomain      = "analysis/program-artifact/compile-key"
	artifactIDDomain      = "analysis/program-artifact/artifact"
	grammarIdentityDomain = "analysis/program-artifact/grammar"
)

// GrammarIdentity is the pointer-free cold grammar admitted by the Program
// artifact compiler. Exact live SchemaBinding authority is joined later.
type GrammarIdentity struct {
	schema identity.ContentID
	abi    uint64
	id     identity.ContentID
}

func grammarIdentity(capability grammar.Capability) (GrammarIdentity, bool) {
	if !capability.Available() {
		return GrammarIdentity{}, false
	}
	identity := GrammarIdentity{schema: capability.SchemaDigest(), abi: capability.ABIVersion()}
	identity.id = digest(grammarIdentityDomain, artifactFormat, bytesField(identity.schema), uintField(identity.abi))
	return identity, identity.Available()
}

func (grammar GrammarIdentity) Available() bool {
	return grammar.schema.Available() && grammar.abi == GrammarABIVersion && grammar.id.Available()
}

func (grammar GrammarIdentity) SchemaDigest() identity.ContentID {
	if !grammar.Available() {
		return identity.ContentID{}
	}
	return grammar.schema
}

func (grammar GrammarIdentity) ABIVersion() uint64 {
	if !grammar.Available() {
		return 0
	}
	return grammar.abi
}

func (grammar GrammarIdentity) ID() identity.ContentID {
	if !grammar.Available() {
		return identity.ContentID{}
	}
	return grammar.id
}

// CompileKey is the complete reusable cold compiler identity. Every law
// version is retained as data and committed by both the key and Artifact ID.
type CompileKey struct {
	program             identity.ContentID
	grammar             GrammarIdentity
	format              uint64
	compilerLaw         uint64
	operatorLaw         uint64
	substituteLaw       uint64
	summaryLaw          uint64
	wtoLaw              uint64
	routeLaw            uint64
	valuesLaw           uint64
	bodyOutcomeLaw      uint64
	functionBoundaryLaw uint64
	occurrenceLaw       uint64
	diagnosticLaw       uint64
	packReceiptLaw      uint64
	id                  identity.ContentID
}

func NewCompileKeyAuthorized(input program.TransformerInput, capability grammar.Capability) (CompileKey, bool) {
	identity, identityOK := grammarIdentity(capability)
	if !input.Available() || !identityOK {
		return CompileKey{}, false
	}
	key := CompileKey{
		program: input.ContentID(), grammar: identity, format: artifactFormat,
		compilerLaw: compilerLawVersion, operatorLaw: operatorLawVersion,
		substituteLaw: substitutionLawVersion, summaryLaw: summaryLawVersion,
		wtoLaw: wtoLawVersion, routeLaw: routeLawVersion, valuesLaw: valuesLawVersion,
		bodyOutcomeLaw: bodyOutcomeLawVersion, functionBoundaryLaw: functionBoundaryLawVersion, occurrenceLaw: occurrenceLawVersion, diagnosticLaw: diagnosticLawVersion, packReceiptLaw: packReceiptLawVersion,
	}
	key.id = digest(compileKeyDomain, artifactFormat, key.identityFields()...)
	return key, key.Available()
}

func (key CompileKey) identityFields() []field {
	return []field{
		bytesField(key.program), bytesField(key.grammar.ID()), bytesField(key.grammar.SchemaDigest()),
		uintField(key.grammar.ABIVersion()), uintField(key.format), uintField(key.compilerLaw),
		uintField(key.operatorLaw), uintField(key.substituteLaw), uintField(key.summaryLaw),
		uintField(key.wtoLaw), uintField(key.routeLaw), uintField(key.valuesLaw), uintField(key.bodyOutcomeLaw), uintField(key.functionBoundaryLaw), uintField(key.occurrenceLaw), uintField(key.diagnosticLaw), uintField(key.packReceiptLaw),
	}
}

func (key CompileKey) Available() bool {
	return key.program.Available() && key.grammar.Available() && key.format == artifactFormat &&
		key.compilerLaw == compilerLawVersion && key.operatorLaw == operatorLawVersion &&
		key.substituteLaw == substitutionLawVersion && key.summaryLaw == summaryLawVersion &&
		key.wtoLaw == wtoLawVersion && key.routeLaw == routeLawVersion && key.valuesLaw == valuesLawVersion &&
		key.bodyOutcomeLaw == bodyOutcomeLawVersion && key.functionBoundaryLaw == functionBoundaryLawVersion && key.occurrenceLaw == occurrenceLawVersion &&
		key.diagnosticLaw == diagnosticLawVersion && key.packReceiptLaw == packReceiptLawVersion && key.id.Available()
}

func (key CompileKey) ProgramID() identity.ContentID {
	if !key.Available() {
		return identity.ContentID{}
	}
	return key.program
}

func (key CompileKey) Grammar() GrammarIdentity {
	if !key.Available() {
		return GrammarIdentity{}
	}
	return key.grammar
}

func (key CompileKey) SchemaDigest() identity.ContentID {
	return key.Grammar().SchemaDigest()
}

func (key CompileKey) ABIVersion() uint64 { return key.Grammar().ABIVersion() }

func (key CompileKey) ID() identity.ContentID {
	if !key.Available() {
		return identity.ContentID{}
	}
	return key.id
}

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

// EnvironmentEdge is a scalar copy of one exact Program StructuralRoute.
// Its ID is the Program-issued route occurrence ContextID. RouteID is the
// parent final-route semantic ID and is not a second artifact identity.
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
// distinct from StructuralRoute and therefore never owns a guard, recurrence
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
type Artifact struct {
	key                    CompileKey
	id                     identity.ContentID
	sealed                 identity.ContentID
	pointAttachments       []PointAttachmentRow
	points                 []Point
	environment            []EnvironmentEdge
	localTransfers         []LocalTransfer
	regions                []Region
	events                 []WTOEvent
	values                 []ValuesRow
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
	packReceipt            PackReceipt
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
	role     program.AllocationRole
	form     program.AllocationForm
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
func (row HeapAllocationRow) Role() program.AllocationRole {
	if !row.Available() {
		return program.AllocationInvalid
	}
	return row.role
}
func (row HeapAllocationRow) Form() program.AllocationForm {
	if !row.Available() {
		return program.AllocationFormInvalid
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
	return artifact != nil && artifact.key.Available() && artifact.id.Available() && artifact.sealed == artifact.id
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
// source receipts without exporting the authored source coordinate.
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

// PackReceipt returns Program's immutable Pack-specific source plane.  The
// receipt contains only semantic IDs; mounted substitutions remain owned by
// Link Boundary and Static.
func (artifact *Artifact) PackReceipt() (PackReceipt, bool) {
	if !artifact.Available() || !artifact.packReceipt.Available() {
		return PackReceipt{}, false
	}
	return artifact.packReceipt, true
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

// CompileDetailedAuthorized is the exact diagnostic lane for the sole
// authorized compiler. The capability type is behind programartifact's
// internal import boundary, so diagnostics do not create a raw Compile API.
func CompileDetailedAuthorized(input program.TransformerInput, capability grammar.Capability) (*Artifact, CompileFailure) {
	if !input.Available() {
		return nil, compileFailure(CompileStageAuthority, CompileRowAuthority, -1, -1, CompileReasonProgramUnavailable)
	}
	if !capability.Available() {
		return nil, compileFailure(CompileStageAuthority, CompileRowAuthority, -1, -1, CompileReasonGrammarAuthorityUnavailable)
	}
	key, ok := NewCompileKeyAuthorized(input, capability)
	if !ok {
		return nil, compileFailure(CompileStageAuthority, CompileRowAuthority, -1, -1, CompileReasonCompileKeyUnavailable)
	}
	transaction := compiler{
		input: input, key: key, points: make(map[identity.ContentID]struct{}), pointGeometry: make(map[identity.ContentID]Point),
		occurrenceSpans: make(map[occurrenceLookup]occurrenceSpanGeometry), routeProofs: make(map[identity.ContentID]program.StructuralRoute), localStages: make(map[identity.ContentID]identity.ContentID), computationStages: make(map[identity.ContentID][]computationStage), callStages: make(map[identity.ContentID]callStageSet),
		pointIDsBySite:     make(map[identity.ContentID][]identity.ContentID),
		environmentByRoute: make(map[identity.ContentID]EnvironmentEdge), environmentRouteDuplicates: make(map[identity.ContentID]struct{}),
		diagnosticObservationByID: make(map[identity.ContentID]int),
	}
	if failure := transaction.indexPointAttachmentsFailure(); failure.Available() {
		return nil, failure
	}
	if failure := transaction.copyValuesFailure(); failure.Available() {
		return nil, failure
	}
	if failure := transaction.copyBodiesAndOutcomesFailure(); failure.Available() {
		return nil, failure
	}
	if failure := transaction.copyFunctionBoundariesFailure(); failure.Available() {
		return nil, failure
	}
	if failure := transaction.copyCallTargetsFailure(); failure.Available() {
		return nil, failure
	}
	if failure := transaction.copyBoundaryRowsFailure(); failure.Available() {
		return nil, failure
	}
	if receipt, failure := transaction.copyPackReceiptFailure(); failure.Available() {
		return nil, failure
	} else {
		transaction.packReceipt = receipt
	}
	if failure := transaction.copyHeapGeometryFailure(); failure.Available() {
		return nil, failure
	}
	if failure := transaction.copyLocalWTOFailure(); failure.Available() {
		return nil, failure
	}
	if failure := transaction.emitRoutesFailure(); failure.Available() {
		return nil, failure
	}
	if failure := transaction.canonicalizePointDecisionsFailure(); failure.Available() {
		return nil, failure
	}
	if failure := transaction.copyOccurrenceCatalogFailure(); failure.Available() {
		return nil, failure
	}
	if failure := transaction.copyDiagnosticObservationsFailure(); failure.Available() {
		return nil, failure
	}
	if failure := transaction.copyStaticRowsFailure(); failure.Available() {
		return nil, failure
	}
	if failure := transaction.copyStaticGraphFailure(); failure.Available() {
		return nil, failure
	}
	if failure := transaction.deriveArithmeticSummariesFailure(); failure.Available() {
		return nil, failure
	}
	if failure := transaction.deriveRuleOccurrencesFailure(); failure.Available() {
		return nil, failure
	}
	if failure := transaction.installLocalStagesFailure(); failure.Available() {
		return nil, failure
	}
	if transaction.ruleOccurrences == nil {
		return nil, compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceUnavailable)
	}
	if failure := transaction.finalizeFailure(); failure.Available() {
		return nil, failure
	}
	artifact, failure := transaction.sealArtifact()
	if failure.Available() {
		return nil, failure
	}
	return artifact, CompileFailure{}
}

type compiler struct {
	input                      program.TransformerInput
	key                        CompileKey
	pointAttachments           []PointAttachmentRow
	points                     map[identity.ContentID]struct{}
	environment                []EnvironmentEdge
	localTransfers             []LocalTransfer
	regions                    []Region
	events                     []WTOEvent
	values                     []ValuesRow
	bodies                     []BodyRow
	functionBoundaries         []FunctionBoundaryRow
	callTargets                []CallTargetRow
	boundaries                 []BoundaryRow
	outcomes                   []OutcomeRow
	returnValues               []ReturnValue
	heapAllocations            []HeapAllocationRow
	heapIndexes                []HeapIndexRow
	packReceipt                PackReceipt
	occurrences                []OccurrenceRow
	exactScalarSummaries       []ExactScalarSummaryRow
	exactScalarStates          map[identity.ContentID]exactScalarState
	arithmeticSummaries        []ArithmeticSummaryRow
	unarySummaries             []UnarySummaryRow
	ruleOccurrences            map[RuleRole][]RuleOccurrence
	diagnosticObservations     []DiagnosticObservationRow
	staticTypeArguments        []StaticTypeArgumentRow
	staticTypeValues           []StaticTypeValueRow
	staticTypeNodes            []StaticTypeNodeRow
	staticExpressions          []StaticExpressionRow
	staticInputs               []StaticInputRow
	diagnosticObservationByID  map[identity.ContentID]int
	pointGeometry              map[identity.ContentID]Point
	occurrenceSpans            map[occurrenceLookup]occurrenceSpanGeometry
	routeProofs                map[identity.ContentID]program.StructuralRoute
	localStages                map[identity.ContentID]identity.ContentID
	computationStages          map[identity.ContentID][]computationStage
	callStages                 map[identity.ContentID]callStageSet
	pointIDsBySite             map[identity.ContentID][]identity.ContentID
	pointDecisionAdds          map[identity.ContentID][]identity.ContentID
	environmentByRoute         map[identity.ContentID]EnvironmentEdge
	environmentRouteDuplicates map[identity.ContentID]struct{}
}

func (compiler *compiler) copyLocalWTO() bool {
	return !compiler.copyLocalWTOFailure().Available()
}

func (compiler *compiler) copyLocalWTOFailure() CompileFailure {
	wto := compiler.input.LocalWTO()
	regions := make(map[identity.ContentID]int, wto.Count())
	for index := 0; index < wto.Count(); index++ {
		parent, ok := wto.At(index)
		if !ok || !parent.Available() || !parent.ID().Available() {
			return compileFailure(CompileStageLocalWTO, CompileRowRegion, index, -1, CompileReasonRegionUnavailable)
		}
		if _, exists := regions[parent.ID()]; exists {
			return compileFailure(CompileStageLocalWTO, CompileRowRegion, index, -1, CompileReasonRegionDuplicate)
		}
		header, headerOK := parent.HeaderPoint()
		if !headerOK || !compiler.installPoint(header) {
			return compileFailure(CompileStageLocalWTO, CompileRowRegion, index, -1, CompileReasonRegionHeaderUnavailable)
		}
		members := make([]identity.ContentID, parent.PointCount())
		if len(members) == 0 {
			return compileFailure(CompileStageLocalWTO, CompileRowRegion, index, -1, CompileReasonRegionEmpty)
		}
		for pointIndex := range members {
			point, pointOK := parent.PointAt(pointIndex)
			if !pointOK || !compiler.installPoint(point) {
				return compileFailure(CompileStageLocalWTO, CompileRowRegion, index, pointIndex, CompileReasonRegionMemberUnavailable)
			}
			members[pointIndex] = point.PathID()
			if pointIndex != 0 && members[pointIndex] == members[pointIndex-1] {
				return compileFailure(CompileStageLocalWTO, CompileRowRegion, index, pointIndex, CompileReasonRegionMemberDuplicate)
			}
		}
		if members[0] != header.PathID() {
			return compileFailure(CompileStageLocalWTO, CompileRowRegion, index, 0, CompileReasonRegionHeaderMismatch)
		}
		regions[parent.ID()] = len(compiler.regions)
		compiler.regions = append(compiler.regions, Region{
			id: parent.ID(), head: header.PathID(), sourceHead: header.PathID(), parent: parent.ParentID(), cyclic: parent.Cyclic(), members: members,
		})
	}

	pointEvents := make(map[identity.ContentID]struct{}, len(compiler.points))
	entered := make([]bool, len(compiler.regions))
	exited := make([]bool, len(compiler.regions))
	type frame struct {
		region int
		next   int
	}
	stack := make([]frame, 0, len(compiler.regions))
	for index := 0; index < wto.EventCount(); index++ {
		parent, ok := wto.EventAt(index)
		if !ok || !parent.Available() {
			return compileFailure(CompileStageLocalWTO, CompileRowWTOEvent, index, -1, CompileReasonEventUnavailable)
		}
		event := WTOEvent{}
		switch parent.Kind() {
		case flow.WTOEventEnter:
			region, regionOK := parent.Region()
			if !regionOK || !region.Available() {
				return compileFailure(CompileStageLocalWTO, CompileRowWTOEvent, index, -1, CompileReasonEventRegionUnavailable)
			}
			regionIndex, exists := regions[region.ID()]
			if !exists {
				return compileFailure(CompileStageLocalWTO, CompileRowWTOEvent, index, -1, CompileReasonEventRegionUnknown)
			}
			if entered[regionIndex] || exited[regionIndex] {
				return compileFailure(CompileStageLocalWTO, CompileRowWTOEvent, index, regionIndex, CompileReasonEventRegionRepeated)
			}
			row := compiler.regions[regionIndex]
			if len(stack) == 0 {
				if row.parent.Available() {
					return compileFailure(CompileStageLocalWTO, CompileRowWTOEvent, index, regionIndex, CompileReasonEventRootParent)
				}
			} else {
				parentFrame := stack[len(stack)-1]
				if !entered[parentFrame.region] || parentFrame.next == 0 || row.parent != compiler.regions[parentFrame.region].id {
					return compileFailure(CompileStageLocalWTO, CompileRowWTOEvent, index, regionIndex, CompileReasonEventParentMismatch)
				}
			}
			entered[regionIndex] = true
			stack = append(stack, frame{region: regionIndex})
			event.kind, event.region = WTOEventEnter, region.ID()
		case flow.WTOEventPoint:
			point, pointOK := parent.Point()
			// Parent LocalWTO may schedule an acyclic phase vertex outside every
			// cyclic Region.  It is still a total parent-issued point and must be
			// retained, rather than being treated as malformed merely because the
			// region stack is empty.
			if !pointOK || !compiler.installPoint(point) {
				return compileFailure(CompileStageLocalWTO, CompileRowWTOEvent, index, -1, CompileReasonEventPointUnavailable)
			}
			id := point.PathID()
			if _, exists := pointEvents[id]; exists {
				return compileFailure(CompileStageLocalWTO, CompileRowWTOEvent, index, -1, CompileReasonEventPointRepeated)
			}
			if len(stack) != 0 {
				current := &stack[len(stack)-1]
				row := compiler.regions[current.region]
				if current.next >= len(row.members) || row.members[current.next] != id || current.next == 0 && row.head != id {
					return compileFailure(CompileStageLocalWTO, CompileRowWTOEvent, index, current.next, CompileReasonEventPointOrder)
				}
				current.next++
			}
			pointEvents[id] = struct{}{}
			event.kind, event.point = WTOEventPoint, id
		case flow.WTOEventExit:
			region, regionOK := parent.Region()
			if !regionOK || !region.Available() || len(stack) == 0 {
				return compileFailure(CompileStageLocalWTO, CompileRowWTOEvent, index, -1, CompileReasonEventExitUnavailable)
			}
			current := stack[len(stack)-1]
			if compiler.regions[current.region].id != region.ID() || current.next != len(compiler.regions[current.region].members) || exited[current.region] {
				return compileFailure(CompileStageLocalWTO, CompileRowWTOEvent, index, current.region, CompileReasonEventExitMismatch)
			}
			exited[current.region] = true
			stack = stack[:len(stack)-1]
			event.kind, event.region = WTOEventExit, region.ID()
		default:
			return compileFailure(CompileStageLocalWTO, CompileRowWTOEvent, index, -1, CompileReasonEventKindUnknown)
		}
		if !event.Available() {
			return compileFailure(CompileStageLocalWTO, CompileRowWTOEvent, index, -1, CompileReasonEventUnavailable)
		}
		compiler.events = append(compiler.events, event)
	}
	if len(stack) != 0 {
		return compileFailure(CompileStageLocalWTO, CompileRowWTOEvent, wto.EventCount(), len(stack), CompileReasonEventUnbalanced)
	}
	for index := range compiler.regions {
		if !entered[index] || !exited[index] {
			return compileFailure(CompileStageLocalWTO, CompileRowRegion, index, -1, CompileReasonRegionIncomplete)
		}
	}
	for point := range compiler.points {
		if _, exists := pointEvents[point]; !exists {
			return compileFailure(CompileStageLocalWTO, CompileRowPoint, -1, -1, CompileReasonPointUnscheduled)
		}
	}
	return CompileFailure{}
}

func (compiler *compiler) installPoint(point flow.WTOPoint) bool {
	if !point.Available() || !point.PathID().Available() {
		return false
	}
	if compiler.pointGeometry == nil {
		compiler.pointGeometry = make(map[identity.ContentID]Point)
	}
	if existing, exists := compiler.pointGeometry[point.PathID()]; exists {
		return existing.Available()
	}
	entryBody, bodyOK := compiler.input.BodyAt(0)
	entrySite, entryOK := entryBody.EntrySite()
	if !bodyOK || !entryOK || !entrySite.Available() {
		return false
	}
	decisions := make(map[identity.ContentID]struct{})
	initial := false
	for index := 0; index < point.SiteCount(); index++ {
		site, siteOK := point.SiteAt(index)
		if !siteOK || !site.Available() || !compiler.input.OwnsSite(site) {
			return false
		}
		if site.ContextID() == entrySite.ContextID() {
			initial = true
		}
		count, countOK := compiler.input.GuardCount(site)
		if !countOK {
			return false
		}
		for guardIndex := 0; guardIndex < count; guardIndex++ {
			guard, guardOK := compiler.input.GuardAt(site, guardIndex)
			decisionID := guard.PathID()
			if !guardOK || !decisionID.Available() {
				return false
			}
			decisions[decisionID] = struct{}{}
		}
	}
	ordered := make([]identity.ContentID, 0, len(decisions))
	for decision := range decisions {
		ordered = append(ordered, decision)
	}
	identity.SortContentIDs(ordered)
	compiler.points[point.PathID()] = struct{}{}
	compiler.pointGeometry[point.PathID()] = Point{id: point.PathID(), decisions: ordered, initial: initial}
	return true
}

func (compiler *compiler) containsPoint(point flow.WTOPoint) bool {
	if !point.Available() || !point.PathID().Available() {
		return false
	}
	_, exists := compiler.points[point.PathID()]
	return exists
}

// admitPointDecision joins an exact guarded StructuralRoute decision into the
// already parent-issued source point scope. Point Site attachments describe
// ambient continuation guards, while the route proof is the sole authority
// for its edge-local guard. Keeping both sources in one canonical set avoids
// reconstructing scope from Terms at Link time.
func (compiler *compiler) admitPointDecision(point, decision identity.ContentID) bool {
	geometry, exists := compiler.pointGeometry[point]
	if !exists || !geometry.Available() || !decision.Available() {
		return false
	}
	if compiler.pointDecisionAdds == nil {
		compiler.pointDecisionAdds = make(map[identity.ContentID][]identity.ContentID)
	}
	compiler.pointDecisionAdds[point] = append(compiler.pointDecisionAdds[point], decision)
	return true
}

// canonicalizePointDecisionsFailure batches route-local decision admission by
// owner point. The old path inserted each decision into a sorted slice, moving
// O(D) tail elements for every route. One radix pass and in-place duplicate
// removal gives the same canonical owner-local order in O(D) work.
func (compiler *compiler) canonicalizePointDecisionsFailure() CompileFailure {
	for point, additions := range compiler.pointDecisionAdds {
		geometry, known := compiler.pointGeometry[point]
		if !known || !geometry.Available() {
			return compileFailure(CompileStageRoutes, CompileRowRoute, -1, -1, CompileReasonRouteGuard)
		}
		if len(additions) == 0 {
			continue
		}
		decisions := make([]identity.ContentID, 0, len(geometry.decisions)+len(additions))
		decisions = append(decisions, geometry.decisions...)
		decisions = append(decisions, additions...)
		identity.SortContentIDs(decisions)
		unique := 0
		for _, decision := range decisions {
			if !decision.Available() {
				return compileFailure(CompileStageRoutes, CompileRowRoute, -1, -1, CompileReasonRouteGuard)
			}
			if unique == 0 || decisions[unique-1] != decision {
				decisions[unique] = decision
				unique++
			}
		}
		geometry.decisions = decisions[:unique]
		compiler.pointGeometry[point] = geometry
	}
	return CompileFailure{}
}

func (compiler *compiler) emitRoutesFailure() CompileFailure {
	routes := compiler.input.StructuralRoutes()
	for index := 0; index < routes.Count(); index++ {
		route, ok := routes.At(index)
		if !ok {
			return compileFailure(CompileStageRoutes, CompileRowRoute, index, -1, CompileReasonRouteUnavailable)
		}
		if failure := compiler.admitEnvironmentFailure(route, index); failure.Available() {
			return failure
		}
	}
	return CompileFailure{}
}

func (compiler *compiler) admitEnvironment(route program.StructuralRoute) bool {
	return !compiler.admitEnvironmentFailure(route, -1).Available()
}

func (compiler *compiler) admitEnvironmentFailure(route program.StructuralRoute, rowIndex int) CompileFailure {
	if !compiler.input.OwnsStructuralRoute(route) {
		return compileFailure(CompileStageRoutes, CompileRowRoute, rowIndex, -1, CompileReasonRouteForeign)
	}
	from, fromOK := route.FromPoint()
	to, toOK := route.ToPoint()
	routeID, routeOK := route.RouteID()
	arm, armOK := route.Kind()
	kind, kindOK := routeKind(arm)
	if !fromOK || !toOK || !compiler.containsPoint(from) || !compiler.containsPoint(to) {
		return compileFailure(CompileStageRoutes, CompileRowRoute, rowIndex, -1, CompileReasonRouteEndpoints)
	}
	if !routeOK {
		return compileFailure(CompileStageRoutes, CompileRowRoute, rowIndex, -1, CompileReasonRouteIdentity)
	}
	if !armOK || !kindOK {
		return compileFailure(CompileStageRoutes, CompileRowRoute, rowIndex, -1, CompileReasonRouteArm)
	}
	occurrenceID := route.ContextID()
	if !occurrenceID.Available() {
		return compileFailure(CompileStageRoutes, CompileRowRoute, rowIndex, -1, CompileReasonRouteIdentity)
	}
	guardID, conditionID, guarded, truth := identity.ContentID{}, identity.ContentID{}, false, false
	decisionID := identity.ContentID{}
	if guard, guardOK := route.Guard(); guardOK {
		truthValue, truthOK := guard.Truth()
		if !compiler.input.OwnsRouteGuard(guard) || !truthOK {
			return compileFailure(CompileStageRoutes, CompileRowRoute, rowIndex, -1, CompileReasonRouteGuard)
		}
		guardID, guarded, truth = guard.ContextID(), true, truthValue
		if !guardID.Available() {
			return compileFailure(CompileStageRoutes, CompileRowRoute, rowIndex, -1, CompileReasonRouteGuard)
		}
		var decisionOK bool
		decisionID, decisionOK = guard.DecisionPathID()
		if !decisionOK || !decisionID.Available() || !compiler.admitPointDecision(from.PathID(), decisionID) {
			return compileFailure(CompileStageRoutes, CompileRowRoute, rowIndex, -1, CompileReasonRouteGuard)
		}
		conditionID, _ = guard.ConditionValueSpanID()
	}

	component, resetDigest := identity.ContentID{}, identity.ContentID{}
	hasReset := route.HasResetWitness()
	mu, hasMu := identity.ContentID{}, route.HasMu()
	if hasMu {
		var muOK bool
		mu, muOK = route.MuPathID()
		if !muOK || !mu.Available() {
			return compileFailure(CompileStageRoutes, CompileRowRoute, rowIndex, -1, CompileReasonRouteMu)
		}
	}
	resets := []identity.ContentID(nil)
	if recurrence, recurrenceOK := route.Recurrence(); recurrenceOK {
		if !compiler.input.OwnsRouteRecurrence(recurrence) {
			return compileFailure(CompileStageRoutes, CompileRowRoute, rowIndex, -1, CompileReasonRouteRecurrence)
		}
		proofRouteID, proofRouteIDOK := recurrence.RouteID()
		if !proofRouteIDOK || proofRouteID != routeID {
			return compileFailure(CompileStageRoutes, CompileRowRoute, rowIndex, -1, CompileReasonRouteRecurrence)
		}
		component = recurrence.ComponentID()
		if !component.Available() {
			return compileFailure(CompileStageRoutes, CompileRowRoute, rowIndex, -1, CompileReasonRouteRecurrence)
		}
		if hasReset {
			// ResetCount is a Mu/reset-witness capability. Ordinary
			// intra-component routes intentionally have no witness and their
			// parent proof returns (0,false), rather than fabricating an empty
			// reset interval. Only consume the count when the witness exists.
			count, countOK := recurrence.ResetCount()
			if !countOK || count < 0 {
				return compileFailure(CompileStageRoutes, CompileRowRoute, rowIndex, -1, CompileReasonRouteReset)
			}
			var resetOK bool
			resetDigest, resetOK = recurrence.ResetDigest()
			routeResetDigest, routeResetOK := route.ResetDigest()
			if !resetOK || !resetDigest.Available() || !routeResetOK || routeResetDigest != resetDigest || route.ResetCount() != count {
				return compileFailure(CompileStageRoutes, CompileRowRoute, rowIndex, -1, CompileReasonRouteReset)
			}
			resets = make([]identity.ContentID, count)
			for index := range resets {
				path, pathOK := route.ResetPathAt(index)
				if !pathOK || !path.Available() {
					return compileFailure(CompileStageRoutes, CompileRowRoute, rowIndex, index, CompileReasonRouteResetMember)
				}
				resets[index] = path
				if index != 0 && !contentIDBefore(resets[index-1], resets[index]) {
					return compileFailure(CompileStageRoutes, CompileRowRoute, rowIndex, index, CompileReasonRouteResetOrder)
				}
			}
		}
	} else if hasMu || hasReset {
		return compileFailure(CompileStageRoutes, CompileRowRoute, rowIndex, -1, CompileReasonRouteRecurrence)
	}
	if hasMu && !component.Available() || hasMu != hasReset {
		return compileFailure(CompileStageRoutes, CompileRowRoute, rowIndex, -1, CompileReasonRouteMuResetMismatch)
	}

	row := EnvironmentEdge{
		id: occurrenceID, from: from.PathID(), to: to.PathID(), route: routeID,
		guard: guardID, decision: decisionID, condition: conditionID, guarded: guarded, truth: truth, component: component,
		mu: mu, hasMu: hasMu, reset: resetDigest, resets: resets, hasReset: hasReset, arm: kind,
	}
	if !row.Available() {
		return compileFailure(CompileStageRoutes, CompileRowEnvironment, rowIndex, -1, CompileReasonEnvironmentUnavailable)
	}
	compiler.environment = append(compiler.environment, row)
	if prior, duplicate := compiler.routeProofs[row.route]; duplicate && !prior.Equal(route) {
		return compileFailure(CompileStageRoutes, CompileRowRoute, rowIndex, -1, CompileReasonRouteIdentity)
	}
	compiler.routeProofs[row.route] = route
	if _, exists := compiler.environmentByRoute[row.route]; exists {
		compiler.environmentRouteDuplicates[row.route] = struct{}{}
	} else {
		compiler.environmentByRoute[row.route] = row
	}
	return CompileFailure{}
}

func (compiler *compiler) finalizeFailure() CompileFailure {
	// Least-significant key first preserves canonical (From, To, ID) order.
	identity.SortByContentID(compiler.environment, func(row EnvironmentEdge) identity.ContentID { return row.id })
	identity.SortByContentID(compiler.environment, func(row EnvironmentEdge) identity.ContentID { return row.to })
	identity.SortByContentID(compiler.environment, func(row EnvironmentEdge) identity.ContentID { return row.from })
	for index := 1; index < len(compiler.environment); index++ {
		if compiler.environment[index-1].id == compiler.environment[index].id {
			return compileFailure(CompileStageCanonicalize, CompileRowEnvironment, index, -1, CompileReasonEnvironmentDuplicate)
		}
	}
	identity.SortByContentID(compiler.localTransfers, func(row LocalTransfer) identity.ContentID { return row.id })
	identity.SortByContentID(compiler.localTransfers, func(row LocalTransfer) identity.ContentID { return row.to })
	identity.SortByContentID(compiler.localTransfers, func(row LocalTransfer) identity.ContentID { return row.from })
	for index := 1; index < len(compiler.localTransfers); index++ {
		if compiler.localTransfers[index-1].id == compiler.localTransfers[index].id {
			return compileFailure(CompileStageCanonicalize, CompileRowEnvironment, index, -1, CompileReasonEnvironmentDuplicate)
		}
	}
	identity.SortByContentID(compiler.diagnosticObservations, func(row DiagnosticObservationRow) identity.ContentID { return row.id })
	for index := 1; index < len(compiler.diagnosticObservations); index++ {
		if compiler.diagnosticObservations[index-1].id == compiler.diagnosticObservations[index].id {
			return compileFailure(CompileStageCanonicalize, CompileRowRoute, index, -1, CompileReasonRouteGuard)
		}
	}
	return CompileFailure{}
}

func (compiler *compiler) sealArtifact() (*Artifact, CompileFailure) {
	pointIDs := make([]identity.ContentID, 0, len(compiler.points))
	for id := range compiler.points {
		pointIDs = append(pointIDs, id)
	}
	identity.SortContentIDs(pointIDs)
	points := make([]Point, len(pointIDs))
	for index, id := range pointIDs {
		point, ok := compiler.pointGeometry[id]
		if !ok || !point.Available() {
			return nil, compileFailure(CompileStageSeal, CompileRowPoint, index, -1, CompileReasonPointUnavailable)
		}
		points[index] = point
	}
	occurrenceByID := make(map[occurrenceLookup]uint32, len(compiler.occurrences))
	for index, row := range compiler.occurrences {
		if uint64(index) > uint64(^uint32(0)) {
			return nil, compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		occurrenceByID[occurrenceLookup{kind: row.kind, id: row.id}] = uint32(index)
	}
	artifact := &Artifact{
		key: compiler.key, pointAttachments: compiler.pointAttachments, points: points, environment: compiler.environment, localTransfers: compiler.localTransfers,
		regions: compiler.regions, events: compiler.events, values: compiler.values,
		bodies: compiler.bodies, functionBoundaries: compiler.functionBoundaries, callTargets: compiler.callTargets, outcomes: compiler.outcomes, returnValues: compiler.returnValues,
		boundaries:      compiler.boundaries,
		heapAllocations: compiler.heapAllocations, heapIndexes: compiler.heapIndexes,
		packReceipt: compiler.packReceipt,
		occurrences: compiler.occurrences, exactScalarSummaries: compiler.exactScalarSummaries, arithmeticSummaries: compiler.arithmeticSummaries, unarySummaries: compiler.unarySummaries, occurrenceByID: occurrenceByID, ruleOccurrences: compiler.ruleOccurrences,
		diagnosticObservations: compiler.diagnosticObservations, staticTypeArguments: compiler.staticTypeArguments, staticTypeValues: compiler.staticTypeValues, staticTypeNodes: compiler.staticTypeNodes, staticExpressions: compiler.staticExpressions, staticInputs: compiler.staticInputs,
	}
	artifact.id = artifactID(artifact)
	if failure := artifact.validUnsealedFailure(); failure.Available() {
		return nil, failure
	}
	artifact.sealed = artifact.id
	return artifact, CompileFailure{}
}
