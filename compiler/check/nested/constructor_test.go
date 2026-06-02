package nested

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	interprocdomain "github.com/wippyai/go-lua/compiler/check/domain/interproc"
	"github.com/wippyai/go-lua/types/typ"
)

func TestDetectConstructorPattern_NilInputs(t *testing.T) {
	classSym, selfSym := DetectConstructorPattern(api.FlowEvidence{}, api.FlowEvidence{}, nil, nil, nil)
	if classSym != 0 || selfSym != 0 {
		t.Errorf("expected (0, 0) for nil inputs, got (%d, %d)", classSym, selfSym)
	}
}

func TestDetectConstructorPattern_NilGraph(t *testing.T) {
	classSym, selfSym := DetectConstructorPattern(api.FlowEvidence{}, api.FlowEvidence{}, nil, nil, nil)
	if classSym != 0 || selfSym != 0 {
		t.Errorf("expected (0, 0) for nil nestedGraph, got (%d, %d)", classSym, selfSym)
	}
}

func TestFindSetmetatablePatternByName_NilGraph(t *testing.T) {
	result := findSetmetatablePatternByName(nil, "Test")
	if result != 0 {
		t.Errorf("expected 0 for nil graph, got %d", result)
	}
}

func TestIsSymbolReturned_NilGraph(t *testing.T) {
	result := isSymbolReturned(nil, 1)
	if result {
		t.Error("expected false for nil graph")
	}
}

func TestIsSymbolReturned_ZeroSymbol(t *testing.T) {
	result := isSymbolReturned(nil, 0)
	if result {
		t.Error("expected false for zero symbol")
	}
}

func TestCollectConstructorFields_NilInputs(t *testing.T) {
	result := CollectConstructorFields(nil, 0, nil)
	if result != nil {
		t.Errorf("expected nil for nil graph and zero symbol, got %v", result)
	}
}

func TestCollectConstructorFields_ZeroSymbol(t *testing.T) {
	result := CollectConstructorFields(nil, 0, nil)
	if result != nil {
		t.Errorf("expected nil for zero symbol, got %v", result)
	}
}

func TestCollectConstructorFields_NilEvidence(t *testing.T) {
	result := CollectConstructorFields(nil, cfg.SymbolID(1), nil)
	if result != nil {
		t.Errorf("expected nil for nil evidence, got %v", result)
	}
}

func TestCollectConstructorLiteralFields_ReturnsTypedCarrier(t *testing.T) {
	table := &ast.TableExpr{Fields: []*ast.Field{
		{Key: &ast.StringExpr{Value: "name"}, Value: &ast.StringExpr{Value: "wolf"}},
		{Key: &ast.StringExpr{Value: ""}, Value: &ast.StringExpr{Value: "skip"}},
	}}

	result := CollectConstructorLiteralFields(table, 0, func(ast.Expr, cfg.Point) typ.Type {
		return typ.String
	})
	projected := interprocdomain.ProjectValueFieldMap(result)
	if !typ.TypeEquals(projected["name"], typ.String) {
		t.Fatalf("expected typed field carrier with name:string, got %v", projected)
	}
	if !typ.TypeEquals(projected[""], typ.String) {
		t.Fatalf("expected empty bracket-string key to survive, got %v", projected)
	}
}

func TestMergeConstructorFieldMaps_JoinsTypedCarrier(t *testing.T) {
	left := interprocdomain.LiftTypeFieldMap(map[string]typ.Type{"x": typ.Number})
	right := interprocdomain.LiftTypeFieldMap(map[string]typ.Type{"x": typ.String, "y": typ.Boolean})

	result := MergeConstructorFieldMaps(left, right)
	projected := interprocdomain.ProjectValueFieldMap(result)
	if !typ.TypeEquals(projected["x"], typ.NewUnion(typ.Number, typ.String)) {
		t.Fatalf("expected merged x field, got %v", projected["x"])
	}
	if !typ.TypeEquals(projected["y"], typ.Boolean) {
		t.Fatalf("expected y:boolean, got %v", projected["y"])
	}
}
