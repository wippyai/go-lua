package witness_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/check/certificate"
	"github.com/wippyai/go-lua/analysis/relation/mount/address"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement/expand"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/plan"
	regionpkg "github.com/wippyai/go-lua/analysis/relation/schema/region"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

type mountInventory struct {
	fence      address.Fence
	relation   model.RelationID
	column     model.ColumnID
	key        model.KeyID
	scope      model.ScopeID
	scope2     model.ScopeID
	expression model.ExpressionID
	typeID     model.TypeID
	accesses   []arrangement.Access
}

func (inventory *mountInventory) Fence() address.Fence { return inventory.fence }
func (inventory *mountInventory) ResolveRelation(id model.RelationID) (uint64, bool) {
	return 1, id == inventory.relation
}
func (inventory *mountInventory) ResolveColumn(id model.ColumnID) (uint64, bool) {
	return 2, id == inventory.column
}
func (inventory *mountInventory) ResolveKey(id model.KeyID) (uint64, bool) {
	return 3, id == inventory.key
}
func (inventory *mountInventory) ResolveScope(id model.ScopeID) (uint64, bool) {
	if id == inventory.scope {
		return 4, true
	}
	if inventory.scope2.Available() && id == inventory.scope2 {
		return 5, true
	}
	return 0, false
}
func (inventory *mountInventory) ResolveExpression(id model.ExpressionID) (uint64, bool) {
	return 6, inventory.expression.Available() && id == inventory.expression
}
func (inventory *mountInventory) ResolveDependency(model.DependencyID) (uint64, bool) {
	return 0, false
}
func (inventory *mountInventory) Resolve(access arrangement.Access) (arrangement.Handle, bool) {
	// The fixture inventory owns one stable physical coordinate per exact
	// logical access. Reusing a handful of shape slots would make two distinct
	// vector accesses collide as soon as a committed Publish is present.
	for index, candidate := range inventory.accesses {
		if candidate.Equal(access) {
			return arrangement.NewHandle(inventory.fence, uint64(index+1))
		}
	}
	inventory.accesses = append(inventory.accesses, access)
	return arrangement.NewHandle(inventory.fence, uint64(len(inventory.accesses)))
}
func (inventory *mountInventory) ResolveExpand(model.ExpandContract) ([]expand.Vector, bool) {
	return nil, false
}
func (inventory *mountInventory) ResolveDenominator(model.DenominatorRef) (witness.DenominatorEvidence, bool) {
	return witness.DenominatorEvidence{}, false
}

func finite(t *testing.T, label string) regionpkg.Region {
	t.Helper()
	atom, ok := regionpkg.NewAtom(content(t, "region/"+label))
	if !ok {
		t.Fatal("issue region atom")
	}
	value, ok := regionpkg.FromAtom(atom)
	if !ok {
		t.Fatal("seal region")
	}
	return value
}

type algebraRegistry struct{ algebra testAlgebra }

func (registry algebraRegistry) Resolve(typeID model.TypeID) (binding.ValueAlgebra, bool) {
	return registry.algebra, registry.algebra.Type() == typeID
}

type testAlgebra struct{ typeID model.TypeID }

func (algebra testAlgebra) Type() model.TypeID { return algebra.typeID }
func (algebra testAlgebra) Join(left, right binding.ValueToken) (binding.ValueToken, bool) {
	if left.Type() != algebra.typeID || right.Type() != algebra.typeID {
		return binding.ValueToken{}, false
	}
	return right, true
}
func (algebra testAlgebra) Widen(left, right binding.ValueToken) (binding.ValueToken, bool) {
	return algebra.Join(left, right)
}
func (algebra testAlgebra) LessOrEqual(left, right binding.ValueToken) bool {
	return left.Type() == algebra.typeID && right.Type() == algebra.typeID
}

type operationFactory struct{ value signature.Signature }

func (factory operationFactory) Bind(value signature.Signature) (binding.Binding, bool) {
	if value.Digest() != factory.value.Digest() {
		return nil, false
	}
	return operationBinding{value: value}, true
}

