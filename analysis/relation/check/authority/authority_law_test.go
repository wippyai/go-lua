package authority

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/region"
	"github.com/wippyai/go-lua/analysis/relation/schema/plan"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

type authorityFixture struct {
	owner           model.OwnerID
	foreignOwner    model.OwnerID
	schemaID        model.SchemaID
	foreignSchema   model.SchemaID
	relation        model.RelationID
	foreignRelation model.RelationID
	missingRelation model.RelationID
	column          model.ColumnID
	foreignColumn   model.ColumnID
	missingColumn   model.ColumnID
	key             model.KeyID
	foreignKey      model.KeyID
	missingKey      model.KeyID
	typeID          model.TypeID
	foreignType     model.TypeID
	foreignScope    model.ScopeSchema
	scope           model.ScopeSchema
	operation       model.OperationID
	expressionID    model.ExpressionID
	dependencyID    model.DependencyID
}

func newAuthorityFixture(t *testing.T) authorityFixture {
	t.Helper()
	owner := issueOwner(t, "owner")
	foreignOwner := issueOwner(t, "foreign-owner")
	schemaID := issueSchema(t, owner, "schema")
	foreignSchema := issueSchema(t, owner, "foreign-schema")
	relation := issueRelation(t, owner, "output")
	foreignRelation := issueRelation(t, foreignOwner, "input")
	missingRelation := issueRelation(t, owner, "missing")
	column := issueColumn(t, relation, "output")
	foreignColumn := issueColumn(t, foreignRelation, "input")
	missingColumn := issueColumn(t, missingRelation, "missing")
	key := issueKey(t, relation, "output")
	foreignKey := issueKey(t, foreignRelation, "input")
	missingKey := issueKey(t, missingRelation, "missing")
	typeID := issueType(t, owner, "output")
	foreignType := issueType(t, foreignOwner, "input")
	scopeID := issueScope(t, owner, "output")
	foreignScopeID := issueScope(t, foreignOwner, "input")
	scope := model.DefineScopeSchema(scopeID, []model.ColumnID{column}, region.True())
	foreignScope := model.DefineScopeSchema(foreignScopeID, []model.ColumnID{foreignColumn}, region.True())
	operation := issueOperation(t, owner, "operation")
	expressionID := issueExpression(t, owner, "expression")
	dependencyID := issueDependency(t, owner, "dependency")
	return authorityFixture{
		owner: owner, foreignOwner: foreignOwner, schemaID: schemaID, foreignSchema: foreignSchema,
		relation: relation, foreignRelation: foreignRelation, missingRelation: missingRelation,
		column: column, foreignColumn: foreignColumn, missingColumn: missingColumn,
		key: key, foreignKey: foreignKey, missingKey: missingKey,
		typeID: typeID, foreignType: foreignType, scope: scope, foreignScope: foreignScope,
		operation: operation, expressionID: expressionID, dependencyID: dependencyID,
	}
}

func (value authorityFixture) signature(t *testing.T, schema model.SchemaID, inputRelation model.RelationID, inputColumn model.ColumnID, inputType model.TypeID, inputKey model.KeyID, outputs []signature.Output) signature.Signature {
	t.Helper()
	inputDenominator, ok := model.NewDenominatorRef(inputRelation, inputKey)
	if !ok {
		t.Fatal("input denominator unavailable")
	}
	delivery, ok := signature.NewScalarDelivery()
	if !ok {
		t.Fatal("scalar delivery unavailable")
	}
	cardinality, ok := model.NewCardinality(model.ExactlyOne, 0)
	if !ok {
		t.Fatal("cardinality unavailable")
	}
	outcomes, ok := outcome.NewSet(outcome.Produced, outcome.NoSelection, outcome.Refused)
	if !ok {
		t.Fatal("outcomes unavailable")
	}
	sealed, ok := signature.Seal(signature.Spec{
		Identity:    signature.Identity{Operation: value.operation, Version: 1},
		Fence:       signature.Fence{Owner: value.owner, Schema: schema},
		Inputs:      []signature.Input{{Relation: inputRelation, Column: inputColumn, Type: inputType, Presence: signature.RequirePresent, Delivery: delivery, Denominator: inputDenominator}},
		Outputs:     outputs,
		Cardinality: cardinality,
		Outcomes:    outcomes,
	})
	if !ok {
		t.Fatal("signature unavailable")
	}
	return sealed
}

