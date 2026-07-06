package transferfacts

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/compiler/ast"
)

func TestNewSourceExprRefKeyCopiesScopedSourceFields(t *testing.T) {
	expr := &ast.FuncCallExpr{Func: ident("make")}
	source := sourceprovenance.ASTSource{
		Kind:         sourceprovenance.SourceCall,
		Expr:         expr,
		ExprIndex:    1,
		TargetIndex:  2,
		ResultIndex:  3,
		CallPoint:    cfg.Point(42),
		HasCallPoint: true,
		Final:        true,
		Expanded:     true,
		Adjusted:     false,
		OpenTail:     true,
	}

	key := newSourceExprRefKey(source)
	if key.expr != expr ||
		key.kind != source.Kind ||
		key.exprIndex != source.ExprIndex ||
		key.targetIndex != source.TargetIndex ||
		key.resultIndex != source.ResultIndex ||
		key.callPoint != source.CallPoint ||
		key.hasCallPoint != source.HasCallPoint ||
		key.final != source.Final ||
		key.expanded != source.Expanded ||
		key.adjusted != source.Adjusted ||
		key.openTail != source.OpenTail {
		t.Fatalf("source expr-ref key mismatch: %#v from %#v", key, source)
	}
}
