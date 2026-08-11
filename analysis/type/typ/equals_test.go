package typ

import "testing"

func TestTypeEqualsAcyclicSmallProductDoesNotAllocate(t *testing.T) {
	left := Func().
		Param("input", newRecord().Field("value", Number).Build()).
		Returns(MaterializeUnion([]Type{Number, String})).
		Build()
	right := Func().
		Param("input", newRecord().Field("value", Number).Build()).
		Returns(MaterializeUnion([]Type{Number, String})).
		Build()

	if allocs := testing.AllocsPerRun(100, func() {
		if !typeEquals(left, right) {
			t.Fatal("equivalent acyclic products should compare equal")
		}
	}); allocs != 0 {
		t.Fatalf("acyclic equality allocated %g times per comparison, want 0", allocs)
	}
}

func TestTypeEqualsIdentity(t *testing.T) {
	if !typeEquals(Number, Number) {
		t.Error("number should equal number")
	}

	if !typeEquals(String, String) {
		t.Error("string should equal string")
	}
}

func TestTypeEqualsNil(t *testing.T) {
	if typeEquals(nil, Number) {
		t.Error("nil should not equal number")
	}

	if typeEquals(Number, nil) {
		t.Error("number should not equal nil")
	}

	if !typeEquals(nil, nil) {
		t.Error("nil should equal nil")
	}
}

func TestTypeEqualsTypedNil(t *testing.T) {
	var nilFunction *Function
	var nilType Type = nilFunction

	if !typeEquals(nilType, nil) {
		t.Error("typed nil should equal nil")
	}
	if typeEquals(nilType, Func().Build()) {
		t.Error("typed nil should not equal a concrete function")
	}
}

func TestTypeEqualsDifferentKinds(t *testing.T) {
	if typeEquals(Number, String) {
		t.Error("number should not equal string")
	}
}

func TestTypeEqualsAlias(t *testing.T) {
	alias := NewAlias("MyNum", Number)

	if !typeEquals(alias, Number) {
		t.Error("alias to number should equal number")
	}

	if !typeEquals(Number, alias) {
		t.Error("number should equal alias to number")
	}
}

func TestTypeEqualsRef(t *testing.T) {
	r1 := NewRef("mod", "T")
	r2 := NewRef("mod", "T")
	r3 := NewRef("mod", "U")

	if !typeEquals(r1, r2) {
		t.Error("mod.T should equal mod.T")
	}

	if typeEquals(r1, r3) {
		t.Error("mod.T should not equal mod.U")
	}
}

func TestTypeEqualsOptional(t *testing.T) {
	o1 := MaterializeOptional(Number)
	o2 := MaterializeOptional(Number)
	o3 := MaterializeOptional(String)

	if !typeEquals(o1, o2) {
		t.Error("number? should equal number?")
	}

	if typeEquals(o1, o3) {
		t.Error("number? should not equal string?")
	}
}

func TestTypeEqualsUnion(t *testing.T) {
	u1 := MaterializeUnion([]Type{Number, String})
	u2 := MaterializeUnion([]Type{Number, String})
	u3 := MaterializeUnion([]Type{Number, Boolean})

	if !typeEquals(u1, u2) {
		t.Error("number | string should equal number | string")
	}

	if typeEquals(u1, u3) {
		t.Error("number | string should not equal number | boolean")
	}
}

func TestTypeEqualsArray(t *testing.T) {
	a1 := NewArray(Number)
	a2 := NewArray(Number)
	a3 := NewArray(String)

	if !typeEquals(a1, a2) {
		t.Error("number[] should equal number[]")
	}

	if typeEquals(a1, a3) {
		t.Error("number[] should not equal string[]")
	}
}

func TestTypeEqualsMap(t *testing.T) {
	m1 := NewMap(String, Number)
	m2 := NewMap(String, Number)
	m3 := NewMap(String, String)

	if !typeEquals(m1, m2) {
		t.Error("maps should be equal")
	}

	if typeEquals(m1, m3) {
		t.Error("maps with different value types should not be equal")
	}
}

func TestTypeEqualsTuple(t *testing.T) {
	t1 := NewTuple(Number, String)
	t2 := NewTuple(Number, String)
	t3 := NewTuple(String, Number)

	if !typeEquals(t1, t2) {
		t.Error("tuples should be equal")
	}

	if typeEquals(t1, t3) {
		t.Error("tuples with different element order should not be equal")
	}
}

