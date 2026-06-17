package diagnostics

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/diagnostics/internal/readmodel"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/typeannotation"
	"github.com/wippyai/go-lua/analysis/lua/typecall"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/refinement"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
	"github.com/wippyai/go-lua/compiler/ast"
)

// directCallContract reports contract mismatches for direct function calls that
// resolve to a known callable contract.
type directCallContract producerContext

func (p directCallContract) Produce(result *body.Result) []diagnostic.Diagnostic {
	return produceDirectCallContract(result, producerContext(p), nil)
}

func produceDirectCallContract(result *body.Result, context producerContext, inherited map[symbol.ID]*ast.FunctionExpr) []diagnostic.Diagnostic {
	if result == nil {
		return nil
	}
	graph := result.Graph()
	if graph == nil {
		return nil
	}
	defs := directCallDefinitions(result, inherited)
	envs := guardEnvironments(result)
	producer := directCallContract(context)
	var out []diagnostic.Diagnostic
	for _, point := range graph.RPO() {
		fact, ok := result.Call(point)
		if !ok || fact.Call == nil {
			continue
		}
		site, ok := result.CallSite(point)
		if !ok || site.CalleeSymbol() == 0 {
			continue
		}
		if _, _, _, member := callMemberAccess(fact); member {
			if !hasTypedCallSignature(result, site) {
				continue
			}
			if d, ok := memberCall(context).typedSignatureStructuralDiagnostic(result, point, fact, envs[point]); ok {
				out = append(out, d)
				continue
			}
		}
		d, ok := producer.call(result, point, fact, site, defs[site.CalleeSymbol()], defs, envs[point])
		if !ok {
			continue
		}
		out = append(out, d)
	}
	return out
}

func (p directCallContract) call(
	result *body.Result,
	point cfg.Point,
	fact semantics.CallFact,
	site factflow.CallSite,
	def *ast.FunctionExpr,
	defs map[symbol.ID]*ast.FunctionExpr,
	env guardEnv,
) (diagnostic.Diagnostic, bool) {
	name := result.SymbolName(site.CalleeSymbol())
	if name == "" {
		name = "call target"
	}

	if d, ok := p.possiblyNilCallee(result, point, fact, name); ok {
		return d, true
	}

	if def != nil {
		return p.callFunction(result, point, fact, name, def, defs, env)
	}

	if sig, ok := result.CallSignature(site); ok && sig.Type != nil {
		contract := lowerDirectFunctionType(sig.Type)
		contract.name = name
		contract.declSpan = ast.SpanOf(fact.Call)
		return p.directFunctionCall(result, point, fact, contract, defs, env)
	}

	baseExpr, ok := result.SymbolTypeAnnotation(site.CalleeSymbol())
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	baseType, ok := lowerType(baseExpr, p.resolver)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	if typ.IsAny(baseType) || typ.IsUnknown(baseType) {
		return diagnostic.Diagnostic{}, false
	}
	callable, ok := typecall.Callable(baseType)
	if !ok {
		return directNotCallableDiagnostic(point, fact.Call, name, baseType), true
	}
	contract := lowerDirectFunctionType(callable)
	contract.name = name
	contract.declSpan = ast.SpanOf(fact.Call)
	return p.directFunctionCall(result, point, fact, contract, defs, env)
}

// possiblyNilCallee flags a direct call whose callee value is possibly nil
// after flow narrowing. A member access is owned by the member-call producer.
func (p directCallContract) possiblyNilCallee(
	result *body.Result,
	point cfg.Point,
	fact semantics.CallFact,
	name string,
) (diagnostic.Diagnostic, bool) {
	if _, _, _, member := callMemberAccess(fact); member {
		return diagnostic.Diagnostic{}, false
	}
	if fact.Func == nil || fact.Call == nil {
		return diagnostic.Diagnostic{}, false
	}
	calleeType, ok := calleeFlowType(result, p.resolver, point, fact.Func)
	if !ok {
		if t, ok := boundaryMaybeNilCalleeType(result, point, fact.Func); ok {
			return directPossiblyNilCalleeDiagnostic(point, fact.Call, name, t), true
		}
		return diagnostic.Diagnostic{}, false
	}
	if typ.IsAny(calleeType) || typ.IsUnknown(calleeType) || typ.IsNever(calleeType) {
		return diagnostic.Diagnostic{}, false
	}
	if !projectionHasNil(calleeType) {
		return diagnostic.Diagnostic{}, false
	}
	return directPossiblyNilCalleeDiagnostic(point, fact.Call, name, calleeType), true
}

