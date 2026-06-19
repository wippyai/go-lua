package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/compiler/ast"
)

type frozenTableMutations producerContext

func (p frozenTableMutations) Produce(result *body.Result) []diagnostic.Diagnostic {
	if result == nil {
		return nil
	}
	graph := result.Graph()
	if graph == nil {
		return nil
	}
	envs := cachedGuardEnvironments(result)
	var out []diagnostic.Diagnostic
	for _, point := range graph.RPO() {
		if !guardEnvReachableAt(envs, point) {
			continue
		}
		if fact, ok := result.OrdinaryAssignment(point); ok {
			if d, ok := frozenTableMutationDiagnostic(result, graph, point, fact); ok {
				out = append(out, d)
			}
		}
		if d, ok := frozenTableCallMutationDiagnostic(result, graph, point); ok {
			out = append(out, d)
		}
	}
	return out
}

func frozenTableMutationDiagnostic(result *body.Result, graph cfg.Graph, point cfg.Point, fact semantics.OrdinaryAssignmentFact) (diagnostic.Diagnostic, bool) {
	if fact.Target == nil || !fact.HasContainerPath || fact.ContainerPath.IsEmpty() {
		return diagnostic.Diagnostic{}, false
	}
	tableID, ok := frozenMutationContainerIdentity(result, point, fact.ContainerPath)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	in, ok := result.StateAt(point)
	if !ok || !in.IsTableFrozen(tableID) {
		return diagnostic.Diagnostic{}, false
	}
	frozenSpan, hasFrozenSpan := frozenProofSpan(result, graph, point, fact.ContainerPath)
	return newFrozenTableMutationDiagnostic(fact, fact.ContainerPath, frozenSpan, hasFrozenSpan), true
}

func frozenTableCallMutationDiagnostic(result *body.Result, graph cfg.Graph, point cfg.Point) (diagnostic.Diagnostic, bool) {
	outcome, ok := result.CallOutcomeAt(point)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	site, ok := result.CallSite(point)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	in, ok := result.StateAt(point)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	call, ok := result.Call(point)
	if !ok || call.Call == nil {
		return diagnostic.Diagnostic{}, false
	}
	callSpan := ast.SpanOf(call.Call)
	for _, target := range frozenCallInvalidationTargets(result, site, outcome) {
		tableID, ok := frozenMutationContainerIdentity(result, point, target)
		if !ok || !in.IsTableFrozen(tableID) {
			continue
		}
		frozenSpan, hasFrozenSpan := frozenProofSpan(result, graph, point, target)
		return newFrozenTableCallMutationDiagnostic(callSpan, target, frozenSpan, hasFrozenSpan), true
	}
	return diagnostic.Diagnostic{}, false
}

func frozenCallInvalidationTargets(result *body.Result, site factflow.CallSite, outcome callpayload.CallOutcome) []pathdom.Path {
	var out []pathdom.Path
	appendSubstituted := func(bindings []pathdom.Path, target pathdom.Path) {
		substituted, ok := target.Substitute(bindings)
		if !ok || substituted.IsEmpty() {
			return
		}
		for _, existing := range out {
			if existing.Equal(substituted) {
				return
			}
		}
		out = append(out, substituted)
	}
	argBindings := callGuardArgumentBindings(result, site)
	callBindings := callGuardCallBindings(result, site)
	for _, invalidation := range outcome.ParamPathInvalidations {
		appendSubstituted(argBindings, invalidation.Path)
	}
	for _, invalidation := range outcome.NormalReturnFacts.PathInvalidations {
		appendSubstituted(callBindings, invalidation.Path)
	}
	return out
}

func frozenMutationContainerIdentity(result *body.Result, point cfg.Point, container pathdom.Path) (identity.ID, bool) {
	reg := result.Registry()
	if reg == nil {
		return identity.ID{}, false
	}
	value, ok := result.PathValueAtBoundary(point, container)
	if !ok {
		return identity.ID{}, false
	}
	id, ok := product.Get(reg, value, identity.Key).ID()
	return id, ok && id != (identity.ID{})
}

func frozenProofSpan(result *body.Result, graph cfg.Graph, stop cfg.Point, container pathdom.Path) (diagnostic.Span, bool) {
	if result == nil || graph == nil || container.IsEmpty() {
		return diagnostic.Span{}, false
	}
	for _, point := range graph.RPO() {
		if point == stop {
			break
		}
		outcome, ok := result.CallOutcomeAt(point)
		if !ok || len(outcome.NormalReturnFacts.FrozenTables) == 0 {
			continue
		}
		site, ok := result.CallSite(point)
		if !ok {
			continue
		}
		bindings := callGuardCallBindings(result, site)
		for _, fact := range outcome.NormalReturnFacts.FrozenTables {
			target, ok := fact.Target.Substitute(bindings)
			if !ok || !target.Equal(container) {
				continue
			}
			if call, ok := result.Call(point); ok && call.Call != nil {
				return ast.SpanOf(call.Call), true
			}
			return diagnostic.Span{}, false
		}
	}
	return diagnostic.Span{}, false
}

func newFrozenTableMutationDiagnostic(
	fact semantics.OrdinaryAssignmentFact,
	container pathdom.Path,
	frozenSpan diagnostic.Span,
	hasFrozenSpan bool,
) diagnostic.Diagnostic {
	targetSpan := ast.SpanOf(fact.Target)
	containerName := container.String()
	evidence := []diagnostic.Evidence{
		{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Span:    targetSpan,
			Message: frozenAssignmentEvidence(containerName),
		},
		{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Message: frozenIncomingStateEvidence(containerName),
		},
	}
	labels := []diagnostic.Label{sourceLabel(targetSpan, labelFrozenTableMutation)}
	if hasFrozenSpan && frozenSpan.Valid() {
		evidence[1].Span = frozenSpan
		evidence[1].Message = frozenAssignmentProofEvidence(containerName)
		labels = append(labels, sourceLabel(frozenSpan, labelFreezeProof))
	}
	return diagnostic.New(diagnostic.DiagnosticSpec{
		Span:        targetSpan,
		Code:        CodeFrozenTableMutation,
		Message:     frozenTableMutationMessage(containerName),
		Severity:    diagnostic.SeverityWarning,
		Explanation: diagnostic.NewExplanation(evidence...),
		Help:        frozenTableAssignmentHelp(),
		Labels:      labels,
	})
}

func newFrozenTableCallMutationDiagnostic(
	callSpan diagnostic.Span,
	container pathdom.Path,
	frozenSpan diagnostic.Span,
	hasFrozenSpan bool,
) diagnostic.Diagnostic {
	containerName := container.String()
	evidence := []diagnostic.Evidence{
		{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Span:    callSpan,
			Message: frozenCallMutationEvidence(containerName),
		},
		{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Message: frozenIncomingStateEvidence(containerName),
		},
	}
	labels := []diagnostic.Label{sourceLabel(callSpan, labelFrozenTableCall)}
	if hasFrozenSpan && frozenSpan.Valid() {
		evidence[1].Span = frozenSpan
		evidence[1].Message = frozenCallProofEvidence(containerName)
		labels = append(labels, sourceLabel(frozenSpan, labelFreezeProof))
	}
	return diagnostic.New(diagnostic.DiagnosticSpec{
		Span:        callSpan,
		Code:        CodeFrozenTableMutation,
		Message:     frozenTableCallMutationMessage(containerName),
		Severity:    diagnostic.SeverityWarning,
		Explanation: diagnostic.NewExplanation(evidence...),
		Help:        frozenTableCallHelp(),
		Labels:      labels,
	})
}
