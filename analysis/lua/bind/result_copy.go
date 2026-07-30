package bind

import (
	"github.com/wippyai/go-lua/compiler/ast"
)

func cloneSymbols(ids []ID) []ID {
	if len(ids) == 0 {
		return nil
	}
	return append([]ID(nil), ids...)
}

func cloneFunctions(fns []*ast.FunctionExpr) []*ast.FunctionExpr {
	if len(fns) == 0 {
		return nil
	}
	return append([]*ast.FunctionExpr(nil), fns...)
}

func cloneParamSlots(slots []ParamSlot) []ParamSlot {
	if len(slots) == 0 {
		return nil
	}
	return append([]ParamSlot(nil), slots...)
}

func cloneCaptures(captures []Capture) []Capture {
	if len(captures) == 0 {
		return nil
	}
	return append([]Capture(nil), captures...)
}

func cloneIdentExprs(exprs []*ast.IdentExpr) []*ast.IdentExpr {
	if len(exprs) == 0 {
		return nil
	}
	return append([]*ast.IdentExpr(nil), exprs...)
}

func cloneTypeDecls(decls []TypeDecl) []TypeDecl {
	if len(decls) == 0 {
		return nil
	}
	return append([]TypeDecl(nil), decls...)
}
