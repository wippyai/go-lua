package returns

import (
	"fmt"

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
	case TypeProjection:
		return typeProjectionEquals(av, b)
	case *TypeProjection:
		return typeProjectionEquals(*av, b)
	default:
		return reservedReturnTypeEquals(a, b)
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

func typeProjectionEquals(a TypeProjection, b ReturnType) bool {
	bb, ok := normalizeTypeProjection(b)
	if !ok || a.Source.Index != bb.Source.Index {
		return false
	}
	return projection.Equal(a.Projection, bb.Projection)
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
