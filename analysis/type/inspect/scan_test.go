package inspect

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/kind"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
)

func TestContainsAny_RecursiveTypeTerminates(t *testing.T) {
	node := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return typetable.NewRecord().
			Field("next", typeexpr.Optional(self)).
			Field("value", typ.String).
			Build()
	})

	if ContainsAny(node) {
		t.Fatal("ContainsAny() reported any in recursive type with only concrete fields")
	}
}

func TestContainsRecursive_UsesProductFlags(t *testing.T) {
	node := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return typetable.NewRecord().Field("next", typeexpr.Optional(self)).Build()
	})
	if !ContainsRecursive(typeexpr.Union(typ.String, node)) {
		t.Fatal("ContainsRecursive() missed recursive union member")
	}
	if ContainsRecursive(typetable.NewRecord().Field("name", typ.String).Build()) {
		t.Fatal("ContainsRecursive() reported recursion in concrete record")
	}
}

func TestContainsRecursive_DoesNotCallHashOrEquals(t *testing.T) {
	root := &typ.Tuple{Elements: []typ.Type{
		&typ.Record{Fields: []typ.Field{{Name: "value", Type: panickingIdentityType{}}}},
	}}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("ContainsRecursive() called Hash or Equals: %v", r)
		}
	}()

	if ContainsRecursive(root) {
		t.Fatal("ContainsRecursive() reported recursion through non-recursive fake type")
	}
}

func TestContainsAny_RecursiveTypeFindsNestedAny(t *testing.T) {
	node := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return typetable.NewRecord().
			Field("next", typeexpr.Optional(self)).
			Field("value", typ.Any).
			Build()
	})

	if !ContainsAny(node) {
		t.Fatal("ContainsAny() missed any inside recursive type")
	}
}

func TestContainsAny_NestedFunctionType(t *testing.T) {
	fn := typ.Func().
		Returns(typ.Func().
			Param("x", typ.String).
			Returns(typ.Any).
			Build()).
		Build()

	if !ContainsAny(fn) {
		t.Fatal("ContainsAny() missed any inside nested function return")
	}
}

func TestContains_PrunesEquivalentRebuiltStructuralNodes(t *testing.T) {
	leaf := func() typ.Type {
		return typetable.NewRecord().Field("payload", typ.String).Build()
	}
	root := typetable.NewRecord().
		Field("left", typ.NewArray(leaf())).
		Field("right", typ.NewArray(leaf())).
		Build()

	leafVisits := 0
	Contains(root, func(node typ.Type) bool {
		rec, ok := node.(*typ.Record)
		if !ok || len(rec.Fields) != 1 {
			return false
		}
		field := rec.GetField("payload")
		if field != nil && typ.TypeEquals(field.Type, typ.String) {
			leafVisits++
		}
		return false
	})

	if leafVisits != 1 {
		t.Fatalf("expected equivalent rebuilt record to be visited once, got %d visits", leafVisits)
	}
}

func TestContains_PrunesEquivalentFunctionReturnShapes(t *testing.T) {
	resultShape := func() typ.Type {
		return typ.Func().
			Param("ctx", typetable.NewRecord().Field("id", typ.String).Build()).
			Returns(typetable.NewRecord().
				Field("ok", typ.Boolean).
				Field("value", typ.Number).
				Build()).
			Build()
	}
	root := typ.NewTuple(resultShape(), resultShape(), resultShape())

	functionVisits := 0
	Contains(root, func(node typ.Type) bool {
		if _, ok := node.(*typ.Function); ok {
			functionVisits++
		}
		return false
	})

	if functionVisits != 1 {
		t.Fatalf("expected equivalent function return shapes to be visited once, got %d visits", functionVisits)
	}
}

func TestContains_FindsTypeParamInsideRepeatedShape(t *testing.T) {
	tp := typ.NewTypeParam("T", nil)
	repeated := func() typ.Type {
		return typ.Func().
			Param("input", typ.String).
			Returns(typetable.NewRecord().Field("value", tp).Build()).
			Build()
	}
	root := typ.NewTuple(repeated(), repeated())

	if !Contains(root, func(node typ.Type) bool {
		_, ok := node.(*typ.TypeParam)
		return ok
	}) {
		t.Fatal("Contains() missed type parameter inside repeated structural shape")
	}
	if !ContainsTypeParam(root) {
		t.Fatal("ContainsTypeParam() missed type parameter inside repeated structural shape")
	}
}

