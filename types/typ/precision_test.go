package typ

import "testing"

func TestMorePreciseRecordFieldReplacesUnknown(t *testing.T) {
	baseline := NewRecord().
		Field("value", Unknown).
		Field("ok", Boolean).
		Build()
	candidate := NewRecord().
		Field("value", String).
		Field("ok", Boolean).
		Build()

	if !MorePrecise(candidate, baseline) {
		t.Fatalf("expected %v to be more precise than %v", candidate, baseline)
	}
	if MorePrecise(baseline, candidate) {
		t.Fatalf("baseline should not be more precise than candidate")
	}
}

func TestMorePreciseUnionMembersRefineCommonRecord(t *testing.T) {
	baseline := NewRecord().
		Field("channel", Any).
		Field("value", Unknown).
		Field("ok", Boolean).
		OptField("default", Boolean).
		Build()
	eventChannel := NewInterface("Channel<Event>", nil)
	timeChannel := NewInterface("Channel<Time>", nil)
	eventType := NewRecord().Field("kind", String).Build()
	timeType := NewRecord().Field("sec", Number).Build()
	candidate := NewUnion(
		NewRecord().
			Field("channel", eventChannel).
			Field("value", eventType).
			Field("ok", Boolean).
			Build(),
		NewRecord().
			Field("channel", timeChannel).
			Field("value", timeType).
			Field("ok", Boolean).
			Build(),
	)

	if !MorePrecise(candidate, baseline) {
		t.Fatalf("effect-projected union should refine raw select result: %v vs %v", candidate, baseline)
	}
	if MorePrecise(baseline, candidate) {
		t.Fatalf("raw select result should not refine effect-projected union")
	}
}

func TestMorePreciseCoalescesCompatibleRecordUnionBaseline(t *testing.T) {
	members := make([]Type, 0, 128)
	for i := 0; i < cap(members); i++ {
		members = append(members, NewRecord().
			Field("name", LiteralString("suite")).
			Field("index", LiteralInt(int64(i))).
			Build())
	}
	baseline := NewUnion(members...)
	candidate := NewRecord().
		Field("name", LiteralString("suite")).
		Field("index", Integer).
		Field("full_path", String).
		Build()

	if !MorePrecise(candidate, baseline) {
		t.Fatalf("candidate should refine coalesced compatible record family: %v vs %v", candidate, baseline)
	}
}

func TestComparePrecisionUnionToUnionMatchesMembersIndependently(t *testing.T) {
	baseline := NewUnion(
		NewRecord().Field("kind", LiteralString("a")).Field("value", Any).Build(),
		NewRecord().Field("kind", LiteralString("b")).Field("value", Any).Build(),
	)
	candidate := NewUnion(
		NewRecord().Field("kind", LiteralString("a")).Field("value", String).Build(),
		NewRecord().Field("kind", LiteralString("b")).Field("value", Integer).Build(),
	)

	if strict, comparable := ComparePrecision(candidate, baseline); !strict || !comparable {
		t.Fatalf("union member precision = (%v, %v), want strict comparable", strict, comparable)
	}
}

func TestComparePrecisionStaticStringMemberRefinesUnknown(t *testing.T) {
	baseline := NewRecord().
		StaticStringIndex("raw-key", Unknown).
		Build()
	candidate := NewRecord().
		StaticStringIndex("raw-key", String).
		Build()

	if strict, comparable := ComparePrecision(candidate, baseline); !strict || !comparable {
		t.Fatalf("static member precision = (%v, %v), want strict comparable", strict, comparable)
	}
	if SameProductFamily(candidate, baseline) {
		t.Fatalf("static member precision variants must not share product family")
	}
}

func TestComparePrecisionDistinguishesDotFieldAndStaticStringMember(t *testing.T) {
	field := NewRecord().
		Field("raw-key", String).
		Build()
	index := NewRecord().
		StaticStringIndex("raw-key", String).
		Build()

	if strict, comparable := ComparePrecision(index, field); strict || comparable {
		t.Fatalf("static string index vs dot field precision = (%v, %v), want unrelated", strict, comparable)
	}
	if SameProductFamily(index, field) {
		t.Fatalf("static string index and dot field must not share product family")
	}
}

