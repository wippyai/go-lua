package derive

import (
	"github.com/wippyai/go-lua/domain/effect"
	"github.com/wippyai/go-lua/domain/effect/returns"
	"github.com/wippyai/go-lua/domain/type/typ"
	"github.com/wippyai/go-lua/domain/type/typeexpr"
)

// ErrorReturnFromShape recovers the (value, error) presence correlation for a
// function whose final return slot is structurally Optional(ctx.ErrorType).
//
// The convention is the Lua error idiom generalized to any arity: when the
// trailing error is present every preceding result is absent, and when the error
// is absent they are present. That is exactly the relation the body summarizer
// proves for functions it can see; this recovers it from the type for functions
// it cannot. It emits one returns.ErrorReturn per leading value slot.
//
// The rule is inert unless ctx.ErrorType is set, never recognizes a type by name
// (it compares structural identity), and yields nothing when known already
// carries an ErrorReturn so an explicit effect always wins.
func ErrorReturnFromShape(fn *typ.Function, known effect.Row, ctx Context) []effect.Label {
	if fn == nil || ctx.ErrorType == nil {
		return nil
	}
	errorIndex := len(fn.Returns) - 1
	if errorIndex < 1 {
		return nil
	}
	if !typ.TypeEquals(fn.Returns[errorIndex], typeexpr.Optional(ctx.ErrorType)) {
		return nil
	}
	if known.Has(isErrorReturn) {
		return nil
	}
	labels := make([]effect.Label, 0, errorIndex)
	for value := range errorIndex {
		labels = append(labels, returns.ErrorReturn{ValueIndex: value, ErrorIndex: errorIndex})
	}
	return labels
}

func isErrorReturn(label effect.Label) bool {
	_, ok := effect.NormalizeLabel(label).(returns.ErrorReturn)
	return ok
}
