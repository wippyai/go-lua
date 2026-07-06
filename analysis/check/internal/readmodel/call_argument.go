package readmodel

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/internal/sourcebridge"
	readapi "github.com/wippyai/go-lua/analysis/check/readmodel"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/lua/cfgfacts"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
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
	if callArgumentExpectedHasObjectEntries(want) {
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
	if mismatch, ok := subtype.RecordInterfaceMismatch(arg.TypeWithPresence, want); ok {
		switch mismatch.Kind {
		case subtype.InterfaceMismatchMissingMethod:
			plan.MissingRequiredMethod = mismatch.Method.Name
			plan.MissingRequiredMethodType = mismatch.Expected
		case subtype.InterfaceMismatchMethodType:
			plan.MethodMismatchName = mismatch.Method.Name
			plan.MethodMismatchExpected = mismatch.Expected
			plan.MethodMismatchActual = mismatch.Actual
		}
	}
	return plan, true
}

func callArgumentExpectedHasObjectEntries(t typ.Type) bool {
	switch tt := unwrap.Optional(t).(type) {
	case *typ.Record, *typ.Array, *typ.Map, *typ.ReadonlyMap, *typ.Tuple:
		return true
	case *typ.Union:
		for _, member := range tt.Members {
			if callArgumentExpectedHasObjectEntries(member) {
				return true
			}
		}
	}
	return false
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
	if reduced, ok := r.callArgumentRuntimeKindReducedType(point, source, value); ok {
		got = reduced
	}
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

func (r Reader) callArgumentRuntimeKindReducedType(point cfg.Point, source factflow.ValueSource, value product.Value) (typ.Type, bool) {
	if r.result == nil {
		return nil, false
	}
	if r.result.SourceHasRuntimeValidation(source) {
		return nil, false
	}
	p, ok := r.callArgumentExpressionPath(source)
	if !ok || !r.rootPathArgumentUsesBoundary(point, p) {
		return nil, false
	}
	declared, ok := r.result.DeclaredPathTypeAt(point, p, true)
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
	arg.Mismatch = CallArgumentMismatch{}
	return arg, true
}

func (r Reader) callerOwnedParameterArgument(point cfg.Point, source factflow.ValueSource) bool {
	if r.result == nil {
		return false
	}
	return r.callerOwnedParameterSource(point, source, nil)
}

func (r Reader) callerOwnedParameterSource(point cfg.Point, source factflow.ValueSource, active map[factflow.ExprRef]struct{}) bool {
	if r.result == nil {
		return false
	}
	if p, ok := r.valueSourcePath(source); ok {
		if r.callerOwnedParameterPath(p) {
			return true
		}
		if r.callerOwnedParameterDeclarationSource(point, p, active) {
			return true
		}
	}
	if !source.HasExpr || source.ExprRef == 0 {
		return false
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
	if r.result.SourceHasRuntimeValidation(source) {
		if value, ok := r.result.SourceValueAtBoundary(point, source); ok {
			return value, true
		}
	}
	if value, ok := r.trustedReadableSourceValueBeforeBoundary(point, source); ok {
		return value, true
	}
	if p, ok := r.callArgumentExpressionPath(source); ok {
		return r.callArgumentPathValue(point, source, p)
	}
	if source.Kind == factflow.ValueSourceCall {
		return r.result.SourceValueAtBoundary(point, source)
	}
	if value, ok := r.result.SourceValueBeforeBoundary(point, source); ok {
		return value, true
	}
	return r.result.SourceValueAtBoundary(point, source)
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
			return value, true
		}
	}
	if r.rootPathArgumentUsesBoundary(point, p) {
		if value, ok := r.result.PathValueBeforeBoundary(point, p); ok {
			if r.valueHasReadableType(value) {
				return value, true
			}
			if declared, ok := r.declaredRootArgumentValue(point, p); ok {
				return declared, true
			}
			return value, true
		}
		if value, ok := r.result.PathValueAtBoundary(point, p); ok {
			return value, true
		}
		if value, ok := r.result.SourceValueAtBoundary(point, source); ok {
			return value, true
		}
	}
	if source.Adjusted || source.Expanded {
		if r.rootPathArgumentUsesBoundary(point, p) {
			if value, ok := r.trustedReadableSourceValueAtBoundary(point, source); ok {
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
					return boundary, true
				}
			}
			return value, true
		}
		if value, ok := r.result.PathValueAtBoundary(point, p); ok {
			return value, true
		}
	}
	return product.Value{}, false
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
	if r.result == nil || p.IsEmpty() || len(p.Segments) != 0 {
		return false
	}
	if declared, ok := r.result.DeclaredPathTypeAt(point, p, true); ok && declared != nil &&
		!typ.IsAny(declared) && !typ.IsUnknown(declared) {
		return true
	}
	if _, _, ok := r.result.DominatingBranchCheckForPath(point, p, func(_ cfg.Point, check branchcond.Check, _ bool) bool {
		return check.Kind != branchcond.CheckNone
	}); ok {
		return true
	}
	if _, ok := r.result.DominatingTruthyBranchForPath(point, p); ok {
		return true
	}
	if value, ok := r.result.DominatingBranchRefinementValueForPath(point, p); ok && r.valueHasReadableType(value) {
		return true
	}
	return r.rootPathHasTrustedDominatingAssignmentSource(point, p) ||
		r.rootPathHasTrustedDeclarationSource(point, p) ||
		r.rootPathHasTrustedNumericForVariable(point, p) ||
		r.rootPathHasTrustedCurrentValue(point, p) ||
		r.rootPathHasTrustedEntryValue(p) ||
		r.rootPathHasTrustedGenericForVariable(point, p)
}

