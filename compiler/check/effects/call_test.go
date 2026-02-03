package effects

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/typ"
)

func TestResolveCallEffect_NilInfo(t *testing.T) {
	if got := ResolveCallEffect(nil, 0, nil, nil, nil, nil); got != nil {
		t.Error("nil call info should return nil effect")
	}
}

func TestResolveCallEffect_SynthFallback(t *testing.T) {
	info := &cfg.CallInfo{
		Callee: &ast.IdentExpr{Value: "fn"},
	}
	synth := func(expr ast.Expr, p cfg.Point) typ.Type {
		return typ.Func().Effects(effect.WithModuleLoad()).Build()
	}
	if got := ResolveCallEffect(info, 0, synth, nil, nil, nil); got == nil {
		t.Error("expected effect from synthesized function type")
	}
}

func TestCallTerminates_UsesSynth(t *testing.T) {
	info := &cfg.CallInfo{
		Callee: &ast.IdentExpr{Value: "fn"},
	}
	synth := func(expr ast.Expr, p cfg.Point) typ.Type {
		return typ.Func().Returns(typ.Never).Build()
	}
	if !CallTerminates(info, 0, synth, nil, nil, nil) {
		t.Error("expected terminates for never-returning function")
	}
}

func TestCallTerminates_NormalFunction(t *testing.T) {
	info := &cfg.CallInfo{
		Callee: &ast.IdentExpr{Value: "fn"},
	}
	synth := func(expr ast.Expr, p cfg.Point) typ.Type {
		return typ.Func().Returns(typ.String).Build()
	}
	if CallTerminates(info, 0, synth, nil, nil, nil) {
		t.Error("expected non-terminating function")
	}
}
