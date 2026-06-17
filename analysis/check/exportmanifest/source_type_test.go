package exportmanifest

import (
	"testing"

	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/compiler/ast"
)

func TestValueSourceFromASTSourcePreservesVarargBoundarySource(t *testing.T) {
	shape, ok := sourceprovenance.NewSourceShape(true, true, false, true)
	if !ok {
		t.Fatal("vararg shape rejected")
	}
	source, ok := sourceprovenance.NewVarargSource(&ast.Comma3Expr{}, 0, 2, 1, shape)
	if !ok {
		t.Fatal("AST vararg source rejected")
	}

	got, ok := valueSourceFromASTSource(source)
	if !ok {
		t.Fatal("valueSourceFromASTSource returned false")
	}
	if !got.Valid() {
		t.Fatalf("lowered vararg source is invalid: %#v", got)
	}
	if got.Kind != factflow.ValueSourceVararg || got.HasExpr || got.ExprRef != 0 || got.TargetIndex != 2 || got.ResultIndex != 1 || !got.OpenTail {
		t.Fatalf("lowered vararg source = %#v, want boundary vararg without expr ref", got)
	}
}

func TestValueSourceFromASTSourceRejectsMalformedSources(t *testing.T) {
	nilSource := sourceprovenance.NewNilSource(0)
	nilSource.Final = true
	if nilSource.Valid() {
		t.Fatalf("malformed nil source unexpectedly valid: %#v", nilSource)
	}
	if got, ok := valueSourceFromASTSource(nilSource); ok {
		t.Fatalf("malformed nil source lowered to %#v, want false", got)
	}

	shape, ok := sourceprovenance.NewSourceShape(true, true, false, true)
	if !ok {
		t.Fatal("call shape rejected")
	}
	callSource, ok := sourceprovenance.NewCallSource(&ast.FuncCallExpr{}, 0, 0, 0, cfg.Point(42), shape)
	if !ok {
		t.Fatal("call source rejected")
	}
	callSource.HasCallPoint = false
	if callSource.Valid() {
		t.Fatalf("malformed call source unexpectedly valid: %#v", callSource)
	}
	if got, ok := valueSourceFromASTSource(callSource); ok {
		t.Fatalf("malformed call source lowered to %#v, want false", got)
	}
}