func TestContains_NoDepthCapFindsDeepNestedPredicate(t *testing.T) {
	tpe := typ.Never
	for i := 0; i < typ.DefaultRecursionDepth+8; i++ {
		tpe = typetable.NewRecord().Field("next", tpe).Build()
	}

	if !Contains(tpe, typ.IsNever) {
		t.Fatal("Contains() missed predicate beyond the old recursion-depth budget")
	}
}

func TestContains_PrunesEquivalentRecursiveBranches(t *testing.T) {
	nodeShape := func() typ.Type {
		return typ.NewRecursive("Node", func(self typ.Type) typ.Type {
			return typetable.NewRecord().
				Field("next", typeexpr.Optional(self)).
				Field("value", typ.String).
				Build()
		})
	}
	root := typ.NewTuple(nodeShape(), nodeShape())

	recursiveVisits := 0
	Contains(root, func(node typ.Type) bool {
		if _, ok := node.(*typ.Recursive); ok {
			recursiveVisits++
		}
		return false
	})

	if recursiveVisits != 1 {
		t.Fatalf("expected equivalent recursive branch to be visited once, got %d visits", recursiveVisits)
	}
}

func TestContainsAny_SeesAnyThroughLaterAssignedRecursiveBody(t *testing.T) {
	a := typ.NewRecursivePlaceholder("A")
	b := typ.NewRecursivePlaceholder("B")

	a.SetBody(typetable.NewRecord().
		Field("b", b).
		Build())
	b.SetBody(typetable.NewRecord().
		Field("value", typ.Any).
		Build())

	if !ContainsAny(a) {
		t.Fatal("ContainsAny() missed any through recursive body assigned after wrapper construction")
	}
}

func TestContainsAny_TypeParamConstraint(t *testing.T) {
	tp := typ.NewTypeParam("T", typetable.NewRecord().Field("value", typ.Any).Build())
	fn := typ.Func().TypeParam("T", tp.Constraint).Returns(tp).Build()

	if !ContainsAny(fn) {
		t.Fatal("ContainsAny() missed any inside type parameter constraint")
	}
}

func TestContainsTypeParam_RecursiveConstraintTerminates(t *testing.T) {
	rec := typ.NewRecursive("Box", func(self typ.Type) typ.Type {
		tp := typ.NewTypeParam("T", self)
		return typetable.NewRecord().
			Field("value", tp).
			Field("next", typeexpr.Optional(self)).
			Build()
	})

	if !ContainsTypeParam(rec) {
		t.Fatal("ContainsTypeParam() missed type parameter inside recursive body")
	}
}

func TestContainsAny_NonRecursiveStructuralFlag(t *testing.T) {
	withoutAny := typ.Func().
		Param("input", typetable.NewRecord().
			Field("meta", typetable.NewMap(typ.String, typ.Number)).
			Field("items", typ.NewArray(typetable.NewRecord().Field("id", typ.String).Build())).
			Build()).
		Returns(typ.NewTuple(typ.String, typ.Number)).
		Build()
	withAny := typ.Func().
		Param("input", withoutAny).
		Returns(typetable.NewRecord().Field("value", typ.Any).Build()).
		Build()

	if ContainsAny(withoutAny) {
		t.Fatal("ContainsAny() reported any in concrete non-recursive product")
	}
	if !ContainsAny(withAny) {
		t.Fatal("ContainsAny() missed any recorded in non-recursive product flag")
	}
}

func TestContainsInstantiated_NonRecursiveStructuralFlag(t *testing.T) {
	tp := typ.NewTypeParam("T", nil)
	box := typ.NewGeneric("Box", []*typ.TypeParam{tp}, typetable.NewRecord().Field("value", tp).Build())
	withoutInst := typetable.NewRecord().
		Field("id", typ.String).
		Field("items", typ.NewArray(typetable.NewRecord().Field("name", typ.String).Build())).
		Build()
	withInst := typetable.NewRecord().
		Field("box", typ.Instantiate(box, typ.Number)).
		Build()

	if ContainsInstantiated(withoutInst) {
		t.Fatal("ContainsInstantiated() reported instantiation in concrete product")
	}
	if !ContainsInstantiated(withInst) {
		t.Fatal("ContainsInstantiated() missed nested instantiation flag")
	}
}

