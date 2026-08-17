package function

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/compiler/ast"
)

func (w *Writer) validExprOrigin(fn *ast.FunctionExpr) error {
	if w == nil || w.binding == nil || w.static == nil || fn == nil {
		return fmt.Errorf("lualower: invalid Function expression")
	}
	origin, ok := w.binding.FunctionOrigin(fn)
	if !ok || origin.Func != fn || origin.Static != (w.static.StaticDepth() > 0) {
		return fmt.Errorf("lualower: unsupported ambiguous Function origin")
	}
	switch origin.Kind {
	case bind.FunctionOriginLiteral:
		return nil
	case bind.FunctionOriginLocalAssignment:
		stmt, ok := origin.Stmt.(*ast.LocalAssignStmt)
		if !ok || stmt == nil || origin.LocalIndex < 0 || origin.LocalIndex >= len(stmt.Exprs) || stmt.Exprs[origin.LocalIndex] != fn {
			return fmt.Errorf("lualower: invalid local Function origin")
		}
		return nil
	default:
		return fmt.Errorf("lualower: unsupported Function expression origin")
	}
}

func (w *Writer) validMethodDef(stmt *ast.FuncDefStmt, origin bind.FunctionOrigin) error {
	if stmt.Name.Method == "" || stmt.Name.Receiver == nil || stmt.Name.Func != nil || !functionTarget(stmt.Name.Receiver) || !stmt.Name.MethodPosition.Valid() ||
		origin.Kind != bind.FunctionOriginMethod || origin.Method != stmt.Name.Method {
		return fmt.Errorf("lualower: invalid method function definition")
	}
	return nil
}

func functionTarget(target ast.Expr) bool {
	for target != nil {
		switch current := target.(type) {
		case *ast.IdentExpr:
			return current != nil && current.Value != ""
		case *ast.AttrGetExpr:
			if current == nil || current.KeySyntax != ast.AttrKeyDot || current.Object == nil || current.Key == nil {
				return false
			}
			key, ok := current.Key.(*ast.StringExpr)
			if !ok || key == nil || key.Value == "" {
				return false
			}
			target = current.Object
		default:
			return false
		}
	}
	return false
}

func (w *Writer) methodPosition(fn *ast.FunctionExpr) (ast.Position, error) {
	origin, ok := w.binding.FunctionOrigin(fn)
	if !ok || origin.Kind != bind.FunctionOriginMethod || origin.Func != fn {
		return ast.Position{}, fmt.Errorf("lualower: missing method Function origin")
	}
	stmt, ok := origin.Stmt.(*ast.FuncDefStmt)
	if !ok || stmt == nil || stmt.Name == nil || stmt.Func != fn || stmt.Name.Method == "" || origin.Method != stmt.Name.Method || !stmt.Name.MethodPosition.Valid() {
		return ast.Position{}, fmt.Errorf("lualower: invalid method Function origin")
	}
	return stmt.Name.MethodPosition, nil
}
