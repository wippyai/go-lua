package constraint_test

import (
	"testing"

	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/typ"
)

func TestInferSetBasic(t *testing.T) {
	cs := constraint.NewInferSet()
	tv := typ.NewTypeVar(1)

	cs.AddSubtype(tv, typ.String)

	solution, err := cs.Solve()
	if err != nil {
		t.Fatalf("Solve failed: %v", err)
	}

	if len(solution) != 1 {
		t.Fatalf("expected 1 solution, got %d", len(solution))
	}

	if solution[1] != typ.String {
		t.Errorf("expected string, got %v", solution[1])
	}
}

func TestInferSetArray(t *testing.T) {
	cs := constraint.NewInferSet()
	tv := typ.NewTypeVar(1)

	arrayOfT := typ.NewArray(tv)
	arrayOfString := typ.NewArray(typ.String)

	constraint.MatchContra(arrayOfT, arrayOfString, cs)

	solution, err := cs.Solve()
	if err != nil {
		t.Fatalf("Solve failed: %v", err)
	}

	if len(solution) != 1 {
		t.Fatalf("expected 1 solution, got %d", len(solution))
	}

	if solution[1] != typ.String {
		t.Errorf("expected string, got %v", solution[1])
	}
}

func TestInferSetMap(t *testing.T) {
	cs := constraint.NewInferSet()
	tvK := typ.NewTypeVar(1)
	tvV := typ.NewTypeVar(2)

	mapKV := typ.NewMap(tvK, tvV)
	mapStringNumber := typ.NewMap(typ.String, typ.Number)

	constraint.MatchContra(mapKV, mapStringNumber, cs)

	solution, err := cs.Solve()
	if err != nil {
		t.Fatalf("Solve failed: %v", err)
	}

	if len(solution) != 2 {
		t.Fatalf("expected 2 solutions, got %d", len(solution))
	}

	if solution[1] != typ.String {
		t.Errorf("expected key to be string, got %v", solution[1])
	}

	if solution[2] != typ.Number {
		t.Errorf("expected value to be number, got %v", solution[2])
	}
}

func TestInferSetMultipleConstraints(t *testing.T) {
	cs := constraint.NewInferSet()
	tv := typ.NewTypeVar(1)

	cs.AddSubtype(typ.Integer, tv)
	cs.AddSubtype(tv, typ.Number)

	solution, err := cs.Solve()
	if err != nil {
		t.Fatalf("Solve failed: %v", err)
	}

	if len(solution) != 1 {
		t.Fatalf("expected 1 solution, got %d", len(solution))
	}

	if solution[1] != typ.Integer {
		t.Errorf("expected integer (lower bound preferred), got %v", solution[1])
	}
}

func TestInferSetRecord(t *testing.T) {
	cs := constraint.NewInferSet()
	tv := typ.NewTypeVar(1)

	patternRec := typ.NewRecord().Field("value", tv).Build()
	concreteRec := typ.NewRecord().Field("value", typ.String).Build()

	constraint.MatchContra(patternRec, concreteRec, cs)

	solution, err := cs.Solve()
	if err != nil {
		t.Fatalf("Solve failed: %v", err)
	}

	if solution[1] != typ.String {
		t.Errorf("expected string, got %v", solution[1])
	}
}

func TestInferSubstitutionApply(t *testing.T) {
	tv := typ.NewTypeVar(1)
	sub := constraint.InferSubstitution{
		1: typ.String,
	}

	result := sub.Apply(typ.NewArray(tv))
	arr, ok := result.(*typ.Array)

	if !ok {
		t.Fatalf("expected array, got %T", result)
	}

	if arr.Element != typ.String {
		t.Errorf("expected array of string, got %v", arr.Element)
	}
}

// any is consistent with every bound; the call check that follows
// instantiation applies the assignability mode to the any-typed argument.
func TestInferSetAnyLowerBoundIsConsistent(t *testing.T) {
	cs := constraint.NewInferSet()
	tv := typ.NewTypeVar(1)

	constraint.MatchContra(typ.NewArray(tv), typ.NewArray(typ.Any), cs)
	cs.AddSubtype(tv, typ.String)

	solution, err := cs.Solve()
	if err != nil {
		t.Fatalf("Solve failed: %v", err)
	}
	if !typ.IsAny(solution[1]) {
		t.Errorf("expected any, got %v", solution[1])
	}
}

