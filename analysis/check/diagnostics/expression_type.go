package diagnostics

import (
	"strings"

	"github.com/wippyai/go-lua/analysis/check/body"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekindof"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/typeannotation"
	"github.com/wippyai/go-lua/analysis/lua/typecall"
	"github.com/wippyai/go-lua/analysis/lua/typeoperator"
	luatypeprojection "github.com/wippyai/go-lua/analysis/lua/typeprojection"
	"github.com/wippyai/go-lua/analysis/lua/valueexpr"
	"github.com/wippyai/go-lua/analysis/type/access"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
	"github.com/wippyai/go-lua/compiler/ast"
)

type expressionTyper struct {
	result        *body.Result
	resolver      typeannotation.Resolver
	point         cfg.Point
	env           guardEnv
	flow          bool
	witnessRefine bool
}

func newExpressionTyper(result *body.Result, resolver typeannotation.Resolver) expressionTyper {
	return expressionTyper{result: result, resolver: resolver}
}

func newFlowExpressionTyper(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, env guardEnv) expressionTyper {
	return expressionTyper{result: result, resolver: resolver, point: point, env: env, flow: true, witnessRefine: true}
}

// newStructuralFlowExpressionTyper types expressions under active flow narrowing
// but suppresses structural witness refinement, so the result reflects the
// declared/origin type narrowed only by sound discriminant narrowing. This keeps
// absence proofs from trusting a partial observed table-literal snapshot that may
// omit declared fields.
func newStructuralFlowExpressionTyper(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, env guardEnv) expressionTyper {
	return expressionTyper{result: result, resolver: resolver, point: point, env: env, flow: true, witnessRefine: false}
}

func (p expressionTyper) typeOf(expr ast.Expr) (typ.Type, bool) {
	return p.typeOfDepth(expr, 0)
}

func (p expressionTyper) typeOfDepth(expr ast.Expr, depth int) (typ.Type, bool) {
	if expr == nil || depth > typ.DefaultRecursionDepth {
		return nil, false
	}
	if t, ok := valueexpr.LiteralType(expr); ok {
		return t, true
	}
	switch e := expr.(type) {
	case *ast.CastExpr:
		// `expr :: T` is a runtime value cast (like Type(x)): it does not prove the
		// type and may fail at runtime, but the linter adopts the asserted concrete
		// type T for inference, so `(x :: {id: string}).id` types as string. A cast
		// to a top-like type (any/unknown) carries no usable type, so it falls
		// through to the underlying expression -- the explicit-any boundary checks
		// (e.g. numeric-for operand) must still see through it as unproven.
		if e.Type != nil {
			if lowered, ok := lowerType(e.Type, p.resolver); ok && !topLikeType(lowered) {
				return lowered, true
			}
		}
		return p.typeOfDepth(e.Expr, depth+1)
	case *ast.NonNilAssertExpr:
		t, ok := p.typeOfDepth(e.Expr, depth+1)
		if !ok {
			return nil, false
		}
		return projectionWithoutNil(t), true
	case *ast.IdentExpr:
		return p.annotatedPathType(e)
	case *ast.FuncCallExpr:
		return p.callResultType(e)
	case *ast.AttrGetExpr:
		return p.attrType(e, depth+1)
	case *ast.LogicalOpExpr:
		return p.binaryType(e.Lhs, e.Operator, e.Rhs, depth+1)
	case *ast.RelationalOpExpr:
		return p.binaryType(e.Lhs, e.Operator, e.Rhs, depth+1)
	case *ast.StringConcatOpExpr:
		return p.binaryType(e.Lhs, "..", e.Rhs, depth+1)
	case *ast.ArithmeticOpExpr:
		return p.binaryType(e.Lhs, e.Operator, e.Rhs, depth+1)
	case *ast.UnaryMinusOpExpr:
		return p.unaryType("-", e.Expr, depth+1)
	case *ast.UnaryNotOpExpr:
		return p.unaryType("not", e.Expr, depth+1)
	case *ast.UnaryLenOpExpr:
		return p.unaryType("#", e.Expr, depth+1)
	case *ast.UnaryBNotOpExpr:
		return p.unaryType("~", e.Expr, depth+1)
	default:
		return nil, false
	}
}

