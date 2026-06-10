package effect

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/constraint/expr"
)

func TestParamRef_String(t *testing.T) {
	tests := []struct {
		ref  ParamRef
		want string
	}{
		{ParamRef{Index: 0}, "param[0]"},
		{ParamRef{Index: 1}, "param[1]"},
		{ParamRef{Index: -1}, "param[last]"},
	}

	for _, tt := range tests {
		got := tt.ref.String()
		if got != tt.want {
			t.Errorf("ParamRef{%d}.String() = %q, want %q", tt.ref.Index, got, tt.want)
		}
	}
}

func TestReturn_String(t *testing.T) {
	r := Return{
		ReturnIndex: 0,
		Transform:   ElementOf{Source: ParamRef{Index: 0}},
	}
	if got := r.String(); got != "ret[0].type = elem(param[0])" {
		t.Errorf("Return.String() = %q", got)
	}
}

func TestReturn_Equals(t *testing.T) {
	r1 := Return{ReturnIndex: 0, Transform: ElementOf{Source: ParamRef{Index: 0}}}
	r2 := Return{ReturnIndex: 0, Transform: ElementOf{Source: ParamRef{Index: 0}}}
	r3 := Return{ReturnIndex: 1, Transform: ElementOf{Source: ParamRef{Index: 0}}}

	if !r1.Equals(r2) {
		t.Error("same Returns should be equal")
	}

	if r1.Equals(r3) {
		t.Error("different index Returns should not be equal")
	}

	if r1.Equals(ErrorReturn{}) {
		t.Error("Return should not equal ErrorReturn")
	}

	// Different transforms should not be equal
	r4 := Return{ReturnIndex: 0, Transform: SameAs{Source: ParamRef{Index: 0}}}
	if r1.Equals(r4) {
		t.Error("different transform Returns should not be equal")
	}

	r5 := Return{ReturnIndex: 0, Transform: ElementOf{Source: ParamRef{Index: 1}}}
	if r1.Equals(r5) {
		t.Error("different source in transform Returns should not be equal")
	}
}

func TestErrorReturn_String(t *testing.T) {
	er := ErrorReturn{ValueIndex: 0, ErrorIndex: 1}
	if got := er.String(); got != "errret(val[0], err[1])" {
		t.Errorf("ErrorReturn.String() = %q", got)
	}
}

func TestErrorReturn_Equals(t *testing.T) {
	e1 := ErrorReturn{ValueIndex: 0, ErrorIndex: 1}
	e2 := ErrorReturn{ValueIndex: 0, ErrorIndex: 1}
	e3 := ErrorReturn{ValueIndex: 1, ErrorIndex: 1}
	e4 := ErrorReturn{ValueIndex: 0, ErrorIndex: 2}

	if !e1.Equals(e2) {
		t.Error("same ErrorReturn should be equal")
	}
	if e1.Equals(e3) {
		t.Error("different value index should not be equal")
	}
	if e1.Equals(e4) {
		t.Error("different error index should not be equal")
	}
	if e1.Equals(Return{}) {
		t.Error("ErrorReturn should not equal Return")
	}
}

func TestReturnLength_String(t *testing.T) {
	rl := ReturnLength{
		ReturnIndex: 0,
		Length:      expr.PL(0),
	}

	got := rl.String()
	if got != "ret[0].len = len(param[0])" {
		t.Errorf("ReturnLength.String() = %q", got)
	}
}

func TestReturnLength_Equals(t *testing.T) {
	rl1 := ReturnLength{ReturnIndex: 0, Length: expr.PL(0)}
	rl2 := ReturnLength{ReturnIndex: 0, Length: expr.PL(0)}
	rl3 := ReturnLength{ReturnIndex: 1, Length: expr.PL(0)}

	if !rl1.Equals(rl2) {
		t.Error("same ReturnLengths should be equal")
	}

	if rl1.Equals(rl3) {
		t.Error("different index ReturnLengths should not be equal")
	}

	// Different lengths should not be equal
	rl4 := ReturnLength{ReturnIndex: 0, Length: expr.PL(1)}
	if rl1.Equals(rl4) {
		t.Error("different Length ReturnLengths should not be equal")
	}

	rl5 := ReturnLength{ReturnIndex: 0, Length: expr.C(5)}
	rl6 := ReturnLength{ReturnIndex: 0, Length: expr.C(5)}

	if !rl5.Equals(rl6) {
		t.Error("same constant Length ReturnLengths should be equal")
	}
}

func TestElementOf_String(t *testing.T) {
	e := ElementOf{Source: ParamRef{Index: 0}}
	if got := e.String(); got != "elem(param[0])" {
		t.Errorf("ElementOf.String() = %q", got)
	}
}

func TestOptionalElementOf_String(t *testing.T) {
	oe := OptionalElementOf{Source: ParamRef{Index: 0}}
	if got := oe.String(); got != "elem(param[0]) | nil" {
		t.Errorf("OptionalElementOf.String() = %q", got)
	}
}

