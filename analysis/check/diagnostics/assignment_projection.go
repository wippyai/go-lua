package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/dominance"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/lua/typeannotation"
	luatypeprojection "github.com/wippyai/go-lua/analysis/lua/typeprojection"
	"github.com/wippyai/go-lua/analysis/lua/valueexpr"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/access"
	"github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse/numparse"
)

func assignmentValueType(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, expr ast.Expr, source sourceprovenance.ASTSource) (typ.Type, bool) {
	if got, ok := valueexpr.LiteralType(expr); ok {
		return got, true
	}
	if got, ok := projectedOptionalIndexType(result, resolver, point, expr); ok {
		return got, true
	}
	if got, ok := untrustedTopLikeExpressionTypeAt(result, resolver, point, expr); ok {
		return got, true
	}
	if got, ok := localScalarOperatorSourceType(result, resolver, expr); ok {
		return got, true
	}
	if got, ok := projectedFlowSourceType(result, resolver, point, guardEnv{}, expr); ok {
		return got, true
	}
	if got, ok := sourceExpressionTypeWithPresence(result, point, source); ok {
		return got, true
	}
	if got, ok := boundaryExpressionConcreteType(result, point, expr); ok {
		return got, true
	}
	if got, ok := newDiagnosticQuery(result).SourceType(point, source); ok {
		return got, true
	}
	if got, ok := explicitTopLikeCallSourceType(result, resolver, expr); ok {
		return got, true
	}
	if got, ok := explicitTopLikeCallFactSourceType(result, resolver, source); ok {
		return got, true
	}
	return staticExpressionType(result, resolver, expr)
}

func boundaryExpressionTypeWithPresence(result *body.Result, point cfg.Point, expr ast.Expr) (typ.Type, bool) {
	if result == nil || expr == nil || !presenceAwareReadExpression(expr) {
		return nil, false
	}
	query := newDiagnosticQuery(result)
	value, ok := query.ExpressionValueBeforeBoundary(point, expr)
	if !ok {
		return nil, false
	}
	got, ok := query.ValueTypeWithPresence(value)
	if !ok || !mixedOptionalType(got) {
		return nil, false
	}
	return got, true
}

func boundaryExpressionConcreteType(result *body.Result, point cfg.Point, expr ast.Expr) (typ.Type, bool) {
	if result == nil || expr == nil || !presenceAwareReadExpression(expr) {
		return nil, false
	}
	query := newDiagnosticQuery(result)
	value, ok := query.ExpressionValueBeforeBoundary(point, expr)
	if !ok {
		return nil, false
	}
	got, ok := query.ValueTypeWithPresence(value)
	if !ok || got == nil || typ.IsAny(got) || typ.IsUnknown(got) || typ.IsNever(got) {
		return nil, false
	}
	return got, true
}

func sourceExpressionTypeWithPresence(result *body.Result, point cfg.Point, source sourceprovenance.ASTSource) (typ.Type, bool) {
	if source.Kind != sourceprovenance.SourceExpression || !presenceAwareReadExpression(source.Expr) {
		return nil, false
	}
	query := newDiagnosticQuery(result)
	value, ok := query.ExpressionValueBeforeBoundary(point, source.Expr)
	if !ok {
		return nil, false
	}
	return query.ValueTypeWithPresence(value)
}

func presenceAwareReadExpression(expr ast.Expr) bool {
	_, ok := expr.(*ast.AttrGetExpr)
	return ok
}

func callInvalidatedBoundaryExprType(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, expr ast.Expr) (typ.Type, bool) {
	if result == nil || expr == nil {
		return nil, false
	}
	if _, objectLiteral := result.ObjectLiteral(expr); objectLiteral {
		return nil, false
	}
	exprPath, ok := result.ExpressionPath(expr)
	if !ok || exprPath.IsEmpty() || !expressionPathInvalidatedByDominatingCall(result, point, exprPath) {
		return nil, false
	}
	return staticExpressionType(result, resolver, expr)
}

type dominatingCallInvalidation struct {
	target   pathdom.Path
	callSpan diagnostic.Span
	callName string
}

func expressionPathInvalidatedByDominatingCall(result *body.Result, point cfg.Point, exprPath pathdom.Path) bool {
	_, ok := dominatingCallInvalidationCause(result, point, exprPath)
	return ok
}

func dominatingCallInvalidationCause(result *body.Result, point cfg.Point, exprPath pathdom.Path) (dominatingCallInvalidation, bool) {
	graph := result.Graph()
	if graph == nil || exprPath.IsEmpty() {
		return dominatingCallInvalidation{}, false
	}
	idom := dominance.ComputeImmediateDominators(graph)
	for _, candidate := range graph.RPO() {
		if candidate == point {
			continue
		}
		if !dominance.Dominates(idom, candidate, point) {
			continue
		}
		site, ok := result.CallSite(candidate)
		if !ok {
			continue
		}
		outcome, ok := result.CallOutcomeAt(candidate)
		if !ok || !callOutcomeHasExplicitGuardInvalidation(outcome) {
			continue
		}
		invalidated, ok := callOutcomeGuardInvalidationPaths(result, site, outcome)
		if !ok {
			return dominatingCallInvalidation{callSpan: callInvalidationSpan(result, candidate), callName: callInvalidationName(result, candidate)}, true
		}
		for _, target := range invalidated {
			if exprPath.HasPrefix(target.path) {
				return dominatingCallInvalidation{
					target:   target.path,
					callSpan: callInvalidationSpan(result, candidate),
					callName: callInvalidationName(result, candidate),
				}, true
			}
		}
	}
	return dominatingCallInvalidation{}, false
}