func TestMorePreciseRecordMayRemoveOptionalBaselineField(t *testing.T) {
	baseline := NewRecord().
		Field("value", Unknown).
		OptField("default", Boolean).
		Build()
	candidate := NewRecord().
		Field("value", String).
		Build()

	if !MorePrecise(candidate, baseline) {
		t.Fatalf("record without optional field should refine optional baseline field: %v vs %v", candidate, baseline)
	}
}

func TestMorePreciseRequiredFieldRefinesOptionalBaselineField(t *testing.T) {
	baseline := NewRecord().
		OptField("value", String).
		Build()
	candidate := NewRecord().
		Field("value", String).
		Build()

	if !MorePrecise(candidate, baseline) {
		t.Fatalf("required field should refine optional baseline field: %v vs %v", candidate, baseline)
	}
	if MorePrecise(baseline, candidate) {
		t.Fatal("optional field should not refine required field")
	}
}

func TestMorePreciseLiteralRefinesPrimitive(t *testing.T) {
	if !MorePrecise(LiteralString("ready"), String) {
		t.Fatal("string literal should be more precise than string")
	}
	if !MorePrecise(LiteralInt(3), Number) {
		t.Fatal("integer literal should be more precise than number")
	}
	if !MorePrecise(Integer, Number) {
		t.Fatal("integer should be more precise than number")
	}
	if MorePrecise(String, LiteralString("ready")) {
		t.Fatal("string should not be more precise than a string literal")
	}
}

func TestMorePreciseConcreteTypeArgRefinesSymbolicTypeParam(t *testing.T) {
	tp := NewTypeParam("T", nil)
	result := NewGeneric("Result", []*TypeParam{tp}, NewRecord().Field("value", tp).Build())
	concrete := Instantiate(result, String)
	symbolic := Instantiate(result, tp)

	if !MorePrecise(concrete, symbolic) {
		t.Fatalf("%v should refine symbolic instantiation %v", concrete, symbolic)
	}
	if MorePrecise(symbolic, concrete) {
		t.Fatalf("symbolic instantiation %v should not refine %v", symbolic, concrete)
	}
	if MorePrecise(String, tp) {
		t.Fatal("free concrete type should not globally refine a type parameter outside an instantiation argument")
	}
}

func TestContainsFreeTypeParamTreatsClosedInstantiationAsClosed(t *testing.T) {
	tp := NewTypeParam("T", nil)
	box := NewGeneric("Box", []*TypeParam{tp}, NewRecord().Field("value", tp).Build())

	if ContainsFreeTypeParam(Instantiate(box, String)) {
		t.Fatal("closed Box<string> should not report a free type parameter")
	}
	if !ContainsFreeTypeParam(Instantiate(box, tp)) {
		t.Fatal("Box<T> with symbolic argument should report a free type parameter")
	}
}

func TestContainsFreeTypeParamKeepsFunctionOwnedParamsScoped(t *testing.T) {
	tp := NewTypeParam("T", nil)
	fn := Func().TypeParamRef(tp).Returns(tp).Build()
	closedFunctionWithFreeSibling := NewRecord().
		Field("call", fn).
		Field("value", tp).
		Build()

	if !ContainsFreeTypeParam(closedFunctionWithFreeSibling) {
		t.Fatal("free sibling type parameter was hidden by function-owned parameter scan")
	}
	if ContainsFreeTypeParam(fn) {
		t.Fatal("function-owned type parameter was reported as free")
	}
}

func TestRefineWithFallbackRepairsTypeParamLeafAndKeepsLiteral(t *testing.T) {
	tp := NewTypeParam("T", nil)
	summary := NewRecord().
		Field("value", LiteralString("hello")).
		Field("get", Func().OptParam("self", Any).Returns(tp).Build()).
		Build()
	fallback := NewRecord().
		Field("value", String).
		Field("get", Func().OptParam("self", Self).Returns(String).Build()).
		Build()

	refined, changed := RefineWithFallback(summary, fallback)
	if !changed {
		t.Fatal("RefineWithFallback did not repair open type-param leaf")
	}
	rec, ok := refined.(*Record)
	if !ok {
		t.Fatalf("refined = %T, want record", refined)
	}
	value := rec.GetField("value")
	if value == nil || !TypeEquals(value.Type, LiteralString("hello")) {
		t.Fatalf("value field = %#v, want literal hello", value)
	}
	get := rec.GetField("get")
	if get == nil {
		t.Fatal("missing get field")
	}
	fn, ok := get.Type.(*Function)
	if get == nil || !ok || len(fn.Returns) != 1 || !TypeEquals(fn.Returns[0], String) {
		t.Fatalf("get field = %#v, want function returning string", get)
	}
}

