package typ

import "testing"

func TestContainsAny_RecursiveTypeTerminates(t *testing.T) {
	node := NewRecursive("Node", func(self Type) Type {
		return newRecord().
			Field("next", NewOptional(self)).
			Field("value", String).
			Build()
	})

	if ContainsAny(node) {
		t.Fatal("ContainsAny() reported any in recursive type with only concrete fields")
	}
}

func TestContainsRecursive_UsesProductFlags(t *testing.T) {
	node := NewRecursive("Node", func(self Type) Type {
		return newRecord().Field("next", NewOptional(self)).Build()
	})
	if !ContainsRecursive(NewUnion(String, node)) {
		t.Fatal("ContainsRecursive() missed recursive union member")
	}
	if ContainsRecursive(newRecord().Field("name", String).Build()) {
		t.Fatal("ContainsRecursive() reported recursion in concrete record")
	}
}

func TestContainsAny_RecursiveTypeFindsNestedAny(t *testing.T) {
	node := NewRecursive("Node", func(self Type) Type {
		return newRecord().
			Field("next", NewOptional(self)).
			Field("value", Any).
			Build()
	})

	if !ContainsAny(node) {
		t.Fatal("ContainsAny() missed any inside recursive type")
	}
}

func TestContainsAny_NestedFunctionType(t *testing.T) {
	fn := Func().
		Returns(Func().
			Param("x", String).
			Returns(Any).
			Build()).
		Build()

	if !ContainsAny(fn) {
		t.Fatal("ContainsAny() missed any inside nested function return")
	}
}

func TestContains_PrunesEquivalentRebuiltStructuralNodes(t *testing.T) {
	leaf := func() Type {
		return newRecord().Field("payload", String).Build()
	}
	root := newRecord().
		Field("left", NewArray(leaf())).
		Field("right", NewArray(leaf())).
		Build()

	leafVisits := 0
	Contains(root, func(node Type) bool {
		rec, ok := node.(*Record)
		if !ok || len(rec.Fields) != 1 {
			return false
		}
		field := rec.GetField("payload")
		if field != nil && typeEquals(field.Type, String) {
			leafVisits++
		}
		return false
	})

	if leafVisits != 1 {
		t.Fatalf("expected equivalent rebuilt record to be visited once, got %d visits", leafVisits)
	}
}

func TestContains_PrunesEquivalentFunctionReturnShapes(t *testing.T) {
	resultShape := func() Type {
		return Func().
			Param("ctx", newRecord().Field("id", String).Build()).
			Returns(newRecord().
				Field("ok", Boolean).
				Field("value", Number).
				Build()).
			Build()
	}
	root := NewTuple(resultShape(), resultShape(), resultShape())

	functionVisits := 0
	Contains(root, func(node Type) bool {
		if _, ok := node.(*Function); ok {
			functionVisits++
		}
		return false
	})

	if functionVisits != 1 {
		t.Fatalf("expected equivalent function return shapes to be visited once, got %d visits", functionVisits)
	}
}

func TestContains_FindsTypeParamInsideRepeatedShape(t *testing.T) {
	tp := NewTypeParam("T", nil)
	repeated := func() Type {
		return Func().
			Param("input", String).
			Returns(newRecord().Field("value", tp).Build()).
			Build()
	}
	root := NewTuple(repeated(), repeated())

	if !Contains(root, func(node Type) bool {
		_, ok := node.(*TypeParam)
		return ok
	}) {
		t.Fatal("Contains() missed type parameter inside repeated structural shape")
	}
	if !ContainsTypeParam(root) {
		t.Fatal("ContainsTypeParam() missed type parameter inside repeated structural shape")
	}
}

func TestContains_NoDepthCapFindsDeepNestedPredicate(t *testing.T) {
	tpe := Never
	for i := 0; i < DefaultRecursionDepth+8; i++ {
		tpe = newRecord().Field("next", tpe).Build()
	}

	if !Contains(tpe, IsNever) {
		t.Fatal("Contains() missed predicate beyond the old recursion-depth budget")
	}
}

