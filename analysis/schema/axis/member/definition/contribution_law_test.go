package definition

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
)

const specimenPackage = "example/axis"

func specimenType(name string) GoType { return GoType{PackagePath: specimenPackage, Name: name} }

func specimenMethod(name, receiver string) GoSymbol {
	return GoSymbol{PackagePath: specimenPackage, Name: name, Receiver: specimenType(receiver)}
}

func specimenAxis() schema.EntryReference {
	return schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "specimen"}
}

// specimenBase is one axis owner's whole authored base: carriers, one
// candidate relation with its directory, and one destination projection. It
// declares no reducer, which is the shape Compose requires.
func specimenBase() Definition {
	provider := member.RelationRef{Axis: specimenAxis(), Member: "specimen/candidates"}
	return Definition{
		Name: "Specimen",
		Axis: "specimen",
		Binding: Binding{Key: KeyNormalization{
			Carrier:    "KeyCarrier",
			Dense:      GoType{Name: "uint32"},
			Normalizer: specimenMethod("KeyIndex", "Schema"),
		}},
		Signature: Signature{Key: "KeyCarrier", Fact: "FactCarrier"},
		Carriers: []Carrier{
			{Name: "KeyCarrier", Key: "carrier/specimen/key", Type: specimenType("Key")},
			{Name: "FactCarrier", Key: "carrier/specimen/fact", Type: specimenType("Fact")},
			{Name: "SeedCarrier", Key: "carrier/specimen/seed", Type: specimenType("Seed")},
		},
		Relations: []Relation{{
			Name:              "Candidates",
			Key:               "specimen/candidates",
			Subject:           "SeedCarrier",
			CandidateProvider: provider,
			CandidateResolver: specimenMethod("SeedForOccurrence", "Schema"),
			CandidateOrdinal:  specimenMethod("SeedOrdinal", "Schema"),
			CandidateAt:       specimenMethod("SeedAt", "Schema"),
		}},
		Projections: []Projection{{
			Name:              "SeedCoordinate",
			Key:               "specimen/coordinate",
			Relation:          "Candidates",
			CandidateProvider: provider,
			Role:              member.Destination,
			Result:            "KeyCarrier",
			Accessor:          specimenMethod("Result", "Seed"),
		}},
	}
}

func specimenContribution(rule schema.Key, name string, key schema.Key) Contribution {
	return Contribution{
		Axis: "specimen",
		Rule: rule,
		Reducers: []Reducer{{
			Name:           name,
			Key:            key,
			Candidate:      "SeedCarrier",
			Outputs:        []ReducerOutput{{Axis: specimenAxis(), Carrier: "FactCarrier"}},
			Implementation: GoSymbol{PackagePath: specimenPackage, Name: name + "Fold"},
		}},
	}
}

func specimenSource(contributions ...Contribution) Source {
	return Source{Package: "specimen", Name: "specimen", Base: specimenBase(), Contributions: contributions}
}

// TestComposeFoldsRuleContributionsIntoTheAxisDefinition is the baseline: the
// axis vocabulary a generator renders is the base plus the reducers its rules
// declared, in roster order, each carrying the rule that declared it.
func TestComposeFoldsRuleContributionsIntoTheAxisDefinition(t *testing.T) {
	composed, ok := specimenSource(
		specimenContribution("specimen-first", "First", "specimen/reducer/first"),
		specimenContribution("specimen-second", "Second", "specimen/reducer/second"),
	).Compose()
	if !ok {
		t.Fatal("specimen source did not compose")
	}
	if !composed.Complete() {
		t.Fatal("composed definition is incomplete")
	}
	if len(composed.Reducers) != 2 {
		t.Fatalf("composed reducers = %d, want 2", len(composed.Reducers))
	}
	want := []struct {
		rule schema.Key
		key  schema.Key
	}{
		{"specimen-first", "specimen/reducer/first"},
		{"specimen-second", "specimen/reducer/second"},
	}
	for index, row := range composed.Reducers {
		if row.Key != want[index].key {
			t.Fatalf("reducer[%d] key = %q, want %q", index, row.Key, want[index].key)
		}
		if row.Rule != want[index].rule {
			t.Fatalf("reducer[%d] rule = %q, want %q", index, row.Rule, want[index].rule)
		}
	}
}

// TestComposeRefusesABaseThatDeclaresItsOwnReducer is the law that removes the
// central per-axis list. A reducer authored beside the carriers is a fold no
// rule is on record as folding with, so it cannot be merged - it is refused.
func TestComposeRefusesABaseThatDeclaresItsOwnReducer(t *testing.T) {
	source := specimenSource(specimenContribution("specimen-first", "First", "specimen/reducer/first"))
	source.Base.Reducers = []Reducer{{
		Name:           "Central",
		Key:            "specimen/reducer/central",
		Candidate:      "SeedCarrier",
		Outputs:        []ReducerOutput{{Axis: specimenAxis(), Carrier: "FactCarrier"}},
		Implementation: GoSymbol{PackagePath: specimenPackage, Name: "CentralFold"},
	}}
	if _, ok := source.Compose(); ok {
		t.Fatal("a base that hand-lists a reducer composed")
	}
}