type operationBinding struct{ value signature.Signature }

func (bindingValue operationBinding) Signature() signature.Signature { return bindingValue.value }
func (bindingValue operationBinding) NewWorker(binding.Fence) (binding.Worker, bool) {
	return nil, false
}

func TestSpecializeAdmitsAddressArrangementRegionAndCodec(t *testing.T) {
	owner := issueOwner(t, "owner")
	schemaID := issueSchema(t, owner, "schema")
	relation := issueRelation(t, owner, "relation")
	column := issueColumn(t, relation, "column")
	key := issueKey(t, relation, "key")
	scope := issueScope(t, owner, "scope")
	scope2 := issueScope(t, owner, "scope2")
	typeID := issueType(t, owner, "type")
	builder := plan.NewBuilder(schemaID)
	if !builder.AddRelation(model.DefineRelationSchema(relation, []model.ColumnID{column}, []model.KeyID{key}, scope)) ||
		!builder.AddColumn(model.DefineColumnSchema(column, typeID)) ||
		!builder.AddKey(model.DefineKeySchema(key, []model.ColumnID{column})) ||
		!builder.AddScope(model.DefineScopeSchema(scope, nil, finite(t, "scope"))) ||
		!builder.AddScope(model.DefineScopeSchema(scope2, nil, finite(t, "scope2"))) {
		t.Fatal("add declaration")
	}
	schema, ok := builder.Build()
	if !ok {
		t.Fatal("build schema")
	}
	cert, refusal := certificate.Check(schema)
	if refusal != nil || !cert.Available() {
		t.Fatalf("check schema: %v", refusal)
	}
	store, ok := identity.IssueStore()
	if !ok {
		t.Fatal("issue store")
	}
	fence, ok := address.NewFence(schemaID, cert.Digest(), store, identity.MountID{1}, identity.Generation(1))
	if !ok {
		t.Fatal("new fence")
	}
	inventory := &mountInventory{fence: fence, relation: relation, column: column, key: key, scope: scope, scope2: scope2, typeID: typeID}
	mounted, ok := witness.Specialize(cert, inventory, nil, algebraRegistry{algebra: testAlgebra{typeID: typeID}}, newLineageFactory(t, owner))
	if !ok || !mounted.Available() {
		t.Fatal("valid mount refused")
	}
	if requirements := mounted.AlgebraRequirements(); len(requirements) != 0 {
		t.Fatalf("unpublished typed column requested an algebra: %+v", requirements)
	}
	if _, ok := mounted.Algebra(typeID); ok {
		t.Fatal("unpublished typed column admitted an algebra")
	}
	if codecTypes := mounted.CodecTypes(); len(codecTypes) != 1 || codecTypes[0] != typeID {
		t.Fatalf("typed column codec catalogue = %+v", codecTypes)
	}
	if _, ok := mounted.IssueValue(typeID, content(t, "unpublished-codec")); !ok {
		t.Fatal("unpublished typed column did not retain its codec")
	}
	scopeValue, ok := mounted.Scope(scope)
	if !ok || !scopeValue.Available() {
		t.Fatal("scope lookup failed")
	}
	region, ok := mounted.RegionForScope(scopeValue)
	if !ok {
		t.Fatal("scope region lookup failed")
	}
	identityValue := region.Identity()
	if !identityValue.Available() || identityValue != finite(t, "scope").Identity() {
		t.Fatal("scope region identity changed")
	}
	left, leftOK := mounted.Scope(scope)
	right, rightOK := mounted.Scope(scope2)
	if !leftOK || !rightOK {
		t.Fatal("second scope lookup failed")
	}
	joined, joinedOK := mounted.ConjoinScopes(left, right)
	if !joinedOK || !joined.Available() || !mounted.EntailsScopes(joined, left) || !mounted.EntailsScopes(joined, right) {
		t.Fatal("scope conjunction did not retain both formulas")
	}
	joinedRegion, joinedRegionOK := mounted.RegionForScope(joined)
	if !joinedRegionOK || !regionpkg.Entails(joinedRegion, region) {
		t.Fatal("dynamic scope formula was not recoverable")
	}
	reversed, reversedOK := mounted.ConjoinScopes(right, left)
	if !reversedOK || !joined.Same(reversed) {
		t.Fatal("scope conjunction was not canonical/commutative")
	}
	idempotent, idempotentOK := mounted.ConjoinScopes(left, left)
	if !idempotentOK || !idempotent.Same(left) || !mounted.EntailsScopes(left, left) {
		t.Fatal("scope conjunction was not idempotent/reflexive")
	}
	staleInventory := *inventory
	staleInventory.fence, _ = address.NewFence(schemaID, cert.Digest(), store, identity.MountID{1}, identity.Generation(2))
	staleMounted, staleOK := witness.Specialize(cert, &staleInventory, nil, algebraRegistry{algebra: testAlgebra{typeID: typeID}}, newLineageFactory(t, owner))
	if !staleOK {
		t.Fatal("stale fixture mount refused")
	}
	staleScope, staleScopeOK := staleMounted.Scope(scope)
	if !staleScopeOK {
		t.Fatal("stale scope lookup failed")
	}
	if _, ok := mounted.ConjoinScopes(left, staleScope); ok || mounted.EntailsScopes(left, staleScope) {
		t.Fatal("stale scope crossed the exact runtime fence")
	}
	if len(mounted.WideningPermits()) != 0 || len(mounted.Denominators()) != 0 {
		t.Fatal("empty certificate projections were not preserved")
	}
}