func TestContains_PrunesEquivalentRecursiveBranches(t *testing.T) {
	nodeShape := func() Type {
		return NewRecursive("Node", func(self Type) Type {
			return newRecord().
				Field("next", NewOptional(self)).
				Field("value", String).
				Build()
		})
	}
	root := NewTuple(nodeShape(), nodeShape())

	recursiveVisits := 0
	Contains(root, func(node Type) bool {
		if _, ok := node.(*Recursive); ok {
			recursiveVisits++
		}
		return false
	})

	if recursiveVisits != 1 {
		t.Fatalf("expected equivalent recursive branch to be visited once, got %d visits", recursiveVisits)
	}
}

func TestContainsAny_SeesAnyThroughLaterAssignedRecursiveBody(t *testing.T) {
	a := NewRecursivePlaceholder("A")
	b := NewRecursivePlaceholder("B")

	a.SetBody(newRecord().
		Field("b", b).
		Build())
	b.SetBody(newRecord().
		Field("value", Any).
		Build())

	if !ContainsAny(a) {
		t.Fatal("ContainsAny() missed any through recursive body assigned after wrapper construction")
	}
}

func TestContainsAny_TypeParamConstraint(t *testing.T) {
	tp := NewTypeParam("T", newRecord().Field("value", Any).Build())
	fn := Func().TypeParam("T", tp.Constraint).Returns(tp).Build()

	if !ContainsAny(fn) {
		t.Fatal("ContainsAny() missed any inside type parameter constraint")
	}
}

func TestContainsTypeParam_RecursiveConstraintTerminates(t *testing.T) {
	rec := NewRecursive("Box", func(self Type) Type {
		tp := NewTypeParam("T", self)
		return newRecord().
			Field("value", tp).
			Field("next", NewOptional(self)).
			Build()
	})

	if !ContainsTypeParam(rec) {
		t.Fatal("ContainsTypeParam() missed type parameter inside recursive body")
	}
}

func TestContainsAny_NonRecursiveStructuralFlag(t *testing.T) {
	withoutAny := Func().
		Param("input", newRecord().
			Field("meta", NewMap(String, Number)).
			Field("items", NewArray(newRecord().Field("id", String).Build())).
			Build()).
		Returns(NewTuple(String, Number)).
		Build()
	withAny := Func().
		Param("input", withoutAny).
		Returns(newRecord().Field("value", Any).Build()).
		Build()

	if ContainsAny(withoutAny) {
		t.Fatal("ContainsAny() reported any in concrete non-recursive product")
	}
	if !ContainsAny(withAny) {
		t.Fatal("ContainsAny() missed any recorded in non-recursive product flag")
	}
}

func TestContainsInstantiated_NonRecursiveStructuralFlag(t *testing.T) {
	tp := NewTypeParam("T", nil)
	box := NewGeneric("Box", []*TypeParam{tp}, newRecord().Field("value", tp).Build())
	withoutInst := newRecord().
		Field("id", String).
		Field("items", NewArray(newRecord().Field("name", String).Build())).
		Build()
	withInst := newRecord().
		Field("box", Instantiate(box, Number)).
		Build()

	if ContainsInstantiated(withoutInst) {
		t.Fatal("ContainsInstantiated() reported instantiation in concrete product")
	}
	if !ContainsInstantiated(withInst) {
		t.Fatal("ContainsInstantiated() missed nested instantiation flag")
	}
}

func TestContainsNever_NonRecursiveStructuralFlag(t *testing.T) {
	withoutNever := Func().
		Param("input", newRecord().
			Field("meta", NewMap(String, Number)).
			Field("items", NewArray(newRecord().Field("id", String).Build())).
			Build()).
		Returns(NewTuple(String, Number)).
		Build()
	withNever := Func().
		Param("input", withoutNever).
		Returns(newRecord().Field("value", Never).Build()).
		Build()

	if ContainsNever(withoutNever) {
		t.Fatal("ContainsNever() reported never in concrete non-recursive product")
	}
	if !ContainsNever(withNever) {
		t.Fatal("ContainsNever() missed never recorded in non-recursive product flag")
	}
}

