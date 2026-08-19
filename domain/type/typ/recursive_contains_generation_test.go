package typ

import "testing"

func TestRecursiveContainsMemoTracksNestedGenerations(t *testing.T) {
	generic := NewGeneric("Box", []*TypeParam{NewTypeParam("T", nil)}, String)
	cases := []struct {
		name      string
		marker    func() Type
		predicate func(Type) bool
	}{
		{name: "any", marker: func() Type { return Any }, predicate: ContainsAny},
		{name: "never", marker: func() Type { return Never }, predicate: ContainsNever},
		{name: "type-param", marker: func() Type { return NewTypeParam("T", nil) }, predicate: ContainsTypeParam},
		{name: "instantiated", marker: func() Type { return Instantiate(generic, String) }, predicate: ContainsInstantiated},
		{name: "generic", marker: func() Type { return generic }, predicate: ContainsGeneric},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			child := NewRecursivePlaceholder("Child")
			parent := NewRecursive("Parent", func(Type) Type {
				return newRecord().Field("child", child).Build()
			})

			if tc.predicate(parent) {
				t.Fatal("unresolved child unexpectedly contains marker")
			}
			if parent.containsMemo.Load() != nil {
				t.Fatal("incomplete recursive graph must not publish a definitive negative memo")
			}

			child.SetBody(newRecord().Field("marker", tc.marker()).Build())
			if !tc.predicate(parent) {
				t.Fatal("parent memo missed marker introduced by late-filled child")
			}
			if parent.containsMemo.Load() == nil {
				t.Fatal("parent memo did not publish once the nested child body was sealed")
			}
		})
	}
}

func TestRecursiveContainsMemoTracksMutualGraphGenerations(t *testing.T) {
	generic := NewGeneric("Box", []*TypeParam{NewTypeParam("T", nil)}, String)
	cases := []struct {
		name      string
		marker    func() Type
		predicate func(Type) bool
	}{
		{name: "any", marker: func() Type { return Any }, predicate: ContainsAny},
		{name: "never", marker: func() Type { return Never }, predicate: ContainsNever},
		{name: "type-param", marker: func() Type { return NewTypeParam("T", nil) }, predicate: ContainsTypeParam},
		{name: "instantiated", marker: func() Type { return Instantiate(generic, String) }, predicate: ContainsInstantiated},
		{name: "generic", marker: func() Type { return generic }, predicate: ContainsGeneric},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			left := NewRecursivePlaceholder("Left")
			right := NewRecursivePlaceholder("Right")
			left.SetBody(newRecord().Field("right", right).Build())

			if tc.predicate(left) {
				t.Fatal("open mutual graph unexpectedly contains marker")
			}
			if left.containsMemo.Load() != nil {
				t.Fatal("open mutual graph must not publish a definitive negative memo")
			}

			right.SetBody(newRecord().Field("marker", tc.marker()).Field("left", left).Build())
			if !tc.predicate(left) {
				t.Fatal("mutual graph missed introduced marker")
			}
			if left.containsMemo.Load() == nil {
				t.Fatal("mutual graph memo did not publish once the peer body was sealed")
			}
		})
	}
}

func TestRecursiveContainsMemoPreservesDeepDefinitivePositives(t *testing.T) {
	generic := NewGeneric("Box", []*TypeParam{NewTypeParam("T", nil)}, String)
	cases := []struct {
		name      string
		marker    func() Type
		predicate func(Type) bool
	}{
		{name: "any", marker: func() Type { return Any }, predicate: ContainsAny},
		{name: "never", marker: func() Type { return Never }, predicate: ContainsNever},
		{name: "type-param", marker: func() Type { return NewTypeParam("T", nil) }, predicate: ContainsTypeParam},
		{name: "instantiated", marker: func() Type { return Instantiate(generic, String) }, predicate: ContainsInstantiated},
		{name: "generic", marker: func() Type { return generic }, predicate: ContainsGeneric},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			node := NewRecursivePlaceholder("Deep")
			deepMarker := nestInFunctions(tc.marker(), 257)
			node.SetBody(newRecord().Field("marker", deepMarker).OptField("next", node).Build())

			if !tc.predicate(node) {
				t.Fatal("marker nested 257 nodes deep was lost")
			}
			if node.containsMemo.Load() == nil {
				t.Fatal("complete deep recursive graph did not publish containment memo")
			}
		})
	}
}

func TestRecursiveContainsMemoPreservesIntrinsicMarkersWithRecursiveBodies(t *testing.T) {
	node := NewRecursivePlaceholder("Node")
	param := NewTypeParam("T", Func().Param("label", String).Returns(Any).Build())
	generic := NewGeneric("Box", []*TypeParam{param}, nil)
	generic.SetBody(RebuildRecord(RecordParts{Fields: []Field{
		{Name: "next", Type: MaterializeOptional(node)},
		{Name: "value", Type: param},
	}}))
	node.SetBody(RebuildRecord(RecordParts{Fields: []Field{{
		Name: "box", Type: Instantiate(generic, Func().Param("label", Number).Returns(String).Build()),
	}}}))

	for name, predicate := range map[string]func(Type) bool{
		"any":          ContainsAny,
		"instantiated": ContainsInstantiated,
		"generic":      ContainsGeneric,
	} {
		t.Run(name, func(t *testing.T) {
			if !predicate(node) {
				t.Fatalf("recursive generic graph lost intrinsic %s marker", name)
			}
		})
	}
}

// TestRecursiveContainsMemoTreatsInstantiatedFormalsAsBound builds two
// independent recursive graphs around the same generic declaration -- one
// instantiated with a closed argument, one with the declaration's own free
// formal -- and checks ContainsTypeParam distinguishes them.
func TestRecursiveContainsMemoTreatsInstantiatedFormalsAsBound(t *testing.T) {
	param := NewTypeParam("T", nil)
	generic := NewGeneric("Box", []*TypeParam{param}, nil)
	generic.SetBody(RebuildRecord(RecordParts{Fields: []Field{
		{Name: "value", Type: param},
	}}))

	bound := NewRecursivePlaceholder("Bound")
	bound.SetBody(RebuildRecord(RecordParts{Fields: []Field{{
		Name: "box", Type: Instantiate(generic, String),
	}}}))
	if ContainsTypeParam(bound) {
		t.Fatal("recursive generic application exposed a declaration formal as free")
	}

	free := NewRecursivePlaceholder("Free")
	free.SetBody(RebuildRecord(RecordParts{Fields: []Field{{
		Name: "box", Type: Instantiate(generic, param),
	}}}))
	if !ContainsTypeParam(free) {
		t.Fatal("recursive generic application lost a free type parameter in its argument")
	}
}

func nestInFunctions(inner Type, depth int) Type {
	for i := 0; i < depth; i++ {
		inner = Func().Returns(inner).Build()
	}
	return inner
}
