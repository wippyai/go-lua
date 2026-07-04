package readmodel

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	readapi "github.com/wippyai/go-lua/analysis/check/readmodel"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// callArgumentMismatchSubjectPlan projects object-literal facts into pure
// readmodel candidates. Public readmodel owns selecting the report subject.
func (r Reader) callArgumentMismatchSubjectPlan(point cfg.Point, arg CallArgument, want typ.Type) (readapi.CallArgumentMismatchSubjectPlan, bool) {
	if r.result == nil || want == nil {
		return readapi.CallArgumentMismatchSubjectPlan{}, false
	}
	site, ok := r.result.CallSite(point)
	if !ok || arg.Index < 0 {
		return readapi.CallArgumentMismatchSubjectPlan{}, false
	}
	source, ok := site.ArgumentSourceAt(arg.Index)
	if !ok || !source.HasExpr {
		return readapi.CallArgumentMismatchSubjectPlan{}, false
	}
	lit, ok := r.result.ObjectLiteralExpr(source.ExprRef)
	if !ok {
		return readapi.CallArgumentMismatchSubjectPlan{}, false
	}
	plan := readapi.CallArgumentMismatchSubjectPlan{
		Argument: arg,
		Expected: want,
	}
	for _, entry := range lit.Entries() {
		suffix := entry.Suffix()
		expected, ok := body.ExpectedTypeAtSegments(want, suffix.Segments)
		if !ok || expected == nil {
			continue
		}
		value, ok := r.objectEntryValue(point, entry)
		if !ok {
			continue
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
				CallerOwnedParameter: arg.CallerOwnedParameter,
				FunctionType:         arg.FunctionType,
				Span:                 sourceSpanFromFactflow(entry.ValueSpan()),
				Label:                readapi.CallArgumentMemberLabel(arg.Index, suffix.Segments, entry.ValueLabel()),
			},
			Expected:    expected,
			LabelSuffix: readapi.CallArgumentExpectedLabelSuffix(suffix.Segments),
			Admissible:  r.ValueProofAdmissible(value, expected),
		})
	}
	if field, ok := body.MissingRequiredRecordField(want, func(name string) bool {
		for _, entry := range lit.Entries() {
			suffix := entry.Suffix()
			if len(suffix.Segments) != 1 {
				continue
			}
			seg := suffix.Segments[0]
			if seg.Kind == segment.SegmentField && seg.Name == name {
				return true
			}
		}
		return false
	}); ok {
		plan.MissingRequiredField = field
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
		Argument:            arg,
		Expected:            want,
		ExpectedLabel:       expectedLabel,
		ExpectedSpan:        expectedSpan,
		ValueAdmissible:     r.ValueProofAdmissible(arg.Value, want),
		ValueProvenMismatch: r.ValueWitnessProvenMismatch(arg.Value, want),
		IsSubtype:           r.IsSubtype,
		SubjectPlan:         subjectPlan,
	}
	return readapi.PlanCallArgumentCheck(plan)
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
			site, _ := r.result.CallSite(point)
			origin = readapi.CallArgumentObligationOrigin{
				HasOrigin:    true,
				FunctionName: r.callContractSourceName(site),
				SubjectLabel: callArgumentLabel(obligation.Index),
			}
		}
		plan.OutcomeParams = append(plan.OutcomeParams, readapi.IndexedCallArgumentObligation{
			Index: obligation.Index,
			Obligation: CallArgumentObligation{
				Type:          obligation.Type,
				ExpectedLabel: CallContractSource{}.ParameterLabel(obligation.Index),
				ExpectedSpan:  callArgumentSpanByIndex(args, obligation.Index),
				Origin:        origin,
			},
		})
	}
	return readapi.PlanCallArgumentReports(plan)
}

func callArgumentSpanByIndex(args []CallArgument, index int) SourceSpan {
	for _, arg := range args {
		if arg.Index == index {
			return arg.Span
		}
	}
	return SourceSpan{}
}