func TestSpecializeRefusesForeignFenceAndMissingRegion(t *testing.T) {
	owner := issueOwner(t, "hostile-owner")
	schemaID := issueSchema(t, owner, "hostile-schema")
	relation := issueRelation(t, owner, "hostile-relation")
	column := issueColumn(t, relation, "hostile-column")
	key := issueKey(t, relation, "hostile-key")
	scope := issueScope(t, owner, "hostile-scope")
	typeID := issueType(t, owner, "hostile-type")
	builder := plan.NewBuilder(schemaID)
	if !builder.AddRelation(model.DefineRelationSchema(relation, []model.ColumnID{column}, []model.KeyID{key}, scope)) || !builder.AddColumn(model.DefineColumnSchema(column, typeID)) || !builder.AddKey(model.DefineKeySchema(key, []model.ColumnID{column})) || !builder.AddScope(model.DefineScopeSchema(scope, nil, finite(t, "hostile-scope"))) {
		t.Fatal("add declaration")
	}
	schema, ok := builder.Build()
	if !ok {
		t.Fatal("build schema")
	}
	cert, refusal := certificate.Check(schema)
	if refusal != nil {
		t.Fatalf("check schema: %v", refusal)
	}
	store, _ := identity.IssueStore()
	fence, _ := address.NewFence(schemaID, cert.Digest(), store, identity.MountID{1}, identity.Generation(1))
	base := &mountInventory{fence: fence, relation: relation, column: column, key: key, scope: scope, typeID: typeID}
	if _, ok := witness.Specialize(cert, base, nil, algebraRegistry{algebra: testAlgebra{typeID: typeID}}, newLineageFactory(t, owner)); !ok {
		t.Fatal("baseline hostile fixture refused")
	}
	foreign := *base
	foreign.fence, _ = address.NewFence(schemaID, identity.ContentID{0: 9}, store, identity.MountID{2}, identity.Generation(1))
	if mounted, ok := witness.Specialize(cert, &foreign, nil, algebraRegistry{algebra: testAlgebra{typeID: typeID}}, newLineageFactory(t, owner)); ok || mounted.Available() {
		t.Fatal("foreign arrangement/address fence accepted")
	}
}

