package typ

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/annotation"
)

func TestTypePropertiesPromotedThroughSimpleCompositeNodes(t *testing.T) {
	typeParam := NewTypeParam("T", String)
	ann := []annotation.Annotation{{Name: "min"}}
	cases := []struct {
		name string
		node interface {
			Type
			propertyCarrier
		}
		want typeProperties
	}{
		{name: "alias", node: NewAlias("A", Any), want: typeProperties{containsAny: true}},
		{name: "meta", node: NewMeta(typeParam), want: typeProperties{containsTypeParam: true}},
		{name: "annotated", node: NewAnnotated(Any, ann).(*Annotated), want: typeProperties{containsAny: true}},
		{name: "optional", node: MaterializeOptional(typeParam).(*Optional), want: typeProperties{containsTypeParam: true}},
		{name: "union", node: MaterializeUnion([]Type{String, typeParam}).(*Union), want: typeProperties{containsTypeParam: true}},
		{name: "intersection", node: MaterializeIntersection([]Type{String, Any}).(*Intersection), want: typeProperties{containsAny: true}},
		{name: "array", node: NewArray(Any), want: typeProperties{containsAny: true}},
		{name: "map", node: RebuildMap(String, Any), want: typeProperties{containsAny: true}},
		{name: "readonly map", node: RebuildReadonlyMap(typeParam, String), want: typeProperties{containsTypeParam: true}},
		{name: "tuple", node: NewTuple(String, Any, typeParam), want: typeProperties{containsAny: true, containsTypeParam: true}},
		{name: "interface", node: NewInterface("Reader", []Method{{Name: "read", Type: Func().Returns(Any).Build()}}), want: typeProperties{containsAny: true}},
		{name: "type param", node: NewTypeParam("U", String), want: typeProperties{containsTypeParam: true}},
		{name: "instantiated", node: Instantiate(NewGeneric("Box", []*TypeParam{typeParam}, NewArray(typeParam)), Any), want: typeProperties{containsAny: true, containsTypeParam: true, containsInstantiated: true, containsGeneric: true}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.node.propertyValues()
			if got.containsAny != tc.want.containsAny ||
				got.containsNever != tc.want.containsNever ||
				got.containsTypeParam != tc.want.containsTypeParam ||
				got.containsInstantiated != tc.want.containsInstantiated ||
				got.containsGeneric != tc.want.containsGeneric ||
				got.containsRecursive != tc.want.containsRecursive ||
				got.containsOpenRecursive != tc.want.containsOpenRecursive {
				t.Fatalf("promoted properties = %#v, want %#v", got, tc.want)
			}
		})
	}
}

type propertyCarrier interface {
	propertyValues() typeProperties
}

func (p typeProperties) propertyValues() typeProperties {
	return p
}
