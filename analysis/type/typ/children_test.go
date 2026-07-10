package typ

import (
	"reflect"
	"testing"
)

func TestWalkChildrenCanonicalOrder(t *testing.T) {
	leaf := func(name string) Type {
		return NewRef("", name)
	}
	label := func(children map[Type]string, child Type) string {
		got, ok := children[child]
		if !ok {
			t.Fatalf("unexpected child %T %v", child, child)
		}
		return got
	}
	collect := func(node Type, children map[Type]string) []string {
		var got []string
		if !WalkChildren(node, func(child Type) bool {
			got = append(got, label(children, child))
			return false
		}) {
			return got
		}
		t.Fatal("WalkChildren should not stop when visit always returns false")
		return nil
	}

	tuple1 := leaf("tuple1")
	tuple2 := leaf("tuple2")
	tuple3 := leaf("tuple3")
	recordField1 := leaf("recordField1")
	recordField2 := leaf("recordField2")
	recordStatic1 := leaf("recordStatic1")
	recordStatic2 := leaf("recordStatic2")
	recordMetatable := leaf("recordMetatable")
	recordMapKey := leaf("recordMapKey")
	recordMapValue := leaf("recordMapValue")
	genTP1Constraint := leaf("genTP1")
	genTP2Constraint := leaf("genTP2")
	genericBody := leaf("genericBody")
	instArg1 := leaf("instArg1")
	instArg2 := leaf("instArg2")
	ifaceMethod1Leaf := leaf("ifaceMethod1")
	ifaceMethod2Leaf := leaf("ifaceMethod2")
	recursiveBody := leaf("recursiveBody")

	tuple := NewTuple(tuple1, tuple2, tuple3)

	fnTP1 := NewTypeParam("fnTP1", leaf("fnTP1"))
	fnTP2 := NewTypeParam("fnTP2", leaf("fnTP2"))
	function := Func().
		TypeParamRef(fnTP1).
		TypeParamRef(fnTP2).
		Param("a", leaf("fnParam1")).
		Param("b", leaf("fnParam2")).
		Variadic(leaf("fnVariadic")).
		Returns(leaf("fnReturn1"), leaf("fnReturn2")).
		Build()

	record := &Record{
		Fields: []Field{
			{Name: "a", Type: recordField1},
			{Name: "b", Type: recordField2},
		},
		StaticMembers: []StaticMember{
			{Kind: StaticMemberStringIndex, Name: "s1", Type: recordStatic1},
			{Kind: StaticMemberStringIndex, Name: "s2", Type: recordStatic2},
		},
		Metatable: recordMetatable,
		MapKey:    recordMapKey,
		MapValue:  recordMapValue,
	}

	genTP1 := NewTypeParam("genTP1", genTP1Constraint)
	genTP2 := NewTypeParam("genTP2", genTP2Constraint)
	generic := NewGeneric("G", []*TypeParam{genTP1, genTP2}, genericBody)

	instantiated := Instantiate(generic, instArg1, instArg2)

	ifaceMethod1 := Func().Returns(ifaceMethod1Leaf).Build()
	ifaceMethod2 := Func().Returns(ifaceMethod2Leaf).Build()
	iface := NewInterface("I", []Method{
		{Name: "m1", Type: ifaceMethod1},
		{Name: "m2", Type: ifaceMethod2},
	})

	recursive := NewRecursivePlaceholder("R")
	recursive.SetBody(recursiveBody)

	cases := []struct {
		name     string
		node     Type
		children map[Type]string
		want     []string
	}{
		{
			name: "Tuple",
			node: tuple,
			children: map[Type]string{
				tuple1: "tuple1",
				tuple2: "tuple2",
				tuple3: "tuple3",
			},
			want: []string{"tuple1", "tuple2", "tuple3"},
		},
		{
			name: "Function",
			node: function,
			children: map[Type]string{
				fnTP1.Constraint:        "fnTP1",
				fnTP2.Constraint:        "fnTP2",
				function.Params[0].Type: "fnParam1",
				function.Params[1].Type: "fnParam2",
				function.Variadic:       "fnVariadic",
				function.Returns[0]:     "fnReturn1",
				function.Returns[1]:     "fnReturn2",
			},
			want: []string{"fnTP1", "fnTP2", "fnParam1", "fnParam2", "fnVariadic", "fnReturn1", "fnReturn2"},
		},
		{
			name: "Record",
			node: record,
			children: map[Type]string{
				record.Fields[0].Type:        "recordField1",
				record.Fields[1].Type:        "recordField2",
				record.StaticMembers[0].Type: "recordStatic1",
				record.StaticMembers[1].Type: "recordStatic2",
				record.Metatable:             "recordMetatable",
				record.MapKey:                "recordMapKey",
				record.MapValue:              "recordMapValue",
			},
			want: []string{"recordField1", "recordField2", "recordStatic1", "recordStatic2", "recordMetatable", "recordMapKey", "recordMapValue"},
		},
		{
			name: "Generic",
			node: generic,
			children: map[Type]string{
				genTP1.Constraint: "genTP1",
				genTP2.Constraint: "genTP2",
				generic.Body:      "genericBody",
			},
			want: []string{"genTP1", "genTP2", "genericBody"},
		},
		{
			name: "Instantiated",
			node: instantiated,
			children: map[Type]string{
				generic:  "generic",
				instArg1: "instArg1",
				instArg2: "instArg2",
			},
			want: []string{"generic", "instArg1", "instArg2"},
		},
		{
			name: "Interface",
			node: iface,
			children: map[Type]string{
				ifaceMethod1: "ifaceMethod1",
				ifaceMethod2: "ifaceMethod2",
			},
			want: []string{"ifaceMethod1", "ifaceMethod2"},
		},
		{
			name: "Recursive",
			node: recursive,
			children: map[Type]string{
				recursiveBody: "recursiveBody",
			},
			want: []string{"recursiveBody"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := collect(tc.node, tc.children)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("WalkChildren order = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestWalkChildrenSkipsAbsentChildSlots(t *testing.T) {
	recursive := NewRecursivePlaceholder("R")
	function := Func().Build()
	record := &Record{
		Fields:        []Field{{Name: "empty"}},
		StaticMembers: []StaticMember{{Kind: StaticMemberStringIndex, Name: "empty"}},
	}

	for _, node := range []Type{recursive, function, record} {
		if WalkChildren(node, func(child Type) bool {
			t.Fatalf("WalkChildren visited absent child %T %v in %T", child, child, node)
			return true
		}) {
			t.Fatalf("WalkChildren stopped on absent child in %T", node)
		}
	}

	recursive.SetBody(Number)
	visited := false
	WalkChildren(recursive, func(child Type) bool {
		visited = true
		if child != Number {
			t.Fatalf("recursive body child = %v, want number", child)
		}
		return false
	})
	if !visited {
		t.Fatal("WalkChildren missed recursive body after SetBody")
	}
}

func TestWalkChildrenUsesTransparentAnnotationPolicy(t *testing.T) {
	target := NewRef("", "target")
	alias := NewAlias("Alias", target)
	node := NewAnnotated(NewAnnotated(alias, nil), nil)

	var got []Type
	WalkChildren(node, func(child Type) bool {
		got = append(got, child)
		return false
	})

	if len(got) != 1 || got[0] != target {
		t.Fatalf("WalkChildren annotated alias children = %#v, want alias target", got)
	}
}
