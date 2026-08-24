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

// TestPlanRetainsTheDeclaredCandidateSource proves the subscription transports
// the rule's candidate source as the sealed relation row rather than as a key
// a later stage must resolve again.
func TestPlanRetainsTheDeclaredCandidateSource(t *testing.T) {
	sealed, failure := sealTable(t, canonicalEntries(t)...)
	if failure.Available() {
		t.Fatalf("canonical surface refused: %+v", failure)
	}
	view, viewOK := sealed.Surface(NewSurface(nil).Kind())
	table, tableOK := NewTable(view)
	plan, planOK := NewPlan(table, []SubscriptionSpec{{
		Family: "family/occurrence", Requirement: "requirement/unrestricted",
		Form: "form/local", Rule: "rule/law", Writes: "axis/law",
		Source: "relation/occurrence-call",
	}})
	if !viewOK || !tableOK || !planOK || plan.Count() != 1 {
		t.Fatal("plan refused a declared candidate source")
	}
	row, rowOK := plan.At(0)
	if !rowOK || row.Source() == nil || row.Source().Key() != "relation/occurrence-call" {
		t.Fatalf("plan lost the candidate source: %+v", row.Source())
	}
	if row.Source().Space() != row.Family().Space() {
		t.Fatalf("candidate source space %s does not root in the family space %s", row.Source().Space(), row.Family().Space())
	}
}

// TestPlanRefusesACandidateSourceRootedElsewhere states the one law the source
// adds: a rule reaches its candidate from the rows it is issued over. A
// relation rooted in another space would hand it a row no occurrence of its
// family can reach.
func TestPlanRefusesACandidateSourceRootedElsewhere(t *testing.T) {
	entries := canonicalEntries(t)
	entries = append(entries, mustEntry(t, Spec{
		Key: "relation/call-geometry", Kind: KindRelation, Ordinal: 3, Space: "row/call", Target: "row/geometry", Cardinality: CardinalityOptional,
		Joins:   []JoinField{{Source: "field/call-id", Target: "field/geometry-occurrence-id", Missing: JoinMissingNoEdge}},
		Program: Program{{Op: OpLiteral, Out: 1, Type: BoolType(), Literal: 1}}, Result: 1,
	}))
	sealed, failure := sealTable(t, entries...)
	if failure.Available() {
		t.Fatalf("extended surface refused: %+v", failure)
	}
	view, viewOK := sealed.Surface(NewSurface(nil).Kind())
	table, tableOK := NewTable(view)
	spec := SubscriptionSpec{
		Family: "family/occurrence", Requirement: "requirement/unrestricted",
		Form: "form/local", Rule: "rule/law", Writes: "axis/law",
	}
	spec.Source = "relation/call-geometry"
	if _, planOK := NewPlan(table, []SubscriptionSpec{spec}); !viewOK || !tableOK || planOK {
		t.Fatal("a candidate source rooted in a foreign row space was admitted")
	}
	spec.Source = "family/occurrence"
	if _, planOK := NewPlan(table, []SubscriptionSpec{spec}); planOK {
		t.Fatal("a candidate source naming a non-relation entry was admitted")
	}
}