func (value authorityFixture) output(t *testing.T, relation model.RelationID, column model.ColumnID, typeID model.TypeID, denominator model.DenominatorRef) signature.Output {
	t.Helper()
	return signature.Output{Relation: relation, Column: column, Type: typeID, Presence: signature.ProducePresent, Denominator: denominator}
}

func (value authorityFixture) schema(t *testing.T, sig signature.Signature, inputRelation model.RelationID, inputColumn model.ColumnID, inputKey model.KeyID, expression algebra.Expression) plan.ExecutionSchema {
	t.Helper()
	outputDenominator, ok := model.NewDenominatorRef(value.relation, value.key)
	if !ok {
		t.Fatal("output denominator unavailable")
	}
	_ = outputDenominator
	inputRef, ok := plan.NewRelationRef(inputRelation)
	if !ok {
		t.Fatal("input reference unavailable")
	}
	outputRef, ok := plan.NewRelationRef(value.relation)
	if !ok {
		t.Fatal("output reference unavailable")
	}
	entry := plan.DefineExpressionRef(value.expressionID, expression)
	dependency := plan.DefineDependency(value.dependencyID, value.expressionID, []plan.RelationRef{inputRef}, []plan.RelationRef{outputRef}, "authority-law")
	builder := plan.NewBuilder(value.schemaID)
	outputRelation := model.DefineRelationSchema(value.relation, []model.ColumnID{value.column}, []model.KeyID{value.key}, value.scope.ID())
	inputRelationSchema := model.DefineRelationSchema(inputRelation, []model.ColumnID{inputColumn}, []model.KeyID{inputKey}, value.foreignScope.ID())
	if !builder.AddRelation(outputRelation) || !builder.AddRelation(inputRelationSchema) ||
		!builder.AddColumn(model.DefineColumnSchema(value.column, value.typeID)) || !builder.AddColumn(model.DefineColumnSchema(inputColumn, value.foreignType)) ||
		!builder.AddKey(model.DefineKeySchema(value.key, []model.ColumnID{value.column})) || !builder.AddKey(model.DefineKeySchema(inputKey, []model.ColumnID{inputColumn})) ||
		!builder.AddScope(value.scope) || !builder.AddScope(value.foreignScope) || !builder.AddExpression(entry) || !builder.AddDependency(dependency) || !builder.AddSignature(sig) {
		t.Fatal("declaration rejected")
	}
	result, ok := builder.Build()
	if !ok {
		t.Fatal("schema build rejected")
	}
	return result
}

func validSchema(t *testing.T) (authorityFixture, plan.ExecutionSchema) {
	t.Helper()
	value := newAuthorityFixture(t)
	denominator, ok := model.NewDenominatorRef(value.relation, value.key)
	if !ok {
		t.Fatal("output denominator unavailable")
	}
	sig := value.signature(t, value.schemaID, value.foreignRelation, value.foreignColumn, value.foreignType, value.foreignKey, []signature.Output{value.output(t, value.relation, value.column, value.typeID, denominator)})
	input := algebra.NewInput(value.foreignRelation)
	apply := algebra.NewApply([]algebra.Expression{input}, algebra.NewApplyContract(sig.Identity(), []algebra.SlotSource{algebra.NewSlotSource(0, 0)}, algebra.OwnerNamed()))
	publish := algebra.NewPublish(apply, algebra.NewPublishContract(value.relation, value.key))
	return value, value.schema(t, sig, value.foreignRelation, value.foreignColumn, value.foreignKey, publish)
}

func TestCrossOwnerInputIsValidWhenEveryReferenceIsDeclared(t *testing.T) {
	_, schema := validSchema(t)
	if report := Check(schema); !report.Valid() {
		t.Fatalf("valid cross-owner input was refused: %s", report.Error())
	}
}

