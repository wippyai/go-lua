package readmodel

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	readapi "github.com/wippyai/go-lua/analysis/check/readmodel"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	luatypeprojection "github.com/wippyai/go-lua/analysis/lua/typeprojection"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// callArgumentMismatchSubjectPlan projects object-literal facts into pure
// readmodel candidates. Public readmodel owns selecting the report subject.
func (r Reader) callArgumentMismatchSubjectPlan(point cfg.Point, arg CallArgument, want typ.Type) (readapi.CallArgumentMismatchSubjectPlan, bool) {
	if r.result == nil || want == nil {
		return readapi.CallArgumentMismatchSubjectPlan{}, false
	}
	site, ok := r.result.CallSiteView(point)
	if !ok || arg.Index < 0 {
		return readapi.CallArgumentMismatchSubjectPlan{}, false
	}
	source, ok := site.ArgumentSourceAt(arg.Index)
	if !ok || !source.HasExpr {
		return readapi.CallArgumentMismatchSubjectPlan{}, false
	}
	lit, ok := r.result.ObjectLiteralView(source.ExprRef)
	if !ok {
		return readapi.CallArgumentMismatchSubjectPlan{}, false
	}
	plan := readapi.CallArgumentMismatchSubjectPlan{
		Argument: arg,
		Expected: want,
	}
	if readapi.CallArgumentExpectedTypeHasObjectEntries(want) {
		lit.ForEachEntry(func(entry factflow.ObjectEntryView) bool {
			suffix := entry.Suffix()
			expected, ok := luatypeprojection.ExpectedTypeAtSegments(want, suffix.Segments)
			if !ok || expected == nil {
				return true
			}
			value, ok := r.objectEntryValue(point, entry)
			if !ok {
				return true
			}
			got, _ := r.ValueTypeWithPresence(value)
			if arg.FunctionType != nil {
				got = arg.FunctionType
			}
			plan.Candidates = append(plan.Candidates, readapi.CallArgumentMismatchCandidate{
				Argument: CallArgument{
					Index:                arg.Index,
					Value:                value,
					ValueHash:            r.ValueHash(value),
					TypeWithPresence:     got,
					UntrustedTopOrigin:   r.ValueHasUntrustedTopOrigin(value),
					ExpandedSource:       entry.Source().Expanded,
					CallerOwnedParameter: arg.CallerOwnedParameter,
					FunctionType:         arg.FunctionType,
					Span:                 sourceSpanFromFactflow(entry.ValueSpan()),
					Label:                readapi.CallArgumentMemberLabel(arg.Index, suffix.Segments, entry.ValueLabel()),
				},
				Expected:    expected,
				LabelSuffix: readapi.CallArgumentExpectedLabelSuffix(suffix.Segments),
				Admissible:  r.ValueProofAdmissible(value, expected),
			})
			return true
		})
	}
	if field, ok := body.ObjectLiteralMissingRequired(lit, want); ok {
		plan.MissingRequiredField = field
	}
	if mismatch, ok := r.result.RecordInterfaceMismatch(arg.TypeWithPresence, want); ok {
		switch mismatch.Kind {
		case body.InterfaceMismatchMissingMethod:
			plan.MissingRequiredMethod = mismatch.MethodName
			plan.MissingRequiredMethodType = mismatch.Expected
		case body.InterfaceMismatchMethodType:
			plan.MethodMismatchName = mismatch.MethodName
			plan.MethodMismatchExpected = mismatch.Expected
			plan.MethodMismatchActual = mismatch.Actual
		}
	}
	return plan, true
}

// checkCallArgument returns the complete solved proof result for one argument
// against one expected type. It is a concrete readmodel helper; public
// obligation producers receive the resulting check on CallArgumentReport.
func (r Reader) checkCallArgument(point cfg.Point, arg CallArgument, want typ.Type, expectedLabel string, expectedSpan SourceSpan) CallArgumentCheck {
	if candidate, ok := r.admissibleCallArgumentProofCandidate(arg, want); ok {
		arg = candidate
	}
	var subjectPlan *readapi.CallArgumentMismatchSubjectPlan
	if plan, ok := r.callArgumentMismatchSubjectPlan(point, arg, want); ok {
		subjectPlan = &plan
	}
	plan := readapi.CallArgumentCheckPlan{
		Argument:                    arg,
		Expected:                    want,
		ExpectedLabel:               expectedLabel,
		ExpectedSpan:                expectedSpan,
		ValueAdmissible:             r.ValueProofAdmissible(arg.Value, want),
		ValueProvenMismatch:         r.ValueWitnessProvenMismatch(arg.Value, want),
		FunctionTypeAdmissible:      r.callArgumentFunctionTypeAdmissible(arg.FunctionType, want),
		TrustedActualProvenMismatch: r.callArgumentSolvedTypeProvenMismatch(arg.TypeWithPresence, want, arg.UntrustedTopOrigin),
		FunctionTypeProvenMismatch:  r.callArgumentFunctionTypeProvenMismatch(arg.FunctionType, want),
		SubjectPlan:                 subjectPlan,
	}
	return readapi.PlanCallArgumentCheck(plan)
}

