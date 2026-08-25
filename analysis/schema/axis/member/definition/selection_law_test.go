package definition

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
)

// selectionContribution is one rule declaring the operation that publishes its
// produced rows beside the relation those rows land in and the tag it stamps.
func selectionContribution() Contribution {
	contribution := routedContribution("specimen-selected", "Routes", "specimen/routes")
	contribution.Projections = append(contribution.Projections, Projection{
		Name:              "RouteTag",
		Key:               "specimen/routes/tag",
		Relation:          "Routes",
		CandidateProvider: contribution.Relations[0].CandidateProvider,
		Role:              member.Predicate,
		Result:            "KeyCarrier",
		Accessor:          specimenMethod("Tag", "Route"),
	})
	contribution.Selections = []Selection{{
		Name:           "RouteSelection",
		Key:            "specimen/routes/selection",
		Relation:       "Routes",
		Tag:            "RouteTag",
		Implementation: GoSymbol{PackagePath: specimenPackage, Name: "ResolveRoute"},
	}}
	return contribution
}

// TestASelectionReachesTheSealedCatalogAsAMember states the lane end to end: a
// rule declares the operation that publishes its produced rows beside the rows
// themselves, and the sealed catalog carries it resolved to the relation and
// tag keys its owner issued.
func TestASelectionReachesTheSealedCatalogAsAMember(t *testing.T) {
	composed, ok := specimenSource(selectionContribution()).Compose()
	if !ok {
		t.Fatal("composing the axis with a selection failed")
	}
	if len(composed.Selections) != 1 {
		t.Fatalf("composed selections = %d, want the one the rule declared", len(composed.Selections))
	}
	catalog, ok := composed.Catalog()
	if !ok {
		t.Fatal("the sealed catalog refused the selection")
	}
	sealed, found := catalog.Selection("specimen/routes/selection")
	if !found {
		t.Fatal("the sealed catalog does not resolve the selection by its owner-issued key")
	}
	if sealed.Relation != "specimen/routes" || sealed.Tag != "specimen/routes/tag" {
		t.Fatalf("sealed selection = %+v, want the relation and tag keys the owner issued", sealed)
	}
}

// TestASelectionNamingRowsItsAxisDoesNotDeclareRefuses states that the
// operation is resolved against the same definition its rows live in, so a
// selection can never reach a sealed catalog naming a relation or a tag
// nobody declared. Composition may refuse it first; what the law fixes is
// that no such selection is ever sealed.
func TestASelectionNamingRowsItsAxisDoesNotDeclareRefuses(t *testing.T) {
	for label, mutate := range map[string]func(*Contribution){
		"unknown relation": func(c *Contribution) { c.Selections[0].Relation = "Absent" },
		"unknown tag":      func(c *Contribution) { c.Selections[0].Tag = "Absent" },
	} {
		contribution := selectionContribution()
		mutate(&contribution)
		composed, composeOK := specimenSource(contribution).Compose()
		if !composeOK {
			continue
		}
		if _, ok := composed.Catalog(); ok {
			t.Fatalf("%s: a selection over rows its axis does not declare reached a sealed catalog", label)
		}
	}
}

// TestAnIncompleteSelectionRefusesAtItsContribution states the row is whole
// where it is written: a rule that names an operation without saying what it
// publishes or what it stamps is refused at the contribution.
func TestAnIncompleteSelectionRefusesAtItsContribution(t *testing.T) {
	for label, mutate := range map[string]func(*Contribution){
		"no key":            func(c *Contribution) { c.Selections[0].Key = schema.Key("") },
		"no relation":       func(c *Contribution) { c.Selections[0].Relation = "" },
		"no tag":            func(c *Contribution) { c.Selections[0].Tag = "" },
		"no implementation": func(c *Contribution) { c.Selections[0].Implementation = GoSymbol{} },
	} {
		contribution := selectionContribution()
		mutate(&contribution)
		if contribution.Available() {
			t.Fatalf("%s: an incomplete selection was admitted at its contribution", label)
		}
	}
}
