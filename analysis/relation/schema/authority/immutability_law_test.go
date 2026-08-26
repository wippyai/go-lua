package authority

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
)

func TestCatalogCopiesInputsAndExposesDefensiveRows(t *testing.T) {
	fixture := newCatalogFixture(t)
	catalog, ok := fixture.seal(t)
	if !ok {
		t.Fatal("valid fixture was rejected")
	}
	wantDigest := catalog.Digest()

	fixture.relations[0].Addressing[0].Column = "value"
	fixture.columns[0].Name = "mutated-column"
	fixture.keys[0].Columns[0] = "value"
	fixture.scopes[0].Dimensions[0] = "value"
	if catalog.Digest() != wantDigest {
		t.Fatal("input mutation changed catalog digest")
	}
	relation, relationOK := catalog.RelationAt(0)
	if !relationOK {
		t.Fatal("relation disappeared after input mutation")
	}
	if relation.Name() != "orders" || !reflect.DeepEqual(relation.Columns(), []schema.Key{"address", "value"}) || !reflect.DeepEqual(relation.Keys(), []schema.Key{"primary"}) {
		t.Fatalf("input mutation leaked into relation: %+v", relation)
	}

	relations := catalog.Relations()
	columns := relations[0].Columns()
	columns[0] = "tampered"
	addresses := relations[0].Addressing()
	addresses[0].Column = "tampered"
	keys := catalog.Keys()
	keyColumns := keys[0].Columns()
	keyColumns[0] = "tampered"
	scopes := catalog.Scopes()
	dimensions := scopes[0].Dimensions()
	dimensions[0] = "tampered"

	relationAgain, _ := catalog.RelationAt(0)
	keyAgain, _ := catalog.KeyAt(0)
	scopeAgain, _ := catalog.ScopeAt(0)
	if relationAgain.Columns()[0] != "address" || relationAgain.Addressing()[0].Column != "address" || keyAgain.Columns()[0] != "address" || scopeAgain.Dimensions()[0] != "address" {
		t.Fatal("mutation of returned rows changed sealed catalog")
	}
}