func TestObservationCheckerRefusesCrossRelationWithoutProjection(t *testing.T) {
	value, source := validSchema(t)
	population, ok := model.NewDenominatorRef(value.relation, value.key)
	if !ok {
		t.Fatal("population")
	}
	operation := source.Signatures()[0]
	columnSchema, ok := findColumn(source.Columns(), value.column)
	if !ok {
		t.Fatal("output column")
	}
	cardinality, ok := model.NewCardinality(model.ExactlyOne, 0)
	if !ok {
		t.Fatal("cardinality")
	}
	observation := algebra.NewObservationContract(
		value.dependencyID, operation.Identity(), algebra.NewObservationSource(0, 0, 0), population,
		algebra.NewObservationOutput(value.column, columnSchema.Type(), population, cardinality),
	)
	builder := plan.NewBuilder(source.SchemaID())
	for _, declaration := range source.Relations() {
		if !builder.AddRelation(declaration) {
			t.Fatal("relation")
		}
	}
	for _, declaration := range source.Columns() {
		if !builder.AddColumn(declaration) {
			t.Fatal("column")
		}
	}
	for _, declaration := range source.Keys() {
		if !builder.AddKey(declaration) {
			t.Fatal("key")
		}
	}
	for _, declaration := range source.Scopes() {
		if !builder.AddScope(declaration) {
			t.Fatal("scope")
		}
	}
	for _, declaration := range source.Expressions() {
		if !builder.AddExpression(declaration) {
			t.Fatal("expression")
		}
	}
	for _, declaration := range source.Dependencies() {
		if !builder.AddDependency(declaration) {
			t.Fatal("dependency")
		}
	}
	for _, declaration := range source.SCCs() {
		if !builder.AddSCC(declaration) {
			t.Fatal("scc")
		}
	}
	for _, declaration := range source.Signatures() {
		if !builder.AddSignature(declaration) {
			t.Fatal("signature")
		}
	}
	if !builder.AddObservation(observation) {
		t.Fatal("observation")
	}
	withObservation, ok := builder.Build()
	if !ok {
		t.Fatal("schema")
	}
	report := Check(withObservation)
	if report.Valid() {
		t.Fatal("cross-relation observation accepted without a source-to-row projection")
	}
	for _, issue := range report.Issues() {
		if issue.Code == CodeInvalidObservation {
			return
		}
	}
	t.Fatalf("observation refusal omitted invalid-observation code: %s", report.Error())
}

func TestObservationCheckerRefusesCompleteDenominatorOutput(t *testing.T) {
	value, source := validSchema(t)
	population, ok := model.NewDenominatorRef(value.relation, value.key)
	if !ok {
		t.Fatal("population")
	}
	operation := source.Signatures()[0]
	columnSchema, ok := findColumn(source.Columns(), value.column)
	if !ok {
		t.Fatal("output column")
	}
	cardinality, ok := model.NewCardinality(model.CompleteDenominator, 0)
	if !ok {
		t.Fatal("complete cardinality")
	}
	observation := algebra.NewObservationContract(
		value.dependencyID, operation.Identity(), algebra.NewObservationSource(0, 0, 0), population,
		algebra.NewObservationOutput(value.column, columnSchema.Type(), population, cardinality),
	)
	schema := observationSchema(t, value, source, source.Expressions()[0].Expression(), observation)
	report := Check(schema)
	if !hasCode(report, CodeInvalidObservation) {
		t.Fatalf("CompleteDenominator observation output was accepted: %s", report.Error())
	}
}

func TestObservationCheckerRejectsForeignDependency(t *testing.T) {
	value, source := validSchema(t)
	inputDenominator, ok := model.NewDenominatorRef(value.foreignRelation, value.foreignKey)
	if !ok {
		t.Fatal("input denominator")
	}
	outputDenominator, ok := model.NewDenominatorRef(value.relation, value.key)
	if !ok {
		t.Fatal("output denominator")
	}
	operation := source.Signatures()[0]
	columnSchema, ok := findColumn(source.Columns(), value.column)
	if !ok {
		t.Fatal("output column")
	}
	cardinality, ok := model.NewCardinality(model.ExactlyOne, 0)
	if !ok {
		t.Fatal("cardinality")
	}
	foreignExpressionID := issueExpression(t, value.foreignOwner, "foreign-observation-expression")
	foreignDependency := issueDependency(t, value.foreignOwner, "foreign-observation")
	foreignExpression := plan.DefineExpressionRef(foreignExpressionID, algebra.NewInput(value.foreignRelation))
	foreignRelationRef, ok := plan.NewRelationRef(value.foreignRelation)
	if !ok {
		t.Fatal("foreign relation reference")
	}
	foreignDeclaration := plan.DefineDependency(foreignDependency, foreignExpressionID, []plan.RelationRef{foreignRelationRef}, nil, "foreign-observation")
	observation := algebra.NewObservationContract(
		foreignDependency, operation.Identity(), algebra.NewObservationSource(0, 0, 0), inputDenominator,
		algebra.NewObservationOutput(value.column, columnSchema.Type(), outputDenominator, cardinality),
	)
	schema := observationSchemaWithExtra(t, value, source, source.Expressions()[0].Expression(), observation, []plan.ExpressionRef{foreignExpression}, []plan.Dependency{foreignDeclaration})
	report := Check(schema)
	if !hasCode(report, CodeInvalidObservation) {
		t.Fatalf("foreign observation dependency was accepted: %s", report.Error())
	}
}

