package typ

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/kind"
)

func TestTypeParam(t *testing.T) {
	tp := NewTypeParam("T", nil)

	if tp.Kind() != kind.TypeParam {
		t.Errorf("Kind: got %v, want TypeParam", tp.Kind())
	}

	if tp.Name != "T" {
		t.Errorf("Name: got %q, want %q", tp.Name, "T")
	}

	if tp.String() != "T" {
		t.Errorf("String: got %q, want %q", tp.String(), "T")
	}
}

func TestTypeParamWithConstraint(t *testing.T) {
	tp := NewTypeParam("T", Number)

	if tp.Constraint != Number {
		t.Error("Constraint should be Number")
	}

	if tp.String() != "T : number" {
		t.Errorf("String: got %q, want %q", tp.String(), "T : number")
	}
}

func TestTypeParamEquality(t *testing.T) {
	tp1 := NewTypeParam("T", nil)
	tp2 := NewTypeParam("T", nil)
	tp3 := NewTypeParam("U", nil)
	tp4 := NewTypeParam("T", Number)

	if !tp1.Equals(tp2) {
		t.Error("T should equal T")
	}

	if tp1.Equals(tp3) {
		t.Error("T should not equal U")
	}

	if tp1.Equals(tp4) {
		t.Error("T should not equal T : number")
	}
}

func TestTypeParamEqualityTreatsTypedNilAsNil(t *testing.T) {
	tp := NewTypeParam("T", nil)
	var nilParam *TypeParam

	if tp.Equals(nilParam) {
		t.Fatal("type parameter should not equal a typed nil parameter")
	}
}

func TestTypeParamEqualityUsesRecursiveGuardForConstraintCycles(t *testing.T) {
	leftRec := NewRecursivePlaceholder("Constraint")
	rightRec := NewRecursivePlaceholder("Constraint")
	leftParam := NewTypeParam("T", leftRec)
	rightParam := NewTypeParam("T", rightRec)
	leftRec.SetBody(newRecord().Field("value", leftParam).Build())
	rightRec.SetBody(newRecord().Field("value", rightParam).Build())

	if !leftParam.Equals(rightParam) {
		t.Fatal("equivalent recursive type-parameter constraints should compare through the shared equality guard")
	}

	changedRec := NewRecursivePlaceholder("Constraint")
	changedParam := NewTypeParam("T", changedRec)
	changedRec.SetBody(newRecord().Field("value", String).Field("next", changedParam).Build())
	if leftParam.Equals(changedParam) {
		t.Fatal("different recursive type-parameter constraints should not compare equal")
	}
}

func TestTypeParamEqualityTerminatesDirectConstraintCycle(t *testing.T) {
	leftParam := NewTypeParam("T", nil)
	rightParam := NewTypeParam("T", nil)
	leftParam.Constraint = leftParam
	rightParam.Constraint = rightParam

	if !leftParam.Equals(rightParam) {
		t.Fatal("direct recursive type-parameter constraints should terminate and compare equal")
	}
}

func TestGenericEqualityUsesRecursiveGuardForTypeParamConstraints(t *testing.T) {
	leftRec := NewRecursivePlaceholder("Constraint")
	rightRec := NewRecursivePlaceholder("Constraint")
	leftParam := NewTypeParam("T", leftRec)
	rightParam := NewTypeParam("T", rightRec)
	leftGeneric := NewGeneric("Box", []*TypeParam{leftParam}, newRecord().Field("value", leftParam).Build())
	rightGeneric := NewGeneric("Box", []*TypeParam{rightParam}, newRecord().Field("value", rightParam).Build())
	leftRec.SetBody(newRecord().Field("value", leftParam).Build())
	rightRec.SetBody(newRecord().Field("value", rightParam).Build())

	if !leftGeneric.Equals(rightGeneric) {
		t.Fatal("generic type parameters with equivalent recursive constraints should share the outer equality guard")
	}
}

