// Package programartifact owns the immutable, reusable analyzer artifact for
// one sealed Program. It retains no Link, engine, schema, runtime, callback,
// raw Term, or domain authority.
package programartifact

import (
	"github.com/wippyai/go-lua/analysis/internal/canonical"
	"github.com/wippyai/go-lua/analysis/internal/programartifact/internal/grammar"
	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/flow"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	programstatic "github.com/wippyai/go-lua/program/static"
)

const (
	// GrammarABIVersion is the cold schema/artifact contract shared with the
	// sole Program schema issuer. It is data in every CompileKey, not an
	// ambient package assumption.
	GrammarABIVersion = grammar.ABIVersion

	artifactFormat             = uint64(32)
	pointGeometryLawVersion    = uint64(1)
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
	schema keyspace.ContentID
	abi    uint64
	id     keyspace.ContentID
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

func (grammar GrammarIdentity) SchemaDigest() keyspace.ContentID {
	if !grammar.Available() {
		return keyspace.ContentID{}
	}
	return grammar.schema
}

func (grammar GrammarIdentity) ABIVersion() uint64 {
	if !grammar.Available() {
		return 0
	}
	return grammar.abi
}

func (grammar GrammarIdentity) ID() keyspace.ContentID {
	if !grammar.Available() {
		return keyspace.ContentID{}
	}
	return grammar.id
}

// CompileKey is the complete reusable cold compiler identity. Every law
// version is retained as data and committed by both the key and Artifact ID.
type CompileKey struct {
	program             keyspace.ContentID
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
	id                  keyspace.ContentID
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

func (key CompileKey) ProgramID() keyspace.ContentID {
	if !key.Available() {
		return keyspace.ContentID{}
	}
	return key.program
}

func (key CompileKey) Grammar() GrammarIdentity {
	if !key.Available() {
		return GrammarIdentity{}
	}
	return key.grammar
}

func (key CompileKey) SchemaDigest() keyspace.ContentID {
	return key.Grammar().SchemaDigest()
}

func (key CompileKey) ABIVersion() uint64 { return key.Grammar().ABIVersion() }

func (key CompileKey) ID() keyspace.ContentID {
	if !key.Available() {
		return keyspace.ContentID{}
	}
	return key.id
}

// Point is an exact parent-issued LocalWTO phase vertex path. Its ordered
// decision IDs and initial disposition are copied from Program-owned Site
// attachments; Link never has to reopen Program to recover point geometry.
type Point struct {
	id        keyspace.ContentID
	decisions []keyspace.ContentID
	initial   bool
}

func (point Point) ID() keyspace.ContentID { return point.id }
func (point Point) Available() bool        { return point.id.Available() }
func (point Point) DecisionCount() int {
	if !point.Available() {
		return 0
	}
	return len(point.decisions)
}
func (point Point) DecisionAt(index int) (keyspace.ContentID, bool) {
	if !point.Available() || index < 0 || index >= len(point.decisions) {
		return keyspace.ContentID{}, false
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
	id        keyspace.ContentID
	from      keyspace.ContentID
	to        keyspace.ContentID
	route     keyspace.ContentID
	guard     keyspace.ContentID
	decision  keyspace.ContentID
	condition keyspace.ContentID
	reset     keyspace.ContentID
	resets    []keyspace.ContentID
	component keyspace.ContentID
	arm       RouteKind
	guarded   bool
	truth     bool
	hasReset  bool
	mu        keyspace.ContentID
	hasMu     bool
}

// LocalTransfer is a Program-artifact-owned acyclic stage transport. It is
// distinct from StructuralRoute and therefore never owns a guard, recurrence
// component, Mu, or reset witness. Full transports carry the complete
// environment. Factor transports carry exactly the factors identified by a
// canonical list of artifact-owned rule roles; consumers resolve those roles
// only through their sealed schema directory.
type LocalTransfer struct {
	id    keyspace.ContentID
	from  keyspace.ContentID
	to    keyspace.ContentID
	full  bool
	roles []RuleRole
}

func (edge LocalTransfer) ID() keyspace.ContentID   { return edge.id }
func (edge LocalTransfer) From() keyspace.ContentID { return edge.from }
func (edge LocalTransfer) To() keyspace.ContentID   { return edge.to }
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

func (edge EnvironmentEdge) ID() keyspace.ContentID      { return edge.id }
func (edge EnvironmentEdge) From() keyspace.ContentID    { return edge.from }
func (edge EnvironmentEdge) To() keyspace.ContentID      { return edge.to }
func (edge EnvironmentEdge) RouteID() keyspace.ContentID { return edge.route }
func (edge EnvironmentEdge) Arm() RouteKind              { return edge.arm }

// DecisionID is the parent-issued decision Site identity for a guarded
// route. It is distinct from GuardID: GuardID authenticates the guard proof,
// while DecisionID is the coordinate needed to issue the exact Link-local
// guard expression. Unguarded routes deliberately have neither identity.
func (edge EnvironmentEdge) DecisionID() (keyspace.ContentID, bool) {
	return edge.decision, edge.Available() && edge.guarded
}

func (edge EnvironmentEdge) GuardID() (keyspace.ContentID, bool) {
	return edge.guard, edge.Available() && edge.guarded
}

// ConditionValueSpanID is the generic Program-issued branch condition value
// identity. It is absent for non-Branch guards and carries no diagnostic or
// domain meaning.
func (edge EnvironmentEdge) ConditionValueSpanID() (keyspace.ContentID, bool) {
	return edge.condition, edge.Available() && edge.guarded && edge.condition.Available()
}

func (edge EnvironmentEdge) Truth() (bool, bool) {
	return edge.truth, edge.Available() && edge.guarded
}

func (edge EnvironmentEdge) ResetDigest() (keyspace.ContentID, bool) {
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

func (edge EnvironmentEdge) ResetAt(index int) (keyspace.ContentID, bool) {
	if !edge.Available() || index < 0 || index >= len(edge.resets) {
		return keyspace.ContentID{}, false
	}
	return edge.resets[index], true
}

func (edge EnvironmentEdge) ComponentID() keyspace.ContentID {
	if !edge.Available() {
		return keyspace.ContentID{}
	}
	return edge.component
}

func (edge EnvironmentEdge) MuPathID() (keyspace.ContentID, bool) {
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
	region keyspace.ContentID
	point  keyspace.ContentID
}

func (event WTOEvent) Kind() WTOEventKind           { return event.kind }
func (event WTOEvent) RegionID() keyspace.ContentID { return event.region }
func (event WTOEvent) PointID() keyspace.ContentID  { return event.point }
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
	id         keyspace.ContentID
	head       keyspace.ContentID
	sourceHead keyspace.ContentID
	parent     keyspace.ContentID
	cyclic     bool
	members    []keyspace.ContentID
}

func (region Region) ID() keyspace.ContentID         { return region.id }
func (region Region) Head() keyspace.ContentID       { return region.head }
func (region Region) SourceHead() keyspace.ContentID { return region.sourceHead }
func (region Region) ParentID() keyspace.ContentID   { return region.parent }
func (region Region) Cyclic() bool                   { return region.cyclic }
func (region Region) MemberCount() int               { return len(region.members) }
func (region Region) MemberAt(index int) (keyspace.ContentID, bool) {
	if !region.id.Available() || index < 0 || index >= len(region.members) {
		return keyspace.ContentID{}, false
	}
	return region.members[index], true
}

// Artifact is immutable after Compile succeeds. The sealed scalar is written
// only after complete deep validation; all hot availability checks are O(1).
type Artifact struct {
	key                    CompileKey
	id                     keyspace.ContentID
	sealed                 keyspace.ContentID
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
	id                   keyspace.ContentID
	kind                 flowkind.FieldKind
	fieldSpan            keyspace.ContentID
	selectorSpan         keyspace.ContentID
	valuesSpan           keyspace.ContentID
	valuesID             keyspace.ContentID
	width                int
	finalOpen            bool
	sharesFirstValueCell bool
	normalized           keyspace.Key
	normalizedOK         bool
}

func (row HeapFieldRow) Available() bool {
	return row.id.Available() && row.kind >= flowkind.FieldList && row.kind <= flowkind.FieldKey && row.fieldSpan.Available() && row.valuesSpan.Available() && row.valuesID.Available() && row.width >= 0 && (row.kind == flowkind.FieldKey) == row.selectorSpan.Available() && (row.kind == flowkind.FieldKey || !row.sharesFirstValueCell)
}
func (row HeapFieldRow) ID() keyspace.ContentID {
	if !row.Available() {
		return keyspace.ContentID{}
	}
	return row.id
}
func (row HeapFieldRow) Kind() flowkind.FieldKind {
	if !row.Available() {
		return 0
	}
	return row.kind
}
func (row HeapFieldRow) FieldSpan() keyspace.ContentID {
	if !row.Available() {
		return keyspace.ContentID{}
	}
	return row.fieldSpan
}
func (row HeapFieldRow) SelectorSpan() keyspace.ContentID {
	if !row.Available() {
		return keyspace.ContentID{}
	}
	return row.selectorSpan
}
func (row HeapFieldRow) Values() (keyspace.ContentID, int, bool, bool) {
	if !row.Available() {
		return keyspace.ContentID{}, 0, false, false
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
func (row HeapFieldRow) ValuesID() keyspace.ContentID {
	if !row.Available() {
		return keyspace.ContentID{}
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
	id       keyspace.ContentID
	role     program.AllocationRole
	form     program.AllocationForm
	rootSpan keyspace.ContentID
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
func (row HeapAllocationRow) ID() keyspace.ContentID {
	if !row.Available() {
		return keyspace.ContentID{}
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
func (row HeapAllocationRow) RootSpan() keyspace.ContentID {
	if !row.Available() {
		return keyspace.ContentID{}
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
	id         keyspace.ContentID
	read       bool
	baseSpan   keyspace.ContentID
	resultSpan keyspace.ContentID
	keySpan    keyspace.ContentID
	lensKind   uint8
	exactKey   keyspace.Key
	valuesSpan keyspace.ContentID
	valuesID   keyspace.ContentID
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
func (row HeapIndexRow) ID() keyspace.ContentID {
	if !row.Available() {
		return keyspace.ContentID{}
	}
	return row.id
}
func (row HeapIndexRow) Read() bool { return row.Available() && row.read }
func (row HeapIndexRow) BaseSpan() keyspace.ContentID {
	if !row.Available() {
		return keyspace.ContentID{}
	}
	return row.baseSpan
}
func (row HeapIndexRow) ResultSpan() keyspace.ContentID {
	if !row.Available() || !row.read {
		return keyspace.ContentID{}
	}
	return row.resultSpan
}
func (row HeapIndexRow) DynamicKeySpan() keyspace.ContentID {
	if !row.Available() {
		return keyspace.ContentID{}
	}
	return row.keySpan
}
func (row HeapIndexRow) ExactKey() (keyspace.Key, bool) {
	if !row.Available() || row.lensKind != 1 {
		return 0, false
	}
	return row.exactKey, true
}
func (row HeapIndexRow) Values() (keyspace.ContentID, int, bool) {
	if !row.Available() || row.read {
		return keyspace.ContentID{}, 0, false
	}
	return row.valuesSpan, row.position, true
}
func (row HeapIndexRow) ValuesID() keyspace.ContentID {
	if !row.Available() || row.read {
		return keyspace.ContentID{}
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

func (artifact *Artifact) ID() keyspace.ContentID {
	if !artifact.Available() {
		return keyspace.ContentID{}
	}
	return artifact.id
}

func (artifact *Artifact) PointCount() int {
	if !artifact.Available() {
		return 0
	}
	return len(artifact.points)
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

func (artifact *Artifact) StaticExpressionByID(id keyspace.ContentID) (StaticExpressionRow, bool) {
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

// Compile is the sole artifact mint. It consumes exact Program proofs and a
// pure grammar identity; no generic row-authoring surface is exported.
func CompileAuthorized(input program.TransformerInput, capability grammar.Capability) (*Artifact, bool) {
	artifact, failure := CompileDetailedAuthorized(input, capability)
	return artifact, artifact != nil && !failure.Available()
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
		input: input, key: key, points: make(map[keyspace.ContentID]struct{}), pointGeometry: make(map[keyspace.ContentID]Point),
		occurrenceSpans: make(map[occurrenceLookup]occurrenceSpanGeometry), routeProofs: make(map[keyspace.ContentID]program.StructuralRoute), localStages: make(map[keyspace.ContentID]keyspace.ContentID), computationStages: make(map[keyspace.ContentID][]computationStage), callStages: make(map[keyspace.ContentID]callStageSet),
		pointIDsBySite:     make(map[keyspace.ContentID][]keyspace.ContentID),
		environmentByRoute: make(map[keyspace.ContentID]EnvironmentEdge), environmentRouteDuplicates: make(map[keyspace.ContentID]struct{}),
		diagnosticObservationByID: make(map[keyspace.ContentID]int),
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
	if failure := transaction.copyUnresolvedValueObservationsFailure(); failure.Available() {
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
	points                     map[keyspace.ContentID]struct{}
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
	exactScalarStates          map[keyspace.ContentID]exactScalarState
	arithmeticSummaries        []ArithmeticSummaryRow
	unarySummaries             []UnarySummaryRow
	ruleOccurrences            map[RuleRole][]RuleOccurrence
	diagnosticObservations     []DiagnosticObservationRow
	staticTypeArguments        []StaticTypeArgumentRow
	staticTypeValues           []StaticTypeValueRow
	staticTypeNodes            []StaticTypeNodeRow
	staticExpressions          []StaticExpressionRow
	staticInputs               []StaticInputRow
	diagnosticObservationByID  map[keyspace.ContentID]int
	pointGeometry              map[keyspace.ContentID]Point
	occurrenceSpans            map[occurrenceLookup]occurrenceSpanGeometry
	routeProofs                map[keyspace.ContentID]program.StructuralRoute
	localStages                map[keyspace.ContentID]keyspace.ContentID
	computationStages          map[keyspace.ContentID][]computationStage
	callStages                 map[keyspace.ContentID]callStageSet
	pointIDsBySite             map[keyspace.ContentID][]keyspace.ContentID
	pointDecisionAdds          map[keyspace.ContentID][]keyspace.ContentID
	environmentByRoute         map[keyspace.ContentID]EnvironmentEdge
	environmentRouteDuplicates map[keyspace.ContentID]struct{}
}

func (compiler *compiler) copyLocalWTO() bool {
	return !compiler.copyLocalWTOFailure().Available()
}

func (compiler *compiler) copyLocalWTOFailure() CompileFailure {
	wto := compiler.input.LocalWTO()
	regions := make(map[keyspace.ContentID]int, wto.Count())
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
		members := make([]keyspace.ContentID, parent.PointCount())
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

	pointEvents := make(map[keyspace.ContentID]struct{}, len(compiler.points))
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
		compiler.pointGeometry = make(map[keyspace.ContentID]Point)
	}
	if existing, exists := compiler.pointGeometry[point.PathID()]; exists {
		return existing.Available()
	}
	entryBody, bodyOK := compiler.input.BodyAt(0)
	entrySite, entryOK := entryBody.EntrySite()
	if !bodyOK || !entryOK || !entrySite.Available() {
		return false
	}
	decisions := make(map[keyspace.ContentID]struct{})
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
	ordered := make([]keyspace.ContentID, 0, len(decisions))
	for decision := range decisions {
		ordered = append(ordered, decision)
	}
	radixContentIDs(ordered)
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
func (compiler *compiler) admitPointDecision(point, decision keyspace.ContentID) bool {
	geometry, exists := compiler.pointGeometry[point]
	if !exists || !geometry.Available() || !decision.Available() {
		return false
	}
	if compiler.pointDecisionAdds == nil {
		compiler.pointDecisionAdds = make(map[keyspace.ContentID][]keyspace.ContentID)
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
		decisions := make([]keyspace.ContentID, 0, len(geometry.decisions)+len(additions))
		decisions = append(decisions, geometry.decisions...)
		decisions = append(decisions, additions...)
		radixContentIDs(decisions)
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

func (compiler *compiler) emitRoutes() bool {
	return !compiler.emitRoutesFailure().Available()
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
	guardID, conditionID, guarded, truth := keyspace.ContentID{}, keyspace.ContentID{}, false, false
	decisionID := keyspace.ContentID{}
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
		if route.DiagnosticObservationKind() != program.DiagnosticObservationInvalid {
			observation, observationOK := route.DiagnosticObservation()
			if !observationOK || !compiler.admitDiagnosticObservation(observation) {
				return compileFailure(CompileStageRoutes, CompileRowRoute, rowIndex, -1, CompileReasonRouteGuard)
			}
		}
		conditionID, _ = guard.ConditionValueSpanID()
	}

	component, resetDigest := keyspace.ContentID{}, keyspace.ContentID{}
	hasReset := route.HasResetWitness()
	mu, hasMu := keyspace.ContentID{}, route.HasMu()
	if hasMu {
		var muOK bool
		mu, muOK = route.MuPathID()
		if !muOK || !mu.Available() {
			return compileFailure(CompileStageRoutes, CompileRowRoute, rowIndex, -1, CompileReasonRouteMu)
		}
	}
	resets := []keyspace.ContentID(nil)
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
			resets = make([]keyspace.ContentID, count)
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

func (compiler *compiler) finalize() bool {
	return !compiler.finalizeFailure().Available()
}

func (compiler *compiler) finalizeFailure() CompileFailure {
	// Least-significant key first preserves canonical (From, To, ID) order.
	radixRows(compiler.environment, func(row EnvironmentEdge) keyspace.ContentID { return row.id })
	radixRows(compiler.environment, func(row EnvironmentEdge) keyspace.ContentID { return row.to })
	radixRows(compiler.environment, func(row EnvironmentEdge) keyspace.ContentID { return row.from })
	for index := 1; index < len(compiler.environment); index++ {
		if compiler.environment[index-1].id == compiler.environment[index].id {
			return compileFailure(CompileStageCanonicalize, CompileRowEnvironment, index, -1, CompileReasonEnvironmentDuplicate)
		}
	}
	radixRows(compiler.localTransfers, func(row LocalTransfer) keyspace.ContentID { return row.id })
	radixRows(compiler.localTransfers, func(row LocalTransfer) keyspace.ContentID { return row.to })
	radixRows(compiler.localTransfers, func(row LocalTransfer) keyspace.ContentID { return row.from })
	for index := 1; index < len(compiler.localTransfers); index++ {
		if compiler.localTransfers[index-1].id == compiler.localTransfers[index].id {
			return compileFailure(CompileStageCanonicalize, CompileRowEnvironment, index, -1, CompileReasonEnvironmentDuplicate)
		}
	}
	radixRows(compiler.diagnosticObservations, func(row DiagnosticObservationRow) keyspace.ContentID { return row.id })
	for index := 1; index < len(compiler.diagnosticObservations); index++ {
		if compiler.diagnosticObservations[index-1].id == compiler.diagnosticObservations[index].id {
			return compileFailure(CompileStageCanonicalize, CompileRowRoute, index, -1, CompileReasonRouteGuard)
		}
	}
	return CompileFailure{}
}

func (compiler *compiler) artifact() *Artifact {
	artifact, failure := compiler.sealArtifact()
	if failure.Available() {
		return nil
	}
	return artifact
}

func (compiler *compiler) sealArtifact() (*Artifact, CompileFailure) {
	pointIDs := make([]keyspace.ContentID, 0, len(compiler.points))
	for id := range compiler.points {
		pointIDs = append(pointIDs, id)
	}
	radixContentIDs(pointIDs)
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
		key: compiler.key, points: points, environment: compiler.environment, localTransfers: compiler.localTransfers,
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

func (artifact *Artifact) validUnsealed() bool {
	return !artifact.validUnsealedFailure().Available()
}

func (artifact *Artifact) validUnsealedFailure() CompileFailure {
	if artifact == nil || !artifact.key.Available() || !artifact.id.Available() || artifact.sealed.Available() {
		return compileFailure(CompileStageSeal, CompileRowAuthority, -1, -1, CompileReasonArtifactIdentity)
	}
	if !sortedPoints(artifact.points) {
		return compileFailure(CompileStageSeal, CompileRowPoint, -1, -1, CompileReasonPointOrder)
	}
	pointRows := make(map[keyspace.ContentID]struct{}, len(artifact.points))
	for index, row := range artifact.points {
		if !row.Available() {
			return compileFailure(CompileStageSeal, CompileRowPoint, index, -1, CompileReasonPointUnavailable)
		}
		for decisionIndex, decision := range row.decisions {
			if !decision.Available() || decisionIndex > 0 && !contentIDBefore(row.decisions[decisionIndex-1], decision) {
				return compileFailure(CompileStageSeal, CompileRowPoint, index, decisionIndex, CompileReasonPointUnavailable)
			}
		}
		pointRows[row.id] = struct{}{}
	}
	seenDiagnosticObservations := make(map[keyspace.ContentID]struct{}, len(artifact.diagnosticObservations))
	for index, row := range artifact.diagnosticObservations {
		if !row.Available() {
			return compileFailure(CompileStageSeal, CompileRowRoute, index, -1, CompileReasonRouteGuard)
		}
		if row.Kind() == DiagnosticObservationBranchCondition {
			branch, branchOK := row.BranchCondition()
			if !branchOK {
				return compileFailure(CompileStageSeal, CompileRowRoute, index, -1, CompileReasonRouteGuard)
			}
			for pointIndex := 0; pointIndex < branch.EvidencePointCount(); pointIndex++ {
				point, pointOK := branch.EvidencePointAt(pointIndex)
				if !pointOK || !point.Available() {
					return compileFailure(CompileStageSeal, CompileRowRoute, index, pointIndex, CompileReasonRouteEndpoints)
				}
				if _, exists := pointRows[point]; !exists {
					return compileFailure(CompileStageSeal, CompileRowRoute, index, pointIndex, CompileReasonRouteEndpoints)
				}
			}
		}
		if _, duplicate := seenDiagnosticObservations[row.id]; duplicate || index > 0 && !contentIDBefore(artifact.diagnosticObservations[index-1].id, row.id) {
			return compileFailure(CompileStageSeal, CompileRowRoute, index, -1, CompileReasonRouteGuard)
		}
		seenDiagnosticObservations[row.id] = struct{}{}
	}
	valueRows := make(map[keyspace.ContentID]struct{}, len(artifact.values))
	for index, row := range artifact.values {
		if !row.Available() {
			return compileFailure(CompileStageSeal, CompileRowValues, index, -1, CompileReasonValuesUnavailable)
		}
		if _, duplicate := valueRows[row.id]; duplicate {
			return compileFailure(CompileStageSeal, CompileRowValues, index, -1, CompileReasonValuesDuplicate)
		}
		valueRows[row.id] = struct{}{}
		memberRows := make(map[keyspace.ContentID]struct{}, len(row.members))
		for memberIndex, member := range row.members {
			if !member.Available() {
				return compileFailure(CompileStageSeal, CompileRowValues, index, memberIndex, CompileReasonValuesMember)
			}
			if _, duplicate := memberRows[member.id]; duplicate {
				return compileFailure(CompileStageSeal, CompileRowValues, index, memberIndex, CompileReasonValuesDuplicate)
			}
			memberRows[member.id] = struct{}{}
		}
	}
	bodyRows := make(map[keyspace.ContentID]BodyRow, len(artifact.bodies))
	rootRows := make(map[keyspace.ContentID]struct{})
	outcomeRows := make(map[keyspace.ContentID]int, len(artifact.outcomes))
	if len(artifact.bodies) == 0 || !fitsUint32(len(artifact.bodies)) || !fitsUint32(len(artifact.outcomes)) || !fitsUint32(len(artifact.returnValues)) {
		return compileFailure(CompileStageSeal, CompileRowBody, -1, -1, CompileReasonBodyRange)
	}
	outcomeCursor := uint32(0)
	for bodyIndex, row := range artifact.bodies {
		if !row.Available() || row.outcomeStart != outcomeCursor || uint64(row.outcomeEnd) > uint64(len(artifact.outcomes)) {
			return compileFailure(CompileStageSeal, CompileRowBody, bodyIndex, -1, CompileReasonBodyRange)
		}
		if _, duplicate := bodyRows[row.id]; duplicate {
			return compileFailure(CompileStageSeal, CompileRowBody, bodyIndex, -1, CompileReasonBodyDuplicate)
		}
		bodyRows[row.id] = row
		for rootIndex, root := range row.roots {
			if !root.Available() {
				return compileFailure(CompileStageSeal, CompileRowBody, bodyIndex, rootIndex, CompileReasonBodyUnavailable)
			}
			if _, duplicate := rootRows[root.id]; duplicate {
				return compileFailure(CompileStageSeal, CompileRowBody, bodyIndex, rootIndex, CompileReasonBodyDuplicate)
			}
			rootRows[root.id] = struct{}{}
		}
		if len(row.entryPoints) == 0 {
			return compileFailure(CompileStageSeal, CompileRowBody, bodyIndex, -1, CompileReasonBodyUnavailable)
		}
		entryRows := make(map[keyspace.ContentID]struct{}, len(row.entryPoints))
		for pointIndex, point := range row.entryPoints {
			if !point.Available() {
				return compileFailure(CompileStageSeal, CompileRowBody, bodyIndex, pointIndex, CompileReasonBodyUnavailable)
			}
			if _, known := pointRows[point]; !known {
				return compileFailure(CompileStageSeal, CompileRowBody, bodyIndex, pointIndex, CompileReasonBodyUnavailable)
			}
			if _, duplicate := entryRows[point]; duplicate {
				return compileFailure(CompileStageSeal, CompileRowBody, bodyIndex, pointIndex, CompileReasonBodyUnavailable)
			}
			entryRows[point] = struct{}{}
		}
		var mandatory [OutcomeCancel + 1]bool
		for outcomeIndex := row.outcomeStart; outcomeIndex < row.outcomeEnd; outcomeIndex++ {
			outcome := artifact.outcomes[outcomeIndex]
			if outcome.body != row.id {
				return compileFailure(CompileStageSeal, CompileRowOutcome, bodyIndex, int(outcomeIndex-row.outcomeStart), CompileReasonOutcomeBody)
			}
			switch outcome.kind {
			case OutcomeNormal, OutcomeThrow, OutcomeYield, OutcomeCancel:
				mandatory[outcome.kind] = true
			}
		}
		for _, kind := range [...]OutcomeKind{OutcomeNormal, OutcomeThrow, OutcomeYield, OutcomeCancel} {
			if !mandatory[kind] {
				return compileFailure(CompileStageSeal, CompileRowOutcome, bodyIndex, -1, CompileReasonOutcomeKind)
			}
		}
		outcomeCursor = row.outcomeEnd
	}
	callableBodies := 0
	for _, body := range artifact.bodies {
		if body.Callable() {
			callableBodies++
		}
	}
	seenFunctions := make(map[keyspace.ContentID]struct{}, len(artifact.functionBoundaries))
	seenFunctionBodies := make(map[keyspace.ContentID]struct{}, len(artifact.functionBoundaries))
	for functionIndex, row := range artifact.functionBoundaries {
		body, bodyOK := bodyRows[row.body]
		if !row.Available() || !bodyOK || !body.Callable() || body.context != row.bodyContext || body.entry != row.entry ||
			body.function != row.id || body.formal != row.callFormal {
			return compileFailure(CompileStageSeal, CompileRowBody, functionIndex, -1, CompileReasonBodyUnavailable)
		}
		if _, duplicate := seenFunctions[row.id]; duplicate {
			return compileFailure(CompileStageSeal, CompileRowBody, functionIndex, -1, CompileReasonBodyDuplicate)
		}
		if _, duplicate := seenFunctionBodies[row.body]; duplicate {
			return compileFailure(CompileStageSeal, CompileRowBody, functionIndex, -1, CompileReasonBodyDuplicate)
		}
		seenFunctions[row.id], seenFunctionBodies[row.body] = struct{}{}, struct{}{}
		seenFormalIDs := make(map[keyspace.ContentID]struct{}, len(row.formals))
		seenFormalCells := make(map[keyspace.ContentID]struct{}, len(row.formals))
		seenFormalStorage := make(map[keyspace.ContentID]struct{}, len(row.formals))
		for portIndex, port := range row.formals {
			if !port.Available() || uint64(port.position) != uint64(portIndex) {
				return compileFailure(CompileStageSeal, CompileRowBody, functionIndex, portIndex, CompileReasonBodyUnavailable)
			}
			if _, duplicate := seenFormalIDs[port.id]; duplicate {
				return compileFailure(CompileStageSeal, CompileRowBody, functionIndex, portIndex, CompileReasonBodyDuplicate)
			}
			if _, duplicate := seenFormalCells[port.cell]; duplicate {
				return compileFailure(CompileStageSeal, CompileRowBody, functionIndex, portIndex, CompileReasonBodyDuplicate)
			}
			if _, duplicate := seenFormalStorage[port.storage]; duplicate {
				return compileFailure(CompileStageSeal, CompileRowBody, functionIndex, portIndex, CompileReasonBodyDuplicate)
			}
			seenFormalIDs[port.id], seenFormalCells[port.cell], seenFormalStorage[port.storage] = struct{}{}, struct{}{}, struct{}{}
		}
		seenCaptureIDs := make(map[keyspace.ContentID]struct{}, len(row.captures))
		for captureIndex, capture := range row.captures {
			if !capture.Available() || capture.innerBody != row.body {
				return compileFailure(CompileStageSeal, CompileRowBody, functionIndex, captureIndex, CompileReasonBodyUnavailable)
			}
			if _, outerOK := bodyRows[capture.outerBody]; !outerOK {
				return compileFailure(CompileStageSeal, CompileRowBody, functionIndex, captureIndex, CompileReasonBodyUnavailable)
			}
			if _, duplicate := seenCaptureIDs[capture.id]; duplicate {
				return compileFailure(CompileStageSeal, CompileRowBody, functionIndex, captureIndex, CompileReasonBodyDuplicate)
			}
			seenCaptureIDs[capture.id] = struct{}{}
		}
		if len(row.outcomes) != body.OutcomeCount() {
			return compileFailure(CompileStageSeal, CompileRowOutcome, functionIndex, -1, CompileReasonBodyRange)
		}
		for outcomeIndex, id := range row.outcomes {
			artifactIndex := uint64(body.outcomeStart) + uint64(outcomeIndex)
			if artifactIndex >= uint64(len(artifact.outcomes)) || artifact.outcomes[artifactIndex].id != id {
				return compileFailure(CompileStageSeal, CompileRowOutcome, functionIndex, outcomeIndex, CompileReasonOutcomeReference)
			}
		}
	}
	if len(artifact.functionBoundaries) != callableBodies || !artifact.packReceipt.Available() || !artifact.packReceipt.validAgainst(artifact) {
		return compileFailure(CompileStageSeal, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceStorage)
	}
	seenCallAllocations := make(map[keyspace.ContentID]struct{}, len(artifact.callTargets))
	seenCallBodies := make(map[keyspace.ContentID]struct{}, len(artifact.callTargets))
	bodyByContext := make(map[keyspace.ContentID]BodyRow, len(artifact.bodies))
	for _, body := range artifact.bodies {
		if _, duplicate := bodyByContext[body.context]; duplicate {
			return compileFailure(CompileStageSeal, CompileRowBody, -1, -1, CompileReasonBodyDuplicate)
		}
		bodyByContext[body.context] = body
	}
	for index, target := range artifact.callTargets {
		body, bodyOK := bodyByContext[target.context]
		if !target.Available() || !bodyOK || !body.Callable() || body.ID() != target.body ||
			body.context != target.context || body.function != target.function || body.formal != target.formal {
			return compileFailure(CompileStageSeal, CompileRowBody, index, -1, CompileReasonBodyUnavailable)
		}
		if _, duplicate := seenCallAllocations[target.allocation]; duplicate {
			return compileFailure(CompileStageSeal, CompileRowBody, index, -1, CompileReasonBodyDuplicate)
		}
		if _, duplicate := seenCallBodies[target.context]; duplicate {
			return compileFailure(CompileStageSeal, CompileRowBody, index, -1, CompileReasonBodyDuplicate)
		}
		seenCallAllocations[target.allocation], seenCallBodies[target.context] = struct{}{}, struct{}{}
	}
	seenBoundaries := make(map[keyspace.ContentID]struct{}, len(artifact.boundaries))
	for index, row := range artifact.boundaries {
		if !row.Available() || (row.kind == BoundaryCapture && uint64(row.position) > uint64(^uint32(0))) {
			return compileFailure(CompileStageSeal, CompileRowBody, index, -1, CompileReasonBodyUnavailable)
		}
		if _, duplicate := seenBoundaries[row.id]; duplicate {
			return compileFailure(CompileStageSeal, CompileRowBody, index, -1, CompileReasonBodyDuplicate)
		}
		seenBoundaries[row.id] = struct{}{}
	}
	if outcomeCursor != uint32(len(artifact.outcomes)) {
		return compileFailure(CompileStageSeal, CompileRowBody, -1, -1, CompileReasonBodyRange)
	}
	for index, row := range artifact.values {
		if _, exists := bodyRows[row.body]; !exists {
			return compileFailure(CompileStageSeal, CompileRowValues, index, -1, CompileReasonValuesBody)
		}
	}
	seenStaticArguments := make(map[keyspace.ContentID]struct{}, len(artifact.staticTypeArguments))
	for index, row := range artifact.staticTypeArguments {
		if !row.Available() {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		if _, duplicate := seenStaticArguments[row.id]; duplicate {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		seenStaticArguments[row.id] = struct{}{}
	}
	seenStaticValues := make(map[keyspace.ContentID]struct{}, len(artifact.staticTypeValues))
	for index, row := range artifact.staticTypeValues {
		if !row.Available() {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		if _, duplicate := seenStaticValues[row.id]; duplicate {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		seenStaticValues[row.id] = struct{}{}
		if _, exists := bodyRows[row.body]; !exists {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
	}
	seenStaticNodes := make(map[keyspace.ContentID]struct{}, len(artifact.staticTypeNodes))
	for index, row := range artifact.staticTypeNodes {
		// A TypeRefUnresolved is a complete Static leaf: Static sealed its
		// targetless disposition and ProgramArtifact retained its exact lexical
		// proof as a DiagnosticObservation. All other references must retain
		// their resolved/canonical target edge.
		zeroChildAllowed := row.Kind() == StaticNodePrimitive || row.Kind() == StaticNodeLiteral || row.Kind() == StaticNodeUnknown || row.Kind() == StaticNodeTypeParam || row.Kind() == StaticNodeInterface || row.Kind() == StaticNodeTypeFunction ||
			row.Kind() == StaticNodeReference && row.Resolution() == uint8(programstatic.TypeRefUnresolved)
		if !row.Available() || row.ChildCount() == 0 && !zeroChildAllowed || row.Kind() == StaticNodeTypeOf && !row.operand.Available() {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		if _, duplicate := seenStaticNodes[row.id]; duplicate {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		seenStaticNodes[row.id] = struct{}{}
	}
	for functionIndex, function := range artifact.functionBoundaries {
		for formalIndex, formal := range function.formals {
			if formal.declared.Available() {
				if _, exists := seenStaticNodes[formal.declared]; !exists {
					return compileFailure(CompileStageSeal, CompileRowBody, functionIndex, formalIndex, CompileReasonBodyUnavailable)
				}
			}
		}
	}
	for index, row := range artifact.staticTypeNodes {
		for childIndex := 0; childIndex < row.ChildCount(); childIndex++ {
			child, ok := row.ChildAt(childIndex)
			if !ok {
				return compileFailure(CompileStageSeal, CompileRowOccurrence, index, childIndex, CompileReasonOccurrenceUnavailable)
			}
			if _, exists := seenStaticNodes[child]; !exists {
				return compileFailure(CompileStageSeal, CompileRowOccurrence, index, childIndex, CompileReasonOccurrenceUnavailable)
			}
		}
	}
	seenStaticExpressions := make(map[keyspace.ContentID]struct{}, len(artifact.staticExpressions))
	for index, row := range artifact.staticExpressions {
		if !row.Available() {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		if _, duplicate := seenStaticExpressions[row.id]; duplicate {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		seenStaticExpressions[row.id] = struct{}{}
		if _, exists := seenStaticNodes[row.reference]; !exists {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
	}
	seenStaticInputs := make(map[keyspace.ContentID]struct{}, len(artifact.staticInputs))
	for index, row := range artifact.staticInputs {
		if !row.Available() {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		if _, duplicate := seenStaticInputs[row.id]; duplicate {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		seenStaticInputs[row.id] = struct{}{}
		if _, exists := seenStaticExpressions[row.expression]; !exists {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
	}
	returnCursor := uint32(0)
	for index, row := range artifact.outcomes {
		if !row.Available() {
			return compileFailure(CompileStageSeal, CompileRowOutcome, index, -1, CompileReasonOutcomeUnavailable)
		}
		if _, exists := bodyRows[row.body]; !exists {
			return compileFailure(CompileStageSeal, CompileRowOutcome, index, -1, CompileReasonOutcomeBody)
		}
		if _, duplicate := outcomeRows[row.id]; duplicate {
			return compileFailure(CompileStageSeal, CompileRowOutcome, index, -1, CompileReasonOutcomeDuplicate)
		}
		outcomePoints := make(map[keyspace.ContentID]struct{}, len(row.points))
		for pointIndex, point := range row.points {
			if !point.Available() {
				return compileFailure(CompileStageSeal, CompileRowOutcome, index, pointIndex, CompileReasonOutcomeUnavailable)
			}
			if _, known := pointRows[point]; !known {
				return compileFailure(CompileStageSeal, CompileRowOutcome, index, pointIndex, CompileReasonOutcomeUnavailable)
			}
			if _, duplicate := outcomePoints[point]; duplicate {
				return compileFailure(CompileStageSeal, CompileRowOutcome, index, pointIndex, CompileReasonOutcomeUnavailable)
			}
			outcomePoints[point] = struct{}{}
		}
		if row.returnStart != returnCursor || uint64(row.returnEnd) > uint64(len(artifact.returnValues)) {
			return compileFailure(CompileStageSeal, CompileRowOutcome, index, -1, CompileReasonOutcomeRange)
		}
		for valueIndex := row.returnStart; valueIndex < row.returnEnd; valueIndex++ {
			value := artifact.returnValues[valueIndex]
			if !value.Available() {
				return compileFailure(CompileStageSeal, CompileRowReturnValue, index, int(valueIndex-row.returnStart), CompileReasonReturnValueUnavailable)
			}
			if _, exists := valueRows[value.id]; !exists {
				return compileFailure(CompileStageSeal, CompileRowReturnValue, index, int(valueIndex-row.returnStart), CompileReasonReturnValueReference)
			}
		}
		outcomeRows[row.id] = index
		returnCursor = row.returnEnd
	}
	if returnCursor != uint32(len(artifact.returnValues)) {
		return compileFailure(CompileStageSeal, CompileRowReturnValue, -1, -1, CompileReasonOutcomeRange)
	}
	for index, row := range artifact.outcomes {
		if !row.hasPropagation {
			continue
		}
		nextIndex, exists := outcomeRows[row.propagation]
		if !exists || nextIndex == index {
			return compileFailure(CompileStageSeal, CompileRowOutcome, index, -1, CompileReasonOutcomeReference)
		}
		next := artifact.outcomes[nextIndex]
		if next.kind != row.kind || next.hasTarget != row.hasTarget || next.target != row.target {
			return compileFailure(CompileStageSeal, CompileRowOutcome, index, -1, CompileReasonOutcomePropagation)
		}
	}
	environmentByRoute := make(map[keyspace.ContentID]EnvironmentEdge, len(artifact.environment))
	environmentRouteDuplicates := make(map[keyspace.ContentID]struct{})
	for index, row := range artifact.environment {
		if !row.Available() {
			return compileFailure(CompileStageSeal, CompileRowEnvironment, index, -1, CompileReasonEnvironmentUnavailable)
		}
		if _, exists := environmentByRoute[row.route]; exists {
			environmentRouteDuplicates[row.route] = struct{}{}
		} else {
			environmentByRoute[row.route] = row
		}
		if _, exists := pointRows[row.from]; !exists {
			return compileFailure(CompileStageSeal, CompileRowEnvironment, index, 0, CompileReasonEnvironmentEndpointUnknown)
		}
		if _, exists := pointRows[row.to]; !exists {
			return compileFailure(CompileStageSeal, CompileRowEnvironment, index, 1, CompileReasonEnvironmentEndpointUnknown)
		}
		for resetIndex, reset := range row.resets {
			if !reset.Available() {
				return compileFailure(CompileStageSeal, CompileRowEnvironment, index, resetIndex, CompileReasonRouteResetMember)
			}
			if resetIndex != 0 && !contentIDBefore(row.resets[resetIndex-1], reset) {
				return compileFailure(CompileStageSeal, CompileRowEnvironment, index, resetIndex, CompileReasonRouteResetOrder)
			}
		}
	}
	seenLocalTransfers := make(map[keyspace.ContentID]struct{}, len(artifact.localTransfers))
	for index, row := range artifact.localTransfers {
		if !row.Available() {
			return compileFailure(CompileStageSeal, CompileRowEnvironment, index, -1, CompileReasonEnvironmentUnavailable)
		}
		if _, exists := pointRows[row.from]; !exists {
			return compileFailure(CompileStageSeal, CompileRowEnvironment, index, 0, CompileReasonEnvironmentEndpointUnknown)
		}
		if _, exists := pointRows[row.to]; !exists {
			return compileFailure(CompileStageSeal, CompileRowEnvironment, index, 1, CompileReasonEnvironmentEndpointUnknown)
		}
		if _, duplicate := seenLocalTransfers[row.id]; duplicate {
			return compileFailure(CompileStageSeal, CompileRowEnvironment, index, -1, CompileReasonEnvironmentDuplicate)
		}
		seenLocalTransfers[row.id] = struct{}{}
	}
	regionRows := make(map[keyspace.ContentID]struct{}, len(artifact.regions))
	for index, row := range artifact.regions {
		if !row.id.Available() || !row.head.Available() || !row.sourceHead.Available() || len(row.members) == 0 {
			return compileFailure(CompileStageSeal, CompileRowRegion, index, -1, CompileReasonRegionUnavailable)
		}
		if _, exists := regionRows[row.id]; exists {
			return compileFailure(CompileStageSeal, CompileRowRegion, index, -1, CompileReasonRegionDuplicate)
		}
		regionRows[row.id] = struct{}{}
		if _, exists := pointRows[row.head]; !exists || row.members[0] != row.head {
			return compileFailure(CompileStageSeal, CompileRowRegion, index, 0, CompileReasonRegionHeaderMismatch)
		}
		if _, exists := pointRows[row.sourceHead]; !exists {
			return compileFailure(CompileStageSeal, CompileRowRegion, index, 0, CompileReasonRegionHeaderMismatch)
		}
		for memberIndex, member := range row.members {
			if _, exists := pointRows[member]; !exists || memberIndex != 0 && member == row.members[memberIndex-1] {
				return compileFailure(CompileStageSeal, CompileRowRegion, index, memberIndex, CompileReasonRegionReference)
			}
		}
	}
	for index, row := range artifact.regions {
		if row.parent.Available() {
			if _, exists := regionRows[row.parent]; !exists {
				return compileFailure(CompileStageSeal, CompileRowRegion, index, -1, CompileReasonRegionReference)
			}
		}
	}
	for index, event := range artifact.events {
		if !event.Available() {
			return compileFailure(CompileStageSeal, CompileRowWTOEvent, index, -1, CompileReasonEventUnavailable)
		}
		if event.kind == WTOEventPoint {
			if _, exists := pointRows[event.point]; !exists {
				return compileFailure(CompileStageSeal, CompileRowWTOEvent, index, -1, CompileReasonEventReference)
			}
		} else if _, exists := regionRows[event.region]; !exists {
			return compileFailure(CompileStageSeal, CompileRowWTOEvent, index, -1, CompileReasonEventReference)
		}
	}
	occurrenceRows := make(map[OccurrenceKind]map[keyspace.ContentID]struct{})
	for index, row := range artifact.occurrences {
		if !row.Available() {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		if row.body.Available() {
			if _, exists := bodyRows[row.body]; !exists {
				return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
			}
		}
		for pointIndex, point := range row.points {
			if _, exists := pointRows[point]; !exists {
				return compileFailure(CompileStageSeal, CompileRowOccurrence, index, pointIndex, CompileReasonOccurrenceUnavailable)
			}
		}
		rows := occurrenceRows[row.kind]
		if rows == nil {
			rows = make(map[keyspace.ContentID]struct{})
			occurrenceRows[row.kind] = rows
		}
		if _, duplicate := rows[row.id]; duplicate {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		rows[row.id] = struct{}{}
	}
	for index, row := range artifact.exactScalarSummaries {
		if !row.Available() || index > 0 && !contentIDBefore(artifact.exactScalarSummaries[index-1].id, row.id) {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		_, exists := occurrenceRows[OccurrenceBinaryArithmetic][row.occurrence]
		if !exists {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		binary, found := artifact.occurrenceByID[occurrenceLookup{kind: OccurrenceBinaryArithmetic, id: row.occurrence}]
		if !found || uint64(binary) >= uint64(len(artifact.occurrences)) || artifact.occurrences[binary].body != row.body {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		left, right, _, endpointsOK := artifact.occurrences[binary].BinaryArithmetic()
		wantSubject := artifact.occurrences[binary].ID()
		switch row.role {
		case ExactScalarSummaryLeft:
			wantSubject = left
		case ExactScalarSummaryRight:
			wantSubject = right
		case ExactScalarSummaryResult:
		default:
			endpointsOK = false
		}
		if !endpointsOK || row.subject != wantSubject {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
	}
	for index, row := range artifact.arithmeticSummaries {
		if !row.Available() || index > 0 && !contentIDBefore(artifact.arithmeticSummaries[index-1].id, row.id) {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		binary, found := artifact.occurrenceByID[occurrenceLookup{kind: OccurrenceBinaryArithmetic, id: row.occurrence}]
		if !found || uint64(binary) >= uint64(len(artifact.occurrences)) {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
		occurrence := artifact.occurrences[binary]
		_, _, op, endpointsOK := occurrence.BinaryArithmetic()
		if !endpointsOK || occurrence.body != row.body || op != row.op {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
		}
	}
	for index, row := range artifact.unarySummaries {
		if !row.Available() || index > 0 && !contentIDBefore(artifact.unarySummaries[index-1].id, row.id) {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, 0, CompileReasonOccurrenceUnavailable)
		}
		unary, found := artifact.occurrenceByID[occurrenceLookup{kind: OccurrenceUnary, id: row.occurrence}]
		if !found || uint64(unary) >= uint64(len(artifact.occurrences)) {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, 1, CompileReasonOccurrenceUnavailable)
		}
		occurrence := artifact.occurrences[unary]
		if occurrence.body != row.body {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, 2, CompileReasonOccurrenceUnavailable)
		}
		if flowkind.UnaryOp(occurrence.code) != row.op {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, 3, CompileReasonOccurrenceUnavailable)
		}
		pointFound := false
		for _, point := range occurrence.points {
			pointFound = pointFound || point == row.point
		}
		if !pointFound {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, 5, CompileReasonOccurrenceUnavailable)
		}
	}
	valuesRows := make(map[keyspace.ContentID]struct{}, len(artifact.values))
	for _, row := range artifact.values {
		if !row.Available() {
			return compileFailure(CompileStageSeal, CompileRowValues, -1, -1, CompileReasonValuesUnavailable)
		}
		valuesRows[row.ID()] = struct{}{}
	}
	seenHeapAllocations := make(map[keyspace.ContentID]struct{}, len(artifact.heapAllocations))
	for index, allocation := range artifact.heapAllocations {
		if !allocation.Available() {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceAllocation)
		}
		if _, exists := occurrenceRows[OccurrenceAllocation][allocation.id]; !exists {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceAllocation)
		}
		if _, duplicate := seenHeapAllocations[allocation.id]; duplicate {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceAllocation)
		}
		seenHeapAllocations[allocation.id] = struct{}{}
		for fieldIndex, field := range allocation.fields {
			if !field.Available() {
				return compileFailure(CompileStageSeal, CompileRowOccurrence, index, fieldIndex, CompileReasonOccurrenceAllocation)
			}
			if _, exists := occurrenceRows[OccurrenceAllocationField][field.id]; !exists {
				return compileFailure(CompileStageSeal, CompileRowOccurrence, index, fieldIndex, CompileReasonOccurrenceAllocation)
			}
			if _, exists := valuesRows[field.valuesID]; !exists {
				return compileFailure(CompileStageSeal, CompileRowOccurrence, index, fieldIndex, CompileReasonOccurrenceAllocation)
			}
		}
	}
	seenHeapIndexes := make(map[keyspace.ContentID]struct{}, len(artifact.heapIndexes))
	for index, access := range artifact.heapIndexes {
		if !access.Available() {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceIndexShape)
		}
		kind := OccurrenceIndexWrite
		if access.read {
			kind = OccurrenceIndexRead
		}
		if _, exists := occurrenceRows[kind][access.id]; !exists {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceIndexShape)
		}
		if _, duplicate := seenHeapIndexes[access.id]; duplicate {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceIndexShape)
		}
		if !access.read {
			if _, exists := valuesRows[access.valuesID]; !exists {
				return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceIndexShape)
			}
		}
		seenHeapIndexes[access.id] = struct{}{}
	}
	if artifact.ruleOccurrences == nil {
		return compileFailure(CompileStageSeal, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceUnavailable)
	}
	for role, rows := range artifact.ruleOccurrences {
		if !role.valid() || !ruleRoleSupported(role) {
			return compileFailure(CompileStageSeal, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceUnavailable)
		}
		for index, occurrence := range rows {
			if !occurrence.Available() || occurrence.role != role || int(occurrence.occurrence) >= len(artifact.occurrences) {
				return compileFailure(CompileStageSeal, CompileRowOccurrence, index, -1, CompileReasonOccurrenceUnavailable)
			}
			if _, exists := pointRows[occurrence.point]; !exists {
				return compileFailure(CompileStageSeal, CompileRowOccurrence, index, 0, CompileReasonOccurrenceUnavailable)
			}
			if occurrence.input.Available() {
				if _, exists := pointRows[occurrence.input]; !exists {
					return compileFailure(CompileStageSeal, CompileRowOccurrence, index, 1, CompileReasonOccurrenceUnavailable)
				}
			}
			if occurrence.inputKind == RuleInputPredecessor {
				if _, duplicate := environmentRouteDuplicates[occurrence.route]; duplicate {
					return compileFailure(CompileStageSeal, CompileRowOccurrence, index, 2, CompileReasonOccurrenceUnavailable)
				}
				edge, found := environmentByRoute[occurrence.route]
				if !found || edge.from != occurrence.input {
					return compileFailure(CompileStageSeal, CompileRowOccurrence, index, 2, CompileReasonOccurrenceUnavailable)
				}
			}
		}
	}
	return CompileFailure{}
}

func routeKind(kind flow.BoundaryArmKind) (RouteKind, bool) {
	switch kind {
	case flow.BoundaryLocal:
		return RouteLocal, true
	case flow.BoundaryResume:
		return RouteResume, true
	case flow.BoundarySelectTrue:
		return RouteSelectTrue, true
	case flow.BoundarySelectFalse:
		return RouteSelectFalse, true
	case flow.BoundaryTail:
		return RouteTail, true
	case flow.BoundaryThrow:
		return RouteThrow, true
	case flow.BoundaryYield:
		return RouteYield, true
	case flow.BoundaryCancel:
		return RouteCancel, true
	default:
		return RouteInvalid, false
	}
}

type field struct {
	bytes []byte
	uint  uint64
	kind  uint8
}

const (
	fieldBytes uint8 = iota + 1
	fieldUint
	fieldBool
)

func bytesField(value keyspace.ContentID) field { return field{bytes: value[:], kind: fieldBytes} }
func uintField(value uint64) field              { return field{uint: value, kind: fieldUint} }
func boolField(value bool) field {
	if value {
		return field{uint: 1, kind: fieldBool}
	}
	return field{kind: fieldBool}
}

func digest(domain string, version uint64, fields ...field) keyspace.ContentID {
	var writer canonical.DigestWriter
	if writer.Reset(domain, version) != nil {
		return keyspace.ContentID{}
	}
	for _, value := range fields {
		var err error
		switch value.kind {
		case fieldBytes:
			err = writer.Bytes(value.bytes)
		case fieldUint, fieldBool:
			err = writer.Uint(value.uint)
		default:
			return keyspace.ContentID{}
		}
		if err != nil {
			return keyspace.ContentID{}
		}
	}
	if writer.Finish() != nil {
		return keyspace.ContentID{}
	}
	return keyspace.ContentID(writer.Sum())
}

func artifactID(artifact *Artifact) keyspace.ContentID {
	fields := append([]field{bytesField(artifact.key.ID())}, artifact.key.identityFields()...)
	fields = append(fields, uintField(pointGeometryLawVersion))
	fields = append(fields, uintField(uint64(len(artifact.points))))
	for _, point := range artifact.points {
		fields = append(fields, bytesField(point.id), boolField(point.initial), uintField(uint64(len(point.decisions))))
		for _, decision := range point.decisions {
			fields = append(fields, bytesField(decision))
		}
	}
	fields = append(fields, uintField(uint64(len(artifact.values))))
	for _, row := range artifact.values {
		fields = append(fields, bytesField(row.id), bytesField(row.body), uintField(uint64(len(row.members))))
		for _, member := range row.members {
			fields = append(fields, bytesField(member.id))
		}
		fields = append(fields, boolField(row.tail.present), uintField(uint64(row.tail.kind)), bytesField(row.tail.id))
	}
	fields = append(fields, uintField(packReceiptLawVersion), uintField(uint64(artifact.packReceipt.BindCount())))
	for index := 0; index < artifact.packReceipt.BindCount(); index++ {
		row, _ := artifact.packReceipt.BindAt(index)
		fields = append(fields, bytesField(row.ID()), bytesField(row.BodyID()), bytesField(row.ValuesID()), uintField(uint64(row.CellCount())))
		for cellIndex := 0; cellIndex < row.CellCount(); cellIndex++ {
			cell, _ := row.CellAt(cellIndex)
			fields = append(fields, bytesField(cell))
		}
	}
	fields = append(fields, uintField(uint64(artifact.packReceipt.BodyCount())))
	for index := 0; index < artifact.packReceipt.BodyCount(); index++ {
		row, _ := artifact.packReceipt.BodyAt(index)
		fields = append(fields, bytesField(row.ID()), bytesField(row.ContextID()), boolField(row.Callable()), uintField(uint64(row.FormalCount())))
		for formalIndex := 0; formalIndex < row.FormalCount(); formalIndex++ {
			formal, _ := row.FormalAt(formalIndex)
			fields = append(fields, bytesField(formal.FormalID()), bytesField(formal.CellID()), bytesField(formal.StorageCellID()))
		}
	}
	fields = append(fields, uintField(uint64(artifact.packReceipt.CallCount())))
	for index := 0; index < artifact.packReceipt.CallCount(); index++ {
		row, _ := artifact.packReceipt.CallAt(index)
		fields = append(fields, bytesField(row.ID()), bytesField(row.BodyID()), bytesField(row.FormalID()), bytesField(row.ValuesID()), bytesField(row.TypeArgumentsID()), bytesField(row.CalleeID()), bytesField(row.ActualsID()), uintField(uint64(row.Form())))
		receiver, hasReceiver := row.ReceiverID()
		tail, hasTail := row.TailID()
		fields = append(fields, boolField(hasReceiver), bytesField(receiver), boolField(hasTail), bytesField(tail), uintField(uint64(row.ArgumentCount())))
		for argumentIndex := 0; argumentIndex < row.ArgumentCount(); argumentIndex++ {
			argument, _ := row.ArgumentAt(argumentIndex)
			fields = append(fields, bytesField(argument))
		}
		fields = append(fields, uintField(uint64(row.TypeArgumentCount())))
		for argumentIndex := 0; argumentIndex < row.TypeArgumentCount(); argumentIndex++ {
			argument, _ := row.TypeArgumentAt(argumentIndex)
			fields = append(fields, bytesField(argument))
		}
	}
	fields = append(fields, uintField(uint64(len(artifact.bodies))))
	for _, body := range artifact.bodies {
		fields = append(fields, bytesField(body.id), bytesField(body.context), bytesField(body.entry), boolField(body.callable), bytesField(body.function), bytesField(body.formal), uintField(uint64(len(body.entryPoints))))
		for _, point := range body.entryPoints {
			fields = append(fields, bytesField(point))
		}
		fields = append(fields, uintField(uint64(len(body.roots))))
		for _, root := range body.roots {
			fields = append(fields, bytesField(root.id), uintField(uint64(root.family)))
		}
		fields = append(fields, uintField(uint64(body.outcomeStart)), uintField(uint64(body.outcomeEnd)))
	}
	fields = append(fields, uintField(functionBoundaryLawVersion), uintField(uint64(len(artifact.functionBoundaries))))
	for _, boundary := range artifact.functionBoundaries {
		fields = append(fields,
			bytesField(boundary.id), bytesField(boundary.body), bytesField(boundary.bodyContext), bytesField(boundary.entry), bytesField(boundary.callFormal),
			uintField(uint64(len(boundary.formals))),
		)
		for _, port := range boundary.formals {
			fields = append(fields, bytesField(port.id), bytesField(port.cell), bytesField(port.storage), bytesField(port.declared), uintField(uint64(port.position)))
		}
		fields = append(fields, boolField(boundary.hasVararg), bytesField(boundary.vararg.id), bytesField(boundary.vararg.cell), uintField(uint64(len(boundary.captures))))
		for _, capture := range boundary.captures {
			fields = append(fields,
				bytesField(capture.id), bytesField(capture.inner), bytesField(capture.outer), bytesField(capture.innerBody), bytesField(capture.outerBody), uintField(uint64(capture.position)),
			)
		}
		fields = append(fields, uintField(uint64(len(boundary.outcomes))))
		for _, outcome := range boundary.outcomes {
			fields = append(fields, bytesField(outcome))
		}
	}
	fields = append(fields, uintField(uint64(len(artifact.callTargets))))
	for _, target := range artifact.callTargets {
		fields = append(fields, bytesField(target.allocation), bytesField(target.body), bytesField(target.context), bytesField(target.function), bytesField(target.formal))
	}
	fields = append(fields, uintField(uint64(len(artifact.boundaries))))
	for _, row := range artifact.boundaries {
		fields = append(fields, uintField(uint64(row.kind)), bytesField(row.id), bytesField(row.owner), uintField(uint64(row.position)), boolField(row.eligible))
	}
	fields = append(fields, uintField(uint64(len(artifact.outcomes))))
	for _, outcome := range artifact.outcomes {
		fields = append(fields,
			bytesField(outcome.id), bytesField(outcome.body), uintField(uint64(outcome.kind)),
			boolField(outcome.hasTarget), bytesField(outcome.target),
			boolField(outcome.hasPropagation), bytesField(outcome.propagation),
			uintField(uint64(outcome.returnStart)), uintField(uint64(outcome.returnEnd)), uintField(uint64(len(outcome.points))),
		)
		for _, point := range outcome.points {
			fields = append(fields, bytesField(point))
		}
	}
	fields = append(fields, uintField(uint64(len(artifact.returnValues))))
	for _, value := range artifact.returnValues {
		fields = append(fields, bytesField(value.id))
	}
	fields = append(fields, uintField(uint64(len(artifact.occurrences))))
	for _, row := range artifact.occurrences {
		fields = append(fields, uintField(uint64(row.kind)), bytesField(row.id), bytesField(row.body), uintField(row.code), uintField(uint64(len(row.points))))
		for _, point := range row.points {
			fields = append(fields, bytesField(point))
		}
		fields = append(fields, uintField(uint64(len(row.inputs))))
		for _, input := range row.inputs {
			fields = append(fields, bytesField(input))
		}
		fields = append(fields, uintField(uint64(row.literalFamily)), boolField(row.literalOK), uintField(uint64(row.literal.Kind)), boolField(row.literal.Bool), uintField(uint64(row.literal.Integer)), uintField(row.literal.FloatBits), field{bytes: []byte(row.literal.String), kind: fieldBytes})
	}
	fields = append(fields, uintField(uint64(len(artifact.exactScalarSummaries))))
	for _, row := range artifact.exactScalarSummaries {
		fields = append(fields, bytesField(row.id), bytesField(row.occurrence), bytesField(row.subject), bytesField(row.body),
			uintField(uint64(row.role)), uintField(uint64(row.literal.Kind)), uintField(uint64(row.literal.Integer)), uintField(row.literal.FloatBits))
	}
	fields = append(fields, uintField(uint64(len(artifact.arithmeticSummaries))))
	for _, row := range artifact.arithmeticSummaries {
		fields = append(fields, bytesField(row.id), bytesField(row.occurrence), bytesField(row.body), uintField(uint64(row.op)),
			uintField(uint64(row.left)), uintField(uint64(row.right)), uintField(uint64(row.result)), uintField(uint64(row.divisor)))
	}
	fields = append(fields, uintField(uint64(len(artifact.unarySummaries))))
	for _, row := range artifact.unarySummaries {
		fields = append(fields, bytesField(row.id), bytesField(row.occurrence), bytesField(row.body), bytesField(row.point), uintField(uint64(row.op)),
			uintField(uint64(row.operand)), uintField(uint64(row.result)))
	}
	fields = append(fields, uintField(uint64(len(artifact.heapAllocations))))
	for _, allocation := range artifact.heapAllocations {
		fields = append(fields, bytesField(allocation.id), uintField(uint64(allocation.role)), uintField(uint64(allocation.form)), bytesField(allocation.rootSpan), uintField(uint64(len(allocation.fields))))
		for _, field := range allocation.fields {
			fields = append(fields, bytesField(field.id), uintField(uint64(field.kind)), bytesField(field.fieldSpan), bytesField(field.selectorSpan), bytesField(field.valuesSpan), bytesField(field.valuesID), uintField(uint64(field.width)), boolField(field.finalOpen), boolField(field.sharesFirstValueCell), uintField(uint64(field.normalized)), boolField(field.normalizedOK))
		}
	}
	fields = append(fields, uintField(uint64(len(artifact.heapIndexes))))
	for _, access := range artifact.heapIndexes {
		fields = append(fields, bytesField(access.id), boolField(access.read), bytesField(access.baseSpan), bytesField(access.resultSpan), bytesField(access.keySpan), uintField(uint64(access.lensKind)), uintField(uint64(access.exactKey)), bytesField(access.valuesSpan), bytesField(access.valuesID), uintField(uint64(access.position+1)))
	}
	fields = append(fields, uintField(diagnosticLawVersion), uintField(uint64(len(artifact.diagnosticObservations))))
	for _, row := range artifact.diagnosticObservations {
		fields = append(fields,
			bytesField(row.id), uintField(uint64(row.kind)),
			field{bytes: []byte(row.location.File), kind: fieldBytes}, uintField(uint64(row.location.StartLine)),
			uintField(uint64(row.location.StartCol)), uintField(uint64(row.location.EndLine)), uintField(uint64(row.location.EndCol)),
		)
		switch row.kind {
		case DiagnosticObservationBranchCondition:
			fields = append(fields, bytesField(row.branch.decision), bytesField(row.branch.value), uintField(uint64(len(row.branch.points))))
			for _, point := range row.branch.points {
				fields = append(fields, bytesField(point))
			}
		case DiagnosticObservationTypeReferenceUnresolved:
			fields = append(fields, bytesField(row.unresolved.reference), bytesField(row.unresolved.root), uintField(uint64(len(row.unresolved.path))))
			for _, component := range row.unresolved.path {
				fields = append(fields, field{bytes: []byte(component), kind: fieldBytes})
			}
		case DiagnosticObservationValueReferenceUnresolved:
			fields = append(fields, bytesField(row.value.read), bytesField(row.value.cell), field{bytes: []byte(row.value.name), kind: fieldBytes})
		}
	}
	fields = append(fields, uintField(uint64(len(artifact.staticTypeArguments))))
	for _, row := range artifact.staticTypeArguments {
		fields = append(fields, bytesField(row.id), bytesField(row.call), bytesField(row.types), bytesField(row.reference), uintField(uint64(row.index)))
	}
	fields = append(fields, uintField(uint64(len(artifact.staticTypeValues))))
	for _, row := range artifact.staticTypeValues {
		fields = append(fields, bytesField(row.id), bytesField(row.body), bytesField(row.reference), bytesField(row.root), field{bytes: []byte(row.name), kind: fieldBytes})
	}
	fields = append(fields, uintField(uint64(len(artifact.staticTypeNodes))))
	for _, row := range artifact.staticTypeNodes {
		exact := row.exact
		fields = append(fields, bytesField(row.id), bytesField(row.owner), uintField(uint64(row.kind)), field{bytes: []byte(row.name), kind: fieldBytes}, uintField(uint64(row.key)), uintField(uint64(row.literal)), uintField(row.bits), uintField(uint64(exact.Kind)), boolField(exact.Bool), uintField(uint64(exact.Integer)), uintField(exact.FloatBits), field{bytes: []byte(exact.String), kind: fieldBytes}, boolField(row.flag), uintField(uint64(row.resolution)), uintField(uint64(row.assertParam)), bytesField(row.declaration), bytesField(row.operand), bytesField(row.scope), bytesField(row.assertionNarrow), uintField(uint64(row.assertionCoordinate[0])), uintField(uint64(row.assertionCoordinate[1])), uintField(uint64(row.assertionCoordinate[2])), uintField(uint64(row.assertionCoordinate[3])), bytesField(row.typeFunctionVariadic), uintField(uint64(len(row.aliasParams))))
		for _, child := range row.aliasParams {
			fields = append(fields, bytesField(child))
		}
		fields = append(fields, uintField(uint64(len(row.interfaceExtends))))
		for _, child := range row.interfaceExtends {
			fields = append(fields, bytesField(child))
		}
		fields = append(fields, uintField(uint64(len(row.interfaceMemberTypes))))
		for _, child := range row.interfaceMemberTypes {
			fields = append(fields, bytesField(child))
		}
		fields = append(fields, uintField(uint64(len(row.typeFunctionTypeParams))))
		for _, child := range row.typeFunctionTypeParams {
			fields = append(fields, bytesField(child))
		}
		fields = append(fields, uintField(uint64(len(row.typeFunctionParams))))
		for _, child := range row.typeFunctionParams {
			fields = append(fields, bytesField(child))
		}
		fields = append(fields, uintField(uint64(len(row.typeFunctionReturns))))
		for _, child := range row.typeFunctionReturns {
			fields = append(fields, bytesField(child))
		}
		fields = append(fields, uintField(uint64(len(row.fieldKeys))))
		for index, key := range row.fieldKeys {
			fields = append(fields, uintField(uint64(key)))
			text := ""
			if index < len(row.fieldTexts) {
				text = row.fieldTexts[index]
			}
			optional := false
			if index < len(row.fieldOptional) {
				optional = row.fieldOptional[index]
			}
			readonly := false
			if index < len(row.fieldReadonly) {
				readonly = row.fieldReadonly[index]
			}
			fields = append(fields, field{bytes: []byte(text), kind: fieldBytes}, boolField(optional), boolField(readonly))
		}
		fields = append(fields, uintField(uint64(len(row.keys))))
		for _, key := range row.keys {
			fields = append(fields, uintField(uint64(key)))
		}
		for index := range row.keys {
			text := ""
			if index < len(row.texts) {
				text = row.texts[index]
			}
			fields = append(fields, field{bytes: []byte(text), kind: fieldBytes})
			optional := false
			if index < len(row.optional) {
				optional = row.optional[index]
			}
			memberKind := uint8(0)
			if index < len(row.memberKinds) {
				memberKind = row.memberKinds[index]
			}
			fields = append(fields, boolField(optional), uintField(uint64(memberKind)))
		}
		fields = append(fields, uintField(uint64(len(row.segments))))
		for _, segment := range row.segments {
			fields = append(fields, uintField(uint64(segment)))
		}
		fields = append(fields, boolField(row.returnsKnown))
		fields = append(fields, uintField(uint64(len(row.sourceKeys))))
		for _, key := range row.sourceKeys {
			fields = append(fields, uintField(uint64(key)))
		}
		fields = append(fields, uintField(uint64(len(row.canonicalKeys))))
		for _, key := range row.canonicalKeys {
			fields = append(fields, uintField(uint64(key)))
		}
		fields = append(fields, uintField(uint64(len(row.children))))
		for _, child := range row.children {
			fields = append(fields, bytesField(child))
		}
	}
	fields = append(fields, uintField(uint64(len(artifact.staticExpressions))))
	for _, row := range artifact.staticExpressions {
		fields = append(fields, bytesField(row.id), bytesField(row.reference), bytesField(row.owner))
	}
	fields = append(fields, uintField(uint64(len(artifact.staticInputs))))
	for _, row := range artifact.staticInputs {
		exact := row.literal
		fields = append(fields, bytesField(row.id), bytesField(row.owner), uintField(uint64(row.kind)), uintField(uint64(row.operandKind)), bytesField(row.expression), bytesField(row.source), bytesField(row.target), bytesField(row.operand), bytesField(row.frontier), bytesField(row.operandReference), bytesField(row.operandSubject), bytesField(row.operandBody), uintField(uint64(exact.Kind)), boolField(exact.Bool), uintField(uint64(exact.Integer)), uintField(exact.FloatBits), field{bytes: []byte(exact.String), kind: fieldBytes}, uintField(uint64(row.cursor)))
	}
	fields = append(fields, uintField(uint64(len(artifact.environment))))
	for _, edge := range artifact.environment {
		fields = append(fields,
			bytesField(edge.id), bytesField(edge.from), bytesField(edge.to), bytesField(edge.route),
			uintField(uint64(edge.arm)), bytesField(edge.guard), bytesField(edge.decision), bytesField(edge.condition), boolField(edge.guarded), boolField(edge.truth),
			bytesField(edge.component), bytesField(edge.mu), boolField(edge.hasMu),
			bytesField(edge.reset), boolField(edge.hasReset), uintField(uint64(len(edge.resets))),
		)
		for _, reset := range edge.resets {
			fields = append(fields, bytesField(reset))
		}
	}
	fields = append(fields, uintField(uint64(len(artifact.localTransfers))))
	for _, edge := range artifact.localTransfers {
		fields = append(fields, bytesField(edge.id), bytesField(edge.from), bytesField(edge.to), boolField(edge.full), uintField(uint64(len(edge.roles))))
		for _, role := range edge.roles {
			fields = append(fields, uintField(uint64(role)))
		}
	}
	for roleIndex := 0; roleIndex < MountedRuleRoleCount(); roleIndex++ {
		role, roleOK := MountedRuleRoleAt(roleIndex)
		if !roleOK {
			continue
		}
		rows := artifact.ruleOccurrences[role]
		fields = append(fields, uintField(uint64(role)), uintField(uint64(len(rows))))
		for _, row := range rows {
			fields = append(fields,
				uintField(uint64(row.occurrence)), bytesField(row.point), bytesField(row.input),
				uintField(uint64(row.stage)), uintField(uint64(row.inputKind)), bytesField(row.route),
			)
		}
	}
	fields = append(fields, uintField(uint64(len(artifact.regions))))
	for _, region := range artifact.regions {
		fields = append(fields,
			bytesField(region.id), bytesField(region.head), bytesField(region.sourceHead), bytesField(region.parent), boolField(region.cyclic),
			uintField(uint64(len(region.members))),
		)
		for _, member := range region.members {
			fields = append(fields, bytesField(member))
		}
	}
	fields = append(fields, uintField(uint64(len(artifact.events))))
	for _, event := range artifact.events {
		fields = append(fields, uintField(uint64(event.kind)), bytesField(event.region), bytesField(event.point))
	}
	return digest(artifactIDDomain, artifactFormat, fields...)
}

func radixRows[T any](rows []T, key func(T) keyspace.ContentID) {
	if len(rows) < 2 {
		return
	}
	scratch := make([]T, len(rows))
	source, target := rows, scratch
	for byteIndex := len(keyspace.ContentID{}) - 1; byteIndex >= 0; byteIndex-- {
		var offsets [256]int
		for _, row := range source {
			offsets[key(row)[byteIndex]]++
		}
		total := 0
		for index := range offsets {
			count := offsets[index]
			offsets[index] = total
			total += count
		}
		for _, row := range source {
			bucket := key(row)[byteIndex]
			target[offsets[bucket]] = row
			offsets[bucket]++
		}
		source, target = target, source
	}
	if len(source) != 0 && &source[0] != &rows[0] {
		copy(rows, source)
	}
}

func radixContentIDs(ids []keyspace.ContentID) {
	radixRows(ids, func(id keyspace.ContentID) keyspace.ContentID { return id })
}

func sortedPoints(rows []Point) bool {
	for index := 1; index < len(rows); index++ {
		if !contentIDBefore(rows[index-1].id, rows[index].id) {
			return false
		}
	}
	return true
}

func contentIDBefore(left, right keyspace.ContentID) bool {
	for index := range left {
		if left[index] != right[index] {
			return left[index] < right[index]
		}
	}
	return false
}
