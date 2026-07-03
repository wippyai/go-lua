package body

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	luatypeprojection "github.com/wippyai/go-lua/analysis/lua/typeprojection"
	"github.com/wippyai/go-lua/analysis/type/access"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

// OrdinaryAssignmentTargetType is the syntax-owned target type projection for
// an ordinary assignment. Body resolves the target expression and exposes the
// target value separately so readmodel can apply diagnostic-only widening
// without importing AST.
type OrdinaryAssignmentTargetType struct {
	Type        typ.Type
	TargetValue product.Value
	HasValue    bool
}

func (r *Result) OrdinaryAssignmentTargetTypeAt(point cfg.Point, fact OrdinaryAssignmentFact) (OrdinaryAssignmentTargetType, bool) {
	if fact.HasPath && !fact.Path.IsEmpty() {
		if declared, ok := r.declaredWritePathType(fact.Path); ok {
			return r.ordinaryAssignmentTargetTypeResult(point, fact.Target, declared), true
		}
		if declared, ok := r.dominatingDeclarationSourceWritePathType(point, fact.Path); ok {
			return r.ordinaryAssignmentTargetTypeResult(point, fact.Target, declared), true
		}
	}
	attr, ok := assignmentTargetAttrExpr(fact.Target)
	if !ok || attr.Object == nil || attr.Key == nil {
		return OrdinaryAssignmentTargetType{}, false
	}
	container, ok := r.ExpressionTypeBeforeBoundary(point, attr.Object)
	if !ok || container == nil {
		return OrdinaryAssignmentTargetType{}, false
	}
	if attr.KeySyntax != ast.AttrKeyIndex {
		name := ast.KeyName(attr.Key)
		if name == "" {
			return OrdinaryAssignmentTargetType{}, false
		}
		t, ok := access.Field(container, name)
		if !ok {
			return OrdinaryAssignmentTargetType{}, false
		}
		return r.ordinaryAssignmentTargetTypeResult(point, fact.Target, t), true
	}
	if key, ok := attr.Key.(*ast.StringExpr); ok && key != nil {
		t, ok := access.Field(container, key.Value)
		if !ok {
			return OrdinaryAssignmentTargetType{}, false
		}
		return r.ordinaryAssignmentTargetTypeResult(point, fact.Target, t), true
	}
	keyType, ok := r.ExpressionTypeBeforeBoundary(point, attr.Key)
	if !ok {
		keyType, ok = LiteralExpressionType(attr.Key)
	}
	if !ok || keyType == nil {
		return OrdinaryAssignmentTargetType{}, false
	}
	t, ok := luatypeprojection.DynamicWriteValueType(container, keyType)
	if !ok {
		return OrdinaryAssignmentTargetType{}, false
	}
	return r.ordinaryAssignmentTargetTypeResult(point, fact.Target, t), true
}

func (r *Result) ordinaryAssignmentTargetTypeResult(point cfg.Point, target ast.Expr, t typ.Type) OrdinaryAssignmentTargetType {
	out := OrdinaryAssignmentTargetType{Type: t}
	if r != nil && target != nil {
		if value, ok := r.ExpressionValueBeforeBoundary(point, target); ok {
			out.TargetValue = value
			out.HasValue = true
		}
	}
	return out
}

func (r *Result) declaredWritePathType(p path.Path) (typ.Type, bool) {
	if p.Symbol == 0 {
		return nil, false
	}
	declared, ok := r.SymbolDeclaredType(p.Symbol)
	if !ok || declared == nil {
		return nil, false
	}
	if len(p.Segments) == 0 {
		return declared, true
	}
	return luatypeprojection.ApplyWriteSegments(declared, p.Segments)
}

func (r *Result) dominatingDeclarationSourceWritePathType(point cfg.Point, p path.Path) (typ.Type, bool) {
	if r == nil || p.IsEmpty() || p.Symbol == 0 {
		return nil, false
	}
	declaration, ok := r.DominatingPathRootDeclarationSource(point, p)
	if !ok || !declaration.Source.HasExpr {
		return nil, false
	}
	sourcePath, ok := r.ExpressionRefPath(declaration.Source.ExprRef)
	if !ok || sourcePath.IsEmpty() || sourcePath.Symbol == 0 {
		return nil, false
	}
	return r.declaredWritePathType(sourcePath.AppendSegments(p.Segments))
}