func (r Reader) callArgumentFunctionTypeAdmissible(fn *typ.Function, expected typ.Type) bool {
	return r.result != nil && r.result.CallArgumentFunctionTypeAdmissible(fn, expected)
}

func (r Reader) callArgumentSolvedTypeProvenMismatch(actual, expected typ.Type, untrustedTopOrigin bool) bool {
	return r.result != nil && r.result.CallArgumentSolvedTypeProvenMismatch(actual, expected, untrustedTopOrigin)
}

func (r Reader) callArgumentFunctionTypeProvenMismatch(fn *typ.Function, expected typ.Type) bool {
	return r.result != nil && r.result.CallArgumentFunctionTypeProvenMismatch(fn, expected)
}

func (r Reader) callArgumentReports(point cfg.Point, contract callContract, hasContract bool, args []CallArgument, params []callParamObligation) []CallArgumentReport {
	plan := readapi.CallArgumentReportPlan{
		Args:             args,
		GenericConflicts: contract.GenericInferenceConflicts,
		Check: func(arg CallArgument, obligation CallArgumentObligation) CallArgumentCheck {
			check := r.checkCallArgument(point, arg, obligation.Type, obligation.ExpectedLabel, obligation.ExpectedSpan)
			check.ExpectedOrigin = obligation.Origin
			return check
		},
	}
	if hasContract {
		for _, violation := range contract.GenericConstraintViolations {
			if violation.Constraint == nil {
				continue
			}
			plan.GenericConstraints = append(plan.GenericConstraints, readapi.IndexedCallArgumentObligation{
				Index: violation.Index,
				Obligation: CallArgumentObligation{
					Type:          violation.Constraint,
					ExpectedLabel: contract.Source.ParameterLabel(violation.Index),
					ExpectedSpan:  contract.Source.ParameterSpan(violation.Index),
				},
			})
		}
		for _, arg := range args {
			param, ok := contract.Contract.ParamAt(arg.Index)
			if !ok || !param.Explicit || param.Type == nil {
				continue
			}
			plan.ExplicitParams = append(plan.ExplicitParams, readapi.IndexedCallArgumentObligation{
				Index: arg.Index,
				Obligation: CallArgumentObligation{
					Type:          param.AcceptedType(),
					ExpectedLabel: contract.Source.ParameterLabel(arg.Index),
					ExpectedSpan:  contract.Source.ParameterSpan(arg.Index),
				},
			})
		}
	}
	for _, obligation := range params {
		if obligation.Type == nil {
			continue
		}
		origin := obligation.Origin
		if !origin.HasOrigin {
			if r.result != nil && r.result.HasBodyOwnedParamObligations() &&
				!callArgumentIndexHasUntrustedTop(args, obligation.Index) {
				continue
			}
			site, _ := r.result.CallSiteView(point)
			origin = readapi.CallArgumentObligationOrigin{
				HasOrigin:    true,
				FunctionName: r.callContractSourceName(site),
				SubjectLabel: callArgumentLabel(obligation.Index),
			}
		}
		plan.OutcomeParams = append(plan.OutcomeParams, readapi.IndexedCallArgumentObligation{
			Index: obligation.Index,
			Obligation: CallArgumentObligation{
				Type:             obligation.Type,
				ExpectedLabel:    CallContractSource{}.ParameterLabel(obligation.Index),
				ExpectedSpan:     callArgumentSpanByIndex(args, obligation.Index),
				Origin:           origin,
				SignatureSurface: obligation.SignatureSurface,
			},
		})
	}
	return readapi.PlanCallArgumentReports(plan)
}

func callArgumentIndexHasUntrustedTop(args []CallArgument, index int) bool {
	for _, arg := range args {
		if arg.Index == index {
			return arg.UntrustedTopOrigin
		}
	}
	return false
}

func callArgumentSpanByIndex(args []CallArgument, index int) SourceSpan {
	for _, arg := range args {
		if arg.Index == index {
			return arg.Span
		}
	}
	return SourceSpan{}
}

func (r Reader) callArityReport(site factflow.CallSiteView, contract callContract, hasContract bool) CallArityReport {
	if !hasContract {
		return CallArityReport{}
	}
	actual := site.ArgumentSourceCount()
	required := contract.Contract.RequiredArity()
	fixed := contract.Contract.ParamCount()
	return readapi.PlanCallArityReport(readapi.CallArityReportPlan{
		HasContract:    true,
		CallableName:   contract.Source.Name,
		ActualCount:    actual,
		RequiredCount:  required,
		FixedCount:     fixed,
		HasVararg:      contract.Contract.HasVararg(),
		CallSpan:       sourceSpanFromFactflow(site.CallSpan()),
		ParameterSpans: contract.Source.ParameterSpans,
		ArgumentSpans:  callArgumentSpans(site),
	})
}