func callInvalidatedPathEvidence(result *body.Result, point cfg.Point, expr ast.Expr) []diagnostic.Evidence {
	if result == nil || expr == nil {
		return nil
	}
	exprPath, ok := result.ExpressionPath(expr)
	if !ok || exprPath.IsEmpty() {
		return nil
	}
	cause, ok := dominatingCallInvalidationCause(result, point, exprPath)
	if !ok {
		return nil
	}
	return []diagnostic.Evidence{{
		Kind:    diagnostic.EvidenceAbstractFact,
		Trust:   diagnostic.TrustProven,
		Span:    cause.callSpan,
		Message: callInvalidatedPathEvidenceMessage(cause, exprPath),
	}}
}

func callInvalidationSpan(result *body.Result, point cfg.Point) diagnostic.Span {
	if fact, ok := result.Call(point); ok {
		return ast.SpanOf(fact.Call)
	}
	return diagnostic.Span{}
}

func callInvalidationName(result *body.Result, point cfg.Point) string {
	if fact, ok := result.Call(point); ok {
		return callEvidenceNameOK(fact.Call)
	}
	return ""
}

func callInvalidatedPathEvidenceMessage(cause dominatingCallInvalidation, read pathdom.Path) string {
	actor := cause.callName
	if actor == "" {
		actor = "earlier call"
	}
	target := cause.target.String()
	readName := read.String()
	if target == "" {
		return actor + " may change state before the read"
	}
	if target == readName {
		return actor + " may change " + readName + " before the read"
	}
	return actor + " may change " + target + ", so the read of " + readName + " needs a fresh check"
}

func localScalarOperatorSourceType(result *body.Result, resolver typeannotation.Resolver, expr ast.Expr) (typ.Type, bool) {
	if !isScalarOperatorExpression(expr) {
		return nil, false
	}
	return newExpressionTyper(result, resolver).typeOf(expr)
}

func isScalarOperatorExpression(expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.LogicalOpExpr:
		return true
	case *ast.RelationalOpExpr:
		return true
	case *ast.StringConcatOpExpr:
		return true
	case *ast.ArithmeticOpExpr:
		return true
	case *ast.UnaryMinusOpExpr:
		return true
	case *ast.UnaryNotOpExpr:
		return true
	case *ast.UnaryLenOpExpr:
		return true
	case *ast.UnaryBNotOpExpr:
		return true
	case *ast.CastExpr:
		return isScalarOperatorExpression(e.Expr)
	case *ast.NonNilAssertExpr:
		return isScalarOperatorExpression(e.Expr)
	default:
		return false
	}
}

func projectedOptionalIndexType(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, expr ast.Expr) (typ.Type, bool) {
	optionalRead := shouldProjectOptionalIndex(result, expr)
	if !optionalRead && !literalIndexReadProvenInRange(result, resolver, point, expr) {
		return nil, false
	}
	got, ok := staticIndexProjectionType(result, resolver, point, expr)
	if !ok {
		return nil, false
	}
	if indexReadProvenInRange(result, resolver, point, expr) {
		withoutNil := projectionWithoutNil(got)
		if withoutNil != nil && !typ.IsNever(withoutNil) {
			return withoutNil, true
		}
	}
	if optionalRead && !projectionHasNil(got) {
		if staticMember, ok := literalStaticIndexMemberProjectionType(result, resolver, point, expr); ok {
			return staticMember, true
		}
		return normalize.Optional(got), true
	}
	if !projectionHasNil(got) {
		return nil, false
	}
	return got, true
}

func optionalIndexReadLacksProof(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, expr ast.Expr) bool {
	if !optionalIndexReadRequiresProof(result, resolver, point, expr) {
		return false
	}
	got, ok := staticIndexProjectionType(result, resolver, point, expr)
	return ok && projectionHasNil(got)
}

func optionalIndexReadRequiresProof(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, expr ast.Expr) bool {
	attr, ok := expr.(*ast.AttrGetExpr)
	if !ok || attr.KeySyntax != ast.AttrKeyIndex {
		return false
	}
	return !indexReadProvenInRange(result, resolver, point, expr) &&
		!literalStaticIndexReadProvenPresent(result, resolver, point, expr)
}

type literalStaticIndexKey struct {
	kind  typ.StaticMemberKind
	name  string
	index int64
}

func literalStaticIndexMemberProjectionType(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, expr ast.Expr) (typ.Type, bool) {
	attr, ok := expr.(*ast.AttrGetExpr)
	if !ok || attr.KeySyntax != ast.AttrKeyIndex {
		return nil, false
	}
	key, ok := literalStaticIndexKeyOf(attr.Key)
	if !ok {
		return nil, false
	}
	if container, ok := indexContainerType(result, resolver, point, attr.Object); ok {
		if member, ok := staticRecordMemberType(container, key); ok {
			return member, true
		}
	}
	if container, ok := newExpressionTyper(result, resolver).typeOf(attr.Object); ok {
		if member, ok := staticRecordMemberType(container, key); ok {
			return member, true
		}
	}
	flowTyper := newFlowExpressionTyper(result, resolver, point, guardEnv{})
	if container, ok := flowTyper.broadType(attr.Object); ok {
		if member, ok := staticRecordMemberType(container, key); ok {
			return member, true
		}
	}
	if container, ok := flowTyper.typeOf(attr.Object); ok {
		return staticRecordMemberType(container, key)
	}
	return nil, false
}

