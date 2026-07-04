package body

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/compiler/ast"
)

// AssignmentCallInvalidation records a prior call whose solved outcome
// invalidated an assignment source read, making any earlier guard stale.
type AssignmentCallInvalidation struct {
	CallLabel        string
	ReadLabel        string
	InvalidatedLabel string
	Span             SourceSpan
}

// AssignmentCallInvalidations returns concrete call invalidations that can
// reach the assignment source read at point. It only reports explicit solved
// invalidation authority; unknown-call conservatism does not produce a
// specific evidence node.
func (r *Result) AssignmentCallInvalidations(point cfg.Point, expr ast.Expr) []AssignmentCallInvalidation {
	if r == nil || expr == nil || r.Graph() == nil {
		return nil
	}
	readPath, ok := r.ExpressionPath(expr)
	if !ok || readPath.IsEmpty() {
		return nil
	}
	readLabel := AssignmentSourceLabel(expr)
	if readLabel == "" {
		readLabel = readPath.String()
	}
	var out []AssignmentCallInvalidation
	seen := map[cfg.Point]struct{}{}
	for _, candidate := range r.Graph().RPO() {
		if candidate == point {
			break
		}
		if !r.PointCanReach(candidate, point) {
			continue
		}
		site, ok := r.CallSite(candidate)
		if !ok {
			continue
		}
		outcome, ok := r.CallOutcomeAt(candidate)
		if !ok || !CallOutcomeHasExplicitGuardInvalidation(outcome) {
			continue
		}
		if _, done := seen[candidate]; done {
			continue
		}
		invalidation, ok := r.callInvalidationEvidence(site, outcome, readPath)
		if !ok {
			continue
		}
		seen[candidate] = struct{}{}
		invalidation.ReadLabel = readLabel
		out = append(out, invalidation)
	}
	return out
}

func (r *Result) callInvalidationEvidence(site factflow.CallSite, outcome callpayload.CallOutcome, readPath pathdom.Path) (AssignmentCallInvalidation, bool) {
	callLabel := callSiteEvidenceLabel(site)
	if callLabel == "" {
		callLabel = "call"
	}
	span := callInvalidationSourceSpan(site.CalleeSpan())
	if span.StartLine == 0 {
		span = callInvalidationSourceSpan(site.CallSpan())
	}
	if CallOutcomeHasGlobalGuardInvalidation(outcome) {
		return AssignmentCallInvalidation{
			CallLabel:        callLabel,
			InvalidatedLabel: readPath.String(),
			Span:             span,
		}, true
	}
	invalidated, ok := r.CallOutcomeGuardInvalidationPaths(site, outcome)
	if !ok {
		return AssignmentCallInvalidation{}, false
	}
	for _, candidate := range invalidated {
		if !callInvalidationPathClearsGuardFact(candidate.Path, readPath) {
			continue
		}
		label := candidate.Path.String()
		if label == "" {
			label = readPath.String()
		}
		return AssignmentCallInvalidation{
			CallLabel:        callLabel,
			InvalidatedLabel: label,
			Span:             span,
		}, true
	}
	return AssignmentCallInvalidation{}, false
}

func callInvalidationSourceSpan(span factflow.SourceSpan) SourceSpan {
	return SourceSpan{
		StartLine: span.StartLine,
		StartCol:  span.StartCol,
		EndLine:   span.EndLine,
		EndCol:    span.EndCol,
	}
}

func callSiteEvidenceLabel(site factflow.CallSite) string {
	if site.MethodName() != "" {
		if receiver, ok := site.ReceiverPath(); ok && !receiver.IsEmpty() {
			return receiver.String() + ":" + site.MethodName() + "(...)"
		}
	}
	if callee := site.CalleePath(); !callee.IsEmpty() {
		return callee.String() + "(...)"
	}
	if method, ok := site.MethodPath(); ok && !method.IsEmpty() {
		return method.String() + "(...)"
	}
	return ""
}