// calleeFlowType resolves the callee value's type with nil presence after flow
// narrowing. The post-solve boundary value reflects truthiness guards and is
// available for inferred callees; the flow typer covers annotated callees.
func calleeFlowType(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, callee ast.Expr) (typ.Type, bool) {
	if value, ok := result.ExpressionValueAtBoundary(point, callee); ok {
		if t, ok := readmodel.New(result).ValueTypeWithPresence(value); ok {
			return t, true
		}
	}
	return newFlowExpressionTyper(result, resolver, point, guardEnv{}).typeOf(callee)
}

// boundaryMaybeNilCalleeType detects a callee whose flow value is possibly nil
// when no concrete type witness is recoverable. The presence axis carries the
// narrowing-aware nil signal: Maybe means the value may be nil at the call, so
// the callee is possibly-nil even when its type cannot be pinned. Present or
// Absent presence (e.g. after a truthiness guard) does not flag here.
func boundaryMaybeNilCalleeType(result *body.Result, point cfg.Point, callee ast.Expr) (typ.Type, bool) {
	value, ok := result.ExpressionValueAtBoundary(point, callee)
	if !ok {
		return nil, false
	}
	if !presence.Equal(product.PresenceOf(value), presence.Maybe()) {
		return nil, false
	}
	inner := typ.Unknown
	if t, ok := readmodel.New(result).ValueType(value); ok && t != nil && !typ.IsUnknown(t) && !typ.IsAny(t) {
		inner = t
	}
	if projectionHasNil(inner) {
		return inner, true
	}
	return normalize.Optional(inner), true
}

func directPossiblyNilCalleeDiagnostic(point cfg.Point, call *ast.FuncCallExpr, name string, calleeType typ.Type) diagnostic.Diagnostic {
	span := ast.SpanOf(call)
	return diagnostic.Diagnostic{
		Position: diagnostic.Position{
			Line:      span.StartLine,
			Column:    span.StartCol,
			EndLine:   span.EndLine,
			EndColumn: span.EndCol,
		},
		Span:     span,
		Code:     CodeNotCallable,
		Severity: diagnostic.SeverityError,
		Message:  fmt.Sprintf("%s is %s, possibly nil and not callable", name, formatType(calleeType)),
		Explanation: diagnostic.NewExplanation(
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnostic.TrustProven,
				Span:    span,
				Message: fmt.Sprintf("call at CFG point %d resolves to %s", point, name),
			},
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnostic.TrustProven,
				Span:    span,
				Message: fmt.Sprintf("%s is %s at the call", name, formatType(calleeType)),
			},
		),
		Labels: []diagnostic.Label{{Span: span, Message: "possibly-nil call target"}},
	}
}

type directFunctionContract struct {
	name      string
	declSpan  ast.Span
	source    *typ.Function
	params    []directCallParam
	returns   []directCallResult
	variadic  directCallParam
	hasVararg bool
}

type directCallParam struct {
	typ      typ.Type
	explicit bool
	optional bool
}

type directCallResult struct {
	typ      typ.Type
	explicit bool
}

func (p directCallContract) callFunction(
	result *body.Result,
	point cfg.Point,
	fact semantics.CallFact,
	name string,
	fn *ast.FunctionExpr,
	defs map[symbol.ID]*ast.FunctionExpr,
	env guardEnv,
) (diagnostic.Diagnostic, bool) {
	contract, ok := lowerDirectFunctionContractInScope(fn, p.resolver)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	contract.name = name
	contract.declSpan = ast.SpanOf(fn)
	return p.directFunctionCall(result, point, fact, contract, defs, env)
}