// forEachCallArgument visits solved argument values for the call at point.
// It preserves the factflow argument order and reads the argument value from the
// solved pre-call state when available, falling back to the call boundary for
// specialized bodies that only materialize the argument there.
func (r Reader) forEachCallArgument(point cfg.Point, visit func(CallArgument) bool) bool {
	if r.result == nil || visit == nil {
		return false
	}
	site, ok := r.result.CallSiteView(point)
	if !ok {
		return false
	}
	visited := false
	site.ForEachArgumentSource(func(index int, source factflow.ValueSource) bool {
		value, ok := r.callArgumentValue(point, source)
		if !ok {
			value, ok = r.unknownArgumentValue()
		}
		if !ok {
			return true
		}
		visited = true
		return visit(r.callArgument(point, site, index, source, value))
	})
	return visited
}

func (r Reader) callArgument(point cfg.Point, site factflow.CallSiteView, index int, source factflow.ValueSource, value product.Value) CallArgument {
	got, _ := r.ValueTypeWithPresence(value)
	if reduced, ok := r.callArgumentRuntimeKindReducedType(point, source, value); ok {
		got = reduced
		if !r.ValueHasUntrustedTopOrigin(value) {
			if reducedValue, valueOK := r.valueFromType(reduced); valueOK {
				value = reducedValue
			}
		}
	}
	arg := CallArgument{
		Index:                index,
		Value:                value,
		ValueHash:            r.ValueHash(value),
		TypeWithPresence:     got,
		UntrustedTopOrigin:   r.ValueHasUntrustedTopOrigin(value),
		ExplicitTopOrigin:    r.ValueHasExplicitTopOrigin(value),
		RuntimeValidated:     r.callArgumentRuntimeValidated(source, value),
		ExpandedSource:       source.Expanded,
		CallerOwnedParameter: r.callerOwnedParameterArgument(point, source),
		Span:                 callArgumentSpan(site, index),
		Label:                r.callArgumentLabel(site, index, source),
		Nilability:           nilabilityProvenanceForCallArgument(r, point, index, value),
	}
	if candidate, ok := r.callArgumentBoundaryCandidate(point, source, value); ok {
		arg.ProofCandidateValue = candidate
		arg.ProofCandidateHash = r.ValueHash(candidate)
		arg.ProofCandidateType, _ = r.ValueTypeWithPresence(candidate)
		arg.ProofCandidateTop = r.ValueHasUntrustedTopOrigin(candidate)
		arg.ProofCandidateExplicitTop = r.ValueHasExplicitTopOrigin(candidate)
		arg.ProofCandidateRuntime = r.callArgumentRuntimeValidated(source, candidate)
		arg.HasProofCandidate = true
	}
	if fn, ok := r.contextualFunctionArgumentType(point, source); ok {
		arg.FunctionType = fn
		arg.TypeWithPresence = fn
	} else if fn, ok := r.functionArgumentPathType(point, source, value); ok {
		arg.FunctionType = fn
		arg.TypeWithPresence = fn
	} else if fn, ok := r.functionArgumentPathStaticType(point, source); ok {
		arg.FunctionType = fn
		arg.TypeWithPresence = fn
	} else if fn, ok := r.result.FunctionValueTypeForValueAtBoundary(point, value); ok {
		arg.FunctionType = fn
		arg.TypeWithPresence = fn
	}
	return arg
}

func (r Reader) callArgumentRuntimeValidated(source factflow.ValueSource, value product.Value) bool {
	if r.result != nil && r.result.SourceHasRuntimeValidation(source) {
		return true
	}
	return r.ValueHasRuntimeValidationProof(value)
}

func (r Reader) valueFromType(t typ.Type) (product.Value, bool) {
	if r.result == nil || r.typeValues == nil || t == nil {
		return product.Value{}, false
	}
	reg := r.result.Registry()
	if reg == nil {
		return product.Value{}, false
	}
	return typevalue.WithWitness(reg, r.typeValues.FromType(reg, t), t), true
}

func (r Reader) callArgumentRuntimeKindReducedType(point cfg.Point, source factflow.ValueSource, value product.Value) (typ.Type, bool) {
	if r.result == nil {
		return nil, false
	}
	if r.result.SourceHasRuntimeValidation(source) {
		return nil, false
	}
	p, ok := r.callArgumentExpressionPath(source)
	if !ok || !r.pathHasPositiveRuntimeTypeGuard(point, p) {
		return nil, false
	}
	declared, ok := r.explicitTopDeclaredPathTypeAt(p)
	if !ok || declared == nil {
		return nil, false
	}
	return r.RuntimeKindReducedType(value, declared)
}

