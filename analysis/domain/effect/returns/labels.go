package returns

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/constraint/expr"
	"github.com/wippyai/go-lua/analysis/domain/effect"
)

type Return struct {
	ReturnIndex int
	Transform   ReturnType
}

func (Return) EffectLabel() {}
func (r Return) String() string {
	return fmt.Sprintf("ret[%d].type = %s", r.ReturnIndex, r.Transform)
}
func (r Return) Equals(other effect.Label) bool {
	if o, ok := other.(Return); ok {
		return r.ReturnIndex == o.ReturnIndex && returnTypeEquals(r.Transform, o.Transform)
	}
	return false
}

type ErrorReturn struct {
	ValueIndex int
	ErrorIndex int
}

func (ErrorReturn) EffectLabel() {}
func (e ErrorReturn) String() string {
	return fmt.Sprintf("errret(val[%d], err[%d])", e.ValueIndex, e.ErrorIndex)
}
func (e ErrorReturn) Equals(other effect.Label) bool {
	if o, ok := other.(ErrorReturn); ok {
		return e.ValueIndex == o.ValueIndex && e.ErrorIndex == o.ErrorIndex
	}
	return false
}

type ReturnLength struct {
	ReturnIndex int
	Length      expr.Expr
}

func (ReturnLength) EffectLabel() {}
func (r ReturnLength) String() string {
	return fmt.Sprintf("ret[%d].len = %s", r.ReturnIndex, r.Length)
}
func (r ReturnLength) Equals(other effect.Label) bool {
	if o, ok := other.(ReturnLength); ok {
		return r.ReturnIndex == o.ReturnIndex && expr.ExprEquals(r.Length, o.Length)
	}
	return false
}

type ReturnType interface {
	returnType()
	String() string
}

type SelectCaseOfParam struct {
	Source effect.ParamRef
}

func (SelectCaseOfParam) returnType() {}
func (s SelectCaseOfParam) String() string {
	return fmt.Sprintf("select_case(%s)", s.Source)
}

const (
	SelectResultChannelField = "channel"
	SelectResultValueField   = "value"
)

type SelectResultOfCases struct {
	Cases   effect.ParamRef
	Default effect.ParamRef
}

func (SelectResultOfCases) returnType() {}
func (s SelectResultOfCases) String() string {
	return fmt.Sprintf("select_result(%s, %s)", s.Cases, s.Default)
}

type ElementOf struct {
	Source effect.ParamRef
}

func (ElementOf) returnType() {}
func (e ElementOf) String() string {
	return fmt.Sprintf("elem(%s)", e.Source)
}

type OptionalElementOf struct {
	Source effect.ParamRef
}

func (OptionalElementOf) returnType() {}
func (e OptionalElementOf) String() string {
	return fmt.Sprintf("elem(%s) | nil", e.Source)
}

type CallbackReturn struct {
	CallbackParam effect.ParamRef
}

func (CallbackReturn) returnType() {}
func (c CallbackReturn) String() string {
	return fmt.Sprintf("callback_ret(%s)", c.CallbackParam)
}

type ArrayOfCallbackReturn struct {
	CallbackParam effect.ParamRef
}

func (ArrayOfCallbackReturn) returnType() {}
func (a ArrayOfCallbackReturn) String() string {
	return fmt.Sprintf("array(callback_ret(%s))", a.CallbackParam)
}

type SameAs struct {
	Source effect.ParamRef
}

func (SameAs) returnType() {}
func (s SameAs) String() string {
	return fmt.Sprintf("same(%s)", s.Source)
}

type DeepElementOf struct {
	Source effect.ParamRef
}

func (DeepElementOf) returnType() {}
func (d DeepElementOf) String() string {
	return fmt.Sprintf("deep_elem(%s)", d.Source)
}

type StringUnpackValue struct {
	Format effect.ParamRef
}

func (StringUnpackValue) returnType() {}
func (s StringUnpackValue) String() string {
	return fmt.Sprintf("string_unpack(%s)", s.Format)
}

type CorrelatedReturn struct {
	Indices []int
}

func (CorrelatedReturn) EffectLabel() {}
func (c CorrelatedReturn) String() string {
	return fmt.Sprintf("correlated_return(%v)", c.Indices)
}
func (c CorrelatedReturn) Equals(other effect.Label) bool {
	o, ok := other.(CorrelatedReturn)
	if !ok || len(c.Indices) != len(o.Indices) {
		return false
	}
	for i := range c.Indices {
		if c.Indices[i] != o.Indices[i] {
			return false
		}
	}
	return true
}

func returnTypeEquals(a, b ReturnType) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return VisitReturnType(a, ReturnTypeVisitor[bool]{
		ElementOf: func(av ElementOf) bool {
			bv, ok := b.(ElementOf)
			return ok && av.Source.Index == bv.Source.Index
		},
		OptionalElementOf: func(av OptionalElementOf) bool {
			bv, ok := b.(OptionalElementOf)
			return ok && av.Source.Index == bv.Source.Index
		},
		CallbackReturn: func(av CallbackReturn) bool {
			bv, ok := b.(CallbackReturn)
			return ok && av.CallbackParam.Index == bv.CallbackParam.Index
		},
		ArrayOfCallbackReturn: func(av ArrayOfCallbackReturn) bool {
			bv, ok := b.(ArrayOfCallbackReturn)
			return ok && av.CallbackParam.Index == bv.CallbackParam.Index
		},
		SameAs: func(av SameAs) bool {
			bv, ok := b.(SameAs)
			return ok && av.Source.Index == bv.Source.Index
		},
		DeepElementOf: func(av DeepElementOf) bool {
			bv, ok := b.(DeepElementOf)
			return ok && av.Source.Index == bv.Source.Index
		},
		StringUnpackValue: func(av StringUnpackValue) bool {
			bv, ok := b.(StringUnpackValue)
			return ok && av.Format.Index == bv.Format.Index
		},
		TypeProjection: func(av TypeProjection) bool {
			bv, ok := b.(TypeProjection)
			if !ok || av.Source.Index != bv.Source.Index || len(av.Steps) != len(bv.Steps) {
				return false
			}
			for i := range av.Steps {
				if !typeProjectionStepEquals(av.Steps[i], bv.Steps[i]) {
					return false
				}
			}
			return true
		},
		SelectCaseOfParam: func(av SelectCaseOfParam) bool {
			bv, ok := b.(SelectCaseOfParam)
			return ok && av.Source.Index == bv.Source.Index
		},
		SelectResultOfCases: func(av SelectResultOfCases) bool {
			bv, ok := b.(SelectResultOfCases)
			return ok && av.Cases.Index == bv.Cases.Index && av.Default.Index == bv.Default.Index
		},
		Default: func(ReturnType) bool {
			return false
		},
	})
}