func (p expressionTyper) annotatedPathType(expr ast.Expr) (typ.Type, bool) {
	accessPath, ok := p.result.ExpressionPath(expr)
	if !ok || accessPath.Symbol == 0 {
		return nil, false
	}
	annotation, ok := p.result.SymbolTypeAnnotation(accessPath.Symbol)
	var t typ.Type
	if ok {
		lowered, loweredOK := lowerType(annotation, p.resolver)
		if !loweredOK {
			return nil, false
		}
		t = transparentComparableType(p.result, lowered)
	} else if p.flow {
		lowered, loweredOK := p.flowPathType(accessPath)
		if !loweredOK {
			return nil, false
		}
		t = lowered
	} else {
		return nil, false
	}
	if p.flow {
		t = p.flowRootType(t, accessPath)
	}
	current := accessPath.RootOnly()
	for _, seg := range accessPath.Segments {
		next, ok := expressionSegmentType(t, seg)
		if !ok {
			return nil, false
		}
		t = next
		current = current.Append(seg)
		if p.flow {
			if narrowed, ok := applyRuntimeTypeNarrowing(t, current, p.env, p.resolver); ok {
				t = narrowed
			}
			if narrowed, ok := applyLiteralNarrowing(t, current, p.env); ok {
				t = narrowed
			}
		}
	}
	if p.flow {
		if narrowed, ok := p.literalNarrowedBroadType(expr, accessPath, t); ok {
			t = narrowed
		}
	}
	return p.refineFlowExpressionType(expr, t), true
}

func (p expressionTyper) literalNarrowedBroadType(expr ast.Expr, accessPath pathdom.Path, current typ.Type) (typ.Type, bool) {
	if current == nil || accessPath.IsEmpty() || (len(p.env.constraints) == 0 && len(p.env.truthy) == 0 && len(p.env.falsy) == 0) {
		return nil, false
	}
	broad, ok := p.broadType(expr)
	if !ok || broad == nil || !newDiagnosticQuery(p.result).IsEquivalent(current, broad) {
		return nil, false
	}
	return applyLiteralNarrowing(broad, accessPath, p.env)
}

// broadType returns the un-narrowed declared shape of a path expression: the
// lowered root annotation when present, otherwise the full variant-origin
// family, projected through the path suffix. It reflects what the receiver
// could be before any flow narrowing, so callers can tell discriminant collapse
// apart from a single-shape observed snapshot.
func (p expressionTyper) broadType(expr ast.Expr) (typ.Type, bool) {
	accessPath, ok := p.result.ExpressionPath(expr)
	if !ok || accessPath.Symbol == 0 {
		return nil, false
	}
	var t typ.Type
	if annotation, ok := p.result.SymbolTypeAnnotation(accessPath.Symbol); ok {
		lowered, loweredOK := lowerType(annotation, p.resolver)
		if !loweredOK {
			return nil, false
		}
		t = transparentComparableType(p.result, lowered)
	} else {
		value, ok := p.result.SymbolValueAtBoundary(p.point, accessPath.Symbol)
		if !ok {
			return nil, false
		}
		full, ok := newDiagnosticQuery(p.result).FullVariantOriginType(value)
		if !ok {
			return nil, false
		}
		t = full
	}
	for _, seg := range accessPath.Segments {
		next, ok := expressionSegmentType(t, seg)
		if !ok {
			return nil, false
		}
		t = next
	}
	return t, t != nil
}

func (p expressionTyper) flowPathType(accessPath pathdom.Path) (typ.Type, bool) {
	if p.result == nil || accessPath.Symbol == 0 {
		return nil, false
	}
	value, ok := p.result.SymbolValueAtBoundary(p.point, accessPath.Symbol)
	if !ok {
		return nil, false
	}
	query := newDiagnosticQuery(p.result)
	if query.ValueHasUntrustedTopOrigin(value) {
		return nil, false
	}
	if t, ok := query.VariantOriginType(value); ok {
		return t, true
	}
	return query.ValueTypeWithPresence(value)
}

// freshRecordAbsentFieldType types a dot-field read whose name is provably
// absent from a local-exclusive exact table value. A non-discriminated table
// local (or a tracked indexed parent such as dogs[1]) can carry a complete
// concrete witness without variant origin; when that exact parent is still local
// and has no field, Lua reads nil. Imported, escaped, or opaque values are left
// to the other producers, where the modeled type may omit reachable members.
func (p expressionTyper) freshRecordAbsentFieldType(expr ast.Expr) (typ.Type, bool) {
	if p.result == nil {
		return nil, false
	}
	attr, ok := expr.(*ast.AttrGetExpr)
	if !ok || attr.KeySyntax != ast.AttrKeyDot {
		return nil, false
	}
	name := ast.KeyName(attr.Key)
	if name == "" {
		return nil, false
	}
	query := newDiagnosticQuery(p.result)
	value, ok := query.ExpressionValueAtBoundary(p.point, attr.Object)
	if !ok {
		return nil, false
	}
	if !query.ValueHasLocalExclusiveExactIdentity(p.point, value) {
		return nil, false
	}
	t, ok := query.ValueType(value)
	if !ok || t == nil {
		return nil, false
	}
	if _, ok := access.Field(t, name); ok {
		return nil, false
	}
	if !recordProvablyMissesField(t, name) {
		return nil, false
	}
	return typ.Nil, true
}

