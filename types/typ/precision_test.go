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
