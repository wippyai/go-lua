package returns

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/effect"
)

func TestReturnEffect(t *testing.T) {
	r := Returns(0, ElementOf{Source: effect.ParamRef{Index: 0}})

	got := GetReturn(r, 0)
	if got == nil {
		t.Error("Should find return for index 0")
	}

	if GetReturn(r, 1) != nil {
		t.Error("Should not find return for index 1")
	}
}

func TestReturnLengthEffect(t *testing.T) {
	r := effect.Row{Labels: []effect.Label{ReturnLength{ReturnIndex: 0, Length: nil}}}

	rl := GetReturnLength(r, 0)
	if rl == nil {
		t.Error("Should find return length")
	}

	if GetReturnLength(r, 1) != nil {
		t.Error("Should not find return length for wrong index")
	}
}

func TestErrorReturnEffect(t *testing.T) {
	r := effect.Row{Labels: []effect.Label{ErrorReturn{ValueIndex: 0, ErrorIndex: 1}}}

	er := GetErrorReturn(r, 0)
	if er == nil {
		t.Error("Should find error return")
	}

	if GetErrorReturn(r, 1) != nil {
		t.Error("Should not find error return for wrong value index")
	}
}

func TestNewRow(t *testing.T) {
	r := effect.Row{Labels: []effect.Label{
		Return{ReturnIndex: 0, Transform: ElementOf{Source: effect.ParamRef{Index: 0}}},
		ErrorReturn{ValueIndex: 0, ErrorIndex: 1},
	}}
	if GetReturn(r, 0) == nil || GetErrorReturn(r, 0) == nil {
		t.Error("Row should have the given labels")
	}
}

func TestGetCorrelatedReturn(t *testing.T) {
	r := effect.Row{Labels: []effect.Label{CorrelatedReturn{Indices: []int{0, 1, 2}}}}

	cr := GetCorrelatedReturn(r, 1)
	if cr == nil {
		t.Error("Should find correlated return for index 1")
	}

	if GetCorrelatedReturn(r, 5) != nil {
		t.Error("Should not find correlated return for index 5")
	}

	if GetCorrelatedReturn(effect.Empty, 0) != nil {
		t.Error("Empty row should not have correlated return")
	}
}
