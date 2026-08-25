package certificate_test

import (
	"go/ast"
	"go/parser"
	gotoken "go/token"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/check/certificate"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/plan"
)

type fixture struct {
	owner    model.OwnerID
	schemaID model.SchemaID
	relation model.RelationID
	column   model.ColumnID
	typeID   model.TypeID
	key      model.KeyID
	scope    model.ScopeID
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	owner := issueOwner(t, "owner")
	relation := issueRelation(t, owner, "relation")
	column, ok := model.IssueColumnID(relation, token(t, "column"))
	if !ok {
		t.Fatal("column identity unavailable")
	}
	typeID, ok := model.IssueTypeID(owner, token(t, "type"))
	if !ok {
		t.Fatal("type identity unavailable")
	}
	key, ok := model.IssueKeyID(relation, token(t, "key"))
	if !ok {
		t.Fatal("key identity unavailable")
	}
	scope, ok := model.IssueScopeID(owner, token(t, "scope"))
	if !ok {
		t.Fatal("scope identity unavailable")
	}
	schemaID, ok := model.IssueSchemaID(owner, token(t, "schema"))
	if !ok {
		t.Fatal("schema identity unavailable")
	}
	return fixture{owner: owner, schemaID: schemaID, relation: relation, column: column, typeID: typeID, key: key, scope: scope}
}

func (value fixture) relationSchema() model.RelationSchema {
	return model.DefineRelationSchema(value.relation, []model.ColumnID{value.column}, []model.KeyID{value.key}, value.scope)
}

func (value fixture) columnSchema() model.ColumnSchema {
	return model.DefineColumnSchema(value.column, value.typeID)
}

func (value fixture) keySchema() model.KeySchema {
	return model.DefineKeySchema(value.key, []model.ColumnID{value.column})
}

func (value fixture) scopeSchema() model.ScopeSchema {
	return model.DefineScopeSchema(value.scope, nil)
}

func buildSchema(t *testing.T, value fixture, reverse bool, extras func(*plan.Builder)) plan.ExecutionSchema {
	t.Helper()
	builder := plan.NewBuilder(value.schemaID)
	if reverse {
		builder.AddScope(value.scopeSchema())
		builder.AddKey(value.keySchema())
		builder.AddColumn(value.columnSchema())
		builder.AddRelation(value.relationSchema())
	} else {
		builder.AddRelation(value.relationSchema())
		builder.AddColumn(value.columnSchema())
		builder.AddKey(value.keySchema())
		builder.AddScope(value.scopeSchema())
	}
	if extras != nil {
		extras(builder)
	}
	schema, ok := builder.Build()
	if !ok {
		t.Fatal("schema build rejected")
	}
	return schema
}

func TestStableCertificateDigest(t *testing.T) {
	value := newFixture(t)
	schema := buildSchema(t, value, false, nil)
	first, firstRefusal := certificate.Check(schema)
	second, secondRefusal := certificate.Check(schema)
	if firstRefusal != nil || secondRefusal != nil || !first.Available() || !second.Available() {
		t.Fatalf("valid schema refused: %v / %v", firstRefusal, secondRefusal)
	}
	if first.Digest() != second.Digest() {
		t.Fatal("equal schemas produced different certificate digests")
	}
	digest := schema.Digest()
	expected, ok := identity.DeriveContentID("analysis/relation/check/certificate/v1", digest[:])
	if !ok || first.Digest() != expected {
		t.Fatal("certificate digest is not the versioned schema-digest derivation")
	}
}

func TestDeclarationOrderDoesNotChangeCertificate(t *testing.T) {
	value := newFixture(t)
	forward, forwardRefusal := certificate.Check(buildSchema(t, value, false, nil))
	reverse, reverseRefusal := certificate.Check(buildSchema(t, value, true, nil))
	if forwardRefusal != nil || reverseRefusal != nil {
		t.Fatalf("declaration-order fixture refused: %v / %v", forwardRefusal, reverseRefusal)
	}
	if forward.Digest() != reverse.Digest() {
		t.Fatal("declaration order changed certificate digest")
	}
}