func (p directCallContract) directFunctionCall(
	result *body.Result,
	point cfg.Point,
	fact semantics.CallFact,
	contract directFunctionContract,
	defs map[symbol.ID]*ast.FunctionExpr,
	env guardEnv,
) (diagnostic.Diagnostic, bool) {
	call := fact.Call
	if call == nil {
		return diagnostic.Diagnostic{}, false
	}
	args := call.Args
	required := contract.requiredArity()
	if len(args) < required {
		return tooFewArgsDiagnostic(point, call, contract.name, required, len(args), contract.declSpan), true
	}
	if !contract.hasVararg && len(args) > len(contract.params) {
		return tooManyArgsDiagnostic(point, call, contract.name, len(contract.params), len(args), contract.declSpan, args[len(contract.params)]), true
	}
	contract, violations := instantiateDirectFunctionContract(result, point, fact, contract, p.resolver, defs)
	if len(violations) > 0 {
		violation := violations[0]
		if violation.Index >= 0 && violation.Index < len(args) {
			return argTypeDiagnostic(call, contract.name, violation.Index, violation.Got, violation.Constraint, args[violation.Index], contract.declSpan), true
		}
	}
	for i, arg := range args {
		var want typ.Type
		if i < len(contract.params) {
			param := contract.params[i]
			want = param.typ
			if !param.explicit || want == nil || typ.IsAny(want) || typ.IsUnknown(want) {
				continue
			}
		} else if contract.hasVararg {
			want = contract.variadic.typ
			if !contract.variadic.explicit || want == nil || typ.IsAny(want) || typ.IsUnknown(want) {
				continue
			}
		} else {
			break
		}
		if refinement.ContainsFreeTypeParam(want) {
			continue
		}
		if mismatch, ok := objectLiteralMemberMismatch(result, point, arg, want, env); ok {
			extra := boundaryDiagnosticEvidence(result, point, ast.SpanOf(mismatch.expr), mismatch.want, boundaryValueFromExpr(mismatch.expr))
			return argTypeDiagnostic(call, contract.name, i, mismatch.got, mismatch.want, mismatch.expr, contract.declSpan, extra...), true
		}
		got, ok := untrustedTopLikeExpressionTypeAt(result, p.resolver, point, arg)
		untrustedTopLike := ok
		if !ok {
			got, ok = projectedFlowSourceType(result, p.resolver, point, guardEnv{}, arg)
		}
		if !ok {
			got, ok = boundaryCallArgumentSourceType(result, point, fact, i)
		}
		if !ok {
			got, ok = boundaryExprType(result, p.resolver, arg)
		}
		if !ok {
			continue
		}
		readBoundary := boundaryCallArgumentReader(fact, i, arg)
		if untrustedTopLike {
			if !boundaryProofTypeMismatch(result, point, got, want, readBoundary) {
				continue
			}
		} else if !directCallArgumentTypeMismatch(result, point, got, want, readBoundary) {
			continue
		}
		extra := boundaryDiagnosticEvidence(result, point, ast.SpanOf(arg), want, readBoundary)
		if len(extra) == 0 {
			extra = explicitTopLikeCastEvidence(ast.SpanOf(arg), want, arg)
		}
		return argTypeDiagnostic(call, contract.name, i, got, want, arg, contract.declSpan, extra...), true
	}
	return diagnostic.Diagnostic{}, false
}

func directCallArgumentTypeMismatch(result *body.Result, point cfg.Point, got, want typ.Type, read boundaryValueReader) bool {
	if !boundaryTypeMismatch(result, point, got, want, nil) {
		return false
	}
	if read != nil {
		if value, ok := read(result, point); ok && readmodel.New(result).ValueAdmissible(value, want) {
			return false
		}
	}
	return true
}

func boundaryCallArgumentSourceType(result *body.Result, point cfg.Point, fact semantics.CallFact, index int) (typ.Type, bool) {
	if index < 0 || index >= len(fact.ArgumentSources) {
		return nil, false
	}
	return readmodel.New(result).SourceType(point, fact.ArgumentSources[index])
}

func boundaryCallArgumentReader(fact semantics.CallFact, index int, argumentExpr ast.Expr) boundaryValueReader {
	return func(result *body.Result, point cfg.Point) (product.Value, bool) {
		if index >= 0 && index < len(fact.ArgumentSources) {
			if value, ok := boundaryValueFromASTSource(fact.ArgumentSources[index])(result, point); ok {
				return value, true
			}
		}
		return boundaryValueFromExpr(argumentExpr)(result, point)
	}
}

