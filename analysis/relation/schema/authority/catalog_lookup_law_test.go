package authority

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
)

func TestCatalogNamedRowsResolveExactSealedAuthority(t *testing.T) {
	catalog, ok := newCatalogFixture(t).seal(t)
	if !ok {
		t.Fatal("seal fixture catalog")
	}

	column, columnOK := catalog.ColumnByName("address")
	key, keyOK := catalog.KeyByName("primary")
	scope, scopeOK := catalog.ScopeByName("order-scope")
	denominator, denominatorOK := catalog.DenominatorByName("orders/all")
	if !columnOK || !keyOK || !scopeOK || !denominatorOK ||
		!column.Available() || !key.Available() || !scope.Available() || !denominator.Available() {
		t.Fatal("named owner authority was unavailable")
	}
	if column.Relation() != "orders" || key.Relation() != "orders" ||
		denominator.Relation() != "orders" || denominator.Key() != key.Name() ||
		denominator.Reference().Key() != key.ID() || denominator.Reference().Relation() != column.ID().Relation() {
		t.Fatal("named authority no longer describes one exact owner-local relation")
	}
	if dimensions := scope.Dimensions(); len(dimensions) != 1 || dimensions[0] != column.Name() {
		t.Fatal("named scope lost its declared column dimension")
	}
}

func TestCatalogNamedRowsRefuseUnavailableAndUnknownLabels(t *testing.T) {
	catalog, ok := newCatalogFixture(t).seal(t)
	if !ok {
		t.Fatal("seal fixture catalog")
	}
	for _, lookup := range []func(schema.Key) bool{
		func(name schema.Key) bool { value, ok := catalog.ColumnByName(name); return ok || value.Available() },
		func(name schema.Key) bool { value, ok := catalog.KeyByName(name); return ok || value.Available() },
		func(name schema.Key) bool { value, ok := catalog.ScopeByName(name); return ok || value.Available() },
		func(name schema.Key) bool {
			value, ok := catalog.DenominatorByName(name)
			return ok || value.Available()
		},
	} {
		if lookup("") || lookup("absent") {
			t.Fatal("named catalog lookup accepted an unavailable or unknown label")
		}
	}
	if value, found := (Catalog{}).ColumnByName("address"); found || value.Available() {
		t.Fatal("unavailable catalog resolved a named column")
	}
}
