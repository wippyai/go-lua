package census

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/parsersource"
)

// TestUseSlotRowsCloseOverTheCensus states the projection law of the new grain.
// A slot row is a refinement of a carrier row and its fills are product rows, so
// a slot naming a carrier the census does not state, or a fill naming a
// construction it does not state, would publish an edge into a denominator this
// census does not own.
func TestUseSlotRowsCloseOverTheCensus(t *testing.T) {
	root := moduleRoot(t)
	projection, err := CurrentProjection(root)
	if err != nil {
		t.Fatal(err)
	}
	carriers := make(map[string]bool, len(projection.Rows))
	forms := make(map[string]parsersource.ConstructorClass, len(projection.Rows))
	for _, row := range projection.Rows {
		switch row.Kind {
		case RowCarrier:
			carriers[row.Key] = true
		case RowForm:
			forms[row.Key] = row.Class
		}
	}
	products := make(map[string]bool, len(projection.Products))
	for _, row := range projection.Products {
		products[row.Key] = true
	}
	if len(projection.Uses) == 0 {
		t.Fatal("the census publishes no use slot")
	}
	filled := 0
	for _, row := range projection.Uses {
		if row.Kind != RowUseSlot {
			t.Fatalf("use row %s is stated at grain %d", row.Key, row.Kind)
		}
		if !carriers[row.Owner] {
			t.Fatalf("use slot %s refines %s, which is no carrier row", row.Key, row.Owner)
		}
		if row.Accepts == "" || row.AcceptsClass == 0 {
			t.Fatalf("use slot %s accepts %q of class %d", row.Key, row.Accepts, row.AcceptsClass)
		}
		// An abstract child stands for a class of forms and has no form row of
		// its own. A concrete one does, and the two readings of its class must
		// be the same reading.
		if class, stated := forms[FormRow(row.Accepts)]; stated && class != row.AcceptsClass {
			t.Fatalf("use slot %s accepts %s at class %d, its form row states %d", row.Key, row.Accepts, row.AcceptsClass, class)
		}
		for _, fill := range row.FilledBy {
			if !products[fill] {
				t.Fatalf("use slot %s is filled by %s, which is no product row", row.Key, fill)
			}
		}
		if len(row.FilledBy) != 0 {
			filled++
		}
	}
	if filled == 0 {
		t.Fatal("no slot is filled by any construction, so the fill column proves nothing")
	}
}

// TestUnfilledSlotsAreNamed states what an empty fill column means. A slot the
// declarations admit and no action fills is evidence, not an omission: it is a
// coordinate the language declares and this parser never reaches. Each is named
// here with the reason it stands, so a slot that stops being filled cannot pass
// as one that never was.
func TestUnfilledSlotsAreNamed(t *testing.T) {
	root := moduleRoot(t)
	projection, err := CurrentProjection(root)
	if err != nil {
		t.Fatal(err)
	}
	// A mutation reaches these two coordinates: the parser constructs the form
	// first and assigns the carrier afterwards, so no construction of it names
	// the coordinate and the census states the fill in its mutation grain.
	assigned := map[string]string{
		"use:IfStmt.Else":                "assigned onto an already constructed statement",
		"use:IntersectionTypeExpr.Types": "assigned onto an already constructed type expression",
		"use:UnionTypeExpr.Types":        "assigned onto an already constructed type expression",
	}
	mutated := make(map[string]bool, len(assigned))
	value, err := Current(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, mutation := range value.Mutations {
		mutated[UseSlotRow(mutation.Constructor, mutation.Field)] = true
	}
	var unaccounted []string
	for _, row := range projection.Uses {
		if len(row.FilledBy) != 0 {
			continue
		}
		if _, named := assigned[row.Key]; named {
			if !mutated[row.Key] {
				t.Fatalf("slot %s is named as filled by assignment and no mutation reaches it", row.Key)
			}
			continue
		}
		unaccounted = append(unaccounted, row.Key)
	}
	sort.Strings(unaccounted)
	if len(unaccounted) != 0 {
		t.Fatalf("the census admits slots no action fills and no account names: %v", unaccounted)
	}
}

// TestDriftGuardRejectsARelocatedOperand is the drift guard of the consumption
// grain. Exchanging the two operands a binary expression is built from leaves
// every other grain identical - the same alternative, the same form, the same
// carriers, the same whole-constructor field vector - and changes only which
// operand each slot receives. A census that did not close over the edges would
// keep describing a parser that consumes its operands the other way round.
func TestDriftGuardRejectsARelocatedOperand(t *testing.T) {
	root := moduleRoot(t)
	copied := copyParserSources(t, root)
	if err := Generated.Validate(copied); err != nil {
		t.Fatalf("census rejected an unmodified copy of the parser sources: %v", err)
	}
	grammarPath := filepath.Join(copied, "compiler", "parse", "parser.go.y")
	contents, err := os.ReadFile(grammarPath)
	if err != nil {
		t.Fatal(err)
	}
	const original = `$$ = &ast.ArithmeticOpExpr{Lhs: $1, Operator: "+", Rhs: $3}`
	const edited = `$$ = &ast.ArithmeticOpExpr{Lhs: $3, Operator: "+", Rhs: $1}`
	if strings.Count(string(contents), original) != 1 {
		t.Fatalf("parser.go.y does not state %q exactly once", original)
	}
	mutated := strings.Replace(string(contents), original, edited, 1)
	if err := os.WriteFile(grammarPath, []byte(mutated), 0o644); err != nil {
		t.Fatal(err)
	}
	edit, err := Build(copied)
	if err != nil {
		t.Fatal(err)
	}
	current, err := Current(root)
	if err != nil {
		t.Fatal(err)
	}
	// The grains that do not see the edit are stated first, so the rejection
	// below is attributable to the consumption grain alone.
	if !sameProducts(current, edit) {
		t.Fatal("exchanging two operands changed the product grain, so this edit does not isolate the consumption grain")
	}
	if len(current.Slots) != len(edit.Slots) {
		t.Fatal("exchanging two operands changed the slot denominator")
	}
	if err := Generated.Validate(copied); err == nil {
		t.Fatal("census accepted an operand relocation it was not generated from")
	}
}

func sameProducts(left, right Census) bool {
	if len(left.Products) != len(right.Products) {
		return false
	}
	for index, product := range left.Products {
		other := right.Products[index]
		if product.Owner != other.Owner || product.Ordinal != other.Ordinal || product.Constructor != other.Constructor || len(product.Fields) != len(other.Fields) {
			return false
		}
		for coordinate, field := range product.Fields {
			if field.Field != other.Fields[coordinate].Field || field.Assigned != other.Fields[coordinate].Assigned {
				return false
			}
			if len(field.States) != len(other.Fields[coordinate].States) {
				return false
			}
			for state, value := range field.States {
				if value != other.Fields[coordinate].States[state] {
					return false
				}
			}
		}
	}
	return true
}