func (r Reader) functionArgumentPathStaticType(point cfg.Point, source factflow.ValueSource) (*typ.Function, bool) {
	p, ok := r.valueSourcePath(source)
	if !ok || p.IsEmpty() {
		return nil, false
	}
	if fn, ok := r.result.PathSignatureTypeAt(point, p); ok && fn != nil {
		return fn, true
	}
	if t, ok := r.result.DeclaredPathTypeAt(point, p, true); ok && t != nil {
		fn, ok := t.(*typ.Function)
		if ok && fn != nil {
			return fn, true
		}
	}
	return nil, false
}

func (r Reader) admissibleCallArgumentProofCandidate(arg CallArgument, want typ.Type) (CallArgument, bool) {
	if !arg.HasProofCandidate || want == nil || !r.ValueProofAdmissible(arg.ProofCandidateValue, want) {
		return CallArgument{}, false
	}
	arg.Value = arg.ProofCandidateValue
	arg.ValueHash = arg.ProofCandidateHash
	arg.TypeWithPresence = arg.ProofCandidateType
	arg.UntrustedTopOrigin = arg.ProofCandidateTop
	arg.ExplicitTopOrigin = arg.ProofCandidateExplicitTop
	arg.RuntimeValidated = arg.ProofCandidateRuntime
	arg.Mismatch = CallArgumentMismatch{}
	return arg, true
}

func (r Reader) callerOwnedParameterArgument(point cfg.Point, source factflow.ValueSource) bool {
	return r.result != nil && r.result.CallerOwnedParameterSource(point, source)
}

func (r Reader) contextualFunctionArgumentType(point cfg.Point, source factflow.ValueSource) (*typ.Function, bool) {
	if r.result == nil || !source.HasExpr || source.ExprRef == 0 {
		return nil, false
	}
	if _, ok := r.result.ExpressionFunction(source.ExprRef); !ok {
		return nil, false
	}
	t, ok := r.result.SignatureArgumentTypeAtBoundary(point, source)
	if !ok {
		return nil, false
	}
	fn, ok := t.(*typ.Function)
	return fn, ok && fn != nil
}

func (r Reader) functionArgumentPathType(point cfg.Point, source factflow.ValueSource, value product.Value) (*typ.Function, bool) {
	p, ok := r.valueSourcePath(source)
	if !ok || p.IsEmpty() {
		return nil, false
	}
	return r.result.FunctionValueTypeForPathValueAtBoundary(point, p, value)
}

func (r Reader) callArgumentSpan(point cfg.Point, index int) SourceSpan {
	if r.result == nil {
		return SourceSpan{}
	}
	site, ok := r.result.CallSiteView(point)
	if !ok {
		return SourceSpan{}
	}
	return callArgumentSpan(site, index)
}

func (r Reader) unknownArgumentValue() (product.Value, bool) {
	if r.result == nil || r.result.Registry() == nil || r.typeValues == nil {
		return product.Value{}, false
	}
	return r.typeValues.FromType(r.result.Registry(), typ.Unknown), true
}

func (r Reader) callArgumentValue(point cfg.Point, source factflow.ValueSource) (product.Value, bool) {
	value, ok := r.callArgumentSelectedValue(point, source)
	if !ok {
		return product.Value{}, false
	}
	if p, pathOK := r.callArgumentSourcePath(source); pathOK {
		value = r.callArgumentDeclaredTopValue(point, source, p, value)
	}
	return value, true
}

func (r Reader) callArgumentSelectedValue(point cfg.Point, source factflow.ValueSource) (product.Value, bool) {
	if r.result == nil {
		return product.Value{}, false
	}
	if r.sourceHasRuntimeValidationAuthority(point, source) {
		if value, ok := r.result.SourceValueAtBoundary(point, source); ok {
			if before, beforeOK := r.result.SourceValueBeforeBoundary(point, source); beforeOK &&
				!r.callArgumentRuntimeValidationCanAdoptBoundary(before, value) {
				return before, true
			}
			return value, true
		}
	}
	if p, ok := r.callArgumentExpressionPath(source); ok && r.rootPathArgumentUsesBoundary(point, p) {
		if value, valueOK := r.callArgumentSharperBoundaryValue(point, source, p); valueOK {
			return r.callArgumentDeclaredTopValue(point, source, p, value), true
		}
	}
	if p, ok := r.valueSourcePath(source); ok && r.rootPathArgumentUsesBoundary(point, p) {
		if value, valueOK := r.callArgumentSharperBoundaryValue(point, source, p); valueOK {
			return r.callArgumentDeclaredTopValue(point, source, p, value), true
		}
	}
	if value, ok := r.callArgumentSharperSourceBoundaryValue(point, source); ok {
		return value, true
	}
	if value, ok := r.callArgumentNonNilAssertionSourceValue(point, source); ok {
		return value, true
	}
	if p, ok := r.callArgumentExpressionPath(source); ok {
		return r.callArgumentPathValue(point, source, p)
	}
	if p, ok := r.valueSourcePath(source); ok && !p.IsEmpty() {
		return r.callArgumentPathValue(point, source, p)
	}
	if value, ok := r.trustedReadableSourceValueBeforeBoundary(point, source); ok {
		return value, true
	}
	if source.Kind == factflow.ValueSourceCall {
		return r.result.SourceValueAtBoundary(point, source)
	}
	if p, ok := r.callArgumentExpressionPath(source); ok {
		if value, valueOK := r.declaredTopArgumentValue(point, source, p); valueOK {
			return value, true
		}
	}
	if p, ok := r.valueSourcePath(source); ok && !p.IsEmpty() {
		if value, valueOK := r.declaredTopArgumentValue(point, source, p); valueOK {
			return value, true
		}
	}
	if value, ok := r.result.SourceValueBeforeBoundary(point, source); ok {
		return value, true
	}
	return r.result.SourceValueAtBoundary(point, source)
}

