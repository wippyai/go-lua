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
		Name:     "RouteSelection",
		Key:      "specimen/routes/selection",
		Relation: "Routes",
		Tag:      "RouteTag",
	}}
	return contribution
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
// where it is written: a rule that names a selection without saying what it
// publishes or what it stamps is refused at the contribution.
func TestAnIncompleteSelectionRefusesAtItsContribution(t *testing.T) {
	for label, mutate := range map[string]func(*Contribution){
		"no key":      func(c *Contribution) { c.Selections[0].Key = schema.Key("") },
		"no relation": func(c *Contribution) { c.Selections[0].Relation = "" },
		"no tag":      func(c *Contribution) { c.Selections[0].Tag = "" },
	} {
		contribution := selectionContribution()
		mutate(&contribution)
		if contribution.Available() {
			t.Fatalf("%s: an incomplete selection was admitted at its contribution", label)
		}
	}
}