func TestSpecializeAdmitsExactOperationAndRejectsWrongDenominatorRows(t *testing.T) {
	value := newSemanticAdmissionFixture(t)
	mounted, ok := witness.Specialize(value.cert, &value.evidence, operationFactory{value: value.operation}, algebraRegistry{algebra: testAlgebra{typeID: value.typeID}}, newLineageFactory(t, value.owner))
	if !ok || !mounted.Available() {
		t.Fatal("semantic mount refused")
	}
	if _, ok := mounted.Binding(value.operation.Identity()); !ok {
		t.Fatal("exact operation binding missing")
	}
	scopeValue, ok := mounted.Scope(value.scope)
	if !ok || !scopeValue.Available() {
		t.Fatal("admitted scope missing")
	}
	scopeToken, ok := mounted.ScopeToken(scopeValue)
	if !ok || !scopeToken.ValidFor(mounted.RuntimeFence()) {
		t.Fatal("narrow scope token projection refused")
	}
	denominatorWitness, witnessOK := mounted.Denominator(value.denominator)
	if !witnessOK || !denominatorWitness.Contains(value.row) {
		t.Fatal("denominator witness missing")
	}
	cell, ok := mounted.IssueCell(denominatorWitness, scopeValue, value.column, value.row)
	if !ok || !cell.ValidFor(mounted.RuntimeFence()) {
		t.Fatal("mounted cell issuance refused")
	}
	cellRegion, cellRegionOK := mounted.RegionForToken(cell.Scope())
	var cellRegionID identity.ContentID
	cellRegionIDOK := false
	if cellRegionOK {
		cellRegionID = cellRegion.Identity()
		cellRegionIDOK = cellRegionID.Available()
	}
	scopeRegion, scopeRegionOK := mounted.RegionForScope(scopeValue)
	var scopeRegionID identity.ContentID
	scopeRegionIDOK := false
	if scopeRegionOK {
		scopeRegionID = scopeRegion.Identity()
		scopeRegionIDOK = scopeRegionID.Available()
	}
	if !cellRegionOK || !cellRegionIDOK || !scopeRegionOK || !scopeRegionIDOK || cellRegionID != scopeRegionID {
		t.Fatal("cell scope token did not round-trip through the mounted arena")
	}
	valueToken, ok := mounted.IssueValue(value.typeID, content(t, "semantic-value"))
	if !ok || !valueToken.ValidFor(mounted.RuntimeFence()) {
		t.Fatal("mounted value issuance refused")
	}
	foreignType := issueType(t, value.owner, "semantic-foreign-type")
	if _, ok := mounted.IssueValue(foreignType, content(t, "foreign-value")); ok {
		t.Fatal("value for unadmitted type accepted")
	}
	if index, ok := mounted.RowIndex(value.relation, value.row); !ok || index != 0 {
		t.Fatalf("row index = %d/%v", index, ok)
	}
	if _, ok := mounted.RowIndex(value.relation, model.RowID{}); ok {
		t.Fatal("zero row accepted")
	}
	duplicateEvidence := value.evidence
	duplicateEvidence.rows = []model.RowID{value.row, value.row}
	if duplicateMounted, duplicateOK := witness.Specialize(value.cert, &duplicateEvidence, operationFactory{value: value.operation}, algebraRegistry{algebra: testAlgebra{typeID: value.typeID}}, newLineageFactory(t, value.owner)); duplicateOK || duplicateMounted.Available() {
		t.Fatal("duplicate denominator row accepted")
	}
	foreignRelation := issueRelation(t, value.owner, "semantic-foreign-relation")
	foreignRow, foreignRowOK := model.IssueRowID(foreignRelation, content(t, "semantic-foreign-row"))
	if !foreignRowOK {
		t.Fatal("issue foreign row")
	}
	foreignEvidence := value.evidence
	foreignEvidence.rows = []model.RowID{foreignRow}
	if foreignMounted, foreignOK := witness.Specialize(value.cert, &foreignEvidence, operationFactory{value: value.operation}, algebraRegistry{algebra: testAlgebra{typeID: value.typeID}}, newLineageFactory(t, value.owner)); foreignOK || foreignMounted.Available() {
		t.Fatal("wrong-relation denominator row accepted")
	}
}