func (r Reader) callArgumentSourcePath(source factflow.ValueSource) (path.Path, bool) {
	if p, ok := r.callArgumentExpressionPath(source); ok {
		return p, true
	}
	return r.valueSourcePath(source)
}

func (r Reader) callArgumentExpressionPath(source factflow.ValueSource) (path.Path, bool) {
	if r.result == nil || !source.HasExpr || source.ExprRef == 0 {
		return path.Path{}, false
	}
	p, ok := r.result.ExpressionPathRef(source.ExprRef)
	return p, ok && !p.IsEmpty()
}

func (r Reader) callArgumentPathValue(point cfg.Point, source factflow.ValueSource, p path.Path) (product.Value, bool) {
	if len(p.Segments) > 0 {
		if value, ok := r.result.PathValueAtBoundary(point, p); ok {
			return r.callArgumentDeclaredTopValue(point, source, p, value), true
		}
	}
	if r.rootPathArgumentUsesBoundary(point, p) {
		if value, ok := r.callArgumentSharperBoundaryValue(point, source, p); ok {
			return r.callArgumentDeclaredTopValue(point, source, p, value), true
		}
		if value, ok := r.result.PathValueBeforeBoundary(point, p); ok {
			if r.valueHasReadableType(value) {
				return r.callArgumentDeclaredTopValue(point, source, p, value), true
			}
			if declared, ok := r.declaredRootArgumentValue(point, p); ok {
				return declared, true
			}
			return r.callArgumentDeclaredTopValue(point, source, p, value), true
		}
		if value, ok := r.result.PathValueAtBoundary(point, p); ok {
			return r.callArgumentDeclaredTopValue(point, source, p, value), true
		}
		if value, ok := r.result.SourceValueAtBoundary(point, source); ok {
			return r.callArgumentDeclaredTopValue(point, source, p, value), true
		}
	}
	if source.Adjusted || source.Expanded {
		if r.rootPathArgumentUsesBoundary(point, p) {
			if value, ok := r.trustedReadableSourceValueAtBoundary(point, source); ok {
				if before, beforeOK := r.result.SourceValueBeforeBoundary(point, source); beforeOK &&
					!r.callArgumentBoundaryCanRefine(before, value) {
					return product.Value{}, false
				}
				return value, true
			}
		}
		if value, ok := r.result.SourceValueAtBoundary(point, source); ok && r.ValueHasExplicitTopOrigin(value) {
			return value, true
		}
	}
	if source.Kind == factflow.ValueSourcePath {
		if value, ok := r.result.SourceValueBeforeBoundary(point, source); ok {
			if !r.valueHasReadableType(value) {
				if boundary, boundaryOK := r.trustedReadableSourceValueAtBoundary(point, source); boundaryOK {
					return r.callArgumentDeclaredTopValue(point, source, p, boundary), true
				}
			}
			return r.callArgumentDeclaredTopValue(point, source, p, value), true
		}
		if value, ok := r.result.PathValueAtBoundary(point, p); ok {
			return r.callArgumentDeclaredTopValue(point, source, p, value), true
		}
	}
	return product.Value{}, false
}

func (r Reader) callArgumentSharperSourceBoundaryValue(point cfg.Point, source factflow.ValueSource) (product.Value, bool) {
	if r.result == nil {
		return product.Value{}, false
	}
	boundary, ok := r.result.SourceValueAtBoundary(point, source)
	if !ok {
		return product.Value{}, false
	}
	before, beforeOK := r.result.SourceValueBeforeBoundary(point, source)
	if !beforeOK {
		if r.sourceHasDeclaredTopRoot(point, source) {
			return product.Value{}, false
		}
		if source.Adjusted || source.Expanded || r.sourceHasRuntimeValidationAuthority(point, source) {
			return boundary, true
		}
		return product.Value{}, false
	}
	if presence.Equal(product.PresenceOf(boundary), presence.Present()) &&
		!presence.Equal(product.PresenceOf(before), presence.Present()) {
		if r.callArgumentPresenceOnlyRefinement(before, boundary) &&
			r.callArgumentSourceBoundaryHasRuntimeOrTypeProof(point, source) {
			return boundary, true
		}
	}
	beforeType, beforeTypeOK := r.ValueTypeWithPresence(before)
	boundaryType, boundaryTypeOK := r.ValueTypeWithPresence(boundary)
	if beforeTypeOK && boundaryTypeOK && body.TypeMayBeNilMismatch(beforeType, boundaryType) {
		if r.sourceHasRuntimeValidationAuthority(point, source) ||
			(r.callArgumentNilabilityOnlyRefinement(beforeType, boundaryType) &&
				r.callArgumentSourceBoundaryHasRuntimeOrTypeProof(point, source)) {
			return boundary, true
		}
	}
	return product.Value{}, false
}