func lowerDirectFunctionContract(fn *ast.FunctionExpr, resolver typeannotation.Resolver) (directFunctionContract, bool) {
	if fn == nil {
		return directFunctionContract{}, false
	}
	if fnType, ok := lowerFunctionExprType(fn, resolver); ok {
		return lowerDirectFunctionType(fnType), true
	}
	contract := directFunctionContract{
		params:  make([]directCallParam, 0),
		returns: make([]directCallResult, 0),
	}
	if fn.ParList != nil {
		contract.params = make([]directCallParam, 0, len(fn.ParList.Names))
		for i := range fn.ParList.Names {
			param, ok := lowerDirectCallParam(typeExprAt(fn.ParList.Types, i), resolver)
			if !ok {
				return directFunctionContract{}, false
			}
			contract.params = append(contract.params, param)
		}
		if fn.ParList.HasVargs {
			variadic, ok := lowerDirectCallParam(fn.ParList.VarargType, resolver)
			if !ok {
				return directFunctionContract{}, false
			}
			contract.hasVararg = true
			contract.variadic = variadic
		}
	}
	contract.returns = make([]directCallResult, 0, len(fn.ReturnTypes))
	for _, retExpr := range fn.ReturnTypes {
		ret, ok := lowerDirectCallResult(retExpr, resolver)
		if !ok {
			return directFunctionContract{}, false
		}
		contract.returns = append(contract.returns, ret)
	}
	return contract, true
}

func lowerDirectFunctionContractInScope(fn *ast.FunctionExpr, resolver typeannotation.Resolver) (directFunctionContract, bool) {
	for current := resolver; current != nil; current = parentTypeResolver(current) {
		if contract, ok := lowerDirectFunctionContract(fn, current); ok {
			return contract, true
		}
	}
	return lowerDirectFunctionContract(fn, nil)
}

func parentTypeResolver(resolver typeannotation.Resolver) typeannotation.Resolver {
	if r, ok := resolver.(*resultResolver); ok {
		return r.parent
	}
	return nil
}

func lowerDirectFunctionType(fn *typ.Function) directFunctionContract {
	contract := directFunctionContract{
		source:  fn,
		params:  make([]directCallParam, 0, len(fn.Params)),
		returns: make([]directCallResult, 0, len(fn.Returns)),
	}
	for _, param := range fn.Params {
		optional := param.Optional || isOptionalType(param.Type)
		explicit := param.Type != nil && !typ.IsAny(param.Type) && !typ.IsUnknown(param.Type)
		if !explicit {
			optional = true
		}
		contract.params = append(contract.params, directCallParam{
			typ:      param.Type,
			explicit: explicit,
			optional: optional,
		})
	}
	for _, ret := range fn.Returns {
		contract.returns = append(contract.returns, directCallResult{
			typ:      ret,
			explicit: ret != nil && !typ.IsAny(ret) && !typ.IsUnknown(ret),
		})
	}
	if fn.Variadic != nil {
		contract.hasVararg = true
		contract.variadic = directCallParam{
			typ:      fn.Variadic,
			explicit: !typ.IsAny(fn.Variadic) && !typ.IsUnknown(fn.Variadic),
			optional: isOptionalType(fn.Variadic) || typ.IsAny(fn.Variadic) || typ.IsUnknown(fn.Variadic),
		}
	}
	return contract
}

func lowerFunctionExprType(fn *ast.FunctionExpr, resolver typeannotation.Resolver) (*typ.Function, bool) {
	if fn == nil {
		return nil, false
	}
	expr := &ast.FunctionTypeExpr{
		TypeParams: fn.TypeParams,
		Returns:    fn.ReturnTypes,
	}
	if fn.ParList != nil {
		expr.Params = make([]ast.FunctionParamExpr, 0, len(fn.ParList.Names))
		for i, name := range fn.ParList.Names {
			t := typeExprAt(fn.ParList.Types, i)
			if t == nil {
				return nil, false
			}
			expr.Params = append(expr.Params, ast.FunctionParamExpr{Name: name, Type: t})
		}
		if fn.ParList.HasVargs {
			if fn.ParList.VarargType == nil {
				return nil, false
			}
			expr.Variadic = fn.ParList.VarargType
		}
	}
	lowered, ok := lowerType(expr, resolver)
	if !ok {
		return nil, false
	}
	fnType, ok := lowered.(*typ.Function)
	return fnType, ok
}

