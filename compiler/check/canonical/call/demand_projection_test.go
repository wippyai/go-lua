package call

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/domain/callobligation"
	"github.com/wippyai/go-lua/types/typ"
)

func TestCallArgDemandsForCallSummaryWins(t *testing.T) {
	t.Parallel()

	call := &ast.FuncCallExpr{Args: []ast.Expr{&ast.IdentExpr{Value: "arg"}}}
	fallbackUsed := false

	got := CallArgDemandsForCall(CallArgDemandsInput{
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
	})

	if len(got) != 1 || got[0].Source != callobligation.SourceBody || got[0].Type != typ.String {
		t.Fatalf("CallArgDemandsForCall summary = %#v, want string", got)
	}
	if fallbackUsed {
		t.Fatal("function fallback ran despite summary demands")
	}
}

func TestCallArgDemandsForCallEmptySummaryHitBlocksFallback(t *testing.T) {
	t.Parallel()

	call := &ast.FuncCallExpr{Args: []ast.Expr{&ast.IdentExpr{Value: "arg"}}}
	fallbackUsed := false

	got := CallArgDemandsForCall(CallArgDemandsInput{
		Call: call,
		SummaryDemands: func(*ast.FuncCallExpr) ([]callobligation.Obligation, bool) {
			return nil, true
		},
		FunctionShape: func(*ast.FuncCallExpr) *typ.Function {
			fallbackUsed = true
			return typ.Func().Param("x", typ.Number).Build()
		},
	})

	if got != nil {
		t.Fatalf("CallArgDemandsForCall empty summary hit = %#v, want nil", got)
	}
	if fallbackUsed {
		t.Fatal("function fallback ran despite authoritative empty summary hit")
	}
}

func TestCallArgDemandsForCallFunctionFallback(t *testing.T) {
	t.Parallel()

	call := &ast.FuncCallExpr{Args: []ast.Expr{&ast.IdentExpr{Value: "arg"}}}

	got := CallArgDemandsForCall(CallArgDemandsInput{
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
	})

	if len(got) != 1 || got[0].Source != callobligation.SourceSignature || got[0].Type != typ.Number {
		t.Fatalf("CallArgDemandsForCall fallback = %#v, want number", got)
	}
}

func TestCallArgDemandsForCallSubstitutesSelfFromReceiver(t *testing.T) {
	t.Parallel()

	timeType := typ.NewInterface("time.Time", nil)
	call := &ast.FuncCallExpr{
		Receiver: &ast.IdentExpr{Value: "now"},
		Method:   "sub",
		Args:     []ast.Expr{&ast.IdentExpr{Value: "other"}},
	}

	got := CallArgDemandsForCall(CallArgDemandsInput{
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
	})

	if len(got) != 1 || got[0].Source != callobligation.SourceSignature || !typ.TypeEquals(got[0].Type, timeType) {
		t.Fatalf("CallArgDemandsForCall self substitution = %#v, want time.Time signature", got)
	}
}

func TestCallArgDemandsForCallEmptyInputSkipsProviders(t *testing.T) {
	t.Parallel()

	providerUsed := false
	got := CallArgDemandsForCall(CallArgDemandsInput{
		Call: &ast.FuncCallExpr{},
		SummaryDemands: func(*ast.FuncCallExpr) ([]callobligation.Obligation, bool) {
			providerUsed = true
			return []callobligation.Obligation{callobligation.Body(typ.String)}, true
		},
		FunctionShape: func(*ast.FuncCallExpr) *typ.Function {
			providerUsed = true
			return typ.Func().Param("x", typ.Number).Build()
		},
	})

	if got != nil {
		t.Fatalf("CallArgDemandsForCall no args = %#v, want nil", got)
	}
	if providerUsed {
		t.Fatal("providers ran for call without arguments")
	}
}