// recordProvablyMissesField reports whether t is a plain Lua table record that
// provably lacks the named field. The record carries no metatable (which could
// resolve the name through __index, unmodeled by a structural field
// projection), no open tail, and no map component admitting the name, so the
// read is definitively nil.
func recordProvablyMissesField(t typ.Type, name string) bool {
	rec, ok := unwrap.Alias(t).(*typ.Record)
	if !ok || rec == nil {
		return false
	}
	if rec.Open || rec.Metatable != nil || rec.HasMapComponent() {
		return false
	}
	return rec.GetField(name) == nil && rec.GetStaticStringIndex(name) == nil
}

func (p expressionTyper) flowRootType(t typ.Type, accessPath pathdom.Path) typ.Type {
	root := rootPath(accessPath)
	if root.Symbol == 0 {
		return t
	}
	if p.witnessRefine {
		if value, ok := p.result.SymbolValueAtBoundary(p.point, root.Symbol); ok {
			if refined, ok := newDiagnosticQuery(p.result).RefineDeclaredType(t, value); ok {
				if !(projectionHasNil(refined) && !projectionHasNil(t)) {
					t = refined
				}
			}
		}
	} else if value, ok := p.result.SymbolValueAtBoundary(p.point, root.Symbol); ok {
		if refined, ok := newDiagnosticQuery(p.result).NarrowDeclaredByOrigin(t, value); ok {
			t = refined
		}
	}
	if narrowed, ok := applyLiteralNarrowing(t, root, p.env); ok {
		t = narrowed
	}
	if narrowed, ok := applyRuntimeTypeNarrowing(t, root, p.env, p.resolver); ok {
		t = narrowed
	}
	return t
}

func applyRuntimeTypeNarrowing(t typ.Type, target pathdom.Path, env guardEnv, resolver typeannotation.Resolver) (typ.Type, bool) {
	if t == nil || target.IsEmpty() {
		return nil, false
	}
	t = resolveRuntimeNarrowingRef(t, resolver)
	for _, check := range env.typeChecks {
		if !samePathIgnoringVersion(check.target, target) {
			continue
		}
		tag, ok := runtimekind.ParseTag(check.name)
		if !ok {
			continue
		}
		narrowed, changed := runtimekindof.RestrictTypeToRuntimeKind(t, runtimekind.Singleton(tag))
		if changed && narrowed != typ.Never {
			return narrowed, true
		}
	}
	return nil, false
}

func resolveRuntimeNarrowingRef(t typ.Type, resolver typeannotation.Resolver) typ.Type {
	ref, ok := t.(*typ.Ref)
	if !ok || resolver == nil || ref.Name == "" {
		return t
	}
	path := []string{ref.Name}
	if ref.Module != "" {
		path = append(strings.Split(ref.Module, "."), ref.Name)
	}
	resolved, ok := resolver.ResolveTypeRef(path)
	if !ok || resolved == nil {
		return t
	}
	return resolved
}

func samePathIgnoringVersion(left, right pathdom.Path) bool {
	if !samePathRootIgnoringVersion(left, right) || len(left.Segments) != len(right.Segments) {
		return false
	}
	for i := range left.Segments {
		if left.Segments[i] != right.Segments[i] {
			return false
		}
	}
	return true
}

func rootPath(p pathdom.Path) pathdom.Path {
	return p.RootOnly()
}

func expressionSegmentType(t typ.Type, seg segment.Segment) (typ.Type, bool) {
	switch seg.Kind {
	case segment.SegmentField:
		if field, ok := access.Field(t, seg.Name); ok {
			return field, true
		}
		if access.MissingFieldReadsNil(t) {
			return typ.Nil, true
		}
		return nil, false
	case segment.SegmentIndexString, segment.SegmentIndexInt:
		key, ok := luatypeprojection.SegmentKeyType(seg)
		if !ok {
			return nil, false
		}
		return access.RuntimeIndex(t, key)
	default:
		return nil, false
	}
}

