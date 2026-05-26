package paramevidence

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/typ"
)

type staticExpressionObserver map[ast.Expr]typ.Type

func (o staticExpressionObserver) TypeOf(expr ast.Expr, _ cfg.Point) typ.Type {
	return o[expr]
}

func TestCallEntryProjector_EntryEvidenceUsesSolvedArgumentObservation(t *testing.T) {
	arg := &ast.StringExpr{Value: "raw"}
	calleeSym := cfg.SymbolID(10)
	projector := NewCallEntryProjector(CallEntryConfig{
		Observer: staticExpressionObserver{arg: typ.Any},
		ArgumentObservation: func(_ cfg.Point, got ast.Expr, current typ.Type) typ.Type {
			if got != arg || !typ.TypeEquals(current, typ.Any) {
				t.Fatalf("ArgumentObservation got expr=%T current=%v, want arg any", got, current)
			}
			return typ.String
		},
	})

	entries := projector.EntryEvidence(1, api.CallEvidence{
		Point: 1,
		Info:  &cfg.CallInfo{Args: []ast.Expr{arg}},
	}, calleeSym)
	got := entries[calleeSym]
	if len(got) != 1 || !typ.TypeEquals(got[0], typ.String) {
		t.Fatalf("entry evidence = %v, want [string]", got)
	}
}

func TestCallEntryProjector_EntryEvidencePreservesArrayElementDiscriminants(t *testing.T) {
	arg := &ast.TableExpr{}
	calleeSym := cfg.SymbolID(13)
	functionResult := typ.NewRecord().
		Field("role", typ.LiteralString("function_result")).
		Field("function_call_id", typ.LiteralString("tool")).
		Build()
	developer := typ.NewRecord().
		Field("role", typ.LiteralString("developer")).
		Field("content", typ.LiteralString("merge")).
		Build()
	observed := typ.NewTuple(functionResult, developer)
	projector := NewCallEntryProjector(CallEntryConfig{
		Observer: staticExpressionObserver{arg: observed},
	})

	entries := projector.EntryEvidence(1, api.CallEvidence{
		Point: 1,
		Info:  &cfg.CallInfo{Args: []ast.Expr{arg}},
	}, calleeSym)
	got := entries[calleeSym]
	if len(got) != 1 || !typ.TypeEquals(got[0], observed) {
		t.Fatalf("entry evidence = %v, want %v", got, observed)
	}
}

func TestCallEntryProjector_MethodEntryEvidenceIncludesReceiver(t *testing.T) {
	receiver := &ast.IdentExpr{Value: "obj"}
	arg := &ast.StringExpr{Value: "name"}
	calleeSym := cfg.SymbolID(11)
	projector := NewCallEntryProjector(CallEntryConfig{
		Observer: staticExpressionObserver{
			receiver: typ.Number,
			arg:      typ.String,
		},
	})

	entries := projector.EntryEvidence(1, api.CallEvidence{
		Point: 1,
		Info: &cfg.CallInfo{
			Method:   "set",
			Receiver: receiver,
			Args:     []ast.Expr{arg},
		},
	}, calleeSym)
	got := entries[calleeSym]
	if len(got) != 2 {
		t.Fatalf("entry evidence len = %d, want 2: %v", len(got), got)
	}
	if !typ.TypeEquals(got[0], typ.Number) || !typ.TypeEquals(got[1], typ.String) {
		t.Fatalf("entry evidence = %v, want [number string]", got)
	}
}

func TestCallEntryProjector_EvidenceAllowedFiltersSlots(t *testing.T) {
	arg := &ast.StringExpr{Value: "name"}
	calleeSym := cfg.SymbolID(12)
	projector := NewCallEntryProjector(CallEntryConfig{
		Observer: staticExpressionObserver{arg: typ.String},
		EvidenceAllowed: func(sym cfg.SymbolID, idx int) bool {
			return sym != calleeSym || idx != 0
		},
	})

	entries := projector.EntryEvidence(1, api.CallEvidence{
		Point: 1,
		Info:  &cfg.CallInfo{Args: []ast.Expr{arg}},
	}, calleeSym)
	if got := entries[calleeSym]; len(got) != 0 {
		t.Fatalf("entry evidence = %v, want filtered empty slot", got)
	}
}
