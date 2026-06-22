package diagnostics

import (
	"fmt"
	"strings"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/typecall"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

type genericInferenceConflict struct {
	Param         *typ.TypeParam
	Contributions []typecall.InferenceContribution
}

func genericObjectLiteralInferenceConflictDiagnostic(result *body.Result, call *ast.FuncCallExpr, name string, index int, arg ast.Expr, mismatch objectLiteralTypeMismatch, declSpan ast.Span, trace typecall.GenericCallTrace) (diagnostic.Diagnostic, bool) {
	conflict, ok := genericInferenceConflictForObjectLiteralMismatch(index, mismatch.segments, trace)
	if !ok {
		return diagnostic.Diagnostic{}, false
	}
	subject := fmt.Sprintf("argument %d", index+1)
	if mismatch.suffix != "" {
		subject += mismatch.suffix
	}
	frameExpr := mismatch.expr
	if frameExpr == nil {
		frameExpr = arg
	}
	callSpan := ast.SpanOf(call)
	argName := exprEvidenceName(frameExpr)
	argSpan := directCallArgumentSpan(call, frameExpr, index, argName)
	primarySpan := argSpan
	if !primarySpan.Valid() {
		primarySpan = callSpan
	}
	evidenceSubject := subject
	if argName != "" && argName != unknownSourceName {
		evidenceSubject = fmt.Sprintf("%s (%s)", subject, argName)
	}
	evidence := []diagnostic.Evidence{
		{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Span:    argSpan,
			Message: assignmentSourceTypeEvidenceDisplay(evidenceSubject, mismatch.got, ""),
		},
		{
			Kind:    diagnostic.EvidenceUserAssertion,
			Trust:   diagnostic.TrustClaimed,
			Span:    directCallDeclarationEvidenceSpan(call, declSpan),
			Message: genericInferenceConflictExpectationEvidence(name, index, conflict.paramName()),
		},
	}
	evidence = append(evidence, genericInferenceConflictContributionEvidence(result, name, arg, conflict)...)
	return diagnostic.New(diagnostic.DiagnosticSpec{
		Span:        primarySpan,
		Code:        CodeDirectCallArgType,
		Severity:    diagnostic.SeverityError,
		Message:     genericInferenceConflictMessage(name, index, conflict),
		Explanation: diagnostic.NewExplanation(evidence...),
		Help:        genericInferenceConflictHelp(conflict.paramName()),
		Labels:      []diagnostic.Label{sourceLabel(argSpan, labelArgumentValue)},
	}), true
}

func genericInferenceConflictForObjectLiteralMismatch(index int, segments []segment.Segment, trace typecall.GenericCallTrace) (genericInferenceConflict, bool) {
	params := genericInferenceMismatchParams(index, segments, trace)
	for _, param := range params {
		contributions := genericInferenceContributionsForParam(index, param, trace)
		if genericInferenceHasDistinctTypes(contributions) {
			return genericInferenceConflict{Param: param, Contributions: contributions}, true
		}
	}
	return genericInferenceConflict{}, false
}

func genericInferenceContributionsForParam(index int, param *typ.TypeParam, trace typecall.GenericCallTrace) []typecall.InferenceContribution {
	seen := map[string]struct{}{}
	var out []typecall.InferenceContribution
	for _, contribution := range trace.Contributions {
		if contribution.Index != index || contribution.Param == nil || contribution.Type == nil {
			continue
		}
		if !sameInferenceParam(param, contribution.Param) {
			continue
		}
		display := inferenceContributionDisplay(contribution)
		if display == "" {
			continue
		}
		key := display + "\x00" + formatType(contribution.Type)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, contribution)
		if len(out) >= 4 {
			break
		}
	}
	return out
}

func genericInferenceHasDistinctTypes(contributions []typecall.InferenceContribution) bool {
	if len(contributions) < 2 {
		return false
	}
	first := contributions[0].Type
	for _, contribution := range contributions[1:] {
		if !typ.SameNodeOrAcyclicEqual(first, contribution.Type) {
			return true
		}
	}
	return false
}

