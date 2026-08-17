package function

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
)

func TestFunctionWriterStartsCleanAndPreservesSourceCoordinates(t *testing.T) {
	w := New(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "function.lua")
	if !w.Clean() {
		t.Fatal("new function writer retained private continuation")
	}

	identifier := &ast.IdentExpr{}
	identifier.SetPosFromToken(ast.Position{Line: 2, Column: 3, EndLine: 2, EndColumn: 8})
	got := w.span(identifier)
	if got.File != "function.lua" || got.StartLine != 2 || got.StartCol != 3 || got.EndLine != 2 || got.EndCol != 8 {
		t.Fatalf("span = %#v, want function.lua:2:3-2:8", got)
	}

	local := &ast.LocalAssignStmt{NamePositions: []ast.Position{{Line: 4, Column: 1, EndLine: 4, EndColumn: 5}}}
	name := w.nameSpan(local, 0)
	if name.StartLine != 4 || name.StartCol != 1 || name.EndCol != 5 {
		t.Fatalf("nameSpan = %#v, want authored local-name span", name)
	}
}