func literalStaticIndexReadProvenPresent(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, expr ast.Expr) bool {
	_, ok := literalStaticIndexMemberProjectionType(result, resolver, point, expr)
	return ok
}

func literalStaticIndexKeyOf(expr ast.Expr) (literalStaticIndexKey, bool) {
	switch key := expr.(type) {
	case *ast.StringExpr:
		return literalStaticIndexKey{kind: typ.StaticMemberStringIndex, name: key.Value}, true
	case *ast.NumberExpr:
		index, ok := numparse.ParseIntegerLiteral(key.Value)
		if !ok {
			return literalStaticIndexKey{}, false
		}
		return literalStaticIndexKey{kind: typ.StaticMemberIntIndex, index: index}, true
	default:
		return literalStaticIndexKey{}, false
	}
}

func staticRecordMemberType(container typ.Type, key literalStaticIndexKey) (typ.Type, bool) {
	rec, ok := unwrap.Alias(container).(*typ.Record)
	if !ok || rec == nil {
		return nil, false
	}
	var member *typ.StaticMember
	switch key.kind {
	case typ.StaticMemberStringIndex:
		member = rec.GetStaticStringIndex(key.name)
	case typ.StaticMemberIntIndex:
		member = rec.GetStaticIntIndex(key.index)
	default:
		return nil, false
	}
	if member == nil || member.Optional || member.Type == nil || projectionHasNil(member.Type) {
		return nil, false
	}
	return member.Type, true
}

func staticIndexProjectionType(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, expr ast.Expr) (typ.Type, bool) {
	attr, ok := expr.(*ast.AttrGetExpr)
	if !ok || attr.KeySyntax != ast.AttrKeyIndex {
		if got, ok := newExpressionTyper(result, resolver).typeOf(expr); ok {
			return got, true
		}
		return nil, false
	}
	if got, ok := declaredIndexProjectionType(result, resolver, point, attr); ok {
		return got, true
	}
	container, ok := newExpressionTyper(result, resolver).typeOf(attr.Object)
	if ok {
		if key, keyOK := indexProjectionKeyType(result, resolver, point, attr.Key); keyOK {
			if got, gotOK := access.RuntimeIndex(container, key); gotOK {
				return got, true
			}
		}
	}
	if got, ok := newExpressionTyper(result, resolver).typeOf(expr); ok {
		return got, true
	}
	return nil, false
}

func declaredIndexProjectionType(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, attr *ast.AttrGetExpr) (typ.Type, bool) {
	if attr == nil || attr.KeySyntax != ast.AttrKeyIndex {
		return nil, false
	}
	container, ok := indexContainerType(result, resolver, point, attr.Object)
	if !ok {
		return nil, false
	}
	key, ok := indexProjectionKeyType(result, resolver, point, attr.Key)
	if !ok {
		return nil, false
	}
	return access.RuntimeIndex(container, key)
}

// indexContainerType resolves the element-read container type for an index
// projection. It uses the declared annotation unless the flow-sensitive
// boundary value widened the array element type through a covariant alias: an
// array container whose element type widened carries the wider element on its
// boundary value, and reading elements at the narrower declared element would
// miss the widening. The substitution applies only to arrays whose element type
// strictly widened, so map/record member nil narrowing is untouched.
func indexContainerType(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, object ast.Expr) (typ.Type, bool) {
	declared, hasDeclared := declaredPathType(result, resolver, object)
	if !hasDeclared {
		return declared, false
	}
	declaredElement, ok := arrayElementType(declared)
	if !ok {
		return declared, true
	}
	query := newDiagnosticQuery(result)
	value, ok := query.ExpressionValueAtBoundary(point, object)
	if !ok {
		return declared, true
	}
	flow, ok := query.ValueType(value)
	if !ok || flow == nil || topLikeType(flow) {
		return declared, true
	}
	flowElement, ok := arrayElementType(flow)
	if !ok {
		return declared, true
	}
	if subtype.IsSubtype(declaredElement, flowElement) && !subtype.IsSubtype(flowElement, declaredElement) {
		return flow, true
	}
	return declared, true
}

func arrayElementType(t typ.Type) (typ.Type, bool) {
	if t == nil {
		return nil, false
	}
	array, ok := unwrap.Alias(t).(*typ.Array)
	if !ok || array == nil || array.Element == nil {
		return nil, false
	}
	return array.Element, true
}

func declaredPathType(result *body.Result, resolver typeannotation.Resolver, expr ast.Expr) (typ.Type, bool) {
	if result == nil || expr == nil {
		return nil, false
	}
	accessPath, ok := result.ExpressionPath(expr)
	if !ok || accessPath.Symbol == 0 {
		return nil, false
	}
	annotation, ok := result.SymbolTypeAnnotation(accessPath.Symbol)
	if !ok {
		return nil, false
	}
	root, ok := lowerType(annotation, resolver)
	if !ok {
		return nil, false
	}
	return expectedTypeAtSegments(transparentComparableType(result, root), accessPath.Segments)
}

