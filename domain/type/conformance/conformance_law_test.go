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

// TestVerdictCatalogIsClosed states the verdict vocabulary itself: six
// answers and the absent one, and nothing beyond them.
func TestVerdictCatalogIsClosed(t *testing.T) {
	for _, verdict := range []Verdict{
		VerdictAbstain, VerdictConforms, VerdictViolates,
		VerdictMayBeNil, VerdictMemberAbsent, VerdictUnproven,
	} {
		if !verdict.Available() {
			t.Fatalf("verdict %d is not available", verdict)
		}
	}
	for _, verdict := range []Verdict{VerdictInvalid, verdictLimit, Verdict(255)} {
		if verdict.Available() {
			t.Fatalf("verdict %d answered as a member of the catalog", verdict)
		}
	}
}

// TestMayBeNilConformanceIsTheNilPresenceVerdict states the whole of the
// narrower judgment: it fires exactly when nil is the value's only excess
// over an admitted, nil-excluding declaration, and abstains for every other
// combination, including the ones where a general containment violation
// exists alongside nil.
func TestMayBeNilConformanceIsTheNilPresenceVerdict(t *testing.T) {
	for _, testCase := range []struct {
		name        string
		declaredMay runtimekind.Set
		observed    runtimekind.Set
		want        Verdict
	}{
		{"nothing observed", set(runtimekind.String), 0, VerdictAbstain},
		{"observation proved nothing", set(runtimekind.String), runtimekind.All, VerdictAbstain},

		// The declaration already admits nil, so an observed nil is not a
		// finding this judgment names.
		{"nil admitted by an optional declaration", set(runtimekind.Nil, runtimekind.String), set(runtimekind.Nil, runtimekind.String), VerdictAbstain},
		{"nil admitted, value narrowed to nil", set(runtimekind.Nil, runtimekind.String), set(runtimekind.Nil), VerdictAbstain},

		// The value carries no nil, so there is no nil-presence question.
		{"non-optional declaration, no nil observed", set(runtimekind.String), set(runtimekind.String), VerdictAbstain},
		{"disjoint family with no nil", set(runtimekind.String), set(runtimekind.Number), VerdictAbstain},

		// Nil is excluded by the declaration, observed, and the remainder
		// after removing nil conforms: the finding is exactly nil presence.
		{"single family plus nil", set(runtimekind.String), set(runtimekind.Nil, runtimekind.String), VerdictMayBeNil},
		{"pair declaration plus nil", set(runtimekind.String, runtimekind.Number), set(runtimekind.Nil, runtimekind.String), VerdictMayBeNil},
		{"narrowed to exactly nil against a non-optional declaration", set(runtimekind.String), set(runtimekind.Nil), VerdictMayBeNil},
		{"non-nil partition declaration narrowed to nil", runtimekind.NonNil, set(runtimekind.Nil), VerdictMayBeNil},

		// Nil is excluded and observed, but another excess family remains
		// after removing nil: the finding is a general violation, not this
		// judgment's to name.
		{"nil plus a disjoint family", set(runtimekind.String), set(runtimekind.Nil, runtimekind.Number), VerdictAbstain},
		{"nil plus an unadmitted reference", runtimekind.Scalar, set(runtimekind.Nil, runtimekind.Table), VerdictAbstain},

		// A declaration admitting nothing still admits nothing: nil alone
		// against it is a nil-presence finding like any other.
		{"nothing declared, nil observed", 0, set(runtimekind.Nil), VerdictMayBeNil},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			verdict := MayBeNilConformance(testCase.declaredMay, testCase.observed)
			if verdict != testCase.want {
				t.Fatalf("MayBeNilConformance(%d, %d) = %d, want %d", testCase.declaredMay, testCase.observed, verdict, testCase.want)
			}
			if !verdict.Available() {
				t.Fatalf("verdict %d is outside the closed catalog", verdict)
			}
		})
	}
}

// TestMayBeNilConformanceRefusesAMalformedSet mirrors the malformed-set law
// MayKindConformance states: a set with a bit outside the closed vocabulary is
// a caller defect, never an answer about a program.
func TestMayBeNilConformanceRefusesAMalformedSet(t *testing.T) {
	foreign := runtimekind.Set(1) << uint(runtimekind.Count)
	for _, testCase := range []struct {
		name                  string
		declaredMay, observed runtimekind.Set
	}{
		{"malformed declaration", runtimekind.All | foreign, set(runtimekind.Nil)},
		{"malformed observation", set(runtimekind.String), set(runtimekind.Nil) | foreign},
		{"both malformed", foreign, foreign},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if verdict := MayBeNilConformance(testCase.declaredMay, testCase.observed); verdict != VerdictInvalid {
				t.Fatalf("MayBeNilConformance answered %d for a malformed set", verdict)
			}
		})
	}
}

