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

func TestRetain(t *testing.T) {
	r := Retain{Param: effect.ParamRef{Index: 0}}
	if got := r.String(); got != "retain(param[0])" {
		t.Errorf("Retain.String() = %q", got)
	}

	if !r.Equals(Retain{Param: effect.ParamRef{Index: 0}}) {
		t.Error("same Retain should be equal")
	}

	if r.Equals(Retain{Param: effect.ParamRef{Index: 1}}) {
		t.Error("different param Retain should not be equal")
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

func TestSendParam(t *testing.T) {
	s := SendParam{Param: effect.ParamRef{Index: 2}}
	if got := s.String(); got != "send(param[2])" {
		t.Errorf("SendParam.String() = %q", got)
	}

	if !s.Equals(SendParam{Param: effect.ParamRef{Index: 2}}) {
		t.Error("same SendParam should be equal")
	}

	if s.Equals(SendParam{Param: effect.ParamRef{Index: 3}}) {
		t.Error("different Param SendParam should not be equal")
	}

	if s.Equals(Send{FromParam: 2}) {
		t.Error("SendParam should not equal suffix Send")
	}
}

func TestExport(t *testing.T) {
	e := Export{Param: effect.ParamRef{Index: 2}}
	if got := e.String(); got != "export(param[2])" {
		t.Errorf("Export.String() = %q", got)
	}

	if !e.Equals(Export{Param: effect.ParamRef{Index: 2}}) {
		t.Error("same Export should be equal")
	}

	if e.Equals(Export{Param: effect.ParamRef{Index: 3}}) {
		t.Error("different Param Export should not be equal")
	}
}

func TestOpaque(t *testing.T) {
	o := Opaque{Param: effect.ParamRef{Index: 2}}
	if got := o.String(); got != "opaque(param[2])" {
		t.Errorf("Opaque.String() = %q", got)
	}

	if !o.Equals(Opaque{Param: effect.ParamRef{Index: 2}}) {
		t.Error("same Opaque should be equal")
	}

	if o.Equals(Opaque{Param: effect.ParamRef{Index: 3}}) {
		t.Error("different Param Opaque should not be equal")
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
		Retain{},
		Store{},
		BorrowAll{},
		Send{},
		SendParam{},
		Export{},
		Opaque{},
		Freeze{},
	}

	for _, l := range labels {
		_ = l.String()
		_ = l.Equals(l)
	}
}

func TestMarkerMethods(t *testing.T) {
	Borrow{}.EffectLabel()
	Retain{}.EffectLabel()
	Store{}.EffectLabel()
	BorrowAll{}.EffectLabel()
	Send{}.EffectLabel()
	SendParam{}.EffectLabel()
	Export{}.EffectLabel()
	Opaque{}.EffectLabel()
	Freeze{}.EffectLabel()
}

func TestRowNormalization(t *testing.T) {
	r := effect.Row{Labels: []effect.Label{
		Borrow{Param: effect.ParamRef{Index: 0}},
		Retain{Param: effect.ParamRef{Index: 4}},
		Store{Param: effect.ParamRef{Index: 0}, Into: effect.ParamRef{Index: 1}},
		Send{FromParam: 2},
		SendParam{Param: effect.ParamRef{Index: 1}},
		Export{Param: effect.ParamRef{Index: 5}},
		Opaque{Param: effect.ParamRef{Index: 6}},
		Freeze{Param: effect.ParamRef{Index: 3}},
		BorrowAll{},
	}}

	borrow, ok := effect.NormalizeLabel(r.Labels[0]).(Borrow)
	if !ok || borrow.Param.Index != 0 {
		t.Fatal("Should normalize borrow label")
	}
	retain, ok := effect.NormalizeLabel(r.Labels[1]).(Retain)
	if !ok || retain.Param.Index != 4 {
		t.Fatal("Should normalize retain label")
	}
	store, ok := effect.NormalizeLabel(r.Labels[2]).(Store)
	if !ok || store.Param.Index != 0 || store.Into.Index != 1 {
		t.Fatal("Should normalize store label")
	}
	send, ok := effect.NormalizeLabel(r.Labels[3]).(Send)
	if !ok || send.FromParam != 2 {
		t.Fatal("Should normalize send label")
	}
	sendParam, ok := effect.NormalizeLabel(r.Labels[4]).(SendParam)
	if !ok || sendParam.Param.Index != 1 {
		t.Fatal("Should normalize send_param label")
	}
	export, ok := effect.NormalizeLabel(r.Labels[5]).(Export)
	if !ok || export.Param.Index != 5 {
		t.Fatal("Should normalize export label")
	}
	opaque, ok := effect.NormalizeLabel(r.Labels[6]).(Opaque)
	if !ok || opaque.Param.Index != 6 {
		t.Fatal("Should normalize opaque label")
	}
	freeze, ok := effect.NormalizeLabel(r.Labels[7]).(Freeze)
	if !ok || freeze.Param.Index != 3 {
		t.Fatal("Should normalize freeze label")
	}
	if _, ok := effect.NormalizeLabel(r.Labels[8]).(BorrowAll); !ok {
		t.Fatal("Should normalize borrow_all label")
	}
}
