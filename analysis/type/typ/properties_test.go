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
	}{
		{name: "alias", node: NewAlias("A", Any)},
		{name: "meta", node: NewMeta(typeParam)},
		{name: "annotated", node: NewAnnotated(Any, ann).(*Annotated)},
		{name: "optional", node: MaterializeOptional(typeParam).(*Optional)},
		{name: "union", node: MaterializeUnion([]Type{String, typeParam}).(*Union)},
		{name: "intersection", node: MaterializeIntersection([]Type{String, Any}).(*Intersection)},
		{name: "array", node: NewArray(Any)},
		{name: "map", node: RebuildMap(String, Any)},
		{name: "readonly map", node: RebuildReadonlyMap(typeParam, String)},
		{name: "tuple", node: NewTuple(String, Any, typeParam)},
		{name: "interface", node: NewInterface("Reader", []Method{{Name: "read", Type: Func().Returns(Any).Build()}})},
		{name: "type param", node: NewTypeParam("U", String)},
		{name: "instantiated", node: Instantiate(NewGeneric("Box", []*TypeParam{typeParam}, NewArray(typeParam)), Any)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !tc.node.hasAnyOrTypeParam() {
				t.Fatalf("%s properties were not promoted from child types", tc.name)
			}
		})
	}
}

type propertyCarrier interface {
	hasAnyOrTypeParam() bool
}

func (p typeProperties) hasAnyOrTypeParam() bool {
	return p.containsAny || p.containsTypeParam
}