// TestAxisWithNoContributionsComposesNoReducerRows states the third half of the
// composition law: an axis whose rules declare no fold has an empty reducer
// vocabulary rather than a missing one, and the generator has nothing to emit
// for it.
func TestAxisWithNoContributionsComposesNoReducerRows(t *testing.T) {
	composed, ok := specimenSource().Compose()
	if !ok {
		t.Fatal("an axis with no reducer contributions did not compose")
	}
	if len(composed.Reducers) != 0 {
		t.Fatalf("composed reducers = %d, want 0", len(composed.Reducers))
	}
	catalog, catalogOK := composed.Catalog()
	if !catalogOK || catalog.ReducerCount() != 0 {
		t.Fatalf("catalog reducers = %d/%t, want 0/true", catalog.ReducerCount(), catalogOK)
	}
}

// TestComposeRefusesContradictoryContributions states what one vocabulary
// means: two rules cannot claim one reducer key or one reducer name, one rule
// cannot contribute twice, and a contribution cannot place a reducer in an
// axis other than the one it names.
func TestComposeRefusesContradictoryContributions(t *testing.T) {
	cases := map[string][]Contribution{
		"duplicate-reducer-key": {
			specimenContribution("specimen-first", "First", "specimen/reducer/shared"),
			specimenContribution("specimen-second", "Second", "specimen/reducer/shared"),
		},
		"duplicate-reducer-name": {
			specimenContribution("specimen-first", "Shared", "specimen/reducer/first"),
			specimenContribution("specimen-second", "Shared", "specimen/reducer/second"),
		},
		"duplicate-rule": {
			specimenContribution("specimen-first", "First", "specimen/reducer/first"),
			specimenContribution("specimen-first", "Second", "specimen/reducer/second"),
		},
		"no-reducer": {
			{Axis: "specimen", Rule: "specimen-empty"},
		},
		"unnamed-rule": {
			specimenContribution("", "First", "specimen/reducer/first"),
		},
	}
	for name, contributions := range cases {
		t.Run(name, func(t *testing.T) {
			if _, ok := specimenSource(contributions...).Compose(); ok {
				t.Fatal("contradictory contributions composed")
			}
		})
	}
	foreign := specimenContribution("specimen-first", "First", "specimen/reducer/first")
	foreign.Axis = "other"
	if _, ok := specimenSource(foreign).Compose(); ok {
		t.Fatal("a contribution placed a reducer in a foreign axis")
	}
}

// TestComposeRefusesAContributionCarrierThatContradictsTheBase states the
// carrier fence: a contribution may repeat a carrier the base declares, but a
// repeat that disagrees is two declarations of one name.
func TestComposeRefusesAContributionCarrierThatContradictsTheBase(t *testing.T) {
	agreeing := specimenContribution("specimen-first", "First", "specimen/reducer/first")
	agreeing.Carriers = []Carrier{{Name: "FactCarrier", Key: "carrier/specimen/fact", Type: specimenType("Fact")}}
	if _, ok := specimenSource(agreeing).Compose(); !ok {
		t.Fatal("a contribution repeating a base carrier verbatim was refused")
	}
	contradicting := specimenContribution("specimen-first", "First", "specimen/reducer/first")
	contradicting.Carriers = []Carrier{{Name: "FactCarrier", Key: "carrier/specimen/other", Type: specimenType("Fact")}}
	if _, ok := specimenSource(contradicting).Compose(); ok {
		t.Fatal("a contribution redefined a base carrier")
	}
}

// TestRosterRefusesTwoOwnersOfOneVocabulary states the registry law: a name or
// an axis appearing twice is two owners of one vocabulary.
func TestRosterRefusesTwoOwnersOfOneVocabulary(t *testing.T) {
	if _, ok := NewRoster(specimenSource(), specimenSource()); ok {
		t.Fatal("two sources of one axis were admitted")
	}
	second := specimenSource()
	second.Name = "specimen-two"
	second.Base.Axis = "specimen-two"
	roster, ok := NewRoster(specimenSource(), second)
	if !ok || roster.Count() != 2 {
		t.Fatalf("distinct sources rejected: ok=%t count=%d", ok, roster.Count())
	}
	if _, resolved := roster.Source("absent"); resolved {
		t.Fatal("an unregistered source resolved")
	}
}
