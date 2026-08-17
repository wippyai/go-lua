package returns

import (
	"fmt"
	"reflect"

	"github.com/wippyai/go-lua/domain/effect"
	"github.com/wippyai/go-lua/domain/effect/capability"
	"github.com/wippyai/go-lua/domain/type/projection"
	"github.com/wippyai/go-lua/domain/type/typ"
)

type Return struct {
	ReturnIndex int
	Transform   ReturnType
}

// CapabilityID answers the capability of the transform the label carries: the
// audited vocabulary distinguishes the seven return transforms, so the label's
// classification is its transform's kind. A Return with an absent transform,
// a typed nil pointer spelling included, carries no classifiable payload and
// answers the empty string.
func (r Return) CapabilityID() string {
	return KindOfReturnType(r.Transform).CapabilityID()
}
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

func (ErrorReturn) CapabilityID() string { return capability.ReturnsErrorReturn }
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

// IsNilReturnType reports whether transform is absent, including typed nil
// pointer values stored behind the ReturnType interface.
func IsNilReturnType(transform ReturnType) bool {
	if transform == nil {
		return true
	}
	v := reflect.ValueOf(transform)
	return v.Kind() == reflect.Pointer && v.IsNil()
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

type ConditionalType struct {
	Source     effect.ParamRef
	Projection projection.Projection
	When       typ.Type
	Then       typ.Type
}

func (ConditionalType) returnType() {}
func (c ConditionalType) String() string {
	path := c.Projection.String()
	source := c.Source.String()
	if path != "" {
		source += "." + path
	}
	return fmt.Sprintf("if_type(%s <: %s -> %s)", source, c.When, c.Then)
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
	aNil := IsNilReturnType(a)
	bNil := IsNilReturnType(b)
	if aNil || bNil {
		return aNil && bNil
	}
	kind := KindOfReturnType(a)
	if kind == ReturnTypeUnknown || kind != KindOfReturnType(b) {
		return false
	}
	switch kind {
	case ReturnTypeSameAs:
		aa, aok := AsSameAs(a)
		bb, bok := AsSameAs(b)
		return aok && bok && aa.Source.Index == bb.Source.Index
	case ReturnTypeElementOf:
		aa, aok := AsElementOf(a)
		bb, bok := AsElementOf(b)
		return aok && bok && aa.Source.Index == bb.Source.Index
	case ReturnTypeOptionalElementOf:
		aa, aok := AsOptionalElementOf(a)
		bb, bok := AsOptionalElementOf(b)
		return aok && bok && aa.Source.Index == bb.Source.Index
	case ReturnTypeCallbackReturn:
		aa, aok := AsCallbackReturn(a)
		bb, bok := AsCallbackReturn(b)
		return aok && bok && aa.CallbackParam.Index == bb.CallbackParam.Index
	case ReturnTypeArrayOfCallbackReturn:
		aa, aok := AsArrayOfCallbackReturn(a)
		bb, bok := AsArrayOfCallbackReturn(b)
		return aok && bok && aa.CallbackParam.Index == bb.CallbackParam.Index
	case ReturnTypeTypeProjection:
		aa, aok := AsTypeProjection(a)
		bb, bok := AsTypeProjection(b)
		return aok && bok && aa.Source.Index == bb.Source.Index && projection.Equal(aa.Projection, bb.Projection)
	case ReturnTypeConditionalType:
		aa, aok := AsConditionalType(a)
		bb, bok := AsConditionalType(b)
		return aok && bok &&
			aa.Source.Index == bb.Source.Index &&
			projection.Equal(aa.Projection, bb.Projection) &&
			typ.TypeEquals(aa.When, bb.When) &&
			typ.TypeEquals(aa.Then, bb.Then)
	default:
		return false
	}
}

// AsElementOf returns the concrete ElementOf transform for value and non-nil
// pointer spellings. Typed nil pointers are treated as absent.
func AsElementOf(r ReturnType) (ElementOf, bool) {
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

// AsOptionalElementOf returns the concrete OptionalElementOf transform for
// value and non-nil pointer spellings. Typed nil pointers are treated as absent.
func AsOptionalElementOf(r ReturnType) (OptionalElementOf, bool) {
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

// AsCallbackReturn returns the concrete CallbackReturn transform for value and
// non-nil pointer spellings. Typed nil pointers are treated as absent.
func AsCallbackReturn(r ReturnType) (CallbackReturn, bool) {
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

// AsArrayOfCallbackReturn returns the concrete ArrayOfCallbackReturn transform
// for value and non-nil pointer spellings. Typed nil pointers are treated as
// absent.
func AsArrayOfCallbackReturn(r ReturnType) (ArrayOfCallbackReturn, bool) {
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

// AsSameAs returns the concrete SameAs transform for value and non-nil pointer
// spellings. Typed nil pointers are treated as absent.
func AsSameAs(r ReturnType) (SameAs, bool) {
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

// AsTypeProjection returns the concrete TypeProjection transform for value and
// non-nil pointer spellings. Typed nil pointers are treated as absent.
func AsTypeProjection(r ReturnType) (TypeProjection, bool) {
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

// AsConditionalType returns the concrete ConditionalType transform for value
// and non-nil pointer spellings. Typed nil pointers are treated as absent.
func AsConditionalType(r ReturnType) (ConditionalType, bool) {
	switch rr := r.(type) {
	case ConditionalType:
		return rr, true
	case *ConditionalType:
		if rr != nil {
			return *rr, true
		}
	}
	return ConditionalType{}, false
}
