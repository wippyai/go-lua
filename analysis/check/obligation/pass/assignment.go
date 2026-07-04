package pass

import (
	"fmt"
	"strings"

	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/check/readmodel"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// Assignments emits annotated-assignment type obligations from solved state.
type Assignments struct{}

func (Assignments) Name() string {
	return "assignment.type"
}

func (Assignments) Produce(ctx Context) []judgment.Judgment {
	functionKey := ctx.FunctionKey
	if functionKey == "" {
		functionKey = "body"
	}
	var out []judgment.Judgment
	ctx.Reader.ForEachAssignment(func(assignment readmodel.Assignment) bool {
		if assignment.Check.Admissible || assignment.Expected == nil {
			return true
		}
		out = append(out, assignmentJudgment(ctx, functionKey, assignment))
		return true
	})
	ctx.Reader.ForEachOptionalAssignmentTarget(func(target readmodel.OptionalAssignmentTarget) bool {
		out = append(out, optionalAssignmentTargetJudgment(ctx, functionKey, target))
		return true
	})
	return suppressAssignmentCascadeJudgments(out)
}

func suppressAssignmentCascadeJudgments(in []judgment.Judgment) []judgment.Judgment {
	if len(in) < 2 {
		return in
	}
	refutedTargets := make(map[string]assignmentCascadeCause)
	refutedDynamicRoots := make(map[string]assignmentCascadeCause)
	out := in[:0]
	for _, item := range in {
		if item.Code == judgment.CodeAssignment &&
			item.Verdict == judgment.VerdictRefuted &&
			item.Subject.Label != "" {
			cause := assignmentCascadeCauseFor(item)
			refutedTargets[item.Subject.Label] = cause
			if root, ok := refutedDynamicAssignmentRootKey(item); ok {
				refutedDynamicRoots[root] = cause
			}
		}
		if item.Code == judgment.CodeAssignment &&
			item.Actual.Label != "" &&
			item.Subject.Label != item.Actual.Label {
			if cause, ok := refutedTargets[item.Actual.Label]; ok && assignmentCascadeCausePrecedes(cause, item) {
				continue
			}
			if assignmentSourceCoveredByDynamicRoot(item.Actual.Key, refutedDynamicRoots, item) {
				continue
			}
		}
		out = append(out, item)
	}
	return out
}

type assignmentCascadeCause struct {
	point cfg.Point
	span  judgment.SpanRef
}

func assignmentCascadeCauseFor(item judgment.Judgment) assignmentCascadeCause {
	cause := assignmentCascadeCause{point: item.Point}
	if len(item.Spans) != 0 {
		cause.span = item.Spans[0]
	}
	return cause
}

func assignmentCascadeCausePrecedes(cause assignmentCascadeCause, item judgment.Judgment) bool {
	if cause.point < item.Point {
		return true
	}
	if len(item.Spans) == 0 || cause.span.StartLine == 0 || item.Spans[0].StartLine == 0 {
		return false
	}
	return spanBefore(cause.span, item.Spans[0])
}

func spanBefore(a, b judgment.SpanRef) bool {
	if a.File != "" && b.File != "" && a.File != b.File {
		return false
	}
	if a.StartLine != b.StartLine {
		return a.StartLine < b.StartLine
	}
	return a.StartCol < b.StartCol
}

func refutedDynamicAssignmentRootKey(item judgment.Judgment) (string, bool) {
	if !assignmentHasDynamicTargetDetail(item) {
		return "", false
	}
	prefix := fmt.Sprintf("assignment:%d:", item.Point)
	targetKey, ok := strings.CutPrefix(item.Subject.Key, prefix)
	if !ok || targetKey == "" || !strings.HasPrefix(targetKey, "path:") {
		return "", false
	}
	return targetKey, true
}

func assignmentHasDynamicTargetDetail(item judgment.Judgment) bool {
	for _, evidence := range item.Evidence {
		if evidence.Detail.Kind == judgment.EvidenceDetailDynamicAssignmentTarget {
			return true
		}
	}
	return false
}

func assignmentSourceCoveredByDynamicRoot(sourceKey string, roots map[string]assignmentCascadeCause, item judgment.Judgment) bool {
	if sourceKey == "" || len(roots) == 0 {
		return false
	}
	for root, cause := range roots {
		if !assignmentCascadeCausePrecedes(cause, item) {
			continue
		}
		if sourceKey == root || strings.HasPrefix(sourceKey, root+".") || strings.HasPrefix(sourceKey, root+"[") {
			return true
		}
	}
	return false
}

func assignmentJudgment(ctx Context, functionKey string, assignment readmodel.Assignment) judgment.Judgment {
	verdict := judgment.VerdictUnknown
	if assignment.Check.ProvenMismatch {
		verdict = judgment.VerdictRefuted
	}
	actualTrust := judgment.EvidenceTrustUnknown
	if assignment.ActualTypeKnown() {
		actualTrust = judgment.EvidenceTrustProven
	}
	missingProofTrust := judgment.EvidenceTrustUnknown
	if assignment.MissingProofRefuted() {
		missingProofTrust = judgment.EvidenceTrustRefuted
	}
	var missingProofDetail judgment.EvidenceDetail
	switch assignment.Check.Mismatch.Kind {
	case readmodel.AssignmentMismatchMissingRequiredField:
		missingProofDetail = judgment.MissingRequiredFieldTypeEvidenceDetail(
			assignment.Check.Mismatch.Field,
			assignment.Check.Mismatch.Type,
		)
	case readmodel.AssignmentMismatchMayBeNil:
		if assignment.SourceIndexedRead {
			missingProofDetail = judgment.IndexedReadMissingProofEvidenceDetail()
		} else {
			missingProofDetail = judgment.MayBeNilEvidenceDetail()
		}
	}
	var sourceDetail judgment.EvidenceDetail
	if assignment.CallResult.Present {
		if assignment.CallResult.UnderSupplied {
			sourceDetail = judgment.UnderSuppliedCallResultAssignmentEvidenceDetail(
				assignment.CallResult.CallableName,
				assignment.CallResult.ResultIndex,
			)
		} else {
			sourceDetail = judgment.CallResultAssignmentEvidenceDetail(
				assignment.CallResult.CallableName,
				assignment.CallResult.ResultIndex,
			)
		}
	}
	evidence := judgment.EvidenceChain{
		{
			Kind:   judgment.EvidenceAbstractFact,
			Trust:  actualTrust,
			Detail: sourceDetail,
			Origin: judgment.OriginRef{
				Point: assignment.Point,
				Key:   "assignment:actual",
			},
			Span: spanFromReadModel(ctx.SourceFile, assignment.SourceSpan),
		},
		{
			Kind:  judgment.EvidenceUserAssertion,
			Trust: judgment.EvidenceTrustClaimed,
			Origin: judgment.OriginRef{
				Point: assignment.Point,
				Key:   "assignment:expected",
			},
			Span: spanFromReadModel(ctx.SourceFile, assignment.DeclarationSpan),
		},
	}
	if assignment.CallResult.Present && !assignment.CallResult.UnderSupplied && assignment.CallResult.ReturnSpan.StartLine != 0 {
		evidence = append(evidence, judgment.Evidence{
			Kind:   judgment.EvidenceUserAssertion,
			Trust:  judgment.EvidenceTrustClaimed,
			Detail: sourceDetail,
			Origin: judgment.OriginRef{
				Point: assignment.Point,
				Key:   "assignment:call-result-return",
			},
			Span: spanFromReadModel(ctx.SourceFile, assignment.CallResult.ReturnSpan),
		})
	}
	if assignment.ExpectedSource == readmodel.AssignmentExpectedDynamicTarget {
		evidence[1].Detail = judgment.DynamicAssignmentTargetEvidenceDetail(assignment.TargetLabel)
	}
	if assignment.ExplicitTopOrigin {
		sourceLabel := assignment.SourceLabel
		if sourceLabel == "" {
			sourceLabel = "assigned value"
		}
		evidence = append(evidence, judgment.Evidence{
			Kind:   judgment.EvidenceUserAssertion,
			Trust:  judgment.EvidenceTrustClaimed,
			Detail: judgment.UserAssertedAnyEvidenceDetail(sourceLabel),
			Origin: judgment.OriginRef{
				Point: assignment.Point,
				Key:   "assignment:explicit-any",
			},
			Span: spanFromReadModel(ctx.SourceFile, assignment.SourceSpan),
		})
	}
	for i, access := range assignment.NilableAccesses {
		evidence = append(evidence, judgment.Evidence{
			Kind:  judgment.EvidenceAbstractFact,
			Trust: judgment.EvidenceTrustProven,
			Detail: judgment.EvidenceDetail{
				Kind:         judgment.EvidenceDetailMayBeNil,
				SubjectLabel: access.Label,
				Field:        access.Access,
			},
			Origin: judgment.OriginRef{
				Point: assignment.Point,
				Key:   fmt.Sprintf("assignment:nilable-access:%d", i),
			},
			Span: spanFromReadModel(ctx.SourceFile, access.Span),
		})
	}
	for i, contributor := range assignment.SourceContributors {
		evidence = append(evidence, judgment.Evidence{
			Kind:  judgment.EvidenceAbstractFact,
			Trust: judgment.EvidenceTrustProven,
			Detail: judgment.AssignmentSourceContributionEvidenceDetail(
				contributor.RootLabel,
				contributor.ReadLabel,
				contributor.Type,
			),
			Origin: judgment.OriginRef{
				Point: assignment.Point,
				Key:   fmt.Sprintf("assignment:source-contributor:%d", i),
			},
			Span: spanFromReadModel(ctx.SourceFile, contributor.Span),
		})
	}
	evidence = append(evidence, judgment.Evidence{
		Kind:   judgment.EvidenceMissingProof,
		Trust:  missingProofTrust,
		Detail: missingProofDetail,
		Origin: judgment.OriginRef{
			Point: assignment.Point,
			Key:   "assignment:proof",
		},
	})
	if assignment.UntrustedTopOrigin {
		evidence = append(evidence, judgment.Evidence{
			Kind:  judgment.EvidencePrecisionBoundary,
			Trust: judgment.EvidenceTrustUnknown,
			Origin: judgment.OriginRef{
				Point: assignment.Point,
				Key:   "assignment:untrusted",
			},
		})
	}
	actual := judgment.NewValueRef(assignment.ValueHash, assignment.EffectiveActualType()).WithLabel(assignment.SourceLabel)
	if assignment.SourceKey != "" {
		actual.Key = assignment.SourceKey
	}
	return judgment.Judgment{
		Code:  judgment.CodeAssignment,
		Point: assignment.Point,
		Subject: judgment.NewSubjectRef(
			functionKey,
			judgment.SubjectPath,
			fmt.Sprintf("assignment:%d:%s", assignment.Point, assignment.TargetKey),
		).WithLabel(assignment.TargetLabel),
		Expected: judgment.NewTypeRef(assignment.Expected).WithLabel(assignment.ExpectedLabel),
		Actual:   actual,
		Verdict:  verdict,
		Evidence: evidence,
		Spans:    []judgment.SpanRef{spanFromReadModel(ctx.SourceFile, assignment.SourceSpan)},
	}
}

func optionalAssignmentTargetJudgment(ctx Context, functionKey string, target readmodel.OptionalAssignmentTarget) judgment.Judgment {
	evidence := judgment.EvidenceChain{
		{
			Kind:  judgment.EvidenceAbstractFact,
			Trust: judgment.EvidenceTrustProven,
			Origin: judgment.OriginRef{
				Point: target.Point,
				Key:   "assignment:optional-container",
			},
			Span: spanFromReadModel(ctx.SourceFile, target.ContainerSpan),
		},
		{
			Kind:  judgment.EvidenceMissingProof,
			Trust: judgment.EvidenceTrustRefuted,
			Detail: judgment.EvidenceDetail{
				Kind:         judgment.EvidenceDetailMayBeNil,
				SubjectLabel: target.ContainerLabel,
			},
			Origin: judgment.OriginRef{
				Point: target.Point,
				Key:   "assignment:optional-target",
			},
			Span: spanFromReadModel(ctx.SourceFile, target.TargetSpan),
		},
	}
	return judgment.Judgment{
		Code:  judgment.CodeAssignmentTarget,
		Point: target.Point,
		Subject: judgment.NewSubjectRef(
			functionKey,
			judgment.SubjectPath,
			fmt.Sprintf("assignment:%d:%s", target.Point, target.TargetKey),
		).WithLabel(target.TargetLabel),
		Actual:   judgment.NewValueRef(0, target.ContainerType).WithLabel(target.ContainerLabel),
		Verdict:  judgment.VerdictRefuted,
		Evidence: evidence,
		Spans:    []judgment.SpanRef{spanFromReadModel(ctx.SourceFile, target.TargetSpan)},
	}
}
