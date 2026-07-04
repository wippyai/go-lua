package body

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	luatypeprojection "github.com/wippyai/go-lua/analysis/lua/typeprojection"
	"github.com/wippyai/go-lua/analysis/type/access"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/subtype"
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
	Declared    bool
}

func (r *Result) OrdinaryAssignmentTargetTypeAt(point cfg.Point, fact OrdinaryAssignmentFact) (OrdinaryAssignmentTargetType, bool) {
	declared, hasDeclared := r.ordinaryAssignmentDeclaredPathType(point, fact)
	attr, ok := assignmentTargetAttrExpr(fact.Target)
	if !ok || attr.Object == nil || attr.Key == nil {
		if hasDeclared {
			return r.ordinaryAssignmentDeclaredTargetTypeResult(point, fact.Target, declared), true
		}
		return OrdinaryAssignmentTargetType{}, false
	}
	container, ok := r.ExpressionTypeBeforeBoundary(point, attr.Object)
	declaredContainer, hasDeclaredContainer := r.DeclaredExpressionTypeAt(point, attr.Object)
	if !hasDeclaredContainer {
		declaredContainer, hasDeclaredContainer = r.dynamicWriteDeclaredContainerType(point)
	}
	if !ok || container == nil {
		if hasDeclaredContainer && declaredContainer != nil {
			container = declaredContainer
		} else if hasDeclared {
			return r.ordinaryAssignmentDeclaredTargetTypeResult(point, fact.Target, declared), true
		} else {
			return OrdinaryAssignmentTargetType{}, false
		}
	}
	var projected typ.Type
	if attr.KeySyntax != ast.AttrKeyIndex {
		name := ast.KeyName(attr.Key)
		if name == "" {
			if hasDeclared {
				return r.ordinaryAssignmentDeclaredTargetTypeResult(point, fact.Target, declared), true
			}
			return OrdinaryAssignmentTargetType{}, false
		}
		t, ok := access.Field(container, name)
		if !ok {
			if hasDeclared {
				return r.ordinaryAssignmentDeclaredTargetTypeResult(point, fact.Target, declared), true
			}
			return OrdinaryAssignmentTargetType{}, false
		}
		projected = t
	} else if key, ok := attr.Key.(*ast.StringExpr); ok && key != nil {
		t, ok := access.Field(container, key.Value)
		if !ok {
			if hasDeclared {
				return r.ordinaryAssignmentDeclaredTargetTypeResult(point, fact.Target, declared), true
			}
			return OrdinaryAssignmentTargetType{}, false
		}
		projected = t
	} else {
		keyType, ok := r.dynamicWriteKeyType(point)
		if !ok {
			keyType, ok = r.ExpressionTypeBeforeBoundary(point, attr.Key)
		}
		if !ok {
			keyType, ok = LiteralExpressionType(attr.Key)
		}
		if !ok || keyType == nil {
			if hasDeclared {
				return r.ordinaryAssignmentDeclaredTargetTypeResult(point, fact.Target, declared), true
			}
			return OrdinaryAssignmentTargetType{}, false
		}
		t, ok := luatypeprojection.DynamicWriteValueType(container, keyType)
		if !ok && hasDeclaredContainer && declaredContainer != nil && !typ.TypeEquals(container, declaredContainer) {
			t, ok = luatypeprojection.DynamicWriteValueType(declaredContainer, keyType)
		}
		if !ok {
			if hasDeclared {
				return r.ordinaryAssignmentDeclaredTargetTypeResult(point, fact.Target, declared), true
			}
			return OrdinaryAssignmentTargetType{}, false
		}
		projected = t
	}
	if hasDeclared {
		projected = widerWritableType(projected, declared)
	}
	hasSyntaxDeclared := false
	if syntaxDeclared, ok := r.ordinaryAssignmentDeclaredTargetExpressionType(point, attr); ok {
		hasSyntaxDeclared = true
		projected = widerWritableType(projected, syntaxDeclared)
	}
	if !hasDeclared && !hasSyntaxDeclared && inferredNilOnlyWriteTarget(projected) {
		return OrdinaryAssignmentTargetType{}, false
	}
	out := r.ordinaryAssignmentTargetTypeResult(point, fact.Target, projected)
	out.Declared = hasDeclared || hasSyntaxDeclared
	return out, true
}

func inferredNilOnlyWriteTarget(t typ.Type) bool {
	return typ.IsNever(t) || (t != nil && t.Kind() == kind.Nil)
}

func (r *Result) ordinaryAssignmentDeclaredPathType(point cfg.Point, fact OrdinaryAssignmentFact) (typ.Type, bool) {
	if !fact.HasPath || fact.Path.IsEmpty() {
		return nil, false
	}
	if declared, ok := r.declaredWritePathType(fact.Path); ok {
		return declared, true
	}
	return r.dominatingDeclarationSourceWritePathType(point, fact.Path)
}

func (r *Result) ordinaryAssignmentDeclaredTargetExpressionType(point cfg.Point, attr *ast.AttrGetExpr) (typ.Type, bool) {
	if attr == nil || attr.Object == nil || attr.Key == nil {
		return nil, false
	}
	container, ok := r.DeclaredExpressionTypeAt(point, attr.Object)
	if !ok || container == nil {
		return nil, false
	}
	if attr.KeySyntax != ast.AttrKeyIndex {
		name := ast.KeyName(attr.Key)
		if name == "" {
			return nil, false
		}
		return access.Field(container, name)
	}
	if key, ok := attr.Key.(*ast.StringExpr); ok && key != nil {
		return access.WritableIndex(container, typ.LiteralString(key.Value))
	}
	keyType, ok := LiteralExpressionType(attr.Key)
	if !ok {
		keyType, ok = r.DeclaredExpressionTypeAt(point, attr.Key)
	}
	if !ok || keyType == nil {
		return nil, false
	}
	return luatypeprojection.DynamicWriteValueType(container, keyType)
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

func (r *Result) dynamicWriteDeclaredContainerType(point cfg.Point) (typ.Type, bool) {
	if r == nil {
		return nil, false
	}
	write, ok := r.DynamicIndexWrite(point)
	if !ok {
		return nil, false
	}
	return r.DeclaredPathTypeAt(point, write.TablePathRef(), true)
}

func (r *Result) dynamicWriteKeyType(point cfg.Point) (typ.Type, bool) {
	if r == nil {
		return nil, false
	}
	write, ok := r.DynamicIndexWrite(point)
	if !ok {
		return nil, false
	}
	value, ok := r.SourceValueBeforeBoundary(point, write.KeySource())
	if !ok {
		return nil, false
	}
	return r.valueTypeWithPresence(value)
}

func (r *Result) ordinaryAssignmentDeclaredTargetTypeResult(point cfg.Point, target ast.Expr, t typ.Type) OrdinaryAssignmentTargetType {
	out := r.ordinaryAssignmentTargetTypeResult(point, target, t)
	out.Declared = true
	return out
}

func widerWritableType(a, b typ.Type) typ.Type {
	switch {
	case a == nil:
		return b
	case b == nil:
		return a
	case subtype.IsSubtype(a, b) || subtype.IsFreshAssignable(a, b):
		return b
	case subtype.IsSubtype(b, a):
		return a
	default:
		return a
	}
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
