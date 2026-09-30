package nested

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/typ"
)

func TestCollectCapturedFieldAssignments_NilGraph(t *testing.T) {
	result := CollectCapturedFieldAssignments(nil, nil, nil)
	if result == nil {
		t.Error("expected empty map, got nil")
	}
	if len(result) != 0 {
		t.Errorf("expected empty map, got %v", result)
	}
}

func TestCapturedSlotComparisonUsesSemanticAliasIdentity(t *testing.T) {
	union := func() typ.Type { return typ.NewUnion(typ.LiteralString("ok"), typ.LiteralString("failed")) }
	for _, tc := range []struct {
		name           string
		current, bound typ.Type
		wantSame       bool
	}{
		{"equal_allocations", union(), union(), true},
		{"same_alias", typ.NewAlias("Outcome", union()), typ.NewAlias("Outcome", union()), true},
		{"restore_alias", union(), typ.NewAlias("Outcome", union()), false},
		{"different_alias", typ.NewAlias("Other", union()), typ.NewAlias("Outcome", union()), false},
		{"widen_domain", typ.LiteralString("ok"), typ.NewAlias("Outcome", union()), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			current := typ.NewRecord().Field("outcome", tc.current).Build()
			bound := typ.NewRecord().Field("outcome", tc.bound).Build()
			got := NormalizeCapturedTableType(current, cfg.TableMutation{Escaped: true}, bound)
			if tc.wantSame && got != current {
				t.Fatal("semantically identical bound rebuilds capture")
			}
			if !tc.wantSame {
				field := got.(*typ.Record).GetField("outcome")
				alias, ok := field.Type.(*typ.Alias)
				if !ok || alias.Name != "Outcome" {
					t.Fatalf("lost alias bound: %v", field.Type)
				}
			}
		})
	}
}

func TestCollectCapturedFieldAssignments_EmptyCapturedSyms(t *testing.T) {
	result := CollectCapturedFieldAssignments(&cfg.Graph{}, map[cfg.SymbolID]bool{}, nil)
	if result == nil {
		t.Error("expected empty map, got nil")
	}
	if len(result) != 0 {
		t.Errorf("expected empty map, got %v", result)
	}
}

func TestEnrichSelfTypeWithConstructorFields_NilInputs(t *testing.T) {
	result := EnrichSelfTypeWithConstructorFields(nil, 0, nil)
	if result != nil {
		t.Error("expected nil for nil inputs")
	}
}

func TestEnrichSelfTypeWithConstructorFields_NilSelfType(t *testing.T) {
	result := EnrichSelfTypeWithConstructorFields(nil, 1, nil)
	if result != nil {
		t.Error("expected nil for nil selfType")
	}
}

func TestMergeFieldsIntoSelfType_EmptyFields(t *testing.T) {
	selfType := typ.Number
	result := mergeFieldsIntoSelfType(selfType, nil)
	if result != selfType {
		t.Errorf("expected original selfType for empty fields, got %v", result)
	}
}

func TestMergeFieldsIntoSelfType_NonRecordNonInterface(t *testing.T) {
	selfType := typ.Number
	fields := map[string]typ.Type{"x": typ.String}
	result := mergeFieldsIntoSelfType(selfType, fields)
	if result != selfType {
		t.Errorf("expected original selfType for non-record/interface, got %v", result)
	}
}

func TestCollectCapturedContainerMutations_AssignmentCallSite(t *testing.T) {
	code := `
		local c = {}
		local _ = send(c, 1)
	`
	stmts, err := parse.ParseString(code, "test.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{HasVargs: true},
		Stmts:   stmts,
	}
	graph := cfg.Build(fn, "send")
	if graph == nil {
		t.Fatal("expected graph")
	}
	symC, ok := graph.SymbolAt(graph.Exit(), "c")
	if !ok || symC == 0 {
		t.Fatal("expected symbol for c")
	}

	captured := map[cfg.SymbolID]bool{symC: true}
	result := CollectCapturedContainerMutations(graph, captured, nestedContainerSendSynth())
	muts := result[symC]
	if len(muts) != 1 {
		t.Fatalf("expected 1 container mutation for c, got %d", len(muts))
	}
	if !typ.TypeEquals(muts[0].ValueType, typ.Integer) {
		t.Fatalf("expected integer mutation value, got %v", muts[0].ValueType)
	}
}

func nestedContainerSendSynth() func(ast.Expr, cfg.Point) typ.Type {
	spec := contract.NewSpec().WithEffects(effect.Mutate{
		Target: effect.ParamRef{Index: 0},
		Transform: effect.ContainerElementUnion{
			Container: effect.ParamRef{Index: 0},
			Value:     effect.ParamRef{Index: 1},
		},
	})
	send := typ.Func().
		Param("container", typ.Any).
		Param("value", typ.Any).
		Returns(typ.Nil).
		Spec(spec).
		Build()

	return func(expr ast.Expr, _ cfg.Point) typ.Type {
		switch v := expr.(type) {
		case *ast.IdentExpr:
			if v.Value == "send" {
				return send
			}
		case *ast.NumberExpr:
			return typ.Integer
		}
		return typ.Unknown
	}
}