func (r Reader) callArityReport(site factflow.CallSite, contract callContract, hasContract bool) CallArityReport {
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
	site, ok := r.result.CallSite(point)
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

func (r Reader) callArgument(point cfg.Point, site factflow.CallSite, index int, source factflow.ValueSource, value product.Value) CallArgument {
	got, _ := r.ValueTypeWithPresence(value)
	arg := CallArgument{
		Index:                index,
		Value:                value,
		ValueHash:            r.ValueHash(value),
		TypeWithPresence:     got,
		UntrustedTopOrigin:   r.ValueHasUntrustedTopOrigin(value),
		ExplicitTopOrigin:    r.ValueHasExplicitTopOrigin(value),
		CallerOwnedParameter: r.callerOwnedParameterArgument(point, source),
		Span:                 callArgumentSpan(site, index),
		Label:                r.callArgumentLabel(site, index, source),
	}
	if candidate, ok := r.callArgumentBoundaryCandidate(point, source, value); ok {
		arg.ProofCandidateValue = candidate
		arg.ProofCandidateHash = r.ValueHash(candidate)
		arg.ProofCandidateType, _ = r.ValueTypeWithPresence(candidate)
		arg.ProofCandidateTop = r.ValueHasUntrustedTopOrigin(candidate)
		arg.ProofCandidateExplicitTop = r.ValueHasExplicitTopOrigin(candidate)
		arg.HasProofCandidate = true
	}
	if fn, ok := r.contextualFunctionArgumentType(point, source); ok {
		arg.FunctionType = fn
		arg.TypeWithPresence = fn
	} else if fn, ok := r.result.FunctionValueTypeForValueAtBoundary(point, value); ok {
		arg.FunctionType = fn
		arg.TypeWithPresence = fn
	}
	return arg
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
	arg.Mismatch = CallArgumentMismatch{}
	return arg, true
}

func (r Reader) callerOwnedParameterArgument(point cfg.Point, source factflow.ValueSource) bool {
	if r.result == nil || !source.HasExpr {
		return false
	}
	return r.callerOwnedParameterSource(point, source, nil)
}

func (r Reader) callerOwnedParameterSource(point cfg.Point, source factflow.ValueSource, active map[factflow.ExprRef]struct{}) bool {
	if r.result == nil || !source.HasExpr || source.ExprRef == 0 {
		return false
	}
	p, ok := r.result.ExpressionRefPath(source.ExprRef)
	if ok && r.callerOwnedParameterPath(p) {
		return true
	}
	if ok && r.callerOwnedParameterDeclarationSource(point, p, active) {
		return true
	}
	if active == nil {
		active = make(map[factflow.ExprRef]struct{}, 1)
	}
	if _, seen := active[source.ExprRef]; seen {
		return false
	}
	active[source.ExprRef] = struct{}{}
	op, ok := r.result.ExpressionOperationRef(source.ExprRef)
	if ok {
		if r.callerOwnedParameterSource(point, op.Left(), active) {
			return true
		}
		if op.Kind() == factflow.ExpressionOperationBinary && r.callerOwnedParameterSource(point, op.Right(), active) {
			return true
		}
	}
	if dyn, ok := r.result.DynamicIndexExpressionRef(source.ExprRef); ok {
		if tableSource, ok := dyn.TableSource(); ok && r.callerOwnedParameterSource(point, tableSource, active) {
			return true
		}
		if tablePath := dyn.TablePathRef(); !tablePath.IsEmpty() {
			if r.callerOwnedParameterPath(tablePath) || r.callerOwnedParameterDeclarationSource(point, tablePath, active) {
				return true
			}
		}
	}
	return false
}

func (r Reader) callerOwnedParameterDeclarationSource(point cfg.Point, p path.Path, active map[factflow.ExprRef]struct{}) bool {
	if p.IsEmpty() || p.Symbol == 0 || point == 0 || r.result == nil || r.result.Graph() == nil {
		return false
	}
	declaration, ok := r.result.DominatingPathRootDeclarationSource(point, p)
	if !ok || !declaration.Source.HasExpr {
		return false
	}
	return r.callerOwnedParameterSource(declaration.Point, declaration.Source, active)
}

func (r Reader) callerOwnedParameterPath(p path.Path) bool {
	if p.Symbol == 0 {
		return false
	}
	fn := r.result.Function()
	if fn == nil {
		return false
	}
	for _, slot := range r.result.FunctionParamSlots(fn) {
		if slot.Symbol != p.Symbol {
			continue
		}
		if !r.result.SymbolHasTypeAnnotation(slot.Symbol) {
			return true
		}
		t, ok := r.result.SymbolDeclaredType(slot.Symbol)
		return ok && readapi.ObligationTypeContainsFreeTypeParam(t)
	}
	return false
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

func (r Reader) callArgumentSpan(point cfg.Point, index int) SourceSpan {
	if r.result == nil {
		return SourceSpan{}
	}
	site, ok := r.result.CallSite(point)
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
	if r.result == nil {
		return product.Value{}, false
	}
	if source.Kind == factflow.ValueSourceCall {
		return r.result.SourceValueAtBoundary(point, source)
	}
	if value, ok := r.result.SourceValueBeforeBoundary(point, source); ok {
		return value, true
	}
	return r.result.SourceValueAtBoundary(point, source)
}

func (r Reader) callArgumentBoundaryCandidate(point cfg.Point, source factflow.ValueSource, current product.Value) (product.Value, bool) {
	if r.result == nil || !source.HasExpr || r.ValueHasUntrustedTopOrigin(current) {
		return product.Value{}, false
	}
	p, ok := r.result.ExpressionPathRef(source.ExprRef)
	if !ok || p.IsEmpty() {
		return product.Value{}, false
	}
	value, ok := r.result.PathValueAtBoundary(point, p)
	if !ok {
		return product.Value{}, false
	}
	if r.valueHasReadableType(value) && r.ValueHash(value) != r.ValueHash(current) {
		return value, true
	}
	return product.Value{}, false
}

func (r Reader) objectEntryValue(point cfg.Point, entry factflow.ObjectEntry) (product.Value, bool) {
	if value, ok := r.callArgumentValue(point, entry.Source()); ok {
		return value, true
	}
	return r.unknownArgumentValue()
}

func callArgumentSpan(site factflow.CallSite, index int) SourceSpan {
	span, ok := site.ArgumentSpanAt(index)
	if !ok {
		return SourceSpan{}
	}
	return sourceSpanFromFactflow(span)
}

func callArgumentSpans(site factflow.CallSite) []SourceSpan {
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

func (r Reader) callArgumentLabel(site factflow.CallSite, index int, source factflow.ValueSource) string {
	if index < 0 {
		return ""
	}
	if label, ok := site.ArgumentLabelAt(index); ok {
		return label
	}
	if r.result != nil && source.HasExpr {
		if p, ok := r.result.ExpressionPathRef(source.ExprRef); ok && !p.IsEmpty() {
			display := p.Clone()
			display.Root = p.DisplayRoot(r.result.SymbolName)
			return display.String()
		}
	}
	return ""
}
