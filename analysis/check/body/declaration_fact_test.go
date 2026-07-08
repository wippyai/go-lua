package body

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/compiler/ast"
)

func TestBodyDeclarationFactsPreserveSourceMetadata(t *testing.T) {
	typeDef := &ast.TypeDefStmt{Name: "Alias"}
	interfaceDef := &ast.InterfaceDefStmt{Name: "Shape"}
	target := &ast.IdentExpr{Value: "f"}
	functionDef := &ast.FuncDefStmt{
		Name: &ast.FuncName{Func: target},
		Func: &ast.FunctionExpr{},
	}
	stmts := []ast.Stmt{typeDef, interfaceDef, functionDef}
	bindings := bind.BindChunk(stmts, bind.Options{})
	prepared, err := PrepareBoundChunk(stmts, bindings, Config{Registry: standard.Registry()})
	if err != nil {
		t.Fatalf("PrepareBoundChunk: %v", err)
	}
	result := solvePreparedForTest(t, prepared, SolveConfig{})

	var sawAlias, sawInterface, sawFunction bool
	for _, point := range result.Graph().RPO() {
		if fact, ok := result.TypeDefinition(point); ok {
			switch fact.Kind {
			case TypeDefinitionAlias:
				if fact.Type != typeDef || fact.Type.Name != "Alias" || fact.Stmt != typeDef {
					t.Fatalf("alias fact = %#v", fact)
				}
				sawAlias = true
			case TypeDefinitionInterface:
				if fact.Interface != interfaceDef || fact.Interface.Name != "Shape" || fact.Stmt != interfaceDef {
					t.Fatalf("interface fact = %#v", fact)
				}
				sawInterface = true
			}
		}
		if fact, ok := result.FunctionDefinition(point); ok {
			if fact.Stmt == nil || fact.Name == nil || fact.Func == nil {
				t.Fatalf("function fact missing source metadata: %#v", fact)
			}
			if !fact.HasTargetSymbol || fact.TargetSymbol == 0 || !fact.HasTargetPath {
				t.Fatalf("function fact missing target metadata: %#v", fact)
			}
			sawFunction = true
		}
	}
	if !sawAlias || !sawInterface || !sawFunction {
		t.Fatalf("declaration facts alias=%v interface=%v function=%v", sawAlias, sawInterface, sawFunction)
	}
}

func TestBodyDeclarationFactsSkipUnmappedDeadDeclarations(t *testing.T) {
	typeDef := &ast.TypeDefStmt{Name: "Dead"}
	functionDef := &ast.FuncDefStmt{
		Name: &ast.FuncName{Func: &ast.IdentExpr{Value: "dead"}},
		Func: &ast.FunctionExpr{},
	}
	stmts := []ast.Stmt{&ast.ReturnStmt{}, typeDef, functionDef}
	bindings := bind.BindChunk(stmts, bind.Options{})
	prepared, err := PrepareBoundChunk(stmts, bindings, Config{Registry: standard.Registry()})
	if err != nil {
		t.Fatalf("PrepareBoundChunk: %v", err)
	}
	result := solvePreparedForTest(t, prepared, SolveConfig{})

	for _, point := range result.Graph().RPO() {
		if point == cfg.Point(0) {
			continue
		}
		if fact, ok := result.TypeDefinition(point); ok {
			t.Fatalf("dead type declaration produced fact at %v: %#v", point, fact)
		}
		if fact, ok := result.FunctionDefinition(point); ok {
			t.Fatalf("dead function declaration produced fact at %v: %#v", point, fact)
		}
	}
}
