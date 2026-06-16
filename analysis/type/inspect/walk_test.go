package inspect

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestWalkChildrenCanonicalOrder(t *testing.T) {
	leaf := func(name string) typ.Type {
		return typ.NewRef("", name)
	}

	left := leaf("left")
	right := leaf("right")
	variadic := leaf("variadic")
	meta := leaf("meta")
	fn := typ.Func().Param("x", left).Variadic(variadic).Returns(right).Build()
	record := &typ.Record{
		Fields: []typ.Field{{Name: "a", Type: left}},
		StaticMembers: []typ.StaticMember{{
			Kind: typ.StaticMemberStringIndex,
			Name: "m",
			Type: right,
		}},
		Metatable: meta,
	}

	type testcase struct {
		name string
		node typ.Type
		want []string
	}

	cases := []testcase{
		{name: "Function", node: fn, want: []string{"left", "variadic", "right"}},
		{name: "Record", node: record, want: []string{"left", "right", "meta"}},
	}

	label := func(child typ.Type) string {
		switch child {
		case left:
			return "left"
		case right:
			return "right"
		case variadic:
			return "variadic"
		case meta:
			return "meta"
		default:
			t.Fatalf("unexpected child %T %v", child, child)
			return ""
		}
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotInspect := make([]string, 0, len(tc.want))
			WalkChildren(tc.node, func(child typ.Type) bool {
				gotInspect = append(gotInspect, label(child))
				return false
			})

			if !reflect.DeepEqual(gotInspect, tc.want) {
				t.Fatalf("inspect.WalkChildren order = %v, want %v", gotInspect, tc.want)
			}
		})
	}
}
