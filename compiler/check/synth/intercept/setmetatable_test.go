package intercept

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/domain/metatable"
	"github.com/wippyai/go-lua/types/kind"
	querycore "github.com/wippyai/go-lua/types/query/core"
	"github.com/wippyai/go-lua/types/typ"
)

func TestSetMetatableIntercept_AttachesMetatableToReturnedRecord(t *testing.T) {
	table := typ.NewRecord().Field("nodes", typ.NewMap(typ.String, typ.Any)).Build()
	method := typ.Func().Param("self", typ.Any).Returns(typ.Boolean).Build()
	prototype := typ.NewRecord().Field("has_cycles", method).Build()
	meta := typ.NewRecord().Field("__index", prototype).Build()

	ex := &ast.FuncCallExpr{
		Func: &ast.IdentExpr{Value: "setmetatable"},
		Args: []ast.Expr{
			&ast.IdentExpr{Value: "table"},
			&ast.IdentExpr{Value: "meta"},
		},
	}
	result := (&SetMetatableIntercept{}).InterceptCall(ex, CallEnv{
		Recurse: func(expr ast.Expr) typ.Type {
			if ident, ok := expr.(*ast.IdentExpr); ok && ident.Value == "meta" {
				return meta
			}
			return table
		},
	})

	if !result.Skip || len(result.Types) != 1 {
		t.Fatalf("expected intercepted single return, got %#v", result)
	}
	if _, ok := querycore.Method(result.Types[0], "has_cycles"); !ok {
		t.Fatalf("expected returned record to expose metatable method, got %s", typ.FormatShort(result.Types[0]))
	}
}

func TestSetMetatableIntercept_OptionalMetatableKeepsNilVariantSound(t *testing.T) {
	table := typ.NewRecord().Field("nodes", typ.NewMap(typ.String, typ.Any)).Build()
	method := typ.Func().Param("self", typ.Any).Returns(typ.Boolean).Build()
	prototype := typ.NewRecord().Field("has_cycles", method).Build()
	meta := typ.NewRecord().Field("__index", prototype).Build()

	got := metatable.With(table, typ.NewOptional(meta))
	union, ok := got.(*typ.Union)
	if !ok || len(union.Members) != 2 {
		t.Fatalf("expected optional metatable to produce two variants, got %s", typ.FormatShort(got))
	}

	hasPlain := false
	hasMeta := false
	for _, member := range union.Members {
		rec, ok := member.(*typ.Record)
		if !ok {
			t.Fatalf("expected record variants, got %T", member)
		}
		if rec.Metatable == nil {
			hasPlain = true
			continue
		}
		if _, ok := querycore.Method(rec, "has_cycles"); ok {
			hasMeta = true
		}
	}
	if !hasPlain || !hasMeta {
		t.Fatalf("expected plain and metatabled variants, got %s", typ.FormatShort(got))
	}
	if _, ok := querycore.Method(got, "has_cycles"); ok {
		t.Fatal("optional metatable must not prove method exists on all variants")
	}
}

func TestSetMetatableIntercept_RemovesMetatableForNil(t *testing.T) {
	method := typ.Func().Param("self", typ.Any).Returns(typ.Boolean).Build()
	meta := typ.NewRecord().Field("has_cycles", method).Build()
	table := typ.NewRecord().Metatable(meta).Build()

	got := metatable.With(table, typ.Nil)
	rec, ok := got.(*typ.Record)
	if !ok {
		t.Fatalf("expected record, got %T", got)
	}
	if rec.Metatable != nil {
		t.Fatalf("expected nil metatable to remove metatable, got %s", typ.FormatShort(rec.Metatable))
	}
	if got.Kind() == kind.Never {
		t.Fatal("setmetatable nil removal should not produce never")
	}
}
