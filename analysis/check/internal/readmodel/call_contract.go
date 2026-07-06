package readmodel

import (
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/analysis/check/contract"
	"github.com/wippyai/go-lua/analysis/check/internal/callcontract"
	readapi "github.com/wippyai/go-lua/analysis/check/readmodel"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

const (
	CallContractSourceLocalFunction     = readapi.CallContractSourceLocalFunction
	CallContractSourceImportedSignature = readapi.CallContractSourceImportedSignature
	CallContractSourceFunctionValue     = readapi.CallContractSourceFunctionValue
	CallContractSourceMemberFunction    = readapi.CallContractSourceMemberFunction
)

type callContract struct {
	Contract                    contract.Contract
	Source                      CallContractSource
	GenericConstraintViolations []callcontract.ArgumentConstraintViolation
	GenericInferenceConflicts   []CallGenericInferenceConflict
}

type callParamObligation struct {
	Index  int
	Type   typ.Type
	Origin readapi.CallArgumentObligationOrigin
}

// callContractAt resolves the canonical callable contract for the call at
// point. It covers imported/registered signatures and local function values.
func (r Reader) callContractAt(point cfg.Point) (callContract, bool) {
	if r.result == nil {
		return callContract{}, false
	}
	site, ok := r.result.CallSite(point)
	if !ok {
		return callContract{}, false
	}
	if callContract, ok := r.declaredLocalCallContract(point, site); ok {
		return callContract, true
	}
	if fn, ok := r.result.CallSignatureType(site); ok {
		instantiated, violations, conflicts := r.instantiateCallFunctionType(point, site, fn)
		name, _ := r.result.CallSignatureName(site)
		return callContract{
			Contract:                    callcontract.BindReceiver(contract.FromFunctionType(instantiated), r.callReceiverTypeOrNil(point, site), callReceiverSupplied(site)),
			Source:                      CallContractSource{Kind: CallContractSourceImportedSignature, Name: name},
			GenericConstraintViolations: violations,
			GenericInferenceConflicts:   conflicts,
		}, true
	}
	if fn, ok := r.memberCallFunctionType(point, site); ok {
		instantiated, violations, conflicts := r.instantiateCallFunctionType(point, site, fn)
		return callContract{
			Contract:                    callcontract.BindReceiver(contract.FromFunctionType(instantiated), r.callReceiverTypeOrNil(point, site), callReceiverSupplied(site)),
			Source:                      CallContractSource{Kind: CallContractSourceMemberFunction, Name: r.callContractSourceName(site)},
			GenericConstraintViolations: violations,
			GenericInferenceConflicts:   conflicts,
		}, true
	}
	if fn, ok := r.result.FunctionValueTypeForCallSiteAtBoundary(point, site); ok {
		instantiated, violations, conflicts := r.instantiateCallFunctionType(point, site, fn)
		return callContract{
			Contract:                    callcontract.BindReceiver(contract.FromFunctionType(instantiated), r.callReceiverTypeOrNil(point, site), callReceiverSupplied(site)),
			Source:                      CallContractSource{Kind: CallContractSourceFunctionValue, Name: r.callContractSourceName(site), ParameterSpans: r.localFunctionParamTypeSpans(point, site), ResultSpans: r.localFunctionReturnTypeSpans(site)},
			GenericConstraintViolations: violations,
			GenericInferenceConflicts:   conflicts,
		}, true
	}
	return callContract{}, false
}

func (r Reader) declaredLocalCallContract(point cfg.Point, site factflow.CallSite) (callContract, bool) {
	if r.result == nil {
		return callContract{}, false
	}
	fn, ok := r.result.FunctionValueTypeForCalleePath(site.View().CalleePathKey())
	if !ok || fn == nil {
		return callContract{}, false
	}
	instantiated, violations, conflicts := r.instantiateCallFunctionType(point, site, fn)
	return callContract{
		Contract:                    callcontract.BindReceiver(contract.FromFunctionType(instantiated), r.callReceiverTypeOrNil(point, site), callReceiverSupplied(site)),
		Source:                      CallContractSource{Kind: CallContractSourceLocalFunction, Name: r.callContractSourceName(site), ParameterSpans: r.localFunctionParamTypeSpans(point, site), ResultSpans: r.localFunctionReturnTypeSpans(site)},
		GenericConstraintViolations: violations,
		GenericInferenceConflicts:   conflicts,
	}, true
}

func (r Reader) localFunctionParamTypeSpans(point cfg.Point, site factflow.CallSite) []SourceSpan {
	if r.result == nil {
		return nil
	}
	spans := r.result.FunctionParamTypeSpansForCalleePath(site.View().CalleePathKey())
	if len(spans) == 0 {
		spans = r.result.FunctionParamTypeSpansForTargetPath(site.CalleePathRef())
	}
	if len(spans) == 0 {
		if fn := r.result.DominatingFunctionDefinitionForPath(point, site.CalleePathRef()); fn != nil {
			slots := r.result.FunctionParamSlots(fn)
			spans = make([]factflow.SourceSpan, len(slots))
			for i := range slots {
				if span, ok := r.result.FunctionParamTypeSpan(fn, i); ok {
					spans[i] = span
				}
			}
		}
	}
	if len(spans) == 0 {
		return nil
	}
	out := make([]SourceSpan, len(spans))
	for i, span := range spans {
		out[i] = sourceSpanFromFactflow(span)
	}
	return out
}

func (r Reader) localFunctionReturnTypeSpans(site factflow.CallSite) []SourceSpan {
	if r.result == nil {
		return nil
	}
	if spans := r.result.FunctionReturnTypeSpansForCalleePath(site.View().CalleePathKey()); len(spans) != 0 {
		out := make([]SourceSpan, len(spans))
		for i, span := range spans {
			out[i] = sourceSpanFromFactflow(span)
		}
		return out
	}
	spans := r.result.FunctionReturnTypeSpansForTargetPath(site.CalleePathRef())
	if len(spans) == 0 {
		return sourceSpansFromBody(r.result.FallbackFunctionReturnTypeSpans(site))
	}
	out := make([]SourceSpan, len(spans))
	for i, span := range spans {
		out[i] = sourceSpanFromFactflow(span)
	}
	return out
}

func (r Reader) callContractSourceName(site factflow.CallSite) string {
	if r.result == nil {
		return ""
	}
	if methodPath, ok := site.MethodPath(); ok && !methodPath.IsEmpty() {
		display := methodPath.Clone()
		display.Root = methodPath.DisplayRoot(r.result.SymbolName)
		return display.String()
	}
	if callPath := site.CalleePathRef(); !callPath.IsEmpty() {
		display := callPath.Clone()
		display.Root = callPath.DisplayRoot(r.result.SymbolName)
		return display.String()
	}
	if name := r.result.SymbolName(site.CalleeSymbol()); name != "" {
		return name
	}
	if method := site.MethodName(); method != "" {
		return method
	}
	return ""
}

func (r Reader) instantiateCallFunctionType(point cfg.Point, site factflow.CallSite, fn *typ.Function) (*typ.Function, []callcontract.ArgumentConstraintViolation, []CallGenericInferenceConflict) {
	if r.result == nil || fn == nil || len(fn.TypeParams) == 0 {
		return fn, nil, nil
	}
	args := make([]typ.Type, site.ArgumentSourceCount())
	site.ForEachArgumentSource(func(index int, source factflow.ValueSource) bool {
		if fn, ok := r.contextualFunctionArgumentType(point, source); ok {
			args[index] = fn
			return true
		}
		if value, ok := r.callArgumentValue(point, source); ok {
			if t, ok := r.ValueTypeWithPresence(value); ok {
				args[index] = t
			}
		}
		return true
	})
	instantiated, violations, trace := callcontract.InstantiateGenericCallWithTrace(fn, args)
	conflicts := r.genericInferenceConflicts(point, trace)
	if instantiated == nil {
		return fn, violations, conflicts
	}
	return instantiated, violations, conflicts
}

func (r Reader) genericInferenceConflicts(point cfg.Point, trace callcontract.GenericCallTrace) []CallGenericInferenceConflict {
	conflicts := callcontract.PlanGenericInferenceConflicts(trace)
	if len(conflicts) == 0 {
		return nil
	}
	out := make([]CallGenericInferenceConflict, 0, len(conflicts))
	site, _ := r.result.CallSite(point)
	for _, conflict := range conflicts {
		out = append(out, CallGenericInferenceConflict{
			Index:         conflict.Index,
			FunctionName:  r.callContractSourceName(site),
			ParamName:     conflict.ParamName,
			Span:          r.callArgumentSpan(point, conflict.Index),
			Contributions: r.genericInferenceReportContributions(point, conflict.Index, conflict.Contributions),
		})
	}
	return out
}

func (r Reader) genericInferenceReportContributions(point cfg.Point, index int, contributions []callcontract.InferenceContribution) []CallGenericInferenceContribution {
	if len(contributions) == 0 {
		return nil
	}
	out := make([]CallGenericInferenceContribution, 0, len(contributions))
	for _, contribution := range contributions {
		out = append(out, CallGenericInferenceContribution{
			Type:  contribution.Type,
			Span:  r.inferenceContributionSpan(point, index, contribution),
			Label: r.inferenceContributionLabel(point, index, contribution),
		})
	}
	return out
}

func (r Reader) inferenceContributionLabel(point cfg.Point, index int, contribution callcontract.InferenceContribution) string {
	if r.result == nil {
		return genericInferenceContributionLabel(index, contribution)
	}
	site, ok := r.result.CallSite(point)
	if !ok || index < 0 {
		return genericInferenceContributionLabel(index, contribution)
	}
	source, ok := site.ArgumentSourceAt(index)
	if !ok || !source.HasExpr {
		return genericInferenceContributionLabel(index, contribution)
	}
	fact, ok := r.result.ObjectLiteralExpr(source.ExprRef)
	if !ok {
		return genericInferenceContributionLabel(index, contribution)
	}
	bestDepth := -1
	bestLabel := ""
	for _, entry := range fact.Entries() {
		suffix := entry.Suffix()
		depth := len(suffix.Segments)
		if depth <= bestDepth || !callcontract.InferenceContributionHasSegmentPrefix(contribution, suffix.Segments) {
			continue
		}
		if label := readapi.CallArgumentMemberLabel(index, suffix.Segments, entry.ValueLabel()); label != "" {
			label += genericInferenceCallableContributionSuffix(contribution, depth)
			bestDepth = depth
			bestLabel = label
		}
	}
	if bestLabel != "" {
		return bestLabel
	}
	return genericInferenceContributionLabel(index, contribution)
}

func genericInferenceCallableContributionSuffix(contribution callcontract.InferenceContribution, prefixDepth int) string {
	if prefixDepth < 0 || prefixDepth >= len(contribution.Path) {
		return ""
	}
	var b strings.Builder
	for _, step := range contribution.Path[prefixDepth:] {
		switch step.Kind {
		case callcontract.InferencePathFunctionParam:
			b.WriteString(" parameter ")
			if step.Name != "" {
				b.WriteString(step.Name)
			} else {
				b.WriteString(strconv.Itoa(step.Index + 1))
			}
		case callcontract.InferencePathFunctionReturn:
			b.WriteString(" return ")
			b.WriteString(strconv.Itoa(step.Index))
		}
	}
	return b.String()
}

func genericInferenceContributionLabel(index int, contribution callcontract.InferenceContribution) string {
	var b strings.Builder
	b.WriteString("argument ")
	b.WriteString(strconv.Itoa(index + 1))
	for _, step := range contribution.Path {
		switch step.Kind {
		case callcontract.InferencePathField:
			b.WriteString(".")
			b.WriteString(step.Name)
		case callcontract.InferencePathStaticString:
			b.WriteString("[")
			b.WriteString(strconv.Quote(step.Name))
			b.WriteString("]")
		case callcontract.InferencePathStaticInt:
			b.WriteString("[")
			b.WriteString(strconv.Itoa(step.Index))
			b.WriteString("]")
		case callcontract.InferencePathFunctionParam:
			b.WriteString(" parameter ")
			if step.Name != "" {
				b.WriteString(step.Name)
			} else {
				b.WriteString(strconv.Itoa(step.Index + 1))
			}
		case callcontract.InferencePathFunctionReturn:
			b.WriteString(" return ")
			b.WriteString(strconv.Itoa(step.Index))
		}
	}
	return b.String()
}

func (r Reader) inferenceContributionSpan(point cfg.Point, index int, contribution callcontract.InferenceContribution) SourceSpan {
	if r.result == nil {
		return SourceSpan{}
	}
	site, ok := r.result.CallSite(point)
	if !ok || index < 0 {
		return SourceSpan{}
	}
	plan := readapi.GenericInferenceContributionSpanPlan{
		Fallback: callArgumentSpan(site, index),
	}
	source, ok := site.ArgumentSourceAt(index)
	if ok && source.HasExpr {
		fact, ok := r.result.ObjectLiteralExpr(source.ExprRef)
		if ok {
			for _, entry := range fact.Entries() {
				suffix := entry.Suffix()
				plan.Candidates = append(plan.Candidates, readapi.GenericInferenceContributionSpanCandidate{
					Span:         sourceSpanFromFactflow(entry.ValueSpan()),
					SegmentDepth: len(suffix.Segments),
					Matches:      callcontract.InferenceContributionHasSegmentPrefix(contribution, suffix.Segments),
				})
			}
		}
	}
	return readapi.PlanGenericInferenceContributionSpan(plan)
}

// callParamObligationsAt returns pre-call argument obligations projected from
// the solved call outcome at point.
func (r Reader) callParamObligationsAt(point cfg.Point) []callParamObligation {
	if r.result == nil {
		return nil
	}
	outcome, ok := r.result.CallOutcomeAt(point)
	if !ok || len(outcome.ParamObligations) == 0 {
		return nil
	}
	out := make([]callParamObligation, 0, len(outcome.ParamObligations))
	for _, obligation := range outcome.ParamObligations {
		t, ok := r.ValueTypeWithPresence(obligation.Value)
		if !ok || !readapi.CallArgumentObligationTypeReportable(t) {
			continue
		}
		next := callParamObligation{
			Index:  obligation.ParamIndex,
			Type:   t,
			Origin: r.callParamObligationOrigin(point, obligation),
		}
		out = appendPreferredCallParamObligation(out, next)
	}
	return out
}

func appendPreferredCallParamObligation(out []callParamObligation, next callParamObligation) []callParamObligation {
	if !next.Origin.HasOrigin {
		for _, existing := range out {
			if existing.Index == next.Index && existing.Origin.HasOrigin && typ.TypeEquals(existing.Type, next.Type) {
				return out
			}
		}
		return append(out, next)
	}
	for i, existing := range out {
		if existing.Index != next.Index || existing.Origin.HasOrigin || !typ.TypeEquals(existing.Type, next.Type) {
			continue
		}
		out[i] = next
		return out
	}
	return append(out, next)
}

func (r Reader) callParamObligationOrigin(point cfg.Point, obligation callpayload.CallParamObligation) readapi.CallArgumentObligationOrigin {
	if !obligation.Origin.HasOrigin {
		return readapi.CallArgumentObligationOrigin{}
	}
	site, _ := r.result.CallSite(point)
	return readapi.CallArgumentObligationOrigin{
		HasOrigin:         true,
		FunctionName:      r.callContractSourceName(site),
		SubjectLabel:      callParamObligationSubjectLabel(obligation.Origin),
		ProviderLabel:     callParamObligationProviderLabel(obligation.Origin),
		MemberParamNumber: obligation.Origin.MemberParamIndex + 1,
	}
}

func callParamObligationSubjectLabel(origin callpayload.CallParamObligationOrigin) string {
	if origin.SubjectLabel != "" {
		return origin.SubjectLabel
	}
	return callArgumentLabel(origin.ArgParam)
}

func callParamObligationProviderLabel(origin callpayload.CallParamObligationOrigin) string {
	if origin.ProviderLabel != "" {
		return origin.ProviderLabel
	}
	var segs []segment.Segment
	if origin.ReceiverPath != "" {
		var ok bool
		segs, ok = pathaddr.RelativeStaticMemberSuffixSegments(origin.ReceiverPath)
		if !ok {
			return readapi.CallArgumentMemberLabel(origin.ReceiverParam, []segment.Segment{origin.Member}, "")
		}
	}
	segs = append(segs, origin.Member)
	return readapi.CallArgumentMemberLabel(origin.ReceiverParam, segs, "")
}

func callArgumentLabel(index int) string {
	return "argument " + strconv.Itoa(index+1)
}

func (r Reader) callReceiverTypeOrNil(point cfg.Point, site factflow.CallSite) typ.Type {
	receiver, ok := r.callReceiverType(point, site)
	if !ok {
		return nil
	}
	return receiver
}

func callReceiverSupplied(site factflow.CallSite) bool {
	if _, ok := site.ReceiverSource(); ok {
		return true
	}
	return false
}
