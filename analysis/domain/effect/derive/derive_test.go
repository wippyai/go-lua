package derive

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/effect"
	"github.com/wippyai/go-lua/analysis/domain/effect/returns"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
)

func errType() typ.Type {
	return typ.NewInterface("Error", []typ.Method{
		{Name: "message", Type: typ.Func().Param("self", typ.Self).Returns(typ.String).Build()},
	})
}

func errorReturnsOf(row effect.Row) []returns.ErrorReturn {
	var out []returns.ErrorReturn
	for _, label := range row.Labels {
		if er, ok := effect.NormalizeLabel(label).(returns.ErrorReturn); ok {
			out = append(out, er)
		}
	}
	return out
}

func applyDefault(fn *typ.Function, known effect.Row, e typ.Type) effect.Row {
	return Apply(fn, known, Context{ErrorType: e}, Default...)
}

// Two-return convention: a single value correlated with the trailing error.
func TestErrorReturnFromShape_TwoReturn(t *testing.T) {
	fn := typ.Func().Param("name", typ.String).Returns(typ.Any, typeexpr.Optional(errType())).Build()
	got := errorReturnsOf(applyDefault(fn, effect.Empty, errType()))
	if len(got) != 1 || got[0] != (returns.ErrorReturn{ValueIndex: 0, ErrorIndex: 1}) {
		t.Fatalf("got %v, want [{0 1}]", got)
	}
}

// Multi-value: every leading slot correlates with the trailing error, proving
// the rule is not hardcoded to exactly two returns.
func TestErrorReturnFromShape_MultiValue(t *testing.T) {
	fn := typ.Func().Param("s", typ.String).Returns(typ.String, typ.String, typeexpr.Optional(errType())).Build()
	got := errorReturnsOf(applyDefault(fn, effect.Empty, errType()))
	want := map[returns.ErrorReturn]bool{{ValueIndex: 0, ErrorIndex: 2}: true, {ValueIndex: 1, ErrorIndex: 2}: true}
	if len(got) != 2 {
		t.Fatalf("got %v, want two labels", got)
	}
	for _, er := range got {
		if !want[er] {
			t.Fatalf("unexpected label %v", er)
		}
	}
}

// A trailing optional of a non-error type is not an error return.
func TestErrorReturnFromShape_NonErrorTrailingOptional(t *testing.T) {
	fn := typ.Func().Param("s", typ.String).Returns(typ.String, typeexpr.Optional(typ.String)).Build()
	if got := errorReturnsOf(applyDefault(fn, effect.Empty, errType())); len(got) != 0 {
		t.Fatalf("got %v, want none for non-error trailing optional", got)
	}
}

// Nil ErrorType disables the rule entirely.
func TestErrorReturnFromShape_NoErrorTypeIsInert(t *testing.T) {
	fn := typ.Func().Returns(typ.Any, typeexpr.Optional(errType())).Build()
	if got := errorReturnsOf(Apply(fn, effect.Empty, Context{}, Default...)); len(got) != 0 {
		t.Fatalf("got %v, want rule inert without ErrorType", got)
	}
}

// An already-known ErrorReturn is not duplicated or overridden.
func TestErrorReturnFromShape_RespectsKnownEffect(t *testing.T) {
	fn := typ.Func().Returns(typ.Any, typeexpr.Optional(errType())).Build()
	known := effect.Empty.With(returns.ErrorReturn{ValueIndex: 0, ErrorIndex: 1})
	if got := errorReturnsOf(applyDefault(fn, known, errType())); len(got) != 1 {
		t.Fatalf("got %v, want the single known label preserved", got)
	}
}

// Rules compose: a second registered rule also contributes.
func TestApply_ComposesRules(t *testing.T) {
	marker := returns.Return{ReturnIndex: 0}
	extra := func(*typ.Function, effect.Row, Context) []effect.Label { return []effect.Label{marker} }
	fn := typ.Func().Returns(typ.Any, typeexpr.Optional(errType())).Build()
	row := Apply(fn, effect.Empty, Context{ErrorType: errType()}, ErrorReturnFromShape, extra)
	if len(errorReturnsOf(row)) != 1 {
		t.Fatalf("error-return rule did not contribute: %v", row.Labels)
	}
	if !row.Has(func(l effect.Label) bool { return l.Equals(marker) }) {
		t.Fatalf("second rule did not contribute: %v", row.Labels)
	}
}