func TestContainsInstantiated_SeesLaterAssignedRecursiveBody(t *testing.T) {
	tp := NewTypeParam("T", nil)
	box := NewGeneric("Box", []*TypeParam{tp}, newRecord().Field("value", tp).Build())
	a := NewRecursivePlaceholder("A")
	b := NewRecursivePlaceholder("B")

	a.SetBody(newRecord().Field("b", b).Build())
	b.SetBody(newRecord().Field("box", Instantiate(box, String)).Build())

	if !ContainsInstantiated(a) {
		t.Fatal("ContainsInstantiated() missed instantiation through later assigned recursive body")
	}
}

func TestContainsPredicates_ClosedRecursiveProductUsesStableFlags(t *testing.T) {
	node := NewRecursive("Node", func(self Type) Type {
		return newRecord().
			Field("next", NewOptional(self)).
			Field("value", String).
			Build()
	})
	fn := Func().
		Param("node", node).
		Returns(newRecord().
			Field("root", node).
			Field("children", NewArray(node)).
			Build()).
		Build()

	if knownContainsOpenRecursive(fn) {
		t.Fatal("closed recursive product should not require dynamic predicate scan")
	}
	if ContainsAny(fn) {
		t.Fatal("ContainsAny() reported any in closed concrete recursive product")
	}
	if ContainsTypeParam(fn) {
		t.Fatal("ContainsTypeParam() reported type parameter in closed concrete recursive product")
	}
	if ContainsInstantiated(fn) {
		t.Fatal("ContainsInstantiated() reported instantiation in closed concrete recursive product")
	}
	if ContainsNever(fn) {
		t.Fatal("ContainsNever() reported never in closed concrete recursive product")
	}
}

func TestContainsAny_OpenRecursiveProductSeesLaterBodyMutation(t *testing.T) {
	node := NewRecursivePlaceholder("Node")
	wrapper := newRecord().
		Field("root", node).
		Field("label", String).
		Build()

	if !knownContainsOpenRecursive(wrapper) {
		t.Fatal("product built over placeholder must remain dynamically checked")
	}
	if ContainsAny(wrapper) {
		t.Fatal("ContainsAny() reported any before recursive placeholder body existed")
	}

	node.SetBody(newRecord().Field("value", Any).Build())

	if !ContainsAny(wrapper) {
		t.Fatal("ContainsAny() missed any introduced by later recursive placeholder body")
	}
}

func TestContainsTypeParam_OpenRecursiveProductSeesLaterBodyMutation(t *testing.T) {
	node := NewRecursivePlaceholder("Node")
	wrapper := NewTuple(String, node)

	if !knownContainsOpenRecursive(wrapper) {
		t.Fatal("tuple built over placeholder must remain dynamically checked")
	}
	if ContainsTypeParam(wrapper) {
		t.Fatal("ContainsTypeParam() reported type parameter before recursive placeholder body existed")
	}

	node.SetBody(newRecord().Field("value", NewTypeParam("T", nil)).Build())

	if !ContainsTypeParam(wrapper) {
		t.Fatal("ContainsTypeParam() missed type parameter introduced by later recursive placeholder body")
	}
}

func TestContainsInstantiated_OpenRecursiveProductSeesLaterBodyMutation(t *testing.T) {
	tp := NewTypeParam("T", nil)
	box := NewGeneric("Box", []*TypeParam{tp}, newRecord().Field("value", tp).Build())
	node := NewRecursivePlaceholder("Node")
	wrapper := NewMap(String, node)

	if !knownContainsOpenRecursive(wrapper) {
		t.Fatal("map built over placeholder must remain dynamically checked")
	}
	if ContainsInstantiated(wrapper) {
		t.Fatal("ContainsInstantiated() reported instantiation before recursive placeholder body existed")
	}

	node.SetBody(newRecord().Field("box", Instantiate(box, Number)).Build())

	if !ContainsInstantiated(wrapper) {
		t.Fatal("ContainsInstantiated() missed instantiation introduced by later recursive placeholder body")
	}
}

func TestContainsNever_OpenRecursiveProductSeesLaterBodyMutation(t *testing.T) {
	node := NewRecursivePlaceholder("Node")
	wrapper := NewTuple(String, node)

	if !knownContainsOpenRecursive(wrapper) {
		t.Fatal("tuple built over placeholder must remain dynamically checked")
	}
	if ContainsNever(wrapper) {
		t.Fatal("ContainsNever() reported never before recursive placeholder body existed")
	}

	node.SetBody(newRecord().Field("value", Never).Build())

	if !ContainsNever(wrapper) {
		t.Fatal("ContainsNever() missed never introduced by later recursive placeholder body")
	}
}

