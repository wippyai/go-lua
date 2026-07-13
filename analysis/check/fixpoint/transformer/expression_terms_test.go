package transformer

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	luasourcevalue "github.com/wippyai/go-lua/analysis/lua/sourcevalue"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestScalarBinaryValueDifferentialAgainstLuaKernel(t *testing.T) {
	reg := standard.Registry()
	arena := NewArena(reg)
	shape := Shape{Params: 2}
	left := arena.Root(Root{Kind: RootParam, Index: 0})
	right := arena.Root(Root{Kind: RootParam, Index: 1})
	values := []product.Value{
		typevalue.LiteralString(reg, "value"),
		typevalue.LiteralInt(reg, 7),
		typevalue.LiteralBool(reg, true),
		typevalue.LiteralBool(reg, false),
		typevalue.Nil(reg),
		product.Top(),
	}
	for _, operator := range []string{"==", "~=", "and", "or"} {
		term, ok := arena.ScalarBinaryValue(operator, left, right)
		if !ok {
			t.Fatalf("ScalarBinaryValue(%q) rejected", operator)
		}
		for li, leftValue := range values {
			for ri, rightValue := range values {
				cursor, err := NewBindingCursor(shape, []product.Value{leftValue, rightValue}, nil)
				if err != nil {
					t.Fatal(err)
				}
				got, gotOK := arena.evalValue(term, cursor, SpecializationContext{})
				want, wantOK := luasourcevalue.BinaryOperationValue(reg, nil, operator, leftValue, rightValue)
				if gotOK != wantOK || gotOK && !product.Equal(reg, got, want) {
					t.Fatalf("%s sample %d/%d: got ok=%v value=%#v, canonical ok=%v value=%#v", operator, li, ri, gotOK, got, wantOK, want)
				}
			}
		}
	}
}

func TestScalarBinaryValuePreservesCallerDependence(t *testing.T) {
	reg := standard.Registry()
	arena := NewArena(reg)
	root := arena.Root(Root{Kind: RootParam})
	fixed := arena.Constant(typevalue.LiteralString(reg, "fixed"))
	equal, _ := arena.ScalarBinaryValue("==", root, fixed)

	equalCursor, _ := NewBindingCursor(Shape{Params: 1}, []product.Value{typevalue.LiteralString(reg, "fixed")}, nil)
	differentCursor, _ := NewBindingCursor(Shape{Params: 1}, []product.Value{typevalue.LiteralString(reg, "other")}, nil)
	gotEqual, equalOK := arena.evalValue(equal, equalCursor, SpecializationContext{})
	gotDifferent, differentOK := arena.evalValue(equal, differentCursor, SpecializationContext{})
	if !equalOK || !differentOK || product.Equal(reg, gotEqual, gotDifferent) {
		t.Fatalf("caller substitution collapsed: equal=%#v/%v different=%#v/%v", gotEqual, equalOK, gotDifferent, differentOK)
	}
}

func TestScalarBinaryValueCanonicalStructureAndDeterminism(t *testing.T) {
	reg := standard.Registry()
	arena := NewArena(reg)
	arena.fingerprintMask = 0
	left := arena.Root(Root{Kind: RootParam, Index: 0})
	right := arena.Root(Root{Kind: RootParam, Index: 1})

	eqLR, _ := arena.ScalarBinaryValue("==", left, right)
	eqRL, _ := arena.ScalarBinaryValue("==", right, left)
	neLR, _ := arena.ScalarBinaryValue("~=", left, right)
	andLR, _ := arena.ScalarBinaryValue("and", left, right)
	andRL, _ := arena.ScalarBinaryValue("and", right, left)
	orLR, _ := arena.ScalarBinaryValue("or", left, right)
	if eqLR != eqRL {
		t.Fatalf("commutative equality was not canonical: %d != %d", eqLR, eqRL)
	}
	if eqLR == neLR || eqLR == andLR || andLR == orLR || andLR == andRL {
		t.Fatalf("structurally distinct colliding nodes aliased: eq=%d ne=%d and=%d reverse=%d or=%d", eqLR, neLR, andLR, andRL, orLR)
	}
	if again, _ := arena.ScalarBinaryValue("and", left, right); again != andLR {
		t.Fatalf("identical operation did not intern: %d != %d", again, andLR)
	}

	build := func(reverse bool) string {
		a := NewArena(reg)
		var l, r ValueTerm
		if reverse {
			r = a.Constant(typevalue.LiteralString(reg, "right"))
			l = a.Constant(typevalue.LiteralString(reg, "left"))
		} else {
			l = a.Constant(typevalue.LiteralString(reg, "left"))
			r = a.Constant(typevalue.LiteralString(reg, "right"))
		}
		term, _ := a.ScalarBinaryValue("==", l, r)
		return a.canonicalValue(term)
	}
	if first, second := build(false), build(true); first != second {
		t.Fatalf("canonical spelling depends on construction order: %q != %q", first, second)
	}
	if _, ok := arena.ScalarBinaryValue("+", left, right); ok {
		t.Fatal("unsupported scalar operation admitted")
	}
}

