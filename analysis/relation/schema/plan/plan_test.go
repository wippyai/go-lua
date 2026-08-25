package plan

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

type fixture struct {
	owner       model.OwnerID
	schemaID    model.SchemaID
	relationA   model.RelationSchema
	relationB   model.RelationSchema
	columnA     model.ColumnSchema
	columnB     model.ColumnSchema
	keyA        model.KeySchema
	keyB        model.KeySchema
	scope       model.ScopeSchema
	refA        RelationRef
	refB        RelationRef
	expressionA ExpressionRef
	expressionB ExpressionRef
	dependencyA Dependency
	dependencyB Dependency
	scc         SCC
	signatureA  signature.Signature
	signatureB  signature.Signature
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	ownerToken := testToken(t, "owner")
	owner, ok := model.IssueOwnerID(ownerToken)
	if !ok {
		t.Fatal("owner identity unavailable")
	}

	relationAID, ok := model.IssueRelationID(owner, testToken(t, "relation-a"))
	if !ok {
		t.Fatal("relation A identity unavailable")
	}
	relationBID, ok := model.IssueRelationID(owner, testToken(t, "relation-b"))
	if !ok {
		t.Fatal("relation B identity unavailable")
	}
	columnA, ok := model.IssueColumnID(relationAID, testToken(t, "column-a"))
	if !ok {
		t.Fatal("column A identity unavailable")
	}
	columnB, ok := model.IssueColumnID(relationBID, testToken(t, "column-b"))
	if !ok {
		t.Fatal("column B identity unavailable")
	}
	typeA, ok := model.IssueTypeID(owner, testToken(t, "type-a"))
	if !ok {
		t.Fatal("type A identity unavailable")
	}
	typeB, ok := model.IssueTypeID(owner, testToken(t, "type-b"))
	if !ok {
		t.Fatal("type B identity unavailable")
	}
	keyIDA, ok := model.IssueKeyID(relationAID, testToken(t, "key-a"))
	if !ok {
		t.Fatal("key A identity unavailable")
	}
	keyIDB, ok := model.IssueKeyID(relationBID, testToken(t, "key-b"))
	if !ok {
		t.Fatal("key B identity unavailable")
	}
	scopeID, ok := model.IssueScopeID(owner, testToken(t, "scope"))
	if !ok {
		t.Fatal("scope identity unavailable")
	}
	scope := model.DefineScopeSchema(scopeID, []model.ColumnID{columnB, columnA})
	relationA := model.DefineRelationSchema(relationAID, []model.ColumnID{columnA}, []model.KeyID{keyIDA}, scope.ID())
	relationB := model.DefineRelationSchema(relationBID, []model.ColumnID{columnB}, []model.KeyID{keyIDB}, scope.ID())
	columnSchemaA := model.DefineColumnSchema(columnA, typeA)
	columnSchemaB := model.DefineColumnSchema(columnB, typeB)
	keySchemaA := model.DefineKeySchema(keyIDA, []model.ColumnID{columnA})
	keySchemaB := model.DefineKeySchema(keyIDB, []model.ColumnID{columnB})
	refA, _ := NewRelationRef(relationAID)
	refB, _ := NewRelationRef(relationBID)
	expressionIDA, _ := model.IssueExpressionID(owner, testToken(t, "expression-a"))
	expressionIDB, _ := model.IssueExpressionID(owner, testToken(t, "expression-b"))
	expressionA := DefineExpressionRef(expressionIDA, algebra.NewInput(relationAID))
	expressionB := DefineExpressionRef(expressionIDB, algebra.NewInput(relationBID))
	dependencyIDA, _ := model.IssueDependencyID(owner, testToken(t, "dependency-a"))
	dependencyIDB, _ := model.IssueDependencyID(owner, testToken(t, "dependency-b"))
	dependencyA := DefineDependency(dependencyIDA, expressionA.ID(), []RelationRef{refB}, []RelationRef{refA}, "alpha")
	dependencyB := DefineDependency(dependencyIDB, expressionB.ID(), []RelationRef{refA}, []RelationRef{refB}, "beta")
	dependencyRefA := DefineDependencyRef(dependencyIDA)
	dependencyRefB := DefineDependencyRef(dependencyIDB)
	edgeAB := DefineDependencyEdge(dependencyRefA, dependencyRefB)
	edgeBA := DefineDependencyEdge(dependencyRefB, dependencyRefA)
	recurrence := DefineRecurrence(Positive, nil)
	scc := DefineSCC([]DependencyRef{dependencyRefB, dependencyRefA}, []DependencyEdge{edgeBA, edgeAB}, recurrence)
	signatureA, ok := signature.Seal(signature.Spec{Identity: signature.Identity{Version: 1}})
	if !ok {
		t.Fatal("signature A unavailable")
	}
	signatureB, ok := signature.Seal(signature.Spec{Identity: signature.Identity{Version: 2}})
	if !ok {
		t.Fatal("signature B unavailable")
	}
	schemaID, ok := model.IssueSchemaID(owner, testToken(t, "schema"))
	if !ok {
		t.Fatal("schema identity unavailable")
	}
	return fixture{owner: owner, schemaID: schemaID, relationA: relationA, relationB: relationB, columnA: columnSchemaA, columnB: columnSchemaB, keyA: keySchemaA, keyB: keySchemaB, scope: scope, refA: refA, refB: refB, expressionA: expressionA, expressionB: expressionB, dependencyA: dependencyA, dependencyB: dependencyB, scc: scc, signatureA: signatureA, signatureB: signatureB}
}

