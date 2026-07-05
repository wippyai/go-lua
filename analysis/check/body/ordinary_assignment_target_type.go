package body

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	luatypeprojection "github.com/wippyai/go-lua/analysis/lua/typeprojection"
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
	declared, hasDeclared, hasHardDeclared := r.ordinaryAssignmentDeclaredPathType(point, fact)
	attr, ok := assignmentTargetAttrExpr(fact.Target)
	if !ok || attr.Object == nil || attr.Key == nil {
		if hasDeclared {
			return r.ordinaryAssignmentDeclaredTargetTypeResult(point, fact.Target, declared, hasHardDeclared), true
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
			return r.ordinaryAssignmentDeclaredTargetTypeResult(point, fact.Target, declared, hasHardDeclared), true
		} else {
			return OrdinaryAssignmentTargetType{}, false
		}
	}
	var projected typ.Type
	if seg, ok := staticAssignmentWriteSegment(attr); ok {
		writeContainer := staticAssignmentWriteContainer(container, declaredContainer, hasDeclaredContainer)
		t, ok := luatypeprojection.ApplyWriteSegments(writeContainer, []segment.Segment{seg})
		if !ok {
			if hasDeclared {
				return r.ordinaryAssignmentDeclaredTargetTypeResult(point, fact.Target, declared, hasHardDeclared), true
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
				return r.ordinaryAssignmentDeclaredTargetTypeResult(point, fact.Target, declared, hasHardDeclared), true
			}
			return OrdinaryAssignmentTargetType{}, false
		}
		t, ok := luatypeprojection.DynamicWriteValueType(container, keyType)
		if !ok && hasDeclaredContainer && declaredContainer != nil && !typ.TypeEquals(container, declaredContainer) {
			t, ok = luatypeprojection.DynamicWriteValueType(declaredContainer, keyType)
		}
		if !ok {
			if hasDeclared {
				return r.ordinaryAssignmentDeclaredTargetTypeResult(point, fact.Target, declared, hasHardDeclared), true
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
	out.Declared = hasHardDeclared || hasSyntaxDeclared
	return out, true
}

func inferredNilOnlyWriteTarget(t typ.Type) bool {
	return typ.IsNever(t) || (t != nil && t.Kind() == kind.Nil)
}

func (r *Result) ordinaryAssignmentDeclaredPathType(point cfg.Point, fact OrdinaryAssignmentFact) (typ.Type, bool, bool) {
	if !fact.HasPath || fact.Path.IsEmpty() {
		return nil, false, false
	}
	if declared, ok := r.declaredWritePathType(fact.Path); ok {
		return declared, true, true
	}
	inferred, ok := r.dominatingDeclarationSourceWritePathType(point, fact.Path)
	return inferred, ok, false
}

func (r *Result) ordinaryAssignmentDeclaredTargetExpressionType(point cfg.Point, attr *ast.AttrGetExpr) (typ.Type, bool) {
	if attr == nil || attr.Object == nil || attr.Key == nil {
		return nil, false
	}
	container, ok := r.DeclaredExpressionTypeAt(point, attr.Object)
	if !ok || container == nil {
		return nil, false
	}
	if seg, ok := staticAssignmentWriteSegment(attr); ok {
		return luatypeprojection.ApplyWriteSegments(container, []segment.Segment{seg})
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

func staticAssignmentWriteSegment(attr *ast.AttrGetExpr) (segment.Segment, bool) {
	if attr == nil || attr.Key == nil {
		return segment.Segment{}, false
	}
	if attr.KeySyntax != ast.AttrKeyIndex {
		name := ast.KeyName(attr.Key)
		if name == "" {
			return segment.Segment{}, false
		}
		return segment.Segment{Kind: segment.SegmentField, Name: name}, true
	}
	if key, ok := attr.Key.(*ast.StringExpr); ok && key != nil {
		return segment.Segment{Kind: segment.SegmentIndexString, Name: key.Value}, true
	}
	return segment.Segment{}, false
}

func staticAssignmentWriteContainer(current, declared typ.Type, hasDeclared bool) typ.Type {
	if hasDeclared && declared != nil && !declaredAssignmentContainerIsUnion(declared, 0) {
		return declared
	}
	return current
}

func declaredAssignmentContainerIsUnion(t typ.Type, depth int) bool {
	if t == nil || depth > typ.DefaultRecursionDepth {
		return false
	}
	switch tt := t.(type) {
	case *typ.Annotated:
		return declaredAssignmentContainerIsUnion(tt.Inner, depth+1)
	case *typ.Alias:
		next := tt.UnaliasedTarget()
		if next == nil || next == t {
			return false
		}
		return declaredAssignmentContainerIsUnion(next, depth+1)
	case *typ.Optional:
		return declaredAssignmentContainerIsUnion(tt.Inner, depth+1)
	case *typ.Recursive:
		if tt.Body == nil || tt.Body == t {
			return false
		}
		return declaredAssignmentContainerIsUnion(tt.Body, depth+1)
	case *typ.Union:
		return true
	default:
		return false
	}
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
	return r.ValueTypeWithPresence(value)
}

func (r *Result) ordinaryAssignmentDeclaredTargetTypeResult(point cfg.Point, target ast.Expr, t typ.Type, hardDeclared bool) OrdinaryAssignmentTargetType {
	out := r.ordinaryAssignmentTargetTypeResult(point, target, t)
	out.Declared = hardDeclared
	return out
}

func widerWritableType(a, b typ.Type) typ.Type {
	switch {
	case a == nil:
		return b
	case b == nil:
		return a
	case assignmentWriteIntersectionIncludesDeclared(a, b):
		return b
	case subtype.IsSubtype(a, b) || subtype.IsFreshAssignable(a, b):
		return b
	case subtype.IsSubtype(b, a):
		return a
	default:
		return a
	}
}

func assignmentWriteIntersectionIncludesDeclared(projected, declared typ.Type) bool {
	intersection, ok := projected.(*typ.Intersection)
	if !ok || declared == nil {
		return false
	}
	for _, member := range intersection.Members {
		if typ.TypeEquals(member, declared) || subtype.IsSubtype(member, declared) {
			return true
		}
	}
	return false
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
	if !ok {
		return nil, false
	}
	if declaration.Source.HasExpr {
		if t, ok := r.declarationSourceRefinementWritePathType(declaration.Source.ExprRef, p.Segments); ok {
			return t, true
		}
	}
	sourcePath, ok := r.ValueSourcePath(declaration.Source)
	if !ok || sourcePath.IsEmpty() || sourcePath.Symbol == 0 {
		return nil, false
	}
	return r.declaredWritePathType(sourcePath.AppendSegments(p.Segments))
}

func (r *Result) declarationSourceRefinementWritePathType(expr factflow.ExprRef, segments []segment.Segment) (typ.Type, bool) {
	if r == nil || expr == 0 {
		return nil, false
	}
	refinement, ok := r.facts.ExpressionRefinement(expr)
	if !ok || refinement.Mode() != factflow.ExpressionRefinementRuntimeValidation {
		return nil, false
	}
	t, ok := r.ValueTypeWithPresence(refinement.Refinement())
	if !ok || t == nil {
		return nil, false
	}
	if len(segments) == 0 {
		return t, true
	}
	return luatypeprojection.ApplyWriteSegments(t, segments)
}