func (c genericInferenceConflict) paramName() string {
	if c.Param != nil && c.Param.Name != "" {
		return c.Param.Name
	}
	return "type parameter"
}

func genericInferenceConflictMessage(name string, index int, conflict genericInferenceConflict) string {
	first, second, ok := firstDistinctGenericInferencePair(conflict.Contributions)
	if !ok {
		return fmt.Sprintf("%s cannot infer one %s for argument %d", name, conflict.paramName(), index+1)
	}
	return fmt.Sprintf("%s cannot infer one %s for argument %d: %s implies %s, but %s implies %s",
		name,
		conflict.paramName(),
		index+1,
		inferenceContributionDisplay(first),
		formatType(first.Type),
		inferenceContributionDisplay(second),
		formatType(second.Type),
	)
}

func firstDistinctGenericInferencePair(contributions []typecall.InferenceContribution) (typecall.InferenceContribution, typecall.InferenceContribution, bool) {
	for i, first := range contributions {
		for _, second := range contributions[i+1:] {
			if !typ.SameNodeOrAcyclicEqual(first.Type, second.Type) {
				return first, second, true
			}
		}
	}
	return typecall.InferenceContribution{}, typecall.InferenceContribution{}, false
}

func genericInferenceConflictExpectationEvidence(name string, index int, paramName string) string {
	return fmt.Sprintf("%s parameter %d requires one consistent %s across this argument", name, index+1, paramName)
}

func genericInferenceConflictContributionEvidence(result *body.Result, name string, arg ast.Expr, conflict genericInferenceConflict) []diagnostic.Evidence {
	if result == nil || arg == nil || len(conflict.Contributions) == 0 {
		return nil
	}
	fact, _ := result.ObjectLiteral(arg)
	out := make([]diagnostic.Evidence, 0, len(conflict.Contributions))
	for _, contribution := range conflict.Contributions {
		out = append(out, diagnostic.Evidence{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Span:    inferenceContributionSpan(fact, contribution, arg),
			Message: genericInferenceContributionEvidence(name, contribution),
		})
	}
	return out
}

func genericInferenceConflictHelp(paramName string) string {
	return fmt.Sprintf("Make each use of `%s` in this argument agree on the same type, or split the callee signature into separate type parameters if those values are intentionally different.", paramName)
}

func genericInferenceEvidenceForObjectLiteralMismatch(result *body.Result, name string, index int, arg ast.Expr, mismatch objectLiteralTypeMismatch, trace typecall.GenericCallTrace) []diagnostic.Evidence {
	if result == nil || arg == nil || len(mismatch.segments) == 0 || len(trace.Contributions) == 0 {
		return nil
	}
	params := genericInferenceMismatchParams(index, mismatch.segments, trace)
	if len(params) == 0 {
		return nil
	}
	fact, _ := result.ObjectLiteral(arg)
	var out []diagnostic.Evidence
	seen := map[string]struct{}{}
	for _, contribution := range trace.Contributions {
		if contribution.Index != index || contribution.Param == nil || contribution.Type == nil {
			continue
		}
		if !genericInferenceParamSetContains(params, contribution.Param) {
			continue
		}
		display := inferenceContributionDisplay(contribution)
		if display == "" {
			continue
		}
		key := display + "\x00" + formatType(contribution.Type)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, diagnostic.Evidence{
			Kind:    diagnostic.EvidenceAbstractFact,
			Trust:   diagnostic.TrustProven,
			Span:    inferenceContributionSpan(fact, contribution, arg),
			Message: genericInferenceContributionEvidence(name, contribution),
		})
		if len(out) >= 4 {
			break
		}
	}
	return out
}