func TestRefineWithFallbackDoesNotReplaceWholeConcreteLeaf(t *testing.T) {
	refined, changed := RefineWithFallback(String, LiteralString("signature-only"))
	if changed {
		t.Fatalf("RefineWithFallback changed concrete summary leaf to %v", refined)
	}
	if !TypeEquals(refined, String) {
		t.Fatalf("refined = %v, want original string summary", refined)
	}
}

func TestRefineWithFallbackRepairsFunctionReturnDespiteParamShapeMismatch(t *testing.T) {
	tp := NewTypeParam("T", nil)
	summary := Func().
		OptParam("self", Any).
		Returns(tp).
		Build()
	fallback := Func().
		Param("self", NewRecord().Field("value", String).Build()).
		Returns(String).
		Build()

	refined, changed := RefineWithFallback(summary, fallback)
	if !changed {
		t.Fatal("RefineWithFallback did not repair covariant return")
	}
	fn, ok := refined.(*Function)
	if !ok {
		t.Fatalf("refined = %T, want function", refined)
	}
	if len(fn.Params) != 1 || !fn.Params[0].Optional || !TypeEquals(fn.Params[0].Type, Any) {
		t.Fatalf("param = %#v, want original optional any parameter", fn.Params)
	}
	if len(fn.Returns) != 1 || !TypeEquals(fn.Returns[0], String) {
		t.Fatalf("returns = %#v, want string", fn.Returns)
	}
}

func TestRefineWithFallbackRepairsFunctionReturnWithInstantiatedSelfParam(t *testing.T) {
	tp := NewTypeParam("T", nil)
	container := NewGeneric("Container", []*TypeParam{tp},
		NewRecord().
			Field("value", tp).
			Field("get", Func().Param("self", Instantiate(NewGeneric("Container", []*TypeParam{tp}, NewRecord().Build()), tp)).Returns(tp).Build()).
			Build(),
	)
	summary := NewRecord().
		Field("value", LiteralString("hello")).
		Field("get", Func().OptParam("self", Any).Returns(tp).Build()).
		Build()
	fallback := NewRecord().
		Field("value", String).
		Field("get", Func().Param("self", Instantiate(container, String)).Returns(String).Build()).
		Build()

	refined, changed := RefineWithFallback(summary, fallback)
	if !changed {
		t.Fatal("RefineWithFallback did not repair function return with instantiated self fallback")
	}
	rec, ok := refined.(*Record)
	if !ok {
		t.Fatalf("refined = %T, want record", refined)
	}
	get := rec.GetField("get")
	fn, ok := get.Type.(*Function)
	if get == nil || !ok || len(fn.Returns) != 1 || !TypeEquals(fn.Returns[0], String) {
		t.Fatalf("get field = %#v, want function returning string", get)
	}
}

func TestRefineWithFallbackRepairsDeferredRefLeaf(t *testing.T) {
	summary := Func().
		OptParam("self", Any).
		Returns(NewRef("", "T")).
		Build()
	fallback := Func().
		Param("self", NewRecord().Field("value", String).Build()).
		Returns(String).
		Build()

	refined, changed := RefineWithFallback(summary, fallback)
	if !changed {
		t.Fatal("RefineWithFallback did not repair deferred ref leaf")
	}
	fn, ok := refined.(*Function)
	if !ok || len(fn.Returns) != 1 || !TypeEquals(fn.Returns[0], String) {
		t.Fatalf("refined = %v, want function returning string", refined)
	}
}

func TestNeedsSameExpressionFallbackFindsNestedRepairableLeaves(t *testing.T) {
	clean := NewRecord().
		Field("value", LiteralString("hello")).
		Field("get", Func().OptParam("self", Any).Returns(String).Build()).
		Build()
	if NeedsSameExpressionFallback(clean) {
		t.Fatalf("clean record reported fallback need: %v", clean)
	}

	open := NewRecord().
		Field("value", LiteralString("hello")).
		Field("get", Func().OptParam("self", Any).Returns(NewRef("", "T")).Build()).
		Build()
	if !NeedsSameExpressionFallback(open) {
		t.Fatalf("open record did not report fallback need: %v", open)
	}
}

