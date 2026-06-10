package ownership

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/returns"
)

func TestBorrow(t *testing.T) {
	b := Borrow{Param: effect.ParamRef{Index: 0}}
	if got := b.String(); got != "borrow(param[0])" {
		t.Errorf("Borrow.String() = %q", got)
	}

	if !b.Equals(Borrow{Param: effect.ParamRef{Index: 0}}) {
		t.Error("same Borrow should be equal")
	}

	if b.Equals(Borrow{Param: effect.ParamRef{Index: 1}}) {
		t.Error("different param Borrow should not be equal")
	}

	if b.Equals(BorrowAll{}) {
		t.Error("Borrow should not equal BorrowAll")
	}
}

func TestStore(t *testing.T) {
	s := Store{Param: effect.ParamRef{Index: 0}, Into: effect.ParamRef{Index: 1}}
	if got := s.String(); got != "store(param[0] into param[1])" {
		t.Errorf("Store with into.String() = %q", got)
	}

	sUnknown := Store{Param: effect.ParamRef{Index: 0}, Into: effect.ParamRef{Index: -1}}
	if got := sUnknown.String(); got != "store(param[0])" {
		t.Errorf("Store unknown into.String() = %q", got)
	}

	if !s.Equals(Store{Param: effect.ParamRef{Index: 0}, Into: effect.ParamRef{Index: 1}}) {
		t.Error("same Store should be equal")
	}

	if s.Equals(Store{Param: effect.ParamRef{Index: 1}, Into: effect.ParamRef{Index: 1}}) {
		t.Error("different param Store should not be equal")
	}

	if s.Equals(BorrowAll{}) {
		t.Error("Store should not equal BorrowAll")
	}

	s2 := Store{Param: effect.ParamRef{Index: 0}, Into: effect.ParamRef{Index: 2}}
	if s.Equals(s2) {
		t.Error("different Into Store should not be equal")
	}
	if s.Equals(sUnknown) {
		t.Error("different Into (known vs unknown) Store should not be equal")
	}
}

func TestBorrowAll(t *testing.T) {
	ba := BorrowAll{}
	if got := ba.String(); got != "borrow_all" {
		t.Errorf("BorrowAll.String() = %q", got)
	}

	if !ba.Equals(BorrowAll{}) {
		t.Error("BorrowAll should equal BorrowAll")
	}

	if ba.Equals(returns.Return{}) {
		t.Error("BorrowAll should not equal Return")
	}
}

func TestSend(t *testing.T) {
	s := Send{FromParam: 2}
	if got := s.String(); got != "send(params[2:])" {
		t.Errorf("Send.String() = %q", got)
	}

	if !s.Equals(Send{FromParam: 2}) {
		t.Error("same Send should be equal")
	}

	if s.Equals(Send{FromParam: 3}) {
		t.Error("different FromParam should not be equal")
	}

	if s.Equals(BorrowAll{}) {
		t.Error("Send should not equal BorrowAll")
	}
}

func TestFreeze(t *testing.T) {
	f := Freeze{Param: effect.ParamRef{Index: 0}}
	if got := f.String(); got != "freeze(param[0])" {
		t.Errorf("Freeze.String() = %q", got)
	}

	if !f.Equals(Freeze{Param: effect.ParamRef{Index: 0}}) {
		t.Error("same Freeze should be equal")
	}

	if f.Equals(Freeze{Param: effect.ParamRef{Index: 1}}) {
		t.Error("different Param should not be equal")
	}

	if f.Equals(BorrowAll{}) {
		t.Error("Freeze should not equal BorrowAll")
	}
}

func TestAllLabelsImplementInterface(t *testing.T) {
	labels := []effect.Label{
		Borrow{},
		Store{},
		BorrowAll{},
		Send{},
		Freeze{},
	}

	for _, l := range labels {
		_ = l.String()
		_ = l.Equals(l)
	}
}

func TestMarkerMethods(t *testing.T) {
	Borrow{}.EffectLabel()
	Store{}.EffectLabel()
	BorrowAll{}.EffectLabel()
	Send{}.EffectLabel()
	Freeze{}.EffectLabel()
}

func TestSelectors(t *testing.T) {
	r := BorrowsOnly()
	if !HasBorrow(r) {
		t.Error("BorrowsOnly should have borrow")
	}

	if !BorrowsAllParams(r) {
		t.Error("BorrowsOnly should borrow all params")
	}

	r2 := WithBorrow(0)
	if !HasBorrow(r2) {
		t.Error("WithBorrow should have borrow")
	}

	b := GetBorrow(r2, 0)
	if b == nil {
		t.Error("Should find borrow for param 0")
	}

	if GetBorrow(r2, 1) != nil {
		t.Error("Should not find borrow for param 1")
	}

	r3 := WithStore(0, 1)
	if !HasStore(r3) {
		t.Error("WithStore should have store")
	}

	s := GetStore(r3, 0)
	if s == nil {
		t.Error("Should find store for param 0")
	}

	if GetStore(r3, 1) != nil {
		t.Error("Should not find store for param 1")
	}

	if !HasSend(WithSend(2)) {
		t.Error("WithSend should have send")
	}

	if !HasFreeze(WithFreeze(0)) {
		t.Error("WithFreeze should have freeze")
	}
}
