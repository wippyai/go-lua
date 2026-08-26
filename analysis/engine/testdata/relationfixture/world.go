// Package testfixture contains the one public-API fixture shared by the W3
// relation kernel laws.  It is deliberately an internal test dependency: it
// is not a production execution surface and it does not expose state's
// physical column implementation.
//
// The fixture is intentionally assembled through the same boundaries as a
// real solve:
//
//	schema declarations -> certificate -> mounted witness -> cofiber and
//	geometry -> initial database -> apply -> transaction/publication ->
//	authenticated Readers.
//
// Keeping this construction here prevents each operator package from growing
// its own fake Reader, row identity, scope map, or column writer.
package testfixture

import (
	"bytes"
	"sync"

	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"github.com/wippyai/go-lua/analysis/engine/relation/cofiber/lower"
	"github.com/wippyai/go-lua/analysis/engine/relation/publish"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/database"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/geometry"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/store"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/check/certificate"
	"github.com/wippyai/go-lua/analysis/relation/mount/address"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	schemaalgebra "github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/plan"
	"github.com/wippyai/go-lua/analysis/relation/schema/region"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/lineage"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

const fixtureDomain = "analysis/engine/testdata/relationfixture/public/v1"

// Fixture is a complete, immutable-public-API W3 test world. The left root
// contains only left rows, so joining it against an empty right relation
// produces NoSelection. Both contains one matching and one non-matching
// two-component key, so the same plan proves both positive correspondence and
// an unmatched candidate.
type Fixture struct {
	mounted    witness.Mounted
	view       geometry.Geometry
	door       publish.Door
	base       database.Version
	left       database.Version
	both       database.Version
	leftDelta  database.Delta
	rightDelta database.Delta
	scratch    *store.ReadScratch
	seedType   model.TypeID
	seedAscent []model.TypeID

	leftRelation                   model.RelationID
	rightRelation                  model.RelationID
	applyRelation                  model.RelationID
	emptyRelation                  model.RelationID
	leftKey                        model.KeyID
	leftCoordinateKey              model.KeyID
	rightKey                       model.KeyID
	applyKey                       model.KeyID
	emptyKey                       model.KeyID
	applyValue                     model.ColumnID
	applyFact                      model.ColumnID
	leftKeys                       [2]model.ColumnID
	rightKeys                      [2]model.ColumnID
	leftPayload                    [2]model.ColumnID
	rightPayload                   [2]model.ColumnID
	leftRows                       [2]model.RowID
	rightRows                      [2]model.RowID
	applyRow                       model.RowID
	leftOperation                  signature.Identity
	rightOperation                 signature.Identity
	leftExpression                 model.ExpressionID
	rightExpression                model.ExpressionID
	projectExpression              model.ExpressionID
	payloadExpression              model.ExpressionID
	selectExpression               model.ExpressionID
	groupExpression                model.ExpressionID
	mergeExpression                model.ExpressionID
	mergeApplyExpression           model.ExpressionID
	completeExpression             model.ExpressionID
	twoScalarApplyExpression       model.ExpressionID
	twoScalarApplyObservation      identity.ContentID
	scalarSpanApplyExpression      model.ExpressionID
	emptyExpression                model.ExpressionID
	emptyCompleteExpression        model.ExpressionID
	scalarEmptyApplyExpression     model.ExpressionID
	correlatedApplyExpression      model.ExpressionID
	mixedPopulationApplyExpression model.ExpressionID
	sharedCompleteApplyExpression  model.ExpressionID
	sharedEmptyApplyExpression     model.ExpressionID
	leftDependency                 model.DependencyID
	rightDependency                model.DependencyID
	completeDependency             model.DependencyID
	twoScalarApplyDependency       model.DependencyID
	correlatedApplyDependency      model.DependencyID
	mixedPopulationApplyDependency model.DependencyID
	sharedCompleteApplyDependency  model.DependencyID
	sharedEmptyApplyDependency     model.DependencyID
	twoScalarApplyWorker           *applyWorker
	scalarSpanApplyWorker          *applyWorker
	scalarEmptyApplyWorker         *applyWorker
	correlatedApplyWorker          *applyWorker
	mixedPopulationApplyWorker     *applyWorker
	sharedCompleteApplyWorker      *applyWorker
	sharedEmptyApplyWorker         *applyWorker

	leftKeyLayout    arrangement.Layout
	rightKeyLayout   arrangement.Layout
	leftValueLayout  arrangement.Layout
	rightValueLayout arrangement.Layout
	inputLayout      arrangement.Layout
	rightInputLayout arrangement.Layout
	applyFactLayout  arrangement.Layout

	scopes scopeSet
}

// scopeSet contains normalized runtime scopes. No support.Mask crosses this
// API: operators receive only the mounted Scope value and use Reader.Conjoin
// for scope algebra.
type scopeSet struct {
	overlapLeft, overlapRight   witness.Scope
	disjointLeft, disjointRight witness.Scope
}

var cachedFixtures sync.Map

// New returns one immutable fixture world for the requested mount. Cold
// certificate/mount/database construction is cached by the mount byte; each
// caller receives fresh read scratch because scratch is deliberately not
// concurrent-safe while all mounted roots and identities remain immutable.
func New(t TB, mountBytes ...byte) Fixture {
	t.Helper()
	mountByte := byte(0x71)
	if len(mountBytes) > 1 {
		t.Fatalf("fixture: at most one mount byte, got %d", len(mountBytes))
	}
	if len(mountBytes) == 1 {
		mountByte = mountBytes[0]
	}
	if cached, ok := cachedFixtures.Load(mountByte); ok {
		fixture := cached.(Fixture)
		fixture.scratch = store.NewReadScratch(fixture.view.Manager())
		if fixture.scratch == nil || !fixture.scratch.Available() {
			t.Fatalf("fixture cached scratch")
		}
		return fixture
	}
	fixture := build(t, mountByte)
	cachedFixtures.Store(mountByte, fixture)
	return fixture
}

// build performs the cold certificate -> mount -> cofiber -> database ->
// transaction/publication construction once for a mount identity.
func build(t TB, mountByte byte) Fixture {
	t.Helper()
	issue := func(label string, fn func(identity.ContentID) (any, bool)) any {
		t.Helper()
		id, ok := identity.DeriveContentID(fixtureDomain, []byte(label))
		if !ok {
			t.Fatalf("fixture identity %s", label)
		}
		value, ok := fn(id)
		if !ok {
			t.Fatalf("fixture issue %s", label)
		}
		return value
	}
	owner := issue("owner", func(id identity.ContentID) (any, bool) { return model.IssueOwnerID(id) }).(model.OwnerID)
	schemaID := issue("schema", func(id identity.ContentID) (any, bool) { return model.IssueSchemaID(owner, id) }).(model.SchemaID)
	leftRelation := issue("relation-left", func(id identity.ContentID) (any, bool) { return model.IssueRelationID(owner, id) }).(model.RelationID)
	rightRelation := issue("relation-right", func(id identity.ContentID) (any, bool) { return model.IssueRelationID(owner, id) }).(model.RelationID)
	applyRelation := issue("relation-apply", func(id identity.ContentID) (any, bool) { return model.IssueRelationID(owner, id) }).(model.RelationID)
	emptyRelation := issue("relation-empty", func(id identity.ContentID) (any, bool) { return model.IssueRelationID(owner, id) }).(model.RelationID)
	leftScopeID := issue("scope-left", func(id identity.ContentID) (any, bool) { return model.IssueScopeID(owner, id) }).(model.ScopeID)
	rightScopeID := issue("scope-right", func(id identity.ContentID) (any, bool) { return model.IssueScopeID(owner, id) }).(model.ScopeID)
	typeID := issue("value-type", func(id identity.ContentID) (any, bool) { return model.IssueTypeID(owner, id) }).(model.TypeID)
	leftKeyA := issue("left-key-a", func(id identity.ContentID) (any, bool) { return model.IssueColumnID(leftRelation, id) }).(model.ColumnID)
	leftKeyB := issue("left-key-b", func(id identity.ContentID) (any, bool) { return model.IssueColumnID(leftRelation, id) }).(model.ColumnID)
	leftValueA := issue("left-value-a", func(id identity.ContentID) (any, bool) { return model.IssueColumnID(leftRelation, id) }).(model.ColumnID)
	leftValueB := issue("left-value-b", func(id identity.ContentID) (any, bool) { return model.IssueColumnID(leftRelation, id) }).(model.ColumnID)
	rightKeyA := issue("right-key-a", func(id identity.ContentID) (any, bool) { return model.IssueColumnID(rightRelation, id) }).(model.ColumnID)
	rightKeyB := issue("right-key-b", func(id identity.ContentID) (any, bool) { return model.IssueColumnID(rightRelation, id) }).(model.ColumnID)
	rightValueA := issue("right-value-a", func(id identity.ContentID) (any, bool) { return model.IssueColumnID(rightRelation, id) }).(model.ColumnID)
	rightValueB := issue("right-value-b", func(id identity.ContentID) (any, bool) { return model.IssueColumnID(rightRelation, id) }).(model.ColumnID)
	applyValue := issue("apply-value", func(id identity.ContentID) (any, bool) { return model.IssueColumnID(applyRelation, id) }).(model.ColumnID)
	applyFact := issue("apply-fact", func(id identity.ContentID) (any, bool) { return model.IssueColumnID(applyRelation, id) }).(model.ColumnID)
	emptyValue := issue("empty-value", func(id identity.ContentID) (any, bool) { return model.IssueColumnID(emptyRelation, id) }).(model.ColumnID)
	leftKey := issue("key-left", func(id identity.ContentID) (any, bool) { return model.IssueKeyID(leftRelation, id) }).(model.KeyID)
	leftCoordinateKey := issue("key-left-coordinate", func(id identity.ContentID) (any, bool) { return model.IssueKeyID(leftRelation, id) }).(model.KeyID)
	rightKey := issue("key-right", func(id identity.ContentID) (any, bool) { return model.IssueKeyID(rightRelation, id) }).(model.KeyID)
	applyKey := issue("key-apply", func(id identity.ContentID) (any, bool) { return model.IssueKeyID(applyRelation, id) }).(model.KeyID)
	emptyKey := issue("key-empty", func(id identity.ContentID) (any, bool) { return model.IssueKeyID(emptyRelation, id) }).(model.KeyID)
	leftOperationID := issue("operation-left", func(id identity.ContentID) (any, bool) { return model.IssueOperationID(owner, id) }).(model.OperationID)
	rightOperationID := issue("operation-right", func(id identity.ContentID) (any, bool) { return model.IssueOperationID(owner, id) }).(model.OperationID)
	twoScalarApplyOperationID := issue("operation-two-scalar-apply", func(id identity.ContentID) (any, bool) { return model.IssueOperationID(owner, id) }).(model.OperationID)
	scalarSpanApplyOperationID := issue("operation-scalar-complete-apply", func(id identity.ContentID) (any, bool) { return model.IssueOperationID(owner, id) }).(model.OperationID)
	scalarEmptyApplyOperationID := issue("operation-scalar-empty-complete-apply", func(id identity.ContentID) (any, bool) { return model.IssueOperationID(owner, id) }).(model.OperationID)
	correlatedApplyOperationID := issue("operation-correlated-apply", func(id identity.ContentID) (any, bool) { return model.IssueOperationID(owner, id) }).(model.OperationID)
	mixedPopulationApplyOperationID := issue("operation-mixed-population-span-slots-apply", func(id identity.ContentID) (any, bool) { return model.IssueOperationID(owner, id) }).(model.OperationID)
	sharedCompleteApplyOperationID := issue("operation-shared-complete-apply", func(id identity.ContentID) (any, bool) { return model.IssueOperationID(owner, id) }).(model.OperationID)
	sharedEmptyApplyOperationID := issue("operation-shared-empty-complete-apply", func(id identity.ContentID) (any, bool) { return model.IssueOperationID(owner, id) }).(model.OperationID)
	leftExpressionID := issue("expression-left", func(id identity.ContentID) (any, bool) { return model.IssueExpressionID(owner, id) }).(model.ExpressionID)
	rightExpressionID := issue("expression-right", func(id identity.ContentID) (any, bool) { return model.IssueExpressionID(owner, id) }).(model.ExpressionID)
	leftSeedPublicationExpressionID := issue("expression-left-seed-publication", func(id identity.ContentID) (any, bool) { return model.IssueExpressionID(owner, id) }).(model.ExpressionID)
	rightSeedPublicationExpressionID := issue("expression-right-seed-publication", func(id identity.ContentID) (any, bool) { return model.IssueExpressionID(owner, id) }).(model.ExpressionID)
	projectExpressionID := issue("expression-project", func(id identity.ContentID) (any, bool) { return model.IssueExpressionID(owner, id) }).(model.ExpressionID)
	payloadExpressionID := issue("expression-payload", func(id identity.ContentID) (any, bool) { return model.IssueExpressionID(owner, id) }).(model.ExpressionID)
	selectExpressionID := issue("expression-select", func(id identity.ContentID) (any, bool) { return model.IssueExpressionID(owner, id) }).(model.ExpressionID)
	groupExpressionID := issue("expression-group", func(id identity.ContentID) (any, bool) { return model.IssueExpressionID(owner, id) }).(model.ExpressionID)
	mergeExpressionID := issue("expression-merge", func(id identity.ContentID) (any, bool) { return model.IssueExpressionID(owner, id) }).(model.ExpressionID)
	mergeApplyExpressionID := issue("expression-merge-apply", func(id identity.ContentID) (any, bool) { return model.IssueExpressionID(owner, id) }).(model.ExpressionID)
	completeExpressionID := issue("expression-complete", func(id identity.ContentID) (any, bool) { return model.IssueExpressionID(owner, id) }).(model.ExpressionID)
	twoScalarApplyExpressionID := issue("expression-two-scalar-apply", func(id identity.ContentID) (any, bool) { return model.IssueExpressionID(owner, id) }).(model.ExpressionID)
	scalarSpanApplyExpressionID := issue("expression-scalar-complete-apply", func(id identity.ContentID) (any, bool) { return model.IssueExpressionID(owner, id) }).(model.ExpressionID)
	emptyExpressionID := issue("expression-empty", func(id identity.ContentID) (any, bool) { return model.IssueExpressionID(owner, id) }).(model.ExpressionID)
	emptyCompleteExpressionID := issue("expression-empty-complete", func(id identity.ContentID) (any, bool) { return model.IssueExpressionID(owner, id) }).(model.ExpressionID)
	scalarEmptyApplyExpressionID := issue("expression-scalar-empty-complete-apply", func(id identity.ContentID) (any, bool) { return model.IssueExpressionID(owner, id) }).(model.ExpressionID)
	correlatedApplyExpressionID := issue("expression-correlated-apply", func(id identity.ContentID) (any, bool) { return model.IssueExpressionID(owner, id) }).(model.ExpressionID)
	mixedPopulationApplyExpressionID := issue("expression-mixed-population-span-slots-apply", func(id identity.ContentID) (any, bool) { return model.IssueExpressionID(owner, id) }).(model.ExpressionID)
	sharedCompleteApplyExpressionID := issue("expression-shared-complete-apply", func(id identity.ContentID) (any, bool) { return model.IssueExpressionID(owner, id) }).(model.ExpressionID)
	sharedEmptyApplyExpressionID := issue("expression-shared-empty-complete-apply", func(id identity.ContentID) (any, bool) { return model.IssueExpressionID(owner, id) }).(model.ExpressionID)
	leftDependencyID := issue("dependency-left", func(id identity.ContentID) (any, bool) { return model.IssueDependencyID(owner, id) }).(model.DependencyID)
	rightDependencyID := issue("dependency-right", func(id identity.ContentID) (any, bool) { return model.IssueDependencyID(owner, id) }).(model.DependencyID)
	completeDependencyID := issue("dependency-complete", func(id identity.ContentID) (any, bool) { return model.IssueDependencyID(owner, id) }).(model.DependencyID)
	twoScalarApplyDependencyID := issue("dependency-two-scalar-apply", func(id identity.ContentID) (any, bool) { return model.IssueDependencyID(owner, id) }).(model.DependencyID)
	correlatedApplyDependencyID := issue("dependency-correlated-apply", func(id identity.ContentID) (any, bool) { return model.IssueDependencyID(owner, id) }).(model.DependencyID)
	mixedPopulationApplyDependencyID := issue("dependency-mixed-population-span-slots-apply", func(id identity.ContentID) (any, bool) { return model.IssueDependencyID(owner, id) }).(model.DependencyID)
	sharedCompleteApplyDependencyID := issue("dependency-shared-complete-apply", func(id identity.ContentID) (any, bool) { return model.IssueDependencyID(owner, id) }).(model.DependencyID)
	sharedEmptyApplyDependencyID := issue("dependency-shared-empty-complete-apply", func(id identity.ContentID) (any, bool) { return model.IssueDependencyID(owner, id) }).(model.DependencyID)
	leftRowA := issue("left-row-a", func(id identity.ContentID) (any, bool) { return model.IssueRowID(leftRelation, id) }).(model.RowID)
	leftRowB := issue("left-row-b", func(id identity.ContentID) (any, bool) { return model.IssueRowID(leftRelation, id) }).(model.RowID)
	rightRowA := issue("right-row-a", func(id identity.ContentID) (any, bool) { return model.IssueRowID(rightRelation, id) }).(model.RowID)
	rightRowB := issue("right-row-b", func(id identity.ContentID) (any, bool) { return model.IssueRowID(rightRelation, id) }).(model.RowID)
	applyRow := issue("apply-row", func(id identity.ContentID) (any, bool) { return model.IssueRowID(applyRelation, id) }).(model.RowID)
	leftDenominator, ok := model.NewDenominatorRef(leftRelation, leftKey)
	if !ok {
		t.Fatal("left denominator")
	}
	leftCoordinateDenominator, ok := model.NewDenominatorRef(leftRelation, leftCoordinateKey)
	if !ok {
		t.Fatal("left coordinate denominator")
	}
	rightDenominator, ok := model.NewDenominatorRef(rightRelation, rightKey)
	if !ok {
		t.Fatal("right denominator")
	}
	applyDenominator, ok := model.NewDenominatorRef(applyRelation, applyKey)
	if !ok {
		t.Fatal("apply denominator")
	}
	emptyDenominator, ok := model.NewDenominatorRef(emptyRelation, emptyKey)
	if !ok {
		t.Fatal("empty denominator")
	}
	exact, ok := model.NewCardinality(model.BoundedMany, 2)
	if !ok {
		t.Fatal("cardinality")
	}
	outcomes, ok := outcome.NewSet(outcome.Produced, outcome.NoCandidate, outcome.NoSelection, outcome.Opaque, outcome.Refused)
	if !ok {
		t.Fatal("outcomes")
	}
	leftSignature, ok := signature.Seal(signature.Spec{
		Identity: signature.Identity{Operation: leftOperationID, Version: 1},
		Fence:    signature.Fence{Owner: owner, Schema: schemaID},
		Outputs: []signature.Output{
			{Relation: leftRelation, Column: leftKeyA, Type: typeID, Presence: signature.ProducePresent, Denominator: leftDenominator},
			{Relation: leftRelation, Column: leftKeyB, Type: typeID, Presence: signature.ProducePresent, Denominator: leftDenominator},
			{Relation: leftRelation, Column: leftValueA, Type: typeID, Presence: signature.ProducePresent, Denominator: leftDenominator},
			{Relation: leftRelation, Column: leftValueB, Type: typeID, Presence: signature.ProducePresent, Denominator: leftDenominator},
		},
		Cardinality: exact, Outcomes: outcomes,
	})
	if !ok {
		t.Fatal("left signature")
	}
	rightSignature, ok := signature.Seal(signature.Spec{
		Identity: signature.Identity{Operation: rightOperationID, Version: 1},
		Fence:    signature.Fence{Owner: owner, Schema: schemaID},
		Outputs: []signature.Output{
			{Relation: rightRelation, Column: rightKeyA, Type: typeID, Presence: signature.ProducePresent, Denominator: rightDenominator},
			{Relation: rightRelation, Column: rightKeyB, Type: typeID, Presence: signature.ProducePresent, Denominator: rightDenominator},
			{Relation: rightRelation, Column: rightValueA, Type: typeID, Presence: signature.ProducePresent, Denominator: rightDenominator},
			{Relation: rightRelation, Column: rightValueB, Type: typeID, Presence: signature.ProducePresent, Denominator: rightDenominator},
		},
		Cardinality: exact, Outcomes: outcomes,
	})
	if !ok {
		t.Fatal("right signature")
	}
	scalarDelivery, ok := signature.NewScalarDelivery()
	if !ok {
		t.Fatal("scalar delivery")
	}
	twoScalarApplySignature, ok := signature.Seal(signature.Spec{
		Identity: signature.Identity{Operation: twoScalarApplyOperationID, Version: 1},
		Fence:    signature.Fence{Owner: owner, Schema: schemaID},
		Inputs: []signature.Input{
			{Relation: leftRelation, Column: leftValueA, Type: typeID, Presence: signature.RequirePresent, Delivery: scalarDelivery, Denominator: leftDenominator},
			{Relation: rightRelation, Column: rightValueA, Type: typeID, Presence: signature.RequirePresent, Delivery: scalarDelivery, Denominator: rightDenominator},
		},
		Outputs:     []signature.Output{{Relation: applyRelation, Column: applyValue, Type: typeID, Presence: signature.ProducePresent, Denominator: applyDenominator}},
		Cardinality: exact,
		Outcomes:    outcomes,
	})
	if !ok {
		t.Fatal("two scalar Apply signature")
	}
	completeDelivery, ok := signature.NewCompleteSpanDelivery(leftKey)
	if !ok {
		t.Fatal("complete span delivery")
	}
	scalarSpanApplySignature, ok := signature.Seal(signature.Spec{
		Identity: signature.Identity{Operation: scalarSpanApplyOperationID, Version: 1},
		Fence:    signature.Fence{Owner: owner, Schema: schemaID},
		Inputs: []signature.Input{
			{Relation: leftRelation, Column: leftValueA, Type: typeID, Presence: signature.RequirePresent, Delivery: scalarDelivery, Denominator: leftDenominator},
			{Relation: leftRelation, Column: leftValueB, Type: typeID, Presence: signature.RequirePresent, Delivery: completeDelivery, Denominator: leftDenominator},
		},
		Outputs:     []signature.Output{{Relation: applyRelation, Column: applyValue, Type: typeID, Presence: signature.ProducePresent, Denominator: applyDenominator}},
		Cardinality: exact,
		Outcomes:    outcomes,
	})
	if !ok {
		t.Fatal("scalar complete Apply signature")
	}
	emptyCompleteDelivery, ok := signature.NewCompleteSpanDelivery(emptyKey)
	if !ok {
		t.Fatal("empty complete span delivery")
	}
	scalarEmptyApplySignature, ok := signature.Seal(signature.Spec{
		Identity: signature.Identity{Operation: scalarEmptyApplyOperationID, Version: 1},
		Fence:    signature.Fence{Owner: owner, Schema: schemaID},
		Inputs: []signature.Input{
			{Relation: leftRelation, Column: leftValueA, Type: typeID, Presence: signature.RequirePresent, Delivery: scalarDelivery, Denominator: leftDenominator},
			{Relation: emptyRelation, Column: emptyValue, Type: typeID, Presence: signature.RequirePresent, Delivery: emptyCompleteDelivery, Denominator: emptyDenominator},
		},
		Outputs:     []signature.Output{{Relation: applyRelation, Column: applyValue, Type: typeID, Presence: signature.ProducePresent, Denominator: applyDenominator}},
		Cardinality: exact,
		Outcomes:    outcomes,
	})
	if !ok {
		t.Fatal("scalar empty-complete Apply signature")
	}
	correlatedRightDelivery, ok := signature.NewCompleteSpanDelivery(rightKey)
	if !ok {
		t.Fatal("correlated right complete delivery")
	}
	correlatedApplySignature, ok := signature.Seal(signature.Spec{
		Identity: signature.Identity{Operation: correlatedApplyOperationID, Version: 1},
		Fence:    signature.Fence{Owner: owner, Schema: schemaID},
		Inputs: []signature.Input{
			{Relation: leftRelation, Column: leftValueA, Type: typeID, Presence: signature.RequirePresent, Delivery: completeDelivery, Denominator: leftDenominator},
			{Relation: rightRelation, Column: rightValueA, Type: typeID, Presence: signature.RequirePresent, Delivery: correlatedRightDelivery, Denominator: rightDenominator},
		},
		Outputs:     []signature.Output{{Relation: applyRelation, Column: applyValue, Type: typeID, Presence: signature.ProducePresent, Denominator: applyDenominator}},
		Cardinality: exact,
		Outcomes:    outcomes,
	})
	if !ok {
		t.Fatal("correlated Apply signature")
	}
	mixedPopulationApplySignature, ok := signature.Seal(signature.Spec{
		Identity: signature.Identity{Operation: mixedPopulationApplyOperationID, Version: 1},
		Fence:    signature.Fence{Owner: owner, Schema: schemaID},
		Inputs: []signature.Input{
			{Relation: leftRelation, Column: leftValueA, Type: typeID, Presence: signature.RequirePresent, Delivery: scalarDelivery, Denominator: leftCoordinateDenominator},
			{Relation: rightRelation, Column: rightValueA, Type: typeID, Presence: signature.RequirePresent, Delivery: correlatedRightDelivery, Denominator: rightDenominator},
			{Relation: rightRelation, Column: rightValueB, Type: typeID, Presence: signature.RequirePresent, Delivery: correlatedRightDelivery, Denominator: rightDenominator},
			{Relation: rightRelation, Column: rightKeyA, Type: typeID, Presence: signature.RequirePresent, Delivery: correlatedRightDelivery, Denominator: rightDenominator},
		},
		Outputs:     []signature.Output{{Relation: applyRelation, Column: applyValue, Type: typeID, Presence: signature.ProducePresent, Denominator: applyDenominator}},
		Cardinality: exact,
		Outcomes:    outcomes,
	})
	if !ok {
		t.Fatal("mixed population span-slots Apply signature")
	}
	// The shared forms exercise the generic broadcast ABI: Q is the scalar
	// population child, while the second child is one globally closed Complete
	// vector with no Q projection or site-stamped rows.
	sharedCompleteApplySignature, ok := signature.Seal(signature.Spec{
		Identity: signature.Identity{Operation: sharedCompleteApplyOperationID, Version: 1},
		Fence:    signature.Fence{Owner: owner, Schema: schemaID},
		Inputs: []signature.Input{
			{Relation: leftRelation, Column: leftValueA, Type: typeID, Presence: signature.RequirePresent, Delivery: scalarDelivery, Denominator: leftCoordinateDenominator},
			{Relation: rightRelation, Column: rightValueA, Type: typeID, Presence: signature.RequirePresent, Delivery: correlatedRightDelivery, Denominator: rightDenominator},
		},
		Outputs:     []signature.Output{{Relation: applyRelation, Column: applyValue, Type: typeID, Presence: signature.ProducePresent, Denominator: applyDenominator}},
		Cardinality: exact,
		Outcomes:    outcomes,
	})
	if !ok {
		t.Fatal("shared complete Apply signature")
	}
	sharedEmptyApplySignature, ok := signature.Seal(signature.Spec{
		Identity: signature.Identity{Operation: sharedEmptyApplyOperationID, Version: 1},
		Fence:    signature.Fence{Owner: owner, Schema: schemaID},
		Inputs: []signature.Input{
			{Relation: leftRelation, Column: leftValueA, Type: typeID, Presence: signature.RequirePresent, Delivery: scalarDelivery, Denominator: leftCoordinateDenominator},
			{Relation: emptyRelation, Column: emptyValue, Type: typeID, Presence: signature.RequirePresent, Delivery: emptyCompleteDelivery, Denominator: emptyDenominator},
		},
		Outputs:     []signature.Output{{Relation: applyRelation, Column: applyValue, Type: typeID, Presence: signature.ProducePresent, Denominator: applyDenominator}},
		Cardinality: exact,
		Outcomes:    outcomes,
	})
	if !ok {
		t.Fatal("shared empty Complete Apply signature")
	}
	leftRelationRef, ok := plan.NewRelationRef(leftRelation)
	if !ok {
		t.Fatal("left relation reference")
	}
	rightRelationRef, ok := plan.NewRelationRef(rightRelation)
	if !ok {
		t.Fatal("right relation reference")
	}
	emptyRelationRef, ok := plan.NewRelationRef(emptyRelation)
	if !ok {
		t.Fatal("empty relation reference")
	}
	payloadExpression := schemaalgebra.NewJoin(
		schemaalgebra.NewInput(leftRelation),
		schemaalgebra.NewInput(rightRelation),
		schemaalgebra.NewJoinContract(
			[]model.ColumnID{leftValueA, leftValueB},
			[]model.ColumnID{rightValueA, rightValueB},
		),
	)
	// These zero-input Publish specimens are the exact schema authority for
	// the direct seed applications below. The fixture seeds both input
	// relations through the public Apply -> Publish boundary, so their
	// owner-issued Present outputs must certify the corresponding ascent at
	// mount rather than relying on a signature-wide algebra default.
	leftSeedPublication := schemaalgebra.NewPublish(
		schemaalgebra.NewApply(nil, schemaalgebra.NewApplyContract(leftSignature.Identity(), nil, schemaalgebra.OwnerNamed())),
		schemaalgebra.NewPublishContract(leftRelation, leftKey, leftKeyA, leftKeyB, leftValueA, leftValueB),
	)
	rightSeedPublication := schemaalgebra.NewPublish(
		schemaalgebra.NewApply(nil, schemaalgebra.NewApplyContract(rightSignature.Identity(), nil, schemaalgebra.OwnerNamed())),
		schemaalgebra.NewPublishContract(rightRelation, rightKey, rightKeyA, rightKeyB, rightValueA, rightValueB),
	)
	projectExpression := schemaalgebra.NewProject(
		schemaalgebra.NewInput(leftRelation),
		schemaalgebra.NewProjectContract(rightRelation, []schemaalgebra.ColumnMapping{
			schemaalgebra.NewColumnMapping(leftKeyA, rightKeyA),
			schemaalgebra.NewColumnMapping(leftKeyB, rightKeyB),
			schemaalgebra.NewColumnMapping(leftValueA, rightValueA),
			schemaalgebra.NewColumnMapping(leftValueB, rightValueB),
		}, rightKey),
	)
	selectExpression := schemaalgebra.NewSelect(
		schemaalgebra.NewInput(leftRelation),
		schemaalgebra.NewSelectContract(schemaalgebra.SelectByScope, leftScopeID),
	)
	groupExpression := schemaalgebra.NewGroup(
		schemaalgebra.NewInput(leftRelation),
		schemaalgebra.NewGroupContract(leftKey, exact),
	)
	mergeExpression := schemaalgebra.NewMerge(
		[]schemaalgebra.Expression{schemaalgebra.NewInput(leftRelation), schemaalgebra.NewInput(leftRelation)},
		schemaalgebra.NewMergeContract(leftKey),
	)
	completeExpression := schemaalgebra.NewComplete(schemaalgebra.NewInput(leftRelation), leftDenominator)
	twoScalarApplyExpression := schemaalgebra.NewApply(
		[]schemaalgebra.Expression{schemaalgebra.NewInput(leftRelation), schemaalgebra.NewInput(rightRelation)},
		schemaalgebra.NewApplyContract(twoScalarApplySignature.Identity(), []schemaalgebra.SlotSource{
			schemaalgebra.NewSlotSource(0, 2),
			schemaalgebra.NewSlotSource(1, 2),
		}, schemaalgebra.OwnerNamed()),
	)
	mergeApplyExpression := schemaalgebra.NewMerge(
		[]schemaalgebra.Expression{
			twoScalarApplyExpression,
			schemaalgebra.NewColumnProject(
				schemaalgebra.NewInput(applyRelation),
				schemaalgebra.NewColumnProjectContract([]schemaalgebra.ColumnSlot{schemaalgebra.NewColumnSlot(applyValue, 0)}),
			),
		},
		schemaalgebra.NewMergeContract(applyKey),
	)
	twoScalarApplyObservationCardinality, ok := model.NewCardinality(model.ExactlyOne, 0)
	if !ok {
		t.Fatal("two scalar Apply observation cardinality")
	}
	twoScalarApplyObservation := schemaalgebra.NewObservationContract(
		twoScalarApplyDependencyID,
		twoScalarApplySignature.Identity(),
		schemaalgebra.NewObservationSource(0, 0, 0),
		leftDenominator,
		schemaalgebra.NewObservationOutput(applyValue, typeID, applyDenominator, twoScalarApplyObservationCardinality),
	)
	scalarSpanApplyExpression := schemaalgebra.NewApply(
		[]schemaalgebra.Expression{schemaalgebra.NewInput(leftRelation), completeExpression},
		schemaalgebra.NewApplyContract(scalarSpanApplySignature.Identity(), []schemaalgebra.SlotSource{
			schemaalgebra.NewSlotSource(0, 2),
			schemaalgebra.NewSlotSource(1, 3),
		}, schemaalgebra.OwnerNamed()),
	)
	emptyCompleteExpression := schemaalgebra.NewComplete(schemaalgebra.NewInput(emptyRelation), emptyDenominator)
	scalarEmptyApplyExpression := schemaalgebra.NewApply(
		[]schemaalgebra.Expression{schemaalgebra.NewInput(leftRelation), emptyCompleteExpression},
		schemaalgebra.NewApplyContract(scalarEmptyApplySignature.Identity(), []schemaalgebra.SlotSource{
			schemaalgebra.NewSlotSource(0, 2),
			schemaalgebra.NewSlotSource(1, 0),
		}, schemaalgebra.OwnerNamed()),
	)
	correlatedRight := schemaalgebra.NewComplete(
		schemaalgebra.NewSelect(
			schemaalgebra.NewInput(rightRelation),
			schemaalgebra.NewSelectContract(schemaalgebra.SelectByScope, rightScopeID),
		),
		rightDenominator,
	)
	correlatedLeft := schemaalgebra.NewComplete(
		schemaalgebra.NewSelect(
			schemaalgebra.NewInput(leftRelation),
			schemaalgebra.NewSelectContract(schemaalgebra.SelectByScope, leftScopeID),
		),
		leftDenominator,
	)
	correlation := schemaalgebra.NewApplyCorrelation(
		leftCoordinateDenominator,
		leftValueA,
		typeID,
		[][]model.ColumnID{{leftValueA}, {rightValueA}},
	)
	correlatedContract, ok := schemaalgebra.NewCorrelatedApplyContract(
		correlatedApplySignature.Identity(),
		[]schemaalgebra.SlotSource{schemaalgebra.NewSlotSource(0, 2), schemaalgebra.NewSlotSource(1, 2)},
		correlation,
		schemaalgebra.OwnerNamed(),
	)
	if !ok {
		t.Fatal("correlated Apply contract")
	}
	correlatedApplyExpression := schemaalgebra.NewApply(
		[]schemaalgebra.Expression{correlatedLeft, correlatedRight},
		correlatedContract,
	)
	mixedPopulationCorrelation := schemaalgebra.NewApplyCorrelation(
		leftCoordinateDenominator,
		leftValueA,
		typeID,
		[][]model.ColumnID{{leftValueA}, {rightValueA}},
	)
	mixedPopulationContract, ok := schemaalgebra.NewCorrelatedApplyContract(
		mixedPopulationApplySignature.Identity(),
		[]schemaalgebra.SlotSource{
			schemaalgebra.NewSlotSource(0, 2),
			schemaalgebra.NewSlotSource(1, 2),
			schemaalgebra.NewSlotSource(1, 3),
			schemaalgebra.NewSlotSource(1, 0),
		},
		mixedPopulationCorrelation,
		schemaalgebra.OwnerNamed(),
	)
	if !ok {
		t.Fatal("mixed population span-slots Apply contract")
	}
	mixedPopulationApplyExpression := schemaalgebra.NewApply(
		[]schemaalgebra.Expression{schemaalgebra.NewInput(leftRelation), correlatedRight},
		mixedPopulationContract,
	)
	sharedCorrelation := schemaalgebra.NewApplyCorrelation(
		leftCoordinateDenominator,
		leftValueA,
		typeID,
		[][]model.ColumnID{{leftValueA}, {}},
	)
	sharedCompleteContract, ok := schemaalgebra.NewCorrelatedApplyContract(
		sharedCompleteApplySignature.Identity(),
		[]schemaalgebra.SlotSource{
			schemaalgebra.NewSlotSource(0, 2),
			schemaalgebra.NewSlotSource(1, 2),
		},
		sharedCorrelation,
		schemaalgebra.OwnerNamed(),
	)
	if !ok {
		t.Fatal("shared complete Apply contract")
	}
	sharedCompleteApplyExpression := schemaalgebra.NewApply(
		[]schemaalgebra.Expression{schemaalgebra.NewInput(leftRelation), correlatedRight},
		sharedCompleteContract,
	)
	sharedEmptyComplete := schemaalgebra.NewComplete(
		schemaalgebra.NewSelect(
			schemaalgebra.NewInput(emptyRelation),
			schemaalgebra.NewSelectContract(schemaalgebra.SelectByScope, leftScopeID),
		),
		emptyDenominator,
	)
	sharedEmptyContract, ok := schemaalgebra.NewCorrelatedApplyContract(
		sharedEmptyApplySignature.Identity(),
		[]schemaalgebra.SlotSource{
			schemaalgebra.NewSlotSource(0, 2),
			schemaalgebra.NewSlotSource(1, 0),
		},
		sharedCorrelation,
		schemaalgebra.OwnerNamed(),
	)
	if !ok {
		t.Fatal("shared empty Complete Apply contract")
	}
	sharedEmptyApplyExpression := schemaalgebra.NewApply(
		[]schemaalgebra.Expression{schemaalgebra.NewInput(leftRelation), sharedEmptyComplete},
		sharedEmptyContract,
	)
	// Scope formulas are owner-issued schema values. Their physical support
	// masks below use the same Boolean shape, so cofiber can prove the exact
	// schema-to-guard translation at bootstrap without a neutral adapter.
	leftScopeAtomID := issue("scope-atom-left", func(id identity.ContentID) (any, bool) { return id, true }).(identity.ContentID)
	rightScopeAtomID := issue("scope-atom-right", func(id identity.ContentID) (any, bool) { return id, true }).(identity.ContentID)
	leftScopeAtom, ok := region.NewAtom(leftScopeAtomID)
	if !ok {
		t.Fatal("left scope atom")
	}
	rightScopeAtom, ok := region.NewAtom(rightScopeAtomID)
	if !ok {
		t.Fatal("right scope atom")
	}
	leftScopeRegion, ok := region.FromAtom(leftScopeAtom)
	if !ok {
		t.Fatal("left scope region")
	}
	rightScopeRegion, ok := scopeUnion(leftScopeAtom, rightScopeAtom)
	if !ok {
		t.Fatal("right scope region")
	}
	builder := plan.NewBuilder(schemaID)
	capability, capabilityOK := model.NewAscendingCapability(typeID)
	if !capabilityOK || !builder.AddTypeCapability(capability) {
		t.Fatal("ascending type capability")
	}
	if !builder.AddRelation(model.DefineRelationSchema(leftRelation, []model.ColumnID{leftKeyA, leftKeyB, leftValueA, leftValueB}, []model.KeyID{leftKey, leftCoordinateKey}, leftScopeID)) ||
		!builder.AddRelation(model.DefineRelationSchema(rightRelation, []model.ColumnID{rightKeyA, rightKeyB, rightValueA, rightValueB}, []model.KeyID{rightKey}, rightScopeID)) ||
		!builder.AddRelation(model.DefineRelationSchema(applyRelation, []model.ColumnID{applyValue, applyFact}, []model.KeyID{applyKey}, leftScopeID)) ||
		!builder.AddRelation(model.DefineRelationSchema(emptyRelation, []model.ColumnID{emptyValue}, []model.KeyID{emptyKey}, leftScopeID)) ||
		!builder.AddColumn(model.DefineColumnSchema(leftKeyA, typeID)) || !builder.AddColumn(model.DefineColumnSchema(leftKeyB, typeID)) ||
		!builder.AddColumn(model.DefineColumnSchema(leftValueA, typeID)) || !builder.AddColumn(model.DefineColumnSchema(leftValueB, typeID)) ||
		!builder.AddColumn(model.DefineColumnSchema(rightKeyA, typeID)) || !builder.AddColumn(model.DefineColumnSchema(rightKeyB, typeID)) ||
		!builder.AddColumn(model.DefineColumnSchema(rightValueA, typeID)) || !builder.AddColumn(model.DefineColumnSchema(rightValueB, typeID)) ||
		!builder.AddColumn(model.DefineColumnSchema(applyValue, typeID)) || !builder.AddColumn(model.DefineColumnSchema(applyFact, typeID)) ||
		!builder.AddColumn(model.DefineColumnSchema(emptyValue, typeID)) ||
		!builder.AddKey(model.DefineKeySchema(leftKey, []model.ColumnID{leftKeyA, leftKeyB})) ||
		!builder.AddKey(model.DefineKeySchema(leftCoordinateKey, []model.ColumnID{leftValueA})) ||
		!builder.AddKey(model.DefineKeySchema(rightKey, []model.ColumnID{rightKeyA, rightKeyB})) ||
		!builder.AddKey(model.DefineKeySchema(applyKey, []model.ColumnID{applyValue})) ||
		!builder.AddKey(model.DefineKeySchema(emptyKey, []model.ColumnID{emptyValue})) ||
		!builder.AddScope(model.DefineScopeSchema(leftScopeID, nil, leftScopeRegion)) || !builder.AddScope(model.DefineScopeSchema(rightScopeID, nil, rightScopeRegion)) ||
		!builder.AddExpression(plan.DefineExpressionRef(leftExpressionID, schemaalgebra.NewInput(leftRelation))) ||
		!builder.AddExpression(plan.DefineExpressionRef(rightExpressionID, schemaalgebra.NewInput(rightRelation))) ||
		!builder.AddExpression(plan.DefineExpressionRef(leftSeedPublicationExpressionID, leftSeedPublication)) ||
		!builder.AddExpression(plan.DefineExpressionRef(rightSeedPublicationExpressionID, rightSeedPublication)) ||
		!builder.AddExpression(plan.DefineExpressionRef(projectExpressionID, projectExpression)) ||
		!builder.AddExpression(plan.DefineExpressionRef(payloadExpressionID, payloadExpression)) ||
		!builder.AddExpression(plan.DefineExpressionRef(selectExpressionID, selectExpression)) ||
		!builder.AddExpression(plan.DefineExpressionRef(groupExpressionID, groupExpression)) ||
		!builder.AddExpression(plan.DefineExpressionRef(mergeExpressionID, mergeExpression)) ||
		!builder.AddExpression(plan.DefineExpressionRef(mergeApplyExpressionID, mergeApplyExpression)) ||
		!builder.AddExpression(plan.DefineExpressionRef(completeExpressionID, completeExpression)) ||
		!builder.AddExpression(plan.DefineExpressionRef(twoScalarApplyExpressionID, twoScalarApplyExpression)) ||
		!builder.AddExpression(plan.DefineExpressionRef(scalarSpanApplyExpressionID, scalarSpanApplyExpression)) ||
		!builder.AddExpression(plan.DefineExpressionRef(emptyExpressionID, schemaalgebra.NewInput(emptyRelation))) ||
		!builder.AddExpression(plan.DefineExpressionRef(emptyCompleteExpressionID, emptyCompleteExpression)) ||
		!builder.AddExpression(plan.DefineExpressionRef(scalarEmptyApplyExpressionID, scalarEmptyApplyExpression)) ||
		!builder.AddExpression(plan.DefineExpressionRef(correlatedApplyExpressionID, correlatedApplyExpression)) ||
		!builder.AddExpression(plan.DefineExpressionRef(mixedPopulationApplyExpressionID, mixedPopulationApplyExpression)) ||
		!builder.AddExpression(plan.DefineExpressionRef(sharedCompleteApplyExpressionID, sharedCompleteApplyExpression)) ||
		!builder.AddExpression(plan.DefineExpressionRef(sharedEmptyApplyExpressionID, sharedEmptyApplyExpression)) ||
		!builder.AddDependency(plan.DefineDependency(leftDependencyID, leftExpressionID, []plan.RelationRef{leftRelationRef}, nil, "left-input")) ||
		!builder.AddSCC(plan.DefineSCC([]plan.DependencyRef{plan.DefineDependencyRef(leftDependencyID)}, nil, plan.DefineRecurrence(plan.Acyclic, nil))) ||
		!builder.AddDependency(plan.DefineDependency(rightDependencyID, rightExpressionID, []plan.RelationRef{rightRelationRef}, nil, "right-input")) ||
		!builder.AddSCC(plan.DefineSCC([]plan.DependencyRef{plan.DefineDependencyRef(rightDependencyID)}, nil, plan.DefineRecurrence(plan.Acyclic, nil))) ||
		!builder.AddDependency(plan.DefineDependency(completeDependencyID, completeExpressionID, []plan.RelationRef{leftRelationRef}, nil, "left-complete")) ||
		!builder.AddSCC(plan.DefineSCC([]plan.DependencyRef{plan.DefineDependencyRef(completeDependencyID)}, nil, plan.DefineRecurrence(plan.Acyclic, nil))) ||
		!builder.AddDependency(plan.DefineDependency(twoScalarApplyDependencyID, twoScalarApplyExpressionID, []plan.RelationRef{leftRelationRef, rightRelationRef}, nil, "two-scalar-apply")) ||
		!builder.AddSCC(plan.DefineSCC([]plan.DependencyRef{plan.DefineDependencyRef(twoScalarApplyDependencyID)}, nil, plan.DefineRecurrence(plan.Acyclic, nil))) ||
		!builder.AddDependency(plan.DefineDependency(correlatedApplyDependencyID, correlatedApplyExpressionID, []plan.RelationRef{leftRelationRef, rightRelationRef}, nil, "correlated-apply")) ||
		!builder.AddSCC(plan.DefineSCC([]plan.DependencyRef{plan.DefineDependencyRef(correlatedApplyDependencyID)}, nil, plan.DefineRecurrence(plan.Acyclic, nil))) ||
		!builder.AddDependency(plan.DefineDependency(mixedPopulationApplyDependencyID, mixedPopulationApplyExpressionID, []plan.RelationRef{leftRelationRef, rightRelationRef}, nil, "mixed-population-span-slots-apply")) ||
		!builder.AddSCC(plan.DefineSCC([]plan.DependencyRef{plan.DefineDependencyRef(mixedPopulationApplyDependencyID)}, nil, plan.DefineRecurrence(plan.Acyclic, nil))) ||
		!builder.AddDependency(plan.DefineDependency(sharedCompleteApplyDependencyID, sharedCompleteApplyExpressionID, []plan.RelationRef{leftRelationRef, rightRelationRef}, nil, "shared-complete-apply")) ||
		!builder.AddSCC(plan.DefineSCC([]plan.DependencyRef{plan.DefineDependencyRef(sharedCompleteApplyDependencyID)}, nil, plan.DefineRecurrence(plan.Acyclic, nil))) ||
		!builder.AddDependency(plan.DefineDependency(sharedEmptyApplyDependencyID, sharedEmptyApplyExpressionID, []plan.RelationRef{leftRelationRef, emptyRelationRef}, nil, "shared-empty-complete-apply")) ||
		!builder.AddSCC(plan.DefineSCC([]plan.DependencyRef{plan.DefineDependencyRef(sharedEmptyApplyDependencyID)}, nil, plan.DefineRecurrence(plan.Acyclic, nil))) ||
		!builder.AddSignature(leftSignature) || !builder.AddSignature(rightSignature) || !builder.AddSignature(twoScalarApplySignature) || !builder.AddSignature(scalarSpanApplySignature) || !builder.AddSignature(scalarEmptyApplySignature) || !builder.AddSignature(correlatedApplySignature) || !builder.AddSignature(mixedPopulationApplySignature) || !builder.AddSignature(sharedCompleteApplySignature) || !builder.AddSignature(sharedEmptyApplySignature) || !builder.AddObservation(twoScalarApplyObservation) {
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
	manager, err := guard.New([]guard.Atom{1, 2})
	if err != nil {
		t.Fatal(err)
	}
	leftMask, rightExtent, _, disjointMask, disjointRightMask := buildMasks(t, manager)
	inv := &inventory{
		fence:        fence,
		relations:    map[model.RelationID]uint64{leftRelation: 1, rightRelation: 2, applyRelation: 3, emptyRelation: 4},
		columns:      map[model.ColumnID]uint64{leftKeyA: 1, leftKeyB: 2, leftValueA: 3, leftValueB: 4, rightKeyA: 5, rightKeyB: 6, rightValueA: 7, rightValueB: 8, applyValue: 9, applyFact: 10, emptyValue: 11},
		keys:         map[model.KeyID]uint64{leftKey: 1, leftCoordinateKey: 5, rightKey: 2, applyKey: 3, emptyKey: 4},
		scopes:       map[model.ScopeID]uint64{leftScopeID: 1, rightScopeID: 2},
		expressions:  map[model.ExpressionID]uint64{leftExpressionID: 1, rightExpressionID: 2, leftSeedPublicationExpressionID: 3, rightSeedPublicationExpressionID: 4, projectExpressionID: 5, payloadExpressionID: 6, selectExpressionID: 7, groupExpressionID: 8, mergeExpressionID: 9, completeExpressionID: 10, twoScalarApplyExpressionID: 11, scalarSpanApplyExpressionID: 12, emptyExpressionID: 13, emptyCompleteExpressionID: 14, scalarEmptyApplyExpressionID: 15, mergeApplyExpressionID: 16, correlatedApplyExpressionID: 17, mixedPopulationApplyExpressionID: 18, sharedCompleteApplyExpressionID: 19, sharedEmptyApplyExpressionID: 20},
		dependencies: map[model.DependencyID]uint64{leftDependencyID: 1, rightDependencyID: 2, completeDependencyID: 3, twoScalarApplyDependencyID: 4, correlatedApplyDependencyID: 5, mixedPopulationApplyDependencyID: 6, sharedCompleteApplyDependencyID: 7, sharedEmptyApplyDependencyID: 8},
		denominators: map[model.DenominatorRef]witness.DenominatorEvidence{},
		partitions:   map[uint32]map[model.RowID]witness.DenominatorEvidence{},
	}
	leftEvidence, ok := witness.NewDenominatorEvidence([]model.RowID{leftRowA, leftRowB}, content("left-denominator"))
	if !ok {
		t.Fatal("left evidence")
	}
	rightEvidence, ok := witness.NewDenominatorEvidence([]model.RowID{rightRowA, rightRowB}, content("right-denominator"))
	if !ok {
		t.Fatal("right evidence")
	}
	applyEvidence, ok := witness.NewDenominatorEvidence([]model.RowID{applyRow}, content("apply-denominator"))
	if !ok {
		t.Fatal("apply evidence")
	}
	emptyEvidence, ok := witness.NewDenominatorEvidence([]model.RowID{}, content("empty-denominator"))
	if !ok {
		t.Fatal("empty evidence")
	}
	leftCoordinateEvidence, ok := witness.NewDenominatorEvidence([]model.RowID{leftRowA, leftRowB}, content("left-coordinate-denominator"))
	if !ok {
		t.Fatal("left coordinate evidence")
	}
	partitionEmpty, ok := witness.NewDenominatorEvidence([]model.RowID{}, content("partition-empty"))
	if !ok {
		t.Fatal("partition empty evidence")
	}
	leftPartitionA, ok := witness.NewDenominatorEvidence([]model.RowID{leftRowA}, content("left-partition-a"))
	if !ok {
		t.Fatal("left partition a evidence")
	}
	rightPartitionA, ok := witness.NewDenominatorEvidence([]model.RowID{rightRowA}, content("right-partition-a"))
	if !ok {
		t.Fatal("right partition a evidence")
	}
	rightPartitionB, ok := witness.NewDenominatorEvidence([]model.RowID{rightRowB}, content("right-partition-b"))
	if !ok {
		t.Fatal("right partition b evidence")
	}
	inv.denominators[leftDenominator] = leftEvidence
	inv.denominators[leftCoordinateDenominator] = leftCoordinateEvidence
	inv.denominators[rightDenominator] = rightEvidence
	inv.denominators[applyDenominator] = applyEvidence
	inv.denominators[emptyDenominator] = emptyEvidence
	inv.partitions[0] = map[model.RowID]witness.DenominatorEvidence{leftRowA: leftPartitionA, leftRowB: partitionEmpty}
	inv.partitions[1] = map[model.RowID]witness.DenominatorEvidence{leftRowA: rightPartitionA, leftRowB: rightPartitionB}
	if mountByte == 0xF3 {
		// The second right row shares q0's lookup coordinate, while the
		// authenticated q0 posting names only rightRowA. q1 is intentionally
		// empty so this variant isolates exact posting filtering.
		inv.partitions[1][leftRowB] = partitionEmpty
	}
	lineageOwner := mustIssue(t, "lineage-owner", model.IssueOwnerID)
	lineageFactory, ok := lineage.NewFactory(lineageOwner)
	if !ok {
		t.Fatal("lineage factory")
	}
	leftWorker, rightWorker := &worker{}, &worker{}
	twoScalarApplyWorker, scalarSpanApplyWorker, scalarEmptyApplyWorker, correlatedApplyWorker, mixedPopulationApplyWorker, sharedCompleteApplyWorker, sharedEmptyApplyWorker := &applyWorker{}, &applyWorker{}, &applyWorker{}, &applyWorker{}, &applyWorker{}, &applyWorker{}, &applyWorker{}
	factory := bindingFactory{bindings: map[signature.Identity]binding.Binding{
		leftSignature.Identity():                 operationBinding{operation: leftSignature, worker: leftWorker},
		rightSignature.Identity():                operationBinding{operation: rightSignature, worker: rightWorker},
		twoScalarApplySignature.Identity():       operationBinding{operation: twoScalarApplySignature, worker: twoScalarApplyWorker},
		scalarSpanApplySignature.Identity():      operationBinding{operation: scalarSpanApplySignature, worker: scalarSpanApplyWorker},
		scalarEmptyApplySignature.Identity():     operationBinding{operation: scalarEmptyApplySignature, worker: scalarEmptyApplyWorker},
		correlatedApplySignature.Identity():      operationBinding{operation: correlatedApplySignature, worker: correlatedApplyWorker},
		mixedPopulationApplySignature.Identity(): operationBinding{operation: mixedPopulationApplySignature, worker: mixedPopulationApplyWorker},
		sharedCompleteApplySignature.Identity():  operationBinding{operation: sharedCompleteApplySignature, worker: sharedCompleteApplyWorker},
		sharedEmptyApplySignature.Identity():     operationBinding{operation: sharedEmptyApplySignature, worker: sharedEmptyApplyWorker},
	}}
	algebras := algebraRegistry{
		value:    algebra{typeID: typeID},
		equality: tokenEquality{typeID: typeID},
	}
	mounted, ok := witness.Specialize(certificateValue, inv, factory, algebras, lineageFactory)
	if !ok || !mounted.Available() {
		t.Fatal("mounted")
	}
	// Lowering owns only the physical extent of each owner-issued atom. The
	// right relation's scope is the neutral union of both atoms, so its extent
	// is deliberately the raw second literal here; lower evaluates the union
	// rather than receiving a table of whole-region formulas.
	lowering, ok := lower.New(manager, map[identity.ContentID]support.Mask{
		leftScopeAtomID:  leftMask,
		rightScopeAtomID: rightExtent,
	})
	if !ok || !lowering.Available() {
		t.Fatal("cofiber lowering")
	}
	lowerFactory, ok := lower.NewFactory(mounted.RuntimeFence().Mount(), lowering)
	if !ok || !lowerFactory.Available() {
		t.Fatal("cofiber lowering factory")
	}
	view, ok := lowerFactory.Bind(mounted)
	if !ok || !view.Available() {
		t.Fatal("geometry")
	}
	base, ok := database.Bootstrap(mounted, view)
	if !ok || !base.Available() {
		t.Fatal("database")
	}
	scratch := store.NewReadScratch(manager)
	if scratch == nil || !scratch.Available() {
		t.Fatal("scratch")
	}
	door, ok := publish.New(mounted, view)
	if !ok || !door.Available() {
		t.Fatal("publish door")
	}
	leftDeclared, ok := mounted.Scope(leftScopeID)
	if !ok {
		t.Fatal("left scope")
	}
	rightDeclared, ok := mounted.Scope(rightScopeID)
	if !ok {
		t.Fatal("right scope")
	}
	overlapLeft, ok := view.Scope(mustScopeToken(t, mounted, leftDeclared))
	if !ok {
		t.Fatal("left normalized scope")
	}
	overlapRight, ok := view.Scope(mustScopeToken(t, mounted, rightDeclared))
	if !ok {
		t.Fatal("right normalized scope")
	}
	disjointLeft, ok := view.Normalize(leftMask)
	if !ok {
		t.Fatal("disjoint left")
	}
	disjointRight, ok := view.Normalize(disjointMask)
	if !ok {
		t.Fatal("disjoint right")
	}
	disjointQ, ok := view.Normalize(disjointRightMask)
	if !ok {
		t.Fatal("disjoint q")
	}
	leftValues := issueValues(t, mounted, typeID, "left")
	rightValues := issueValues(t, mounted, typeID, "right")
	// The first declared correspondence vector is a match. The second differs only
	// in its second component, proving that the physical lookup is genuinely
	// vector-valued rather than a first-column shortcut.
	leftValues[0][2] = mustValue(t, mounted, typeID, "match-value-a")
	leftValues[0][3] = mustValue(t, mounted, typeID, "match-value-b")
	rightValues[0][2] = leftValues[0][2]
	rightValues[0][3] = leftValues[0][3]
	leftValues[1][2] = mustValue(t, mounted, typeID, "miss-value-a")
	leftValues[1][3] = mustValue(t, mounted, typeID, "miss-value-b-left")
	rightValues[1][2] = leftValues[1][2]
	rightValues[1][3] = mustValue(t, mounted, typeID, "miss-value-b-right")
	if mountByte == 0xF3 {
		rightValues[1][2] = rightValues[0][2]
	}
	if mountByte == 0xF4 {
		// The mixed runtime law addresses three distinct right columns through
		// one Complete child. Give each column the population coordinate while
		// retaining distinct row keys; this makes the child posting a genuine
		// multi-slot source without changing the historical fixture mounts.
		for row := range rightValues {
			rightValues[row][0] = leftValues[row][2]
			rightValues[row][2] = leftValues[row][2]
			rightValues[row][3] = leftValues[row][2]
		}
	}
	leftSecondScope := overlapLeft
	if mountByte == 0xF1 {
		leftSecondScope = disjointQ
	}
	prepareWorker(t, mounted, leftWorker, leftSignature, leftDenominator, overlapLeft, leftSecondScope, leftRowA, leftRowB, []model.ColumnID{leftKeyA, leftKeyB, leftValueA, leftValueB}, leftValues)
	prepareWorker(t, mounted, rightWorker, rightSignature, rightDenominator, overlapRight, overlapRight, rightRowA, rightRowB, []model.ColumnID{rightKeyA, rightKeyB, rightValueA, rightValueB}, rightValues)
	leftRoot, leftDelta := commitRows(t, mounted, door, base, scratch, leftWorker, leftSignature.Identity(), overlapLeft, leftSecondScope)
	bothRoot, rightDelta := commitRows(t, mounted, door, leftRoot, scratch, rightWorker, rightSignature.Identity(), overlapRight, overlapRight)
	leftKeyLayout := mustKeyLayout(t, mounted, leftKey)
	rightKeyLayout := mustKeyLayout(t, mounted, rightKey)
	leftValueLayout := mustVectorLayout(t, mounted, leftRelation, []model.ColumnID{leftValueA, leftValueB})
	rightValueLayout := mustVectorLayout(t, mounted, rightRelation, []model.ColumnID{rightValueA, rightValueB})
	applyFactLayout := mustVectorLayout(t, mounted, applyRelation, []model.ColumnID{applyValue, applyFact})
	inputLayout := mustRelationLayout(t, mounted, leftRelation)
	rightInputLayout := mustRelationLayout(t, mounted, rightRelation)
	return Fixture{
		mounted: mounted, view: view, door: door, base: base, left: leftRoot, both: bothRoot, leftDelta: leftDelta, rightDelta: rightDelta, scratch: scratch, seedType: typeID, seedAscent: certificateValue.AlgebraRequirements(),
		leftRelation: leftRelation, rightRelation: rightRelation, applyRelation: applyRelation, emptyRelation: emptyRelation, leftKey: leftKey, leftCoordinateKey: leftCoordinateKey, rightKey: rightKey, applyKey: applyKey, emptyKey: emptyKey, applyValue: applyValue, applyFact: applyFact,
		leftKeys: [2]model.ColumnID{leftKeyA, leftKeyB}, rightKeys: [2]model.ColumnID{rightKeyA, rightKeyB},
		leftPayload: [2]model.ColumnID{leftValueA, leftValueB}, rightPayload: [2]model.ColumnID{rightValueA, rightValueB},
		leftRows: [2]model.RowID{leftRowA, leftRowB}, rightRows: [2]model.RowID{rightRowA, rightRowB}, applyRow: applyRow,
		leftOperation: leftSignature.Identity(), rightOperation: rightSignature.Identity(), leftExpression: leftExpressionID, rightExpression: rightExpressionID, projectExpression: projectExpressionID, payloadExpression: payloadExpressionID, selectExpression: selectExpressionID, groupExpression: groupExpressionID, mergeExpression: mergeExpressionID, mergeApplyExpression: mergeApplyExpressionID, completeExpression: completeExpressionID, twoScalarApplyExpression: twoScalarApplyExpressionID, twoScalarApplyObservation: twoScalarApplyObservation.Digest(), scalarSpanApplyExpression: scalarSpanApplyExpressionID, emptyExpression: emptyExpressionID, emptyCompleteExpression: emptyCompleteExpressionID, scalarEmptyApplyExpression: scalarEmptyApplyExpressionID, correlatedApplyExpression: correlatedApplyExpressionID, mixedPopulationApplyExpression: mixedPopulationApplyExpressionID, sharedCompleteApplyExpression: sharedCompleteApplyExpressionID, sharedEmptyApplyExpression: sharedEmptyApplyExpressionID, leftDependency: leftDependencyID, rightDependency: rightDependencyID, completeDependency: completeDependencyID, twoScalarApplyDependency: twoScalarApplyDependencyID, correlatedApplyDependency: correlatedApplyDependencyID, mixedPopulationApplyDependency: mixedPopulationApplyDependencyID, sharedCompleteApplyDependency: sharedCompleteApplyDependencyID, sharedEmptyApplyDependency: sharedEmptyApplyDependencyID, twoScalarApplyWorker: twoScalarApplyWorker, scalarSpanApplyWorker: scalarSpanApplyWorker, scalarEmptyApplyWorker: scalarEmptyApplyWorker, correlatedApplyWorker: correlatedApplyWorker, mixedPopulationApplyWorker: mixedPopulationApplyWorker, sharedCompleteApplyWorker: sharedCompleteApplyWorker, sharedEmptyApplyWorker: sharedEmptyApplyWorker,
		leftKeyLayout: leftKeyLayout, rightKeyLayout: rightKeyLayout, leftValueLayout: leftValueLayout, rightValueLayout: rightValueLayout, inputLayout: inputLayout, rightInputLayout: rightInputLayout, applyFactLayout: applyFactLayout,
		scopes: scopeSet{overlapLeft: overlapLeft, overlapRight: overlapRight, disjointLeft: disjointLeft, disjointRight: disjointRight},
	}
}

// scopeUnion constructs the canonical two-atom OR formula used by the
// fixture's right relation. Region.NewRegion is the transport boundary for a
// reduced ordered BDD; atom identities, rather than guard ordinals, define
// its logical ordering.
func scopeUnion(left, right region.Atom) (region.Region, bool) {
	if !left.Available() || !right.Available() {
		return region.Region{}, false
	}
	leftID, rightID := left.ID(), right.ID()
	if leftID == rightID {
		return region.FromAtom(left)
	}
	low, high := left, right
	if bytes.Compare(leftID[:], rightID[:]) > 0 {
		low, high = right, left
	}
	return region.NewRegion([]region.Node{
		{Atom: high, Low: 0, High: 1},
		{Atom: low, Low: 2, High: 1},
	}, 3)
}