func (r Reader) callArgumentNonNilAssertionSourceValue(point cfg.Point, source factflow.ValueSource) (product.Value, bool) {
	if r.result == nil || !r.result.SourceHasNonNilAssertion(source) {
		return product.Value{}, false
	}
	return r.result.SourceValueAtBoundary(point, source)
}

func (r Reader) callArgumentSourceBoundaryHasRuntimeOrTypeProof(point cfg.Point, source factflow.ValueSource) bool {
	if r.sourceHasRuntimeValidationAuthority(point, source) {
		return true
	}
	p, ok := r.callArgumentSourcePath(source)
	return ok && r.callArgumentBoundaryHasRuntimeOrTypeProof(point, source, p)
}

func (r Reader) callArgumentNilabilityOnlyRefinement(beforeType, boundaryType typ.Type) bool {
	return r.result != nil && r.result.CallArgumentNilabilityOnlyRefinement(beforeType, boundaryType)
}

func (r Reader) callArgumentPresenceOnlyRefinement(before, boundary product.Value) bool {
	beforeType, beforeOK := r.ValueTypeWithPresence(before)
	boundaryType, boundaryOK := r.ValueTypeWithPresence(boundary)
	if !beforeOK || !boundaryOK {
		return true
	}
	return r.callArgumentNilabilityOnlyRefinement(beforeType, boundaryType)
}

func (r Reader) callArgumentRuntimeValidationCanAdoptBoundary(before, boundary product.Value) bool {
	return r.result == nil || r.result.CallArgumentRuntimeValidationCanAdoptBoundary(before, boundary)
}

func (r Reader) sourceHasDeclaredTopRoot(point cfg.Point, source factflow.ValueSource) bool {
	var p path.Path
	var ok bool
	if p, ok = r.callArgumentExpressionPath(source); !ok {
		p, ok = r.valueSourcePath(source)
	}
	if !ok || p.IsEmpty() || len(p.Segments) != 0 {
		return false
	}
	declared, declaredOK := r.explicitTopDeclaredPathTypeAt(p)
	base := unwrap.Optional(declared)
	return declaredOK && base != nil && (typ.IsAny(base) || typ.IsUnknown(base))
}

func (r Reader) sourceHasRuntimeValidationAuthority(_ cfg.Point, source factflow.ValueSource) bool {
	return r.result != nil && r.result.SourceHasRuntimeValidation(source)
}

func (r Reader) trustedReadableSourceValueAtBoundary(point cfg.Point, source factflow.ValueSource) (product.Value, bool) {
	if r.result == nil {
		return product.Value{}, false
	}
	value, ok := r.result.SourceValueAtBoundary(point, source)
	if !ok || !r.valueHasReadableType(value) || r.ValueHasUntrustedTopOrigin(value) {
		return product.Value{}, false
	}
	return value, true
}

func (r Reader) trustedReadableSourceValueBeforeBoundary(point cfg.Point, source factflow.ValueSource) (product.Value, bool) {
	if r.result == nil {
		return product.Value{}, false
	}
	value, ok := r.result.SourceValueBeforeBoundary(point, source)
	if !ok || !r.valueHasReadableType(value) || r.ValueHasUntrustedTopOrigin(value) {
		return product.Value{}, false
	}
	return value, true
}

func (r Reader) declaredRootArgumentValue(point cfg.Point, p path.Path) (product.Value, bool) {
	if r.result == nil || r.typeValues == nil || p.IsEmpty() || len(p.Segments) != 0 {
		return product.Value{}, false
	}
	declared, ok := r.result.DeclaredPathTypeAt(point, p, true)
	if !ok || declared == nil || typ.IsAny(declared) || typ.IsUnknown(declared) {
		return product.Value{}, false
	}
	reg := r.result.Registry()
	if reg == nil {
		return product.Value{}, false
	}
	return typevalue.WithWitness(reg, r.typeValues.FromType(reg, declared), declared), true
}

func (r Reader) rootPathArgumentUsesBoundary(point cfg.Point, p path.Path) bool {
	return r.result != nil && r.result.RootPathArgumentUsesBoundary(point, p)
}