func indexProjectionKeyType(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, expr ast.Expr) (typ.Type, bool) {
	key, ok := newExpressionTyper(result, resolver).typeOf(expr)
	if !ok {
		key, ok = numericForIndexExpressionType(result, expr)
	}
	if !ok {
		key, ok = boundaryNumericIndexExpressionType(result, point, expr)
	}
	return key, ok
}

func numericForIndexExpressionType(result *body.Result, expr ast.Expr) (typ.Type, bool) {
	if result == nil || expr == nil || result.Graph() == nil {
		return nil, false
	}
	indexPath, ok := result.ExpressionPath(expr)
	if !ok || indexPath.Symbol == 0 {
		return nil, false
	}
	for _, point := range result.Graph().RPO() {
		fact, ok := result.NumericFor(point)
		if ok && fact.HasSymbol && fact.Symbol == indexPath.Symbol {
			return typ.Number, true
		}
	}
	return nil, false
}

func boundaryNumericIndexExpressionType(result *body.Result, point cfg.Point, expr ast.Expr) (typ.Type, bool) {
	indexPath, ok := result.ExpressionPath(expr)
	if !ok || indexPath.Symbol == 0 {
		return nil, false
	}
	if _, ok := result.NumericFloorAtBoundary(point, indexPath); ok {
		return typ.Number, true
	}
	return nil, false
}

// indexReadProvenInRange reports whether an array element read attr is provably
// in range at point. Literal integer reads need both a length floor and a
// definitely array-like container type; dynamic reads need an explicit
// index-in-range path proof.
func indexReadProvenInRange(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, expr ast.Expr) bool {
	attr, ok := expr.(*ast.AttrGetExpr)
	if !ok || attr.KeySyntax != ast.AttrKeyIndex {
		return false
	}
	if literalIndexReadProvenInRange(result, resolver, point, attr) {
		return true
	}
	containerPath, ok := result.ExpressionPath(attr.Object)
	if !ok || containerPath.IsEmpty() {
		return false
	}
	if lenOp, ok := attr.Key.(*ast.UnaryLenOpExpr); ok {
		lenPath, ok := result.ExpressionPath(lenOp.Expr)
		if !ok || lenPath.Key() != containerPath.Key() {
			return false
		}
		floor, ok := result.LengthFloorAtBoundary(point, containerPath)
		return ok && floor >= 1
	}
	return symbolicIndexReadProvenInRange(result, point, attr.Key, containerPath)
}

func literalIndexReadProvenInRange(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, expr ast.Expr) bool {
	attr, ok := expr.(*ast.AttrGetExpr)
	if !ok || attr.KeySyntax != ast.AttrKeyIndex {
		return false
	}
	index, ok := literalPositiveIntegerIndex(attr.Key)
	if !ok {
		return false
	}
	containerPath, ok := result.ExpressionPath(attr.Object)
	if !ok || containerPath.IsEmpty() {
		return false
	}
	if _, declaredOK := declaredPathType(result, resolver, attr.Object); !declaredOK && literalIndexReadHasPresentBoundaryValue(result, point, attr) {
		return true
	}
	if !expressionDefinitelyDenseArray(result, resolver, attr.Object) &&
		!rootOptionalArrayReadHasPresentElementProof(result, resolver, point, attr, containerPath) {
		return false
	}
	floor, ok := result.LengthFloorAtBoundary(point, containerPath)
	if ok && floor >= index {
		return true
	}
	return literalIndexReadHasPresentBoundaryValue(result, point, attr)
}

func rootOptionalArrayReadHasPresentElementProof(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, attr *ast.AttrGetExpr, containerPath pathdom.Path) bool {
	if attr == nil || containerPath.Symbol == 0 || len(containerPath.Segments) != 0 {
		return false
	}
	declared, ok := declaredPathType(result, resolver, attr.Object)
	if !ok || !optionalDenseArrayDiagnosticType(declared) {
		return false
	}
	return literalIndexReadHasPresentBoundaryValue(result, point, attr)
}

func optionalDenseArrayDiagnosticType(t typ.Type) bool {
	withoutNil := projectionWithoutNil(t)
	return withoutNil != nil && !typ.IsNever(withoutNil) && !typ.SameNodeOrAcyclicEqual(withoutNil, t) &&
		definitelyDenseArrayDiagnosticType(withoutNil, 0)
}

func literalPositiveIntegerIndex(expr ast.Expr) (int64, bool) {
	num, ok := expr.(*ast.NumberExpr)
	if !ok {
		return 0, false
	}
	index, ok := numparse.ParseIntegerLiteral(num.Value)
	return index, ok && index >= 1
}

func expressionDefinitelyDenseArray(result *body.Result, resolver typeannotation.Resolver, expr ast.Expr) bool {
	t, ok := newExpressionTyper(result, resolver).typeOf(expr)
	return ok && definitelyDenseArrayDiagnosticType(t, 0)
}

func literalIndexReadHasPresentBoundaryValue(result *body.Result, point cfg.Point, expr ast.Expr) bool {
	value, ok := newDiagnosticQuery(result).ExpressionValueAtBoundary(point, expr)
	if !ok || !presence.Equal(product.PresenceOf(value), presence.Present()) {
		return false
	}
	return boundaryValueHasReadableType(result, value)
}

