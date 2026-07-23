package transformer

import (
	"strconv"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestSelectValuePreservesGuardCorrelationAndRebases(t *testing.T) {
	reg := standard.Registry()
	shape := Shape{Params: 1}
	callee := NewArena(reg)
	condition := callee.Root(Root{Kind: RootParam, Index: 0})
	whenTrue := callee.Constant(typevalue.LiteralString(reg, "truthy"))
	whenFalse := callee.Constant(typevalue.LiteralString(reg, "falsy"))
	guard := callee.Truthy(condition)
	selected := callee.SelectValue(guard, whenTrue, whenFalse)

	if selected == 0 || callee.SelectValue(guard, whenTrue, whenFalse) != selected {
		t.Fatalf("select was not structurally interned: %d", selected)
	}
	if got := callee.canonicalValue(selected); got != "sel(t(r1.0),c:"+
		strconv.FormatUint(product.Hash(reg, typevalue.LiteralString(reg, "truthy")), 16)+",c:"+
		strconv.FormatUint(product.Hash(reg, typevalue.LiteralString(reg, "falsy")), 16)+")" {
		t.Fatalf("canonical select = %q", got)
	}
	if got := callee.SelectValue(callee.True(), whenTrue, whenFalse); got != whenTrue {
		t.Fatalf("true select = %d, want %d", got, whenTrue)
	}
	if got := callee.SelectValue(callee.False(), whenTrue, whenFalse); got != whenFalse {
		t.Fatalf("false select = %d, want %d", got, whenFalse)
	}
	if got := callee.SelectValue(guard, whenTrue, whenTrue); got != whenTrue {
		t.Fatalf("equal-arm select = %d, want %d", got, whenTrue)
	}
	collision := NewArena(reg)
	collision.fingerprintMask = 0
	collisionRoot := collision.Root(Root{Kind: RootParam, Index: 0})
	collisionTrue := collision.Constant(typevalue.LiteralString(reg, "truthy"))
	collisionFalse := collision.Constant(typevalue.LiteralString(reg, "falsy"))
	truthySelect := collision.SelectValue(collision.Truthy(collisionRoot), collisionTrue, collisionFalse)
	falsySelect := collision.SelectValue(collision.Falsy(collisionRoot), collisionTrue, collisionFalse)
	if truthySelect == 0 || falsySelect == 0 || truthySelect == falsySelect {
		t.Fatalf("forced fingerprint collision merged distinct selects: %d/%d", truthySelect, falsySelect)
	}

	assertEval := func(name string, input, want product.Value) {
		t.Helper()
		cursor, err := NewBindingCursor(shape, []product.Value{input}, nil)
		if err != nil {
			t.Fatal(err)
		}
		got, exact := callee.evalValue(selected, cursor, SpecializationContext{})
		if !exact || !product.Equal(reg, got, want) {
			t.Fatalf("%s select = %#v/%v, want %#v", name, got, exact, want)
		}
	}
	assertEval("true", typevalue.LiteralBool(reg, true), typevalue.LiteralString(reg, "truthy"))
	assertEval("false", typevalue.LiteralBool(reg, false), typevalue.LiteralString(reg, "falsy"))
	assertEval("unknown", product.Top(), product.Join(reg,
		typevalue.LiteralString(reg, "truthy"),
		typevalue.LiteralString(reg, "falsy"),
	))

	// A decided guard must not evaluate its unreachable arm. CellResult has no
	// resolver here, so either eager evaluation would make the transaction fail.
	unresolved := callee.CellResultValue(CellRef{Function: 1, Slot: 0})
	lazyTrue := callee.SelectValue(guard, whenTrue, unresolved)
	trueCursor, _ := NewBindingCursor(shape, []product.Value{typevalue.LiteralBool(reg, true)}, nil)
	if got, exact := callee.evalValue(lazyTrue, trueCursor, SpecializationContext{}); !exact || !product.Equal(reg, got, typevalue.LiteralString(reg, "truthy")) {
		t.Fatalf("lazy true select = %#v/%v", got, exact)
	}

	caller := NewArena(reg)
	callerShape := Shape{Params: 2}
	callerCondition := caller.Root(Root{Kind: RootParam, Index: 1})
	bindings, err := NewTermRootBindings(shape, callerShape, []ValueTerm{callerCondition}, nil)
	if err != nil {
		t.Fatal(err)
	}
	rebased, err := RebaseTermDAGs(caller, callee, bindings, TermRebaseInput{Values: []ValueTerm{selected}})
	if err != nil || len(rebased.Values) != 1 {
		t.Fatalf("rebase = %#v, %v", rebased, err)
	}
	if got := caller.canonicalValue(rebased.Values[0]); got == callee.canonicalValue(selected) || !caller.ValueDependsOn(rebased.Values[0], callerCondition) {
		t.Fatalf("rebased select did not move to caller root: %q", got)
	}
	callerCursor, _ := NewBindingCursor(callerShape, []product.Value{product.Bottom(reg), typevalue.LiteralBool(reg, false)}, nil)
	got, exact := caller.evalValue(rebased.Values[0], callerCursor, SpecializationContext{})
	if !exact || !product.Equal(reg, got, typevalue.LiteralString(reg, "falsy")) {
		t.Fatalf("rebased false select = %#v/%v", got, exact)
	}

	sealed := NewArena(reg)
	sealedRoot := sealed.Root(Root{Kind: RootParam, Index: 0})
	sealedBindings, err := NewTermRootBindings(shape, shape, []ValueTerm{sealedRoot}, nil)
	if err != nil {
		t.Fatal(err)
	}
	sealed.Seal()
	valuesBefore, guardsBefore := len(sealed.values), len(sealed.guards)
	if _, err := RebaseTermDAGs(sealed, callee, sealedBindings, TermRebaseInput{Values: []ValueTerm{selected}}); err == nil {
		t.Fatal("select rebase into sealed arena unexpectedly succeeded")
	}
	if len(sealed.values) != valuesBefore || len(sealed.guards) != guardsBefore {
		t.Fatalf("failed rebase mutated sealed arena: values %d->%d guards %d->%d", valuesBefore, len(sealed.values), guardsBefore, len(sealed.guards))
	}
}

func TestGuardNotIsCanonicalStructuralComplement(t *testing.T) {
	reg := standard.Registry()
	arena := NewArena(reg)
	shape := Shape{Params: 3}
	values := []ValueTerm{
		arena.Root(Root{Kind: RootParam, Index: 0}),
		arena.Root(Root{Kind: RootParam, Index: 1}),
		arena.Root(Root{Kind: RootParam, Index: 2}),
	}
	guard := arena.And(
		arena.Truthy(values[0]),
		arena.Or(arena.Falsy(values[1]), arena.Truthy(values[2])),
	)
	complement := arena.Not(guard)
	if complement == 0 || arena.Not(complement) != guard {
		t.Fatalf("guard complement is not an involution: g=%d !g=%d !!g=%d", guard, complement, arena.Not(complement))
	}
	if arena.Not(arena.True()) != arena.False() || arena.Not(arena.False()) != arena.True() {
		t.Fatal("constant guard complement is not canonical")
	}
	if arena.Not(arena.Truthy(values[0])) != arena.Falsy(values[0]) || arena.Not(arena.Falsy(values[0])) != arena.Truthy(values[0]) {
		t.Fatal("truth atom complement is not canonical")
	}

	for mask := 0; mask < 8; mask++ {
		bindings := make([]product.Value, 3)
		for index := range bindings {
			bindings[index] = typevalue.LiteralBool(reg, mask&(1<<index) != 0)
		}
		cursor, err := NewBindingCursor(shape, bindings, nil)
		if err != nil {
			t.Fatal(err)
		}
		got, exact := arena.evalGuard(guard, cursor, SpecializationContext{})
		negated, negatedExact := arena.evalGuard(complement, cursor, SpecializationContext{})
		want := mask&1 != 0 && (mask&2 == 0 || mask&4 != 0)
		if !exact || !negatedExact || got != want || negated == want {
			t.Fatalf("mask %03b: g=%v/%v !g=%v/%v want=%v", mask, got, exact, negated, negatedExact, want)
		}
	}
}
