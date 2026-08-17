package static

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
)

func TestStaticDeclarationWalkRejectsInvalidCursorsAndMembers(t *testing.T) {
	w := Writer{}
	if err := w.runAliasConstraints(walkStep{alias: &ast.TypeDefStmt{}, index: -1}); err == nil {
		t.Fatal("runAliasConstraints accepted a negative type-parameter cursor")
	}
	if err := w.runInterfaceExtends(walkStep{iface: &ast.InterfaceDefStmt{Extends: []*ast.TypeRefExpr{nil}}, index: 0}); err == nil {
		t.Fatal("runInterfaceExtends accepted an absent interface extension")
	}
	if err := w.runInterfaceMembers(walkStep{iface: &ast.InterfaceDefStmt{Members: []ast.InterfaceMember{{}}}, index: 0}); err == nil {
		t.Fatal("runInterfaceMembers accepted an invalid member kind")
	}
}