func TestInstantiatedBeforeGenericSetBodyEqualsFreshInstantiation(t *testing.T) {
	tp := NewTypeParam("T", nil)
	g := NewGeneric("Box", []*TypeParam{tp}, nil)
	stale := Instantiate(g, Number)

	g.SetBody(newRecord().Field("value", tp).Build())
	fresh := Instantiate(g, Number)

	if !typeEquals(stale, fresh) {
		t.Fatal("instantiation built before Generic.SetBody should compare equal to a fresh instantiation")
	}
	if EqualityHash(stale) != EqualityHash(fresh) {
		t.Fatalf("stale/fresh instantiations should share refreshed equality hash: %d vs %d", EqualityHash(stale), EqualityHash(fresh))
	}
	if got := MaterializeUnion([]Type{stale, fresh}); got != stale {
		t.Fatalf("stale/fresh instantiations should deduplicate in unions, got %T %[1]v", got)
	}
}

func TestEqualityHashSelfInstantiatingGenericTerminates(t *testing.T) {
	tp := NewTypeParam("T", nil)
	g := NewGeneric("Loop", []*TypeParam{tp}, nil)
	g.SetBody(MaterializeUnion([]Type{
		newRecord().Field("value", tp).Build(),
		Instantiate(g, tp),
	}))
	inst := Instantiate(g, String)

	if EqualityHash(inst) == 0 {
		t.Fatal("self-instantiating generic should produce a non-zero equality hash")
	}
	if got := EqualityHash(inst); got != EqualityHash(inst) {
		t.Fatalf("self-instantiating generic equality hash should be stable: %d vs %d", got, EqualityHash(inst))
	}
}

func TestGeneric(t *testing.T) {
	tp := NewTypeParam("T", nil)
	g := NewGeneric("List", []*TypeParam{tp}, NewArray(tp))

	if g.Kind() != kind.Generic {
		t.Errorf("Kind: got %v, want Generic", g.Kind())
	}

	if g.Name != "List" {
		t.Errorf("Name: got %q, want %q", g.Name, "List")
	}

	if len(g.TypeParams) != 1 {
		t.Errorf("TypeParams: got %d, want 1", len(g.TypeParams))
	}

	if g.String() != "List<T>" {
		t.Errorf("String: got %q, want %q", g.String(), "List<T>")
	}
}

func TestGenericMultipleParams(t *testing.T) {
	k := NewTypeParam("K", nil)
	v := NewTypeParam("V", nil)
	g := NewGeneric("Dict", []*TypeParam{k, v}, NewMap(k, v))

	if g.String() != "Dict<K, V>" {
		t.Errorf("String: got %q, want %q", g.String(), "Dict<K, V>")
	}
}

func TestGenericEquality(t *testing.T) {
	tp1 := NewTypeParam("T", nil)
	tp2 := NewTypeParam("T", nil)
	g1 := NewGeneric("List", []*TypeParam{tp1}, NewArray(tp1))
	g2 := NewGeneric("List", []*TypeParam{tp2}, NewArray(tp2))
	g3 := NewGeneric("Vector", []*TypeParam{tp1}, NewArray(tp1))

	if !g1.Equals(g2) {
		t.Error("List<T> should equal List<T>")
	}

	if g1.Equals(g3) {
		t.Error("List<T> should not equal Vector<T>")
	}
}

func TestInstantiated(t *testing.T) {
	tp := NewTypeParam("T", nil)
	g := NewGeneric("List", []*TypeParam{tp}, NewArray(tp))
	i := Instantiate(g, Number)

	if i.Kind() != kind.Instantiated {
		t.Errorf("Kind: got %v, want Instantiated", i.Kind())
	}

	if i.String() != "List<number>" {
		t.Errorf("String: got %q, want %q", i.String(), "List<number>")
	}

	if len(i.TypeArgs) != 1 {
		t.Errorf("TypeArgs: got %d, want 1", len(i.TypeArgs))
	}

	if i.TypeArgs[0] != Number {
		t.Error("TypeArgs[0] should be Number")
	}
}

func TestInstantiatedEquality(t *testing.T) {
	tp := NewTypeParam("T", nil)
	g := NewGeneric("List", []*TypeParam{tp}, NewArray(tp))
	i1 := Instantiate(g, Number)
	i2 := Instantiate(g, Number)
	i3 := Instantiate(g, String)

	if !i1.Equals(i2) {
		t.Error("List<number> should equal List<number>")
	}

	if i1.Equals(i3) {
		t.Error("List<number> should not equal List<string>")
	}
}
