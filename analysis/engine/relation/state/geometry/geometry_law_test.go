package geometry_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/geometry"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/check/certificate"
	"github.com/wippyai/go-lua/analysis/relation/mount/address"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/schema/plan"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
	"github.com/wippyai/go-lua/analysis/relation/semantic/lineage"
	"github.com/wippyai/go-lua/analysis/relation/semantic/outcome"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

// lawRegion is deliberately neutral. Geometry does not inspect its
// identity; the mounted scope arena authenticates the token-to-region pair,
// and the injected mapper owns conversion to the engine guard universe.
type lawRegion struct{ id identity.ContentID }

func (region lawRegion) Identity() (identity.ContentID, bool) {
	return region.id, region.id.Available()
}

func (region lawRegion) Conjoin(other witness.Region) (witness.Region, bool) {
	if other == nil {
		return nil, false
	}
	id, ok := other.Identity()
	if !ok {
		return nil, false
	}
	return lawRegion{id: id}, true
}

func (region lawRegion) Entails(other witness.Region) bool {
	if other == nil {
		return false
	}
	_, ok := other.Identity()
	return ok
}

func lawMapper(t testing.TB, mounted witness.Mounted, manager *guard.Manager, fn func(witness.Region) (support.Mask, bool)) geometry.RegionMapper {
	t.Helper()
	mapper, ok := geometry.NewRegionMapper(mounted, manager, fn)
	if !ok {
		t.Fatal("region mapper")
	}
	return mapper
}

type mountInventory struct {
	fence       address.Fence
	relation    model.RelationID
	column      model.ColumnID
	key         model.KeyID
	scope       model.ScopeID
	secondScope model.ScopeID
	typeID      model.TypeID
	denominator model.DenominatorRef
	rows        []model.RowID
	evidence    identity.ContentID
	region      witness.Region
	second      witness.Region
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
	switch id {
	case inventory.scope:
		return 4, true
	case inventory.secondScope:
		return 5, true
	default:
		return 0, false
	}
}

func (inventory *mountInventory) ResolveExpression(model.ExpressionID) (uint64, bool) {
	return 0, false
}

func (inventory *mountInventory) ResolveDependency(model.DependencyID) (uint64, bool) {
	return 0, false
}

func (inventory *mountInventory) Resolve(access arrangement.Access) (arrangement.Handle, bool) {
	// The physical slot is intentionally opaque to Geometry. This fixture only
	// needs distinct non-zero coordinates for the certified access forms.
	slot := uint64(2)
	if access.Key().Available() {
		slot = 1
	}
	if access.Key().Available() && len(access.Columns()) != 0 {
		slot = 3
	}
	return arrangement.NewHandle(inventory.fence, slot)
}

func (inventory *mountInventory) ScopeRegion(id model.ScopeID) (witness.Region, bool) {
	switch id {
	case inventory.scope:
		return inventory.region, inventory.region != nil
	case inventory.secondScope:
		return inventory.second, inventory.second != nil
	default:
		return nil, false
	}
}