func TestSpecializeRequiresAlgebraOnlyForPresentPublication(t *testing.T) {
	t.Run("authenticated opaque candidate", func(t *testing.T) {
		value := newSemanticAdmissionFixtureWithPresence(t, signature.RequireOpaque, signature.ProduceOpaque, true, model.DecodeOnly)
		if requirements := value.cert.AlgebraRequirements(); len(requirements) != 0 {
			t.Fatalf("opaque publication requested a value algebra: %+v", requirements)
		}
		mounted, ok := witness.Specialize(value.cert, &value.evidence, operationFactory{value: value.operation}, nil, newLineageFactory(t, value.owner))
		if !ok || !mounted.Available() {
			t.Fatal("opaque publication did not mount without an algebra")
		}
		if _, ok := mounted.Algebra(value.typeID); ok {
			t.Fatal("opaque publication admitted an invented algebra")
		}
		capability, capabilityOK := mounted.TypeCapability(value.typeID)
		if !capabilityOK || !capability.DecodeOnly() {
			t.Fatalf("opaque publication capability = %v/%t, want DecodeOnly/true", capability.Kind(), capabilityOK)
		}
		if token, ok := mounted.IssueValue(value.typeID, content(t, "opaque-candidate")); !ok || !token.ValidFor(mounted.RuntimeFence()) {
			t.Fatal("opaque publication did not retain its declared semantic codec")
		}
		foreignType := issueType(t, value.owner, "opaque-unregistered-type")
		if _, ok := mounted.IssueValue(foreignType, content(t, "opaque-unregistered-value")); ok {
			t.Fatal("unregistered TypeID was issued through the codec catalogue")
		}
	})

	t.Run("present publication", func(t *testing.T) {
		value := newSemanticAdmissionFixture(t)
		requirements := value.cert.AlgebraRequirements()
		if len(requirements) != 1 || requirements[0] != value.typeID {
			t.Fatalf("present output lost its required value algebra: %+v", requirements)
		}
		mounted, ok := witness.Specialize(value.cert, &value.evidence, operationFactory{value: value.operation}, nil, newLineageFactory(t, value.owner))
		if ok || mounted.Available() {
			t.Fatal("present publication mounted without its required algebra")
		}
		registry := &countedAlgebraRegistry{algebra: testAlgebra{typeID: value.typeID}}
		mounted, ok = witness.Specialize(value.cert, &value.evidence, operationFactory{value: value.operation}, registry, newLineageFactory(t, value.owner))
		if !ok || !mounted.Available() {
			t.Fatal("present publication with its required algebra was refused")
		}
		if registry.calls != 1 {
			t.Fatalf("present publication resolved algebra %d times, want once", registry.calls)
		}
		capability, capabilityOK := mounted.TypeCapability(value.typeID)
		if !capabilityOK || !capability.Ascending() {
			t.Fatalf("present publication capability = %v/%t, want Ascending/true", capability.Kind(), capabilityOK)
		}
	})
}

