package artifact

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow"
)

// TestReceiptGeometryAccessorsDoNotExportFlowTypes is the closed receipt law:
// HeapAllocationRow.Role/Form and CallRow.Form are artifact columns.
func TestReceiptGeometryAccessorsDoNotExportFlowTypes(t *testing.T) {
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("artifact source location unavailable")
	}
	root := filepath.Dir(current)
	set := token.NewFileSet()
	pkgs, err := parser.ParseDir(set, root, nil, 0)
	if err != nil {
		t.Fatalf("parse artifact: %v", err)
	}
	want := map[string]string{
		"HeapAllocationRow.Role": "AllocationRole",
		"HeapAllocationRow.Form": "AllocationForm",
		"CallRow.Form":           "CallForm",
	}
	found := make(map[string]string)
	for _, pkg := range pkgs {
		for path, file := range pkg.Files {
			if filepath.Base(path) == "geometry_law_test.go" {
				continue
			}
			for _, decl := range file.Decls {
				fn, isFn := decl.(*ast.FuncDecl)
				if !isFn || fn.Recv == nil || fn.Type.Results == nil || len(fn.Type.Results.List) == 0 {
					continue
				}
				recv := receiverTypeName(fn)
				if recv != "HeapAllocationRow" && recv != "CallRow" {
					continue
				}
				if fn.Name.Name != "Role" && fn.Name.Name != "Form" {
					continue
				}
				found[recv+"."+fn.Name.Name] = resultTypeName(fn.Type.Results.List[0].Type)
			}
		}
	}
	for key, wantType := range want {
		got := found[key]
		if got != wantType {
			t.Errorf("%s returns %q, want receipt column %q", key, got, wantType)
		}
	}
}

func receiverTypeName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return ""
	}
	expr := fn.Recv.List[0].Type
	if star, isStar := expr.(*ast.StarExpr); isStar {
		expr = star.X
	}
	ident, isIdent := expr.(*ast.Ident)
	if !isIdent {
		return ""
	}
	return ident.Name
}

func resultTypeName(expr ast.Expr) string {
	switch typed := expr.(type) {
	case *ast.Ident:
		return typed.Name
	case *ast.SelectorExpr:
		if ident, isIdent := typed.X.(*ast.Ident); isIdent {
			return ident.Name + "." + typed.Sel.Name
		}
	}
	return ""
}

func TestReceiptGeometryPublishesFlowOrdinals(t *testing.T) {
	role, roleOK := receiptAllocationRole(flow.AllocationTable)
	form, formOK := receiptAllocationForm(flow.AllocationFormClosed)
	call, callOK := receiptCallForm(flow.CallFormMethod)
	if !roleOK || !formOK || !callOK {
		t.Fatal("receipt geometry refused a valid Flow constructor")
	}
	if uint64(role) != uint64(flow.AllocationTable) || uint64(form) != uint64(flow.AllocationFormClosed) || uint64(call) != uint64(flow.CallFormMethod) {
		t.Fatalf("receipt ordinals drifted: role=%d form=%d call=%d", role, form, call)
	}
	if _, ok := receiptAllocationRole(flow.AllocationInvalid); ok {
		t.Fatal("invalid allocation role became a receipt column")
	}
	if _, ok := receiptCallForm(0); ok {
		t.Fatal("invalid call form became a receipt column")
	}
}
