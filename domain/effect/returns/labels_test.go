package returns

import (
	"testing"

	"github.com/wippyai/go-lua/domain/constraint/expr"
	"github.com/wippyai/go-lua/domain/effect"
	"github.com/wippyai/go-lua/domain/type/projection"
)

var (
	_ effect.Label = Return{}
	_ effect.Label = ErrorReturn{}
	_ effect.Label = ReturnLength{}
	_ effect.Label = CorrelatedReturn{}

	_ ReturnType = ElementOf{}
	_ ReturnType = OptionalElementOf{}
	_ ReturnType = CallbackReturn{}
	_ ReturnType = ArrayOfCallbackReturn{}
	_ ReturnType = SameAs{}
	_ ReturnType = TypeProjection{}
)

func TestReturn_String(t *testing.T) {
	r := Return{
		ReturnIndex: 0,
		Transform:   ElementOf{Source: effect.ParamRef{Index: 0}},
	}
	if got := r.String(); got != "ret[0].type = elem(param[0])" {
		t.Errorf("Return.String() = %q", got)
	}
}

func TestReturn_Equals(t *testing.T) {
	r1 := Return{ReturnIndex: 0, Transform: ElementOf{Source: effect.ParamRef{Index: 0}}}
	r2 := Return{ReturnIndex: 0, Transform: ElementOf{Source: effect.ParamRef{Index: 0}}}
	r3 := Return{ReturnIndex: 1, Transform: ElementOf{Source: effect.ParamRef{Index: 0}}}

	if !r1.Equals(r2) {
		t.Error("same Returns should be equal")
	}

	if r1.Equals(r3) {
		t.Error("different index Returns should not be equal")
	}

	if r1.Equals(ErrorReturn{}) {
		t.Error("Return should not equal ErrorReturn")
	}

	r4 := Return{ReturnIndex: 0, Transform: SameAs{Source: effect.ParamRef{Index: 0}}}
	if r1.Equals(r4) {
		t.Error("different transform Returns should not be equal")
	}

	r5 := Return{ReturnIndex: 0, Transform: ElementOf{Source: effect.ParamRef{Index: 1}}}
	if r1.Equals(r5) {
		t.Error("different source in transform Returns should not be equal")
	}
}

func TestReturnTypeEqualsNormalizesPointers(t *testing.T) {
	rt := ElementOf{Source: effect.ParamRef{Index: 0}}

	if !returnTypeEquals(&rt, rt) {
		t.Error("pointer return type on left should equal value return type")
	}
	if !returnTypeEquals(rt, &rt) {
		t.Error("value return type on left should equal pointer return type")
	}
	if !returnTypeEquals(&rt, &rt) {
		t.Error("pointer return types on both sides should be equal")
	}
}