// callResultType types the first result of a call expression from its solved
// call-result slot, preserving boundary presence so an optional result reads as
// optional. It lets index/field projections over a call result (make()[1]) be
// typed without a symbol path.
func (p expressionTyper) callResultType(call *ast.FuncCallExpr) (typ.Type, bool) {
	if p.result == nil || call == nil {
		return nil, false
	}
	value, ok := p.result.CallExprResultValue(call, 0)
	if !ok {
		return nil, false
	}
	t, ok := newDiagnosticQuery(p.result).ValueTypeWithPresence(value)
	if !ok {
		return nil, false
	}
	if topLikeType(t) {
		if declared, ok := p.declaredCallResultType(call, 0, 0); ok {
			return declared, true
		}
	}
	if projectionHasNil(t) {
		if declared, ok := p.declaredCallResultType(call, 0, 0); ok && !projectionHasNil(declared) {
			return declared, true
		}
	}
	return t, true
}

func (p expressionTyper) declaredCallResultType(call *ast.FuncCallExpr, index int, depth int) (typ.Type, bool) {
	if call == nil || depth > typ.DefaultRecursionDepth {
		return nil, false
	}
	if call.Receiver != nil && call.Method != "" {
		receiverType, ok := p.typeOfDepth(call.Receiver, depth+1)
		if !ok || receiverType == nil || typ.IsAny(receiverType) || typ.IsUnknown(receiverType) || typ.IsNever(receiverType) {
			return nil, false
		}
		if receiverPath, ok := p.result.ExpressionPath(call.Receiver); ok && (p.env.hasPresent(receiverPath) || p.env.hasTruthy(receiverPath)) {
			if withoutNil := projectionWithoutNil(receiverType); withoutNil != nil && !typ.IsNever(withoutNil) {
				receiverType = withoutNil
			}
		}
		memberType, status := memberCallType(receiverType, segment.Segment{Kind: segment.SegmentField, Name: call.Method})
		if status != typecall.MemberCallOK || memberType == nil {
			return nil, false
		}
		return declaredCallableResultType(memberType, index)
	}
	if call.Func != nil {
		if callPoint, ok := p.result.CallExprPoint(call); ok {
			if fn, ok := p.result.FunctionValueTypeAtBoundary(callPoint, call.Func); ok && fn != nil {
				declared, ok := lowerDirectFunctionType(fn).declaredReturnType(index)
				if usableDeclaredCallResultType(declared, ok) {
					return declared, true
				}
			}
			if fn, ok := p.directFunctionDefinitionAt(callPoint, call.Func); ok && fn != nil {
				if contract, ok := lowerDirectFunctionContractInResultScope(p.result, fn, p.resolver); ok {
					declared, ok := contract.declaredReturnType(index)
					if usableDeclaredCallResultType(declared, ok) {
						return declared, true
					}
				}
			}
		}
		if fn, ok := p.result.FunctionValueTypeAtBoundary(p.point, call.Func); ok && fn != nil {
			declared, ok := lowerDirectFunctionType(fn).declaredReturnType(index)
			if usableDeclaredCallResultType(declared, ok) {
				return declared, true
			}
		}
		if fn, ok := p.result.ExpressionSignatureTypeAt(p.point, call.Func); ok {
			declared, ok := lowerDirectFunctionType(fn).declaredReturnType(index)
			if usableDeclaredCallResultType(declared, ok) {
				return declared, true
			}
		}
		if calleeType, ok := p.typeOfDepth(call.Func, depth+1); ok {
			return declaredCallableResultType(calleeType, index)
		}
	}
	site, _, ok := p.result.CallOutcomeForExpr(call)
	if !ok {
		return nil, false
	}
	fn, ok := p.result.CallSignatureType(site)
	if !ok {
		return nil, false
	}
	declared, ok := lowerDirectFunctionType(fn).declaredReturnType(index)
	if !usableDeclaredCallResultType(declared, ok) {
		return nil, false
	}
	return declared, true
}

func (p expressionTyper) directFunctionDefinitionAt(point cfg.Point, expr ast.Expr) (*ast.FunctionExpr, bool) {
	if p.result == nil || expr == nil || point == 0 {
		return nil, false
	}
	accessPath, ok := p.result.ExpressionPath(expr)
	if !ok || accessPath.Symbol == 0 || len(accessPath.Segments) != 0 {
		return nil, false
	}
	if newDiagnosticFlowCache(p.result).directFunctionReassignedAfterDefinition(point, accessPath.Symbol) {
		return nil, false
	}
	return p.result.FunctionBySymbol(accessPath.Symbol)
}

