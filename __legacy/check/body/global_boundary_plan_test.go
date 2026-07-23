package body

import (
	"reflect"
	"slices"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
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

// TestPrepareBoundChunkGlobalCarrierContractsAlignWithCanonicalOrder proves the
// canonical-order invariant behind materializeBoundaryGlobalTypeValues:
// BoundaryGlobalContracts() must stay positionally aligned with
// BoundaryGlobals() (canonical symbol-sorted order), independent of the
// first-use order the source actually reads those globals in and independent
// of which fresh typevalue.Cache instance materialized them.
//
// This is the regression coverage for "prepare lexical forest: global carrier
// contracts conflict": materializeBoundaryGlobalTypeValues used to build the
// contract vector against first-use order while WithBoundaryGlobals silently
// re-sorted the symbol vector into canonical order, desynchronizing the two
// whenever first-use order disagreed with ascending symbol order.
func TestPrepareBoundChunkGlobalCarrierContractsAlignWithCanonicalOrder(t *testing.T) {
	reg := standard.Registry()
	globals := []string{"first", "second", "unused"}
	globalTypes := map[string]typ.Type{"first": typ.Number, "second": typ.String}

	reversed := parseChunk(t, `
		local value = second
		return first, value, second
	`)
	canonical := parseChunk(t, `
		local value = first
		return second, value, first
	`)

	reversedBindings := bind.BindChunk(reversed, bind.Options{Globals: globals})
	canonicalBindings := bind.BindChunk(canonical, bind.Options{Globals: globals})

	reversedReads := reversedBindings.ChunkGlobalReads()
	canonicalReads := canonicalBindings.ChunkGlobalReads()
	if len(reversedReads) != 2 || reversedReads[0] < reversedReads[1] {
		t.Fatalf("reversed fixture did not exercise reversed canonical order: %v", reversedReads)
	}
	if len(canonicalReads) != 2 || canonicalReads[0] > canonicalReads[1] {
		t.Fatalf("canonical fixture did not exercise ascending read order: %v", canonicalReads)
	}

	reversedPrepared, err := PrepareBoundChunk(reversed, reversedBindings, Config{Registry: reg, Globals: globals, GlobalTypes: globalTypes})
	if err != nil {
		t.Fatal(err)
	}
	canonicalPrepared, err := PrepareBoundChunk(canonical, canonicalBindings, Config{Registry: reg, Globals: globals, GlobalTypes: globalTypes})
	if err != nil {
		t.Fatal(err)
	}

	directFirst := typevalue.NewCache().FromTypeWithWitness(reg, typ.Number)
	directSecond := typevalue.NewCache().FromTypeWithWitness(reg, typ.String)

	for _, tc := range []struct {
		name     string
		prepared *Static
		bindings *bind.Result
	}{
		{"reversed-read-order", reversedPrepared, reversedBindings},
		{"canonical-read-order", canonicalPrepared, canonicalBindings},
	} {
		plan := tc.prepared.OperationPlan()
		firstSym, ok := tc.bindings.GlobalSymbol("first")
		if !ok {
			t.Fatalf("%s: no symbol for global %q", tc.name, "first")
		}
		secondSym, ok := tc.bindings.GlobalSymbol("second")
		if !ok {
			t.Fatalf("%s: no symbol for global %q", tc.name, "second")
		}
		firstIndex, ok := plan.BoundaryGlobalIndex(firstSym)
		if !ok {
			t.Fatalf("%s: global %q missing from boundary", tc.name, "first")
		}
		secondIndex, ok := plan.BoundaryGlobalIndex(secondSym)
		if !ok {
			t.Fatalf("%s: global %q missing from boundary", tc.name, "second")
		}
		contracts := plan.BoundaryGlobalContracts()
		if !product.Equal(reg, contracts[firstIndex], directFirst) {
			t.Fatalf("%s: first carrier contract at index %d is misaligned, want number contract", tc.name, firstIndex)
		}
		if !product.Equal(reg, contracts[secondIndex], directSecond) {
			t.Fatalf("%s: second carrier contract at index %d is misaligned, want string contract", tc.name, secondIndex)
		}
	}
}
