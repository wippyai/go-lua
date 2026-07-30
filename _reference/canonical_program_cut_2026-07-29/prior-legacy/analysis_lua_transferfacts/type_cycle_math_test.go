package transferfacts

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestTransferTypeReachabilityUsesExactRecursiveGraphs(t *testing.T) {
	array := typ.NewRecursivePlaceholder("Array")
	array.SetBody(&typ.Union{Members: []typ.Type{array, typ.NewArray(typ.String)}})
	if !reachesArray(array) {
		t.Fatal("productive recursive array was not reached")
	}

	loop := typ.NewRecursive("Loop", func(self typ.Type) typ.Type { return self })
	if reachesArray(loop) || recordWithCallableField(loop) {
		t.Fatal("cycle-only graph manufactured a container property")
	}

	var deep typ.Type = typ.NewArray(typ.String)
	for range 257 {
		deep = &typ.Alias{Name: "Deep", Target: deep}
	}
	if !reachesArray(deep) {
		t.Fatal("deep acyclic array wrapper was truncated")
	}
}

func TestDynamicIndexMapValueUsesProductiveRecursiveMustProof(t *testing.T) {
	container := typ.NewRecursivePlaceholder("Container")
	container.SetBody(&typ.Union{Members: []typ.Type{container, typ.NewMap(typ.String, typ.Number)}})
	got, ok := dynamicIndexMapValueType(container)
	if !ok || !typ.TypeEquals(got, typ.Number) {
		t.Fatalf("recursive map value = %v/%v, want number", got, ok)
	}

	mismatch := typ.NewRecursivePlaceholder("Mismatch")
	mismatch.SetBody(&typ.Union{Members: []typ.Type{mismatch, typ.NewMap(typ.String, typ.Number), typ.Boolean}})
	if got, ok := dynamicIndexMapValueType(mismatch); ok || got != nil {
		t.Fatalf("productive map mismatch = %v/%v, want failure", got, ok)
	}
}

func TestTypeCanBePresentUsesProductiveRecursiveWitness(t *testing.T) {
	present := typ.NewRecursivePlaceholder("Present")
	present.SetBody(&typ.Union{Members: []typ.Type{present, typ.String}})
	if !typeCanBePresent(present) {
		t.Fatal("productive present arm was lost")
	}

	absent := typ.NewRecursivePlaceholder("Absent")
	absent.SetBody(&typ.Union{Members: []typ.Type{absent, typ.Nil}})
	if typeCanBePresent(absent) {
		t.Fatal("recursive nil-only graph manufactured presence")
	}
}