func TestAscendingKeyEqualityDoesNotGrantUnusedAscent(t *testing.T) {
	owner := issueOwner(t, "ascending-key-equality-owner")
	schemaID := issueSchema(t, owner, "ascending-key-equality-schema")
	relation := issueRelation(t, owner, "ascending-key-equality-relation")
	column := issueColumn(t, relation, "ascending-key-equality-column")
	key := issueKey(t, relation, "ascending-key-equality-key")
	scope := issueScope(t, owner, "ascending-key-equality-scope")
	typeID := issueType(t, owner, "ascending-key-equality-type")
	expressionID := issueExpression(t, owner, "ascending-key-equality-expression")

	builder := plan.NewBuilder(schemaID)
	capability, capabilityOK := model.NewAscendingCapability(typeID)
	project := algebra.NewProject(
		algebra.NewInput(relation),
		algebra.NewProjectContract(relation, []algebra.ColumnMapping{algebra.NewColumnMapping(column, column)}, key),
	)
	if !capabilityOK || !builder.AddTypeCapability(capability) ||
		!builder.AddRelation(model.DefineRelationSchema(relation, []model.ColumnID{column}, []model.KeyID{key}, scope)) ||
		!builder.AddColumn(model.DefineColumnSchema(column, typeID)) ||
		!builder.AddKey(model.DefineKeySchema(key, []model.ColumnID{column})) ||
		!builder.AddScope(model.DefineScopeSchema(scope, nil, finite(t, "ascending-key-equality-scope"))) ||
		!builder.AddExpression(plan.DefineExpressionRef(expressionID, project)) {
		t.Fatal("add ascending equality schema")
	}
	schema, schemaOK := builder.Build()
	if !schemaOK {
		t.Fatal("build ascending equality schema")
	}
	cert, refusal := certificate.Check(schema)
	if refusal != nil || !cert.Available() {
		t.Fatalf("check ascending equality schema: %v", refusal)
	}
	if len(cert.EqualityRequirements()) != 1 {
		t.Fatalf("project equality requirements = %d, want 1", len(cert.EqualityRequirements()))
	}
	if len(cert.AlgebraRequirements()) != 0 {
		t.Fatalf("project unexpectedly requires ascent algebra: %v", cert.AlgebraRequirements())
	}
	store, storeOK := identity.IssueStore()
	if !storeOK {
		t.Fatal("issue store")
	}
	fence, fenceOK := address.NewFence(schemaID, cert.Digest(), store, identity.MountID{7}, identity.Generation(1))
	if !fenceOK {
		t.Fatal("issue fence")
	}
	inventory := &mountInventory{fence: fence, relation: relation, column: column, key: key, scope: scope, expression: expressionID, typeID: typeID}
	mounted, mountedOK := witness.Specialize(cert, inventory, nil, algebraRegistry{algebra: testAlgebra{typeID: typeID}}, newLineageFactory(t, owner))
	if !mountedOK || !mounted.Available() {
		t.Fatal("ascending key equality mount refused")
	}
	if _, ascentOK := mounted.Algebra(typeID); ascentOK {
		t.Fatal("unused ascending algebra entered mounted ascent map")
	}
	if equality, equalityOK := mounted.Equality(typeID); !equalityOK || equality == nil {
		t.Fatal("ascending equality projection was not mounted")
	}
}

func TestMountedRowAtIsTheFencedRelationDirectoryInverse(t *testing.T) {
	value := newSemanticAdmissionFixture(t)
	mounted, ok := witness.Specialize(value.cert, &value.evidence, operationFactory{value: value.operation}, algebraRegistry{algebra: testAlgebra{typeID: value.typeID}}, newLineageFactory(t, value.owner))
	if !ok || !mounted.Available() {
		t.Fatal("semantic mount refused")
	}
	witnessValue, ok := mounted.Denominator(value.denominator)
	if !ok || witnessValue.Len() != 1 {
		t.Fatalf("denominator witness cardinality = %d/%v, want 1/true", witnessValue.Len(), ok)
	}
	row, ok := witnessValue.At(0)
	if !ok || row != value.row {
		t.Fatalf("witness row = %v/%v, want %v/true", row, ok, value.row)
	}
	mountedRow, ok := mounted.RowAt(value.relation, 0)
	if !ok || mountedRow != value.row {
		t.Fatalf("mounted row = %v/%v, want %v/true", mountedRow, ok, value.row)
	}
	if index, ok := mounted.RowIndex(value.relation, mountedRow); !ok || index != 0 {
		t.Fatalf("mounted inverse index = %d/%v, want 0/true", index, ok)
	}
	for _, index := range []int{-1, 1} {
		if _, ok := mounted.RowAt(value.relation, index); ok {
			t.Fatalf("out-of-range mounted row index %d accepted", index)
		}
	}
	foreignRelation := issueRelation(t, value.owner, "semantic-row-at-foreign-relation")
	if _, ok := mounted.RowAt(foreignRelation, 0); ok {
		t.Fatal("foreign denominator row accepted")
	}
	if _, ok := (witness.Mounted{}).RowAt(value.relation, 0); ok {
		t.Fatal("unavailable mounted row accepted")
	}
	if allocations := testing.AllocsPerRun(100, func() {
		if _, ok := mounted.RowAt(value.relation, 0); !ok {
			t.Fatal("fenced row read failed")
		}
	}); allocations != 0 {
		t.Fatalf("mounted row read allocations = %v, want 0", allocations)
	}
}

