package modules

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
)

func TestAliasesFromAssignments_NilEvidence(t *testing.T) {
	aliases := AliasesFromAssignments(nil, nil)
	if aliases != nil {
		t.Errorf("expected nil for nil evidence, got %v", aliases)
	}
}

func TestAliasesFromAssignments_EmptyGraph(t *testing.T) {
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{},
	}
	graph := cfg.Build(fn)
	result := AliasesFromAssignments(nil, graph)
	if result != nil {
		t.Error("expected nil for empty graph")
	}
}