func buildFixture(t *testing.T, value fixture, reverse bool) ExecutionSchema {
	return buildFixtureWithSchemaID(t, value, reverse, value.schemaID)
}

func buildFixtureWithSchemaID(t *testing.T, value fixture, reverse bool, schemaID model.SchemaID) ExecutionSchema {
	t.Helper()
	builder := NewBuilder(schemaID)
	if reverse {
		if !builder.AddRelation(value.relationB) || !builder.AddRelation(value.relationA) || !builder.AddColumn(value.columnB) || !builder.AddColumn(value.columnA) || !builder.AddKey(value.keyB) || !builder.AddKey(value.keyA) || !builder.AddScope(value.scope) || !builder.AddExpression(value.expressionB) || !builder.AddExpression(value.expressionA) || !builder.AddDependency(value.dependencyB) || !builder.AddDependency(value.dependencyA) || !builder.AddSCC(value.scc) || !builder.AddSignature(value.signatureB) || !builder.AddSignature(value.signatureA) {
			t.Fatal("reverse declaration rejected")
		}
	} else {
		if !builder.AddRelation(value.relationA) || !builder.AddRelation(value.relationB) || !builder.AddColumn(value.columnA) || !builder.AddColumn(value.columnB) || !builder.AddKey(value.keyA) || !builder.AddKey(value.keyB) || !builder.AddScope(value.scope) || !builder.AddExpression(value.expressionA) || !builder.AddExpression(value.expressionB) || !builder.AddDependency(value.dependencyA) || !builder.AddDependency(value.dependencyB) || !builder.AddSCC(value.scc) || !builder.AddSignature(value.signatureA) || !builder.AddSignature(value.signatureB) {
			t.Fatal("declaration rejected")
		}
	}
	schema, ok := builder.Build()
	if !ok {
		t.Fatal("schema build rejected")
	}
	return schema
}

func testToken(t *testing.T, label string) identity.ContentID {
	t.Helper()
	token, ok := identity.DeriveContentID("relation/schema/plan/test/v1", []byte(label))
	if !ok {
		t.Fatalf("token %q unavailable", label)
	}
	return token
}

func TestDeclarationOrderDoesNotChangeDigest(t *testing.T) {
	value := newFixture(t)
	forward := buildFixture(t, value, false)
	reverse := buildFixture(t, value, true)
	if !forward.Available() || !reverse.Available() {
		t.Fatal("built schema unavailable")
	}
	if forward.Digest() != reverse.Digest() {
		t.Fatalf("declaration order changed digest: %s != %s", forward.Digest(), reverse.Digest())
	}
}

func TestSchemaIDIsCanonicalArtifactIdentity(t *testing.T) {
	value := newFixture(t)
	baseline := buildFixture(t, value, false)
	if baseline.SchemaID() != value.schemaID {
		t.Fatal("execution schema dropped its canonical schema identity")
	}
	otherID, ok := model.IssueSchemaID(value.owner, testToken(t, "other-schema"))
	if !ok {
		t.Fatal("other schema identity unavailable")
	}
	other := buildFixtureWithSchemaID(t, value, false, otherID)
	if baseline.Digest() == other.Digest() {
		t.Fatal("schema identity did not affect execution schema digest")
	}
}

