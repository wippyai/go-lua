package conformance

import (
	"testing"

	"github.com/wippyai/go-lua/domain/runtimekind"
)

// set builds one may-set from the families it names, so a case below reads as
// the families it is about rather than as a bit pattern.
func set(kinds ...runtimekind.Kind) runtimekind.Set {
	var built runtimekind.Set
	for _, kind := range kinds {
		built |= runtimekind.Bit(kind)
	}
	return built
}

// TestMayKindConformanceIsTheSubsetVerdict is the whole of the judgment: a
// value conforms to a declaration exactly when every family it may carry is a
// family the declaration admits. The table states each verdict against the
// shape it is about, so a rule that widened one case would move a row here
// rather than pass quietly.
func TestMayKindConformanceIsTheSubsetVerdict(t *testing.T) {
	for _, testCase := range []struct {
		name        string
		declaredMay runtimekind.Set
		observed    runtimekind.Set
		want        Verdict
	}{
		// Nothing was observed, so there is no value to judge. This is the
		// unreached assignment, not a conforming one.
		{"nothing observed", set(runtimekind.String), 0, VerdictAbstain},
		{"nothing observed against nothing declared", 0, 0, VerdictAbstain},
		// The observation proved nothing. An interface-bounded value keeps the
		// whole vocabulary, and a verdict over it would be a finding about the
		// analyzer's own precision rather than about the program.
		{"observation proved nothing", set(runtimekind.String), runtimekind.All, VerdictAbstain},
		{"observation proved nothing against an open declaration", runtimekind.All, runtimekind.All, VerdictAbstain},

		// The value is exactly what the declaration admits.
		{"exact single family", set(runtimekind.String), set(runtimekind.String), VerdictConforms},
		{"exact optional family", set(runtimekind.Nil, runtimekind.String), set(runtimekind.Nil, runtimekind.String), VerdictConforms},
		// The value was narrowed below the declaration. A declaration is an
		// upper bound, so a narrower value is the accepted shape and never a
		// finding.
		{"narrowed out of an optional declaration", set(runtimekind.Nil, runtimekind.String), set(runtimekind.String), VerdictConforms},
		{"narrowed to nil out of an optional declaration", set(runtimekind.Nil, runtimekind.String), set(runtimekind.Nil), VerdictConforms},
		{"narrowed out of an open declaration", runtimekind.All, set(runtimekind.Table), VerdictConforms},
		{"narrowed out of the non-nil partition", runtimekind.NonNil, set(runtimekind.Number), VerdictConforms},

		// The value may carry a family the declaration does not admit.
		{"disjoint family", set(runtimekind.String), set(runtimekind.Number), VerdictViolates},
		{"nil against a non-optional declaration", set(runtimekind.String), set(runtimekind.Nil), VerdictViolates},
		{"one family outside an admitted pair", set(runtimekind.Nil, runtimekind.String), set(runtimekind.String, runtimekind.Number), VerdictViolates},
		{"a reference where a scalar is declared", runtimekind.Scalar, set(runtimekind.String, runtimekind.Table), VerdictViolates},
		// A declaration that admits nothing admits nothing. The verdict is over
		// the sets it is given; a caller with no declaration supplies no
		// judgment to make rather than an empty one.
		{"nothing declared", 0, set(runtimekind.String), VerdictViolates},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			verdict := MayKindConformance(testCase.declaredMay, testCase.observed)
			if verdict != testCase.want {
				t.Fatalf("MayKindConformance(%d, %d) = %d, want %d", testCase.declaredMay, testCase.observed, verdict, testCase.want)
			}
			if !verdict.Available() {
				t.Fatalf("verdict %d is outside the closed catalog", verdict)
			}
		})
	}
}

// TestMayKindConformanceRefusesAMalformedSet states that a set carrying a bit
// outside the closed runtime vocabulary is not judged. Such a set is a caller
// defect rather than a program property, and answering it as an abstention
// would publish that defect as a silence nobody looks at.
func TestMayKindConformanceRefusesAMalformedSet(t *testing.T) {
	foreign := runtimekind.Set(1) << uint(runtimekind.Count)
	for _, testCase := range []struct {
		name                  string
		declaredMay, observed runtimekind.Set
	}{
		{"malformed declaration", runtimekind.All | foreign, set(runtimekind.String)},
		{"malformed observation", set(runtimekind.String), set(runtimekind.String) | foreign},
		{"both malformed", foreign, foreign},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if verdict := MayKindConformance(testCase.declaredMay, testCase.observed); verdict != VerdictInvalid {
				t.Fatalf("MayKindConformance answered %d for a malformed set", verdict)
			}
		})
	}
}

// TestVerdictCatalogIsClosed states the verdict vocabulary itself: three
// answers and the absent one, and nothing beyond them.
func TestVerdictCatalogIsClosed(t *testing.T) {
	for _, verdict := range []Verdict{VerdictAbstain, VerdictConforms, VerdictViolates} {
		if !verdict.Available() {
			t.Fatalf("verdict %d is not available", verdict)
		}
	}
	for _, verdict := range []Verdict{VerdictInvalid, Verdict(4), Verdict(255)} {
		if verdict.Available() {
			t.Fatalf("verdict %d answered as a member of the catalog", verdict)
		}
	}
}

// TestMayKindConformanceAllocatesNothing states the cost of the judgment. It is
// pure set algebra over two scalars, so the publication half can reach it from
// any point without a budget.
func TestMayKindConformanceAllocatesNothing(t *testing.T) {
	declared, observed := runtimekind.NonNil, set(runtimekind.String, runtimekind.Nil)
	allocations := testing.AllocsPerRun(100, func() {
		if MayKindConformance(declared, observed) != VerdictViolates {
			t.Fatal("verdict changed under repetition")
		}
	})
	if allocations != 0 {
		t.Fatalf("MayKindConformance allocated %.0f times per run", allocations)
	}
}