func TestLogicalMutationChangesCertificate(t *testing.T) {
	value := newFixture(t)
	baseline, refusal := certificate.Check(buildSchema(t, value, false, nil))
	if refusal != nil {
		t.Fatalf("baseline refused: %v", refusal)
	}
	changedType, ok := model.IssueTypeID(value.owner, token(t, "changed-type"))
	if !ok {
		t.Fatal("changed type unavailable")
	}
	changed := value
	changed.typeID = changedType
	mutated, mutatedRefusal := certificate.Check(buildSchema(t, changed, false, nil))
	if mutatedRefusal != nil {
		t.Fatalf("logical mutation refused: %v", mutatedRefusal)
	}
	if baseline.Digest() == mutated.Digest() {
		t.Fatal("logical column type mutation did not change certificate digest")
	}
}

func TestStructuralMutationRefusesZeroCertificate(t *testing.T) {
	value := newFixture(t)
	builder := plan.NewBuilder(value.schemaID)
	builder.AddRelation(model.RelationSchema{})
	schema, ok := builder.Build()
	if !ok {
		t.Fatal("malformed schema did not retain a digest")
	}
	certificateValue, refusal := certificate.Check(schema)
	assertZeroCertificate(t, certificateValue, refusal, certificate.PassStructural)
	count := 0
	for _, issue := range refusal.Issues() {
		if issue.Pass == certificate.PassStructural {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("structural issue was emitted %d times", count)
	}
}

func TestTypingMutationRefusesZeroCertificate(t *testing.T) {
	value := newFixture(t)
	expressionID := issueExpression(t, value.owner, "typing")
	input := algebra.NewInput(value.relation)
	expression := algebra.NewSelect(input, algebra.NewSelectContract(algebra.SelectModeInvalid, value.scope))
	schema := buildSchema(t, value, false, func(builder *plan.Builder) {
		builder.AddExpression(plan.DefineExpressionRef(expressionID, expression))
	})
	certificateValue, refusal := certificate.Check(schema)
	assertZeroCertificate(t, certificateValue, refusal, certificate.PassTyping)
}

func TestAuthorityMutationRefusesZeroCertificate(t *testing.T) {
	value := newFixture(t)
	expressionID := issueExpression(t, value.owner, "authority")
	input := algebra.NewInput(value.relation)
	publication := algebra.NewPublish(input, algebra.NewPublishContract(value.relation, value.key))
	schema := buildSchema(t, value, false, func(builder *plan.Builder) {
		builder.AddExpression(plan.DefineExpressionRef(expressionID, publication))
	})
	certificateValue, refusal := certificate.Check(schema)
	assertZeroCertificate(t, certificateValue, refusal, certificate.PassAuthority)
}

func TestRecurrenceMutationRefusesZeroCertificate(t *testing.T) {
	value := newFixture(t)
	expressionID := issueExpression(t, value.owner, "recurrence-expression")
	dependencyID, ok := model.IssueDependencyID(value.owner, token(t, "recurrence-dependency"))
	if !ok {
		t.Fatal("dependency identity unavailable")
	}
	input := algebra.NewInput(value.relation)
	ref, ok := plan.NewRelationRef(value.relation)
	if !ok {
		t.Fatal("relation reference unavailable")
	}
	dependency := plan.DefineDependency(dependencyID, expressionID, []plan.RelationRef{ref}, []plan.RelationRef{ref}, "mismatch")
	schema := buildSchema(t, value, false, func(builder *plan.Builder) {
		builder.AddExpression(plan.DefineExpressionRef(expressionID, input))
		builder.AddDependency(dependency)
	})
	certificateValue, refusal := certificate.Check(schema)
	assertZeroCertificate(t, certificateValue, refusal, certificate.PassRecurrence)
}

func TestCertificateAccessorsAreDefensive(t *testing.T) {
	value := newFixture(t)
	expressionID := issueExpression(t, value.owner, "defensive")
	schema := buildSchema(t, value, false, func(builder *plan.Builder) {
		builder.AddExpression(plan.DefineExpressionRef(expressionID, algebra.NewInput(value.relation)))
	})
	certificateValue, refusal := certificate.Check(schema)
	if refusal != nil {
		t.Fatalf("defensive fixture refused: %v", refusal)
	}
	relations := certificateValue.Relations()
	columns := certificateValue.Columns()
	keys := certificateValue.Keys()
	scopes := certificateValue.Scopes()
	expressions := certificateValue.Expressions()
	if len(relations) != 1 || len(columns) != 1 || len(keys) != 1 || len(scopes) != 1 || len(expressions) != 1 {
		t.Fatal("certificate did not expose expected declarations")
	}
	relations[0] = model.RelationSchema{}
	columns[0] = model.ColumnSchema{}
	keys[0] = model.KeySchema{}
	scopes[0] = model.ScopeSchema{}
	expressions[0] = plan.ExpressionRef{}
	if len(certificateValue.Relations()) != 1 || len(certificateValue.Columns()) != 1 || len(certificateValue.Keys()) != 1 || len(certificateValue.Scopes()) != 1 || len(certificateValue.Expressions()) != 1 {
		t.Fatal("mutating declaration slices changed the certificate")
	}
	proof := certificateValue.RecurrenceProof()
	if !proof.Available() {
		t.Fatal("valid certificate lost recurrence proof")
	}
	if certificateValue.MergeRequirements() != nil {
		t.Fatal("empty merge requirements should remain nil")
	}
}

func TestCertificateHasNoMountOrPhysicalSurface(t *testing.T) {
	file, err := parser.ParseFile(gotoken.NewFileSet(), "certificate.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, imported := range file.Imports {
		path := strings.Trim(imported.Path.Value, `"`)
		if strings.Contains(path, "/analysis/relation/mount") || strings.Contains(path, "/analysis/engine/") {
			t.Fatalf("certificate imports physical or mount package %q", path)
		}
	}
	for _, declaration := range file.Decls {
		group, ok := declaration.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range group.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok || typeSpec.Name.Name != "Certificate" {
				continue
			}
			structure, ok := typeSpec.Type.(*ast.StructType)
			if !ok {
				t.Fatal("Certificate is not a struct")
			}
			for _, field := range structure.Fields.List {
				for _, name := range field.Names {
					lower := strings.ToLower(name.Name)
					for _, forbidden := range []string{"physical", "storage", "runtime", "mount", "ordinal", "slot", "handle"} {
						if strings.Contains(lower, forbidden) {
							t.Fatalf("Certificate retains forbidden physical/mount field %q", name.Name)
						}
					}
				}
			}
		}
	}
}

