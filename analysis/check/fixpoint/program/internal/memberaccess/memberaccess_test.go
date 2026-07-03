package memberaccess

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typecall"
)

func TestValidAcceptsOnlyExactSummaryMembers(t *testing.T) {
	cases := []struct {
		name string
		seg  segment.Segment
		want bool
	}{
		{name: "field", seg: segment.Segment{Kind: segment.SegmentField, Name: "send"}, want: true},
		{name: "string index", seg: segment.Segment{Kind: segment.SegmentIndexString, Name: "send"}, want: true},
		{name: "int index", seg: segment.Segment{Kind: segment.SegmentIndexInt, Index: 1}, want: true},
		{name: "empty field", seg: segment.Segment{Kind: segment.SegmentField}, want: false},
		{name: "empty string index", seg: segment.Segment{Kind: segment.SegmentIndexString}, want: false},
		{name: "unknown kind", seg: segment.Segment{Kind: segment.SegmentKind(99), Name: "send"}, want: false},
	}

	for _, tc := range cases {
		if got := Valid(tc.seg); got != tc.want {
			t.Fatalf("%s: Valid(%#v) = %v, want %v", tc.name, tc.seg, got, tc.want)
		}
	}
}

func TestPathsPreserveFieldStringIndexAndIntIndexMemberShapes(t *testing.T) {
	receiver := pathdom.NewPath(7, "provider")

	assertPaths(t, "field", Paths(receiver, segment.Segment{Kind: segment.SegmentField, Name: "send"}),
		receiver.Field("send"),
		receiver.IndexStr("send"),
	)
	assertPaths(t, "string index", Paths(receiver, segment.Segment{Kind: segment.SegmentIndexString, Name: "send"}),
		receiver.Field("send"),
		receiver.IndexStr("send"),
	)
	assertPaths(t, "int index", Paths(receiver, segment.Segment{Kind: segment.SegmentIndexInt, Index: 1}),
		receiver.IndexInt(1),
	)
	if got := Paths(pathdom.Path{}, segment.Segment{Kind: segment.SegmentField, Name: "send"}); got != nil {
		t.Fatalf("Paths(empty receiver) = %#v, want nil", got)
	}
	if got := Paths(receiver, segment.Segment{Kind: segment.SegmentField}); got != nil {
		t.Fatalf("Paths(invalid member) = %#v, want nil", got)
	}
}

func TestCallableResolvesFieldStringIndexAndIntIndexMembers(t *testing.T) {
	method := typ.Func().
		Param("self", typ.Self).
		Param("payload", typ.String).
		Returns(typ.Self).
		Build()
	intMethod := typ.Func().Returns(typ.Number).Build()
	receiver := typetable.NewRecord().
		Field("send", method).
		StaticStringIndex("decode", method).
		StaticIntIndex(1, intMethod).
		StaticIntIndex(2, typ.Number).
		Build()

	assertCallable(t, receiver, segment.Segment{Kind: segment.SegmentField, Name: "send"}, receiver, typ.String, receiver)
	assertCallable(t, receiver, segment.Segment{Kind: segment.SegmentIndexString, Name: "decode"}, receiver, typ.String, receiver)

	got, status, ok := Callable(receiver, segment.Segment{Kind: segment.SegmentIndexInt, Index: 1})
	if status != typecall.MemberCallOK || !ok {
		t.Fatalf("Callable([1]) = status %v ok %v, want ok", status, ok)
	}
	if len(got.Returns) != 1 || !typ.TypeEquals(got.Returns[0], typ.Number) {
		t.Fatalf("Callable([1]) returns = %#v, want number", got.Returns)
	}
	if _, status, ok := Callable(receiver, segment.Segment{Kind: segment.SegmentIndexInt, Index: 2}); status != typecall.MemberCallNotCallable || ok {
		t.Fatalf("Callable(non-callable [2]) = status %v ok %v, want not-callable false", status, ok)
	}
	if _, status, ok := Callable(receiver, segment.Segment{Kind: segment.SegmentIndexString, Name: "missing"}); status != typecall.MemberCallMissing || ok {
		t.Fatalf("Callable(missing string index) = status %v ok %v, want missing false", status, ok)
	}
}

func assertPaths(t *testing.T, name string, got []pathdom.Path, want ...pathdom.Path) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: got %d paths %#v, want %d %#v", name, len(got), got, len(want), want)
	}
	for i := range want {
		if !got[i].Equal(want[i]) {
			t.Fatalf("%s: path[%d] = %#v, want %#v", name, i, got[i], want[i])
		}
	}
}

func assertCallable(t *testing.T, receiver typ.Type, member segment.Segment, wantSelf, wantPayload, wantReturn typ.Type) {
	t.Helper()
	got, status, ok := Callable(receiver, member)
	if status != typecall.MemberCallOK || !ok {
		t.Fatalf("Callable(%#v) = status %v ok %v, want ok", member, status, ok)
	}
	if len(got.Params) != 2 {
		t.Fatalf("Callable(%#v) params = %#v, want self and payload", member, got.Params)
	}
	if !typ.TypeEquals(got.Params[0].Type, wantSelf) {
		t.Fatalf("Callable(%#v) self param = %v, want %v", member, got.Params[0].Type, wantSelf)
	}
	if !typ.TypeEquals(got.Params[1].Type, wantPayload) {
		t.Fatalf("Callable(%#v) payload param = %v, want %v", member, got.Params[1].Type, wantPayload)
	}
	if len(got.Returns) != 1 || !typ.TypeEquals(got.Returns[0], wantReturn) {
		t.Fatalf("Callable(%#v) returns = %#v, want %v", member, got.Returns, wantReturn)
	}
}
