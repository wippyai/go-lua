package census

import (
	"testing"
)

// TestCarrierStateJudgementIsTotal states the shape law of the state-space
// account: every state the denominator holds is decided exactly once, and a
// state the law does not speak about is published as such rather than falling
// through into the impossible set. A judgement that could lose a state would
// let a carrier state enter the language with no account at all.
func TestCarrierStateJudgementIsTotal(t *testing.T) {
	root := moduleRoot(t)
	value, err := Current(root)
	if err != nil {
		t.Fatal(err)
	}
	report := StateSpace(value)
	if err := report.Validate(value); err != nil {
		t.Fatal(err)
	}
	denominator := 0
	for _, constructor := range value.Constructors {
		for _, field := range constructor.Fields {
			denominator += len(field.Form.States())
		}
	}
	if got := len(report.Judged) + len(report.Unjudged); got != denominator {
		t.Fatalf("the account decides %d carrier states, the denominator holds %d", got, denominator)
	}
	if got := len(report.Reachable) + len(report.Impossible); got != len(report.Judged) {
		t.Fatalf("%d judged states split into %d decisions", len(report.Judged), got)
	}
	seen := make(map[string]bool, denominator)
	for _, coordinate := range append(append([]CarrierState(nil), report.Reachable...), report.Impossible...) {
		if seen[coordinate.Key()] {
			t.Fatalf("carrier state %s is decided twice", coordinate.Key())
		}
		seen[coordinate.Key()] = true
	}
	if len(report.Unjudged) == 0 {
		t.Fatal("no state is published unjudged, so the split the law depends on is untested")
	}
	for _, coordinate := range report.Unjudged {
		if seen[coordinate.Key()] {
			t.Fatalf("carrier state %s is both judged and unjudged", coordinate.Key())
		}
	}
}

// TestParserImpossibilityIsDerivedFromProductRows states that the judgement is
// a function of the census rows and nothing else. Removing the constructions of
// one form must make that form's states impossible: a report that stayed the
// same would be reading something other than the census.
func TestParserImpossibilityIsDerivedFromProductRows(t *testing.T) {
	root := moduleRoot(t)
	value, err := Current(root)
	if err != nil {
		t.Fatal(err)
	}
	const form = "WhileStmt"
	before := impossibleFields(StateSpace(value), form)
	if len(before) != 1 {
		t.Fatalf("%s states %d impossible carriers before the row removal, want 1", form, len(before))
	}
	stripped := clone(value)
	stripped.Products = nil
	removed := 0
	for _, product := range value.Products {
		if product.Constructor == form {
			removed++
			continue
		}
		stripped.Products = append(stripped.Products, product)
	}
	if removed == 0 {
		t.Fatalf("no product row builds %s", form)
	}
	after := impossibleFields(StateSpace(stripped), form)
	if len(after) <= len(before) {
		t.Fatalf("%s states %d impossible carriers with its constructions removed, want more than %d", form, len(after), len(before))
	}
}

func impossibleFields(report StateSpaceReport, form string) []CarrierState {
	var result []CarrierState
	for _, coordinate := range report.Impossible {
		if coordinate.Form == form {
			result = append(result, coordinate)
		}
	}
	return result
}

// TestProductRowsCarryTheirConstructorAndCarriers states the projection law of
// the product grain: a product row names the form row it builds and the carrier
// rows it fills, both in the neutral vocabulary the join consumes, so a join
// over this grain never has to re-derive containment from a key spelling.
func TestProductRowsCarryTheirConstructorAndCarriers(t *testing.T) {
	root := moduleRoot(t)
	projection, err := CurrentProjection(root)
	if err != nil {
		t.Fatal(err)
	}
	forms := make(map[string]bool, len(projection.Rows))
	carriers := make(map[string]bool, len(projection.Rows))
	for _, row := range projection.Rows {
		switch row.Kind {
		case RowForm:
			forms[row.Key] = true
		case RowCarrier:
			carriers[row.Key] = true
		}
	}
	if len(projection.Products) == 0 {
		t.Fatal("the projection states no product rows")
	}
	filled := 0
	for _, row := range projection.Products {
		if row.Kind != RowProduct {
			t.Fatalf("product row %s is stated at grain %d", row.Key, row.Kind)
		}
		if !forms[row.Constructs] {
			t.Fatalf("product row %s builds %s, which is not a form row", row.Key, row.Constructs)
		}
		for _, carrier := range row.Builds {
			if !carriers[carrier] {
				t.Fatalf("product row %s fills %s, which is not a carrier row", row.Key, carrier)
			}
			filled++
		}
	}
	if filled == 0 {
		t.Fatal("no product row fills a carrier")
	}
}