func TestObservationCheckerRejectsAmbiguousSameOperationOccurrences(t *testing.T) {
	value, source := validSchema(t)
	inputDenominator, ok := model.NewDenominatorRef(value.foreignRelation, value.foreignKey)
	if !ok {
		t.Fatal("input denominator")
	}
	outputDenominator, ok := model.NewDenominatorRef(value.relation, value.key)
	if !ok {
		t.Fatal("output denominator")
	}
	operation := source.Signatures()[0]
	columnSchema, ok := findColumn(source.Columns(), value.column)
	if !ok {
		t.Fatal("output column")
	}
	cardinality, ok := model.NewCardinality(model.ExactlyOne, 0)
	if !ok {
		t.Fatal("cardinality")
	}
	observation := algebra.NewObservationContract(
		value.dependencyID, operation.Identity(), algebra.NewObservationSource(0, 0, 0), inputDenominator,
		algebra.NewObservationOutput(value.column, columnSchema.Type(), outputDenominator, cardinality),
	)
	root, ok := source.Expressions()[0].Expression().(algebra.Publish)
	if !ok {
		t.Fatal("publish root")
	}
	ambiguous := algebra.NewMerge(
		[]algebra.Expression{root.Child(), root.Child()},
		algebra.NewMergeContract(value.key),
	)
	schema := observationSchema(t, value, source, ambiguous, observation)
	report := Check(schema)
	if !hasCode(report, CodeInvalidObservation) {
		t.Fatalf("observation accepted two same-operation Apply occurrences: %s", report.Error())
	}
}

func observationSchema(t *testing.T, value authorityFixture, source plan.ExecutionSchema, root algebra.Expression, observation algebra.ObservationContract) plan.ExecutionSchema {
	return observationSchemaWithExtra(t, value, source, root, observation, nil, nil)
}

func observationSchemaWithExtra(t *testing.T, value authorityFixture, source plan.ExecutionSchema, root algebra.Expression, observation algebra.ObservationContract, extraExpressions []plan.ExpressionRef, extraDependencies []plan.Dependency) plan.ExecutionSchema {
	t.Helper()
	builder := plan.NewBuilder(source.SchemaID())
	for _, declaration := range source.Relations() {
		if !builder.AddRelation(declaration) {
			t.Fatal("relation")
		}
	}
	for _, declaration := range source.Columns() {
		if !builder.AddColumn(declaration) {
			t.Fatal("column")
		}
	}
	for _, declaration := range source.Keys() {
		if !builder.AddKey(declaration) {
			t.Fatal("key")
		}
	}
	for _, declaration := range source.Scopes() {
		if !builder.AddScope(declaration) {
			t.Fatal("scope")
		}
	}
	for _, declaration := range source.Expressions() {
		if declaration.ID() == value.expressionID {
			declaration = plan.DefineExpressionRef(value.expressionID, root)
		}
		if !builder.AddExpression(declaration) {
			t.Fatal("expression")
		}
	}
	for _, declaration := range extraExpressions {
		if !builder.AddExpression(declaration) {
			t.Fatal("extra expression")
		}
	}
	for _, declaration := range source.Dependencies() {
		if !builder.AddDependency(declaration) {
			t.Fatal("dependency")
		}
	}
	for _, declaration := range extraDependencies {
		if !builder.AddDependency(declaration) {
			t.Fatal("extra dependency")
		}
	}
	for _, declaration := range source.SCCs() {
		if !builder.AddSCC(declaration) {
			t.Fatal("scc")
		}
	}
	for _, declaration := range source.Signatures() {
		if !builder.AddSignature(declaration) {
			t.Fatal("signature")
		}
	}
	if !builder.AddObservation(observation) {
		t.Fatal("observation")
	}
	result, ok := builder.Build()
	if !ok {
		t.Fatal("schema")
	}
	return result
}

