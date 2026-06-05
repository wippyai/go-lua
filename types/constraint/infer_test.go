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

func TestInferSetArrayElementPreservesAliasBound(t *testing.T) {
	cs := constraint.NewInferSet()
	tv := typ.NewTypeVar(1)
	event := typ.NewAlias("protocol.Event", typ.NewRecord().
		Field("kind", typ.String).
		Build())

	constraint.MatchContra(typ.NewArray(tv), typ.NewArray(event), cs)

	solution, err := cs.Solve()
	if err != nil {
		t.Fatalf("Solve failed: %v", err)
	}
	alias, ok := solution[1].(*typ.Alias)
	if !ok || alias.Name != "protocol.Event" {
		t.Fatalf("expected protocol.Event alias, got %v", solution[1])
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

func TestMatchContraFunctionReturnSelectsGenericUnionMember(t *testing.T) {
	cs := constraint.NewInferSet()
	tVar := typ.NewTypeVar(1)
	uVar := typ.NewTypeVar(2)

	failure := typ.NewRecord().
		Field("ok", typ.False).
		Field("error", uVar).
		Build()
	patternSuccess := typ.NewRecord().
		Field("ok", typ.True).
		Field("value", uVar).
		Build()
	pattern := typ.Func().
		Param("value", tVar).
		Returns(typ.NewUnion(patternSuccess, failure)).
		Build()

	envelope := typ.NewRecord().Field("id", typ.String).Build()
	view := typ.NewRecord().Field("label", typ.String).Build()
	concreteSuccess := typ.NewRecord().
		Field("ok", typ.True).
		Field("value", view).
		Build()
	concrete := typ.Func().
		Param("env", envelope).
		Returns(concreteSuccess).
		Build()

	constraint.MatchContra(pattern, concrete, cs)
	solution, err := cs.Solve()
	if err != nil {
		t.Fatalf("Solve failed: %v", err)
	}
	if got := solution[2]; !typ.TypeEquals(got, view) {
		t.Fatalf("U solution = %v, want %v", got, view)
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