func TestArtifactIsMutationResistant(t *testing.T) {
	value := newFixture(t)
	schema := buildFixture(t, value, false)
	digest := schema.Digest()

	relations := schema.Relations()
	if len(relations) != 2 {
		t.Fatalf("unexpected relation count: %d", len(relations))
	}
	columns := relations[0].Columns()
	if len(columns) != 1 {
		t.Fatalf("unexpected column count: %d", len(columns))
	}
	columns[0] = model.ColumnID{}
	relations[0] = model.RelationSchema{}
	registeredColumns := schema.Columns()
	registeredColumns[0] = model.ColumnSchema{}
	registeredKeys := schema.Keys()
	registeredKeys[0] = model.KeySchema{}
	registeredScopes := schema.Scopes()
	registeredScopes[0] = model.ScopeSchema{}

	dependencies := schema.Dependencies()
	reads := dependencies[0].Reads()
	if len(reads) != 1 {
		t.Fatalf("unexpected read count: %d", len(reads))
	}
	reads[0] = RelationRef{}
	dependencies[0] = Dependency{}

	sccs := schema.SCCs()
	members := sccs[0].Members()
	members[0] = DependencyRef{}
	sccs[0] = SCC{}

	signatures := schema.Signatures()
	signatures[0] = signature.Signature{}
	if schema.Digest() != digest {
		t.Fatal("mutating accessor copies changed artifact digest")
	}
	if !schema.Available() {
		t.Fatal("mutating accessor copies invalidated artifact")
	}
	expressions := schema.Expressions()
	if len(expressions) != 2 || expressions[0].Expression() == nil || expressions[1].Expression() == nil {
		t.Fatal("expression DAG entries were not retained")
	}
}

func TestDeclarationsCopyCallerSlices(t *testing.T) {
	value := newFixture(t)
	reads := []RelationRef{value.refA, value.refB}
	writes := []RelationRef{value.refA}
	dependency := DefineDependency(value.dependencyA.ID(), value.expressionA.ID(), reads, writes, "copied")
	reads[0] = RelationRef{}
	writes[0] = RelationRef{}
	if len(dependency.Reads()) != 2 || len(dependency.Writes()) != 1 {
		t.Fatal("dependency retained caller slice mutation")
	}
	ordered := DefineDependency(value.dependencyA.ID(), value.expressionA.ID(), []RelationRef{value.refA, value.refB}, []RelationRef{value.refB, value.refA}, "set-order")
	reversed := DefineDependency(value.dependencyA.ID(), value.expressionA.ID(), []RelationRef{value.refB, value.refA}, []RelationRef{value.refA, value.refB}, "set-order")
	if ordered.Digest() != reversed.Digest() {
		t.Fatal("relation set declaration order changed dependency digest")
	}
	members := []DependencyRef{}
	memberA := DefineDependencyRef(value.dependencyA.ID())
	members = append(members, memberA)
	recurrence := DefineRecurrence(Positive, nil)
	scc := DefineSCC(members, nil, recurrence)
	members[0] = DependencyRef{}
	if len(scc.Members()) != 1 {
		t.Fatal("SCC retained caller member slice mutation")
	}
	memberB := DefineDependencyRef(value.dependencyB.ID())
	first := DefineSCC([]DependencyRef{memberA, memberB}, nil, DefineRecurrence(Positive, nil))
	second := DefineSCC([]DependencyRef{memberB, memberA}, nil, DefineRecurrence(Positive, nil))
	if first.Digest() != second.Digest() {
		t.Fatal("SCC member declaration order changed digest")
	}
}

func TestDependencyUsesNominalExpressionID(t *testing.T) {
	value := newFixture(t)
	alternate := DefineExpressionRef(value.expressionA.ID(), algebra.NewInput(value.relationB.ID()))
	left := DefineDependency(value.dependencyA.ID(), value.expressionA.ID(), []RelationRef{value.refB}, []RelationRef{value.refA}, "left")
	right := DefineDependency(value.dependencyA.ID(), alternate.ID(), []RelationRef{value.refB}, []RelationRef{value.refA}, "right")
	if left.Expression() != value.expressionA.ID() || right.Expression() != value.expressionA.ID() {
		t.Fatal("dependency did not retain the nominal expression identity")
	}
	if left.Digest() != right.Digest() {
		t.Fatal("dependency digest incorporated expression DAG data or debug label")
	}
}

