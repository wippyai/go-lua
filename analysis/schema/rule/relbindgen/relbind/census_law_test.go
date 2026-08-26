package relbind_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/rule/relbindgen/relbind"
)

// censusRow is one row of the declaration census. The corpus reads the census
// rather than restating it, so a family that starts compiling is a binding
// this corpus owes and not a fact two files can disagree about.
type censusRow struct {
	Family string `json:"family"`
	Rule   string `json:"rule"`
	Status string `json:"status"`
}

const compiles = "COMPILES"

func census(t testing.TB) []censusRow {
	t.Helper()
	path := filepath.Join(root(t), "analysis", "schema", "rule", "relcompile", "testdata", "census.json")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read the declaration census: %v", err)
	}
	var rows []censusRow
	if err := json.Unmarshal(content, &rows); err != nil {
		t.Fatalf("decode the declaration census: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("the declaration census is empty")
	}
	return rows
}

func address(row censusRow) string { return row.Family + "#" + row.Rule }

// TestEveryCompilingCensusRowIsDeclared is the coverage law. The binding
// surface is total over what the census proves lowers: a family that compiles
// and has no row here is a hole the next layer would build on.
func TestEveryCompilingCensusRowIsDeclared(t *testing.T) {
	declared := map[string]bool{}
	for _, family := range relbind.Declared().Families {
		if family.Census == "" {
			continue
		}
		declared[family.Census+"#"+family.Rule] = true
	}
	missing := make([]string, 0, 4)
	for _, row := range census(t) {
		if row.Status != compiles || declared[address(row)] {
			continue
		}
		missing = append(missing, address(row))
	}
	sort.Strings(missing)
	for _, row := range missing {
		t.Errorf("census row %s compiles and the corpus declares no family for it", row)
	}
}

// TestEveryDeclaredCensusFamilyAnswersACompilingRow states the converse. A
// binding for a row the census does not prove lowers is a binding for a plan
// no owner declared.
func TestEveryDeclaredCensusFamilyAnswersACompilingRow(t *testing.T) {
	compiling := map[string]bool{}
	for _, row := range census(t) {
		if row.Status == compiles {
			compiling[address(row)] = true
		}
	}
	for _, family := range relbind.Declared().Families {
		if family.Census == "" {
			continue
		}
		if !compiling[family.Census+"#"+family.Rule] {
			t.Errorf("family %s answers census row %s/%s, which does not compile", family.Stem, family.Census, family.Rule)
		}
	}
}

// TestEveryUnboundFamilyNamesWhatItOwes states how a hole is carried. A row
// the corpus cannot bind is declared, named, and reported with the reason; it
// is never quietly absent, and it is never bound with an invented token.
func TestEveryUnboundFamilyNamesWhatItOwes(t *testing.T) {
	pending := 0
	for _, family := range relbind.Declared().Families {
		if family.Emitted() {
			continue
		}
		pending++
		if family.Census == "" {
			t.Errorf("family %s is unbound and answers no census row", family.Stem)
		}
		if len(family.Pending) < 12 {
			t.Errorf("family %s is unbound and does not say what it owes", family.Stem)
		}
	}
	t.Logf("families declared unbound: %d", pending)
}

// TestEveryDeclaredFamilyIsEmittedOrPending states there is no third state. A
// family is either a checked-in binding or a named debt.
func TestEveryDeclaredFamilyIsEmittedOrPending(t *testing.T) {
	corpus := relbind.Declared()
	artifacts, err := relbind.Emit(corpus)
	if err != nil {
		t.Fatalf("emit: %v", err)
	}
	emitted := map[string]bool{}
	for _, artifact := range artifacts {
		emitted[artifact.Name] = true
	}
	bound := 0
	for _, family := range corpus.Families {
		if !family.Emitted() {
			if emitted[relbind.BindingFile(family)] {
				t.Errorf("family %s is declared unbound and was emitted anyway", family.Stem)
			}
			continue
		}
		bound++
		if !emitted[relbind.BindingFile(family)] {
			t.Errorf("family %s is declared bound and emitted no artifact", family.Stem)
		}
	}
	t.Logf("families bound: %d", bound)
}

// TestEveryClassArmOfTheAbiIsDeclared states the corpus exercises the whole
// semantic ABI and not the easy half of it. The four classes are not four
// runtime forms, so what a class arm proves is that one declaration shape
// reaches the substrate and comes back with the right row geometry.
func TestEveryClassArmOfTheAbiIsDeclared(t *testing.T) {
	arms := map[string]bool{
		"scalar judgment at the row it read":     false,
		"grouped reduction over a complete span": false,
		"finite expansion at owner-named rows":   false,
		"cell update at the row it read":         false,
	}
	for _, family := range relbind.Declared().Families {
		if !family.Emitted() {
			continue
		}
		if family.Arm != "" {
			arms[family.Arm] = true
			continue
		}
		arms["scalar judgment at the row it read"] = true
	}
	for arm, proven := range arms {
		if !proven {
			t.Errorf("no declared family proves the %s arm", arm)
		}
	}
}