func TestScalarBinaryValueLatticeSoundnessAtTop(t *testing.T) {
	reg := standard.Registry()
	arena := NewArena(reg)
	left := arena.Root(Root{Kind: RootParam, Index: 0})
	right := arena.Root(Root{Kind: RootParam, Index: 1})
	precise := []product.Value{
		typevalue.LiteralString(reg, "x"),
		typevalue.LiteralInt(reg, 1),
		typevalue.LiteralBool(reg, false),
		typevalue.Nil(reg),
	}
	topCursor, _ := NewBindingCursor(Shape{Params: 2}, []product.Value{product.Top(), product.Top()}, nil)
	for _, operator := range []string{"==", "~=", "and", "or"} {
		term, _ := arena.ScalarBinaryValue(operator, left, right)
		top, topOK := arena.evalValue(term, topCursor, SpecializationContext{})
		canonicalTop, canonicalTopOK := luasourcevalue.BinaryOperationValue(reg, nil, operator, product.Top(), product.Top())
		if topOK != canonicalTopOK || topOK && !product.Equal(reg, top, canonicalTop) {
			t.Fatalf("%s Top result diverges from canonical kernel: got=%#v/%v want=%#v/%v", operator, top, topOK, canonicalTop, canonicalTopOK)
		}
		if !topOK {
			// In particular, logical Top currently has no sourcevalue result.
			// Refusing specialization is sound; inventing a Top-shaped constant
			// here would sever the kernel's authority and caller dependence.
			continue
		}
		for _, lv := range precise {
			for _, rv := range precise {
				cursor, _ := NewBindingCursor(Shape{Params: 2}, []product.Value{lv, rv}, nil)
				value, ok := arena.evalValue(term, cursor, SpecializationContext{})
				if ok && !product.LessOrEq(reg, value, top) {
					t.Fatalf("%s concrete result is outside Top result: %#v !<= %#v", operator, value, top)
				}
			}
		}
	}
}

func TestScalarBinaryValueRebasesWithoutLosingOperation(t *testing.T) {
	reg := standard.Registry()
	callee := NewArena(reg)
	left := callee.Root(Root{Kind: RootParam, Index: 0})
	right := callee.Root(Root{Kind: RootParam, Index: 1})
	source, _ := callee.ScalarBinaryValue("or", left, right)

	caller := NewArena(reg)
	boundLeft := caller.Root(Root{Kind: RootParam, Index: 1})
	boundRight := caller.Constant(typevalue.LiteralString(reg, "fallback"))
	bindings, err := NewTermRootBindings(Shape{Params: 2}, Shape{Params: 2}, []ValueTerm{boundLeft, boundRight}, nil)
	if err != nil {
		t.Fatal(err)
	}
	rebased, err := RebaseTermDAGs(caller, callee, bindings, TermRebaseInput{Values: []ValueTerm{source}})
	if err != nil {
		t.Fatal(err)
	}
	cursor, _ := NewBindingCursor(Shape{Params: 2}, []product.Value{product.Top(), typevalue.LiteralBool(reg, false)}, nil)
	got, gotOK := caller.evalValue(rebased.Values[0], cursor, SpecializationContext{})
	want, wantOK := luasourcevalue.BinaryOperationValue(reg, nil, "or", typevalue.LiteralBool(reg, false), typevalue.LiteralString(reg, "fallback"))
	if gotOK != wantOK || gotOK && !product.Equal(reg, got, want) {
		t.Fatalf("rebased operation differs from canonical kernel: got=%#v/%v want=%#v/%v", got, gotOK, want, wantOK)
	}
}