func TestContainsReadonlyMapTraversesKeyAndValue(t *testing.T) {
	key := NewTypeParam("K", nil)
	value := NewTypeParam("V", nil)
	wrapper := NewReadonlyMap(key, value)

	if !Contains(wrapper, func(node Type) bool { return node == key }) {
		t.Fatal("Contains() missed ReadonlyMap key")
	}
	if !Contains(wrapper, func(node Type) bool { return node == value }) {
		t.Fatal("Contains() missed ReadonlyMap value")
	}
}

func TestContainsReadonlyMapDynamicPredicatesTraverseKeyAndValue(t *testing.T) {
	cases := []struct {
		name     string
		contains func(Type) bool
		marker   func() Type
	}{
		{name: "any", contains: ContainsAny, marker: func() Type { return Any }},
		{name: "never", contains: ContainsNever, marker: func() Type { return Never }},
		{name: "type-param", contains: ContainsTypeParam, marker: func() Type {
			return NewTypeParam("T", nil)
		}},
		{name: "instantiated", contains: ContainsInstantiated, marker: traversalInstantiatedMarker},
	}
	positions := []struct {
		name string
		wrap func(*Recursive) Type
	}{
		{name: "key", wrap: func(node *Recursive) Type { return NewReadonlyMap(node, String) }},
		{name: "value", wrap: func(node *Recursive) Type { return NewReadonlyMap(String, node) }},
	}

	for _, tc := range cases {
		for _, pos := range positions {
			t.Run(tc.name+"/"+pos.name, func(t *testing.T) {
				node := NewRecursivePlaceholder("Node")
				wrapper := pos.wrap(node)

				if !knownContainsOpenRecursive(wrapper) {
					t.Fatal("ReadonlyMap built over placeholder must remain dynamically checked")
				}
				if tc.contains(wrapper) {
					t.Fatal("predicate reported marker before recursive placeholder body existed")
				}

				node.SetBody(newRecord().Field("value", tc.marker()).Build())

				if !tc.contains(wrapper) {
					t.Fatal("predicate missed marker introduced through ReadonlyMap")
				}
			})
		}
	}
}

func TestContainsStaticMemberTraversesTypes(t *testing.T) {
	stringMember := NewTypeParam("S", nil)
	intMember := NewTypeParam("I", nil)
	wrapper := newRecord().
		StaticStringIndex("name", stringMember).
		StaticIntIndex(1, intMember).
		Build()

	if !Contains(wrapper, func(node Type) bool { return node == stringMember }) {
		t.Fatal("Contains() missed string static member type")
	}
	if !Contains(wrapper, func(node Type) bool { return node == intMember }) {
		t.Fatal("Contains() missed integer static member type")
	}
}

func TestContainsStaticMemberDynamicPredicatesTraverseTypes(t *testing.T) {
	cases := []struct {
		name     string
		contains func(Type) bool
		marker   func() Type
	}{
		{name: "any", contains: ContainsAny, marker: func() Type { return Any }},
		{name: "never", contains: ContainsNever, marker: func() Type { return Never }},
		{name: "type-param", contains: ContainsTypeParam, marker: func() Type {
			return NewTypeParam("T", nil)
		}},
		{name: "instantiated", contains: ContainsInstantiated, marker: traversalInstantiatedMarker},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			node := NewRecursivePlaceholder("Node")
			wrapper := newRecord().
				StaticStringIndex("node", node).
				Build()

			if !knownContainsOpenRecursive(wrapper) {
				t.Fatal("record built over static-member placeholder must remain dynamically checked")
			}
			if tc.contains(wrapper) {
				t.Fatal("predicate reported marker before recursive placeholder body existed")
			}

			node.SetBody(newRecord().Field("value", tc.marker()).Build())

			if !tc.contains(wrapper) {
				t.Fatal("predicate missed marker introduced through static member")
			}
		})
	}
}

func traversalInstantiatedMarker() Type {
	tp := NewTypeParam("T", nil)
	box := NewGeneric("Box", []*TypeParam{tp}, newRecord().Field("value", tp).Build())
	return Instantiate(box, Number)
}