func TestReturnTypeEqualsUsesKindVocabularyForEveryTransform(t *testing.T) {
	tests := []struct {
		name      string
		value     ReturnType
		pointer   ReturnType
		different ReturnType
	}{
		{
			name:      "same as",
			value:     SameAs{Source: effect.ParamRef{Index: 1}},
			pointer:   &SameAs{Source: effect.ParamRef{Index: 1}},
			different: SameAs{Source: effect.ParamRef{Index: 2}},
		},
		{
			name:      "element of",
			value:     ElementOf{Source: effect.ParamRef{Index: 2}},
			pointer:   &ElementOf{Source: effect.ParamRef{Index: 2}},
			different: ElementOf{Source: effect.ParamRef{Index: 3}},
		},
		{
			name:      "optional element of",
			value:     OptionalElementOf{Source: effect.ParamRef{Index: 3}},
			pointer:   &OptionalElementOf{Source: effect.ParamRef{Index: 3}},
			different: OptionalElementOf{Source: effect.ParamRef{Index: 4}},
		},
		{
			name:      "callback return",
			value:     CallbackReturn{CallbackParam: effect.ParamRef{Index: 4}},
			pointer:   &CallbackReturn{CallbackParam: effect.ParamRef{Index: 4}},
			different: CallbackReturn{CallbackParam: effect.ParamRef{Index: 5}},
		},
		{
			name:      "array callback return",
			value:     ArrayOfCallbackReturn{CallbackParam: effect.ParamRef{Index: 5}},
			pointer:   &ArrayOfCallbackReturn{CallbackParam: effect.ParamRef{Index: 5}},
			different: ArrayOfCallbackReturn{CallbackParam: effect.ParamRef{Index: 6}},
		},
		{
			name: "type projection",
			value: TypeProjection{
				Source:     effect.ParamRef{Index: 6},
				Projection: projection.Projection{Steps: []projection.Step{projection.Field("payload")}},
			},
			pointer: &TypeProjection{
				Source:     effect.ParamRef{Index: 6},
				Projection: projection.Projection{Steps: []projection.Step{projection.Field("payload")}},
			},
			different: TypeProjection{
				Source:     effect.ParamRef{Index: 6},
				Projection: projection.Projection{Steps: []projection.Step{projection.Field("other")}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !returnTypeEquals(tt.value, tt.pointer) {
				t.Fatalf("value form should equal pointer form")
			}
			if !returnTypeEquals(tt.pointer, tt.value) {
				t.Fatalf("pointer form should equal value form")
			}
			if returnTypeEquals(tt.value, tt.different) {
				t.Fatalf("different payload should not compare equal")
			}
		})
	}

	if returnTypeEquals(ElementOf{Source: effect.ParamRef{Index: 0}}, SameAs{Source: effect.ParamRef{Index: 0}}) {
		t.Fatal("different return-transform kinds should not compare equal")
	}
}

func TestReturnTypeEqualsHandlesTypedNilPointers(t *testing.T) {
	var nilElement *ElementOf
	var nilSame *SameAs

	if !IsNilReturnType(nilElement) {
		t.Fatal("typed nil ElementOf should be nil-like")
	}
	if !IsNilReturnType(nilSame) {
		t.Fatal("typed nil SameAs should be nil-like")
	}
	if !returnTypeEquals(nilElement, nilSame) {
		t.Fatal("nil-like return transforms should compare equal")
	}
	if returnTypeEquals(nilElement, ElementOf{}) {
		t.Fatal("typed nil ElementOf should not equal a concrete ElementOf")
	}
	if returnTypeEquals(SameAs{}, nilSame) {
		t.Fatal("concrete SameAs should not equal a typed nil SameAs")
	}
}

func TestReturnTypeConcreteNormalizers(t *testing.T) {
	same := SameAs{Source: effect.ParamRef{Index: 1}}
	element := ElementOf{Source: effect.ParamRef{Index: 2}}
	optional := OptionalElementOf{Source: effect.ParamRef{Index: 3}}
	callback := CallbackReturn{CallbackParam: effect.ParamRef{Index: 4}}
	arrayCallback := ArrayOfCallbackReturn{CallbackParam: effect.ParamRef{Index: 5}}
	projected := TypeProjection{
		Source:     effect.ParamRef{Index: 6},
		Projection: projection.Projection{Steps: []projection.Step{projection.Field("payload")}},
	}
	var nilSame *SameAs
	var nilElement *ElementOf
	var nilOptional *OptionalElementOf
	var nilCallback *CallbackReturn
	var nilArrayCallback *ArrayOfCallbackReturn
	var nilProjected *TypeProjection

	tests := []struct {
		name     string
		value    ReturnType
		pointer  ReturnType
		nilValue ReturnType
		match    func(ReturnType) bool
	}{
		{
			name:     "same as",
			value:    same,
			pointer:  &same,
			nilValue: nilSame,
			match: func(t ReturnType) bool {
				got, ok := AsSameAs(t)
				return ok && got.Source.Index == same.Source.Index
			},
		},
		{
			name:     "element of",
			value:    element,
			pointer:  &element,
			nilValue: nilElement,
			match: func(t ReturnType) bool {
				got, ok := AsElementOf(t)
				return ok && got.Source.Index == element.Source.Index
			},
		},
		{
			name:     "optional element of",
			value:    optional,
			pointer:  &optional,
			nilValue: nilOptional,
			match: func(t ReturnType) bool {
				got, ok := AsOptionalElementOf(t)
				return ok && got.Source.Index == optional.Source.Index
			},
		},
		{
			name:     "callback return",
			value:    callback,
			pointer:  &callback,
			nilValue: nilCallback,
			match: func(t ReturnType) bool {
				got, ok := AsCallbackReturn(t)
				return ok && got.CallbackParam.Index == callback.CallbackParam.Index
			},
		},
		{
			name:     "array callback return",
			value:    arrayCallback,
			pointer:  &arrayCallback,
			nilValue: nilArrayCallback,
			match: func(t ReturnType) bool {
				got, ok := AsArrayOfCallbackReturn(t)
				return ok && got.CallbackParam.Index == arrayCallback.CallbackParam.Index
			},
		},
		{
			name:     "type projection",
			value:    projected,
			pointer:  &projected,
			nilValue: nilProjected,
			match: func(t ReturnType) bool {
				got, ok := AsTypeProjection(t)
				return ok &&
					got.Source.Index == projected.Source.Index &&
					projection.Equal(got.Projection, projected.Projection)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if !tc.match(tc.value) {
				t.Fatalf("normalizer rejected value form %#v", tc.value)
			}
			if !tc.match(tc.pointer) {
				t.Fatalf("normalizer rejected pointer form %#v", tc.pointer)
			}
			if tc.match(tc.nilValue) {
				t.Fatalf("normalizer accepted typed nil pointer %#v", tc.nilValue)
			}
		})
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

func TestReturnTransforms_String(t *testing.T) {
	tests := []struct {
		name string
		rt   ReturnType
		want string
	}{
		{"element", ElementOf{Source: effect.ParamRef{Index: 0}}, "elem(param[0])"},
		{"optional element", OptionalElementOf{Source: effect.ParamRef{Index: 0}}, "elem(param[0]) | nil"},
		{"callback return", CallbackReturn{CallbackParam: effect.ParamRef{Index: 1}}, "callback_ret(param[1])"},
		{"array callback return", ArrayOfCallbackReturn{CallbackParam: effect.ParamRef{Index: 1}}, "array(callback_ret(param[1]))"},
		{"same as", SameAs{Source: effect.ParamRef{Index: 0}}, "same(param[0])"},
		{
			"type projection",
			TypeProjection{
				Source: effect.ParamRef{Index: 0},
				Projection: projection.Projection{Steps: []projection.Step{
					projection.Field("payload"),
					projection.CallableReturn(),
				}},
			},
			"project_type(param[0].payload.return)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.rt.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTypeProjectionTransform(t *testing.T) {
	base := TypeProjection{
		Source: effect.ParamRef{Index: 0},
		Projection: projection.Projection{Steps: []projection.Step{
			projection.Field("payload"),
			projection.GenericArg(0),
		}},
	}

	if got := (TypeProjection{Source: effect.ParamRef{Index: 0}}).String(); got != "project_type(param[0])" {
		t.Fatalf("empty TypeProjection.String() = %q", got)
	}
	if !returnTypeEquals(base, base) {
		t.Fatal("matching TypeProjection transforms should be equal")
	}
	if returnTypeEquals(base, TypeProjection{
		Source: effect.ParamRef{Index: 1},
		Projection: projection.Projection{Steps: []projection.Step{
			projection.Field("payload"),
			projection.GenericArg(0),
		}},
	}) {
		t.Fatal("different TypeProjection source should not be equal")
	}
	if returnTypeEquals(base, TypeProjection{
		Source: effect.ParamRef{Index: 0},
		Projection: projection.Projection{Steps: []projection.Step{
			projection.Field("payload"),
			projection.GenericArg(1),
		}},
	}) {
		t.Fatal("different TypeProjection descriptor should not be equal")
	}
}

func TestLabelInterface(t *testing.T) {
	labels := []struct {
		label effect.Label
		want  string
	}{
		{Return{ReturnIndex: 1, Transform: SameAs{Source: effect.ParamRef{Index: 2}}}, "ret[1].type = same(param[2])"},
		{ErrorReturn{ValueIndex: 3, ErrorIndex: 4}, "errret(val[3], err[4])"},
		{ReturnLength{ReturnIndex: 5, Length: expr.C(6)}, "ret[5].len = 6"},
	}
	for _, test := range labels {
		if got := test.label.String(); got != test.want {
			t.Errorf("%T.String() = %q, want %q", test.label, got, test.want)
		}
		if !test.label.Equals(test.label) {
			t.Errorf("%T did not equal itself", test.label)
		}
	}
}

func TestReturnTypeInterface(t *testing.T) {
	returnTypes := []struct {
		transform ReturnType
		want      string
	}{
		{ElementOf{Source: effect.ParamRef{Index: 0}}, "elem(param[0])"},
		{OptionalElementOf{Source: effect.ParamRef{Index: 1}}, "elem(param[1]) | nil"},
		{CallbackReturn{CallbackParam: effect.ParamRef{Index: 2}}, "callback_ret(param[2])"},
		{ArrayOfCallbackReturn{CallbackParam: effect.ParamRef{Index: 3}}, "array(callback_ret(param[3]))"},
		{SameAs{Source: effect.ParamRef{Index: 4}}, "same(param[4])"},
		{TypeProjection{Source: effect.ParamRef{Index: 7}, Projection: projection.Projection{Steps: []projection.Step{projection.Field("payload")}}}, "project_type(param[7].payload)"},
	}
	for _, test := range returnTypes {
		if got := test.transform.String(); got != test.want {
			t.Errorf("%T.String() = %q, want %q", test.transform, got, test.want)
		}
	}
}

func TestAllLabelsImplementInterface(t *testing.T) {
	labels := []struct {
		label effect.Label
		want  string
	}{
		{Return{ReturnIndex: 11, Transform: ElementOf{Source: effect.ParamRef{Index: 12}}}, "ret[11].type = elem(param[12])"},
		{ErrorReturn{ValueIndex: 13, ErrorIndex: 14}, "errret(val[13], err[14])"},
		{ReturnLength{ReturnIndex: 15, Length: expr.PL(16)}, "ret[15].len = len(param[16])"},
		{CorrelatedReturn{Indices: []int{17, 18}}, "correlated_return([17 18])"},
	}
	for _, test := range labels {
		if got := test.label.String(); got != test.want {
			t.Errorf("%T.String() = %q, want %q", test.label, got, test.want)
		}
		if !test.label.Equals(test.label) {
			t.Errorf("%T did not equal itself", test.label)
		}
	}
}

func TestMarkerMethods(t *testing.T) {
	label := CorrelatedReturn{Indices: []int{19, 20, 21}}
	if got := label.String(); got != "correlated_return([19 20 21])" || !label.Equals(CorrelatedReturn{Indices: []int{19, 20, 21}}) || label.Equals(CorrelatedReturn{Indices: []int{19, 21, 20}}) {
		t.Errorf("marker-backed CorrelatedReturn rendered/equalled as %q", got)
	}
}

func TestRowNormalization(t *testing.T) {
	r := effect.Row{Labels: []effect.Label{
		Return{ReturnIndex: 0, Transform: ElementOf{Source: effect.ParamRef{Index: 0}}},
		ErrorReturn{ValueIndex: 0, ErrorIndex: 1},
		ReturnLength{ReturnIndex: 0, Length: expr.PL(0)},
		CorrelatedReturn{Indices: []int{0, 1, 2}},
	}}

	ret, ok := effect.NormalizeLabel(r.Labels[0]).(Return)
	if !ok || ret.ReturnIndex != 0 {
		t.Fatal("Should normalize return label")
	}
	errRet, ok := effect.NormalizeLabel(r.Labels[1]).(ErrorReturn)
	if !ok || errRet.ValueIndex != 0 || errRet.ErrorIndex != 1 {
		t.Fatal("Should normalize error return label")
	}
	retLen, ok := effect.NormalizeLabel(r.Labels[2]).(ReturnLength)
	if !ok || retLen.ReturnIndex != 0 {
		t.Fatal("Should normalize return length label")
	}
	corr, ok := effect.NormalizeLabel(r.Labels[3]).(CorrelatedReturn)
	if !ok || len(corr.Indices) != 3 {
		t.Fatal("Should normalize correlated return label")
	}
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
