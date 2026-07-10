package sourcebridge

import (
	"testing"

	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/compiler/ast"
)

func TestValueSourceFromASTSourceLowersBoundarySources(t *testing.T) {
	shape, ok := sourceprovenance.NewSourceShape(true, true, false, true)
	if !ok {
		t.Fatal("source shape rejected")
	}
	callSource, ok := sourceprovenance.NewCallSource(&ast.FuncCallExpr{}, 1, 2, 3, cfg.Point(42), shape)
	if !ok {
		t.Fatal("call source rejected")
	}
	varargSource, ok := sourceprovenance.NewVarargSource(&ast.Comma3Expr{}, 4, 5, 6, shape)
	if !ok {
		t.Fatal("vararg source rejected")
	}

	cases := []struct {
		name   string
		source sourceprovenance.ASTSource
		check  func(t *testing.T, got factflow.ValueSource)
	}{
		{
			name:   "call",
			source: callSource,
			check: func(t *testing.T, got factflow.ValueSource) {
				if got.Kind != factflow.ValueSourceCall || got.HasExpr || got.ExprRef != 0 ||
					got.ExprIndex != 1 || got.TargetIndex != 2 || got.ResultIndex != 3 ||
					!got.HasCallPoint || got.CallPoint != cfg.Point(42) || !got.OpenTail {
					t.Fatalf("call source = %#v, want boundary call without expr ref", got)
				}
			},
		},
		{
			name:   "vararg",
			source: varargSource,
			check: func(t *testing.T, got factflow.ValueSource) {
				if got.Kind != factflow.ValueSourceVararg || got.HasExpr || got.ExprRef != 0 ||
					got.ExprIndex != 4 || got.TargetIndex != 5 || got.ResultIndex != 6 ||
					got.HasCallPoint || got.CallPoint != 0 || !got.OpenTail {
					t.Fatalf("vararg source = %#v, want boundary vararg without expr ref", got)
				}
			},
		},
		{
			name:   "nil",
			source: sourceprovenance.NewNilSource(7),
			check: func(t *testing.T, got factflow.ValueSource) {
				if got.Kind != factflow.ValueSourceNil || got.TargetIndex != 7 || !got.Valid() {
					t.Fatalf("nil source = %#v, want valid nil target source", got)
				}
			},
		},
		{
			name:   "unknown",
			source: sourceprovenance.NewUnknownSource(8),
			check: func(t *testing.T, got factflow.ValueSource) {
				if got.Kind != factflow.ValueSourceUnknown || got.TargetIndex != 8 || !got.Valid() {
					t.Fatalf("unknown source = %#v, want valid unknown target source", got)
				}
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ValueSourceFromASTSource(tc.source)
			if !ok {
				t.Fatal("ValueSourceFromASTSource returned false")
			}
			if !got.Valid() {
				t.Fatalf("lowered source is invalid: %#v", got)
			}
			tc.check(t, got)
		})
	}
}

func TestValueSourceFromASTSourceRejectsMalformedSources(t *testing.T) {
	nilSource := sourceprovenance.NewNilSource(0)
	nilSource.Final = true
	if nilSource.Valid() {
		t.Fatalf("malformed nil source unexpectedly valid: %#v", nilSource)
	}
	if got, ok := ValueSourceFromASTSource(nilSource); ok {
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
	if got, ok := ValueSourceFromASTSource(callSource); ok {
		t.Fatalf("malformed call source lowered to %#v, want false", got)
	}

	exprSource, ok := sourceprovenance.NewExpressionSource(&ast.NilExpr{}, 0, 0, 0, sourceprovenance.SourceShape{})
	if !ok {
		t.Fatal("expression source rejected")
	}
	if got, ok := ValueSourceFromASTSource(exprSource); ok {
		t.Fatalf("expression source lowered to %#v, want false", got)
	}
}
