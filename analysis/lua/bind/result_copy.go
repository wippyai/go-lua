package bind

import (
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

func cloneSymbols(ids []symbol.ID) []symbol.ID {
	if len(ids) == 0 {
		return nil
	}
	return append([]symbol.ID(nil), ids...)
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

func cloneTypeDecls(decls []TypeDecl) []TypeDecl {
	if len(decls) == 0 {
		return nil
	}
	return append([]TypeDecl(nil), decls...)
}