func (inventory *mountInventory) ResolveDenominator(ref model.DenominatorRef) (witness.DenominatorEvidence, bool) {
	if ref != inventory.denominator {
		return witness.DenominatorEvidence{}, false
	}
	return witness.NewDenominatorEvidence(inventory.rows, inventory.evidence)
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

type geometryFixture struct {
	mounted      witness.Mounted
	cell         binding.CellToken
	secondCell   binding.CellToken
	foreignScope binding.ScopeToken
	firstScope   binding.ScopeToken
	manager      *guard.Manager
}

func content(t testing.TB, label string) identity.ContentID {
	t.Helper()
	value, ok := identity.DeriveContentID("engine/relation/state/geometry/law/v1", []byte(label))
	if !ok {
		t.Fatalf("derive content %q", label)
	}
	return value
}

func issueOwner(t testing.TB, label string) model.OwnerID {
	value, ok := model.IssueOwnerID(content(t, "owner/"+label))
	if !ok {
		t.Fatal("issue owner")
	}
	return value
}

func issueSchema(t testing.TB, owner model.OwnerID, label string) model.SchemaID {
	value, ok := model.IssueSchemaID(owner, content(t, "schema/"+label))
	if !ok {
		t.Fatal("issue schema")
	}
	return value
}

func issueRelation(t testing.TB, owner model.OwnerID, label string) model.RelationID {
	value, ok := model.IssueRelationID(owner, content(t, "relation/"+label))
	if !ok {
		t.Fatal("issue relation")
	}
	return value
}

func issueColumn(t testing.TB, relation model.RelationID, label string) model.ColumnID {
	value, ok := model.IssueColumnID(relation, content(t, "column/"+label))
	if !ok {
		t.Fatal("issue column")
	}
	return value
}

func issueKey(t testing.TB, relation model.RelationID, label string) model.KeyID {
	value, ok := model.IssueKeyID(relation, content(t, "key/"+label))
	if !ok {
		t.Fatal("issue key")
	}
	return value
}

func issueScope(t testing.TB, owner model.OwnerID, label string) model.ScopeID {
	value, ok := model.IssueScopeID(owner, content(t, "scope/"+label))
	if !ok {
		t.Fatal("issue scope")
	}
	return value
}

func issueType(t testing.TB, owner model.OwnerID, label string) model.TypeID {
	value, ok := model.IssueTypeID(owner, content(t, "type/"+label))
	if !ok {
		t.Fatal("issue type")
	}
	return value
}

func newLineageFactory(t testing.TB, owner model.OwnerID) lineage.Factory {
	factory, ok := lineage.NewFactory(owner)
	if !ok {
		t.Fatal("lineage factory")
	}
	return factory
}

func newGeometryFixture(t testing.TB, generation identity.Generation) geometryFixture {
	t.Helper()
	owner := issueOwner(t, "geometry")
	schemaID := issueSchema(t, owner, "geometry")
	relation := issueRelation(t, owner, "geometry")
	column := issueColumn(t, relation, "geometry")
	key := issueKey(t, relation, "geometry")
	scope := issueScope(t, owner, "geometry")
	secondScope := issueScope(t, owner, "geometry-second")
	typeID := issueType(t, owner, "geometry")
	operationID, ok := model.IssueOperationID(owner, content(t, "operation/geometry"))
	if !ok {
		t.Fatal("operation id")
	}
	denominator, ok := model.NewDenominatorRef(relation, key)
	if !ok {
		t.Fatal("denominator")
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
	operation, ok := signature.Seal(signature.Spec{
		Identity:  signature.Identity{Operation: operationID, Version: 1},
		Fence:     signature.Fence{Owner: owner, Schema: schemaID},
		Inputs:    []signature.Input{{Relation: relation, Column: column, Type: typeID, Presence: signature.RequirePresent, Delivery: delivery, Denominator: denominator}},
		Outputs:   []signature.Output{{Relation: relation, Column: column, Type: typeID, Presence: signature.ProducePresent}},
		Authority: signature.OutputAuthority{Denominator: denominator}, Cardinality: cardinality, Outcomes: outcomes,
	})
	if !ok {
		t.Fatal("seal operation")
	}
	builder := plan.NewBuilder(schemaID)
	if !builder.AddRelation(model.DefineRelationSchema(relation, []model.ColumnID{column}, []model.KeyID{key}, scope)) ||
		!builder.AddColumn(model.DefineColumnSchema(column, typeID)) ||
		!builder.AddKey(model.DefineKeySchema(key, []model.ColumnID{column})) ||
		!builder.AddScope(model.DefineScopeSchema(scope, nil)) ||
		!builder.AddScope(model.DefineScopeSchema(secondScope, nil)) ||
		!builder.AddSignature(operation) {
		t.Fatal("add schema declarations")
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
	fence, ok := address.NewFence(schemaID, cert.Digest(), store, identity.MountID{7}, generation)
	if !ok {
		t.Fatal("mount fence")
	}
	row, ok := model.IssueRowID(relation, content(t, "row/first"))
	if !ok {
		t.Fatal("first row")
	}
	secondRow, ok := model.IssueRowID(relation, content(t, "row/second"))
	if !ok {
		t.Fatal("second row")
	}
	inventory := &mountInventory{
		fence: fence, relation: relation, column: column, key: key,
		scope: scope, secondScope: secondScope, typeID: typeID,
		denominator: denominator, rows: []model.RowID{row, secondRow},
		evidence: content(t, "evidence"),
		region:   lawRegion{id: content(t, "region/first")},
		second:   lawRegion{id: content(t, "region/second")},
	}
	mounted, ok := witness.Specialize(cert, inventory, operationFactory{value: operation}, algebraRegistry{algebra: testAlgebra{typeID: typeID}}, newLineageFactory(t, owner))
	if !ok || !mounted.Available() {
		t.Fatal("valid mounted witness refused")
	}
	firstScope, ok := mounted.Scope(scope)
	if !ok {
		t.Fatal("first scope")
	}
	firstScopeToken, ok := mounted.ScopeToken(firstScope)
	if !ok {
		t.Fatal("first scope token")
	}
	secondScopeValue, ok := mounted.Scope(secondScope)
	if !ok {
		t.Fatal("second scope")
	}
	secondScopeToken, ok := mounted.ScopeToken(secondScopeValue)
	if !ok {
		t.Fatal("second scope token")
	}
	firstCell, ok := mounted.IssueCell(denominator, firstScope, column, row)
	if !ok {
		t.Fatal("first cell")
	}
	secondCell, ok := mounted.IssueCell(denominator, secondScopeValue, column, row)
	if !ok {
		t.Fatal("second-scope cell")
	}
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal("guard manager: ", err)
	}
	return geometryFixture{mounted: mounted, cell: firstCell, secondCell: secondCell, foreignScope: secondScopeToken, firstScope: firstScopeToken, manager: manager}
}

func trueMask(t testing.TB, manager *guard.Manager) support.Mask {
	t.Helper()
	mask, ok := support.True(manager)
	if !ok {
		t.Fatal("true support mask")
	}
	return mask
}

func TestGeometryResolvesAuthenticatedCellToDeterministicScalarKey(t *testing.T) {
	fixture := newGeometryFixture(t, 1)
	mask := trueMask(t, fixture.manager)
	value, ok := geometry.New(fixture.mounted, lawMapper(t, fixture.mounted, fixture.manager, func(witness.Region) (support.Mask, bool) {
		return mask, true
	}))
	if !ok {
		t.Fatal("geometry")
	}
	first, ok := value.LogicalKey(fixture.cell)
	if !ok || !first.Available() || first.Row() != fixture.cell.Row() {
		t.Fatalf("logical key = %#v/%t, want authenticated row", first, ok)
	}
	second, ok := value.LogicalKey(fixture.cell)
	if !ok || second != first {
		t.Fatalf("repeated logical key = %#v/%t, want %#v/true", second, ok, first)
	}
	resolved, ok := value.Resolve(fixture.cell)
	index, indexOK := fixture.mounted.RowIndex(fixture.cell.Relation(), fixture.cell.Row())
	row, rowOK := fixture.mounted.RowAt(fixture.cell.Relation(), index)
	if !ok || !resolved.Available() || resolved.Logical() != first || !resolved.Mask().Equal(mask) || !indexOK || !rowOK || row != fixture.cell.Row() || resolved.Dense() != geometry.Key(index) {
		t.Fatal("coordinate did not retain the authenticated logical key, dense slot, and full scope mask")
	}
}

func TestGeometryRefusesStaleAndForeignCellOrScope(t *testing.T) {
	fixture := newGeometryFixture(t, 1)
	foreign := newGeometryFixture(t, 2)
	mask := trueMask(t, fixture.manager)
	value, ok := geometry.New(fixture.mounted, lawMapper(t, fixture.mounted, fixture.manager, func(witness.Region) (support.Mask, bool) {
		return mask, true
	}))
	if !ok {
		t.Fatal("geometry")
	}
	if _, ok := value.Mask(foreign.firstScope); ok {
		t.Fatal("foreign scope crossed the exact fence")
	}
	if _, ok := value.LogicalKey(foreign.cell); ok {
		t.Fatal("foreign cell crossed the exact fence")
	}
	if !value.Available() {
		t.Fatal("valid mounted geometry unavailable")
	}
}

func TestGeometryRefusesUnavailableRegionConversion(t *testing.T) {
	fixture := newGeometryFixture(t, 1)
	value, ok := geometry.New(fixture.mounted, lawMapper(t, fixture.mounted, fixture.manager, func(witness.Region) (support.Mask, bool) {
		return support.Mask{}, false
	}))
	if !ok {
		t.Fatal("geometry")
	}
	if _, ok := value.Mask(fixture.firstScope); ok {
		t.Fatal("unavailable region conversion was accepted")
	}
}

func TestGeometryRefusesMaskFromForeignGuardManager(t *testing.T) {
	fixture := newGeometryFixture(t, 1)
	foreignManager, err := guard.New(nil)
	if err != nil {
		t.Fatal("foreign guard manager: ", err)
	}
	foreignMask := trueMask(t, foreignManager)
	value, ok := geometry.New(fixture.mounted, lawMapper(t, fixture.mounted, fixture.manager, func(witness.Region) (support.Mask, bool) {
		return foreignMask, true
	}))
	if !ok {
		t.Fatal("geometry")
	}
	if _, ok := value.Mask(fixture.firstScope); ok {
		t.Fatal("mask from a foreign guard manager was accepted")
	}
}

func TestGeometryDoesNotScopeQualifyTheRowKey(t *testing.T) {
	fixture := newGeometryFixture(t, 1)
	mask := trueMask(t, fixture.manager)
	value, ok := geometry.New(fixture.mounted, lawMapper(t, fixture.mounted, fixture.manager, func(witness.Region) (support.Mask, bool) {
		return mask, true
	}))
	if !ok {
		t.Fatal("geometry")
	}
	first, ok := value.LogicalKey(fixture.cell)
	if !ok {
		t.Fatal("first key")
	}
	second, ok := value.LogicalKey(fixture.secondCell)
	if !ok || second != first {
		t.Fatalf("scope-qualified logical key = %#v/%t, want same row key %#v/true", second, ok, first)
	}
}