func TestTypeEqualsRecord(t *testing.T) {
	r1 := newRecord().Field("x", Number).Build()
	r2 := newRecord().Field("x", Number).Build()
	r3 := newRecord().Field("x", String).Build()

	if !typeEquals(r1, r2) {
		t.Error("records should be equal")
	}

	if typeEquals(r1, r3) {
		t.Error("records with different field types should not be equal")
	}
}

func TestTypeEqualsRecordMetatableParticipatesInIdentity(t *testing.T) {
	metaA := newRecord().Field("a", Func().Param("self", Self).Returns(String).Build()).Build()
	metaB := newRecord().Field("b", Func().Param("self", Self).Returns(Number).Build()).Build()
	a := newRecord().Metatable(metaA).SetOpen(true).Build()
	b := newRecord().Metatable(metaB).SetOpen(true).Build()

	if typeEquals(a, b) {
		t.Fatal("records with different metatables must not be structurally equal")
	}
	if a.Hash() == b.Hash() {
		t.Fatal("test setup expected distinct hashes for distinct metatables")
	}
}

func TestTypeEqualsFunction(t *testing.T) {
	f1 := Func().Param("x", Number).Returns(String).Build()
	f2 := Func().Param("y", Number).Returns(String).Build()
	f3 := Func().Param("x", String).Returns(String).Build()

	if !typeEquals(f1, f2) {
		t.Error("functions with same signature should be equal")
	}

	if typeEquals(f1, f3) {
		t.Error("functions with different param types should not be equal")
	}
}

func TestTypeEqualsDeepProductsExactly(t *testing.T) {
	var left Type = Number
	var right Type = Number
	for i := 0; i < 257; i++ {
		left = NewArray(left)
		right = NewArray(right)
	}
	if !typeEquals(left, right) {
		t.Fatal("equal deep products compared unequal")
	}
}

func TestTypeEqualsIterativeDeepAndCyclicLaws(t *testing.T) {
	const depth = 10_000

	var equalLeft Type = Number
	var equalRight Type = Number
	var different Type = String
	for i := 0; i < depth; i++ {
		equalLeft = NewArray(equalLeft)
		equalRight = NewArray(equalRight)
		different = NewArray(different)
	}
	if !TypeEquals(equalLeft, equalRight) {
		t.Fatalf("equivalent %d-level products compared unequal", depth)
	}
	if TypeEquals(equalLeft, different) || TypeEquals(different, equalLeft) {
		t.Fatal("deep products with distinct leaves must compare unequal in both directions")
	}

	left := &Optional{}
	left.Inner = left
	right := &Optional{}
	rightTail := &Optional{Inner: right}
	right.Inner = rightTail
	if !TypeEquals(left, right) || !TypeEquals(right, left) {
		t.Fatal("bisimilar cyclic products must compare equal in both directions")
	}

	differentCycle := &Optional{Inner: String}
	if TypeEquals(left, differentCycle) || TypeEquals(differentCycle, left) {
		t.Fatal("cyclic and non-bisimilar products must compare unequal in both directions")
	}
}

func TestTypeEqualsDeepAliasDifference(t *testing.T) {
	var left Type = Number
	var right Type = String
	for i := 0; i < 257; i++ {
		left = &Alias{Name: "Left", Target: left}
		right = &Alias{Name: "Right", Target: right}
	}

	if TypeEquals(left, right) {
		t.Fatal("distinct deep alias chains compared equal")
	}
}

func TestTypeEqualsNilNil(t *testing.T) {
	// Both nil should return true
	if !typeEquals(nil, nil) {
		t.Error("nil should equal nil")
	}
}

func TestTypeEqualsIntersection(t *testing.T) {
	i1 := MaterializeIntersection([]Type{Number, String})
	i2 := MaterializeIntersection([]Type{Number, String})
	i3 := MaterializeIntersection([]Type{Number, Boolean})

	if !typeEquals(i1, i2) {
		t.Error("same intersections should be equal")
	}
	if typeEquals(i1, i3) {
		t.Error("different intersections should not be equal")
	}
}

func TestTypeEqualsIntersectionLength(t *testing.T) {
	i1 := MaterializeIntersection([]Type{Number, String})
	i2 := MaterializeIntersection([]Type{Number, String, Boolean})

	if typeEquals(i1, i2) {
		t.Error("intersections of different lengths should not be equal")
	}
}

