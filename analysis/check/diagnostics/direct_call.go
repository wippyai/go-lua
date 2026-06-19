package diagnostics

import (
	"fmt"
	"strings"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/internal/readmodel"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/dominance"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
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
	envs := cachedGuardEnvironments(result)
	producer := directCallContract(context)
	var out []diagnostic.Diagnostic
	for _, point := range graph.RPO() {
		if !guardEnvReachableAt(envs, point) {
			continue
		}
		fact, ok := result.Call(point)
		if !ok || fact.Call == nil {
			continue
		}
		site, ok := result.CallSite(point)
		if !ok || site.CalleeSymbol() == 0 {
			continue
		}
		if directCallSiteUsesMemberAccess(result, site, fact) {
			if access, ok := callMemberAccessInfo(fact); ok {
				switch memberCallInvalidationByPriorCall(result, context.flow, point, access.receiver, access.member) {
				case memberCallInvalidationStale:
					if d, ok := memberCall(context).call(result, point, fact, envs[point]); ok {
						out = append(out, d)
					}
					continue
				case memberCallInvalidationResolved:
					if d, ok := memberCall(context).call(result, point, fact, envs[point]); ok {
						out = append(out, d)
						continue
					}
				}
			}
			if hasTypedCallSignature(result, site) {
				if d, ok := memberCall(context).typedSignatureStructuralDiagnostic(result, point, fact, envs[point]); ok {
					out = append(out, d)
					continue
				}
			} else if _, ok := memberCall(context).call(result, point, fact, envs[point]); ok {
				continue
			} else if contract, ok := currentDirectFunctionContract(result, context, point, fact, directCallDisplayName(result, site), defs[site.CalleeSymbol()]); ok {
				if d, ok := producer.directFunctionCall(result, point, fact, contract, defs, envs[point]); ok {
					out = append(out, d)
				}
				continue
			} else {
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

func directCallSiteUsesMemberAccess(result *body.Result, site factflow.CallSite, fact semantics.CallFact) bool {
	_, _, _, member := callMemberAccess(fact)
	return member || callSiteCalleePathMemberAccess(site) || callExprCalleeMemberAccess(fact) || callSiteSymbolNameMemberAccess(result, site)
}

func callSiteCalleePathMemberAccess(site factflow.CallSite) bool {
	p := site.CalleePath()
	return !p.IsEmpty() && len(p.Segments) > 0
}

func callExprCalleeMemberAccess(fact semantics.CallFact) bool {
	if fact.Call == nil {
		return false
	}
	_, ok := fact.Call.Func.(*ast.AttrGetExpr)
	return ok
}

func callSiteSymbolNameMemberAccess(result *body.Result, site factflow.CallSite) bool {
	if result == nil || site.CalleeSymbol() == 0 {
		return false
	}
	name := result.SymbolName(site.CalleeSymbol())
	return strings.Contains(name, ".") || strings.Contains(name, "[")
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
	name := directCallDisplayName(result, site)

	if d, ok := p.possiblyNilCallee(result, point, fact, name); ok {
		return d, true
	}

	if contract, ok := currentDirectFunctionContract(result, producerContext(p), point, fact, name, def); ok {
		return p.directFunctionCall(result, point, fact, contract, defs, env)
	}

	if def != nil {
		return p.callFunction(result, point, fact, name, def, defs, env)
	}

	if sig, ok := result.CallSignature(site); ok && sig.Type != nil {
		contract := lowerDirectFunctionType(sig.Type)
		if lossyImplicitSelfMemberFallback(result, fact, contract) {
			return diagnostic.Diagnostic{}, false
		}
		contract.name = name
		if signatureName, ok := result.CallSignatureName(site); ok && signatureName != "" {
			contract.name = signatureName
		}
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
	if lossyImplicitSelfMemberFallback(result, fact, contract) {
		return diagnostic.Diagnostic{}, false
	}
	contract.name = name
	contract.declSpan = ast.SpanOf(fact.Call)
	return p.directFunctionCall(result, point, fact, contract, defs, env)
}

func directCallDisplayName(result *body.Result, site factflow.CallSite) string {
	if result != nil {
		if signatureName, ok := result.CallSignatureName(site); ok && signatureName != "" {
			return signatureName
		}
		if callPath := site.CalleePath(); !callPath.IsEmpty() {
			return displayPath(result, callPath)
		}
		if name := result.SymbolName(site.CalleeSymbol()); name != "" {
			return name
		}
	}
	return "call target"
}

func displayPath(result *body.Result, pth path.Path) string {
	if pth.IsEmpty() {
		return ""
	}
	display := pth.Clone()
	if result != nil {
		display.Root = pth.DisplayRoot(result.SymbolName)
	}
	return display.String()
}

func currentDirectFunctionContract(
	result *body.Result,
	context producerContext,
	point cfg.Point,
	fact semantics.CallFact,
	name string,
	def *ast.FunctionExpr,
) (directFunctionContract, bool) {
	if fact.Call == nil || fact.Call.Func == nil {
		return directFunctionContract{}, false
	}
	flow := context.flow
	if flow == nil {
		flow = newDiagnosticFlowCache(result)
	}
	symbolReassigned := fact.HasCalleeSymbol &&
		fact.CalleeSymbol != 0 &&
		flow.directFunctionReassignedAfterDefinition(point, fact.CalleeSymbol)
	if def != nil && !symbolReassigned {
		contract, ok := lowerDirectFunctionContractInResultScope(result, def, context.resolver)
		if ok {
			contract.name = name
			contract.declSpan = ast.SpanOf(def)
			return contract, true
		}
	}
	if fact.HasCalleeSymbol && fact.CalleeSymbol != 0 && !symbolReassigned {
		if fn, ok := result.FunctionBySymbol(fact.CalleeSymbol); ok && fn != nil {
			contract, ok := lowerDirectFunctionContractInResultScope(result, fn, context.resolver)
			if ok {
				contract.name = name
				contract.declSpan = ast.SpanOf(fn)
				return contract, true
			}
		}
	}
	if fact.HasCalleePath && !fact.CalleePath.IsEmpty() {
		if fn, defPoint, ok := dominatingFunctionDefinitionForPathWithPoint(result, point, fact.CalleePath); ok &&
			!memberPathReassignedAfterDefinition(result, context.flow, defPoint, point, fact.CalleePath) {
			contract, ok := memberCall(context).memberFunctionDefinitionContract(result, fn)
			if ok {
				contract.name = name
				contract.declSpan = ast.SpanOf(fn)
				return contract, true
			}
		}
	}
	calleeType, ok := directCallCalleeType(result, context.resolver, point, fact.Call.Func)
	if !ok || typ.IsAny(calleeType) || typ.IsUnknown(calleeType) {
		return directFunctionContract{}, false
	}
	callable, ok := typecall.Callable(calleeType)
	if !ok || callable == nil {
		return directFunctionContract{}, false
	}
	contract := lowerDirectFunctionType(callable)
	if lossyImplicitSelfMemberFallback(result, fact, contract) {
		return directFunctionContract{}, false
	}
	contract.name = name
	contract.declSpan = ast.SpanOf(fact.Call)
	if fact.Call.Method != "" && fact.Call.Receiver != nil {
		if receiverType, ok := newStructuralFlowExpressionTyper(result, context.resolver, point, cachedGuardEnvironments(result)[point]).typeOf(fact.Call.Receiver); ok {
			contract = colonMemberCallContract(receiverType, contract)
		}
	}
	return contract, true
}

func lossyImplicitSelfMemberFallback(result *body.Result, fact semantics.CallFact, contract directFunctionContract) bool {
	if result == nil || fact.Call == nil || len(fact.Call.Args) == 0 || len(contract.params) != 0 {
		return false
	}
	if !fact.HasCalleePath || fact.CalleePath.IsEmpty() || len(fact.CalleePath.Segments) == 0 {
		return false
	}
	fn := result.Function()
	if fn == nil {
		return false
	}
	argPath, ok := result.ExpressionPath(fact.Call.Args[0])
	if !ok || len(argPath.Segments) != 0 || argPath.Symbol == 0 {
		return false
	}
	for _, slot := range result.FunctionParamSlots(fn) {
		if slot.ImplicitSelf && slot.Symbol == argPath.Symbol {
			return true
		}
	}
	return false
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
	callable := false
	if _, ok := typecall.Callable(calleeType); ok {
		callable = true
	}
	return directCalleeDiagnostic(call, CodeDirectCallNotCallable,
		possiblyNilCallTargetMessage(name),
		possiblyNilCallTargetHelp(name),
		diagnostic.Evidence{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Span:    span,
			Message: possiblyNilCalleeTypeEvidence(name, calleeType, callable),
		},
		diagnostic.Evidence{
			Kind:    diagnostic.EvidenceMissingProof,
			Trust:   diagnostic.TrustUnknown,
			Span:    span,
			Message: missingNonNilBeforeCallMessage(name),
		})
}

// directCalleeDiagnostic builds the shared callee-not-callable diagnostic shell:
// the call span/position, the caller's message, and the concrete evidence
// explaining why the target is not callable.
func directCalleeDiagnostic(call *ast.FuncCallExpr, code diagnostic.Code, message string, help string, evidence ...diagnostic.Evidence) diagnostic.Diagnostic {
	span := ast.SpanOf(call)
	return diagnostic.New(diagnostic.DiagnosticSpec{
		Span:        span,
		Code:        code,
		Severity:    diagnostic.SeverityError,
		Message:     message,
		Explanation: diagnostic.NewExplanation(evidence...),
		Help:        help,
		Labels:      []diagnostic.Label{sourceLabel(span, labelCallTarget)},
	})
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
	typ          typ.Type
	display      string
	declSpan     ast.Span
	explicit     bool
	optional     bool
	implicitSelf bool
}

type directCallResult struct {
	typ       typ.Type
	declSpan  ast.Span
	declLabel string
	explicit  bool
}

func (c directFunctionContract) paramDeclSpan(index int) ast.Span {
	if index >= 0 && index < len(c.params) {
		return c.params[index].declSpan
	}
	if c.hasVararg {
		return c.variadic.declSpan
	}
	return ast.Span{}
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
	contract, ok := lowerDirectFunctionContractInResultScope(result, fn, p.resolver)
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
	contract, violations := instantiateDirectFunctionContract(result, point, fact, contract, producerContext(p), defs)
	if len(violations) > 0 {
		violation := violations[0]
		if violation.Index >= 0 && violation.Index < len(args) {
			arg := args[violation.Index]
			if mismatch, ok := genericObjectLiteralArgTypeMismatch(result, arg, violation.Got, violation.Constraint); ok {
				extra := boundaryDiagnosticEvidenceForSubject(result, point, ast.SpanOf(mismatch.expr), exprEvidenceName(mismatch.expr), mismatch.want, boundaryValueFromExpr(mismatch.expr))
				return objectLiteralArgTypeDiagnostic(call, contract.name, violation.Index, arg, mismatch, contract.paramDeclSpan(violation.Index), extra...), true
			}
			if mismatch, ok := objectLiteralMemberMismatch(result, point, arg, violation.Constraint, env); ok {
				extra := boundaryDiagnosticEvidenceForSubject(result, point, ast.SpanOf(mismatch.expr), exprEvidenceName(mismatch.expr), mismatch.want, boundaryValueFromExpr(mismatch.expr))
				return objectLiteralArgTypeDiagnostic(call, contract.name, violation.Index, arg, mismatch, contract.paramDeclSpan(violation.Index), extra...), true
			}
			extra := genericObjectLiteralMissingFieldEvidence(result, arg, violation.Constraint)
			return argTypeDiagnostic(call, contract.name, violation.Index, violation.Got, "", violation.Constraint, "", args[violation.Index], contract.paramDeclSpan(violation.Index), extra...), true
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
		paramDisplay := ""
		if i < len(contract.params) {
			paramDisplay = contract.params[i].display
		} else if contract.hasVararg {
			paramDisplay = contract.variadic.display
		}
		if mismatch, ok := objectLiteralMemberMismatch(result, point, arg, want, env); ok {
			extra := boundaryDiagnosticEvidenceForSubject(result, point, ast.SpanOf(mismatch.expr), exprEvidenceName(mismatch.expr), mismatch.want, boundaryValueFromExpr(mismatch.expr))
			return objectLiteralArgTypeDiagnostic(call, contract.name, i, arg, mismatch, contract.paramDeclSpan(i), extra...), true
		}
		gotDisplay, _ := directCallArgumentDisplayType(result, p.resolver, point, arg)
		readBoundary := boundaryCallArgumentReader(fact, i, arg)
		got, ok := declaredArgumentExprType(result, p.resolver, arg)
		if ok && topLikeType(got) {
			ok = false
		}
		untrustedTopLike := false
		if !ok {
			got, ok = untrustedTopLikeExpressionTypeAt(result, p.resolver, point, arg)
			untrustedTopLike = ok
		}
		if !ok {
			got, ok = projectedStructuralFlowSourceType(result, p.resolver, point, guardEnv{}, arg)
		}
		if !ok {
			if contractGot, contractOK := directCallArgumentContractSourceType(result, producerContext(p), fact, i, defs); contractOK {
				got, ok = contractGot, true
				if topLikeType(got) {
					untrustedTopLike = true
					readBoundary = untrustedAnyBoundaryReader(readBoundary)
				}
			}
		}
		if !ok {
			got, ok = boundaryCallArgumentSourceType(result, point, fact, i)
			if ok && topLikeType(got) {
				untrustedTopLike = true
				readBoundary = untrustedAnyBoundaryReader(readBoundary)
			}
		}
		if !ok {
			got, ok = boundaryExprType(result, p.resolver, arg)
		}
		if !ok || refinement.ContainsFreeTypeParam(got) {
			continue
		}
		if untrustedTopLike {
			if !boundaryProofTypeMismatch(result, point, got, want, readBoundary) {
				continue
			}
		} else if !directCallArgumentTypeMismatch(result, point, got, want, readBoundary) {
			continue
		}
		extra := boundaryDiagnosticEvidenceForSubject(result, point, ast.SpanOf(arg), callArgumentSubject(i, exprEvidenceNameOK(arg)), want, readBoundary)
		if len(extra) == 0 {
			extra = explicitTopLikeCastEvidence(ast.SpanOf(arg), want, arg)
		}
		return argTypeDiagnostic(call, contract.name, i, got, gotDisplay, want, paramDisplay, arg, contract.paramDeclSpan(i), extra...), true
	}
	return diagnostic.Diagnostic{}, false
}

func declaredArgumentExprType(result *body.Result, resolver typeannotation.Resolver, expr ast.Expr) (typ.Type, bool) {
	switch expr.(type) {
	case *ast.IdentExpr, *ast.CastExpr, *ast.NonNilAssertExpr:
		return boundaryExprType(result, resolver, expr)
	default:
		return nil, false
	}
}

func directCallArgumentDisplayType(result *body.Result, resolver typeannotation.Resolver, point cfg.Point, expr ast.Expr) (string, bool) {
	if result == nil || expr == nil {
		return "", false
	}
	if _, ok := expr.(*ast.IdentExpr); !ok {
		return "", false
	}
	accessPath, ok := result.ExpressionPath(expr)
	if !ok || accessPath.Symbol == 0 || len(accessPath.Segments) != 0 {
		return "", false
	}
	graph := result.Graph()
	if graph == nil {
		return "", false
	}
	idom := dominance.ComputeImmediateDominatorInfo(graph).Map()
	visited := make(map[cfg.Point]struct{}, graph.Size())
	for cursor := point; ; {
		if _, ok := visited[cursor]; ok {
			return "", false
		}
		visited[cursor] = struct{}{}
		if fact, ok := result.OrdinaryAssignment(cursor); ok && fact.HasSymbol && fact.Symbol == accessPath.Symbol && (!fact.HasPath || len(fact.Path.Segments) == 0) {
			return "", false
		}
		if fact, ok := result.LocalAssignment(cursor); ok && fact.HasSymbol && fact.Symbol == accessPath.Symbol && fact.Type != nil {
			if rendered := display.AnnotationOrType(fact.Type, nil); rendered != "" {
				return rendered, true
			}
		}
		parent, ok := idom[cursor]
		if !ok || parent == cursor {
			return "", false
		}
		cursor = parent
	}
}

func directCallArgumentContractSourceType(
	result *body.Result,
	context producerContext,
	fact semantics.CallFact,
	index int,
	defs map[symbol.ID]*ast.FunctionExpr,
) (typ.Type, bool) {
	if result == nil || index < 0 || index >= len(fact.ArgumentSources) {
		return nil, false
	}
	source := fact.ArgumentSources[index]
	if source.Kind != sourceprovenance.SourceCall || !source.HasCallPoint || source.ResultIndex < 0 {
		return nil, false
	}
	sourceFact, ok := result.Call(source.CallPoint)
	if !ok {
		return nil, false
	}
	site, ok := result.CallSite(source.CallPoint)
	if !ok {
		return nil, false
	}
	var def *ast.FunctionExpr
	if defs != nil && site.CalleeSymbol() != 0 {
		def = defs[site.CalleeSymbol()]
	}
	contract, _, ok := directCallResultContract(result, context, source.CallPoint, sourceFact, site, def, defs)
	if !ok {
		return nil, false
	}
	if got, ok := contract.returnType(source.ResultIndex); ok && !refinement.ContainsFreeTypeParam(got) {
		return got, true
	}
	got, ok := contract.declaredReturnType(source.ResultIndex)
	if !ok || refinement.ContainsFreeTypeParam(got) {
		return nil, false
	}
	return got, true
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
	contract := directFunctionContract{
		params:  make([]directCallParam, 0),
		returns: make([]directCallResult, 0),
	}
	if fnType, ok := lowerFunctionExprType(fn, resolver); ok {
		contract.source = fnType
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
	fillFunctionReturnDeclarationSpans(&contract, fn)
	return contract, true
}

func lowerDirectFunctionContractForResult(result *body.Result, fn *ast.FunctionExpr, resolver typeannotation.Resolver) (directFunctionContract, bool) {
	if result != nil {
		if slots := result.FunctionParamSlots(fn); len(slots) != 0 {
			return lowerDirectFunctionContractFromParamSlots(fn, slots, resolver)
		}
		if owner := functionResultForExpr(result, fn); owner != nil && owner != result {
			if slots := owner.FunctionParamSlots(fn); len(slots) != 0 {
				return lowerDirectFunctionContractFromParamSlots(fn, slots, resolver)
			}
		}
	}
	return lowerDirectFunctionContract(fn, resolver)
}

func lowerDirectFunctionContractFromParamSlots(fn *ast.FunctionExpr, slots []bind.ParamSlot, resolver typeannotation.Resolver) (directFunctionContract, bool) {
	if fn == nil {
		return directFunctionContract{}, false
	}
	contract := directFunctionContract{
		params:  make([]directCallParam, 0, len(slots)),
		returns: make([]directCallResult, 0, len(fn.ReturnTypes)),
	}
	if fnType, ok := lowerFunctionExprType(fn, resolver); ok {
		contract.source = fnType
	}
	for _, slot := range slots {
		param, ok := lowerDirectCallParam(slot.Type, resolver)
		if !ok {
			return directFunctionContract{}, false
		}
		if slot.ImplicitSelf {
			param.implicitSelf = true
			if slot.Type == nil {
				param.explicit = true
				param.optional = false
			}
		}
		if slot.Vararg {
			contract.hasVararg = true
			contract.variadic = param
			continue
		}
		contract.params = append(contract.params, param)
	}
	for _, retExpr := range fn.ReturnTypes {
		ret, ok := lowerDirectCallResult(retExpr, resolver)
		if !ok {
			return directFunctionContract{}, false
		}
		contract.returns = append(contract.returns, ret)
	}
	fillFunctionReturnDeclarationSpans(&contract, fn)
	return contract, true
}

func lowerDirectFunctionContractInResultScope(result *body.Result, fn *ast.FunctionExpr, resolver typeannotation.Resolver) (directFunctionContract, bool) {
	for current := resolver; current != nil; current = parentTypeResolver(current) {
		if contract, ok := lowerDirectFunctionContractForResult(result, fn, current); ok {
			return contract, true
		}
	}
	return lowerDirectFunctionContractForResult(result, fn, nil)
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
			display:  "",
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
			display:  "",
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
		typ:       t,
		declSpan:  ast.SpanOf(expr),
		declLabel: labelDeclaredReturn,
		explicit:  !typ.IsAny(t) && !typ.IsUnknown(t),
	}, true
}

func fillFunctionReturnDeclarationSpans(contract *directFunctionContract, fn *ast.FunctionExpr) {
	if contract == nil || fn == nil || len(contract.returns) == 0 {
		return
	}
	fallback := functionDeclarationLineSpan(fn)
	if !fallback.Valid() {
		return
	}
	for i := range contract.returns {
		if contract.returns[i].declSpan.Valid() {
			continue
		}
		contract.returns[i].declSpan = fallback
		contract.returns[i].declLabel = labelCalleeDeclaration
	}
}

func functionDeclarationLineSpan(fn *ast.FunctionExpr) ast.Span {
	span := ast.SpanOf(fn)
	if !span.Valid() {
		return ast.Span{}
	}
	if span.EndLine != 0 && span.EndLine != span.StartLine {
		span.EndLine = span.StartLine
		span.EndCol = span.StartCol + len("function")
	}
	return span
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
		display:  display.AnnotationOrType(expr, t),
		declSpan: ast.SpanOf(expr),
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
	ret, ok := c.returnResult(index)
	if !ok {
		return nil, false
	}
	return ret.returnType()
}

func (c directFunctionContract) declaredReturnType(index int) (typ.Type, bool) {
	ret, ok := c.returnResult(index)
	if !ok {
		return nil, false
	}
	return ret.declaredReturnType()
}

func (c directFunctionContract) returnResult(index int) (directCallResult, bool) {
	if index < 0 || index >= len(c.returns) {
		return directCallResult{}, false
	}
	return c.returns[index], true
}

func (r directCallResult) returnType() (typ.Type, bool) {
	if !r.explicit || r.typ == nil || typ.IsAny(r.typ) || typ.IsUnknown(r.typ) {
		return nil, false
	}
	return r.typ, true
}

func (r directCallResult) declaredReturnType() (typ.Type, bool) {
	return r.typ, r.typ != nil
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
	return directCalleeDiagnostic(call, CodeDirectCallNotCallable,
		directNotCallableMessage(name, calleeType),
		directNotCallableHelp(name),
		diagnostic.Evidence{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Span:    span,
			Message: assignmentSourceTypeEvidence(name, calleeType),
		})
}

func tooFewArgsDiagnostic(point cfg.Point, call *ast.FuncCallExpr, name string, want, got int, declSpan ast.Span) diagnostic.Diagnostic {
	span := ast.SpanOf(call)
	declEvidenceSpan := directCallDeclarationEvidenceSpan(call, declSpan)
	return diagnostic.New(diagnostic.DiagnosticSpec{
		Span:     span,
		Code:     CodeDirectCallTooFewArgs,
		Severity: diagnostic.SeverityError,
		Message:  callArityMismatchMessage(name, want, got),
		Help:     callArityHelp(want, got),
		Labels:   []diagnostic.Label{sourceLabel(span, labelCallExpression)},
		Explanation: diagnostic.NewExplanation(
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnostic.TrustProven,
				Span:    span,
				Message: callArgumentCountEvidence(name, got),
			},
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceUserAssertion,
				Trust:   diagnostic.TrustClaimed,
				Span:    declEvidenceSpan,
				Message: callParameterCountEvidence(name, want),
			},
		),
	})
}

func tooManyArgsDiagnostic(point cfg.Point, call *ast.FuncCallExpr, name string, want, got int, declSpan ast.Span, extra ast.Expr) diagnostic.Diagnostic {
	span := ast.SpanOf(call)
	extraSpan := ast.SpanOf(extra)
	declEvidenceSpan := directCallDeclarationEvidenceSpan(call, declSpan)
	return diagnostic.New(diagnostic.DiagnosticSpec{
		Span:     span,
		Code:     CodeDirectCallTooManyArgs,
		Severity: diagnostic.SeverityError,
		Message:  callArityMismatchMessage(name, want, got),
		Help:     callArityHelp(want, got),
		Explanation: diagnostic.NewExplanation(
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceAbstractFact,
				Trust:   diagnostic.TrustProven,
				Span:    span,
				Message: callArgumentCountEvidence(name, got),
			},
			diagnostic.Evidence{
				Kind:    diagnostic.EvidenceUserAssertion,
				Trust:   diagnostic.TrustClaimed,
				Span:    declEvidenceSpan,
				Message: callParameterCountEvidence(name, want),
			},
		),
		Labels: []diagnostic.Label{sourceLabel(extraSpan, labelExtraArgument)},
	})
}

func argTypeDiagnostic(call *ast.FuncCallExpr, name string, index int, got typ.Type, gotDisplay string, want typ.Type, wantDisplay string, arg ast.Expr, declSpan ast.Span, extraEvidence ...diagnostic.Evidence) diagnostic.Diagnostic {
	subject := fmt.Sprintf("argument %d", index+1)
	declEvidenceSpan := directCallDeclarationEvidenceSpan(call, declSpan)
	return argTypeDiagnosticEnvelope(call, arg, index, got,
		argumentTypeMismatchMessageDisplay(subject, arg, got, gotDisplay, want, wantDisplay),
		diagnostic.Evidence{
			Kind:    diagnostic.EvidenceUserAssertion,
			Trust:   diagnostic.TrustClaimed,
			Span:    declEvidenceSpan,
			Message: callParameterTypeEvidenceDisplay(name, index+1, "", want, wantDisplay),
		},
		gotDisplay,
		extraEvidence...)
}

func objectLiteralArgTypeDiagnostic(call *ast.FuncCallExpr, name string, index int, arg ast.Expr, mismatch objectLiteralTypeMismatch, declSpan ast.Span, extraEvidence ...diagnostic.Evidence) diagnostic.Diagnostic {
	subject := fmt.Sprintf("argument %d", index+1)
	if mismatch.suffix != "" {
		subject += mismatch.suffix
	}
	frameExpr := mismatch.expr
	if frameExpr == nil {
		frameExpr = arg
	}
	declEvidenceSpan := directCallDeclarationEvidenceSpan(call, declSpan)
	return argTypeDiagnosticEnvelopeWithSubject(call, frameExpr, index, mismatch.got, "", subject,
		fmt.Sprintf("%s is %s, not %s", subject, formatType(mismatch.got), formatType(mismatch.want)),
		diagnostic.Evidence{
			Kind:    diagnostic.EvidenceUserAssertion,
			Trust:   diagnostic.TrustClaimed,
			Span:    declEvidenceSpan,
			Message: callParameterTypeEvidence(name, index+1, mismatch.suffix, mismatch.want),
		},
		extraEvidence...)
}

func genericObjectLiteralArgTypeMismatch(result *body.Result, arg ast.Expr, actual typ.Type, formal typ.Type) (objectLiteralTypeMismatch, bool) {
	if result == nil || arg == nil || actual == nil || formal == nil {
		return objectLiteralTypeMismatch{}, false
	}
	fact, ok := result.ObjectLiteral(arg)
	if !ok {
		return objectLiteralTypeMismatch{}, false
	}
	for _, entry := range fact.Entries {
		got, gotOK := expectedTypeAtSegments(actual, entry.Suffix.Segments)
		want, wantOK := expectedTypeAtSegments(formal, entry.Suffix.Segments)
		if !gotOK || !wantOK || got == nil || want == nil ||
			typ.IsAny(got) || typ.IsUnknown(got) || typ.IsAny(want) || typ.IsUnknown(want) {
			continue
		}
		if typecall.InstantiatedArgumentAssignable(got, want) {
			continue
		}
		return objectLiteralTypeMismatch{
			expr:   entry.Value,
			got:    got,
			want:   want,
			suffix: segment.FormatSegments(entry.Suffix.Segments),
		}, true
	}
	return objectLiteralTypeMismatch{}, false
}

func genericObjectLiteralMissingFieldEvidence(result *body.Result, arg ast.Expr, formal typ.Type) []diagnostic.Evidence {
	if result == nil || arg == nil || formal == nil {
		return nil
	}
	fact, ok := result.ObjectLiteral(arg)
	if !ok {
		return nil
	}
	field, ok := missingRequiredRecordField(formal, fact)
	if !ok {
		return nil
	}
	return []diagnostic.Evidence{{
		Kind:    diagnostic.EvidenceMissingProof,
		Trust:   diagnostic.TrustUnknown,
		Span:    ast.SpanOf(arg),
		Message: missingRequiredFieldEvidence(field.Name),
	}}
}

// argTypeDiagnosticEnvelope builds the shared argument-type diagnostic shell: the
// call/argument spans and labels, the "argument N is <got>" abstract fact, the
// caller's message, and a second evidence item describing what was expected.
func argTypeDiagnosticEnvelope(call *ast.FuncCallExpr, arg ast.Expr, index int, got typ.Type, message string, expected diagnostic.Evidence, gotDisplay string, extraEvidence ...diagnostic.Evidence) diagnostic.Diagnostic {
	return argTypeDiagnosticEnvelopeWithSubject(call, arg, index, got, gotDisplay, fmt.Sprintf("argument %d", index+1), message, expected, extraEvidence...)
}

func argTypeDiagnosticEnvelopeWithSubject(call *ast.FuncCallExpr, arg ast.Expr, index int, got typ.Type, gotDisplay string, subject string, message string, expected diagnostic.Evidence, extraEvidence ...diagnostic.Evidence) diagnostic.Diagnostic {
	callSpan := ast.SpanOf(call)
	argName := exprEvidenceName(arg)
	argSpan := directCallArgumentSpan(call, arg, index, argName)
	primarySpan := argSpan
	if !primarySpan.Valid() {
		primarySpan = callSpan
	}
	evidenceSubject := subject
	if argName != "" && argName != unknownSourceName {
		evidenceSubject = fmt.Sprintf("%s (%s)", subject, argName)
	}
	extraEvidence = clarifyTypeMismatchEvidence(extraEvidence, argName, got, argSpan, "parameter type")
	evidence := []diagnostic.Evidence{
		{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Span:    argSpan,
			Message: assignmentSourceTypeEvidenceDisplay(evidenceSubject, got, gotDisplay),
		},
		expected,
	}
	evidence = append(evidence, extraEvidence...)
	return diagnostic.New(diagnostic.DiagnosticSpec{
		Span:        primarySpan,
		Code:        CodeDirectCallArgType,
		Severity:    diagnostic.SeverityError,
		Message:     message,
		Explanation: diagnostic.NewExplanation(evidence...),
		Help:        argumentTypeMismatchHelpForEvidence(subject, argName, got, evidence),
		Labels:      []diagnostic.Label{sourceLabel(argSpan, labelArgumentValue)},
	})
}

func directCallArgumentSpan(call *ast.FuncCallExpr, arg ast.Expr, index int, argName string) diagnostic.Span {
	span := ast.SpanOf(arg)
	if _, ok := arg.(*ast.TableExpr); ok && call != nil && index > 0 && index <= len(call.Args)-1 && span.Valid() {
		prev := ast.SpanOf(call.Args[index-1])
		prevLine := prev.StartLine
		prevEndCol := prev.EndCol
		if prevEndCol <= 0 {
			prevEndCol = prev.StartCol
		}
		if prev.Valid() && span.StartLine != prevLine && span.StartCol > prevEndCol {
			span.StartLine = prevLine
			span.EndLine = span.StartLine
			span.EndCol = span.StartCol + 1
			return span
		}
	}
	return spanWithEvidenceName(span, argName)
}

func directCallDeclarationEvidenceSpan(call *ast.FuncCallExpr, declSpan ast.Span) diagnostic.Span {
	if !declSpan.Valid() {
		return diagnostic.Span{}
	}
	callSpan := ast.SpanOf(call)
	if callSpan.Valid() &&
		declSpan.StartLine == callSpan.StartLine &&
		declSpan.StartCol == callSpan.StartCol {
		return diagnostic.Span{}
	}
	return declSpan
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
	envs := cachedGuardEnvironments(result)
	for _, point := range graph.RPO() {
		if !guardEnvReachableAt(envs, point) {
			continue
		}
		if fact, ok := result.LocalAssignment(point); ok && fact.HasSymbol && fact.Symbol != 0 {
			if fn, ok := directFunctionExprFromExpr(fact.Expr); ok {
				if out == nil {
					out = make(map[symbol.ID]*ast.FunctionExpr)
				}
				out[fact.Symbol] = fn
			}
		}
		if fact, ok := result.OrdinaryAssignment(point); ok && fact.HasSymbol && fact.Symbol != 0 {
			if fn, ok := directFunctionExprFromExpr(fact.Value); ok {
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

func directFunctionExprFromExpr(expr ast.Expr) (*ast.FunctionExpr, bool) {
	if fn, ok := expr.(*ast.FunctionExpr); ok {
		return fn, true
	}
	inner, ok := sourceprovenance.ProofInner(expr)
	if !ok {
		return nil, false
	}
	fn, ok := inner.(*ast.FunctionExpr)
	return fn, ok
}
