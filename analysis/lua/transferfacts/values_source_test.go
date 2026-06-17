package transferfacts

import (
	"testing"

	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/compiler/ast"
)

func TestValueSourceLowersExplicitUnknown(t *testing.T) {
	l := lowerer{exprs: make(map[any]factflow.ExprRef)}

	source := l.valueSource(sourceprovenance.NewUnknownSource(2))
	if source.Kind != factflow.ValueSourceUnknown || source.TargetIndex != 2 || !source.Valid() {
		t.Fatalf("unknown value source = %#v, want valid unknown for target 2", source)
	}
	if source.HasExpr || source.ExprRef != 0 || source.HasCallPoint || source.CallPoint != 0 {
		t.Fatalf("unknown value source retained evidence refs: %#v", source)
	}
}

func TestValueSourceExprRefDistinguishesWrappedVarargSlots(t *testing.T) {
	inner := &ast.Comma3Expr{}
	wrapped := &ast.NonNilAssertExpr{Expr: inner}
	shape, ok := sourceprovenance.NewSourceShape(true, true, false, true)
	if !ok {
		t.Fatalf("expanded open-tail shape rejected")
	}

	firstSource, ok := sourceprovenance.NewVarargSource(wrapped, 0, 0, 0, shape)
	if !ok {
		t.Fatalf("first wrapped vararg source rejected")
	}
	secondSource, ok := sourceprovenance.NewVarargSource(wrapped, 0, 1, 1, shape)
	if !ok {
		t.Fatalf("second wrapped vararg source rejected")
	}

	l := lowerer{exprs: make(map[any]factflow.ExprRef)}
	first := l.valueSource(firstSource)
	second := l.valueSource(secondSource)
	if first.Kind != factflow.ValueSourceVararg || second.Kind != factflow.ValueSourceVararg {
		t.Fatalf("wrapped vararg sources = %#v / %#v, want vararg", first, second)
	}
	if first.ExprRef == 0 || second.ExprRef == 0 || first.ExprRef == second.ExprRef {
		t.Fatalf("wrapped vararg refs = %d / %d, want distinct non-zero refs", first.ExprRef, second.ExprRef)
	}
	if first.ResultIndex != 0 || second.ResultIndex != 1 || first.TargetIndex != 0 || second.TargetIndex != 1 {
		t.Fatalf("wrapped vararg slots = %#v / %#v, want slot-specific evidence", first, second)
	}
}