func findColumn(values []model.ColumnSchema, id model.ColumnID) (model.ColumnSchema, bool) {
	for _, value := range values {
		if value.ID() == id {
			return value, true
		}
	}
	return model.ColumnSchema{}, false
}

func TestForeignSchemaFenceIsRejected(t *testing.T) {
	value := newAuthorityFixture(t)
	denominator, _ := model.NewDenominatorRef(value.relation, value.key)
	sig := value.signature(t, value.foreignSchema, value.foreignRelation, value.foreignColumn, value.foreignType, value.foreignKey, []signature.Output{value.output(t, value.relation, value.column, value.typeID, denominator)})
	input := algebra.NewInput(value.foreignRelation)
	publish := algebra.NewPublish(algebra.NewApply([]algebra.Expression{input}, algebra.NewApplyContract(sig.Identity(), []algebra.SlotSource{algebra.NewSlotSource(0, 0)}, algebra.OwnerNamed())), algebra.NewPublishContract(value.relation, value.key))
	if report := Check(value.schema(t, sig, value.foreignRelation, value.foreignColumn, value.foreignKey, publish)); !hasCode(report, CodeForeignSchema) {
		t.Fatalf("foreign schema fence was accepted: %s", report.Error())
	}
}

func TestWrongOutputDenominatorIsRejected(t *testing.T) {
	value := newAuthorityFixture(t)
	wrong, _ := model.NewDenominatorRef(value.foreignRelation, value.foreignKey)
	sig := value.signature(t, value.schemaID, value.foreignRelation, value.foreignColumn, value.foreignType, value.foreignKey, []signature.Output{value.output(t, value.relation, value.column, value.typeID, wrong)})
	input := algebra.NewInput(value.foreignRelation)
	publish := algebra.NewPublish(algebra.NewApply([]algebra.Expression{input}, algebra.NewApplyContract(sig.Identity(), []algebra.SlotSource{algebra.NewSlotSource(0, 0)}, algebra.OwnerNamed())), algebra.NewPublishContract(value.relation, value.key))
	if report := Check(value.schema(t, sig, value.foreignRelation, value.foreignColumn, value.foreignKey, publish)); !hasCode(report, CodeInvalidOutputDenominator) {
		t.Fatalf("wrong output denominator was accepted: %s", report.Error())
	}
}

func TestDuplicateOutputIsRejected(t *testing.T) {
	value := newAuthorityFixture(t)
	denominator, _ := model.NewDenominatorRef(value.relation, value.key)
	output := value.output(t, value.relation, value.column, value.typeID, denominator)
	sig := value.signature(t, value.schemaID, value.foreignRelation, value.foreignColumn, value.foreignType, value.foreignKey, []signature.Output{output, output})
	input := algebra.NewInput(value.foreignRelation)
	publish := algebra.NewPublish(algebra.NewApply([]algebra.Expression{input}, algebra.NewApplyContract(sig.Identity(), []algebra.SlotSource{algebra.NewSlotSource(0, 0)}, algebra.OwnerNamed())), algebra.NewPublishContract(value.relation, value.key))
	if report := Check(value.schema(t, sig, value.foreignRelation, value.foreignColumn, value.foreignKey, publish)); !hasCode(report, CodeDuplicateOutput) {
		t.Fatalf("duplicate output was accepted: %s", report.Error())
	}
}

