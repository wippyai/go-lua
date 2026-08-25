package publish_test

import (
	"bytes"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"github.com/wippyai/go-lua/analysis/engine/relation/apply"
	"github.com/wippyai/go-lua/analysis/engine/relation/cofiber"
	"github.com/wippyai/go-lua/analysis/engine/relation/publish"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/bootstrap"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/database"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/geometry"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/store"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/transaction"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/check/certificate"
	"github.com/wippyai/go-lua/analysis/relation/mount/address"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	schemaalgebra "github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/plan"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/lineage"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

// The fixture deliberately has one relation, one row, and one output. It is
// enough to redeem every public publication outcome while keeping each law
// below a single solve step. No test constructs an aggregate, cell, or
// application by reaching into a private field.
type region struct{ id identity.ContentID }

func (value region) Identity() (identity.ContentID, bool) { return value.id, value.id.Available() }
func (value region) Conjoin(other witness.Region) (witness.Region, bool) {
	otherValue, ok := other.(region)
	if !ok || otherValue.id != value.id {
		return nil, false
	}
	return value, true
}
func (value region) Entails(other witness.Region) bool {
	otherValue, ok := other.(region)
	return ok && otherValue.id == value.id
}

type inventory struct {
	fence       address.Fence
	relation    model.RelationID
	column      model.ColumnID
	key         model.KeyID
	scope       model.ScopeID
	expression  model.ExpressionID
	dependency  model.DependencyID
	region      witness.Region
	denominator model.DenominatorRef
	row         model.RowID
	accesses    []arrangement.Access
}

func (value *inventory) Fence() address.Fence { return value.fence }
func (value *inventory) ResolveRelation(id model.RelationID) (uint64, bool) {
	return 1, id == value.relation
}
func (value *inventory) ResolveColumn(id model.ColumnID) (uint64, bool) {
	return 1, id == value.column
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
func (value *inventory) ScopeRegion(id model.ScopeID) (witness.Region, bool) {
	return value.region, id == value.scope
}
func (value *inventory) ResolveDenominator(ref model.DenominatorRef) (witness.DenominatorEvidence, bool) {
	if ref != value.denominator {
		return witness.DenominatorEvidence{}, false
	}
	return witness.NewDenominatorEvidence([]model.RowID{value.row}, content("denominator"))
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
	result   outcome.Result
	proposal binding.Proposal
}

func (value *worker) Evaluate(_ binding.Frame, buffer *binding.ProposalBuffer) outcome.Result {
	if value.result.Code == outcome.Produced && !buffer.Append(value.proposal) {
		return outcome.Result{}
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
	owner       model.OwnerID
	mounted     witness.Mounted
	view        geometry.Geometry
	door        publish.Door
	aggregate   databaseVersion
	manager     *guard.Manager
	operation   model.OperationID
	scope       witness.Scope
	denominator model.DenominatorRef
	row         model.RowID
	column      model.ColumnID
	typeID      model.TypeID
	value       binding.ValueToken
	proposal    binding.Proposal
	lineage     model.LineageRef
	permit      witness.WideningPermit
	token       binding.CellToken
	worker      *worker
	readScratch *store.ReadScratch
}

// databaseVersion keeps the fixture declaration short without exposing an
// alternate root type in production.
type databaseVersion = database.Version

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

func newFixtureAt(t *testing.T, mountByte byte) fixture {
	t.Helper()
	owner := issue(t, content("owner"), model.IssueOwnerID)
	schemaID := issue(t, content("schema"), func(id identity.ContentID) (model.SchemaID, bool) { return model.IssueSchemaID(owner, id) })
	relation := issue(t, content("relation"), func(id identity.ContentID) (model.RelationID, bool) { return model.IssueRelationID(owner, id) })
	column := issue(t, content("column"), func(id identity.ContentID) (model.ColumnID, bool) { return model.IssueColumnID(relation, id) })
	key := issue(t, content("key"), func(id identity.ContentID) (model.KeyID, bool) { return model.IssueKeyID(relation, id) })
	scopeID := issue(t, content("scope"), func(id identity.ContentID) (model.ScopeID, bool) { return model.IssueScopeID(owner, id) })
	typeID := issue(t, content("type"), func(id identity.ContentID) (model.TypeID, bool) { return model.IssueTypeID(owner, id) })
	expression := issue(t, content("expression"), func(id identity.ContentID) (model.ExpressionID, bool) { return model.IssueExpressionID(owner, id) })
	dependency := issue(t, content("dependency"), func(id identity.ContentID) (model.DependencyID, bool) { return model.IssueDependencyID(owner, id) })
	operation := issue(t, content("operation"), func(id identity.ContentID) (model.OperationID, bool) { return model.IssueOperationID(owner, id) })
	row := issue(t, content("row"), func(id identity.ContentID) (model.RowID, bool) { return model.IssueRowID(relation, id) })
	denominator, ok := model.NewDenominatorRef(relation, key)
	if !ok {
		t.Fatal("denominator")
	}
	cardinality, ok := model.NewCardinality(model.ExactlyOne, 0)
	if !ok {
		t.Fatal("cardinality")
	}
	outcomes, ok := outcome.NewSet(outcome.Produced, outcome.NoCandidate, outcome.NoSelection, outcome.Opaque, outcome.Refused)
	if !ok {
		t.Fatal("outcomes")
	}
	signatureValue, ok := signature.Seal(signature.Spec{
		Identity:  signature.Identity{Operation: operation, Version: 1},
		Fence:     signature.Fence{Owner: owner, Schema: schemaID},
		Outputs:   []signature.Output{{Relation: relation, Column: column, Type: typeID, Presence: signature.ProducePresent}},
		Authority: signature.OutputAuthority{Denominator: denominator}, Cardinality: cardinality, Outcomes: outcomes,
	})
	if !ok {
		t.Fatal("signature")
	}
	relationRef, ok := plan.NewRelationRef(relation)
	if !ok {
		t.Fatal("relation ref")
	}
	expressionRef := plan.DefineExpressionRef(expression, schemaalgebra.NewProject(
		schemaalgebra.NewInput(relation),
		schemaalgebra.NewProjectContract(relation, []schemaalgebra.ColumnMapping{schemaalgebra.NewColumnMapping(column, column)}, key),
	))
	dependencyRef := plan.DefineDependencyRef(dependency)
	dependencyValue := plan.DefineDependency(dependency, expression, []plan.RelationRef{relationRef}, []plan.RelationRef{relationRef}, "publish-law")
	edge := plan.DefineDependencyEdge(dependencyRef, dependencyRef)
	head := plan.DefineWideningHead(dependencyRef, relationRef)
	scc := plan.DefineSCC([]plan.DependencyRef{dependencyRef}, []plan.DependencyEdge{edge}, plan.DefineRecurrence(plan.Positive, []plan.WideningHead{head}))
	builder := plan.NewBuilder(schemaID)
	if !builder.AddRelation(model.DefineRelationSchema(relation, []model.ColumnID{column}, []model.KeyID{key}, scopeID)) ||
		!builder.AddColumn(model.DefineColumnSchema(column, typeID)) ||
		!builder.AddKey(model.DefineKeySchema(key, []model.ColumnID{column})) ||
		!builder.AddScope(model.DefineScopeSchema(scopeID, nil)) || !builder.AddExpression(expressionRef) || !builder.AddDependency(dependencyValue) || !builder.AddSCC(scc) || !builder.AddSignature(signatureValue) {
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
	logical := region{id: content("region")}
	inventory := &inventory{fence: fence, relation: relation, column: column, key: key, scope: scopeID, expression: expression, dependency: dependency, region: logical, denominator: denominator, row: row}
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
	scopes, ok := cofiber.New(mounted, manager, func(value witness.Region) (support.Mask, bool) {
		logicalValue, ok := value.(region)
		return full, ok && logicalValue.id == logical.id
	})
	if !ok {
		t.Fatal("cofiber")
	}
	view, ok := geometry.New(mounted, scopes)
	if !ok {
		t.Fatal("geometry")
	}
	base, ok := bootstrap.NewDatabase(mounted, view)
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
	value, ok := mounted.IssueValue(typeID, content("value"))
	if !ok {
		t.Fatal("value")
	}
	token, ok := mounted.IssueCell(denominator, scope, column, row)
	if !ok {
		t.Fatal("cell")
	}
	presence, ok := model.NewPresence(model.Present)
	if !ok {
		t.Fatal("presence")
	}
	proposal, ok := binding.NewProposal(token, value, presence)
	if !ok {
		t.Fatal("proposal")
	}
	workerValue.result = outcome.Result{Code: outcome.NoSelection}
	workerValue.proposal = proposal
	lineageValue, ok := model.IssueLineageRef(owner, content("lineage"))
	if !ok {
		t.Fatal("lineage")
	}
	return fixture{owner: owner, mounted: mounted, view: view, door: door, aggregate: base, manager: manager, operation: operation, scope: scope, denominator: denominator, row: row, column: column, typeID: typeID, value: value, proposal: proposal, lineage: lineageValue, permit: permit, token: token, worker: workerValue, readScratch: store.NewReadScratch(manager)}
}

func (value fixture) application(t *testing.T, code outcome.Code) apply.Application {
	t.Helper()
	value.worker.result = outcome.Result{Code: code}
	if code == outcome.Refused {
		value.worker.result.RefusalID = issue(t, content("refusal"), func(id identity.ContentID) (model.RefusalID, bool) { return model.IssueRefusalID(value.owner, id) })
	}
	application, ok := apply.Apply(value.mounted, value.operationIdentity(), value.scope)
	if !ok {
		t.Fatalf("apply %v", code)
	}
	return application
}

func (value fixture) operationIdentity() signature.Identity {
	for _, identityValue := range value.mounted.SignatureIdentities() {
		if identityValue.Operation == value.operation {
			return identityValue
		}
	}
	return signature.Identity{}
}

func (value fixture) sidecar() []transaction.Submission {
	if value.worker.result.Code == outcome.Refused || value.worker.result.Code != outcome.Produced {
		return nil
	}
	return []transaction.Submission{{Lineage: value.lineage}}
}

func TestPublishPreservesEverySemanticOutcome(t *testing.T) {
	value := newFixture(t)
	for _, code := range []outcome.Code{outcome.NoCandidate, outcome.NoSelection, outcome.Opaque, outcome.Refused} {
		application := value.application(t, code)
		settlement := value.door.Publish(value.aggregate, value.readScratch, application, nil, witness.WideningPermit{})
		if !settlement.Available() || settlement.Outcome().Code != code || !settlement.Base().Same(value.aggregate) || !settlement.Next().Same(value.aggregate) || settlement.Changed() {
			t.Fatalf("outcome %v was not an exact no-write settlement", code)
		}
	}
	value.worker.result = outcome.Result{Code: outcome.Produced}
	application, ok := apply.Apply(value.mounted, value.operationIdentity(), value.scope)
	if !ok {
		t.Fatal("produced application")
	}
	settlement := value.door.Publish(value.aggregate, value.readScratch, application, value.sidecar(), witness.WideningPermit{})
	if !settlement.Available() || settlement.Outcome().Code != outcome.Produced || !settlement.Changed() || !settlement.Next().SuccessorOf(value.aggregate) {
		t.Fatal("produced application did not commit an aggregate ascent")
	}
}

func TestPublishNoWriteDoesNotRequireReadScratch(t *testing.T) {
	value := newFixture(t)
	for _, code := range []outcome.Code{outcome.NoCandidate, outcome.NoSelection, outcome.Opaque, outcome.Refused} {
		application := value.application(t, code)
		settlement := value.door.Publish(value.aggregate, nil, application, nil, witness.WideningPermit{})
		if !settlement.Available() || settlement.Outcome().Code != code || !settlement.Base().Same(value.aggregate) || !settlement.Next().Same(value.aggregate) || settlement.Changed() {
			t.Fatalf("scratch-less outcome %v was not an exact no-write settlement", code)
		}
	}
}

func TestPublishRejectsMalformedSidecarsWithoutAWrite(t *testing.T) {
	value := newFixture(t)
	value.worker.result = outcome.Result{Code: outcome.Produced}
	application, ok := apply.Apply(value.mounted, value.operationIdentity(), value.scope)
	if !ok {
		t.Fatal("produced application")
	}
	// A produced application has one proposal; a missing sidecar is an
	// authenticated cardinality failure, not an empty or unknown publication.
	settlement := value.door.Publish(value.aggregate, value.readScratch, application, nil, witness.WideningPermit{})
	if settlement.Available() || value.aggregate.Revision() != 1 {
		t.Fatal("malformed sidecars crossed the publication door")
	}
}

func TestPublishRedeemsOnlyTheExactMountedWideningPermit(t *testing.T) {
	value := newFixture(t)
	value.worker.result = outcome.Result{Code: outcome.Produced}
	firstApplication, ok := apply.Apply(value.mounted, value.operationIdentity(), value.scope)
	if !ok {
		t.Fatal("first application")
	}
	first := value.door.Publish(value.aggregate, value.readScratch, firstApplication, value.sidecar(), witness.WideningPermit{})
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
	value.worker.proposal = secondProposal
	secondApplication, ok := apply.Apply(value.mounted, value.operationIdentity(), value.scope)
	if !ok {
		t.Fatal("second application")
	}
	second := value.door.Publish(first.Next(), value.readScratch, secondApplication, value.sidecar(), value.permit)
	if !second.Available() || !second.Changed() || !second.Next().SuccessorOf(first.Next()) {
		t.Fatal("exact mounted permit did not authorize widening")
	}

	foreign := newFixtureAt(t, 0x72)
	foreignApplication := foreign.application(t, outcome.Produced)
	foreignSettlement := value.door.Publish(second.Next(), value.readScratch, foreignApplication, foreign.sidecar(), foreign.permit)
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
	if settlement := value.door.Publish(database.Version{}, value.readScratch, application, nil, witness.WideningPermit{}); settlement.Available() {
		t.Fatal("unsealed aggregate crossed the publication door")
	}
	if foreign, ok := publish.New(value.mounted, geometry.Geometry{}); ok || foreign.Available() {
		t.Fatal("foreign geometry accepted")
	}
	other := newFixtureAt(t, 0x72)
	if settlement := value.door.Publish(other.aggregate, value.readScratch, application, nil, witness.WideningPermit{}); settlement.Available() {
		t.Fatal("foreign aggregate root crossed the mounted door")
	}
}