func lowerDirectCallResult(expr ast.TypeExpr, resolver typeannotation.Resolver) (directCallResult, bool) {
	if expr == nil {
		return directCallResult{}, true
	}
	t, ok := lowerType(expr, resolver)
	if !ok {
		return directCallResult{}, false
	}
	return directCallResult{
		typ:      t,
		explicit: !typ.IsAny(t) && !typ.IsUnknown(t),
	}, true
}

func lowerDirectCallParam(expr ast.TypeExpr, resolver typeannotation.Resolver) (directCallParam, bool) {
	if expr == nil {
		return directCallParam{optional: true}, true
	}
	t, ok := lowerType(expr, resolver)
	if !ok {
		return directCallParam{}, false
	}
	explicit := !typ.IsAny(t) && !typ.IsUnknown(t)
	optional := !explicit || isOptionalType(t)
	return directCallParam{
		typ:      t,
		explicit: explicit,
		optional: optional,
	}, true
}

func (c directFunctionContract) requiredArity() int {
	required := 0
	for i, param := range c.params {
		if param.explicit && !param.optional {
			required = i + 1
		}
	}
	return required
}

func (c directFunctionContract) returnType(index int) (typ.Type, bool) {
	if index < 0 || index >= len(c.returns) {
		return nil, false
	}
	ret := c.returns[index]
	if !ret.explicit || ret.typ == nil || typ.IsAny(ret.typ) || typ.IsUnknown(ret.typ) {
		return nil, false
	}
	return ret.typ, true
}

func (c directFunctionContract) declaredReturnType(index int) (typ.Type, bool) {
	if index < 0 || index >= len(c.returns) {
		return nil, false
	}
	ret := c.returns[index]
	return ret.typ, ret.typ != nil
}

func typeExprAt(exprs []ast.TypeExpr, index int) ast.TypeExpr {
	if index >= 0 && index < len(exprs) {
		return exprs[index]
	}
	return nil
}

func isOptionalType(t typ.Type) bool {
	if t == nil {
		return false
	}
	_, ok := unwrap.Annotated(t).(*typ.Optional)
	return ok
}

func directNotCallableDiagnostic(point cfg.Point, call *ast.FuncCallExpr, name string, calleeType typ.Type) diagnostic.Diagnostic {
	span := ast.SpanOf(call)
	return diagnostic.Diagnostic{
		Position: diagnostic.Position{
			Line:      span.StartLine,
			Column:    span.StartCol,
			EndLine:   span.EndLine,
			EndColumn: span.EndCol,
		},
		Span:     span,
		Code:     CodeDirectCallNotCallable,
		Severity: diagnostic.SeverityError,
		Message:  fmt.Sprintf("%s is %s, not callable", name, formatType(calleeType)),
		Explanation: diagnostic.NewExplanation(
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnostic.TrustProven,
				Span:    span,
				Message: fmt.Sprintf("call at CFG point %d resolves to %s", point, name),
			},
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceUserAssertion,
				Trust:   diagnostic.TrustClaimed,
				Span:    span,
				Message: fmt.Sprintf("%s is annotated %s", name, formatType(calleeType)),
			},
		),
		Labels: []diagnostic.Label{{Span: span, Message: "non-callable call target"}},
	}
}

func tooFewArgsDiagnostic(point cfg.Point, call *ast.FuncCallExpr, name string, want, got int, declSpan ast.Span) diagnostic.Diagnostic {
	span := ast.SpanOf(call)
	return diagnostic.Diagnostic{
		Position: diagnostic.Position{
			Line:      span.StartLine,
			Column:    span.StartCol,
			EndLine:   span.EndLine,
			EndColumn: span.EndCol,
		},
		Span:     span,
		Code:     CodeDirectCallTooFewArgs,
		Severity: diagnostic.SeverityError,
		Message:  fmt.Sprintf("%s expects %d arguments, got %d", name, want, got),
		Explanation: diagnostic.NewExplanation(
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnostic.TrustProven,
				Span:    span,
				Message: fmt.Sprintf("call at CFG point %d passes %d arguments", point, got),
			},
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceUserAssertion,
				Trust:   diagnostic.TrustClaimed,
				Span:    declSpan,
				Message: fmt.Sprintf("%s declares %d parameters", name, want),
			},
		),
		Labels: []diagnostic.Label{{Span: span, Message: "too few arguments"}},
	}
}