func TestTypeEqualsAliasChain(t *testing.T) {
	// A = B, B = C, C = number
	c := NewAlias("C", Number)
	b := NewAlias("B", c)
	a := NewAlias("A", b)

	if !typeEquals(a, Number) {
		t.Error("alias chain should equal base type")
	}
	if !typeEquals(Number, a) {
		t.Error("base type should equal alias chain")
	}
	if !typeEquals(a, c) {
		t.Error("outer alias should equal inner alias")
	}
}

func TestTypeEqualsLocalRef(t *testing.T) {
	// Refs only equal other refs with same module and name
	// Refs do NOT equal aliases - they are different types
	ref := NewRef("", "MyType")
	alias := NewAlias("MyType", Number)

	if typeEquals(ref, alias) {
		t.Error("ref should not equal alias even with same name")
	}

	// But alias should still equal its target
	if !typeEquals(alias, Number) {
		t.Error("alias should equal its target")
	}
}

func TestTypeEqualsLocalRefToLocalRef(t *testing.T) {
	ref1 := NewRef("", "Type")
	ref2 := NewRef("", "Type")
	ref3 := NewRef("", "Other")

	if !typeEquals(ref1, ref2) {
		t.Error("same local refs should be equal")
	}
	if typeEquals(ref1, ref3) {
		t.Error("different local refs should not be equal")
	}
}

func TestTypeEqualsLiteral(t *testing.T) {
	l1 := LiteralString("hello")
	l2 := LiteralString("hello")
	l3 := LiteralString("world")

	if !typeEquals(l1, l2) {
		t.Error("same string literals should be equal")
	}
	if typeEquals(l1, l3) {
		t.Error("different string literals should not be equal")
	}
}

func TestTypeEqualsLiteralInt(t *testing.T) {
	l1 := LiteralInt(42)
	l2 := LiteralInt(42)
	l3 := LiteralInt(100)

	if !typeEquals(l1, l2) {
		t.Error("same int literals should be equal")
	}
	if typeEquals(l1, l3) {
		t.Error("different int literals should not be equal")
	}
}

func TestTypeEqualsLiteralBool(t *testing.T) {
	if !typeEquals(True, True) {
		t.Error("true should equal true")
	}
	if !typeEquals(False, False) {
		t.Error("false should equal false")
	}
	if typeEquals(True, False) {
		t.Error("true should not equal false")
	}
}

func TestTypeEqualsUnionOrder(t *testing.T) {
	// Unions should be order-independent after normalization
	u1 := MaterializeUnion([]Type{Number, String})
	u2 := MaterializeUnion([]Type{String, Number})

	if !typeEquals(u1, u2) {
		t.Error("unions with same members in different order should be equal")
	}
}

func TestTypeEqualsEmptyRecord(t *testing.T) {
	r1 := newRecord().Build()
	r2 := newRecord().Build()

	if !typeEquals(r1, r2) {
		t.Error("empty records should be equal")
	}
}

func TestTypeEqualsRecordFieldOrder(t *testing.T) {
	// Records with same fields in different order
	r1 := newRecord().Field("a", Number).Field("b", String).Build()
	r2 := newRecord().Field("b", String).Field("a", Number).Build()

	if !typeEquals(r1, r2) {
		t.Error("records with same fields should be equal regardless of definition order")
	}
}

func TestTypeEqualsOptionalField(t *testing.T) {
	r1 := newRecord().OptField("x", Number).Build()
	r2 := newRecord().OptField("x", Number).Build()
	r3 := newRecord().Field("x", Number).Build()

	if !typeEquals(r1, r2) {
		t.Error("records with same optional fields should be equal")
	}
	if typeEquals(r1, r3) {
		t.Error("optional field should not equal required field")
	}
}

func TestTypeEqualsSharedDAGNodes(t *testing.T) {
	shared := newRecord().Field("value", String).Build()
	left := newRecord().
		Field("a", shared).
		Field("b", shared).
		Build()
	right := newRecord().
		Field("a", newRecord().Field("value", String).Build()).
		Field("b", newRecord().Field("value", String).Build()).
		Build()

	if !typeEquals(left, right) {
		t.Error("structurally equal DAG-shaped records should be equal even when sharing differs")
	}
}