func TestInferSetConcreteBoundsConflict(t *testing.T) {
	cs := constraint.NewInferSet()
	tv := typ.NewTypeVar(1)

	constraint.MatchContra(typ.NewArray(tv), typ.NewArray(typ.Number), cs)
	cs.AddSubtype(tv, typ.String)

	if _, err := cs.Solve(); err == nil {
		t.Fatal("expected number and string bounds to conflict")
	}
}

// An array and a tuple are maps from integer keys to their elements, so they
// bind an integer-keyed map pattern's value.
func TestInferSetMapPatternFromArrayAndTuple(t *testing.T) {
	for _, tt := range []struct {
		name     string
		concrete typ.Type
		want     typ.Type
	}{
		{"array", typ.NewArray(typ.String), typ.String},
		{"tuple", typ.NewTuple(typ.String, typ.String), typ.String},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cs := constraint.NewInferSet()
			tv := typ.NewTypeVar(1)
			constraint.MatchContra(typ.NewMap(typ.Integer, tv), tt.concrete, cs)

			solution, err := cs.Solve()
			if err != nil {
				t.Fatalf("Solve failed: %v", err)
			}
			if !typ.TypeEquals(solution[1], tt.want) {
				t.Errorf("expected %v, got %v", tt.want, solution[1])
			}
		})
	}
}

func TestInferSetMapPatternKeyFromArray(t *testing.T) {
	cs := constraint.NewInferSet()
	key, value := typ.NewTypeVar(1), typ.NewTypeVar(2)
	constraint.MatchContra(typ.NewMap(key, value), typ.NewArray(typ.Boolean), cs)

	solution, err := cs.Solve()
	if err != nil {
		t.Fatalf("Solve failed: %v", err)
	}
	if !typ.TypeEquals(solution[1], typ.Integer) {
		t.Errorf("key: expected integer, got %v", solution[1])
	}
	if !typ.TypeEquals(solution[2], typ.Boolean) {
		t.Errorf("value: expected boolean, got %v", solution[2])
	}
}

// A table argument binds a map pattern's value to the join of its values: the
// map's value type is one type, and each value is one of its members.
func TestInferSetMapPatternJoinsHeterogeneousValues(t *testing.T) {
	withOrder := typ.NewRecord().Field("id", typ.String).Field("order", typ.Integer).Build()
	plain := typ.NewRecord().Field("id", typ.String).Build()
	for _, tt := range []struct {
		name     string
		pattern  typ.Type
		concrete typ.Type
		want     typ.Type
	}{
		{"tuple of number and string", typ.NewMap(typ.Integer, typ.NewTypeVar(1)),
			typ.NewTuple(typ.Number, typ.String, typ.Number), typ.NewUnion(typ.Number, typ.String)},
		{"tuple of records", typ.NewMap(typ.Integer, typ.NewTypeVar(1)),
			typ.NewTuple(withOrder, plain), typ.NewUnion(withOrder, plain)},
		{"record", typ.NewMap(typ.String, typ.NewTypeVar(1)),
			typ.NewRecord().Field("a", typ.Number).Field("b", typ.String).Build(), typ.NewUnion(typ.Number, typ.String)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cs := constraint.NewInferSet()
			constraint.MatchContra(tt.pattern, tt.concrete, cs)

			solution, err := cs.Solve()
			if err != nil {
				t.Fatalf("Solve failed: %v", err)
			}
			if !typ.TypeEquals(solution[1], tt.want) {
				t.Errorf("expected %v, got %v", tt.want, solution[1])
			}
		})
	}
}

func TestInferSetMapPatternKeyFromRecord(t *testing.T) {
	cs := constraint.NewInferSet()
	key, value := typ.NewTypeVar(1), typ.NewTypeVar(2)
	record := typ.NewRecord().Field("a", typ.Number).Field("b", typ.Number).Build()
	constraint.MatchContra(typ.NewMap(key, value), record, cs)

	solution, err := cs.Solve()
	if err != nil {
		t.Fatalf("Solve failed: %v", err)
	}
	if !typ.TypeEquals(solution[1], typ.String) {
		t.Errorf("key: expected string, got %v", solution[1])
	}
	if !typ.TypeEquals(solution[2], typ.Number) {
		t.Errorf("value: expected number, got %v", solution[2])
	}
}