func TestCallbackReturn_String(t *testing.T) {
	cr := CallbackReturn{CallbackParam: ParamRef{Index: 1}}
	if got := cr.String(); got != "callback_ret(param[1])" {
		t.Errorf("CallbackReturn.String() = %q", got)
	}
}

func TestArrayOfCallbackReturn_String(t *testing.T) {
	acr := ArrayOfCallbackReturn{CallbackParam: ParamRef{Index: 1}}
	if got := acr.String(); got != "array(callback_ret(param[1]))" {
		t.Errorf("ArrayOfCallbackReturn.String() = %q", got)
	}
}

func TestSameAs_String(t *testing.T) {
	sa := SameAs{Source: ParamRef{Index: 0}}
	if got := sa.String(); got != "same(param[0])" {
		t.Errorf("SameAs.String() = %q", got)
	}
}

func TestDeepElementOf_String(t *testing.T) {
	de := DeepElementOf{Source: ParamRef{Index: 0}}
	if got := de.String(); got != "deep_elem(param[0])" {
		t.Errorf("DeepElementOf.String() = %q", got)
	}
}

func TestStringUnpackValue_String(t *testing.T) {
	u := StringUnpackValue{Format: ParamRef{Index: 0}}
	if got := u.String(); got != "string_unpack(param[0])" {
		t.Errorf("StringUnpackValue.String() = %q", got)
	}
}

func TestLabelInterface(t *testing.T) {
	labels := []Label{
		Return{},
		ErrorReturn{},
		ReturnLength{},
	}

	for _, l := range labels {
		_ = l.String()
		_ = l.Equals(l)
	}
}

func TestReturnTypeInterface(t *testing.T) {
	returnTypes := []ReturnType{
		ElementOf{},
		OptionalElementOf{},
		CallbackReturn{},
		ArrayOfCallbackReturn{},
		SameAs{},
		DeepElementOf{},
		StringUnpackValue{},
		TypeProjection{},
	}

	for _, rt := range returnTypes {
		_ = rt.String()
	}
}

func TestAllLabelsImplementInterface(t *testing.T) {
	labels := []Label{
		Return{},
		ErrorReturn{},
		ReturnLength{},
		PassThrough{},
		FlowInto{},
		CorrelatedReturn{},
	}

	for _, l := range labels {
		_ = l.String()
		_ = l.Equals(l)
	}
}

func TestMarkerMethods(t *testing.T) {
	// Test EffectLabel marker methods
	Return{}.EffectLabel()
	ErrorReturn{}.EffectLabel()
	ReturnLength{}.EffectLabel()
	PassThrough{}.EffectLabel()
	FlowInto{}.EffectLabel()
	CorrelatedReturn{}.EffectLabel()

	// Test returnType() marker methods
	ElementOf{}.returnType()
	OptionalElementOf{}.returnType()
	CallbackReturn{}.returnType()
	ArrayOfCallbackReturn{}.returnType()
	SameAs{}.returnType()
	DeepElementOf{}.returnType()
	StringUnpackValue{}.returnType()
}

func TestReturnLengthEqualsNonMatch(t *testing.T) {
	rl := ReturnLength{ReturnIndex: 0}
	if rl.Equals(Return{}) {
		t.Error("ReturnLength should not equal Return")
	}
}

func TestCorrelatedReturn(t *testing.T) {
	cr := CorrelatedReturn{Indices: []int{0, 1, 2}}
	if got := cr.String(); got != "correlated_return([0 1 2])" {
		t.Errorf("CorrelatedReturn.String() = %q", got)
	}

	if !cr.Equals(CorrelatedReturn{Indices: []int{0, 1, 2}}) {
		t.Error("same CorrelatedReturn should be equal")
	}

	if cr.Equals(CorrelatedReturn{Indices: []int{0, 1}}) {
		t.Error("different length should not be equal")
	}

	if cr.Equals(CorrelatedReturn{Indices: []int{0, 1, 3}}) {
		t.Error("different indices should not be equal")
	}

	if cr.Equals(Return{}) {
		t.Error("CorrelatedReturn should not equal Return")
	}
}

func TestSelectCaseOfParam_String(t *testing.T) {
	s := SelectCaseOfParam{Source: ParamRef{Index: 0}}
	if got := s.String(); got != "select_case(param[0])" {
		t.Errorf("SelectCaseOfParam.String() = %q", got)
	}
}

func TestSelectResultOfCases_String(t *testing.T) {
	s := SelectResultOfCases{Cases: ParamRef{Index: 0}, Default: ParamRef{Index: 1}}
	if got := s.String(); got != "select_result(param[0], param[1])" {
		t.Errorf("SelectResultOfCases.String() = %q", got)
	}
}