type semanticAdmissionFixture struct {
	owner       model.OwnerID
	schema      model.SchemaID
	relation    model.RelationID
	column      model.ColumnID
	key         model.KeyID
	scope       model.ScopeID
	typeID      model.TypeID
	cert        certificate.Certificate
	operation   signature.Signature
	denominator model.DenominatorRef
	row         model.RowID
	evidence    evidenceInventory
}

func newSemanticAdmissionFixture(t *testing.T) semanticAdmissionFixture {
	return newSemanticAdmissionFixtureWithPresence(t, signature.RequirePresent, signature.ProducePresent, true, model.Ascending)
}

func newTerminalPresentSemanticAdmissionFixture(t *testing.T) semanticAdmissionFixture {
	return newSemanticAdmissionFixtureWithPresence(t, signature.RequirePresent, signature.ProducePresent, false, model.InvalidTypeCapability)
}

func newSemanticAdmissionFixtureWithPresence(t *testing.T, inputPresence, outputPresence signature.PresenceContract, commit bool, capabilityKind model.TypeCapabilityKind) semanticAdmissionFixture {
	t.Helper()
	owner := issueOwner(t, "semantic-owner")
	schemaID := issueSchema(t, owner, "semantic-schema")
	relation := issueRelation(t, owner, "semantic-relation")
	column := issueColumn(t, relation, "semantic-column")
	key := issueKey(t, relation, "semantic-key")
	scope := issueScope(t, owner, "semantic-scope")
	typeID := issueType(t, owner, "semantic-type")
	operation, ok := model.IssueOperationID(owner, content(t, "semantic-operation"))
	if !ok {
		t.Fatal("issue operation")
	}
	denominator, ok := model.NewDenominatorRef(relation, key)
	if !ok {
		t.Fatal("issue denominator")
	}
	delivery, ok := signature.NewScalarDelivery()
	if !ok {
		t.Fatal("scalar delivery")
	}
	cardinality, ok := model.NewCardinality(model.ExactlyOne, 0)
	if !ok {
		t.Fatal("cardinality")
	}
	outcomes, ok := outcome.NewSet(outcome.Produced, outcome.NoSelection, outcome.Refused)
	if !ok {
		t.Fatal("outcomes")
	}
	operationValue, ok := signature.Seal(signature.Spec{
		Identity:    signature.Identity{Operation: operation, Version: 1},
		Fence:       signature.Fence{Owner: owner, Schema: schemaID},
		Inputs:      []signature.Input{{Relation: relation, Column: column, Type: typeID, Presence: inputPresence, Delivery: delivery, Denominator: denominator}},
		Outputs:     []signature.Output{{Relation: relation, Column: column, Type: typeID, Presence: outputPresence, Denominator: denominator}},
		Cardinality: cardinality, Outcomes: outcomes,
	})
	if !ok {
		t.Fatal("seal operation")
	}
	builder := plan.NewBuilder(schemaID)
	if capabilityKind != model.InvalidTypeCapability {
		capability, capabilityOK := model.NewTypeCapability(typeID, capabilityKind)
		if !capabilityOK || !builder.AddTypeCapability(capability) {
			t.Fatal("add type capability")
		}
	}
	if !builder.AddRelation(model.DefineRelationSchema(relation, []model.ColumnID{column}, []model.KeyID{key}, scope)) || !builder.AddColumn(model.DefineColumnSchema(column, typeID)) || !builder.AddKey(model.DefineKeySchema(key, []model.ColumnID{column})) || !builder.AddScope(model.DefineScopeSchema(scope, nil, finite(t, "semantic-scope"))) || !builder.AddSignature(operationValue) {
		t.Fatal("add semantic declaration")
	}
	var expressionID model.ExpressionID
	if commit {
		apply := algebra.NewApply([]algebra.Expression{algebra.NewInput(relation)}, algebra.NewApplyContract(operationValue.Identity(), []algebra.SlotSource{algebra.NewSlotSource(0, 0)}, algebra.OwnerNamed()))
		publish := algebra.NewPublish(apply, algebra.NewPublishContract(relation, key))
		expressionID = issueExpression(t, owner, "semantic-publish")
		if !builder.AddExpression(plan.DefineExpressionRef(expressionID, publish)) {
			t.Fatal("add semantic publication")
		}
	}
	schema, ok := builder.Build()
	if !ok {
		t.Fatal("build semantic schema")
	}
	cert, refusal := certificate.Check(schema)
	if refusal != nil || !cert.Available() {
		t.Fatalf("check semantic schema: %v", refusal)
	}
	store, ok := identity.IssueStore()
	if !ok {
		t.Fatal("issue store")
	}
	fence, ok := address.NewFence(schemaID, cert.Digest(), store, identity.MountID{3}, identity.Generation(1))
	if !ok {
		t.Fatal("issue fence")
	}
	row, ok := model.IssueRowID(relation, content(t, "semantic-row"))
	if !ok {
		t.Fatal("issue row")
	}
	semanticInventory := &mountInventory{fence: fence, relation: relation, column: column, key: key, scope: scope, expression: expressionID, typeID: typeID}
	evidence := evidenceInventory{mountInventory: semanticInventory, denominator: denominator, rows: []model.RowID{row}, evidence: content(t, "semantic-evidence")}
	return semanticAdmissionFixture{owner: owner, schema: schemaID, relation: relation, column: column, key: key, scope: scope, typeID: typeID, cert: cert, operation: operationValue, denominator: denominator, row: row, evidence: evidence}
}

