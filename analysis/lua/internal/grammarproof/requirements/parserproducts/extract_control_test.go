package parserproducts

import (
	goast "go/ast"
	"go/parser"
	"go/token"
	"testing"
)

func TestControlExtractionFindsOneTopLevelTypeSwitch(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "action.go", `package p; func f() { switch value := result.(type) { case int: _ = value; default: _ = result } }`, 0)
	if err != nil {
		t.Fatal(err)
	}
	branch, index := topLevelTypeSwitch(file.Decls[0].(*goast.FuncDecl).Body)
	if branch == nil || index != 0 {
		t.Fatalf("top-level type switch = %p/%d, want first switch", branch, index)
	}
	if _, index := topLevelTypeSwitch(&goast.BlockStmt{}); index != -1 {
		t.Fatalf("empty block index = %d, want -1", index)
	}
}