func TestBuilderSealsOnce(t *testing.T) {
	value := newFixture(t)
	builder := NewBuilder(value.schemaID)
	if !builder.AddRelation(value.relationA) || !builder.AddRelation(value.relationB) || !builder.AddExpression(value.expressionA) || !builder.AddDependency(value.dependencyA) || !builder.AddSignature(value.signatureA) {
		t.Fatal("declaration rejected")
	}
	schema, ok := builder.Build()
	if !ok || !schema.Available() {
		t.Fatal("schema build rejected")
	}
	if builder.AddRelation(value.relationB) {
		t.Fatal("sealed builder accepted a new relation")
	}
	if _, ok := builder.Build(); ok {
		t.Fatal("sealed builder built twice")
	}
}

func TestBuilderRetainsUncheckedDeclarations(t *testing.T) {
	builder := NewBuilder(model.SchemaID{})
	if !builder.AddRelation(model.RelationSchema{}) || !builder.AddColumn(model.ColumnSchema{}) || !builder.AddKey(model.KeySchema{}) || !builder.AddScope(model.ScopeSchema{}) || !builder.AddExpression(ExpressionRef{}) || !builder.AddDependency(Dependency{}) || !builder.AddSCC(SCC{}) || !builder.AddSignature(signature.Signature{}) {
		t.Fatal("builder rejected an unchecked declaration")
	}
	if !builder.AddDependency(Dependency{}) {
		t.Fatal("builder rejected a duplicate unchecked declaration")
	}
	schema, ok := builder.Build()
	if !ok || len(schema.Relations()) != 1 || len(schema.Columns()) != 1 || len(schema.Keys()) != 1 || len(schema.Scopes()) != 1 || len(schema.Expressions()) != 1 || len(schema.Dependencies()) != 2 || len(schema.SCCs()) != 1 || len(schema.Signatures()) != 1 {
		t.Fatal("builder dropped an unchecked declaration")
	}
}

func TestDefineRetainsMalformedShapeForChecker(t *testing.T) {
	dependency := DefineDependency(model.DependencyID{}, model.ExpressionID{}, []RelationRef{{}}, []RelationRef{{}}, "")
	if dependency.Name() != "" || dependency.ID().Available() || len(dependency.Reads()) != 1 || len(dependency.Writes()) != 1 {
		t.Fatal("DefineDependency normalized away malformed declaration")
	}
	edge := DefineDependencyEdge(DependencyRef{}, DependencyRef{})
	head := DefineWideningHead(DependencyRef{}, RelationRef{})
	recurrence := DefineRecurrence(RecurrenceInvalid, []WideningHead{head})
	scc := DefineSCC([]DependencyRef{{}}, []DependencyEdge{edge}, recurrence)
	if len(scc.Members()) != 1 || len(scc.Edges()) != 1 || len(scc.Recurrence().Heads()) != 1 || scc.Recurrence().Kind() != RecurrenceInvalid {
		t.Fatal("DefineSCC normalized away malformed recurrence shape")
	}
	if dependency.Expression().Available() {
		t.Fatal("DefineDependency normalized away malformed expression reference")
	}
}

func TestMeaningfulDeclarationChangesDigest(t *testing.T) {
	value := newFixture(t)
	baseline := buildFixture(t, value, false)
	changedSignature, ok := signature.Seal(signature.Spec{Identity: signature.Identity{Version: 3}})
	if !ok {
		t.Fatal("changed signature unavailable")
	}
	builder := NewBuilder(value.schemaID)
	for _, relation := range []model.RelationSchema{value.relationA, value.relationB} {
		if !builder.AddRelation(relation) {
			t.Fatal("relation rejected")
		}
	}
	for _, column := range []model.ColumnSchema{value.columnA, value.columnB} {
		if !builder.AddColumn(column) {
			t.Fatal("column rejected")
		}
	}
	for _, key := range []model.KeySchema{value.keyA, value.keyB} {
		if !builder.AddKey(key) {
			t.Fatal("key rejected")
		}
	}
	if !builder.AddScope(value.scope) {
		t.Fatal("scope rejected")
	}
	for _, expression := range []ExpressionRef{value.expressionA, value.expressionB} {
		if !builder.AddExpression(expression) {
			t.Fatal("expression rejected")
		}
	}
	for _, dependency := range []Dependency{value.dependencyA, value.dependencyB} {
		if !builder.AddDependency(dependency) {
			t.Fatal("dependency rejected")
		}
	}
	if !builder.AddSCC(value.scc) || !builder.AddSignature(value.signatureA) || !builder.AddSignature(changedSignature) {
		t.Fatal("changed declaration rejected")
	}
	changed, ok := builder.Build()
	if !ok {
		t.Fatal("changed schema rejected")
	}
	if baseline.Digest() == changed.Digest() {
		t.Fatal("meaningful signature declaration did not change digest")
	}
}
