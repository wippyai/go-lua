package inspect

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/type/kind"
	typetable "github.com/wippyai/go-lua/analysis/domain/type/table"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/domain/type/typeexpr"
)

func testContainsAny(t typ.Type) bool {
	return containsMatching(t, typ.IsAny)
}

func testContainsNever(t typ.Type) bool {
	return containsMatching(t, typ.IsNever)
}

func testContainsTypeParam(t typ.Type) bool {
	return containsMatching(t, func(t typ.Type) bool {
		_, ok := t.(*typ.TypeParam)
		return ok
	})
}

func TestContainsAny_RecursiveTypeTerminates(t *testing.T) {
	node := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return typetable.NewRecord().
			Field("next", typeexpr.Optional(self)).
			Field("value", typ.String).
			Build()
	})

	if testContainsAny(node) {
		t.Fatal("testContainsAny() reported any in recursive type with only concrete fields")
	}
}

func TestContainsRecursive_UsesProductFlags(t *testing.T) {
	node := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return typetable.NewRecord().Field("next", typeexpr.Optional(self)).Build()
	})
	if !typ.ContainsRecursive(typeexpr.Union(typ.String, node)) {
		t.Fatal("typ.ContainsRecursive() missed recursive union member")
	}
	if typ.ContainsRecursive(typetable.NewRecord().Field("name", typ.String).Build()) {
		t.Fatal("typ.ContainsRecursive() reported recursion in concrete record")
	}
}

func TestContainsRecursive_DoesNotCallHashOrEquals(t *testing.T) {
	root := &typ.Tuple{Elements: []typ.Type{
		&typ.Record{Fields: []typ.Field{{Name: "value", Type: panickingIdentityType{}}}},
	}}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("typ.ContainsRecursive() called Hash or Equals: %v", r)
		}
	}()

	if typ.ContainsRecursive(root) {
		t.Fatal("typ.ContainsRecursive() reported recursion through non-recursive fake type")
	}
}

func TestIsMultiArmUnion(t *testing.T) {
	union := typeexpr.Union(typ.String, typ.Number)
	if !IsMultiArmUnion(union) {
		t.Fatal("IsMultiArmUnion missed direct two-arm union")
	}
	if !IsMultiArmUnion(typ.NewAlias("Choice", union)) {
		t.Fatal("IsMultiArmUnion missed alias to two-arm union")
	}
	recursive := typ.NewRecursive("Choice", func(typ.Type) typ.Type {
		return union
	})
	if !IsMultiArmUnion(recursive) {
		t.Fatal("IsMultiArmUnion missed recursive wrapper around two-arm union")
	}
	if IsMultiArmUnion(typeexpr.Union(typ.String)) {
		t.Fatal("IsMultiArmUnion reported single-arm union")
	}
	if IsMultiArmUnion(typ.String) {
		t.Fatal("IsMultiArmUnion reported non-union")
	}
}

func TestContainsAny_RecursiveTypeFindsNestedAny(t *testing.T) {
	node := typ.NewRecursive("Node", func(self typ.Type) typ.Type {
		return typetable.NewRecord().
			Field("next", typeexpr.Optional(self)).
			Field("value", typ.Any).
			Build()
	})

	if !testContainsAny(node) {
		t.Fatal("testContainsAny() missed any inside recursive type")
	}
}

func TestContainsAny_NestedFunctionType(t *testing.T) {
	fn := typ.Func().
		Returns(typ.Func().
			Param("x", typ.String).
			Returns(typ.Any).
			Build()).
		Build()

	if !testContainsAny(fn) {
		t.Fatal("testContainsAny() missed any inside nested function return")
	}
}

