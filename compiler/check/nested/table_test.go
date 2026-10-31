package nested

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
)

func TestMethodOwner_NilInputs(t *testing.T) {
	if got := MethodOwner(nil, nil, nil, 0); got != 0 {
		t.Errorf("expected no owner for nil inputs, got %d", got)
	}
	if got := MethodOwner(&cfg.Graph{}, nil, nil, 1); got != 0 {
		t.Errorf("expected no owner without a function, got %d", got)
	}
	if got := MethodOwner(nil, &ast.FunctionExpr{}, nil, 0); got != 0 {
		t.Errorf("expected no owner without a graph, got %d", got)
	}
}
