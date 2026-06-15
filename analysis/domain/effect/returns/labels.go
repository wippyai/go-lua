package returns

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/constraint/expr"
	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/type/projection"
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
	if o, ok := effect.NormalizeLabel(other).(Return); ok {
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
	if o, ok := effect.NormalizeLabel(other).(ErrorReturn); ok {
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
	if o, ok := effect.NormalizeLabel(other).(ReturnLength); ok {
		return r.ReturnIndex == o.ReturnIndex && expr.ExprEquals(r.Length, o.Length)
	}
	return false
}

type ReturnType interface {
	returnType()
	String() string
}

type TypeProjection struct {
	Source     effect.ParamRef
	Projection projection.Projection
}

func (TypeProjection) returnType() {}
func (p TypeProjection) String() string {
	path := p.Projection.String()
	if path == "" {
		return fmt.Sprintf("project_type(%s)", p.Source)
	}
	return fmt.Sprintf("project_type(%s.%s)", p.Source, path)
}

type SelectCaseOfParam struct {
	Source effect.ParamRef
}

func (SelectCaseOfParam) returnType() {}
func (s SelectCaseOfParam) String() string {
	return fmt.Sprintf("select_case(%s)", s.Source)
}

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
	o, ok := effect.NormalizeLabel(other).(CorrelatedReturn)
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
	switch av := a.(type) {
	case ElementOf:
		return elementOfEquals(av, b)
	case *ElementOf:
		return elementOfEquals(*av, b)
	case OptionalElementOf:
		return optionalElementOfEquals(av, b)
	case *OptionalElementOf:
		return optionalElementOfEquals(*av, b)
	case CallbackReturn:
		return callbackReturnEquals(av, b)
	case *CallbackReturn:
		return callbackReturnEquals(*av, b)
	case ArrayOfCallbackReturn:
		return arrayOfCallbackReturnEquals(av, b)
	case *ArrayOfCallbackReturn:
		return arrayOfCallbackReturnEquals(*av, b)
	case SameAs:
		return sameAsEquals(av, b)
	case *SameAs:
		return sameAsEquals(*av, b)
	case DeepElementOf:
		return deepElementOfEquals(av, b)
	case *DeepElementOf:
		return deepElementOfEquals(*av, b)
	case StringUnpackValue:
		return stringUnpackValueEquals(av, b)
	case *StringUnpackValue:
		return stringUnpackValueEquals(*av, b)
	case TypeProjection:
		return typeProjectionEquals(av, b)
	case *TypeProjection:
		return typeProjectionEquals(*av, b)
	case SelectCaseOfParam:
		return selectCaseOfParamEquals(av, b)
	case *SelectCaseOfParam:
		return selectCaseOfParamEquals(*av, b)
	case SelectResultOfCases:
		return selectResultOfCasesEquals(av, b)
	case *SelectResultOfCases:
		return selectResultOfCasesEquals(*av, b)
	default:
		return false
	}
}

func elementOfEquals(a ElementOf, b ReturnType) bool {
	bb, ok := normalizeElementOf(b)
	return ok && a.Source.Index == bb.Source.Index
}

func optionalElementOfEquals(a OptionalElementOf, b ReturnType) bool {
	bb, ok := normalizeOptionalElementOf(b)
	return ok && a.Source.Index == bb.Source.Index
}

func callbackReturnEquals(a CallbackReturn, b ReturnType) bool {
	bb, ok := normalizeCallbackReturn(b)
	return ok && a.CallbackParam.Index == bb.CallbackParam.Index
}

func arrayOfCallbackReturnEquals(a ArrayOfCallbackReturn, b ReturnType) bool {
	bb, ok := normalizeArrayOfCallbackReturn(b)
	return ok && a.CallbackParam.Index == bb.CallbackParam.Index
}

func sameAsEquals(a SameAs, b ReturnType) bool {
	bb, ok := normalizeSameAs(b)
	return ok && a.Source.Index == bb.Source.Index
}