type evidenceInventory struct {
	*mountInventory
	denominator model.DenominatorRef
	rows        []model.RowID
	evidence    identity.ContentID
}

func (inventory *evidenceInventory) ResolveDenominator(ref model.DenominatorRef) (witness.DenominatorEvidence, bool) {
	if ref != inventory.denominator {
		return witness.DenominatorEvidence{}, false
	}
	return witness.NewDenominatorEvidence(inventory.rows, inventory.evidence)
}

func content(t *testing.T, label string) identity.ContentID {
	t.Helper()
	value, ok := identity.DeriveContentID("witness/specialize/test/v1", []byte(label))
	if !ok {
		t.Fatal("derive content")
	}
	return value
}

func issueOwner(t *testing.T, label string) model.OwnerID {
	value, ok := model.IssueOwnerID(content(t, "owner/"+label))
	if !ok {
		t.Fatal("issue owner")
	}
	return value
}
func issueSchema(t *testing.T, owner model.OwnerID, label string) model.SchemaID {
	value, ok := model.IssueSchemaID(owner, content(t, "schema/"+label))
	if !ok {
		t.Fatal("issue schema")
	}
	return value
}
func issueRelation(t *testing.T, owner model.OwnerID, label string) model.RelationID {
	value, ok := model.IssueRelationID(owner, content(t, "relation/"+label))
	if !ok {
		t.Fatal("issue relation")
	}
	return value
}
func issueColumn(t *testing.T, relation model.RelationID, label string) model.ColumnID {
	value, ok := model.IssueColumnID(relation, content(t, "column/"+label))
	if !ok {
		t.Fatal("issue column")
	}
	return value
}
func issueKey(t *testing.T, relation model.RelationID, label string) model.KeyID {
	value, ok := model.IssueKeyID(relation, content(t, "key/"+label))
	if !ok {
		t.Fatal("issue key")
	}
	return value
}
func issueScope(t *testing.T, owner model.OwnerID, label string) model.ScopeID {
	value, ok := model.IssueScopeID(owner, content(t, "scope/"+label))
	if !ok {
		t.Fatal("issue scope")
	}
	return value
}
func issueType(t *testing.T, owner model.OwnerID, label string) model.TypeID {
	value, ok := model.IssueTypeID(owner, content(t, "type/"+label))
	if !ok {
		t.Fatal("issue type")
	}
	return value
}