func (r Reader) rootPathHasTrustedDominatingAssignmentSource(point cfg.Point, p path.Path) bool {
	if r.result == nil || p.IsEmpty() || p.Symbol == 0 || len(p.Segments) != 0 || r.result.Graph() == nil {
		return false
	}
	var source factflow.ValueSource
	var sourcePoint cfg.Point
	found := false
	for _, candidate := range r.result.Graph().RPO() {
		if candidate == point || !r.result.PointDominates(candidate, point) {
			continue
		}
		candidateSource, ok := r.rootPathAssignmentSourceAt(candidate, p)
		if !ok {
			continue
		}
		if !found || r.result.PointDominates(sourcePoint, candidate) {
			source = candidateSource
			sourcePoint = candidate
			found = true
		}
	}
	if !found {
		return false
	}
	value, ok := r.result.SourceValueAtBoundary(sourcePoint, source)
	return ok && r.valueHasReadableType(value) && !r.ValueHasUntrustedTopOrigin(value)
}

func (r Reader) rootPathAssignmentSourceAt(point cfg.Point, p path.Path) (factflow.ValueSource, bool) {
	if local, ok := r.result.LocalAssignment(point); ok && local.HasSymbol && local.Symbol == p.Symbol {
		return sourcebridge.ValueSourceFromASTSource(local.Source)
	}
	ordinary, ok := r.result.OrdinaryAssignment(point)
	if !ok || !ordinary.HasSymbol || ordinary.Symbol != p.Symbol || (ordinary.HasPath && len(ordinary.Path.Segments) != 0) {
		return factflow.ValueSource{}, false
	}
	return sourcebridge.ValueSourceFromASTSource(ordinary.Source)
}

func (r Reader) rootPathHasTrustedDeclarationSource(point cfg.Point, p path.Path) bool {
	if r.result == nil || p.IsEmpty() || len(p.Segments) != 0 {
		return false
	}
	declaration, ok := r.result.DominatingPathRootDeclarationSource(point, p)
	if !ok {
		return false
	}
	value, ok := r.result.SourceValueAtBoundary(declaration.Point, declaration.Source)
	return ok && r.valueHasReadableType(value) && !r.ValueHasUntrustedTopOrigin(value)
}

func (r Reader) rootPathHasTrustedCurrentValue(point cfg.Point, p path.Path) bool {
	if r.result == nil || p.IsEmpty() || len(p.Segments) != 0 {
		return false
	}
	if value, ok := r.result.PathValueBeforeBoundary(point, p); ok &&
		r.valueHasReadableType(value) && !r.ValueHasUntrustedTopOrigin(value) {
		return true
	}
	if value, ok := r.result.PathValueAtBoundary(point, p); ok &&
		r.valueHasReadableType(value) && !r.ValueHasUntrustedTopOrigin(value) {
		return true
	}
	return false
}

func (r Reader) rootPathHasTrustedNumericForVariable(point cfg.Point, p path.Path) bool {
	if r.result == nil || p.IsEmpty() || p.Symbol == 0 || len(p.Segments) != 0 || r.result.Graph() == nil {
		return false
	}
	for _, candidate := range r.result.Graph().RPO() {
		fact, ok := r.result.NumericFor(candidate)
		if !ok || !fact.HasSymbol || fact.Symbol != p.Symbol {
			continue
		}
		value, ok := r.result.PathValueBeforeBoundary(point, p)
		return ok && r.valueHasReadableType(value)
	}
	return false
}

func (r Reader) rootPathHasTrustedEntryValue(p path.Path) bool {
	if r.result == nil || p.IsEmpty() || p.Symbol == 0 || len(p.Segments) != 0 {
		return false
	}
	entry, ok := r.result.EntryState()
	if !ok {
		return false
	}
	value := entry.ReadValue(r.result.Registry(), statekey.SymbolValue(p.Symbol))
	return r.valueHasReadableType(value) && !r.ValueHasUntrustedTopOrigin(value)
}

func (r Reader) rootPathHasTrustedGenericForVariable(point cfg.Point, p path.Path) bool {
	if r.result == nil || p.IsEmpty() || p.Symbol == 0 || len(p.Segments) != 0 || r.result.Graph() == nil {
		return false
	}
	for _, candidate := range r.result.Graph().RPO() {
		if candidate == point || !r.result.PointDominates(candidate, point) {
			continue
		}
		fact, ok := r.result.GenericFor(candidate)
		if !ok || fact.Role != cfgfacts.GenericForRoleVariable || !fact.HasSymbols {
			continue
		}
		for _, sym := range fact.Symbols {
			if sym != p.Symbol {
				continue
			}
			value, ok := r.result.PathValueAtBoundary(point, p)
			return ok && r.valueHasReadableType(value) && !r.ValueHasUntrustedTopOrigin(value)
		}
	}
	return false
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
	if r.result != nil {
		if p, ok := r.valueSourcePath(source); ok && !p.IsEmpty() {
			return r.displayPath(p)
		}
	}
	return ""
}
