package body

import (
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/compiler/ast"
)

// NilabilitySourceInfo is the body-owned source identity for a nilable use.
type NilabilitySourceInfo struct {
	OptionalField bool
	CallPoint     cfg.Point
	HasCallPoint  bool
}

func (r *Result) NilabilitySourceInfoFor(expr any) NilabilitySourceInfo {
	astExpr, ok := expr.(ast.Expr)
	if !ok || astExpr == nil {
		return NilabilitySourceInfo{}
	}
	info := NilabilitySourceInfo{}
	if attr, ok := astExpr.(*ast.AttrGetExpr); ok && attr.KeySyntax == ast.AttrKeyDot {
		info.OptionalField = true
	}
	if call, ok := astExpr.(*ast.FuncCallExpr); ok {
		info.CallPoint, info.HasCallPoint = r.callExprPoint(call)
	}
	return info
}

func (r *Result) NilabilityAccessEvidenceFor(point cfg.Point, expr any) []NilableAccessEvidence {
	astExpr, ok := expr.(ast.Expr)
	if !ok || astExpr == nil {
		return nil
	}
	return r.AssignmentNilableAccessEvidence(point, astExpr)
}