func declaredCallableResultType(callee typ.Type, index int) (typ.Type, bool) {
	callable, ok := typecall.Callable(callee)
	if !ok || callable == nil {
		return nil, false
	}
	declared, ok := lowerDirectFunctionType(callable).declaredReturnType(index)
	if !usableDeclaredCallResultType(declared, ok) {
		return nil, false
	}
	return declared, true
}

func usableDeclaredCallResultType(declared typ.Type, ok bool) bool {
	if !ok || declared == nil || typ.IsAny(declared) || typ.IsUnknown(declared) || typ.IsNever(declared) {
		return false
	}
	return true
}

func (p expressionTyper) attrType(expr *ast.AttrGetExpr, depth int) (typ.Type, bool) {
	if expr == nil {
		return nil, false
	}
	container, ok := p.typeOfDepth(expr.Object, depth+1)
	if !ok {
		return nil, false
	}
	if expr.KeySyntax == ast.AttrKeyDot {
		name := ast.KeyName(expr.Key)
		if name == "" {
			return nil, false
		}
		t, ok := expressionSegmentType(container, segment.Segment{Kind: segment.SegmentField, Name: name})
		if !ok {
			return nil, false
		}
		if p.flow {
			if accessPath, ok := p.result.ExpressionPath(expr); ok {
				if narrowed, ok := p.literalNarrowedBroadType(expr, accessPath, t); ok {
					t = narrowed
				}
			}
		}
		return p.refineFlowExpressionType(expr, t), true
	}
	key, ok := p.typeOfDepth(expr.Key, depth+1)
	if !ok {
		return nil, false
	}
	t, ok := access.RuntimeIndex(container, key)
	if !ok {
		return nil, false
	}
	if p.flow {
		if accessPath, ok := p.result.ExpressionPath(expr); ok {
			if narrowed, ok := p.literalNarrowedBroadType(expr, accessPath, t); ok {
				t = narrowed
			}
		}
	}
	return p.refineFlowExpressionType(expr, t), true
}

func (p expressionTyper) refineFlowExpressionType(expr ast.Expr, t typ.Type) typ.Type {
	if !p.flow || !p.witnessRefine || p.result == nil || expr == nil || t == nil {
		return t
	}
	query := newDiagnosticQuery(p.result)
	if value, ok := query.ExpressionValueAtBoundary(p.point, expr); ok {
		if refined, ok := query.RefineDeclaredType(t, value); ok {
			if projectionHasNil(refined) && !projectionHasNil(t) {
				return t
			}
			if accessPath, ok := p.result.ExpressionPath(expr); ok &&
				p.hasLiteralNarrowingAtOrBelow(accessPath) &&
				!topLikeType(t) &&
				!query.IsSubtype(refined, t) {
				return t
			}
			t = refined
		}
	}
	if accessPath, ok := p.result.ExpressionPath(expr); ok && p.env.hasPresent(accessPath) {
		if withoutNil := projectionWithoutNil(t); withoutNil != nil && !typ.IsNever(withoutNil) {
			return withoutNil
		}
	}
	return t
}

func (p expressionTyper) hasLiteralNarrowingAtOrBelow(target pathdom.Path) bool {
	if target.IsEmpty() {
		return false
	}
	for _, constraint := range p.env.constraints {
		if constraint.negated {
			continue
		}
		if constraint.target.Equal(target) || constraint.target.HasStrictPrefix(target) {
			return true
		}
	}
	return false
}

func (p expressionTyper) unaryType(op string, expr ast.Expr, depth int) (typ.Type, bool) {
	operand, ok := p.typeOfDepth(expr, depth+1)
	if !ok {
		return nil, false
	}
	return typeoperator.UnaryOp(op, operand)
}

func (p expressionTyper) binaryType(lhs ast.Expr, op string, rhs ast.Expr, depth int) (typ.Type, bool) {
	left, ok := p.typeOfDepth(lhs, depth+1)
	if !ok {
		return nil, false
	}
	right, ok := p.typeOfDepth(rhs, depth+1)
	if !ok {
		return nil, false
	}
	return typeoperator.BinaryOp(left, op, right)
}

func projectionHasNil(t typ.Type) bool {
	return typevalue.ProjectionHasNil(t)
}

func projectionWithoutNil(t typ.Type) typ.Type {
	return typetable.PresentReadonlyEntryValue(t)
}
