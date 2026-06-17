package returns

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/constraint/expr"
	"github.com/wippyai/go-lua/analysis/domain/effect"
)

// ReturnLength is reserved return metadata. It is audited by capability
// descriptors, but not actively lowered into return semantics.
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

// SelectCaseOfParam is reserved return-transform vocabulary. It is not
// actively lowered while select result semantics remain factflow-owned.
type SelectCaseOfParam struct {
	Source effect.ParamRef
}

func (SelectCaseOfParam) returnType() {}
func (s SelectCaseOfParam) String() string {
	return fmt.Sprintf("select_case(%s)", s.Source)
}

// SelectResultOfCases is reserved return-transform vocabulary. It is not
// actively lowered while select result semantics remain factflow-owned.
type SelectResultOfCases struct {
	Cases   effect.ParamRef
	Default effect.ParamRef
}

func (SelectResultOfCases) returnType() {}
func (s SelectResultOfCases) String() string {
	return fmt.Sprintf("select_result(%s, %s)", s.Cases, s.Default)
}

// DeepElementOf is reserved return-transform vocabulary. While inactive,
// lowering falls back to the declared return type.
type DeepElementOf struct {
	Source effect.ParamRef
}

func (DeepElementOf) returnType() {}
func (d DeepElementOf) String() string {
	return fmt.Sprintf("deep_elem(%s)", d.Source)
}

// StringUnpackValue is reserved high-risk return-transform metadata. Stdlib
// signatures must not declare it while lowering ignores it.
type StringUnpackValue struct {
	Format effect.ParamRef
}

func (StringUnpackValue) returnType() {}
func (s StringUnpackValue) String() string {
	return fmt.Sprintf("string_unpack(%s)", s.Format)
}

// CorrelatedReturn is reserved high-risk return metadata. Stdlib signatures
// must not declare it while lowering ignores it.
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

func reservedReturnTypeEquals(a, b ReturnType) bool {
	switch av := a.(type) {
	case DeepElementOf:
		return deepElementOfEquals(av, b)
	case *DeepElementOf:
		return av != nil && deepElementOfEquals(*av, b)
	case StringUnpackValue:
		return stringUnpackValueEquals(av, b)
	case *StringUnpackValue:
		return av != nil && stringUnpackValueEquals(*av, b)
	case SelectCaseOfParam:
		return selectCaseOfParamEquals(av, b)
	case *SelectCaseOfParam:
		return av != nil && selectCaseOfParamEquals(*av, b)
	case SelectResultOfCases:
		return selectResultOfCasesEquals(av, b)
	case *SelectResultOfCases:
		return av != nil && selectResultOfCasesEquals(*av, b)
	default:
		return false
	}
}

func deepElementOfEquals(a DeepElementOf, b ReturnType) bool {
	bb, ok := normalizeDeepElementOf(b)
	return ok && a.Source.Index == bb.Source.Index
}

func stringUnpackValueEquals(a StringUnpackValue, b ReturnType) bool {
	bb, ok := normalizeStringUnpackValue(b)
	return ok && a.Format.Index == bb.Format.Index
}

func selectCaseOfParamEquals(a SelectCaseOfParam, b ReturnType) bool {
	bb, ok := normalizeSelectCaseOfParam(b)
	return ok && a.Source.Index == bb.Source.Index
}

func selectResultOfCasesEquals(a SelectResultOfCases, b ReturnType) bool {
	bb, ok := normalizeSelectResultOfCases(b)
	return ok && a.Cases.Index == bb.Cases.Index && a.Default.Index == bb.Default.Index
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
