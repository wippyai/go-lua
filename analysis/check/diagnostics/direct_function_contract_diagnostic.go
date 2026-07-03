package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/refinement"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

func directFunctionCallContractDiagnostic(
	context producerContext,
	result *body.Result,
	point cfg.Point,
	fact semantics.CallFact,
	site factflow.CallSite,
	contract directFunctionContract,
	defs map[symbol.ID]*ast.FunctionExpr,
	env guardEnv,
) (diagnostic.Diagnostic, bool) {
	call := fact.Call
	if call == nil {
		return diagnostic.Diagnostic{}, false
	}
	contract = bindDirectCallReceiver(result, point, fact, site, contract, context.resolver, env)
	args := call.Args
	required := contract.requiredArity()
	if len(args) < required {
		return tooFewArgsDiagnostic(point, call, contract.name, required, len(args), contract.declSpan), true
	}
	if !contract.hasVararg && len(args) > len(contract.params) {
		if lossyImplicitSelfMemberFallback(result, fact, site, contract) {
			return diagnostic.Diagnostic{}, false
		}
		return tooManyArgsDiagnostic(point, call, contract.name, len(contract.params), len(args), contract.declSpan, args[len(contract.params)]), true
	}
	return directFunctionCallArgumentTypeDiagnostic(context, result, point, fact, contract, defs, env)
}