func deepElementOfEquals(a DeepElementOf, b ReturnType) bool {
	bb, ok := normalizeDeepElementOf(b)
	return ok && a.Source.Index == bb.Source.Index
}

func stringUnpackValueEquals(a StringUnpackValue, b ReturnType) bool {
	bb, ok := normalizeStringUnpackValue(b)
	return ok && a.Format.Index == bb.Format.Index
}

func typeProjectionEquals(a TypeProjection, b ReturnType) bool {
	bb, ok := normalizeTypeProjection(b)
	if !ok || a.Source.Index != bb.Source.Index {
		return false
	}
	return projection.Equal(a.Projection, bb.Projection)
}

func selectCaseOfParamEquals(a SelectCaseOfParam, b ReturnType) bool {
	bb, ok := normalizeSelectCaseOfParam(b)
	return ok && a.Source.Index == bb.Source.Index
}

func selectResultOfCasesEquals(a SelectResultOfCases, b ReturnType) bool {
	bb, ok := normalizeSelectResultOfCases(b)
	return ok && a.Cases.Index == bb.Cases.Index && a.Default.Index == bb.Default.Index
}

func normalizeElementOf(r ReturnType) (ElementOf, bool) {
	switch rr := r.(type) {
	case ElementOf:
		return rr, true
	case *ElementOf:
		if rr != nil {
			return *rr, true
		}
	}
	return ElementOf{}, false
}

func normalizeOptionalElementOf(r ReturnType) (OptionalElementOf, bool) {
	switch rr := r.(type) {
	case OptionalElementOf:
		return rr, true
	case *OptionalElementOf:
		if rr != nil {
			return *rr, true
		}
	}
	return OptionalElementOf{}, false
}

func normalizeCallbackReturn(r ReturnType) (CallbackReturn, bool) {
	switch rr := r.(type) {
	case CallbackReturn:
		return rr, true
	case *CallbackReturn:
		if rr != nil {
			return *rr, true
		}
	}
	return CallbackReturn{}, false
}

func normalizeArrayOfCallbackReturn(r ReturnType) (ArrayOfCallbackReturn, bool) {
	switch rr := r.(type) {
	case ArrayOfCallbackReturn:
		return rr, true
	case *ArrayOfCallbackReturn:
		if rr != nil {
			return *rr, true
		}
	}
	return ArrayOfCallbackReturn{}, false
}

func normalizeSameAs(r ReturnType) (SameAs, bool) {
	switch rr := r.(type) {
	case SameAs:
		return rr, true
	case *SameAs:
		if rr != nil {
			return *rr, true
		}
	}
	return SameAs{}, false
}

func normalizeDeepElementOf(r ReturnType) (DeepElementOf, bool) {
	switch rr := r.(type) {
	case DeepElementOf:
		return rr, true
	case *DeepElementOf:
		if rr != nil {
			return *rr, true
		}
	}
	return DeepElementOf{}, false
}

func normalizeStringUnpackValue(r ReturnType) (StringUnpackValue, bool) {
	switch rr := r.(type) {
	case StringUnpackValue:
		return rr, true
	case *StringUnpackValue:
		if rr != nil {
			return *rr, true
		}
	}
	return StringUnpackValue{}, false
}

func normalizeTypeProjection(r ReturnType) (TypeProjection, bool) {
	switch rr := r.(type) {
	case TypeProjection:
		return rr, true
	case *TypeProjection:
		if rr != nil {
			return *rr, true
		}
	}
	return TypeProjection{}, false
}

func normalizeSelectCaseOfParam(r ReturnType) (SelectCaseOfParam, bool) {
	switch rr := r.(type) {
	case SelectCaseOfParam:
		return rr, true
	case *SelectCaseOfParam:
		if rr != nil {
			return *rr, true
		}
	}
	return SelectCaseOfParam{}, false
}

func normalizeSelectResultOfCases(r ReturnType) (SelectResultOfCases, bool) {
	switch rr := r.(type) {
	case SelectResultOfCases:
		return rr, true
	case *SelectResultOfCases:
		if rr != nil {
			return *rr, true
		}
	}
	return SelectResultOfCases{}, false
}
