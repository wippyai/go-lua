package call

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/domain/callobligation"
	"github.com/wippyai/go-lua/types/typ"
)

func TestCallArgDemandProjectionSummaryWins(t *testing.T) {
	t.Parallel()

	call := &ast.FuncCallExpr{Args: []ast.Expr{&ast.IdentExpr{Value: "arg"}}}
	fallbackUsed := false

	got := (CallArgDemandProjection{
		Call: call,
		SummaryDemands: func(gotCall *ast.FuncCallExpr) ([]callobligation.Obligation, bool) {
			if gotCall != call {
				t.Fatalf("summary saw call %#v, want original", gotCall)
			}
			return []callobligation.Obligation{callobligation.Body(typ.String)}, true
		},
		FunctionShape: func(*ast.FuncCallExpr) *typ.Function {
			fallbackUsed = true
			return typ.Func().Param("x", typ.Number).Build()
		},
	}).Demands()

	if len(got) != 1 || got[0].Source != callobligation.SourceBody || got[0].Type != typ.String {
		t.Fatalf("CallArgDemandProjection summary = %#v, want string", got)
	}
	if fallbackUsed {
		t.Fatal("function fallback ran despite summary demands")
	}
}

func TestCallArgDemandProjectionEmptySummaryHitBlocksFallback(t *testing.T) {
	t.Parallel()

	call := &ast.FuncCallExpr{Args: []ast.Expr{&ast.IdentExpr{Value: "arg"}}}
	fallbackUsed := false

	got := (CallArgDemandProjection{
		Call: call,
		SummaryDemands: func(*ast.FuncCallExpr) ([]callobligation.Obligation, bool) {
			return nil, true
		},
		FunctionShape: func(*ast.FuncCallExpr) *typ.Function {
			fallbackUsed = true
			return typ.Func().Param("x", typ.Number).Build()
		},
	}).Demands()

	if got != nil {
		t.Fatalf("CallArgDemandProjection empty summary hit = %#v, want nil", got)
	}
	if fallbackUsed {
		t.Fatal("function fallback ran despite authoritative empty summary hit")
	}
}

func TestCallArgDemandProjectionFunctionFallback(t *testing.T) {
	t.Parallel()

	call := &ast.FuncCallExpr{Args: []ast.Expr{&ast.IdentExpr{Value: "arg"}}}

	got := (CallArgDemandProjection{
		Call: call,
		SummaryDemands: func(*ast.FuncCallExpr) ([]callobligation.Obligation, bool) {
			return nil, false
		},
		FunctionShape: func(gotCall *ast.FuncCallExpr) *typ.Function {
			if gotCall != call {
				t.Fatalf("function fallback saw call %#v, want original", gotCall)
			}
			return typ.Func().Param("x", typ.Number).Build()
		},
	}).Demands()

	if len(got) != 1 || got[0].Source != callobligation.SourceSignature || got[0].Type != typ.Number {
		t.Fatalf("CallArgDemandProjection fallback = %#v, want number", got)
	}
}

func TestCallArgDemandProjectionSubstitutesSelfFromReceiver(t *testing.T) {
	t.Parallel()

	timeType := typ.NewInterface("time.Time", nil)
	call := &ast.FuncCallExpr{
		Receiver: &ast.IdentExpr{Value: "now"},
		Method:   "sub",
		Args:     []ast.Expr{&ast.IdentExpr{Value: "other"}},
	}

	got := (CallArgDemandProjection{
		Call: call,
		FunctionShape: func(*ast.FuncCallExpr) *typ.Function {
			return typ.Func().
				Param("self", typ.Self).
				Param("other", typ.Self).
				Build()
		},
		SelfType: func(*ast.FuncCallExpr) typ.Type {
			return timeType
		},
	}).Demands()

	if len(got) != 1 || got[0].Source != callobligation.SourceSignature || !typ.TypeEquals(got[0].Type, timeType) {
		t.Fatalf("CallArgDemandProjection self substitution = %#v, want time.Time signature", got)
	}
}

func TestCallArgDemandProjectionEmptyInputSkipsProviders(t *testing.T) {
	t.Parallel()

	providerUsed := false
	got := (CallArgDemandProjection{
		Call: &ast.FuncCallExpr{},
		SummaryDemands: func(*ast.FuncCallExpr) ([]callobligation.Obligation, bool) {
			providerUsed = true
			return []callobligation.Obligation{callobligation.Body(typ.String)}, true
		},
		FunctionShape: func(*ast.FuncCallExpr) *typ.Function {
			providerUsed = true
			return typ.Func().Param("x", typ.Number).Build()
		},
	}).Demands()

	if got != nil {
		t.Fatalf("CallArgDemandProjection no args = %#v, want nil", got)
	}
	if providerUsed {
		t.Fatal("providers ran for call without arguments")
	}
}
