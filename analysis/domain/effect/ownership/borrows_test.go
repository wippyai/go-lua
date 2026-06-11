package ownership

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/mutation"
)

func TestOnlyBorrows(t *testing.T) {
	if !OnlyBorrows(WithBorrow(0)) {
		t.Error("WithBorrow should only borrow")
	}

	if !OnlyBorrows(BorrowsOnly()) {
		t.Error("BorrowsOnly should only borrow")
	}

	if OnlyBorrows(WithStore(0, 1)) {
		t.Error("WithStore should not only borrow")
	}

	if OnlyBorrows(effect.Empty) {
		t.Error("Empty should not only borrow")
	}

	lengthOnly := effect.Row{Labels: []effect.Label{
		Borrow{Param: effect.ParamRef{Index: 0}},
		mutation.LengthChange{Target: effect.ParamRef{Index: 0}, Delta: 1},
	}}
	if !OnlyBorrows(lengthOnly) {
		t.Error("Borrow+LengthChange should only borrow")
	}

	r := effect.Row{Labels: []effect.Label{
		Borrow{Param: effect.ParamRef{Index: 0}},
		mutation.Mutate{Target: effect.ParamRef{Index: 0}},
	}}
	if OnlyBorrows(r) {
		t.Error("Borrow+Mutate should not be only borrows")
	}
}
