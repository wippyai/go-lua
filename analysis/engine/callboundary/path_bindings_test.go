package callboundary

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
)

func TestPathBindingsSubstitutesParamsAndReturnSlots(t *testing.T) {
	arg := pathdom.Path{Root: "$0"}.Field("id")
	ret := pathdom.Path{Root: "ret[1]"}.Field("ok")
	bindings := NewPathBindings(
		[]pathdom.Path{pathdom.NewPath(10, "").Field("input")},
		[]pathdom.Path{
			pathdom.NewPath(20, ""),
			pathdom.NewPath(30, "").Field("result"),
		},
	)

	gotArg, ok := bindings.Substitute(arg)
	if !ok || gotArg.Key() != pathdom.NewPath(10, "").Field("input").Field("id").Key() {
		t.Fatalf("Substitute($0.id) = %s/%v", gotArg.Key(), ok)
	}
	gotRet, ok := bindings.Substitute(ret)
	if !ok || gotRet.Key() != pathdom.NewPath(30, "").Field("result").Field("ok").Key() {
		t.Fatalf("Substitute(ret[1].ok) = %s/%v", gotRet.Key(), ok)
	}
}

func TestReturnSlotIndexRequiresCanonicalRoot(t *testing.T) {
	for _, tc := range []struct {
		name string
		path pathdom.Path
		want int
		ok   bool
	}{
		{name: "canonical", path: pathdom.Path{Root: "ret[12]"}.Field("value"), want: 12, ok: true},
		{name: "negative", path: pathdom.Path{Root: "ret[-1]"}, ok: false},
		{name: "leading zero", path: pathdom.Path{Root: "ret[01]"}, ok: false},
		{name: "symbol root", path: pathdom.NewPath(1, ""), ok: false},
		{name: "placeholder", path: pathdom.Path{Root: "$0"}, ok: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ReturnSlotIndex(tc.path)
			if got != tc.want || ok != tc.ok {
				t.Fatalf("ReturnSlotIndex(%s) = %d/%v, want %d/%v", tc.path.Key(), got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestPathRootedInReturnSlots(t *testing.T) {
	slots := map[int]struct{}{2: {}}
	if !PathRootedInReturnSlots(pathdom.Path{Root: "ret[2]"}.Field("value"), slots) {
		t.Fatalf("ret[2].value should be rooted in selected slots")
	}
	if PathRootedInReturnSlots(pathdom.Path{Root: "ret[3]"}.Field("value"), slots) {
		t.Fatalf("ret[3].value should not be rooted in selected slots")
	}
}
