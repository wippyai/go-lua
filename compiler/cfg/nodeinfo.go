// Package cfg provides CFG construction utilities.
package cfg

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg/extraction"
)

// ExtractAssignTarget extracts assignment target info from an expression.
func ExtractAssignTarget(expr ast.Expr) AssignTarget {
	switch e := expr.(type) {
	case *ast.IdentExpr:
		return AssignTarget{Kind: TargetIdent, Name: e.Value, Expr: expr}
	case *ast.AttrGetExpr:
		baseName, fields := extraction.ExtractFieldPath(e)
		if baseName != "" && len(fields) > 0 {
			return AssignTarget{
				Kind:      TargetField,
				BaseName:  baseName,
				FieldPath: fields,
				Expr:      expr,
			}
		}
		return AssignTarget{Kind: TargetIndex, Base: e.Object, Key: e.Key, Expr: expr}
	default:
		return AssignTarget{Kind: TargetIndex, Expr: expr}
	}
}

// BuildCallInfo creates a CallInfo from a FuncCallExpr, extracting all needed info.
func BuildCallInfo(call *ast.FuncCallExpr, isStmt bool) *CallInfo {
	if call == nil {
		return nil
	}
	info := &CallInfo{
		Callee:       call.Func,
		Args:         call.Args,
		Method:       call.Method,
		Receiver:     call.Receiver,
		IsStmt:       isStmt,
		CalleeName:   extraction.IdentName(call.Func),
		ReceiverName: extraction.IdentName(call.Receiver),
	}
	info.ArgNames = make([]string, len(call.Args))

	for i, arg := range call.Args {
		info.ArgNames[i] = extraction.IdentName(arg)
	}

	ExtractTypeCheckPattern(info)

	return info
}

// ExtractTypeCheckPattern detects Type:is(x) or TypeName(x) patterns.
func ExtractTypeCheckPattern(info *CallInfo) {
	if info == nil {
		return
	}

	if info.Method == "is" && info.ReceiverName != "" && len(info.Args) > 0 {
		info.IsTypeCheck = true
		info.TypeCheckName = info.ReceiverName

		return
	}

	if info.Method == "" && info.Receiver == nil && info.CalleeName != "" && len(info.Args) > 0 {
		info.IsTypeCheck = true
		info.TypeCheckName = info.CalleeName
	}
}

// ExtractSourceCalls pre-extracts CallInfo for each source expression that is a call.
func ExtractSourceCalls(exprs []ast.Expr) []*CallInfo {
	if len(exprs) == 0 {
		return nil
	}

	calls := make([]*CallInfo, len(exprs))

	for i, expr := range exprs {
		if call, ok := expr.(*ast.FuncCallExpr); ok {
			calls[i] = BuildCallInfo(call, false)
		}
	}

	return calls
}