func TestSameOwnerForeignRelationStillRequiresSchemaMembership(t *testing.T) {
	value := newAuthorityFixture(t)
	denominator, _ := model.NewDenominatorRef(value.relation, value.key)
	sig := value.signature(t, value.schemaID, value.missingRelation, value.missingColumn, value.typeID, value.missingKey, []signature.Output{value.output(t, value.relation, value.column, value.typeID, denominator)})
	input := algebra.NewInput(value.missingRelation)
	publish := algebra.NewPublish(algebra.NewApply([]algebra.Expression{input}, algebra.NewApplyContract(sig.Identity(), []algebra.SlotSource{algebra.NewSlotSource(0, 0)}, algebra.OwnerNamed())), algebra.NewPublishContract(value.relation, value.key))
	outputRelation := model.DefineRelationSchema(value.relation, []model.ColumnID{value.column}, []model.KeyID{value.key}, value.scope.ID())
	outputRef, _ := plan.NewRelationRef(value.relation)
	missingRef, _ := plan.NewRelationRef(value.missingRelation)
	entry := plan.DefineExpressionRef(value.expressionID, publish)
	dependency := plan.DefineDependency(value.dependencyID, value.expressionID, []plan.RelationRef{missingRef}, []plan.RelationRef{outputRef}, "authority-law")
	builder := plan.NewBuilder(value.schemaID)
	if !builder.AddRelation(outputRelation) || !builder.AddColumn(model.DefineColumnSchema(value.column, value.typeID)) || !builder.AddKey(model.DefineKeySchema(value.key, []model.ColumnID{value.column})) || !builder.AddScope(value.scope) || !builder.AddExpression(entry) || !builder.AddDependency(dependency) || !builder.AddSignature(sig) {
		t.Fatal("declaration rejected")
	}
	schema, ok := builder.Build()
	if !ok {
		t.Fatal("schema build rejected")
	}
	report := Check(schema)
	if !hasCode(report, CodeUnknownRelation) || !hasCode(report, CodeUnknownColumn) {
		t.Fatalf("same-owner foreign relation escaped membership checks: %s", report.Error())
	}
}

func hasCode(report Report, code Code) bool {
	for _, issue := range report.Issues() {
		if issue.Code == code {
			return true
		}
	}
	return false
}

func content(t *testing.T, value string) identity.ContentID {
	t.Helper()
	result, ok := identity.DeriveContentID("relation/check/authority/test/v1", []byte(value))
	if !ok {
		t.Fatal("content unavailable")
	}
	return result
}

func issueOwner(t *testing.T, value string) model.OwnerID {
	result, ok := model.IssueOwnerID(content(t, "owner/"+value))
	if !ok {
		t.Fatal("owner unavailable")
	}
	return result
}

func issueSchema(t *testing.T, owner model.OwnerID, value string) model.SchemaID {
	result, ok := model.IssueSchemaID(owner, content(t, "schema/"+value))
	if !ok {
		t.Fatal("schema unavailable")
	}
	return result
}

func issueRelation(t *testing.T, owner model.OwnerID, value string) model.RelationID {
	result, ok := model.IssueRelationID(owner, content(t, "relation/"+value))
	if !ok {
		t.Fatal("relation unavailable")
	}
	return result
}

func issueColumn(t *testing.T, relation model.RelationID, value string) model.ColumnID {
	result, ok := model.IssueColumnID(relation, content(t, "column/"+value))
	if !ok {
		t.Fatal("column unavailable")
	}
	return result
}

func issueKey(t *testing.T, relation model.RelationID, value string) model.KeyID {
	result, ok := model.IssueKeyID(relation, content(t, "key/"+value))
	if !ok {
		t.Fatal("key unavailable")
	}
	return result
}

func issueType(t *testing.T, owner model.OwnerID, value string) model.TypeID {
	result, ok := model.IssueTypeID(owner, content(t, "type/"+value))
	if !ok {
		t.Fatal("type unavailable")
	}
	return result
}

func issueScope(t *testing.T, owner model.OwnerID, value string) model.ScopeID {
	result, ok := model.IssueScopeID(owner, content(t, "scope/"+value))
	if !ok {
		t.Fatal("scope unavailable")
	}
	return result
}

func issueOperation(t *testing.T, owner model.OwnerID, value string) model.OperationID {
	result, ok := model.IssueOperationID(owner, content(t, "operation/"+value))
	if !ok {
		t.Fatal("operation unavailable")
	}
	return result
}

func issueExpression(t *testing.T, owner model.OwnerID, value string) model.ExpressionID {
	result, ok := model.IssueExpressionID(owner, content(t, "expression/"+value))
	if !ok {
		t.Fatal("expression unavailable")
	}
	return result
}

func issueDependency(t *testing.T, owner model.OwnerID, value string) model.DependencyID {
	result, ok := model.IssueDependencyID(owner, content(t, "dependency/"+value))
	if !ok {
		t.Fatal("dependency unavailable")
	}
	return result
}