func tooManyArgsDiagnostic(point cfg.Point, call *ast.FuncCallExpr, name string, want, got int, declSpan ast.Span, extra ast.Expr) diagnostic.Diagnostic {
	span := ast.SpanOf(call)
	extraSpan := ast.SpanOf(extra)
	return diagnostic.Diagnostic{
		Position: diagnostic.Position{
			Line:      span.StartLine,
			Column:    span.StartCol,
			EndLine:   span.EndLine,
			EndColumn: span.EndCol,
		},
		Span:     span,
		Code:     CodeDirectCallTooManyArgs,
		Severity: diagnostic.SeverityError,
		Message:  fmt.Sprintf("%s expects %d arguments, got %d", name, want, got),
		Explanation: diagnostic.NewExplanation(
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnostic.TrustProven,
				Span:    span,
				Message: fmt.Sprintf("call at CFG point %d passes %d arguments", point, got),
			},
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceUserAssertion,
				Trust:   diagnostic.TrustClaimed,
				Span:    declSpan,
				Message: fmt.Sprintf("%s declares %d parameters", name, want),
			},
		),
		Labels: []diagnostic.Label{{Span: extraSpan, Message: "extra argument"}},
	}
}

func argTypeDiagnostic(call *ast.FuncCallExpr, name string, index int, got, want typ.Type, arg ast.Expr, declSpan ast.Span, extraEvidence ...diagnostic.Evidence) diagnostic.Diagnostic {
	span := ast.SpanOf(call)
	argSpan := ast.SpanOf(arg)
	evidence := []diagnostic.Evidence{
		{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Span:    argSpan,
			Message: fmt.Sprintf("argument %d is %s", index+1, formatType(got)),
		},
		{
			Kind:    diagnostic.EvidenceUserAssertion,
			Trust:   diagnostic.TrustClaimed,
			Span:    declSpan,
			Message: fmt.Sprintf("%s parameter %d declares %s", name, index+1, formatType(want)),
		},
	}
	evidence = append(evidence, extraEvidence...)
	return diagnostic.Diagnostic{
		Position: diagnostic.Position{
			Line:      span.StartLine,
			Column:    span.StartCol,
			EndLine:   span.EndLine,
			EndColumn: span.EndCol,
		},
		Span:        span,
		Code:        CodeDirectCallArgType,
		Severity:    diagnostic.SeverityError,
		Message:     fmt.Sprintf("argument %d is %s, not %s", index+1, formatType(got), formatType(want)),
		Explanation: diagnostic.NewExplanation(evidence...),
		Labels:      []diagnostic.Label{{Span: argSpan, Message: "argument value"}},
	}
}

func directCallDefinitions(result *body.Result, parent map[symbol.ID]*ast.FunctionExpr) map[symbol.ID]*ast.FunctionExpr {
	if result == nil {
		return parent
	}
	graph := result.Graph()
	if graph == nil {
		return parent
	}
	var out map[symbol.ID]*ast.FunctionExpr
	if len(parent) != 0 {
		out = make(map[symbol.ID]*ast.FunctionExpr, len(parent))
		for id, fn := range parent {
			out[id] = fn
		}
	}
	for _, point := range graph.RPO() {
		if fact, ok := result.LocalAssignment(point); ok && fact.HasSymbol && fact.Symbol != 0 {
			if fn, ok := fact.Expr.(*ast.FunctionExpr); ok {
				if out == nil {
					out = make(map[symbol.ID]*ast.FunctionExpr)
				}
				out[fact.Symbol] = fn
			}
		}
		if fact, ok := result.OrdinaryAssignment(point); ok && fact.HasSymbol && fact.Symbol != 0 {
			if fn, ok := fact.Value.(*ast.FunctionExpr); ok {
				if out == nil {
					out = make(map[symbol.ID]*ast.FunctionExpr)
				}
				out[fact.Symbol] = fn
			}
		}
		fact, ok := result.FunctionDefinition(point)
		if !ok || !fact.HasTargetSymbol || fact.TargetSymbol == 0 || fact.Func == nil {
			continue
		}
		if out == nil {
			out = make(map[symbol.ID]*ast.FunctionExpr)
		}
		out[fact.TargetSymbol] = fact.Func
	}
	if len(out) == 0 {
		return parent
	}
	return out
}
