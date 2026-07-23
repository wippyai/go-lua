package operationplan

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
)

func TestPublicKindCatalogsAreCanonicalDefensiveViews(t *testing.T) {
	facts := Kinds()
	if len(facts) != len(descriptors) {
		t.Fatalf("Kinds has %d entries, want %d", len(facts), len(descriptors))
	}
	for i, descriptor := range descriptors {
		if facts[i] != descriptor.kind {
			t.Fatalf("Kinds[%d]=%d want %d", i, facts[i], descriptor.kind)
		}
	}
	facts[0] = 0
	if Kinds()[0] == 0 {
		t.Fatal("mutating Kinds result changed catalog")
	}
	extensions := ExtensionKinds()
	if !reflect.DeepEqual(extensions, extensionKinds[:]) {
		t.Fatalf("ExtensionKinds=%v want %v", extensions, extensionKinds)
	}
	extensions[0] = 0
	if ExtensionKinds()[0] == 0 {
		t.Fatal("mutating ExtensionKinds result changed catalog")
	}
}

func TestDependencyCursorReportsPresentFamiliesInCatalogOrder(t *testing.T) {
	plan := New(nil, factflow.FactsInput{ExpressionValues: map[factflow.ExprRef]product.Value{2: product.Top()}, ObjectLiterals: map[factflow.ExprRef]factflow.ObjectLiteral{1: {}}})
	cursor := plan.DependencyCursor()
	var got []Kind
	for kind, ok := cursor.Next(); ok; kind, ok = cursor.Next() {
		got = append(got, kind)
	}
	want := []Kind{ObjectLiteral, ExpressionValue}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("dependencies=%v want %v", got, want)
	}
}