func assertZeroCertificate(t *testing.T, value certificate.Certificate, refusal *certificate.Refusal, pass certificate.Pass) {
	t.Helper()
	if refusal == nil {
		t.Fatal("invalid schema was admitted")
	}
	if value.Available() || value.Digest().Available() || len(value.Relations()) != 0 {
		t.Fatal("invalid schema returned a non-zero certificate")
	}
	found := false
	for _, issue := range refusal.Issues() {
		if issue.Pass == pass {
			found = true
		}
	}
	if !found {
		t.Fatalf("refusal omitted expected %s pass", pass)
	}
}

func issueOwner(t *testing.T, label string) model.OwnerID {
	t.Helper()
	owner, ok := model.IssueOwnerID(token(t, label))
	if !ok {
		t.Fatal("owner identity unavailable")
	}
	return owner
}

func issueRelation(t *testing.T, owner model.OwnerID, label string) model.RelationID {
	t.Helper()
	relation, ok := model.IssueRelationID(owner, token(t, label))
	if !ok {
		t.Fatal("relation identity unavailable")
	}
	return relation
}

func issueExpression(t *testing.T, owner model.OwnerID, label string) model.ExpressionID {
	t.Helper()
	expression, ok := model.IssueExpressionID(owner, token(t, label))
	if !ok {
		t.Fatal("expression identity unavailable")
	}
	return expression
}

func token(t *testing.T, label string) identity.ContentID {
	t.Helper()
	value, ok := identity.DeriveContentID("relation/check/certificate/test/v1", []byte(label))
	if !ok {
		t.Fatalf("token %q unavailable", label)
	}
	return value
}
