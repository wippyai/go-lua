package authority

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
)

func TestCatalogDigestIsDeterministicAndIncludesAuthoredOrder(t *testing.T) {
	firstFixture := newCatalogFixture(t)
	first, ok := firstFixture.seal(t)
	if !ok {
		t.Fatal("valid fixture was rejected")
	}
	secondFixture := newCatalogFixture(t)
	second, ok := secondFixture.seal(t)
	if !ok {
		t.Fatal("same valid fixture was rejected")
	}
	if first.Digest() != second.Digest() {
		t.Fatal("same declaration produced different digest")
	}

	ordered := newCatalogFixture(t)
	ordered.columns[0], ordered.columns[1] = ordered.columns[1], ordered.columns[0]
	reordered, ok := ordered.seal(t)
	if !ok {
		t.Fatal("reordered valid fixture was rejected")
	}
	if reordered.Digest() == first.Digest() {
		t.Fatal("authored column order was absent from digest")
	}
	if !reflect.DeepEqual(reordered.Relations()[0].Columns(), []schema.Key{"value", "address"}) {
		t.Fatalf("relation column order = %v", reordered.Relations()[0].Columns())
	}
}

func TestCatalogProjectionCarriesEveryRegistryFactAndExactFence(t *testing.T) {
	fixture := newCatalogFixture(t)
	catalog, ok := fixture.seal(t)
	if !ok {
		t.Fatal("valid fixture was rejected")
	}
	if catalog.Owner() != fixture.owner || catalog.OwnerID() != fixture.owner.ID() {
		t.Fatal("catalog owner fence changed")
	}
	relation, _ := catalog.RelationAt(0)
	columnAddress, _ := catalog.ColumnAt(0)
	columnValue, _ := catalog.ColumnAt(1)
	key, _ := catalog.KeyAt(0)
	scope, _ := catalog.ScopeAt(0)
	denominator, _ := catalog.DenominatorAt(0)
	if relation.ID().Owner() != catalog.OwnerID() || columnAddress.ID().Owner() != catalog.OwnerID() || columnValue.ID().Owner() != catalog.OwnerID() || key.ID().Owner() != catalog.OwnerID() || scope.ID().Owner() != catalog.OwnerID() || denominator.Reference().Owner() != catalog.OwnerID() {
		t.Fatal("local identity escaped owner fence")
	}
	if columnAddress.Type() != fixture.typeID || columnValue.Type() != fixture.typeID || columnAddress.Type().Owner() == catalog.OwnerID() {
		t.Fatal("complete cross-owner type authority was not retained exactly")
	}
	publication, publicationOK := relation.PublicationKey()
	if relation.Name() != "orders" || relation.Scope() != "order-scope" || publication != "primary" || !publicationOK {
		t.Fatal("relation projection omitted address vocabulary")
	}
	if !reflect.DeepEqual(relation.Addressing(), []Address{{Coordinate: CoordinateAddress, Column: "address"}}) {
		t.Fatal("relation addressing declaration changed")
	}
	if !reflect.DeepEqual(key.Columns(), []schema.Key{"address"}) || !reflect.DeepEqual(scope.Dimensions(), []schema.Key{"address"}) {
		t.Fatal("ordered key/scope vectors changed")
	}
	if denominator.Relation() != relation.Name() || denominator.Key() != key.Name() || denominator.Reference().Relation() != relation.ID() || denominator.Reference().Key() != key.ID() {
		t.Fatal("denominator projection omitted canonical relation/key reference")
	}
}
