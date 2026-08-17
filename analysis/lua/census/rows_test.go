package census

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/parsersource"
)

// TestFieldStateRowsEnumerateEveryCarrierStateSpace states the grain law the
// disposition join needs: a carrier is not one row but one row per state its
// declared form admits. A join that could only name the carrier could not state
// that one of its states is parser-impossible while another is ordinary, so a
// carrier with a state space and no state rows is a hole in the denominator.
func TestFieldStateRowsEnumerateEveryCarrierStateSpace(t *testing.T) {
	root := moduleRoot(t)
	value, err := Current(root)
	if err != nil {
		t.Fatal(err)
	}
	projection := Project(value)
	states := make(map[string][]Row)
	carriers := make(map[string]bool)
	for _, row := range projection.Rows {
		if row.Kind == RowCarrier {
			carriers[row.Key] = true
		}
	}
	for _, row := range projection.States {
		if row.Kind != RowFieldState {
			t.Fatalf("state row %s is stated at grain %d", row.Key, row.Kind)
		}
		states[row.Owner] = append(states[row.Owner], row)
	}
	if len(carriers) == 0 {
		t.Fatal("the census states no carriers")
	}
	for _, constructor := range value.Constructors {
		for _, field := range constructor.Fields {
			carrier := CarrierRow(constructor.Name, field.Name)
			want := field.Form.States()
			got := states[carrier]
			if len(got) != len(want) {
				t.Fatalf("carrier %s has %d state rows, its form admits %d", carrier, len(got), len(want))
			}
			present := make(map[parsersource.FieldState]bool, len(got))
			for _, row := range got {
				if row.Key != FieldStateRow(constructor.Name, field.Name, row.State) {
					t.Fatalf("state row %s does not spell its own state %s", row.Key, row.State)
				}
				if present[row.State] {
					t.Fatalf("carrier %s states %s twice", carrier, row.State)
				}
				present[row.State] = true
			}
			for _, state := range want {
				if !present[state] {
					t.Fatalf("carrier %s has no row for state %s", carrier, state)
				}
			}
		}
	}
	for owner := range states {
		if !carriers[owner] {
			t.Fatalf("state rows are owned by %s, which is not a carrier row", owner)
		}
	}
}

// TestFieldStateRowsCoverEverySemanticCarrier states the coverage the join is
// entitled to assume: every field of every semantic form contributes states.
// A form that crosses the semantic boundary with a carrier whose states are
// unenumerated would let a disposition be claimed for a state no row names.
func TestFieldStateRowsCoverEverySemanticCarrier(t *testing.T) {
	root := moduleRoot(t)
	value, err := Current(root)
	if err != nil {
		t.Fatal(err)
	}
	projection := Project(value)
	rows := make(map[string]Row, len(projection.Rows)+len(projection.States))
	for _, row := range projection.Rows {
		rows[row.Key] = row
	}
	for _, row := range projection.States {
		rows[row.Key] = row
	}
	semantic := 0
	for _, constructor := range value.Constructors {
		if !constructor.Semantic {
			continue
		}
		if rows[FormRow(constructor.Name)].Class != constructor.Class {
			t.Fatalf("form %s does not carry its declared class", constructor.Name)
		}
		for _, field := range constructor.Fields {
			states := field.Form.States()
			if len(states) == 0 {
				t.Fatalf("semantic carrier %s.%s has no modelled state space", constructor.Name, field.Name)
			}
			for _, state := range states {
				key := FieldStateRow(constructor.Name, field.Name, state)
				row, exists := rows[key]
				if !exists || row.Kind != RowFieldState {
					t.Fatalf("semantic state row %s is absent from the projection", key)
				}
				semantic++
			}
		}
	}
	if semantic == 0 {
		t.Fatal("the census states no semantic field states")
	}
}

// TestRowKeysAreDisjointByGrain states that the five grains share one key space
// without colliding in it. A collision would silently merge a form with a
// carrier or a carrier with one of its own states, and the join would then be
// counting a denominator it cannot name back.
func TestRowKeysAreDisjointByGrain(t *testing.T) {
	root := moduleRoot(t)
	projection, err := CurrentProjection(root)
	if err != nil {
		t.Fatal(err)
	}
	prefixes := map[RowKind]string{
		RowProduction: "production:",
		RowForm:       "form:",
		RowCarrier:    "carrier:",
		RowFieldState: "state:",
		RowProduct:    "product:",
	}
	seen := make(map[string]RowKind, len(projection.Rows)+len(projection.States)+len(projection.Products))
	counts := make(map[RowKind]int, len(prefixes))
	all := append([]Row(nil), projection.Rows...)
	all = append(all, projection.States...)
	all = append(all, projection.Products...)
	for _, row := range all {
		prefix, known := prefixes[row.Kind]
		if !known {
			t.Fatalf("row %s has no grain", row.Key)
		}
		if !strings.HasPrefix(row.Key, prefix) {
			t.Fatalf("row %s is not spelled at its own grain %s", row.Key, prefix)
		}
		if kind, exists := seen[row.Key]; exists {
			t.Fatalf("row key %s is stated at grains %d and %d", row.Key, kind, row.Kind)
		}
		seen[row.Key] = row.Kind
		counts[row.Kind]++
	}
	for kind := range prefixes {
		if counts[kind] == 0 {
			t.Fatalf("the projection states no rows at grain %d", kind)
		}
	}
}