func TestNeedsSameExpressionFallbackUsesRecursiveFamilySeen(t *testing.T) {
	node := NewRecursive("Node", func(self Type) Type {
		return NewRecord().
			Field("next", NewOptional(self)).
			Field("get", Func().OptParam("self", Any).Returns(self).Build()).
			Build()
	})
	surface := NewRecord().
		Field("left", node).
		Field("right", node).
		Build()

	if NeedsSameExpressionFallback(surface) {
		t.Fatalf("closed recursive surface reported fallback need: %v", surface)
	}

	open := NewRecursive("OpenNode", func(self Type) Type {
		return NewRecord().
			Field("next", NewOptional(self)).
			Field("get", Func().OptParam("self", Any).Returns(NewRef("", "T")).Build()).
			Build()
	})
	if !NeedsSameExpressionFallback(open) {
		t.Fatalf("recursive surface with open return did not report fallback need: %v", open)
	}
}

func TestRefineWithFallbackPreservesFunctionOwnedTypeParam(t *testing.T) {
	tp := NewTypeParam("T", nil)
	summary := Func().TypeParamRef(tp).Param("value", tp).Returns(tp).Build()
	fallback := Func().Param("value", String).Returns(String).Build()

	refined, changed := RefineWithFallback(summary, fallback)
	if changed || refined != summary {
		t.Fatalf("owned type param should not be repaired: %v changed=%v", refined, changed)
	}
}

func TestComparePrecisionUnrelatedShapesNotComparable(t *testing.T) {
	if strict, comparable := ComparePrecision(NewArray(String), NewMap(String, String)); strict || comparable {
		t.Fatalf("array/map precision = (%v, %v), want unrelated", strict, comparable)
	}
}

func TestComparePrecisionRecursiveAliasCycleTerminates(t *testing.T) {
	rec := NewRecursivePlaceholder("Cycle")
	alias := NewAlias("CycleAlias", rec)
	rec.SetBody(alias)

	if strict, comparable := ComparePrecision(String, alias); strict || !comparable {
		t.Fatalf("string vs recursive alias cycle precision = (%v, %v), want non-strict comparable", strict, comparable)
	}
}

func TestPruneLessPreciseRefinableUnionMembers_DropsSoftMapAlternative(t *testing.T) {
	entry := NewRecord().Field("id", String).Build()
	soft := NewMap(String, NewArray(Any))
	precise := NewRecursive("Flow", func(self Type) Type {
		return NewMap(String, NewArray(entry))
	})

	got := PruneLessPreciseRefinableUnionMembers(NewUnion(soft, precise))
	if !TypeEquals(got, precise) {
		t.Fatalf("pruned union = %v, want %v", got, precise)
	}
}

func TestMorePreciseMapRefinesOpenRecordMapComponent(t *testing.T) {
	entry := NewRecord().
		Field("id", String).
		Field("meta", NewRecord().Field("suite", String).Build()).
		Build()
	candidate := NewMap(String, NewArray(entry))
	baseline := NewRecord().
		SetOpen(true).
		MapComponent(String, NewArray(Unknown)).
		Build()

	if !MorePrecise(candidate, baseline) {
		t.Fatalf("precise map should refine open record-map component: %v vs %v", candidate, baseline)
	}
	if MorePrecise(baseline, candidate) {
		t.Fatal("open record-map component should not refine precise map")
	}
}

func TestMorePreciseRecursiveMapRefinesOpenRecordMapComponent(t *testing.T) {
	entry := NewRecord().
		Field("id", String).
		Field("meta", NewOptional(NewMap(String, Any))).
		Build()
	candidate := NewRecursive("Flow", func(self Type) Type {
		return NewMap(String, NewArray(entry))
	})
	baseline := NewRecord().
		SetOpen(true).
		MapComponent(String, NewArray(Unknown)).
		Build()

	if !MorePrecise(candidate, baseline) {
		t.Fatalf("recursive map product should refine open record-map component: %v vs %v", candidate, baseline)
	}
	if MorePrecise(baseline, candidate) {
		t.Fatal("open record-map component should not refine recursive map product")
	}
}