func directFunctionCallArgumentTypeDiagnostic(
	context producerContext,
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
	contract, violations := instantiateDirectFunctionContract(result, point, fact, contract, context, defs)
	if len(violations) == 0 && len(contract.genericTrace.Contributions) != 0 {
		for i, arg := range args {
			if d, ok := genericObjectLiteralInferenceTraceConflictDiagnostic(result, call, contract.name, i, arg, contract.paramDeclSpan(i), contract.genericTrace); ok {
				return d, true
			}
		}
	}
	if len(violations) > 0 {
		violation := violations[0]
		if violation.Index >= 0 && violation.Index < len(args) {
			arg := args[violation.Index]
			if functionLiteralArgumentContextuallyChecked(result, context.resolver, arg, violation.Got, violation.Constraint) {
				goto directCallArgumentLoop
			}
			if d, ok := genericObjectLiteralInferenceTraceConflictDiagnostic(result, call, contract.name, violation.Index, arg, contract.paramDeclSpan(violation.Index), contract.genericTrace); ok {
				return d, true
			}
			readBoundary := boundaryCallArgumentReader(fact, violation.Index, arg)
			if value, ok := readBoundary(result, point); ok &&
				newDiagnosticQuery(result).ValueProofAdmissible(value, violation.Constraint) {
				goto directCallArgumentLoop
			}
			if boundary, ok := boundaryCallArgumentReaderType(result, context.resolver, point, readBoundary, arg); ok &&
				!directCallArgumentTypeMismatch(result, point, boundary, violation.Constraint, readBoundary) {
				goto directCallArgumentLoop
			}
			if flow, ok := directCallArgumentFlowExpressionType(result, context.resolver, point, env, arg); ok &&
				!directCallArgumentTypeMismatch(result, point, flow, violation.Constraint, readBoundary) {
				goto directCallArgumentLoop
			}
			if containsTypeParamSyntax(violation.Got) {
				goto directCallArgumentLoop
			}
			if mismatch, ok := genericObjectLiteralArgTypeMismatch(result, arg, violation.Got, violation.Constraint); ok {
				if d, ok := genericObjectLiteralInferenceConflictDiagnostic(result, call, contract.name, violation.Index, arg, mismatch, contract.paramDeclSpan(violation.Index), contract.genericTrace); ok {
					return d, true
				}
				extra := genericInferenceEvidenceForObjectLiteralMismatch(result, contract.name, violation.Index, arg, mismatch, contract.genericTrace)
				extra = append(extra, boundaryDiagnosticEvidenceForSubject(result, point, ast.SpanOf(mismatch.expr), exprEvidenceName(mismatch.expr), mismatch.want, boundaryValueFromExpr(mismatch.expr))...)
				return objectLiteralArgTypeDiagnostic(call, contract.name, violation.Index, arg, mismatch, contract.paramDeclSpan(violation.Index), extra...), true
			}
			if mismatch, ok := objectLiteralMemberMismatchForCallArgument(result, context.resolver, point, fact, violation.Index, arg, violation.Constraint, env); ok {
				extra := boundaryDiagnosticEvidenceForSubject(result, point, ast.SpanOf(mismatch.expr), exprEvidenceName(mismatch.expr), mismatch.want, boundaryValueFromExpr(mismatch.expr))
				return objectLiteralArgTypeDiagnostic(call, contract.name, violation.Index, arg, mismatch, contract.paramDeclSpan(violation.Index), extra...), true
			}
			extra := genericObjectLiteralMissingFieldEvidence(result, arg, violation.Constraint)
			return argTypeDiagnostic(call, contract.name, violation.Index, violation.Got, "", violation.Constraint, "", args[violation.Index], contract.paramDeclSpan(violation.Index), extra...), true
		}
	}
directCallArgumentLoop:
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
		if contextIndependentImplicitSelfArgument(result, arg) {
			continue
		}
		paramDisplay := ""
		if i < len(contract.params) {
			paramDisplay = contract.params[i].display
		} else if contract.hasVararg {
			paramDisplay = contract.variadic.display
		}
		if mismatch, ok := objectLiteralMemberMismatchForCallArgument(result, context.resolver, point, fact, i, arg, want, env); ok {
			extra := boundaryDiagnosticEvidenceForSubject(result, point, ast.SpanOf(mismatch.expr), exprEvidenceName(mismatch.expr), mismatch.want, boundaryValueFromExpr(mismatch.expr))
			return objectLiteralArgTypeDiagnostic(call, contract.name, i, arg, mismatch, contract.paramDeclSpan(i), extra...), true
		}
		gotDisplay, _ := directCallArgumentDisplayType(result, context.flow, point, arg)
		sourceResolution := resolveDirectCallArgumentSourceType(result, context.resolver, point, env, fact, i, arg, context, defs)
		if !sourceResolution.OK || containsTypeParamSyntax(sourceResolution.Type) {
			continue
		}
		got := sourceResolution.Type
		readBoundary := sourceResolution.ReadBoundary
		if gotDisplay != "" {
			if declared, ok := declaredArgumentExprType(result, context.resolver, arg); !ok || !displayAliasDescribesFlowType(result, got, declared) {
				gotDisplay = ""
			}
		}
		if !sourceResolution.TypeMismatch(result, point, want) {
			continue
		}
		if functionLiteralArgumentContextuallyChecked(result, context.resolver, arg, got, want) {
			continue
		}
		if contextualParameterFlowObligationOwnedByCallSite(result, context, sourceResolution, arg) {
			continue
		}
		// Report the argument's narrowed flow type, not its declared type, so the
		// message reflects the value at the call site. The boundary value carries
		// the point-state narrowing (e.g. on the else edge of type(v) == "number"
		// v is string, not number | string), which the declared type does not.
		if readBoundary != nil {
			narrowReadBoundary := readBoundary
			if argReadBoundary := boundaryCallArgumentReader(fact, i, arg); argReadBoundary != nil {
				narrowReadBoundary = argReadBoundary
			}
			if value, vok := narrowReadBoundary(result, point); vok {
				if narrowed, nok := newDiagnosticQuery(result).RuntimeKindReducedType(value, got); nok && subtype.IsSubtype(narrowed, got) {
					got = narrowed
					gotDisplay = ""
					readBoundary = narrowReadBoundary
					if !directCallArgumentTypeMismatch(result, point, got, want, nil) {
						continue
					}
				}
			}
		}
		hasUntrustedBoundary := boundaryValueHasUntrustedTopOrigin(result, point, readBoundary)
		evidenceSubject := callArgumentBoundaryEvidenceSubject(result, arg, i, hasUntrustedBoundary)
		extra := boundaryDiagnosticEvidenceForSubject(result, point, ast.SpanOf(arg), evidenceSubject, want, readBoundary)
		if len(extra) == 0 {
			extra = explicitTopLikeCastEvidence(ast.SpanOf(arg), want, arg)
		}
		if sourceResolution.UntrustedTopLike && hasUntrustedBoundary {
			if callArgumentCascadesFromInvalidLocalDeclaration(result, context, point, arg, want, defs) {
				continue
			}
			return argProofBoundaryDiagnostic(call, contract.name, i, got, gotDisplay, want, paramDisplay, arg, contract.paramDeclSpan(i), extra...), true
		}
		if callArgumentCascadesFromInvalidLocalDeclaration(result, context, point, arg, want, defs) {
			continue
		}
		return argTypeDiagnostic(call, contract.name, i, got, gotDisplay, want, paramDisplay, arg, contract.paramDeclSpan(i), extra...), true
	}
	return diagnostic.Diagnostic{}, false
}
