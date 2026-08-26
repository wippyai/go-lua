package publish_test

import (
	"bytes"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"github.com/wippyai/go-lua/analysis/engine/relation/apply"
	applycontribution "github.com/wippyai/go-lua/analysis/engine/relation/apply/contribution"
	"github.com/wippyai/go-lua/analysis/engine/relation/cofiber"
	"github.com/wippyai/go-lua/analysis/engine/relation/publish"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/database"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/geometry"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/store"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/transaction"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/check/certificate"
	"github.com/wippyai/go-lua/analysis/relation/mount/address"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement/expand"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	schemaalgebra "github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/plan"
	schemaregion "github.com/wippyai/go-lua/analysis/relation/schema/region"
	"github.com/wippyai/go-lua/analysis/relation/schema/semantic/output"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/lineage"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

type inventory struct {
	fence         address.Fence
	relation      model.RelationID
	keyColumn     model.ColumnID
	column        model.ColumnID
	payloadColumn model.ColumnID
	key           model.KeyID
	scope         model.ScopeID
	expression    model.ExpressionID
	dependency    model.DependencyID
	denominator   model.DenominatorRef
	row           model.RowID
	rows          []model.RowID
	accesses      []arrangement.Access
}

func (value *inventory) Fence() address.Fence { return value.fence }
func (value *inventory) ResolveRelation(id model.RelationID) (uint64, bool) {
	return 1, id == value.relation
}
func (value *inventory) ResolveColumn(id model.ColumnID) (uint64, bool) {
	if id == value.keyColumn {
		return 1, true
	}
	if id == value.column {
		return 2, true
	}
	if id == value.payloadColumn {
		return 3, true
	}
	return 0, false
}
func (value *inventory) ResolveKey(id model.KeyID) (uint64, bool) {
	return 1, id == value.key
}
func (value *inventory) ResolveScope(id model.ScopeID) (uint64, bool) {
	return 1, id == value.scope
}
func (value *inventory) ResolveExpression(id model.ExpressionID) (uint64, bool) {
	return 1, id == value.expression
}
func (value *inventory) ResolveDependency(id model.DependencyID) (uint64, bool) {
	return 1, id == value.dependency
}
func (value *inventory) Resolve(access arrangement.Access) (arrangement.Handle, bool) {
	for index, prior := range value.accesses {
		if prior.Equal(access) {
			return arrangement.NewHandle(value.fence, uint64(index+1))
		}
	}
	value.accesses = append(value.accesses, access)
	return arrangement.NewHandle(value.fence, uint64(len(value.accesses)))
}
func (value *inventory) ResolveDenominator(ref model.DenominatorRef) (witness.DenominatorEvidence, bool) {
	if ref != value.denominator {
		return witness.DenominatorEvidence{}, false
	}
	rows := value.rows
	if rows == nil {
		rows = []model.RowID{value.row}
	}
	return witness.NewDenominatorEvidence(rows, content("denominator"))
}
func (value *inventory) ResolveExpand(model.ExpandContract) ([]expand.Vector, bool) {
	return nil, false
}

type lawAlgebra struct{ typeID model.TypeID }

func (value lawAlgebra) Type() model.TypeID { return value.typeID }
func (value lawAlgebra) Join(left, right binding.ValueToken) (binding.ValueToken, bool) {
	if !left.Available() || !right.Available() || left.Type() != value.typeID || right.Type() != value.typeID {
		return binding.ValueToken{}, false
	}
	leftOpaque, rightOpaque := left.Opaque(), right.Opaque()
	if bytes.Compare(leftOpaque[:], rightOpaque[:]) >= 0 {
		return left, true
	}
	return right, true
}
func (value lawAlgebra) Widen(left, right binding.ValueToken) (binding.ValueToken, bool) {
	if !left.Available() || !right.Available() || left.Type() != value.typeID || right.Type() != value.typeID {
		return binding.ValueToken{}, false
	}
	return right, true
}
func (value lawAlgebra) LessOrEqual(left, right binding.ValueToken) bool {
	if !left.Available() || !right.Available() || left.Type() != value.typeID || right.Type() != value.typeID {
		return false
	}
	leftOpaque, rightOpaque := left.Opaque(), right.Opaque()
	return bytes.Compare(leftOpaque[:], rightOpaque[:]) <= 0
}

type algebraRegistry struct{ value lawAlgebra }

func (value algebraRegistry) Resolve(typeID model.TypeID) (binding.ValueAlgebra, bool) {
	return value.value, value.value.Type() == typeID
}

type worker struct {
	result    outcome.Result
	proposal  binding.Proposal
	proposals []binding.Proposal
	refusal   model.RefusalID
	buffer    *binding.ProposalBuffer
	// emitOpaque lets the focused Opaque publication law supply a row while
	// preserving the existing no-row terminal-outcome fixtures.
	emitOpaque bool
}

func (value *worker) Evaluate(_ binding.Frame, buffer *binding.ProposalBuffer) outcome.Result {
	value.buffer = buffer
	if !value.result.Code.Publishes() || (value.result.Code == outcome.Opaque && !value.emitOpaque) {
		return value.result
	}
	proposals := value.proposals
	if len(proposals) <= 1 {
		proposals = []binding.Proposal{value.proposal}
	}
	for _, proposal := range proposals {
		if !buffer.Append(proposal) {
			if value.refusal.Available() {
				return outcome.Result{Code: outcome.Refused, RefusalID: value.refusal}
			}
			return outcome.Result{}
		}
	}
	return value.result
}

type bindingFactory struct {
	operation signature.Signature
	worker    *worker
}

func (value bindingFactory) Bind(operation signature.Signature) (binding.Binding, bool) {
	if !operation.Available() || operation.Digest() != value.operation.Digest() {
		return nil, false
	}
	return operationBinding{operation: value.operation, worker: value.worker}, true
}

type operationBinding struct {
	operation signature.Signature
	worker    *worker
}

func (value operationBinding) Signature() signature.Signature { return value.operation }
func (value operationBinding) NewWorker(binding.Fence) (binding.Worker, bool) {
	return value.worker, value.worker != nil
}

type fixture struct {
	owner         model.OwnerID
	mounted       witness.Mounted
	view          geometry.Geometry
	door          publish.Door
	aggregate     database.Version
	manager       *guard.Manager
	operation     model.OperationID
	scope         witness.Scope
	denominator   model.DenominatorRef
	row           model.RowID
	rows          []model.RowID
	keyColumn     model.ColumnID
	column        model.ColumnID
	payloadColumn model.ColumnID
	typeID        model.TypeID
	keyValue      binding.ValueToken
	value         binding.ValueToken
	keyToken      binding.CellToken
	proposal      binding.Proposal
	proposals     []binding.Proposal
	lineage       model.LineageRef
	permit        witness.WideningPermit
	token         binding.CellToken
	tokens        []binding.CellToken
	payloadToken  binding.CellToken
	worker        *worker
	readScratch   *store.ReadScratch
	contribution  output.ContributionSpec
}

func content(label string) identity.ContentID {
	value, _ := identity.DeriveContentID("analysis/engine/relation/publish/law/v1", []byte(label))
	return value
}

func issue[T any](t testing.TB, value identity.ContentID, issue func(identity.ContentID) (T, bool)) T {
	t.Helper()
	result, ok := issue(value)
	if !ok {
		t.Fatalf("issue %s", value)
	}
	return result
}

func newFixture(t *testing.T) fixture { return newFixtureAt(t, 0x71) }

func newContributionFixture(t *testing.T) fixture {
	cardinality, ok := model.NewCardinality(model.ExactlyOne, 0)
	if !ok {
		t.Fatal("cardinality")
	}
	return newFixtureWithContribution(t, 0x75, 1, cardinality, true)
}

func newFixtureAt(t *testing.T, mountByte byte) fixture {
	cardinality, ok := model.NewCardinality(model.ExactlyOne, 0)
	if !ok {
		t.Fatal("cardinality")
	}
	return newFixtureWithRows(t, mountByte, 1, cardinality)
}

func newMultiFixture(t *testing.T) fixture {
	cardinality, ok := model.NewCardinality(model.BoundedMany, 2)
	if !ok {
		t.Fatal("bounded cardinality")
	}
	return newFixtureWithRows(t, 0x73, 2, cardinality)
}

func newBoundedOverflowFixture(t *testing.T) fixture {
	cardinality, ok := model.NewCardinality(model.BoundedMany, 2)
	if !ok {
		t.Fatal("bounded cardinality")
	}
	return newFixtureWithRows(t, 0x74, 3, cardinality)
}

func newFixtureWithRows(t *testing.T, mountByte byte, rowCount int, cardinality model.Cardinality) fixture {
	return newFixtureWithContribution(t, mountByte, rowCount, cardinality, false)
}

func newFixtureWithContribution(t *testing.T, mountByte byte, rowCount int, cardinality model.Cardinality, contributionEnabled bool) fixture {
	return newFixtureWithOutputPresence(t, mountByte, rowCount, cardinality, contributionEnabled, signature.ProducePresent)
}

func newOpaqueFixture(t *testing.T) fixture {
	cardinality, ok := model.NewCardinality(model.ExactlyOne, 0)
	if !ok {
		t.Fatal("opaque cardinality")
	}
	return newFixtureWithOutputPresence(t, 0x78, 1, cardinality, false, signature.ProduceOpaque)
}

func newFixtureWithOutputPresence(t *testing.T, mountByte byte, rowCount int, cardinality model.Cardinality, contributionEnabled bool, outputPresence signature.PresenceContract) fixture {
	t.Helper()
	if rowCount <= 0 || !cardinality.Available() {
		t.Fatal("fixture shape")
	}
	owner := issue(t, content("owner"), model.IssueOwnerID)
	schemaID := issue(t, content("schema"), func(id identity.ContentID) (model.SchemaID, bool) { return model.IssueSchemaID(owner, id) })
	relation := issue(t, content("relation"), func(id identity.ContentID) (model.RelationID, bool) { return model.IssueRelationID(owner, id) })
	keyColumn := issue(t, content("key-column"), func(id identity.ContentID) (model.ColumnID, bool) { return model.IssueColumnID(relation, id) })
	column := issue(t, content("column"), func(id identity.ContentID) (model.ColumnID, bool) { return model.IssueColumnID(relation, id) })
	var payloadColumn model.ColumnID
	if contributionEnabled {
		payloadColumn = issue(t, content("payload-column"), func(id identity.ContentID) (model.ColumnID, bool) { return model.IssueColumnID(relation, id) })
	}
	key := issue(t, content("key"), func(id identity.ContentID) (model.KeyID, bool) { return model.IssueKeyID(relation, id) })
	scopeID := issue(t, content("scope"), func(id identity.ContentID) (model.ScopeID, bool) { return model.IssueScopeID(owner, id) })
	typeID := issue(t, content("type"), func(id identity.ContentID) (model.TypeID, bool) { return model.IssueTypeID(owner, id) })
	expression := issue(t, content("expression"), func(id identity.ContentID) (model.ExpressionID, bool) { return model.IssueExpressionID(owner, id) })
	dependency := issue(t, content("dependency"), func(id identity.ContentID) (model.DependencyID, bool) { return model.IssueDependencyID(owner, id) })
	operation := issue(t, content("operation"), func(id identity.ContentID) (model.OperationID, bool) { return model.IssueOperationID(owner, id) })
	rows := make([]model.RowID, rowCount)
	for index := range rows {
		rows[index] = issue(t, content("row/"+string(rune('a'+index))), func(id identity.ContentID) (model.RowID, bool) { return model.IssueRowID(relation, id) })
	}
	row := rows[0]
	denominator, ok := model.NewDenominatorRef(relation, key)
	if !ok {
		t.Fatal("denominator")
	}
	outcomes, ok := outcome.NewSet(outcome.Produced, outcome.NoCandidate, outcome.NoSelection, outcome.Opaque, outcome.Refused)
	if !ok {
		t.Fatal("outcomes")
	}
	delivery, ok := signature.NewScalarDelivery()
	if !ok {
		t.Fatal("scalar delivery")
	}
	outputs := []signature.Output{
		{Relation: relation, Column: keyColumn, Type: typeID, Presence: outputPresence, Denominator: denominator},
		{Relation: relation, Column: column, Type: typeID, Presence: outputPresence, Denominator: denominator},
	}
	if contributionEnabled {
		outputs = append(outputs, signature.Output{Relation: relation, Column: payloadColumn, Type: typeID, Presence: outputPresence, Denominator: denominator})
	}
	signatureValue, ok := signature.Seal(signature.Spec{
		Identity:    signature.Identity{Operation: operation, Version: 1},
		Fence:       signature.Fence{Owner: owner, Schema: schemaID},
		Inputs:      []signature.Input{{Relation: relation, Column: column, Type: typeID, Presence: signature.RequirePresent, Delivery: delivery, Denominator: denominator}},
		Outputs:     outputs,
		Cardinality: cardinality, Outcomes: outcomes,
	})
	if !ok {
		t.Fatal("signature")
	}
	relationRef, ok := plan.NewRelationRef(relation)
	if !ok {
		t.Fatal("relation ref")
	}
	publishColumns := []model.ColumnID{keyColumn, column}
	if contributionEnabled {
		publishColumns = append(publishColumns, payloadColumn)
	}
	expressionRef := plan.DefineExpressionRef(expression, schemaalgebra.NewPublish(
		schemaalgebra.NewApply([]schemaalgebra.Expression{schemaalgebra.NewInput(relation)}, schemaalgebra.NewApplyContract(
			signatureValue.Identity(), []schemaalgebra.SlotSource{schemaalgebra.NewSlotSource(0, 1)}, schemaalgebra.OwnerNamed())),
		schemaalgebra.NewPublishContract(relation, key, publishColumns...),
	))
	dependencyRef := plan.DefineDependencyRef(dependency)
	dependencyValue := plan.DefineDependency(dependency, expression, []plan.RelationRef{relationRef}, []plan.RelationRef{relationRef}, "publish-law")
	edge := plan.DefineDependencyEdge(dependencyRef, dependencyRef)
	head := plan.DefineWideningHead(dependencyRef, relationRef)
	scc := plan.DefineSCC([]plan.DependencyRef{dependencyRef}, []plan.DependencyEdge{edge}, plan.DefineRecurrence(plan.Positive, []plan.WideningHead{head}))
	builder := plan.NewBuilder(schemaID)
	typeCapability, capabilityOK := model.NewAscendingCapability(typeID)
	if !capabilityOK || !builder.AddTypeCapability(typeCapability) {
		t.Fatal("type capability")
	}
	var contribution output.ContributionSpec
	if contributionEnabled {
		contribution, ok = output.Seal(output.Spec{
			Signature: signatureValue,
			Port:      output.OutputPort{Operation: signatureValue.Identity(), Column: column},
			ValueType: typeID,
			Algebra:   typeCapability,
			Reducer:   output.Contributions,
		})
		if !ok || !builder.AddContribution(contribution) {
			t.Fatal("contribution declaration")
		}
	}
	relationColumns := []model.ColumnID{keyColumn, column}
	if contributionEnabled {
		relationColumns = append(relationColumns, payloadColumn)
	}
	if !builder.AddRelation(model.DefineRelationSchema(relation, relationColumns, []model.KeyID{key}, scopeID)) ||
		!builder.AddColumn(model.DefineColumnSchema(keyColumn, typeID)) ||
		!builder.AddColumn(model.DefineColumnSchema(column, typeID)) {
		t.Fatal("schema declarations")
	}
	if contributionEnabled && !builder.AddColumn(model.DefineColumnSchema(payloadColumn, typeID)) {
		t.Fatal("payload column declaration")
	}
	if !builder.AddKey(model.DefineKeySchema(key, []model.ColumnID{keyColumn})) ||
		!builder.AddScope(model.DefineScopeSchema(scopeID, nil, schemaregion.True())) || !builder.AddExpression(expressionRef) || !builder.AddDependency(dependencyValue) || !builder.AddSCC(scc) || !builder.AddSignature(signatureValue) {
		t.Fatal("schema declarations")
	}
	schema, ok := builder.Build()
	if !ok {
		t.Fatal("schema")
	}
	certificateValue, refusal := certificate.Check(schema)
	if refusal != nil || !certificateValue.Available() {
		t.Fatalf("certificate: %v", refusal)
	}
	storeID, ok := identity.IssueStore()
	if !ok {
		t.Fatal("store")
	}
	fence, ok := address.NewFence(schemaID, certificateValue.Digest(), storeID, identity.MountID{mountByte}, identity.Generation(1))
	if !ok {
		t.Fatal("fence")
	}
	inventory := &inventory{fence: fence, relation: relation, keyColumn: keyColumn, column: column, payloadColumn: payloadColumn, key: key, scope: scopeID, expression: expression, dependency: dependency, denominator: denominator, row: row, rows: rows}
	lineageOwner := issue(t, content("lineage-owner"), model.IssueOwnerID)
	lineageFactory, ok := lineage.NewFactory(lineageOwner)
	if !ok {
		t.Fatal("lineage factory")
	}
	workerValue := &worker{}
	mounted, ok := witness.Specialize(certificateValue, inventory, bindingFactory{operation: signatureValue, worker: workerValue}, algebraRegistry{value: lawAlgebra{typeID: typeID}}, lineageFactory)
	if !ok || !mounted.Available() {
		t.Fatal("mounted")
	}
	scope, ok := mounted.Scope(scopeID)
	if !ok {
		t.Fatal("scope")
	}
	manager, err := guard.New([]guard.Atom{1})
	if err != nil {
		t.Fatal(err)
	}
	full, ok := support.True(manager)
	if !ok {
		t.Fatal("full mask")
	}
	scopes, ok := cofiber.New(mounted, manager, func(value schemaregion.Region) (support.Mask, bool) {
		return full, value.IsTrue()
	})
	if !ok {
		t.Fatal("cofiber")
	}
	view, ok := geometry.New(mounted, scopes)
	if !ok {
		t.Fatal("geometry")
	}
	base, ok := database.Bootstrap(mounted, view)
	if !ok || !base.Available() {
		t.Fatal("aggregate")
	}
	door, ok := publish.New(mounted, view)
	if !ok {
		t.Fatal("door")
	}
	permit, ok := mounted.Widening(dependency, relation)
	if !ok || !permit.Available() {
		t.Fatal("widening permit")
	}
	keyValue, ok := mounted.IssueValue(typeID, content("key-value"))
	if !ok {
		t.Fatal("key value")
	}
	value, ok := mounted.IssueValue(typeID, content("value"))
	if !ok {
		t.Fatal("value")
	}
	var payloadValue binding.ValueToken
	if contributionEnabled {
		payloadValue, ok = mounted.IssueValue(typeID, content("payload-value"))
		if !ok {
			t.Fatal("payload value")
		}
	}
	presence, ok := model.NewPresence(model.Present)
	if !ok {
		t.Fatal("presence")
	}
	denominatorWitness, ok := mounted.Denominator(denominator)
	if !ok {
		t.Fatal("denominator witness")
	}
	tokens := make([]binding.CellToken, len(rows))
	keyTokens := make([]binding.CellToken, len(rows))
	payloadTokens := make([]binding.CellToken, len(rows))
	proposalCapacity := len(rows) * 2
	if contributionEnabled {
		proposalCapacity = len(rows) * 3
	}
	proposals := make([]binding.Proposal, 0, proposalCapacity)
	for index, rowID := range rows {
		keyToken, keyTokenOK := mounted.IssueCell(denominatorWitness, scope, keyColumn, rowID)
		if !keyTokenOK {
			t.Fatal("key cell")
		}
		keyProposal, keyProposalOK := binding.NewProposal(keyToken, keyValue, presence)
		if !keyProposalOK {
			t.Fatal("key proposal")
		}
		token, tokenOK := mounted.IssueCell(denominatorWitness, scope, column, rowID)
		if !tokenOK {
			t.Fatal("cell")
		}
		proposalValue, proposalOK := binding.NewProposal(token, value, presence)
		if !proposalOK {
			t.Fatal("proposal")
		}
		keyTokens[index], tokens[index] = keyToken, token
		proposals = append(proposals, keyProposal, proposalValue)
		if contributionEnabled {
			payloadToken, payloadTokenOK := mounted.IssueCell(denominatorWitness, scope, payloadColumn, rowID)
			if !payloadTokenOK {
				t.Fatal("payload cell")
			}
			payloadProposal, payloadProposalOK := binding.NewProposal(payloadToken, payloadValue, presence)
			if !payloadProposalOK {
				t.Fatal("payload proposal")
			}
			payloadTokens[index] = payloadToken
			proposals = append(proposals, payloadProposal)
		}
	}
	proposal := proposals[1]
	workerValue.result = outcome.Result{Code: outcome.NoSelection}
	workerValue.proposal = proposal
	workerValue.proposals = proposals
	workerValue.refusal = issue(t, content("refusal"), func(id identity.ContentID) (model.RefusalID, bool) { return model.IssueRefusalID(owner, id) })
	lineageValue, ok := mounted.DenominatorLineage(denominator)
	if !ok {
		t.Fatal("denominator lineage")
	}
	return fixture{owner: owner, mounted: mounted, view: view, door: door, aggregate: base, manager: manager, operation: operation, scope: scope, denominator: denominator, row: row, rows: rows, keyColumn: keyColumn, column: column, payloadColumn: payloadColumn, typeID: typeID, keyValue: keyValue, value: value, keyToken: keyTokens[0], proposal: proposal, proposals: proposals, lineage: lineageValue, permit: permit, token: tokens[0], tokens: tokens, payloadToken: payloadTokens[0], worker: workerValue, readScratch: store.NewReadScratch(manager), contribution: contribution}
}

func (value fixture) application(t *testing.T, code outcome.Code) apply.Application {
	t.Helper()
	value.worker.result = outcome.Result{Code: code}
	if code == outcome.Refused {
		value.worker.result.RefusalID = value.worker.refusal
	}
	application, ok := apply.Apply(value.mounted, value.operationIdentity(), value.scope, value.lineage, binding.NewOwnerNamedDestination(value.row.Relation()), value.inputSlot(t))
	if !ok {
		t.Fatalf("apply %v", code)
	}
	return application
}

func (value fixture) inputSlot(t *testing.T) binding.Slot {
	return value.inputSlotAt(t, 0)
}

func (value fixture) inputSlotAt(t *testing.T, index int) binding.Slot {
	t.Helper()
	if index < 0 || index >= len(value.tokens) {
		t.Fatal("input row")
	}
	presence, ok := model.NewPresence(model.Present)
	if !ok {
		t.Fatal("input presence")
	}
	cell, ok := binding.NewCell(value.tokens[index], value.typeID, value.value, presence)
	if !ok {
		t.Fatal("input cell")
	}
	slot, ok := binding.NewScalarSlot(cell)
	if !ok {
		t.Fatal("input slot")
	}
	return slot
}

func (value fixture) operationIdentity() signature.Identity {
	for _, identityValue := range value.mounted.SignatureIdentities() {
		if identityValue.Operation == value.operation {
			return identityValue
		}
	}
	return signature.Identity{}
}

func TestPublishPreservesEverySemanticOutcome(t *testing.T) {
	value := newFixture(t)
	entries := value.mounted.Arrangement().Execution().Schedules()
	if len(entries) != 1 || !entries[0].WideningFor(value.permit.Relation()) {
		t.Fatal("fixture widening schedule")
	}
	for _, code := range []outcome.Code{outcome.NoCandidate, outcome.NoSelection, outcome.Opaque, outcome.Refused} {
		application := value.application(t, code)
		// A widening capability is deliberately supplied at the boundary.  It
		// is ignored for every application without a proposal batch; no-write
		// semantics do not depend on recurrence admission.
		settlement := value.door.Publish(value.aggregate, value.readScratch, application, value.permit)
		if !settlement.Available() || settlement.Outcome().Code != code || !settlement.Base().Same(value.aggregate) || !settlement.Next().Same(value.aggregate) || settlement.Changed() {
			t.Fatalf("outcome %v was not an exact no-write settlement", code)
		}
		if permit, ok := value.door.WideningFor(entries[0], value.permit.Relation(), application); !ok || permit.Available() {
			t.Fatalf("outcome %v redeemed widening without a proposal", code)
		}
	}
	value.worker.result = outcome.Result{Code: outcome.Produced}
	application, ok := apply.Apply(value.mounted, value.operationIdentity(), value.scope, value.lineage, binding.NewOwnerNamedDestination(value.row.Relation()), value.inputSlot(t))
	if !ok {
		t.Fatal("produced application")
	}
	permit, ok := value.door.WideningFor(entries[0], value.permit.Relation(), application)
	if !ok || !permit.Available() || permit != value.permit {
		t.Fatal("write-bearing application did not redeem the exact mounted permit")
	}
	// The first publication is a seed, so it is an ordinary join.  The
	// schedule's exact permit is still proven above at the write-demand
	// boundary and is consumed only on a later recurrence ascent.
	settlement := value.door.Publish(value.aggregate, value.readScratch, application, witness.WideningPermit{})
	if !settlement.Available() || settlement.Outcome().Code != outcome.Produced || !settlement.Changed() || !settlement.Next().SuccessorOf(value.aggregate) {
		t.Fatal("produced application did not commit an aggregate ascent")
	}
}

func TestPublishNoWriteDoesNotRequireReadScratch(t *testing.T) {
	value := newFixture(t)
	for _, code := range []outcome.Code{outcome.NoCandidate, outcome.NoSelection, outcome.Opaque, outcome.Refused} {
		application := value.application(t, code)
		settlement := value.door.Publish(value.aggregate, nil, application, witness.WideningPermit{})
		if !settlement.Available() || settlement.Outcome().Code != code || !settlement.Base().Same(value.aggregate) || !settlement.Next().Same(value.aggregate) || settlement.Changed() {
			t.Fatalf("scratch-less outcome %v was not an exact no-write settlement", code)
		}
	}
}

func TestPublishRedeemsApplicationOwnedLineage(t *testing.T) {
	value := newFixture(t)
	value.worker.result = outcome.Result{Code: outcome.Produced}
	application, ok := apply.Apply(value.mounted, value.operationIdentity(), value.scope, value.lineage, binding.NewOwnerNamedDestination(value.row.Relation()), value.inputSlot(t))
	if !ok {
		t.Fatal("produced application")
	}
	// The application owns its provenance. There is no caller-side sidecar
	// parameter that can be omitted or replaced with an unrelated lineage.
	settlement := value.door.Publish(value.aggregate, value.readScratch, application, witness.WideningPermit{})
	if !settlement.Available() || !settlement.Changed() {
		t.Fatal("application-owned lineage did not cross the publication door")
	}
}

func TestPublishRoutesDeclaredContributionInOneAtomicCommit(t *testing.T) {
	value := newContributionFixture(t)
	application := value.application(t, outcome.Produced)
	settlement := value.door.Publish(value.aggregate, value.readScratch, application, witness.WideningPermit{})
	if !settlement.Available() || !settlement.Changed() || !settlement.Next().SuccessorOf(value.aggregate) {
		t.Fatal("declared contribution did not commit through the publication door")
	}
	if settlement.Next().Revision() != value.aggregate.Revision()+1 {
		t.Fatal("declared contribution used more than one database publication")
	}
	if settlement.Next().ContributionState().Same(value.aggregate.ContributionState()) || settlement.Next().ContributionState().Len() != 1 {
		t.Fatal("declared contribution did not advance its producer root")
	}
	delta, ok := settlement.Delta()
	if !ok || len(delta.AffectedContributionTargets()) != 1 {
		t.Fatal("atomic publication lost its exact contribution target")
	}
	target := delta.AffectedContributionTargets()[0]
	if target.Port != value.contribution.Port() || target.Destination != value.row {
		t.Fatal("contribution target was not derived from the sealed output port")
	}
	if !delta.SemanticChanged() || !delta.LineageChanged() {
		t.Fatal("derived aggregate was not committed with the contribution root")
	}
}

func TestSubmissionBatchRefusesDeclaredProposalWithoutTransition(t *testing.T) {
	value := newContributionFixture(t)
	application := value.application(t, outcome.Produced)
	// The public constructor accepts only the application-owned proposal lease;
	// an empty transition set is not allowed to demote a declared contribution
	// proposal into ordinary aggregate admission.
	batch, ok := transaction.NewSubmissionBatch(application, witness.WideningPermit{}, nil)
	if !ok || !batch.Available() {
		t.Fatal("application-owned batch construction")
	}
	prepared, accepted := transaction.Prepare(value.aggregate, value.view, value.readScratch, batch)
	if accepted || prepared.Available() {
		t.Fatal("declared contribution without transition changed the transaction")
	}
}

func TestSubmissionBatchRefusesTransitionFromOtherInvocation(t *testing.T) {
	cardinality, ok := model.NewCardinality(model.ExactlyOne, 0)
	if !ok {
		t.Fatal("cardinality")
	}
	value := newFixtureWithContribution(t, 0x77, 2, cardinality, true)
	value.worker.proposals = value.proposals[:3]
	applicationA := value.application(t, outcome.Produced)
	value.worker.proposals = value.proposals[3:]
	applicationB, ok := apply.Apply(value.mounted, value.operationIdentity(), value.scope, value.lineage, binding.NewOwnerNamedDestination(value.row.Relation()), value.inputSlotAt(t, 1))
	if !ok {
		t.Fatal("second application")
	}
	transitionsA, ok := applycontribution.TransitionsForApplication(value.mounted, applicationA)
	if !ok || len(transitionsA) == 0 {
		t.Fatal("application A transitions")
	}
	// Same operation, runtime fence, destination schema, and payload are not
	// sufficient: the transition's invocation address must be B's exact
	// address, not merely a compatible sibling invocation.
	if batch, accepted := transaction.NewSubmissionBatch(applicationB, witness.WideningPermit{}, transitionsA); accepted || batch.Available() {
		t.Fatal("transition from invocation A paired with B proposal")
	}
}

func TestPublishLeavesUndeclaredOutputOnTheOrdinaryPath(t *testing.T) {
	value := newFixture(t)
	application := value.application(t, outcome.Produced)
	settlement := value.door.Publish(value.aggregate, value.readScratch, application, witness.WideningPermit{})
	if !settlement.Available() || !settlement.Changed() || !settlement.Next().SuccessorOf(value.aggregate) {
		t.Fatal("ordinary output did not retain its ordinary publication path")
	}
	if !settlement.Next().ContributionState().Same(value.aggregate.ContributionState()) {
		t.Fatal("undeclared output was reclassified as a contribution")
	}
	delta, ok := settlement.Delta()
	if !ok || len(delta.AffectedContributionTargets()) != 0 {
		t.Fatal("ordinary output produced contribution transport")
	}
}

func TestPublishOrdinaryRemovalBypassesContributionTransport(t *testing.T) {
	value := newContributionFixture(t)
	seed := value.application(t, outcome.Produced)
	first := value.door.Publish(value.aggregate, value.readScratch, seed, witness.WideningPermit{})
	if !first.Available() || !first.Changed() || !first.Next().SuccessorOf(value.aggregate) {
		t.Fatal("seed contribution did not commit")
	}
	if first.Next().ContributionState().Len() != 1 {
		t.Fatal("seed contribution state was not established")
	}

	// The payload output port is not declared as a contribution. Its sparse
	// removal must therefore remain an ordinary proposal, even on a mount
	// that has a contribution declaration for a different output port.
	removal, ok := binding.NewRemovalProposal(value.payloadToken)
	if !ok {
		t.Fatal("ordinary removal proposal")
	}
	value.worker.result = outcome.Result{Code: outcome.Produced}
	value.worker.proposal = removal
	value.worker.proposals = nil
	application := value.application(t, outcome.Produced)
	second := value.door.Publish(first.Next(), value.readScratch, application, witness.WideningPermit{})
	successor := second.Available() && second.Next().SuccessorOf(first.Next())
	if !second.Available() || !second.Changed() || !successor {
		t.Fatal("ordinary removal did not commit")
	}
	if !second.Next().ContributionState().Same(first.Next().ContributionState()) {
		t.Fatal("ordinary removal changed contribution state")
	}
	delta, ok := second.Delta()
	if !ok || len(delta.AffectedContributionTargets()) != 0 {
		t.Fatal("ordinary removal crossed contribution transport")
	}
	semanticColumns := delta.SemanticColumnIDs()
	removedOrdinaryColumn := false
	for _, column := range semanticColumns {
		if column == value.payloadColumn {
			removedOrdinaryColumn = true
			break
		}
	}
	if !removedOrdinaryColumn {
		t.Fatal("ordinary removal did not change its exact payload column")
	}
}

func TestPublishRefusesUnsignedOrForeignContributionTransport(t *testing.T) {
	value := newContributionFixture(t)
	removal, ok := binding.NewRemovalProposal(value.token)
	if !ok {
		t.Fatal("removal proposal")
	}
	value.worker.result = outcome.Result{Code: outcome.Produced}
	value.worker.proposals[1] = removal
	unsigned, ok := apply.Apply(value.mounted, value.operationIdentity(), value.scope, value.lineage, binding.NewOwnerNamedDestination(value.row.Relation()), value.inputSlot(t))
	if !ok {
		t.Fatal("unsigned contribution application")
	}
	if settlement := value.door.Publish(value.aggregate, value.readScratch, unsigned, witness.WideningPermit{}); settlement.Available() {
		t.Fatal("removal crossed positive contribution transport without signed before side")
	}

	cardinality, cardinalityOK := model.NewCardinality(model.ExactlyOne, 0)
	if !cardinalityOK {
		t.Fatal("foreign cardinality")
	}
	foreign := newFixtureWithContribution(t, 0x76, 1, cardinality, true)
	foreignApplication := foreign.application(t, outcome.Produced)
	if settlement := value.door.Publish(value.aggregate, value.readScratch, foreignApplication, witness.WideningPermit{}); settlement.Available() {
		t.Fatal("foreign contribution application crossed the mounted publication door")
	}
}

func TestPublishRedeemsOnlyTheExactMountedWideningPermit(t *testing.T) {
	value := newFixture(t)
	value.worker.result = outcome.Result{Code: outcome.Produced}
	firstApplication, ok := apply.Apply(value.mounted, value.operationIdentity(), value.scope, value.lineage, binding.NewOwnerNamedDestination(value.row.Relation()), value.inputSlot(t))
	if !ok {
		t.Fatal("first application")
	}
	first := value.door.Publish(value.aggregate, value.readScratch, firstApplication, witness.WideningPermit{})
	if !first.Available() || !first.Changed() {
		t.Fatal("ordinary seed did not commit")
	}
	secondValue := binding.ValueToken{}
	for index := 0; index < 16; index++ {
		candidate, candidateOK := value.mounted.IssueValue(value.typeID, content("value-widened-"+string(rune('a'+index))))
		if !candidateOK {
			t.Fatal("second value")
		}
		leftOpaque, rightOpaque := value.value.Opaque(), candidate.Opaque()
		if bytes.Compare(leftOpaque[:], rightOpaque[:]) < 0 {
			secondValue = candidate
			break
		}
	}
	if !secondValue.Available() {
		t.Fatal("could not choose an ascending widening value")
	}
	secondProposal, ok := binding.NewProposal(value.token, secondValue, mustPresence(t))
	if !ok {
		t.Fatal("second proposal")
	}
	value.worker.proposals[1] = secondProposal
	secondApplication, ok := apply.Apply(value.mounted, value.operationIdentity(), value.scope, value.lineage, binding.NewOwnerNamedDestination(value.row.Relation()), value.inputSlot(t))
	if !ok {
		t.Fatal("second application")
	}
	second := value.door.Publish(first.Next(), value.readScratch, secondApplication, value.permit)
	if !second.Available() || !second.Changed() || !second.Next().SuccessorOf(first.Next()) {
		t.Fatal("exact mounted permit did not authorize widening")
	}

	foreign := newFixtureAt(t, 0x72)
	foreignApplication := foreign.application(t, outcome.Produced)
	foreignSettlement := value.door.Publish(second.Next(), value.readScratch, foreignApplication, foreign.permit)
	if foreignSettlement.Available() {
		t.Fatal("foreign mounted permit crossed the publication door")
	}
}

func mustPresence(t *testing.T) model.Presence {
	t.Helper()
	presence, ok := model.NewPresence(model.Present)
	if !ok {
		t.Fatal("presence")
	}
	return presence
}

func TestPublishRejectsForeignDoorAndUnsealedRoot(t *testing.T) {
	value := newFixture(t)
	application := value.application(t, outcome.NoSelection)
	if settlement := value.door.Publish(database.Version{}, value.readScratch, application, witness.WideningPermit{}); settlement.Available() {
		t.Fatal("unsealed aggregate crossed the publication door")
	}
	if foreign, ok := publish.New(value.mounted, geometry.Geometry{}); ok || foreign.Available() {
		t.Fatal("foreign geometry accepted")
	}
	other := newFixtureAt(t, 0x72)
	if settlement := value.door.Publish(other.aggregate, value.readScratch, application, witness.WideningPermit{}); settlement.Available() {
		t.Fatal("foreign aggregate root crossed the mounted door")
	}
	foreignApplication := other.application(t, outcome.NoSelection)
	if settlement := value.door.Publish(value.aggregate, nil, foreignApplication, witness.WideningPermit{}); settlement.Available() {
		t.Fatal("foreign no-write application crossed the mounted door")
	}
}

func TestPublishRejectsSameRuntimeForeignMountRoot(t *testing.T) {
	value := newFixture(t)
	cardinality, ok := model.NewCardinality(model.BoundedMany, 2)
	if !ok {
		t.Fatal("bounded cardinality")
	}
	// The schema, mount ID, and generation are unchanged, so the semantic
	// runtime fence is shared. The changed signature cardinality produces a
	// distinct certificate/mounted root and therefore a distinct mounted
	// digest. This is the exact sibling-root shape a fence-only check admits.
	foreign := newFixtureWithRows(t, 0x71, 2, cardinality)
	if !value.mounted.RuntimeFence().Same(foreign.mounted.RuntimeFence()) {
		t.Fatal("hostile fixture did not share runtime fence")
	}
	if value.mounted.Digest() == foreign.mounted.Digest() {
		t.Fatal("hostile fixture unexpectedly shared mounted digest")
	}
	if door, ok := publish.New(value.mounted, foreign.view); ok || door.Available() {
		t.Fatal("same-runtime foreign geometry crossed the publication door")
	}
	application := value.application(t, outcome.NoSelection)
	if settlement := value.door.Publish(foreign.aggregate, nil, application, witness.WideningPermit{}); settlement.Available() {
		t.Fatal("same-runtime foreign mounted root crossed publication door")
	}
}