func TestContainsNever_NonRecursiveStructuralFlag(t *testing.T) {
	withoutNever := typ.Func().
		Param("input", typetable.NewRecord().
			Field("meta", typetable.NewMap(typ.String, typ.Number)).
			Field("items", typ.NewArray(typetable.NewRecord().Field("id", typ.String).Build())).
			Build()).
		Returns(typ.NewTuple(typ.String, typ.Number)).
		Build()
	withNever := typ.Func().
		Param("input", withoutNever).
		Returns(typetable.NewRecord().Field("value", typ.Never).Build()).
		Build()

	if ContainsNever(withoutNever) {
		t.Fatal("ContainsNever() reported never in concrete non-recursive product")
	}
	if !ContainsNever(withNever) {
		t.Fatal("ContainsNever() missed never recorded in non-recursive product flag")
	}
}

func TestContainsInstantiated_SeesLaterAssignedRecursiveBody(t *testing.T) {
	tp := typ.NewTypeParam("T", nil)
	box := typ.NewGeneric("Box", []*typ.TypeParam{tp}, typetable.NewRecord().Field("value", tp).Build())
	a := typ.NewRecursivePlaceholder("A")
	b := typ.NewRecursivePlaceholder("B")

	a.SetBody(typetable.NewRecord().Field("b", b).Build())
	b.SetBody(typetable.NewRecord().Field("box", typ.Instantiate(box, typ.String)).Build())

	if !ContainsInstantiated(a) {
		t.Fatal("ContainsInstantiated() missed instantiation through later assigned recursive body")
	}
}

func TestContainsPredicates_ClosedRecursiveProductUsesStableFlags(t *testing.T) {
	node := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return typetable.NewRecord().
			Field("next", typeexpr.Optional(self)).
			Field("value", typ.String).
			Build()
	})
	fn := typ.Func().
		Param("node", node).
		Returns(typetable.NewRecord().
			Field("root", node).
			Field("children", typ.NewArray(node)).
			Build()).
		Build()

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
	node := typ.NewRecursivePlaceholder("Node")
	wrapper := typetable.NewRecord().
		Field("root", node).
		Field("label", typ.String).
		Build()

	if ContainsAny(wrapper) {
		t.Fatal("ContainsAny() reported any before recursive placeholder body existed")
	}

	node.SetBody(typetable.NewRecord().Field("value", typ.Any).Build())

	if !ContainsAny(wrapper) {
		t.Fatal("ContainsAny() missed any introduced by later recursive placeholder body")
	}
}

func TestContainsTypeParam_OpenRecursiveProductSeesLaterBodyMutation(t *testing.T) {
	node := typ.NewRecursivePlaceholder("Node")
	wrapper := typ.NewTuple(typ.String, node)

	if ContainsTypeParam(wrapper) {
		t.Fatal("ContainsTypeParam() reported type parameter before recursive placeholder body existed")
	}

	node.SetBody(typetable.NewRecord().Field("value", typ.NewTypeParam("T", nil)).Build())

	if !ContainsTypeParam(wrapper) {
		t.Fatal("ContainsTypeParam() missed type parameter introduced by later recursive placeholder body")
	}
}

func TestContainsInstantiated_OpenRecursiveProductSeesLaterBodyMutation(t *testing.T) {
	tp := typ.NewTypeParam("T", nil)
	box := typ.NewGeneric("Box", []*typ.TypeParam{tp}, typetable.NewRecord().Field("value", tp).Build())
	node := typ.NewRecursivePlaceholder("Node")
	wrapper := typ.NewMap(typ.String, node)

	if ContainsInstantiated(wrapper) {
		t.Fatal("ContainsInstantiated() reported instantiation before recursive placeholder body existed")
	}

	node.SetBody(typetable.NewRecord().Field("box", typ.Instantiate(box, typ.Number)).Build())

	if !ContainsInstantiated(wrapper) {
		t.Fatal("ContainsInstantiated() missed instantiation introduced by later recursive placeholder body")
	}
}

func TestContainsNever_OpenRecursiveProductSeesLaterBodyMutation(t *testing.T) {
	node := typ.NewRecursivePlaceholder("Node")
	wrapper := typ.NewTuple(typ.String, node)

	if ContainsNever(wrapper) {
		t.Fatal("ContainsNever() reported never before recursive placeholder body existed")
	}

	node.SetBody(typetable.NewRecord().Field("value", typ.Never).Build())

	if !ContainsNever(wrapper) {
		t.Fatal("ContainsNever() missed never introduced by later recursive placeholder body")
	}
}