func TestTypeEqualsFunctionVariadic(t *testing.T) {
	f1 := Func().Variadic(Number).Returns(Nil).Build()
	f2 := Func().Variadic(Number).Returns(Nil).Build()
	f3 := Func().Param("args", Number).Returns(Nil).Build()

	if !typeEquals(f1, f2) {
		t.Error("same variadic functions should be equal")
	}
	if typeEquals(f1, f3) {
		t.Error("variadic should not equal non-variadic")
	}
}

func TestTypeEqualsFunctionMultiReturn(t *testing.T) {
	f1 := Func().Returns(Number, String).Build()
	f2 := Func().Returns(Number, String).Build()
	f3 := Func().Returns(Number).Build()

	if !typeEquals(f1, f2) {
		t.Error("functions with same multi-return should be equal")
	}
	if typeEquals(f1, f3) {
		t.Error("functions with different return counts should not be equal")
	}
}

func TestTypeEqualsFunctionTypeParams(t *testing.T) {
	f1 := Func().TypeParam("T", nil).Param("x", NewTypeParam("T", nil)).Returns(NewTypeParam("T", nil)).Build()
	f2 := Func().TypeParam("T", nil).Param("x", NewTypeParam("T", nil)).Returns(NewTypeParam("T", nil)).Build()
	f3 := Func().TypeParam("U", nil).Param("x", NewTypeParam("U", nil)).Returns(NewTypeParam("U", nil)).Build()

	if !typeEquals(f1, f2) {
		t.Error("functions with same type params should be equal")
	}
	if typeEquals(f1, f3) {
		t.Error("functions with different type param names should not be equal")
	}
	if typeEquals(f1, Func().Param("x", Any).Returns(Any).Build()) {
		t.Error("generic and non-generic functions should not be equal")
	}
}

func TestTypeEqualsGeneric(t *testing.T) {
	params := []*TypeParam{NewTypeParam("K", nil), NewTypeParam("V", nil)}
	g1 := NewGeneric("T", params, NewMap(NewRef("", "K"), NewRef("", "V")))
	g2 := NewGeneric("T", params, NewMap(NewRef("", "K"), NewRef("", "V")))
	g3 := NewGeneric("U", params, NewMap(NewRef("", "K"), NewRef("", "V")))

	if !typeEquals(g1, g2) {
		t.Error("same generics should be equal")
	}
	if typeEquals(g1, g3) {
		t.Error("generics with different names should not be equal")
	}
}

func TestTypeEqualsInterface(t *testing.T) {
	i1 := NewInterface("I", []Method{{Name: "foo", Type: Func().Build()}})
	i2 := NewInterface("I", []Method{{Name: "foo", Type: Func().Build()}})
	i3 := NewInterface("J", []Method{{Name: "foo", Type: Func().Build()}})

	if !typeEquals(i1, i2) {
		t.Error("same interfaces should be equal")
	}
	if typeEquals(i1, i3) {
		t.Error("interfaces with different names should not be equal")
	}
}

func TestTypeEqualsInterfaceMethodOrder(t *testing.T) {
	m1 := Method{Name: "foo", Type: Func().Build()}
	m2 := Method{Name: "bar", Type: Func().Build()}

	i1 := NewInterface("I", []Method{m1, m2})
	i2 := NewInterface("I", []Method{m2, m1})

	// Method order may or may not matter depending on normalization
	_ = i1
	_ = i2
}

func TestTypeEqualsCycleDetection(t *testing.T) {
	// Create recursive type that could cause infinite loop
	rec := NewRecursive("Node", func(self Type) Type {
		return newRecord().OptField("next", self).Build()
	})

	// Should not hang
	if !typeEquals(rec, rec) {
		t.Error("recursive type should equal itself")
	}
}

func TestTypeEqualsClosedSelfRecursiveRecordsTerminate(t *testing.T) {
	left := NewRecursive("Node", func(self Type) Type {
		return newRecord().OptField("next", self).Build()
	})
	right := NewRecursive("Node", func(self Type) Type {
		return newRecord().OptField("next", self).Build()
	})

	// The body reaches its enclosing Recursive node.  This is the minimal
	// closed cycle exported recursive types use; checking its openness and
	// comparing independent but equivalent graphs must both terminate.
	if knownContainsOpenRecursive(left) || knownContainsOpenRecursive(right) {
		t.Fatal("closed self-recursive records must not be open-recursive")
	}
	if !TypeEquals(left, right) {
		t.Fatal("equivalent closed self-recursive records should compare equal")
	}
}