func definitelyDenseArrayDiagnosticType(t typ.Type, depth int) bool {
	if t == nil || depth > typ.DefaultRecursionDepth {
		return false
	}
	switch tt := unwrap.Alias(t).(type) {
	case *typ.Array:
		return true
	case *typ.Tuple:
		return true
	case *typ.Optional:
		return false
	case *typ.Union:
		if len(tt.Members) == 0 {
			return false
		}
		for _, member := range tt.Members {
			if !definitelyDenseArrayDiagnosticType(member, depth+1) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func symbolicIndexReadProvenInRange(result *body.Result, point cfg.Point, index ast.Expr, containerPath pathdom.Path) bool {
	basePath, coeff, offset, ok := indexLinearTerm(result, index)
	if !ok || basePath.IsEmpty() {
		return false
	}
	return result.IndexReadSafeAtBoundary(point, basePath, coeff, offset, containerPath)
}

// indexLinearTerm parses an index expression into a base path with a positive
// coefficient and a constant offset: i is (i, 1, 0), i + 1 is (i, 1, 1), 2*i is
// (i, 2, 0), 2*i + 1 is (i, 2, 1).
func indexLinearTerm(result *body.Result, index ast.Expr) (pathdom.Path, int64, int64, bool) {
	if e, ok := index.(*ast.ArithmeticOpExpr); ok {
		switch e.Operator {
		case "+", "-":
			if base, coeff, ok := indexScaledPath(result, e.Lhs); ok {
				if c, ok := indexConstOperand(e.Rhs); ok {
					if e.Operator == "-" {
						c = -c
					}
					return base, coeff, c, true
				}
			}
			if e.Operator == "+" {
				if base, coeff, ok := indexScaledPath(result, e.Rhs); ok {
					if c, ok := indexConstOperand(e.Lhs); ok {
						return base, coeff, c, true
					}
				}
			}
			return pathdom.Path{}, 0, 0, false
		case "*":
			if base, coeff, ok := indexScaledPath(result, e); ok {
				return base, coeff, 0, true
			}
			return pathdom.Path{}, 0, 0, false
		}
		return pathdom.Path{}, 0, 0, false
	}
	if p, ok := result.ExpressionPath(index); ok && !p.IsEmpty() {
		return p, 1, 0, true
	}
	return pathdom.Path{}, 0, 0, false
}

// indexScaledPath parses a bare path i or a positive scaled path c*i / i*c into
// its base path and coefficient. A non-product path has coefficient 1.
func indexScaledPath(result *body.Result, expr ast.Expr) (pathdom.Path, int64, bool) {
	if e, ok := expr.(*ast.ArithmeticOpExpr); ok && e.Operator == "*" {
		if c, ok := indexConstOperand(e.Lhs); ok && c > 0 {
			if p, ok := result.ExpressionPath(e.Rhs); ok && !p.IsEmpty() {
				return p, c, true
			}
		}
		if c, ok := indexConstOperand(e.Rhs); ok && c > 0 {
			if p, ok := result.ExpressionPath(e.Lhs); ok && !p.IsEmpty() {
				return p, c, true
			}
		}
		return pathdom.Path{}, 0, false
	}
	if p, ok := result.ExpressionPath(expr); ok && !p.IsEmpty() {
		return p, 1, true
	}
	return pathdom.Path{}, 0, false
}

func indexConstOperand(expr ast.Expr) (int64, bool) {
	num, ok := expr.(*ast.NumberExpr)
	if !ok {
		return 0, false
	}
	return numparse.ParseIntegerLiteral(num.Value)
}

func shouldProjectOptionalIndex(result *body.Result, expr ast.Expr) bool {
	attr, ok := expr.(*ast.AttrGetExpr)
	return ok && attr.KeySyntax == ast.AttrKeyIndex
}

// optionalMemberReadType types a dot-field read whose receiver path may be nil.
// The boundary snapshot can be more specific (for example, a stale guard may have
// been invalidated to nil), but diagnostics should report the stable contract:
// the read is a mixed non-nil/nil value unless a guard proves the path present.
// It returns only mixed optional types, so pure nil snapshots and non-optional
// projections continue through the regular resolution chain.
func optionalMemberReadType(result *body.Result, resolver typeannotation.Resolver, flow *diagnosticFlowCache, point cfg.Point, env guardEnv, expr ast.Expr) (typ.Type, bool) {
	attr, ok := expr.(*ast.AttrGetExpr)
	if !ok || attr.KeySyntax != ast.AttrKeyDot {
		return nil, false
	}
	if boundary, ok := boundaryExpressionTypeWithPresence(result, point, expr); ok && typ.Nil.Equals(boundary) {
		return nil, false
	}
	if projected, ok := projectedFlowSourceType(result, resolver, point, env, expr); ok && typ.Nil.Equals(projected) {
		return nil, false
	}
	got, ok := newFlowExpressionTyper(result, resolver, point, env).typeOf(expr)
	if !ok || got == nil {
		if memberReadRootProvenPresent(result, env, expr) {
			return nil, false
		}
		if precise, ok := optionalDominatingDeclarationMemberReadType(result, resolver, flow, point, expr, normalize.Optional(typ.Unknown)); ok {
			return precise, true
		}
		if invalidated, ok := optionalInvalidatedDeclarationMemberReadType(result, flow, point, expr); ok {
			return invalidated, true
		}
		return optionalDeclaredConcreteMemberReadType(result, resolver, expr)
	}
	if mixedOptionalType(got) {
		if nonOptionalDeclaredMemberRead(result, resolver, expr) {
			return nil, false
		}
		if precise, ok := optionalDominatingDeclarationMemberReadType(result, resolver, flow, point, expr, got); ok {
			return precise, true
		}
		if !memberReadReceiverMayBeNil(result, resolver, point, env, attr) {
			return nil, false
		}
		return got, true
	}
	if receiver, ok := attr.Object.(ast.Expr); ok {
		if receiverType, receiverOK := newFlowExpressionTyper(result, resolver, point, env).typeOf(receiver); receiverOK &&
			projectionHasNil(receiverType) &&
			!memberReadRootProvenPresent(result, env, receiver) &&
			!projectionHasNil(got) {
			return normalize.Optional(got), true
		}
	}
	if typ.IsNever(got) {
		if invalidated, ok := optionalInvalidatedDeclarationMemberReadType(result, flow, point, expr); ok {
			return invalidated, true
		}
		return optionalDeclaredConcreteMemberReadType(result, resolver, expr)
	}
	if typ.Nil.Equals(got) {
		if declared, ok := declaredReadProjectionType(result, resolver, expr); ok && mixedOptionalType(declared) {
			return declared, true
		}
		if broad, ok := newExpressionTyper(result, resolver).typeOf(expr); ok && mixedOptionalType(broad) {
			return broad, true
		}
	}
	return nil, false
}

func memberReadRootProvenPresent(result *body.Result, env guardEnv, expr ast.Expr) bool {
	if result == nil || expr == nil {
		return false
	}
	accessPath, ok := result.ExpressionPath(expr)
	if !ok || accessPath.Symbol == 0 {
		return false
	}
	root := accessPath.RootOnly()
	return env.hasPresent(root) || env.hasTruthy(root)
}

func memberReadReceiverMayBeNil(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, env guardEnv, attr *ast.AttrGetExpr) bool {
	if result == nil || attr == nil || attr.Object == nil {
		return false
	}
	receiverType, ok := newFlowExpressionTyper(result, resolver, point, env).typeOf(attr.Object)
	if !ok || receiverType == nil || !projectionHasNil(receiverType) {
		return false
	}
	if receiverPath, ok := result.ExpressionPath(attr.Object); ok && !receiverPath.IsEmpty() {
		if env.hasPresent(receiverPath) || env.hasTruthy(receiverPath) {
			return false
		}
	}
	return true
}

func optionalDominatingDeclarationMemberReadType(result *body.Result, resolver typeannotation.Resolver, flow *diagnosticFlowCache, point cfg.Point, expr ast.Expr, got typ.Type) (typ.Type, bool) {
	inner := projectionWithoutNil(got)
	if inner == nil || (!typ.IsUnknown(inner) && !typ.IsAny(inner)) {
		return nil, false
	}
	declared, ok := dominatingDeclarationProjectionType(result, resolver, flow, point, expr)
	if !ok || declared == nil || typ.IsNever(declared) || typ.IsAny(declared) || typ.IsUnknown(declared) {
		return nil, false
	}
	if projectionHasNil(declared) {
		declaredInner := projectionWithoutNil(declared)
		if declaredInner == nil || typ.IsNever(declaredInner) || typ.IsAny(declaredInner) || typ.IsUnknown(declaredInner) {
			return nil, false
		}
		return declared, true
	}
	return normalize.Optional(declared), true
}

func optionalInvalidatedDeclarationMemberReadType(result *body.Result, flow *diagnosticFlowCache, point cfg.Point, expr ast.Expr) (typ.Type, bool) {
	if result == nil || expr == nil {
		return nil, false
	}
	accessPath, ok := result.ExpressionPath(expr)
	if !ok || accessPath.Symbol == 0 || len(accessPath.Segments) == 0 {
		return nil, false
	}
	_, declarationPoint, ok := dominatingRootLocalAssignment(result, flow, point, accessPath.Symbol)
	if !ok || !pathInvalidatedBetween(result, flow, declarationPoint, point, accessPath) {
		return nil, false
	}
	return normalize.Optional(typ.Unknown), true
}

func nonOptionalDeclaredMemberRead(result *body.Result, resolver typeannotation.Resolver, expr ast.Expr) bool {
	if result == nil || expr == nil {
		return false
	}
	accessPath, ok := result.ExpressionPath(expr)
	if !ok || accessPath.Symbol == 0 {
		return false
	}
	annotation, ok := result.SymbolTypeAnnotation(accessPath.Symbol)
	if !ok {
		return false
	}
	root, ok := lowerType(annotation, resolver)
	if !ok || projectionHasNil(root) {
		return false
	}
	declared, ok := declaredReadProjectionType(result, resolver, expr)
	return ok && declared != nil && !typ.IsNever(declared) && !projectionHasNil(declared)
}

func optionalDeclaredMemberReadType(result *body.Result, resolver typeannotation.Resolver, expr ast.Expr) typ.Type {
	if declared, ok := declaredReadProjectionType(result, resolver, expr); ok && declared != nil && !typ.IsNever(declared) {
		return normalize.Optional(declared)
	}
	if broad, ok := newExpressionTyper(result, resolver).typeOf(expr); ok && broad != nil && !typ.IsNever(broad) {
		return normalize.Optional(broad)
	}
	return normalize.Optional(typ.Unknown)
}

func optionalDeclaredConcreteMemberReadType(result *body.Result, resolver typeannotation.Resolver, expr ast.Expr) (typ.Type, bool) {
	got := optionalDeclaredMemberReadType(result, resolver, expr)
	inner := projectionWithoutNil(got)
	if inner == nil || typ.IsNever(inner) || typ.IsAny(inner) || typ.IsUnknown(inner) {
		return nil, false
	}
	return got, true
}

func declaredReadProjectionType(result *body.Result, resolver typeannotation.Resolver, expr ast.Expr) (typ.Type, bool) {
	if result == nil || expr == nil {
		return nil, false
	}
	accessPath, ok := result.ExpressionPath(expr)
	if !ok || accessPath.Symbol == 0 || len(accessPath.Segments) == 0 {
		return nil, false
	}
	annotation, ok := result.SymbolTypeAnnotation(accessPath.Symbol)
	if !ok {
		return nil, false
	}
	root, ok := lowerType(annotation, resolver)
	if !ok {
		return nil, false
	}
	return luatypeprojection.ApplySegments(transparentComparableType(result, root), accessPath.Segments)
}

func mixedOptionalType(t typ.Type) bool {
	if t == nil || typ.IsAny(t) || typ.IsUnknown(t) || typ.IsNever(t) || !projectionHasNil(t) {
		return false
	}
	withoutNil := projectionWithoutNil(t)
	return withoutNil != nil && !typ.IsNever(withoutNil)
}

func projectedFlowSourceType(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, env guardEnv, expr ast.Expr) (typ.Type, bool) {
	return projectedSourceTypeWith(result, resolver, point, env, expr, newFlowExpressionTyper)
}

// freshRecordAbsentFieldSourceType types a dot-field source whose name is
// provably absent from a freshly constructed table value as nil, so assigning
// it to a non-optional target reports a mismatch.
func freshRecordAbsentFieldSourceType(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, env guardEnv, expr ast.Expr) (typ.Type, bool) {
	return newFlowExpressionTyper(result, resolver, point, env).freshRecordAbsentFieldType(expr)
}

func projectedStructuralFlowSourceType(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, env guardEnv, expr ast.Expr) (typ.Type, bool) {
	return projectedSourceTypeWith(result, resolver, point, env, expr, newStructuralFlowExpressionTyper)
}

func projectedSourceTypeWith(
	result *body.Result,
	resolver typeannotation.Resolver,
	point cfg.Point,
	env guardEnv,
	expr ast.Expr,
	newTyper func(*body.Result, typeannotation.Resolver, cfg.Point, guardEnv) expressionTyper,
) (typ.Type, bool) {
	switch e := expr.(type) {
	case *ast.FuncCallExpr:
		got, ok := newTyper(result, resolver, point, env).typeOf(expr)
		if !ok || got == nil || typ.IsNever(got) {
			return nil, false
		}
		raw, rawOK := newExpressionTyper(result, resolver).typeOf(expr)
		if !rawOK || !typ.SameNodeOrAcyclicEqual(got, raw) {
			return got, true
		}
		return nil, false
	case *ast.AttrGetExpr:
		if e.KeySyntax == ast.AttrKeyIndex && !shouldProjectOptionalIndex(result, e) {
			return nil, false
		}
		got, ok := newTyper(result, resolver, point, env).typeOf(expr)
		if !ok {
			return nil, false
		}
		if typ.IsNever(got) {
			if e.KeySyntax == ast.AttrKeyIndex {
				if raw, rawOK := newExpressionTyper(result, resolver).typeOf(expr); rawOK && raw != nil && !typ.IsNever(raw) {
					return normalize.Optional(raw), true
				}
				return normalize.Optional(typ.Unknown), true
			}
			return nil, false
		}
		raw, rawOK := newExpressionTyper(result, resolver).typeOf(expr)
		if !rawOK || !typ.SameNodeOrAcyclicEqual(got, raw) {
			return got, true
		}
		return nil, false
	case *ast.IdentExpr:
		got, ok := newTyper(result, resolver, point, env).typeOf(expr)
		if !ok || typ.IsNever(got) {
			return nil, false
		}
		raw, rawOK := newExpressionTyper(result, resolver).typeOf(expr)
		if !rawOK || !typ.SameNodeOrAcyclicEqual(got, raw) {
			return got, true
		}
		return nil, false
	default:
		return nil, false
	}
}

func dominatingDeclarationProjectionType(result *body.Result, resolver typeannotation.Resolver, flow *diagnosticFlowCache, point cfg.Point, expr ast.Expr) (typ.Type, bool) {
	if result == nil || expr == nil {
		return nil, false
	}
	accessPath, ok := result.ExpressionPath(expr)
	if !ok || accessPath.Symbol == 0 || len(accessPath.Segments) == 0 {
		return nil, false
	}
	fact, declarationPoint, ok := dominatingRootLocalAssignment(result, flow, point, accessPath.Symbol)
	if !ok {
		return nil, false
	}
	if !localDeclarationSourceCanProject(fact) {
		return nil, false
	}
	root, ok := localDeclarationType(result, resolver, declarationPoint, fact)
	if !ok {
		return nil, false
	}
	if pathInvalidatedBetween(result, flow, declarationPoint, point, accessPath) {
		return nil, false
	}
	got, ok := expectedTypeAtSegments(root, accessPath.Segments)
	if !ok {
		return nil, false
	}
	if projectionHasNil(root) && !projectionHasNil(got) {
		got = normalize.Optional(got)
	}
	if value, ok := newDiagnosticQuery(result).ExpressionValueAtBoundary(point, expr); !ok || !boundaryValueHasReadableType(result, value) {
		got = normalize.Optional(got)
	}
	return got, true
}

func localDeclarationSourceCanProject(fact semantics.LocalAssignmentFact) bool {
	if fact.Type == nil {
		return localDeclarationSourceIsPath(fact.Expr)
	}
	return localDeclarationSourceIsPath(fact.Expr)
}

func localDeclarationSourceIsPath(expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.IdentExpr, *ast.AttrGetExpr:
		return true
	case *ast.CastExpr:
		return localDeclarationSourceIsPath(e.Expr)
	case *ast.NonNilAssertExpr:
		return localDeclarationSourceIsPath(e.Expr)
	default:
		return false
	}
}

func pathInvalidatedBetween(result *body.Result, flow *diagnosticFlowCache, from, to cfg.Point, target pathdom.Path) bool {
	if result == nil || target.IsEmpty() || from == to {
		return false
	}
	graph := result.Graph()
	if graph == nil {
		return false
	}
	for _, candidate := range graph.RPO() {
		if candidate == from || candidate == to {
			continue
		}
		if !diagnosticCanReach(flow, graph, from, candidate) || !diagnosticCanReach(flow, graph, candidate, to) {
			continue
		}
		if invalidation, ok := result.PathDescendantInvalidation(candidate); ok && target.HasStrictPrefix(invalidation.ContainerPath()) {
			return true
		}
		if fact, ok := result.OrdinaryAssignment(candidate); ok && ordinaryAssignmentInvalidatesMemberPath(fact, target) {
			return true
		}
		if callMayInvalidateTrackedPath(result, candidate, target) {
			return true
		}
	}
	return false
}

func boundaryValueHasReadableType(result *body.Result, value product.Value) bool {
	_, ok := newDiagnosticQuery(result).ValueType(value)
	return ok
}

func dominatingRootDeclarationType(result *body.Result, resolver typeannotation.Resolver, flow *diagnosticFlowCache, point cfg.Point, target symbol.ID) (typ.Type, bool) {
	fact, declarationPoint, ok := dominatingRootLocalAssignment(result, flow, point, target)
	if !ok {
		return nil, false
	}
	return localDeclarationType(result, resolver, declarationPoint, fact)
}

func localDeclarationType(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, fact semantics.LocalAssignmentFact) (typ.Type, bool) {
	if fact.Type != nil {
		if lowered, ok := lowerType(fact.Type, resolver); ok {
			return transparentComparableType(result, lowered), true
		}
	}
	if fact.Expr != nil {
		if got, ok := newExpressionTyper(result, resolver).typeOf(fact.Expr); ok {
			return got, true
		}
		if got, ok := newFlowExpressionTyper(result, resolver, point, guardEnv{}).typeOf(fact.Expr); ok {
			return got, true
		}
	}
	return newDiagnosticQuery(result).SourceType(point, fact.Source)
}

func annotatedIdentifierType(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, expr ast.Expr) (typ.Type, bool) {
	if result == nil {
		return nil, false
	}
	ident, ok := expr.(*ast.IdentExpr)
	if !ok {
		return nil, false
	}
	declared, ok := newExpressionTyper(result, resolver).typeOf(expr)
	if !ok {
		return nil, false
	}
	declared = transparentComparableType(result, declared)
	if typ.IsAny(declared) || typ.IsUnknown(declared) {
		return declared, true
	}
	path, ok := result.ExpressionPath(ident)
	if !ok || path.Symbol == 0 || len(path.Segments) != 0 {
		return nil, false
	}
	value, ok := result.SymbolValueAtBoundary(point, path.Symbol)
	if !ok {
		return nil, false
	}
	return newDiagnosticQuery(result).RefineDeclaredType(declared, value)
}

func untrustedAnnotatedIdentifierType(result *body.Result, resolver typeannotation.Resolver, expr ast.Expr) (typ.Type, bool) {
	if result == nil {
		return nil, false
	}
	ident, ok := expr.(*ast.IdentExpr)
	if !ok {
		return nil, false
	}
	path, ok := result.ExpressionPath(ident)
	if !ok || path.Symbol == 0 || len(path.Segments) != 0 {
		return nil, false
	}
	annotation, ok := result.SymbolTypeAnnotation(path.Symbol)
	if !ok {
		return nil, false
	}
	declared, ok := lowerType(annotation, resolver)
	if !ok || (!typ.IsAny(declared) && !typ.IsUnknown(declared)) {
		return nil, false
	}
	return declared, true
}

func refineAssignmentSourceType(result *body.Result, point cfg.Point, expr ast.Expr, got typ.Type) typ.Type {
	if got == nil {
		return got
	}
	if result == nil {
		return got
	}
	if _, ok := result.ExpressionPath(expr); !ok {
		return got
	}
	query := newDiagnosticQuery(result)
	value, ok := query.ExpressionValueAtBoundary(point, expr)
	if !ok {
		return got
	}
	refined, ok := query.RefineDeclaredType(got, value)
	if !ok {
		return got
	}
	if !topLikeType(got) && !subtype.IsSubtype(refined, got) {
		return got
	}
	return refined
}