func TestContainsReadonlyMapTraversesKeyAndValue(t *testing.T) {
	key := typ.NewTypeParam("K", nil)
	value := typ.NewTypeParam("V", nil)
	wrapper := typ.NewReadonlyMap(key, value)

	if !Contains(wrapper, func(node typ.Type) bool { return node == key }) {
		t.Fatal("Contains() missed ReadonlyMap key")
	}
	if !Contains(wrapper, func(node typ.Type) bool { return node == value }) {
		t.Fatal("Contains() missed ReadonlyMap value")
	}
}

func TestContainsReadonlyMapDynamicPredicatesTraverseKeyAndValue(t *testing.T) {
	cases := []struct {
		name     string
		contains func(typ.Type) bool
		marker   func() typ.Type
	}{
		{name: "any", contains: ContainsAny, marker: func() typ.Type { return typ.Any }},
		{name: "never", contains: ContainsNever, marker: func() typ.Type { return typ.Never }},
		{name: "type-param", contains: ContainsTypeParam, marker: func() typ.Type {
			return typ.NewTypeParam("T", nil)
		}},
		{name: "instantiated", contains: ContainsInstantiated, marker: traversalInstantiatedMarker},
	}
	positions := []struct {
		name string
		wrap func(*typ.Recursive) typ.Type
	}{
		{name: "key", wrap: func(node *typ.Recursive) typ.Type { return typ.NewReadonlyMap(node, typ.String) }},
		{name: "value", wrap: func(node *typ.Recursive) typ.Type { return typ.NewReadonlyMap(typ.String, node) }},
	}

	for _, tc := range cases {
		for _, pos := range positions {
			t.Run(tc.name+"/"+pos.name, func(t *testing.T) {
				node := typ.NewRecursivePlaceholder("Node")
				wrapper := pos.wrap(node)

				if tc.contains(wrapper) {
					t.Fatal("predicate reported marker before recursive placeholder body existed")
				}

				node.SetBody(typetable.NewRecord().Field("value", tc.marker()).Build())

				if !tc.contains(wrapper) {
					t.Fatal("predicate missed marker introduced through ReadonlyMap")
				}
			})
		}
	}
}

func TestContainsStaticMemberTraversesTypes(t *testing.T) {
	stringMember := typ.NewTypeParam("S", nil)
	intMember := typ.NewTypeParam("I", nil)
	wrapper := typetable.NewRecord().
		StaticStringIndex("name", stringMember).
		StaticIntIndex(1, intMember).
		Build()

	if !Contains(wrapper, func(node typ.Type) bool { return node == stringMember }) {
		t.Fatal("Contains() missed string static member type")
	}
	if !Contains(wrapper, func(node typ.Type) bool { return node == intMember }) {
		t.Fatal("Contains() missed integer static member type")
	}
}

func TestContainsStaticMemberDynamicPredicatesTraverseTypes(t *testing.T) {
	cases := []struct {
		name     string
		contains func(typ.Type) bool
		marker   func() typ.Type
	}{
		{name: "any", contains: ContainsAny, marker: func() typ.Type { return typ.Any }},
		{name: "never", contains: ContainsNever, marker: func() typ.Type { return typ.Never }},
		{name: "type-param", contains: ContainsTypeParam, marker: func() typ.Type {
			return typ.NewTypeParam("T", nil)
		}},
		{name: "instantiated", contains: ContainsInstantiated, marker: traversalInstantiatedMarker},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			node := typ.NewRecursivePlaceholder("Node")
			wrapper := typetable.NewRecord().
				StaticStringIndex("node", node).
				Build()

			if tc.contains(wrapper) {
				t.Fatal("predicate reported marker before recursive placeholder body existed")
			}

			node.SetBody(typetable.NewRecord().Field("value", tc.marker()).Build())

			if !tc.contains(wrapper) {
				t.Fatal("predicate missed marker introduced through static member")
			}
		})
	}
}

func traversalInstantiatedMarker() typ.Type {
	tp := typ.NewTypeParam("T", nil)
	box := typ.NewGeneric("Box", []*typ.TypeParam{tp}, typetable.NewRecord().Field("value", tp).Build())
	return typ.Instantiate(box, typ.Number)
}

type panickingIdentityType struct{}

func (panickingIdentityType) Kind() kind.Kind { return kind.Unknown }
func (panickingIdentityType) String() string  { return "panickingIdentityType" }
func (panickingIdentityType) Hash() uint64    { panic("Hash called") }
func (panickingIdentityType) Equals(typ.Type) bool {
	panic("Equals called")
}