func (r Reader) callArgumentSharperBoundaryValue(point cfg.Point, source factflow.ValueSource, p path.Path) (product.Value, bool) {
	if r.result == nil || p.IsEmpty() || len(p.Segments) != 0 {
		return product.Value{}, false
	}
	boundary, ok := r.readablePathValueAtBoundary(point, p)
	if !ok {
		return product.Value{}, false
	}
	before, beforeOK := r.result.PathValueBeforeBoundary(point, p)
	if !beforeOK {
		if sourceBefore, sourceBeforeOK := r.result.SourceValueBeforeBoundary(point, source); sourceBeforeOK &&
			!r.callArgumentBoundaryRefinementAccepted(point, source, p, sourceBefore, boundary) {
			return product.Value{}, false
		}
		return boundary, true
	}
	if sourceBefore, sourceBeforeOK := r.result.SourceValueBeforeBoundary(point, source); sourceBeforeOK &&
		!r.callArgumentBoundaryRefinementAccepted(point, source, p, sourceBefore, boundary) {
		return product.Value{}, false
	}
	if !r.callArgumentBoundaryRefinementAccepted(point, source, p, before, boundary) {
		return product.Value{}, false
	}
	return boundary, true
}

func (r Reader) callArgumentBoundaryRefinementAccepted(point cfg.Point, source factflow.ValueSource, p path.Path, before, boundary product.Value) bool {
	hasProof := r.callArgumentBoundaryHasRuntimeOrTypeProof(point, source, p)
	return r.result != nil && r.result.CallArgumentBoundaryRefinementAccepted(before, boundary, hasProof)
}

func (r Reader) callArgumentBoundaryHasRuntimeOrTypeProof(point cfg.Point, source factflow.ValueSource, p path.Path) bool {
	return r.sourceHasRuntimeValidationAuthority(point, source) ||
		r.pathHasRuntimeProof(point, p) ||
		r.rootPathHasDominatingRuntimeValidationAssignment(point, p) ||
		r.rootPathHasTrustedDominatingAssignmentSource(point, p)
}

func (r Reader) callArgumentBoundaryCanRefine(before, boundary product.Value) bool {
	return r.result != nil && r.result.CallArgumentBoundaryCanRefine(before, boundary)
}

func (r Reader) readablePathValueAtBoundary(point cfg.Point, p path.Path) (product.Value, bool) {
	value, ok := r.result.PathValueAtBoundary(point, p)
	if ok {
		return value, true
	}
	if p.Symbol != 0 && p.Version != 0 {
		stable := p
		stable.Version = 0
		if value, ok := r.result.PathValueAtBoundary(point, stable); ok {
			return value, true
		}
	}
	return product.Value{}, false
}

func (r Reader) rootPathHasTrustedDominatingAssignmentSource(point cfg.Point, p path.Path) bool {
	return r.result != nil && r.result.RootPathHasTrustedDominatingAssignmentSource(point, p)
}

func (r Reader) callArgumentBoundaryCandidate(point cfg.Point, source factflow.ValueSource, current product.Value) (product.Value, bool) {
	p, ok := r.valueSourcePath(source)
	if !ok || p.IsEmpty() {
		return product.Value{}, false
	}
	if len(p.Segments) == 0 {
		return product.Value{}, false
	}
	value, ok := r.result.PathValueAtBoundary(point, p)
	if !ok {
		return product.Value{}, false
	}
	currentType, currentTypeOK := r.ValueTypeWithPresence(current)
	candidateType, candidateTypeOK := r.ValueTypeWithPresence(value)
	if currentTypeOK && candidateTypeOK && readapi.TypeMayBeNilMismatch(currentType, candidateType) {
		return product.Value{}, false
	}
	if !r.callArgumentBoundaryCanRefine(current, value) {
		return product.Value{}, false
	}
	if r.valueHasReadableType(value) && r.ValueHash(value) != r.ValueHash(current) {
		return value, true
	}
	return product.Value{}, false
}

func (r Reader) objectEntryValue(point cfg.Point, entry factflow.ObjectEntryView) (product.Value, bool) {
	if value, ok := r.callArgumentValue(point, entry.Source()); ok {
		return value, true
	}
	return r.unknownArgumentValue()
}

func (r Reader) declaredTopArgumentValue(point cfg.Point, source factflow.ValueSource, p path.Path) (product.Value, bool) {
	if r.result == nil || r.typeValues == nil || p.IsEmpty() || len(p.Segments) != 0 {
		return product.Value{}, false
	}
	if r.pathHasPositiveRuntimeTypeGuard(point, p) ||
		r.rootPathHasDominatingRuntimeValidationAssignment(point, p) {
		return product.Value{}, false
	}
	declared, ok := r.explicitTopDeclaredPathTypeAt(p)
	base := unwrap.Optional(declared)
	if !ok || base == nil || (!typ.IsAny(base) && !typ.IsUnknown(base)) {
		return product.Value{}, false
	}
	reg := r.result.Registry()
	if reg == nil {
		return product.Value{}, false
	}
	value := typevalue.WithWitness(reg, r.typeValues.FromType(reg, declared), declared)
	value = product.Set(reg, value, evidence.Key, evidence.ExplicitTop())
	return product.Set(reg, value, assertion.Key, assertion.Any()), true
}

