package transformer

import (
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestLuaTypeNameTermPreservesCallerDependence(t *testing.T) {
	reg := standard.Registry()
	arena := NewArena(reg)
	root := Root{Kind: RootParam, Index: 0}
	term := arena.LuaTypeNameValue(arena.Root(root))
	shape := Shape{Params: 1}

	for _, tag := range []runtimekind.Tag{
		runtimekind.Nil, runtimekind.Boolean, runtimekind.Number, runtimekind.String,
		runtimekind.Table, runtimekind.Function, runtimekind.Thread, runtimekind.Userdata,
	} {
		arg := product.Set(reg, product.Top(), runtimekind.Key, runtimekind.Singleton(tag))
		cursor, err := NewBindingCursor(shape, []product.Value{arg}, nil)
		if err != nil {
			t.Fatal(err)
		}
		got, ok := arena.evalValue(term, cursor, SpecializationContext{})
		literal, exact := typevalue.StringLiteralOf(reg, got)
		if !ok || !exact || literal != tag.String() {
			t.Fatalf("tag %s = %q/%v/%v", tag, literal, ok, exact)
		}
	}
	topCursor, _ := NewBindingCursor(shape, []product.Value{product.Top()}, nil)
	topValue, ok := arena.evalValue(term, topCursor, SpecializationContext{})
	if topType, exact := typevalue.TypeOf(reg, topValue); !ok || !exact || !typ.TypeEquals(topType, typ.String) {
		t.Fatalf("Top type = %v/%v/%v, want string", topType, ok, exact)
	}
}

func TestLuaTypeNameTermRebaseCanonicalDeterminismAndCollision(t *testing.T) {
	reg := standard.Registry()
	callee := NewArena(reg)
	root := Root{Kind: RootParam, Index: 0}
	term := callee.LuaTypeNameValue(callee.Root(root))
	if again := callee.LuaTypeNameValue(callee.Root(root)); again != term {
		t.Fatalf("term interning = %d/%d", term, again)
	}
	if got := callee.canonicalValue(term); got != "lua:type(r1.0)" {
		t.Fatalf("canonical value = %q", got)
	}

	caller := NewArena(reg)
	callerRoot := Root{Kind: RootParam, Index: 1}
	bindings, err := NewTermRootBindings(Shape{Params: 1}, Shape{Params: 2}, []ValueTerm{caller.Root(callerRoot)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	rebased, err := RebaseTermDAGs(caller, callee, bindings, TermRebaseInput{Values: []ValueTerm{term}})
	if err != nil || len(rebased.Values) != 1 || !caller.ValueDependsOn(rebased.Values[0], caller.Root(callerRoot)) {
		t.Fatalf("rebase = %#v, %v", rebased, err)
	}

	collision := NewArena(reg)
	collision.fingerprintMask = 0
	left := collision.LuaTypeNameValue(collision.Root(Root{Kind: RootParam, Index: 0}))
	right := collision.LuaTypeNameValue(collision.Root(Root{Kind: RootParam, Index: 1}))
	if left == right || collision.canonicalValue(left) == collision.canonicalValue(right) {
		t.Fatalf("forced collision merged distinct terms: %d/%d", left, right)
	}
}

func TestLuaTypeNameTermFeedsExactIsStringOperand(t *testing.T) {
	reg := standard.Registry()
	arena := NewArena(reg)
	shape := Shape{Params: 1}
	typeName := arena.LuaTypeNameValue(arena.Root(Root{Kind: RootParam, Index: 0}))
	isString, ok := arena.ScalarBinaryValue("==", typeName, arena.Constant(typevalue.LiteralString(reg, "string")))
	if !ok {
		t.Fatal("is-string term rejected")
	}
	for _, tc := range []struct {
		tag  runtimekind.Tag
		want bool
	}{{runtimekind.String, true}, {runtimekind.Number, false}} {
		arg := product.Set(reg, product.Top(), runtimekind.Key, runtimekind.Singleton(tc.tag))
		cursor, _ := NewBindingCursor(shape, []product.Value{arg}, nil)
		got, exact := arena.evalValue(isString, cursor, SpecializationContext{})
		gotType, typeExact := typevalue.TypeOf(reg, got)
		if !exact || !typeExact || !typ.TypeEquals(gotType, typ.LiteralBool(tc.want)) {
			t.Fatalf("is_str(%s) = %v/%v/%v, want %v", tc.tag, gotType, exact, typeExact, tc.want)
		}
	}
}
