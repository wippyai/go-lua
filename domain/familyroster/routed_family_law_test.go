package familyroster_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/domain/familyroster"
)

// emittedFamily reads one rostered rule's generated family. The freshness law
// above already holds that file to the declaration, so reading it is reading
// what the declaration derives.
func emittedFamily(t *testing.T, key schema.Key) string {
	t.Helper()
	root := moduleRoot(t)
	for _, family := range familyroster.Families() {
		if family.Key() != key {
			continue
		}
		source, err := os.ReadFile(filepath.Join(root, family.Directory, familyroster.GeneratedFileName))
		if err != nil {
			t.Fatalf("%s: %v", string(key), err)
		}
		return string(source)
	}
	t.Fatalf("no rostered family is emitted for rule %s", string(key))
	return ""
}

// TestARoutedWorkerDerivesItsRouteSetFromTheDeclaredRelation is the whole
// payoff of the routed shape, measured on Store.
//
// The member vector a routed row publishes at is the declared relation's own
// answer - Build, Count, At - addressed through the declared key and tag
// projections and the written axis's own key normalizer. A family that walked
// a geometry of its own would be a second derivation of the relation the
// declaration already names, and the two would disagree the moment either
// moved.
//
// Store's relation is a DECLARED derivation, so Build, Count and At are the
// three the emitter generates for join 1 rather than three the owner authored.
// Which of the two a relation uses is the declaration's own statement, and the
// law holds the same thing either way: the worker calls the relation's Build
// once with the carriers the declaration names, and reads every row back
// through that relation's own Count and At.
func TestARoutedWorkerDerivesItsRouteSetFromTheDeclaredRelation(t *testing.T) {
	source := emittedFamily(t, "placement-storage")
	for _, required := range []string{
		"deriveDerived1Rows(lane.family.placementSchema, lane.family.valueSchema, row.candidate, input0)",
		"count := derived1Count(derived)",
		"selected, selectedOK := derived1At(derived, index)",
		"selected.Coordinates()",
		"selected.Predicate()",
		"lane.family.placementSchema.KeyIndex(storageRouteKey)",
		"lane.family.plane.RouteMember(uint32(dense), uint32(destinationDense), uint64(storageRouteTag))",
	} {
		if !strings.Contains(source, required) {
			t.Errorf("the routed worker does not derive its route set through %q", required)
		}
	}
}

// TestARoutedFoldReceivesTheObservedCellAndItsTag holds the emitted fold to
// the correlation the read already established. The fact and the tag are the
// two halves of one observed cell, so neither is reconstructed beside it, and
// the prerequisite the declaration named is the only other carrier passed.
func TestARoutedFoldReceivesTheObservedCellAndItsTag(t *testing.T) {
	source := emittedFamily(t, "placement-storage")
	if !strings.Contains(source, "return StorageFold(fold.candidate, fold.input0, cell.Tag, cell.Value)") {
		t.Error("the routed fold is not called with the candidate, the declared prerequisite, and the observed cell and its tag")
	}
	if !strings.Contains(source, "func (familyReducer) Empty() structure.ReductionOutcome { return structure.NoSelection }") {
		t.Error("the routed fold does not settle an empty derived relation as an empty selection")
	}
}

// TestAnExactPrerequisiteSettlesAbsenceAsAnAbsentCandidate is the reading of
// explicit sparsity at an exact prerequisite read, and it is the one place
// that reading is now stated.
//
// A cursor that answered no cell is unavailable and refuses; a coordinate the
// Factor never wrote is delivered as absence, and a rule that declared
// explicit sparsity has said that absence is not a candidate. Neither is a
// judgment the fold makes: the fold is reached only with a present cell, which
// is why the emitted reducer takes no presence bit. This law carries the
// statement Store's own source gate used to make from beside its family.
func TestAnExactPrerequisiteSettlesAbsenceAsAnAbsentCandidate(t *testing.T) {
	source := emittedFamily(t, "placement-storage")
	body, found := functionBody(source, "func (lane *familyWorker) read0Cell(")
	if !found {
		t.Fatal("the routed worker emits no exact prerequisite cursor")
	}
	for _, required := range []string{
		"if !available {\n\t\t\treturn structure.Refuse\n\t\t}",
		"if !present {\n\t\t\treturn structure.NoCandidate\n\t\t}",
		"case execution.ReadExhausted:",
	} {
		if !strings.Contains(body, required) {
			t.Errorf("the exact prerequisite cursor does not state %q", required)
		}
	}
	if !strings.Contains(body, "if !read.Close(ticket, &lane.read0) {") {
		t.Error("the exact prerequisite cursor is not closed before its cell is used")
	}
}

// TestAnEmittedInvocationAllocatesNothing is the allocation floor. Every
// buffer a routed invocation observes into is sized once, when the family is
// sealed, at the width of the selection the rule spans; a warm invocation
// opens its cursors and its write transaction and allocates nothing of its
// own.
func TestAnEmittedInvocationAllocatesNothing(t *testing.T) {
	for _, family := range familyroster.Families() {
		source := emittedFamily(t, family.Key())
		body, found := functionBody(source, "func (lane *familyWorker) Execute(")
		if !found {
			t.Fatalf("%s: the emitted worker declares no Execute", string(family.Key()))
		}
		if strings.Contains(body, "make(") {
			t.Errorf("%s: an emitted invocation allocates", string(family.Key()))
		}
	}
	routed := emittedFamily(t, "placement-storage")
	if !strings.Contains(routed, "members: make([]execution.RouteMember, sealed.width)") {
		t.Error("the routed family does not size its member buffer at the sealed width")
	}
}

// TestAnEmittedFamilyReachesNoOwnerCallback keeps the emitted half free of the
// two things a generated family may never acquire: a live capability, and a
// second copy of the plan geometry the row already carries.
func TestAnEmittedFamilyReachesNoOwnerCallback(t *testing.T) {
	for _, family := range familyroster.Families() {
		source := emittedFamily(t, family.Key())
		for _, forbidden := range []string{"func(", "interface{", "reflect."} {
			if strings.Contains(source, forbidden) {
				t.Errorf("%s: the emitted family contains %q", string(family.Key()), forbidden)
			}
		}
	}
}

// functionBody answers the body of the function whose declaration begins with
// signature.
func functionBody(source, signature string) (string, bool) {
	start := strings.Index(source, signature)
	if start < 0 {
		return "", false
	}
	rest := source[start:]
	end := strings.Index(rest, "\n}\n")
	if end < 0 {
		return "", false
	}
	return rest[:end], true
}
