package contract

import (
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/typ"
)

// ErrorReturnForValue returns the explicit error-return label that controls the
// given value slot.
//
// Error-return correlation is relational evidence. A function returning
// `(T, Error?)` only proves independent slot types; it does not prove that
// `T` is nil exactly when the error slot is non-nil. Body inference may attach
// an explicit ErrorReturn label once it proves that relation.
func ErrorReturnForValue(fnType typ.Type, valueIndex int) *effect.ErrorReturn {
	if valueIndex < 0 {
		return nil
	}
	if spec := ExtractSpec(fnType); spec != nil {
		hasExplicitErrorReturn := false
		for _, label := range spec.Effects.Labels {
			if er, ok := label.(effect.ErrorReturn); ok {
				hasExplicitErrorReturn = true
				if er.ValueIndex == valueIndex {
					label := er
					return &label
				}
			}
		}
		if hasExplicitErrorReturn {
			return nil
		}
	}
	return nil
}
