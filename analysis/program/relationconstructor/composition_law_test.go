package relationconstructor

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/rule"
)

func catalog(keys ...schema.Key) []rule.Spec {
	specs := make([]rule.Spec, 0, len(keys))
	for _, key := range keys {
		specs = append(specs, rule.Spec{Key: key})
	}
	return specs
}

// TestAnExcludedRuleDoesNotRenumberTheRulesAfterIt is the ordinal identity
// law. A rule's role is its declaration position, and the relation input
// bundle states one row per dense catalog ordinal, so filtering must carry the
// position each admitted rule was declared at. If the filter renumbered its
// survivors, every rule after an excluded one would answer under the identity
// of a different rule.
func TestAnExcludedRuleDoesNotRenumberTheRulesAfterIt(t *testing.T) {
	specs := catalog("a", "b", "c", "d")
	composition, ok := NewComposition("a", "c", "d")
	if !ok {
		t.Fatal("the composition refused three declared names")
	}
	selected, absent, ok := composition.Select(specs)
	if !ok || len(absent) != 0 {
		t.Fatalf("selection refused a fitting composition: absent=%v ok=%v", absent, ok)
	}
	want := []Selection{{Ordinal: 0, Spec: specs[0]}, {Ordinal: 2, Spec: specs[2]}, {Ordinal: 3, Spec: specs[3]}}
	if len(selected) != len(want) {
		t.Fatalf("selected %d rules, want %d", len(selected), len(want))
	}
	for index, expected := range want {
		if selected[index].Ordinal != expected.Ordinal || selected[index].Spec.Key != expected.Spec.Key {
			t.Fatalf("selection %d is %s at ordinal %d, want %s at ordinal %d",
				index, selected[index].Spec.Key, selected[index].Ordinal, expected.Spec.Key, expected.Ordinal)
		}
	}
}

// TestATotalCompositionAdmitsTheWholeCatalogInOrder states that Everything is
// defined by the catalog rather than by a list, so it stays total over a
// catalog it never named.
func TestATotalCompositionAdmitsTheWholeCatalogInOrder(t *testing.T) {
	specs := catalog("a", "b", "c")
	composition := Everything()
	if !composition.Available() || !composition.Total() || len(composition.Names()) != 0 {
		t.Fatal("a total composition names rules of its own")
	}
	selected, absent, ok := composition.Select(specs)
	if !ok || len(absent) != 0 || len(selected) != len(specs) {
		t.Fatalf("total selection: selected=%d absent=%v ok=%v", len(selected), absent, ok)
	}
	for index, selection := range selected {
		if selection.Ordinal != index || selection.Spec.Key != specs[index].Key {
			t.Fatalf("total selection %d is %s at ordinal %d", index, selection.Spec.Key, selection.Ordinal)
		}
	}
}

// TestACompositionNamingAnUndeclaredRuleReportsItByName is the honest absence
// law: a composition that does not fit its catalog states the exact names it
// asked for and did not get, rather than producing a quietly smaller program.
func TestACompositionNamingAnUndeclaredRuleReportsItByName(t *testing.T) {
	specs := catalog("a", "b")
	composition, ok := NewComposition("a", "missing", "absent")
	if !ok {
		t.Fatal("the composition refused its names")
	}
	selected, absent, ok := composition.Select(specs)
	if !ok {
		t.Fatal("selection refused a well formed catalog")
	}
	if len(selected) != 1 || selected[0].Spec.Key != "a" || selected[0].Ordinal != 0 {
		t.Fatalf("selection admitted %d rules", len(selected))
	}
	if len(absent) != 2 || absent[0] != "absent" || absent[1] != "missing" {
		t.Fatalf("absent names are %v, want sorted [absent missing]", absent)
	}
}

// TestAMalformedCompositionOrCatalogRefuses states that construction inputs
// are checked at the boundary: neither an unusable composition nor a catalog
// that declares one rule twice becomes an empty selection.
func TestAMalformedCompositionOrCatalogRefuses(t *testing.T) {
	if _, ok := NewComposition(); ok {
		t.Fatal("an empty composition was admitted")
	}
	if _, ok := NewComposition("a", "a"); ok {
		t.Fatal("a duplicated name was admitted")
	}
	if _, ok := NewComposition("a", ""); ok {
		t.Fatal("an unavailable name was admitted")
	}
	var unavailable Composition
	if unavailable.Available() || unavailable.Admits("a") {
		t.Fatal("the zero composition admits a rule")
	}
	if _, _, ok := unavailable.Select(catalog("a")); ok {
		t.Fatal("the zero composition selected")
	}
	composition, ok := NewComposition("a")
	if !ok {
		t.Fatal("the composition refused one name")
	}
	if _, _, ok := composition.Select(catalog("a", "a")); ok {
		t.Fatal("a catalog declaring one rule twice was admitted")
	}
	if _, _, ok := composition.Select(catalog("a", "")); ok {
		t.Fatal("a catalog with an unavailable rule key was admitted")
	}
}
