package body

import (
	"reflect"
	"slices"
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestPrepareFunctionPublishesExactOrderedDirectGlobalBoundary(t *testing.T) {
	fn := parseFunction(t, `function f(param)
		local first_read = second
		local repeated = second
		return param, first, first_read, repeated
	end`)
	bindings := bind.BindFunction(fn, bind.Options{Globals: []string{"first", "second", "unused"}})
	want := bindings.DirectGlobalReads(fn)
	if len(want) != 2 {
		t.Fatalf("binder direct globals = %v, want exact referenced set", want)
	}
	slices.Sort(want)
	prepared, err := PrepareBoundFunction(fn, bindings, Config{
		Registry: standard.Registry(), Globals: []string{"first", "second", "unused"},
	})
	if err != nil {
		t.Fatal(err)
	}
	plan := prepared.OperationPlan()
	if !plan.BoundaryGlobalsValid() {
		t.Fatal("prepared plan rejected exact binder global census")
	}
	if got := plan.BoundaryGlobals(); !reflect.DeepEqual(got, want) {
		t.Fatalf("boundary globals = %v, want canonical order %v", got, want)
	}
	got := plan.BoundaryGlobals()
	got[0] = 0
	if reread := plan.BoundaryGlobals(); !reflect.DeepEqual(reread, want) {
		t.Fatalf("prepared boundary exposed mutable storage: %v", reread)
	}
}

func TestPrepareBoundChunkPublishesExactCanonicalGlobalBoundary(t *testing.T) {
	stmts := parseChunk(t, `
		local value = second
		return first, value, second
	`)
	globals := []string{"first", "second", "unused"}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: globals})
	firstUse := bindings.ChunkGlobalReads()
	if len(firstUse) != 2 || firstUse[0] < firstUse[1] {
		t.Fatalf("fixture did not exercise reversed canonical order: %v", firstUse)
	}
	want := slices.Clone(firstUse)
	slices.Sort(want)
	prepared, err := PrepareBoundChunk(stmts, bindings, Config{Registry: standard.Registry(), Globals: globals})
	if err != nil {
		t.Fatal(err)
	}
	plan := prepared.OperationPlan()
	if !plan.BoundaryGlobalsValid() || !reflect.DeepEqual(plan.BoundaryGlobals(), want) {
		t.Fatalf("chunk boundary globals = %v valid=%v, want canonical %v", plan.BoundaryGlobals(), plan.BoundaryGlobalsValid(), want)
	}
}
