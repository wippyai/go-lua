package mutator

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/typ"
)

func buildGraph(t *testing.T, code string, globals ...string) *cfg.Graph {
	t.Helper()
	stmts, err := parse.ParseString(code, "test.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	fn := &ast.FunctionExpr{ParList: &ast.ParList{HasVargs: true}, Stmts: stmts}
	graph := cfg.Build(fn, globals...)
	if graph == nil {
		t.Fatal("expected non-nil graph")
	}
	return graph
}

func tableInsertSynth() func(ast.Expr, cfg.Point) typ.Type {
	spec := contract.NewSpec().WithEffects(effect.TableMutator{
		Target: effect.ParamRef{Index: 0},
		Value:  effect.ParamRef{Index: 1},
	})
	tableInsert := typ.Func().
		Param("target", typ.Any).
		Param("value", typ.Any).
		Returns(typ.Nil).
		Spec(spec).
		Build()

	return func(expr ast.Expr, _ cfg.Point) typ.Type {
		switch v := expr.(type) {
		case *ast.AttrGetExpr:
			obj, ok := v.Object.(*ast.IdentExpr)
			if !ok || obj.Value != "table" {
				return typ.Unknown
			}
			switch key := v.Key.(type) {
			case *ast.IdentExpr:
				if key.Value == "insert" {
					return tableInsert
				}
			case *ast.StringExpr:
				if key.Value == "insert" {
					return tableInsert
				}
			}
		case *ast.NumberExpr:
			return typ.Integer
		case *ast.IdentExpr:
			if v.Value == "k" {
				return typ.String
			}
		}
		return typ.Unknown
	}
}