func (r Reader) rootPathHasDominatingRuntimeValidationAssignment(point cfg.Point, p path.Path) bool {
	return r.result != nil && r.result.RootPathHasDominatingRuntimeValidationAssignment(point, p)
}

func (r Reader) callArgumentDeclaredTopValue(point cfg.Point, source factflow.ValueSource, p path.Path, value product.Value) product.Value {
	if r.result == nil || r.ValueHasUntrustedTopOrigin(value) {
		return value
	}
	if r.sourceHasRuntimeValidationAuthority(point, source) || p.IsEmpty() {
		return value
	}
	if r.pathHasRuntimeProof(point, p) || r.rootPathHasDominatingRuntimeValidationAssignment(point, p) {
		return value
	}
	if declaredValue, declarationSource, ok := r.untrustedRootDeclarationValue(point, p); ok &&
		r.untrustedDeclarationStillDescribesValue(declaredValue, value, declarationSource) {
		return declaredValue
	}
	declared, ok := r.explicitTopDeclaredPathTypeAt(p)
	base := unwrap.Optional(declared)
	if !ok || base == nil || (!typ.IsAny(base) && !typ.IsUnknown(base)) {
		return value
	}
	reg := r.result.Registry()
	if reg == nil || r.typeValues == nil {
		return value
	}
	value = typevalue.WithWitness(reg, r.typeValues.FromType(reg, declared), declared)
	value = product.Set(reg, value, evidence.Key, evidence.ExplicitTop())
	return product.Set(reg, value, assertion.Key, assertion.Any())
}

func (r Reader) pathHasRuntimeProof(point cfg.Point, p path.Path) bool {
	return r.result != nil && r.result.PathHasRuntimeProof(point, p)
}

func (r Reader) untrustedRootDeclarationValue(point cfg.Point, p path.Path) (product.Value, factflow.ValueSource, bool) {
	if r.result == nil || p.IsEmpty() || p.Symbol == 0 || len(p.Segments) != 0 {
		return product.Value{}, factflow.ValueSource{}, false
	}
	declaration, ok := r.result.DominatingPathRootDeclarationSource(point, p)
	if !ok {
		return product.Value{}, factflow.ValueSource{}, false
	}
	value, ok := r.result.SourceValueAtBoundary(declaration.Point, declaration.Source)
	if !ok || !r.ValueHasUntrustedTopOrigin(value) {
		return product.Value{}, factflow.ValueSource{}, false
	}
	return value, declaration.Source, true
}

func (r Reader) untrustedDeclarationStillDescribesValue(declared, current product.Value, source factflow.ValueSource) bool {
	declaredType, declaredOK := r.ValueTypeWithPresence(declared)
	currentType, currentOK := r.ValueTypeWithPresence(current)
	if !declaredOK || !currentOK {
		return false
	}
	if typ.TypeEquals(declaredType, currentType) {
		return true
	}
	if (typ.IsAny(declaredType) || typ.IsUnknown(declaredType)) && source.HasExpr && source.ExprRef != 0 {
		_, operationOK := r.result.ExpressionOperationRef(source.ExprRef)
		return operationOK
	}
	return false
}

func (r Reader) explicitTopDeclaredPathTypeAt(p path.Path) (typ.Type, bool) {
	if r.result == nil {
		return nil, false
	}
	return r.result.ExplicitTopDeclaredPathType(p)
}

func (r Reader) pathHasPositiveRuntimeTypeGuard(point cfg.Point, p path.Path) bool {
	return r.result != nil && r.result.PathHasPositiveRuntimeTypeGuard(point, p)
}

func callArgumentSpan(site factflow.CallSiteView, index int) SourceSpan {
	span, ok := site.ArgumentSpanAt(index)
	if !ok {
		return SourceSpan{}
	}
	return sourceSpanFromFactflow(span)
}

func callArgumentSpans(site factflow.CallSiteView) []SourceSpan {
	count := site.ArgumentSourceCount()
	if count <= 0 {
		return nil
	}
	out := make([]SourceSpan, count)
	for i := 0; i < count; i++ {
		out[i] = callArgumentSpan(site, i)
	}
	return out
}

func (r Reader) callArgumentLabel(site factflow.CallSiteView, index int, source factflow.ValueSource) string {
	if index < 0 {
		return ""
	}
	if label, ok := site.ArgumentLabelAt(index); ok {
		return label
	}
	if r.result != nil {
		if p, ok := r.valueSourcePath(source); ok && !p.IsEmpty() {
			return r.displayPath(p)
		}
	}
	return ""
}
