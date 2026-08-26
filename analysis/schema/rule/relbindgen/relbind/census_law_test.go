package relbind_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
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
	Sketch string `json:"sketch"`
	Plane  string `json:"plane"`
}

// bound is the set of census planes this corpus is the binding surface for.
//
// A rule family and a seed lower to a reducer, which is what a binding here
// carries. The diagnostic, observation and query planes carry their operations
// on their own schema surfaces - a query fragment declares, binds and recovers
// its own answer - so a row of those planes is bound already and not by this
// corpus. Reading them still matters: it is how the corpus is held to claiming
// only what it carries.
var bound = map[string]bool{"family": true, "seed": true}

// Applies reports whether this row's plan states a semantic operation.
//
// A binding carries one operation. A row whose plan is relational throughout -
// a selection, a join, a completion - states no operation, so there is nothing
// for a binding to carry and a family for it would be a binding over nothing.
// The plan itself says which kind a row is, so the law reads that rather than
// being told per plane.
func (row censusRow) Applies() bool { return strings.Contains(row.Sketch, "Apply(") }

const compiles = "COMPILES"

// planes is every census the corpus is answerable for.
//
// There is more than one, and a law that read only the first left the other
// free to grow rows no binding covers with nothing to say so.
func planes() []string { return []string{"census.json", "census2.json"} }

func census(t testing.TB) []censusRow {
	t.Helper()
	rows := make([]censusRow, 0, 64)
	for _, plane := range planes() {
		rows = append(rows, censusPlane(t, plane)...)
	}
	if len(rows) == 0 {
		t.Fatal("the declaration census is empty")
	}
	return rows
}

func censusPlane(t testing.TB, plane string) []censusRow {
	t.Helper()
	path := filepath.Join(root(t), "analysis", "schema", "rule", "relcompile", "testdata", plane)
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read the declaration census %s: %v", plane, err)
	}
	var rows []censusRow
	if err := json.Unmarshal(content, &rows); err != nil {
		t.Fatalf("decode the declaration census %s: %v", plane, err)
	}
	if len(rows) == 0 {
		t.Fatalf("the declaration census %s is empty", plane)
	}
	return rows
}

// TestEveryCensusPlaneIsRead states that the coverage laws read every plane
// the corpus answers for. A plane added and not listed above would be a plane
// no law covers, which is how the second one went unread.
func TestEveryCensusPlaneIsRead(t *testing.T) {
	directory := filepath.Join(root(t), "analysis", "schema", "rule", "relcompile", "testdata")
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("read the census directory: %v", err)
	}
	read := map[string]bool{}
	for _, plane := range planes() {
		read[plane] = true
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasPrefix(name, "census") || !strings.HasSuffix(name, ".json") {
			continue
		}
		if !read[name] {
			t.Errorf("%s is a census plane and no coverage law reads it", name)
		}
	}
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
	spurious := make([]string, 0, 4)
	for _, row := range census(t) {
		if row.Status != compiles {
			continue
		}
		if bound[row.Plane] && row.Applies() && !declared[address(row)] {
			missing = append(missing, address(row))
		}
		if !row.Applies() && declared[address(row)] {
			spurious = append(spurious, address(row))
		}
	}
	sort.Strings(missing)
	sort.Strings(spurious)
	for _, row := range missing {
		t.Errorf("census row %s states a semantic operation and the corpus declares no family for it", row)
	}
	for _, row := range spurious {
		t.Errorf("census row %s states no semantic operation and the corpus declares a family for it", row)
	}
}

// TestEveryBoundPlaneRowStatesAnOperation states what the bound planes are.
// Every row this corpus answers for lowers to a reducer, so a relational row
// appearing on one of them is a plane boundary moving, and the corpus would be
// the last place to notice.
func TestEveryBoundPlaneRowStatesAnOperation(t *testing.T) {
	for _, row := range census(t) {
		if row.Status != compiles || !bound[row.Plane] || row.Applies() {
			continue
		}
		t.Errorf("census row %s is on the %s plane and states no semantic operation", address(row), row.Plane)
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