func TestMorePreciseRecursiveRecordAddsEvidence(t *testing.T) {
	baseline := NewRecursive("Suite", func(self Type) Type {
		return NewRecord().
			Field("name", String).
			Field("children", NewArray(self)).
			Build()
	})
	candidate := NewRecursive("Suite", func(self Type) Type {
		return NewRecord().
			Field("name", String).
			Field("children", NewArray(self)).
			Field("full_path", String).
			Build()
	})

	if !MorePrecise(candidate, baseline) {
		t.Fatal("recursive record with extra evidence field should refine the baseline family")
	}
	if MorePrecise(baseline, candidate) {
		t.Fatal("baseline recursive record should not refine the richer candidate")
	}
}

func TestProductFamilyHashTerminatesOnRecursiveMapTower(t *testing.T) {
	node := NewRecursive("Node", func(self Type) Type {
		return NewMap(String, NewOptional(self))
	})
	var tower Type = node
	for i := 0; i < 2048; i++ {
		tower = NewMap(String, NewOptional(NewUnion(tower, Nil)))
	}

	if got := ProductFamilyHash(tower); got == 0 {
		t.Fatal("recursive map tower family hash should be non-zero")
	}
}

func TestNeedsSameExpressionFallbackUsesRecursiveAliasFamilySeen(t *testing.T) {
	node := NewRecursivePlaceholder("Node")
	var tower Type = NewAlias("NodeAlias", NewRecord().
		Field("next", node).
		Build())
	for i := 0; i < 512; i++ {
		tower = NewAlias("TowerAlias", NewMap(String, NewOptional(NewUnion(tower, Nil))))
	}
	node.SetBody(NewRecord().
		Field("next", tower).
		Field("hole", Unknown).
		Build())

	if !NeedsSameExpressionFallback(tower) {
		t.Fatal("recursive alias family with an unknown leaf should need fallback")
	}
}

func TestNeedsSameExpressionFallbackWithinReportsIncomplete(t *testing.T) {
	var tower Type = String
	for i := 0; i < 64; i++ {
		tower = NewMap(String, NewOptional(tower))
	}

	needs, complete := NeedsSameExpressionFallbackWithin(tower, 8)
	if needs || complete {
		t.Fatalf("bounded fallback scan = needs %v complete %v, want false/false", needs, complete)
	}
}

func TestSameProductFamily_EquivalentRecursiveProducts(t *testing.T) {
	left := NewRecursive("Node", func(self Type) Type {
		return NewRecord().
			Field("name", String).
			OptField("next", self).
			Build()
	})
	right := NewRecursive("Node", func(self Type) Type {
		return NewRecord().
			Field("name", String).
			OptField("next", self).
			Build()
	})

	if !SameProductFamily(left, right) {
		t.Fatal("same recursive product family should compare equal without structural unfolding")
	}
}

func TestSameProductFamily_DistinguishesPrecisionVariants(t *testing.T) {
	base := NewRecursive("Node", func(self Type) Type {
		return NewRecord().
			Field("name", String).
			OptField("next", self).
			Build()
	})
	richer := NewRecursive("Node", func(self Type) Type {
		return NewRecord().
			Field("name", String).
			Field("path", String).
			OptField("next", self).
			Build()
	})

	if SameProductFamily(base, richer) {
		t.Fatal("same product family must not collapse strictly richer evidence")
	}
}

func TestComparePrecisionRecursiveUnionUsesExistingFamilies(t *testing.T) {
	base := NewRecursive("Suite", func(self Type) Type {
		return NewRecord().
			Field("name", String).
			Field("children", NewArray(self)).
			Build()
	})
	withParent := NewRecursive("Suite", func(self Type) Type {
		return NewRecord().
			Field("name", String).
			Field("children", NewArray(self)).
			OptField("parent", NewUnion(self, base)).
			Build()
	})
	withPath := NewRecursive("Suite", func(self Type) Type {
		return NewRecord().
			Field("name", String).
			Field("children", NewArray(self)).
			Field("full_path", String).
			OptField("parent", NewUnion(self, withParent)).
			Build()
	})

	if _, comparable := ComparePrecision(NewUnion(withParent, withPath), NewUnion(base, withParent)); !comparable {
		t.Fatal("recursive union precision comparison should use existing family evidence")
	}
}
