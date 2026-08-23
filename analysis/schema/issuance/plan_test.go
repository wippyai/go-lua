package issuance

import "testing"

func TestPlanRetainsSealedDeclarationsWithoutOrdinalProjection(t *testing.T) {
	sealed, failure := sealTable(t, canonicalEntries(t)...)
	if failure.Available() {
		t.Fatalf("canonical surface refused: %+v", failure)
	}
	view, viewOK := sealed.Surface(NewSurface(nil).Kind())
	table, tableOK := NewTable(view)
	plan, planOK := NewPlan(table, []SubscriptionSpec{{
		Family: "family/occurrence", Requirement: "requirement/unrestricted",
		Form: "form/local", Rule: "rule/law", Writes: "axis/law",
	}})
	if !viewOK || !tableOK || !planOK || plan.Count() != 1 {
		t.Fatal("plan failed to retain the sealed family/requirement/form rows")
	}
	row, rowOK := plan.At(0)
	if !rowOK || row.Family().Key() != "family/occurrence" ||
		row.Requirement().Key() != "requirement/unrestricted" ||
		row.Form().Key() != "form/local" {
		t.Fatal("plan projected or lost a sealed declaration")
	}
}

func TestPlanRefusesDuplicateSubscriptionAuthority(t *testing.T) {
	sealed, failure := sealTable(t, canonicalEntries(t)...)
	if failure.Available() {
		t.Fatalf("canonical surface refused: %+v", failure)
	}
	view, viewOK := sealed.Surface(NewSurface(nil).Kind())
	table, tableOK := NewTable(view)
	spec := SubscriptionSpec{
		Family: "family/occurrence", Requirement: "requirement/unrestricted",
		Form: "form/local", Rule: "rule/law", Writes: "axis/law",
	}
	if _, planOK := NewPlan(table, []SubscriptionSpec{spec, spec}); !viewOK || !tableOK || planOK {
		t.Fatal("duplicate subscription tuple acquired a second receipt authority")
	}
}
