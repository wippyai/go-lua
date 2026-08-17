package projection

import (
	"testing"

	"github.com/wippyai/go-lua/domain/type/typ"
)

// TestStepKindCatalogIsTheDenseEnumerationOfEveryValidKind states the density
// law a consumer's exhaustive iteration rests on: the catalog is every kind the
// admission predicate accepts, each once, in ordinal order from the first. A
// step added to the type and not to the catalog is a verdict here rather than a
// step a consumer silently never visits.
func TestStepKindCatalogIsTheDenseEnumerationOfEveryValidKind(t *testing.T) {
	var admitted []StepKind
	for candidate := 0; candidate <= int(^uint8(0)); candidate++ {
		if kind := StepKind(candidate); kind.Valid() {
			admitted = append(admitted, kind)
		}
	}
	catalog := StepKinds()
	if len(admitted) != StepKindCount || len(catalog) != StepKindCount {
		t.Fatalf("catalog holds %d kinds and the type admits %d, declared count is %d", len(catalog), len(admitted), StepKindCount)
	}
	for position, kind := range catalog {
		if kind != admitted[position] {
			t.Fatalf("catalog position %d is kind %d, but the type's ordinal %d is kind %d", position, kind, position, admitted[position])
		}
		if int(kind) != position+1 {
			t.Fatalf("catalog position %d holds kind %d, so the ordinals are not dense from one", position, kind)
		}
	}
	if StepKind(0).Valid() {
		t.Fatal("the zero kind was admitted as a declared member")
	}
}

// TestEveryDeclaredStepKindIsInhabitedByAStep states the catalog is the
// vocabulary and not a list beside it: each declared kind names a step the
// package can build, and that step answers as its own kind.
func TestEveryDeclaredStepKindIsInhabitedByAStep(t *testing.T) {
	samples := map[StepKind]Step{
		StepField:              Field("inner"),
		StepCallableReturn:     CallableReturn(),
		StepGenericArg:         GenericArg(1),
		StepInstantiateGeneric: InstantiateGeneric(typ.String),
	}
	if len(samples) != StepKindCount {
		t.Fatalf("the vocabulary declares %d kinds but %d are inhabited by a step", StepKindCount, len(samples))
	}
	for _, kind := range StepKinds() {
		step, sampled := samples[kind]
		if !sampled {
			t.Fatalf("declared kind %d names no step of the vocabulary", kind)
		}
		if step.Kind != kind {
			t.Fatalf("step %s answers kind %d, not the kind %d it inhabits", step, step.Kind, kind)
		}
		if !step.Kind.Valid() {
			t.Fatalf("declared kind %d is not admitted by the vocabulary's own predicate", kind)
		}
	}
}