// TestMayBeNilConformanceAgreesWithMayKindConformance is the cross-function
// law: the two judgments are asked over the same inputs, so a MayBeNil answer
// must never occur where the containment verdict is not a violation, and the
// narrower judgment must never answer outside its own two-member range.
// The loop covers every valid set pair, which the eight-family vocabulary
// keeps exhaustive.
func TestMayBeNilConformanceAgreesWithMayKindConformance(t *testing.T) {
	for declaredMay := runtimekind.Set(0); declaredMay <= runtimekind.All; declaredMay++ {
		for observed := runtimekind.Set(0); observed <= runtimekind.All; observed++ {
			kind := MayKindConformance(declaredMay, observed)
			nilVerdict := MayBeNilConformance(declaredMay, observed)
			if nilVerdict != VerdictMayBeNil && nilVerdict != VerdictAbstain {
				t.Fatalf("MayBeNilConformance(%d, %d) = %d outside {MayBeNil, Abstain}", declaredMay, observed, nilVerdict)
			}
			if nilVerdict == VerdictMayBeNil && kind != VerdictViolates {
				t.Fatalf("MayBeNilConformance(%d, %d) = MayBeNil while MayKindConformance = %d, not Violates", declaredMay, observed, kind)
			}
		}
	}
}

// TestMayKindConformanceIsPinnedAcrossTheSealedUniverse restates the
// containment law directly for every valid set pair the eight-family
// vocabulary admits, so a change to MayKindConformance's behavior anywhere in
// that universe fails here rather than surfacing downstream.
func TestMayKindConformanceIsPinnedAcrossTheSealedUniverse(t *testing.T) {
	for declaredMay := runtimekind.Set(0); declaredMay <= runtimekind.All; declaredMay++ {
		for observed := runtimekind.Set(0); observed <= runtimekind.All; observed++ {
			want := VerdictConforms
			switch {
			case observed == 0 || observed == runtimekind.All:
				want = VerdictAbstain
			case observed&^declaredMay != 0:
				want = VerdictViolates
			}
			if got := MayKindConformance(declaredMay, observed); got != want {
				t.Fatalf("MayKindConformance(%d, %d) = %d, want %d", declaredMay, observed, got, want)
			}
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

// TestMayBeNilConformanceAllocatesNothing states the same cost law for the
// nil-presence judgment: pure set algebra over two scalars, callable from any
// point without a budget.
func TestMayBeNilConformanceAllocatesNothing(t *testing.T) {
	declared, observed := set(runtimekind.String), set(runtimekind.Nil, runtimekind.String)
	allocations := testing.AllocsPerRun(100, func() {
		if MayBeNilConformance(declared, observed) != VerdictMayBeNil {
			t.Fatal("verdict changed under repetition")
		}
	})
	if allocations != 0 {
		t.Fatalf("MayBeNilConformance allocated %.0f times per run", allocations)
	}
}

// TestVerdictCatalogIsTotalAndDenseFromItsOwnConstants states the vocabulary
// half of this judgment. A declaration table keyed by these ordinals renders one
// message per answer, so the ordinals must be the answers' own constants, dense
// from the first, and each must carry exactly one spelling.
func TestVerdictCatalogIsTotalAndDenseFromItsOwnConstants(t *testing.T) {
	catalog := Catalog()
	if len(catalog) == 0 {
		t.Fatal("the verdict catalog is empty")
	}
	spellings := make(map[string]Verdict, len(catalog))
	keys := make(map[string]Verdict, len(catalog))
	for index, verdict := range catalog {
		if !verdict.Available() {
			t.Fatalf("catalog position %d holds an unavailable verdict %d", index, verdict)
		}
		if verdict.Ordinal() != uint16(index)+1 {
			t.Fatalf("verdict %d has ordinal %d at catalog position %d; the catalog is dense from one", verdict, verdict.Ordinal(), index)
		}
		if verdict.Spelling() == "" {
			t.Fatalf("verdict %d renders no spelling", verdict)
		}
		if prior, duplicate := spellings[verdict.Spelling()]; duplicate {
			t.Fatalf("verdicts %d and %d share the spelling %q", prior, verdict, verdict.Spelling())
		}
		spellings[verdict.Spelling()] = verdict
		if VerdictKey(verdict) == "" {
			t.Fatalf("verdict %d is declared under no key", verdict)
		}
		if prior, duplicate := keys[VerdictKey(verdict)]; duplicate {
			t.Fatalf("verdicts %d and %d share the key %q", prior, verdict, VerdictKey(verdict))
		}
		keys[VerdictKey(verdict)] = verdict
	}
	// Every answer this judgment can give is in the catalog. An answer outside
	// it would be a finding a declaration table keyed by the catalog renders
	// nothing for.
	for _, verdict := range []Verdict{VerdictAbstain, VerdictConforms, VerdictViolates, VerdictMayBeNil, VerdictMemberAbsent, VerdictUnproven} {
		if _, member := spellings[verdict.Spelling()]; !member {
			t.Fatalf("verdict %d is not in the catalog", verdict)
		}
	}
	if VerdictInvalid.Ordinal() != 0 || VerdictInvalid.Spelling() != "" || VerdictKey(VerdictInvalid) != "" {
		t.Fatal("the absent answer is declared as a vocabulary member")
	}
}
