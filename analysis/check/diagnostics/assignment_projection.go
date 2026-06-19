package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/internal/readmodel"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/dominance"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/lua/typeannotation"
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
	if got, ok := projectedFlowSourceType(result, resolver, point, guardEnv{}, expr); ok {
		return got, true
	}
	if got, ok := sourceExpressionTypeWithPresence(result, point, source); ok {
		return got, true
	}
	if got, ok := readmodel.New(result).SourceType(point, source); ok {
		return got, true
	}
	if got, ok := explicitTopLikeCallSourceType(result, resolver, expr); ok {
		return got, true
	}
	if got, ok := explicitTopLikeCallFactSourceType(result, resolver, source); ok {
		return got, true
	}
	return boundaryExprType(result, resolver, expr)
}

func sourceExpressionTypeWithPresence(result *body.Result, point cfg.Point, source sourceprovenance.ASTSource) (typ.Type, bool) {
	if source.Kind != sourceprovenance.SourceExpression || !presenceAwareReadExpression(source.Expr) {
		return nil, false
	}
	reader := readmodel.New(result)
	value, ok := reader.SourceValue(point, source)
	if !ok {
		return nil, false
	}
	return reader.ValueTypeWithPresence(value)
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
	return boundaryExprType(result, resolver, expr)
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
			if pathHasPrefix(exprPath, target) {
				return dominatingCallInvalidation{
					target:   target,
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
	if !shouldProjectOptionalIndex(result, expr) && !literalIndexReadProvenInRange(result, resolver, point, expr) {
		return nil, false
	}
	got, ok := staticIndexProjectionType(result, resolver, point, expr)
	if !ok || !projectionHasNil(got) {
		return nil, false
	}
	if indexReadProvenInRange(result, resolver, point, expr) {
		withoutNil := projectionWithoutNil(got)
		if withoutNil != nil && !typ.IsNever(withoutNil) {
			return withoutNil, true
		}
	}
	return got, true
}

func optionalIndexReadLacksProof(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, expr ast.Expr) bool {
	attr, ok := expr.(*ast.AttrGetExpr)
	if !ok || attr.KeySyntax != ast.AttrKeyIndex {
		return false
	}
	got, ok := staticIndexProjectionType(result, resolver, point, expr)
	return ok && projectionHasNil(got) && !indexReadProvenInRange(result, resolver, point, expr)
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
	if got, ok := newExpressionTyper(result, resolver).typeOf(expr); ok {
		return got, true
	}
	container, ok := newExpressionTyper(result, resolver).typeOf(attr.Object)
	if !ok {
		return nil, false
	}
	key, ok := indexProjectionKeyType(result, resolver, point, attr.Key)
	if !ok {
		return nil, false
	}
	return access.RuntimeIndex(container, key)
}

func declaredIndexProjectionType(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, attr *ast.AttrGetExpr) (typ.Type, bool) {
	if attr == nil || attr.KeySyntax != ast.AttrKeyIndex {
		return nil, false
	}
	container, ok := declaredPathType(result, resolver, attr.Object)
	if !ok {
		return nil, false
	}
	key, ok := indexProjectionKeyType(result, resolver, point, attr.Key)
	if !ok {
		return nil, false
	}
	return access.RuntimeIndex(container, key)
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
	value, ok := result.ExpressionValueAtBoundary(point, expr)
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
	indexPath, ok := result.ExpressionPath(index)
	if !ok || indexPath.IsEmpty() {
		return false
	}
	if !result.IndexInRangeAtBoundary(point, indexPath, containerPath) {
		return false
	}
	return indexValueKnownPositive(result, point, index, indexPath)
}

func indexValueKnownPositive(result *body.Result, point cfg.Point, index ast.Expr, indexPath pathdom.Path) bool {
	num, ok := index.(*ast.NumberExpr)
	if ok {
		value, ok := numparse.ParseIntegerLiteral(num.Value)
		return ok && value >= 1
	}
	floor, ok := result.NumericFloorAtBoundary(point, indexPath)
	return ok && floor >= 1
}

func shouldProjectOptionalIndex(result *body.Result, expr ast.Expr) bool {
	attr, ok := expr.(*ast.AttrGetExpr)
	return ok && attr.KeySyntax == ast.AttrKeyIndex
}

// optionalMemberReadType types a dot-field read whose object is a call result
// (store:lookup_record(id).field) when no symbol-path or index projection owns it.
// The call result carries its own boundary presence, so reading a field off an
// optional result yields an optional projection that an annotated assignment must
// not silently accept. It returns the type only when it is provably optional, so
// a non-optional projection never produces a spurious source type here.
func optionalMemberReadType(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, env guardEnv, expr ast.Expr) (typ.Type, bool) {
	attr, ok := expr.(*ast.AttrGetExpr)
	if !ok || attr.KeySyntax != ast.AttrKeyDot {
		return nil, false
	}
	if _, ok := attr.Object.(*ast.FuncCallExpr); !ok {
		return nil, false
	}
	got, ok := newFlowExpressionTyper(result, resolver, point, env).typeOf(expr)
	if !ok || got == nil {
		return nil, false
	}
	if typ.IsAny(got) || typ.IsUnknown(got) || typ.IsNever(got) {
		return nil, false
	}
	if !projectionHasNil(got) {
		return nil, false
	}
	return got, true
}

func projectedFlowSourceType(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, env guardEnv, expr ast.Expr) (typ.Type, bool) {
	return projectedSourceTypeWith(result, resolver, point, env, expr, newFlowExpressionTyper)
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
	case *ast.AttrGetExpr:
		if e.KeySyntax == ast.AttrKeyIndex && !shouldProjectOptionalIndex(result, e) {
			return nil, false
		}
		got, ok := newTyper(result, resolver, point, env).typeOf(expr)
		if !ok {
			return nil, false
		}
		raw, rawOK := newExpressionTyper(result, resolver).typeOf(expr)
		if !rawOK || !typ.SameNodeOrAcyclicEqual(got, raw) {
			return got, true
		}
		return nil, false
	case *ast.IdentExpr:
		got, ok := newTyper(result, resolver, point, env).typeOf(expr)
		if !ok {
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

func dominatingDeclarationProjectionType(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, expr ast.Expr) (typ.Type, bool) {
	if result == nil || expr == nil {
		return nil, false
	}
	accessPath, ok := result.ExpressionPath(expr)
	if !ok || accessPath.Symbol == 0 || len(accessPath.Segments) == 0 {
		return nil, false
	}
	if _, annotated := result.SymbolTypeAnnotation(accessPath.Symbol); annotated {
		return nil, false
	}
	root, ok := dominatingRootDeclarationType(result, resolver, point, accessPath.Symbol)
	if !ok {
		return nil, false
	}
	got, ok := expectedTypeAtSegments(root, accessPath.Segments)
	if !ok {
		return nil, false
	}
	if value, ok := result.ExpressionValueAtBoundary(point, expr); !ok || !boundaryValueHasReadableType(result, value) {
		got = normalize.Optional(got)
	}
	return got, true
}

func boundaryValueHasReadableType(result *body.Result, value product.Value) bool {
	_, ok := readmodel.New(result).ValueType(value)
	return ok
}

func dominatingRootDeclarationType(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, target symbol.ID) (typ.Type, bool) {
	graph := result.Graph()
	if graph == nil || target == 0 {
		return nil, false
	}
	idom := dominance.ComputeImmediateDominatorInfo(graph).Map()
	visited := make(map[cfg.Point]struct{}, graph.Size())
	for cursor := point; ; {
		if _, ok := visited[cursor]; ok {
			return nil, false
		}
		visited[cursor] = struct{}{}
		if fact, ok := result.OrdinaryAssignment(cursor); ok && fact.HasSymbol && fact.Symbol == target && (!fact.HasPath || len(fact.Path.Segments) == 0) {
			return nil, false
		}
		if fact, ok := result.LocalAssignment(cursor); ok && fact.HasSymbol && fact.Symbol == target {
			return localDeclarationType(result, resolver, cursor, fact)
		}
		parent, ok := idom[cursor]
		if !ok || parent == cursor {
			return nil, false
		}
		cursor = parent
	}
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
	return readmodel.New(result).SourceType(point, fact.Source)
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
	return readmodel.New(result).RefineDeclaredType(declared, value)
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
	value, ok := result.ExpressionValueAtBoundary(point, expr)
	if !ok {
		return got
	}
	refined, ok := readmodel.New(result).RefineDeclaredType(got, value)
	if !ok {
		return got
	}
	if !topLikeType(got) && !subtype.IsSubtype(refined, got) {
		return got
	}
	return refined
}
