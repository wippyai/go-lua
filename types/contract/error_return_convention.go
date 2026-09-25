package contract

import (
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/subtype"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// WithDeclaredErrorReturnConvention records the conventional return contract
// of a declaration that has no Lua return paths to inspect. Body-backed
// functions must instead carry only relations proved from their return paths.
func WithDeclaredErrorReturnConvention(fn *typ.Function) *typ.Function {
	if fn == nil || len(fn.Returns) < 2 {
		return fn
	}
	spec := ExtractSpec(fn)
	if spec != nil {
		for _, label := range spec.Effects.Labels {
			switch label.(type) {
			case effect.ErrorReturn, effect.CorrelatedReturn:
				return fn
			}
		}
	}
	errIndex := len(fn.Returns) - 1
	if !declaredOptionalErrorLike(fn.Returns[errIndex]) {
		return fn
	}
	errInner := unwrap.Optional(fn.Returns[errIndex])
	var values []int
	for i := 0; i < errIndex; i++ {
		if !unwrap.IsOptionalLike(fn.Returns[i]) {
			continue
		}
		// More than two returns can contain a second plausible error slot.
		// The trailing slot is then ambiguous, so the signature proves nothing.
		if errIndex > 1 && declaredHoldsErrorOf(fn.Returns[i], errInner) {
			return fn
		}
		values = append(values, i)
	}
	if len(values) == 0 {
		return fn
	}
	if spec == nil {
		spec = NewSpec()
	} else {
		clone := *spec
		spec = &clone
	}
	for i, value := range values {
		spec.Effects = spec.Effects.With(effect.ErrorReturn{ValueIndex: value, ErrorIndex: errIndex})
		for _, other := range values[i+1:] {
			spec.Effects = spec.Effects.With(effect.CorrelatedReturn{Indices: []int{value, other}})
		}
	}
	b := typ.Func().Effects(fn.Effects).Spec(spec).WithRefinement(fn.Refinement)
	for _, tp := range fn.TypeParams {
		b.TypeParam(tp.Name, tp.Constraint)
	}
	for _, p := range fn.Params {
		if p.Optional {
			b.OptParam(p.Name, p.Type)
		} else {
			b.Param(p.Name, p.Type)
		}
	}
	if fn.Variadic != nil {
		b.Variadic(fn.Variadic)
	}
	return b.Returns(fn.Returns...).Build()
}

func declaredHoldsErrorOf(t, errInner typ.Type) bool {
	inner := unwrap.Optional(t)
	return inner != nil && errInner != nil && !typ.IsAny(inner) && !typ.IsUnknown(inner) && subtype.IsSubtype(inner, errInner)
}

func declaredOptionalErrorLike(t typ.Type) bool {
	inner := unwrap.Optional(t)
	return inner != nil && declaredErrorLike(inner)
}

func declaredErrorLike(t typ.Type) bool {
	t = unwrap.Alias(t)
	if t == nil {
		return false
	}
	switch v := t.(type) {
	case *typ.Union:
		if len(v.Members) == 0 {
			return false
		}
		for _, m := range v.Members {
			if m != nil && m.Kind() != kind.Nil && !declaredErrorLike(m) {
				return false
			}
		}
		return true
	case *typ.Intersection:
		for _, m := range v.Members {
			if declaredErrorLike(m) {
				return true
			}
		}
		return false
	}
	if subtype.IsSubtype(t, typ.LuaError) || subtype.IsSubtype(t, typ.String) {
		return true
	}
	record, ok := t.(*typ.Record)
	if !ok {
		return false
	}
	field := record.GetField("message")
	if field == nil || field.Type == nil {
		return false
	}
	message := field.Type
	if subtype.IsSubtype(message, typ.String) {
		return true
	}
	inner := unwrap.Optional(message)
	return inner != nil && subtype.IsSubtype(inner, typ.String)
}
