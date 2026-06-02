package facts

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/constraint"
)

func TestFieldFuncDefinitionKeepsStringIndexKeyStructural(t *testing.T) {
	fn := &ast.FunctionExpr{}
	info := &cfg.FuncDefInfo{
		FuncExpr: fn,
		TargetPath: constraint.Path{
			Symbol: cfg.SymbolID(3),
			Segments: []constraint.Segment{
				{Kind: constraint.SegmentIndexString, Name: "x-y"},
			},
		},
	}

	base, key, ok := fieldFuncDefinition(info)
	if !ok {
		t.Fatal("fieldFuncDefinition rejected static string-index function")
	}
	if base != cfg.SymbolID(3) {
		t.Fatalf("base = %d, want 3", base)
	}
	if key.Kind != constraint.SegmentIndexString || key.Name != "x-y" {
		t.Fatalf("key = %#v, want string-index x-y", key)
	}
}

func TestTableLiteralFieldKeyKeepsFieldSyntaxCompatibility(t *testing.T) {
	field, ok := tableLiteralFieldKey(&ast.Field{Key: &ast.StringExpr{Value: "handler"}})
	if !ok {
		t.Fatal("string table key rejected")
	}
	if field.Kind != constraint.SegmentField || field.Name != "handler" {
		t.Fatalf("legacy string key = %#v, want field handler", field)
	}
}

func TestTableLiteralFieldKeyKeepsBracketStringStructural(t *testing.T) {
	field, ok := tableLiteralFieldKey(&ast.Field{
		Key:       &ast.StringExpr{Value: "handler"},
		KeySyntax: ast.AttrKeyIndex,
	})
	if !ok {
		t.Fatal("bracket string table key rejected")
	}
	if field.Kind != constraint.SegmentIndexString || field.Name != "handler" {
		t.Fatalf("bracket string key = %#v, want string-index handler", field)
	}
}