func genericInferenceMismatchParams(index int, segments []segment.Segment, trace typecall.GenericCallTrace) []*typ.TypeParam {
	var out []*typ.TypeParam
	for _, contribution := range trace.Contributions {
		if contribution.Index != index || contribution.Param == nil {
			continue
		}
		if !inferenceContributionMatchesSegments(contribution, segments) {
			continue
		}
		if genericInferenceParamSetContains(out, contribution.Param) {
			continue
		}
		out = append(out, contribution.Param)
	}
	return out
}

func genericInferenceParamSetContains(params []*typ.TypeParam, param *typ.TypeParam) bool {
	for _, candidate := range params {
		if sameInferenceParam(candidate, param) {
			return true
		}
	}
	return false
}

func sameInferenceParam(left, right *typ.TypeParam) bool {
	return left == right || (left != nil && right != nil && left.Equals(right))
}

func inferenceContributionMatchesSegments(contribution typecall.InferenceContribution, segments []segment.Segment) bool {
	if len(segments) == 0 {
		return false
	}
	segIndex := 0
	for _, step := range contribution.Path {
		if !inferencePathStepIsValueSegment(step) {
			continue
		}
		if segIndex >= len(segments) || !inferencePathStepMatchesSegment(step, segments[segIndex]) {
			return false
		}
		segIndex++
	}
	return segIndex == len(segments)
}

func inferencePathStepIsValueSegment(step typecall.InferencePathStep) bool {
	switch step.Kind {
	case typecall.InferencePathField, typecall.InferencePathStaticString, typecall.InferencePathStaticInt:
		return true
	default:
		return false
	}
}

func inferencePathStepMatchesSegment(step typecall.InferencePathStep, seg segment.Segment) bool {
	switch step.Kind {
	case typecall.InferencePathField:
		return (seg.Kind == segment.SegmentField || seg.Kind == segment.SegmentIndexString) && step.Name == seg.Name
	case typecall.InferencePathStaticString:
		return (seg.Kind == segment.SegmentIndexString || seg.Kind == segment.SegmentField) && step.Name == seg.Name
	case typecall.InferencePathStaticInt:
		return seg.Kind == segment.SegmentIndexInt && step.Index == seg.Index
	default:
		return false
	}
}

func inferenceContributionSpan(fact semantics.ObjectLiteralFact, contribution typecall.InferenceContribution, fallback ast.Expr) ast.Span {
	for _, entry := range fact.Entries {
		if inferenceContributionMatchesSegments(contribution, entry.Suffix.Segments) {
			return ast.SpanOf(entry.Value)
		}
	}
	return ast.SpanOf(fallback)
}

func genericInferenceContributionEvidence(name string, contribution typecall.InferenceContribution) string {
	paramName := "type parameter"
	if contribution.Param != nil && contribution.Param.Name != "" {
		paramName = contribution.Param.Name
	}
	return fmt.Sprintf("%s inferred %s includes %s from %s", name, paramName, formatType(contribution.Type), inferenceContributionDisplay(contribution))
}

func inferenceContributionDisplay(contribution typecall.InferenceContribution) string {
	var b strings.Builder
	if contribution.Index >= 0 {
		fmt.Fprintf(&b, "argument %d", contribution.Index+1)
	} else {
		b.WriteString("argument")
	}
	for _, step := range contribution.Path {
		switch step.Kind {
		case typecall.InferencePathField:
			b.WriteByte('.')
			b.WriteString(step.Name)
		case typecall.InferencePathStaticString:
			fmt.Fprintf(&b, "[%q]", step.Name)
		case typecall.InferencePathStaticInt:
			fmt.Fprintf(&b, "[%d]", step.Index)
		case typecall.InferencePathFunctionParam:
			if step.Name != "" {
				fmt.Fprintf(&b, " parameter %s", step.Name)
			} else if step.Index > 0 {
				fmt.Fprintf(&b, " parameter %d", step.Index)
			}
		case typecall.InferencePathFunctionReturn:
			if step.Index > 0 {
				fmt.Fprintf(&b, " return %d", step.Index)
			} else {
				b.WriteString(" return")
			}
		case typecall.InferencePathTypeArgument:
			continue
		}
	}
	return b.String()
}