func TestTypeEqualsHashPrefilterRejectsOpenRecursiveCacheWithoutLiveScan(t *testing.T) {
	// A positive open-recursive cache is deliberately conservative: it may be
	// stale after a placeholder body closes, but equality must not re-walk its
	// graph from the hash prefilter to discover that fact.
	stale := &Record{typeProperties: typeProperties{containsOpenRecursive: true}}
	if typeEqualsCanUseHashPrefilter(stale, Number) {
		t.Fatal("open-recursive cache must bypass the equality hash prefilter")
	}
}

func TestTypeEqualsOpenRecursiveWrapperUsesCoinductiveWalk(t *testing.T) {
	left := NewRecursivePlaceholder("Node")
	right := NewRecursivePlaceholder("Node")
	leftWrapper := Func().Returns(left).Build()
	rightWrapper := Func().Returns(right).Build()
	left.SetBody(newRecord().OptField("next", left).Build())
	right.SetBody(newRecord().OptField("next", right).Build())

	if !knownContainsRecursive(leftWrapper) || !knownContainsRecursive(rightWrapper) {
		t.Fatal("test requires recursive-containing wrappers")
	}
	if knownContainsOpenRecursive(leftWrapper) || knownContainsOpenRecursive(rightWrapper) {
		t.Fatal("closed recursive wrappers should not retain stale open-recursive state")
	}
	if !typeEquals(leftWrapper, rightWrapper) {
		t.Fatal("equivalent open-recursive wrappers should compare through coinductive equality")
	}
}

func TestSameNodeOrAcyclicEqualRejectsRecursiveContainingWrappers(t *testing.T) {
	left := NewRecursivePlaceholder("Node")
	right := NewRecursivePlaceholder("Node")
	leftWrapper := Func().Returns(left).Build()
	rightWrapper := Func().Returns(right).Build()
	left.SetBody(newRecord().OptField("next", left).Build())
	right.SetBody(newRecord().OptField("next", right).Build())

	if SameNodeOrAcyclicEqual(leftWrapper, rightWrapper) {
		t.Fatal("recursive-containing wrappers should not be accepted by the acyclic fast path")
	}
	if !typeEquals(leftWrapper, rightWrapper) {
		t.Fatal("full recursive equality should still compare equivalent wrappers")
	}
}

func TestTypeEqualsMutualRecursion(t *testing.T) {
	recA := NewRecursivePlaceholder("A")
	recB := NewRecursivePlaceholder("B")
	recA.SetBody(newRecord().OptField("b", recB).Build())
	recB.SetBody(newRecord().OptField("a", recA).Build())

	// Should not hang
	if !typeEquals(recA, recA) {
		t.Error("mutual recursive A should equal itself")
	}
	if !typeEquals(recB, recB) {
		t.Error("mutual recursive B should equal itself")
	}
}

func TestTypeEqualsNestedOptional(t *testing.T) {
	o1 := MaterializeOptional(MaterializeOptional(Number))
	o2 := MaterializeOptional(MaterializeOptional(Number))

	if !typeEquals(o1, o2) {
		t.Error("nested optionals should be equal")
	}
}

func TestTypeEqualsNestedUnion(t *testing.T) {
	inner := MaterializeUnion([]Type{Number, String})
	u1 := MaterializeUnion([]Type{inner, Boolean})
	u2 := MaterializeUnion([]Type{inner, Boolean})

	if !typeEquals(u1, u2) {
		t.Error("unions with same nested union should be equal")
	}
}

func TestTypeEqualsNeverAnyCases(t *testing.T) {
	if !typeEquals(Never, Never) {
		t.Error("Never should equal Never")
	}
	if !typeEquals(Any, Any) {
		t.Error("Any should equal Any")
	}
	if !typeEquals(Unknown, Unknown) {
		t.Error("Unknown should equal Unknown")
	}
	if typeEquals(Never, Any) {
		t.Error("Never should not equal Any")
	}
}

func TestTypeEqualsDeepAliasFunctionSignature(t *testing.T) {
	aliasChain := func(depth int) Type {
		t := Number
		for i := 0; i < depth; i++ {
			t = NewAlias("N", t)
		}
		return t
	}

	a := Func().Param("v", aliasChain(32)).Returns(Number).Build()
	b := Func().Param("v", aliasChain(32)).Returns(Number).Build()

	if !typeEquals(a, b) {
		t.Fatalf("expected deep alias signatures to be equal:\nleft:  %s\nright: %s", a.String(), b.String())
	}
}
