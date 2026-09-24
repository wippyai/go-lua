package assign

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

func TestInferErrorReturnConvention_CorrelatesEveryValueWithTrailingError(t *testing.T) {
	m := typ.NewMap(typ.String, typ.Any)
	fn := typ.Func().Returns(typ.NewOptional(m), typ.NewOptional(m), typ.NewOptional(m), typ.NewOptional(typ.String)).Build()

	inverse, co := InferErrorReturnConvention(fn)
	if len(inverse) != 3 {
		t.Fatalf("expected each value slot inversely correlated with the error, got %v", inverse)
	}
	for i, c := range inverse {
		if c.ValueIndex != i || c.ErrorIndex != 3 {
			t.Fatalf("unexpected inverse correlation %v", c)
		}
	}
	if len(co) != 3 {
		t.Fatalf("expected the value slots co-correlated pairwise, got %v", co)
	}

	noError := typ.Func().Returns(typ.NewOptional(m), typ.NewOptional(m), typ.Integer).Build()
	if inverse, co := InferErrorReturnConvention(noError); inverse != nil || co != nil {
		t.Fatalf("a trailing non-error slot must not correlate, got %v %v", inverse, co)
	}
}
