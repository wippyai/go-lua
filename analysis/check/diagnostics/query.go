package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/internal/readmodel"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

// diagnosticQuery is the diagnostic producer boundary for solved analysis
// facts. Producers should ask this facade for user-facing proof/type views
// instead of constructing lower-level read models or choosing boundary read
// mechanics directly.
type diagnosticQuery struct {
	result     *body.Result
	reader     readmodel.Reader
	typeValues *typevalue.Cache
}

func newDiagnosticQuery(result *body.Result) diagnosticQuery {
	return diagnosticQuery{
		result:     result,
		reader:     readmodel.New(result),
		typeValues: result.TypeValues(),
	}
}

func (c producerContext) query(result *body.Result) diagnosticQuery {
	return newDiagnosticQuery(result)
}

func (q diagnosticQuery) ValueType(value product.Value) (typ.Type, bool) {
	return q.reader.ValueType(value)
}

func (q diagnosticQuery) ValueTypeWithPresence(value product.Value) (typ.Type, bool) {
	return q.reader.ValueTypeWithPresence(value)
}

func (q diagnosticQuery) ValueProofAdmissible(value product.Value, want typ.Type) bool {
	return q.reader.ValueProofAdmissible(value, want)
}

func (q diagnosticQuery) ValueWitnessProvenMismatch(value product.Value, want typ.Type) bool {
	return q.reader.ValueWitnessProvenMismatch(value, want)
}

func (q diagnosticQuery) SourceWitnessProvenMismatchType(point cfg.Point, source sourceprovenance.ASTSource, want typ.Type) (typ.Type, bool) {
	if q.result == nil {
		return nil, false
	}
	value, ok := q.result.LocalAssignmentSourceValueAtBoundary(point, source)
	if !ok || !q.ValueWitnessProvenMismatch(value, want) {
		return nil, false
	}
	return q.ValueType(value)
}

func (q diagnosticQuery) ValueAdmissible(value product.Value, want typ.Type) bool {
	return q.reader.ValueAdmissible(value, want)
}

func (q diagnosticQuery) ValueHasUntrustedTopOrigin(value product.Value) bool {
	return q.reader.ValueHasUntrustedTopOrigin(value)
}

func (q diagnosticQuery) ValueHasExactIdentity(value product.Value) bool {
	return q.reader.ValueHasExactIdentity(value)
}

func (q diagnosticQuery) ValueHasLocalExclusiveExactIdentity(point cfg.Point, value product.Value) bool {
	return q.reader.ValueHasLocalExclusiveExactIdentity(point, value)
}

func (q diagnosticQuery) RefineDeclaredType(declared typ.Type, value product.Value) (typ.Type, bool) {
	return q.reader.RefineDeclaredType(declared, value)
}

func (q diagnosticQuery) NarrowDeclaredByOrigin(declared typ.Type, value product.Value) (typ.Type, bool) {
	return q.reader.NarrowDeclaredByOrigin(declared, value)
}

func (q diagnosticQuery) VariantOriginType(value product.Value) (typ.Type, bool) {
	return q.reader.VariantOriginType(value)
}

func (q diagnosticQuery) FullVariantOriginType(value product.Value) (typ.Type, bool) {
	return q.reader.FullVariantOriginType(value)
}

func (q diagnosticQuery) IsSubtype(sub, super typ.Type) bool {
	return q.typeValues.IsSubtype(sub, super)
}

func (q diagnosticQuery) IsEquivalent(left, right typ.Type) bool {
	return q.IsSubtype(left, right) && q.IsSubtype(right, left)
}

func (q diagnosticQuery) IsFreshAssignable(sub, super typ.Type) bool {
	return q.typeValues.IsFreshAssignable(sub, super)
}

func (q diagnosticQuery) RuntimeKindReducedType(value product.Value, declared typ.Type) (typ.Type, bool) {
	return q.reader.RuntimeKindReducedType(value, declared)
}

func (q diagnosticQuery) SourceType(point cfg.Point, source sourceprovenance.ASTSource) (typ.Type, bool) {
	return q.reader.SourceType(point, source)
}

func (q diagnosticQuery) SourceValue(point cfg.Point, source sourceprovenance.ASTSource) (product.Value, bool) {
	return q.reader.SourceValue(point, source)
}

func (q diagnosticQuery) ValueSourceForExplanationAtBoundary(point cfg.Point, source factflow.ValueSource) (product.Value, bool) {
	if q.result == nil {
		return product.Value{}, false
	}
	return q.result.SourceValueForExplanationAtBoundary(point, source)
}

func (q diagnosticQuery) SourceValueAtBoundary(point cfg.Point, source factflow.ValueSource) (product.Value, bool) {
	if q.result == nil {
		return product.Value{}, false
	}
	return q.result.SourceValueAtBoundary(point, source)
}

func (q diagnosticQuery) SourceValueBeforeBoundary(point cfg.Point, source factflow.ValueSource) (product.Value, bool) {
	if q.result == nil {
		return product.Value{}, false
	}
	return q.result.SourceValueBeforeBoundary(point, source)
}

func (q diagnosticQuery) ExpressionPath(expr ast.Expr) (pathdom.Path, bool) {
	if q.result == nil || expr == nil {
		return pathdom.Path{}, false
	}
	return q.result.ExpressionPath(expr)
}

func (q diagnosticQuery) ExpressionValueAtBoundary(point cfg.Point, expr ast.Expr) (product.Value, bool) {
	if q.result == nil || expr == nil {
		return product.Value{}, false
	}
	return q.result.ExpressionValueAtBoundary(point, expr)
}

func (q diagnosticQuery) ExpressionValueBeforeBoundary(point cfg.Point, expr ast.Expr) (product.Value, bool) {
	if q.result == nil || expr == nil {
		return product.Value{}, false
	}
	return q.result.ExpressionValueBeforeBoundary(point, expr)
}

func (q diagnosticQuery) PathValueBeforeBoundary(point cfg.Point, path pathdom.Path) (product.Value, bool) {
	if q.result == nil || path.IsEmpty() {
		return product.Value{}, false
	}
	return q.result.PathValueBeforeBoundary(point, path)
}

func (q diagnosticQuery) PathValueAtBoundary(point cfg.Point, path pathdom.Path) (product.Value, bool) {
	if q.result == nil || path.IsEmpty() {
		return product.Value{}, false
	}
	return q.result.PathValueAtBoundary(point, path)
}

func (q diagnosticQuery) CallExprResultValue(call *ast.FuncCallExpr, resultIndex int) (product.Value, bool) {
	if q.result == nil || call == nil {
		return product.Value{}, false
	}
	return q.result.CallExprResultValue(call, resultIndex)
}
