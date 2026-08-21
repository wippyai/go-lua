package manifesttarget

import (
	"strconv"
	"testing"

	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/domain/effect"
	"github.com/wippyai/go-lua/domain/effect/ownership"
)

func TestFormalEffectsProjectionCoversEveryOwnershipKind(t *testing.T) {
	row, err := formalEffects(effect.Row{Labels: []effect.Label{
		ownership.Freeze{Param: effect.ParamRef{Index: -1}},
		ownership.Opaque{Param: effect.ParamRef{Index: 7}},
		ownership.Export{Param: effect.ParamRef{Index: 6}},
		ownership.SendParam{Param: effect.ParamRef{Index: -1}},
		ownership.Send{FromParam: 5},
		ownership.BorrowAll{},
		ownership.Store{Param: effect.ParamRef{Index: -1}, Into: effect.ParamRef{Index: -2}},
		ownership.Retain{Param: effect.ParamRef{Index: 4}},
		ownership.Borrow{Param: effect.ParamRef{Index: -1}},
	}})
	if err != nil {
		t.Fatalf("project ownership row: %v", err)
	}
	if row.Tail != vocabulary.RowClosed {
		t.Fatalf("closed ownership row tail = %d, want closed", row.Tail)
	}
	want := []vocabulary.FormalEffectSpec{
		{Kind: vocabulary.FormalEffectFreeze, Param: -1},
		{Kind: vocabulary.FormalEffectOpaque, Param: 7},
		{Kind: vocabulary.FormalEffectExport, Param: 6},
		{Kind: vocabulary.FormalEffectSendParam, Param: -1},
		{Kind: vocabulary.FormalEffectSendSuffix, FromParam: 5},
		{Kind: vocabulary.FormalEffectBorrowAll},
		{Kind: vocabulary.FormalEffectStore, Param: -1, Into: -2},
		{Kind: vocabulary.FormalEffectRetain, Param: 4},
		{Kind: vocabulary.FormalEffectBorrow, Param: -1},
	}
	if len(row.Occurrences) != len(want) {
		t.Fatalf("projected ownership count = %d, want %d", len(row.Occurrences), len(want))
	}
	for index := range want {
		if row.Occurrences[index] != want[index] {
			t.Fatalf("projected ownership %d = %#v, want %#v", index, row.Occurrences[index], want[index])
		}
	}
	if row.Occurrences[6].HasInto {
		t.Fatal("negative Store Into was marked present")
	}

	open, err := formalEffects(effect.Row{Tail: &effect.Var{Name: "known"}})
	if err != nil {
		t.Fatalf("project open ownership row: %v", err)
	}
	if open.Tail != vocabulary.RowUnknownOpen {
		t.Fatalf("open ownership row tail = %d, want unknown-open", open.Tail)
	}
}

func TestFormalEffectsProjectionIgnoresKnownNonOwnershipLabels(t *testing.T) {
	row, err := formalEffects(effect.Empty.With(ownership.Borrow{Param: effect.ParamRef{Index: 0}}))
	if err != nil {
		t.Fatalf("project ownership row: %v", err)
	}
	if len(row.Occurrences) != 1 || row.Occurrences[0].Kind != vocabulary.FormalEffectBorrow {
		t.Fatalf("ownership projection = %#v, want one Borrow row", row.Occurrences)
	}
	row, err = formalEffects(effect.Empty.With(testFormalNonOwnershipLabel{}))
	if err != nil {
		t.Fatalf("project non-ownership row: %v", err)
	}
	if len(row.Occurrences) != 0 || row.Tail != vocabulary.RowClosed {
		t.Fatalf("non-ownership projection = %#v, want empty closed row", row)
	}
}

func TestFormalEffectsRejectsSignedCoordinateOverflow(t *testing.T) {
	if strconv.IntSize <= 32 {
		t.Skip("Go int cannot represent a value outside signed int32 range")
	}
	values := []int{int(^uint32(0)>>1) + 1, -int(^uint32(0)>>1) - 2}
	for _, value := range values {
		if _, err := formalEffects(effect.Empty.With(ownership.Borrow{Param: effect.ParamRef{Index: value}})); err == nil {
			t.Fatalf("formal parameter %d crossed the int32 boundary", value)
		}
	}
	for _, value := range values {
		if _, err := formalEffects(effect.Empty.With(ownership.Send{FromParam: value})); err == nil {
			t.Fatalf("formal ordinal %d crossed the int32 boundary", value)
		}
	}
}

func TestFormalEffectsRejectsOverflowForEverySignedCoordinate(t *testing.T) {
	if strconv.IntSize <= 32 {
		t.Skip("Go int cannot represent a value outside signed int32 range")
	}

	values := []int{int(^uint32(0)>>1) + 1, -int(^uint32(0)>>1) - 2}
	cases := []struct {
		name  string
		label func(int) effect.Label
	}{
		{name: "borrow parameter", label: func(value int) effect.Label {
			return ownership.Borrow{Param: effect.ParamRef{Index: value}}
		}},
		{name: "retain parameter", label: func(value int) effect.Label {
			return ownership.Retain{Param: effect.ParamRef{Index: value}}
		}},
		{name: "store parameter", label: func(value int) effect.Label {
			return ownership.Store{Param: effect.ParamRef{Index: value}, Into: effect.ParamRef{Index: -1}}
		}},
		{name: "store destination", label: func(value int) effect.Label {
			return ownership.Store{Param: effect.ParamRef{Index: 0}, Into: effect.ParamRef{Index: value}}
		}},
		{name: "send suffix boundary", label: func(value int) effect.Label {
			return ownership.Send{FromParam: value}
		}},
		{name: "send parameter", label: func(value int) effect.Label {
			return ownership.SendParam{Param: effect.ParamRef{Index: value}}
		}},
		{name: "export parameter", label: func(value int) effect.Label {
			return ownership.Export{Param: effect.ParamRef{Index: value}}
		}},
		{name: "opaque parameter", label: func(value int) effect.Label {
			return ownership.Opaque{Param: effect.ParamRef{Index: value}}
		}},
		{name: "freeze parameter", label: func(value int) effect.Label {
			return ownership.Freeze{Param: effect.ParamRef{Index: value}}
		}},
	}
	for _, test := range cases {
		for _, value := range values {
			if _, err := formalEffects(effect.Empty.With(test.label(value))); err == nil {
				t.Errorf("%s %d crossed the int32 boundary", test.name, value)
			}
		}
	}
}

func TestFormalEffectsRetainsSignedInt32Boundary(t *testing.T) {
	row, err := formalEffects(effect.Row{Labels: []effect.Label{
		ownership.Borrow{Param: effect.ParamRef{Index: -1}},
		ownership.Send{FromParam: int(^uint32(0) >> 1)},
	}})
	if err != nil {
		t.Fatalf("project signed int32 boundary: %v", err)
	}
	if len(row.Occurrences) != 2 || row.Occurrences[0].Param != -1 || row.Occurrences[1].FromParam != int32(^uint32(0)>>1) {
		t.Fatalf("signed int32 boundary projection = %#v", row.Occurrences)
	}
}

type testFormalNonOwnershipLabel struct{}

func (testFormalNonOwnershipLabel) CapabilityID() string { return "test.formal.nonownership" }
func (testFormalNonOwnershipLabel) String() string       { return "test.formal.nonownership" }
func (testFormalNonOwnershipLabel) Equals(other effect.Label) bool {
	_, ok := effect.NormalizeLabel(other).(testFormalNonOwnershipLabel)
	return ok
}