func TestContainsUnknown_NestedRecord(t *testing.T) {
	record := typetable.NewRecord().
		Field("value", typ.Unknown).
		Build()
	if !ContainsUnknown(record) {
		t.Fatal("ContainsUnknown() missed unknown inside record field")
	}
	if ContainsUnknown(typetable.NewRecord().Field("value", typ.String).Build()) {
		t.Fatal("ContainsUnknown() reported unknown in concrete record")
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
	containsMatching(root, func(node typ.Type) bool {
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
	containsMatching(root, func(node typ.Type) bool {
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

	if !containsMatching(root, func(node typ.Type) bool {
		_, ok := node.(*typ.TypeParam)
		return ok
	}) {
		t.Fatal("Contains() missed type parameter inside repeated structural shape")
	}
	if !testContainsTypeParam(root) {
		t.Fatal("testContainsTypeParam() missed type parameter inside repeated structural shape")
	}
}

// scanTraversalLawDepth is deeper than any depth budget the scan traversal
// has ever carried, so a reintroduced counter fails these laws instead of
// silently returning a capped answer.
const scanTraversalLawDepth = 12_288

func TestContains_NoDepthCapFindsDeepNestedPredicate(t *testing.T) {
	tpe := typ.Type(typ.Never)
	for i := 0; i < scanTraversalLawDepth; i++ {
		tpe = typetable.NewRecord().Field("next", tpe).Build()
	}

	if !containsMatching(tpe, typ.IsNever) {
		t.Fatal("Contains() missed a predicate reachable only below the traversal law depth")
	}
	if containsMatching(tpe, typ.IsAny) {
		t.Fatal("Contains() reported an absent predicate in a deep acyclic graph")
	}
}

func TestContains_CyclicGraphTerminatesOnSeenPolicyAlone(t *testing.T) {
	cyclic := &typ.Union{Members: make([]typ.Type, 2)}
	cyclic.Members[0] = cyclic
	cyclic.Members[1] = typ.Never

	if !containsMatching(cyclic, typ.IsNever) {
		t.Fatal("Contains() missed a predicate on a cyclic graph")
	}
	if containsMatching(cyclic, typ.IsAny) {
		t.Fatal("Contains() reported an absent predicate on a cyclic graph")
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
	containsMatching(root, func(node typ.Type) bool {
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

	if !testContainsAny(a) {
		t.Fatal("testContainsAny() missed any through recursive body assigned after wrapper construction")
	}
}

func TestContainsAny_TypeParamConstraint(t *testing.T) {
	tp := typ.NewTypeParam("T", typetable.NewRecord().Field("value", typ.Any).Build())
	fn := typ.Func().TypeParam("T", tp.Constraint).Returns(tp).Build()

	if !testContainsAny(fn) {
		t.Fatal("testContainsAny() missed any inside type parameter constraint")
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

	if !testContainsTypeParam(rec) {
		t.Fatal("testContainsTypeParam() missed type parameter inside recursive body")
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

	if testContainsAny(withoutAny) {
		t.Fatal("testContainsAny() reported any in concrete non-recursive product")
	}
	if !testContainsAny(withAny) {
		t.Fatal("testContainsAny() missed any recorded in non-recursive product flag")
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

	if typ.ContainsInstantiated(withoutInst) {
		t.Fatal("typ.ContainsInstantiated() reported instantiation in concrete product")
	}
	if !typ.ContainsInstantiated(withInst) {
		t.Fatal("typ.ContainsInstantiated() missed nested instantiation flag")
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

	if testContainsNever(withoutNever) {
		t.Fatal("testContainsNever() reported never in concrete non-recursive product")
	}
	if !testContainsNever(withNever) {
		t.Fatal("testContainsNever() missed never recorded in non-recursive product flag")
	}
}

func TestContainsInstantiated_SeesLaterAssignedRecursiveBody(t *testing.T) {
	tp := typ.NewTypeParam("T", nil)
	box := typ.NewGeneric("Box", []*typ.TypeParam{tp}, typetable.NewRecord().Field("value", tp).Build())
	a := typ.NewRecursivePlaceholder("A")
	b := typ.NewRecursivePlaceholder("B")

	a.SetBody(typetable.NewRecord().Field("b", b).Build())
	b.SetBody(typetable.NewRecord().Field("box", typ.Instantiate(box, typ.String)).Build())

	if !typ.ContainsInstantiated(a) {
		t.Fatal("typ.ContainsInstantiated() missed instantiation through later assigned recursive body")
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

	if testContainsAny(fn) {
		t.Fatal("testContainsAny() reported any in closed concrete recursive product")
	}
	if testContainsTypeParam(fn) {
		t.Fatal("testContainsTypeParam() reported type parameter in closed concrete recursive product")
	}
	if typ.ContainsInstantiated(fn) {
		t.Fatal("typ.ContainsInstantiated() reported instantiation in closed concrete recursive product")
	}
	if testContainsNever(fn) {
		t.Fatal("testContainsNever() reported never in closed concrete recursive product")
	}
}

func TestContainsAny_OpenRecursiveProductSeesLaterBodyMutation(t *testing.T) {
	node := typ.NewRecursivePlaceholder("Node")
	wrapper := typetable.NewRecord().
		Field("root", node).
		Field("label", typ.String).
		Build()

	if testContainsAny(wrapper) {
		t.Fatal("testContainsAny() reported any before recursive placeholder body existed")
	}

	node.SetBody(typetable.NewRecord().Field("value", typ.Any).Build())

	if !testContainsAny(wrapper) {
		t.Fatal("testContainsAny() missed any introduced by later recursive placeholder body")
	}
}

func TestContainsTypeParam_OpenRecursiveProductSeesLaterBodyMutation(t *testing.T) {
	node := typ.NewRecursivePlaceholder("Node")
	wrapper := typ.NewTuple(typ.String, node)

	if testContainsTypeParam(wrapper) {
		t.Fatal("testContainsTypeParam() reported type parameter before recursive placeholder body existed")
	}

	node.SetBody(typetable.NewRecord().Field("value", typ.NewTypeParam("T", nil)).Build())

	if !testContainsTypeParam(wrapper) {
		t.Fatal("testContainsTypeParam() missed type parameter introduced by later recursive placeholder body")
	}
}

func TestContainsInstantiated_OpenRecursiveProductSeesLaterBodyMutation(t *testing.T) {
	tp := typ.NewTypeParam("T", nil)
	box := typ.NewGeneric("Box", []*typ.TypeParam{tp}, typetable.NewRecord().Field("value", tp).Build())
	node := typ.NewRecursivePlaceholder("Node")
	wrapper := typ.NewMap(typ.String, node)

	if typ.ContainsInstantiated(wrapper) {
		t.Fatal("typ.ContainsInstantiated() reported instantiation before recursive placeholder body existed")
	}

	node.SetBody(typetable.NewRecord().Field("box", typ.Instantiate(box, typ.Number)).Build())

	if !typ.ContainsInstantiated(wrapper) {
		t.Fatal("typ.ContainsInstantiated() missed instantiation introduced by later recursive placeholder body")
	}
}

func TestContainsNever_OpenRecursiveProductSeesLaterBodyMutation(t *testing.T) {
	node := typ.NewRecursivePlaceholder("Node")
	wrapper := typ.NewTuple(typ.String, node)

	if testContainsNever(wrapper) {
		t.Fatal("testContainsNever() reported never before recursive placeholder body existed")
	}

	node.SetBody(typetable.NewRecord().Field("value", typ.Never).Build())

	if !testContainsNever(wrapper) {
		t.Fatal("testContainsNever() missed never introduced by later recursive placeholder body")
	}
}

func TestContainsReadonlyMapTraversesKeyAndValue(t *testing.T) {
	key := typ.NewTypeParam("K", nil)
	value := typ.NewTypeParam("V", nil)
	wrapper := typ.NewReadonlyMap(key, value)

	if !containsMatching(wrapper, func(node typ.Type) bool { return node == key }) {
		t.Fatal("Contains() missed ReadonlyMap key")
	}
	if !containsMatching(wrapper, func(node typ.Type) bool { return node == value }) {
		t.Fatal("Contains() missed ReadonlyMap value")
	}
}

func TestContainsReadonlyMapDynamicPredicatesTraverseKeyAndValue(t *testing.T) {
	cases := []struct {
		name     string
		contains func(typ.Type) bool
		marker   func() typ.Type
	}{
		{name: "any", contains: testContainsAny, marker: func() typ.Type { return typ.Any }},
		{name: "never", contains: testContainsNever, marker: func() typ.Type { return typ.Never }},
		{name: "type-param", contains: testContainsTypeParam, marker: func() typ.Type {
			return typ.NewTypeParam("T", nil)
		}},
		{name: "instantiated", contains: typ.ContainsInstantiated, marker: traversalInstantiatedMarker},
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

	if !containsMatching(wrapper, func(node typ.Type) bool { return node == stringMember }) {
		t.Fatal("Contains() missed string static member type")
	}
	if !containsMatching(wrapper, func(node typ.Type) bool { return node == intMember }) {
		t.Fatal("Contains() missed integer static member type")
	}
}

func TestContainsStaticMemberDynamicPredicatesTraverseTypes(t *testing.T) {
	cases := []struct {
		name     string
		contains func(typ.Type) bool
		marker   func() typ.Type
	}{
		{name: "any", contains: testContainsAny, marker: func() typ.Type { return typ.Any }},
		{name: "never", contains: testContainsNever, marker: func() typ.Type { return typ.Never }},
		{name: "type-param", contains: testContainsTypeParam, marker: func() typ.Type {
			return typ.NewTypeParam("T", nil)
		}},
		{name: "instantiated", contains: typ.ContainsInstantiated, marker: traversalInstantiatedMarker},
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
