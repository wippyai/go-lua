package pass

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/check/readmodel"
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
		if assignment.Check.Admissible || assignment.Expected == nil || assignment.CascadeFromRefutedAssignment() {
			return true
		}
		out = append(out, assignmentJudgment(ctx, functionKey, assignment))
		return true
	})
	ctx.Reader.ForEachOptionalAssignmentTarget(func(target readmodel.OptionalAssignmentTarget) bool {
		out = append(out, optionalAssignmentTargetJudgment(ctx, functionKey, target))
		return true
	})
	return out
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
	case readmodel.AssignmentMismatchMissingRequiredMethod:
		missingProofDetail = judgment.MissingRequiredMethodTypeEvidenceDetail(
			assignment.Check.Mismatch.Field,
			assignment.Check.Mismatch.Type,
		)
	case readmodel.AssignmentMismatchMethodType:
		missingProofDetail = judgment.MethodTypeMismatchEvidenceDetail(
			assignment.Check.Mismatch.Field,
			assignment.Check.Mismatch.ActualType,
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
	if assignment.ParentContext.SourceLabel != "" && assignment.ParentContext.SourceType != nil {
		evidence = append(evidence, judgment.Evidence{
			Kind:   judgment.EvidenceAbstractFact,
			Trust:  judgment.EvidenceTrustProven,
			Detail: judgment.AssignmentParentActualEvidenceDetail(assignment.ParentContext.SourceLabel, assignment.ParentContext.SourceType),
			Origin: judgment.OriginRef{
				Point: assignment.Point,
				Key:   "assignment:parent-actual",
			},
			Span: spanFromReadModel(ctx.SourceFile, assignment.ParentContext.SourceSpan),
		})
	}
	if assignment.ParentContext.TargetLabel != "" && assignment.ParentContext.Expected != nil {
		evidence = append(evidence, judgment.Evidence{
			Kind:   judgment.EvidenceUserAssertion,
			Trust:  judgment.EvidenceTrustClaimed,
			Detail: judgment.AssignmentParentExpectedEvidenceDetail(assignment.ParentContext.TargetLabel, assignment.ParentContext.Expected),
			Origin: judgment.OriginRef{
				Point: assignment.Point,
				Key:   "assignment:parent-expected",
			},
			Span: spanFromReadModel(ctx.SourceFile, assignment.ParentContext.DeclarationSpan),
		})
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
				Cause:        judgment.EvidenceCause{Kind: judgment.EvidenceCauseFlowAssign},
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
	for i, invalidation := range assignment.CallInvalidations {
		evidence = append(evidence, judgment.Evidence{
			Kind:  judgment.EvidenceAbstractFact,
			Trust: judgment.EvidenceTrustProven,
			Detail: judgment.AssignmentCallInvalidationEvidenceDetail(
				invalidation.CallLabel,
				invalidation.InvalidatedLabel,
				invalidation.ReadLabel,
			),
			Origin: judgment.OriginRef{
				Point: assignment.Point,
				Key:   fmt.Sprintf("assignment:call-invalidation:%d", i),
			},
			Span: spanFromReadModel(ctx.SourceFile, invalidation.Span),
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
				Cause:        judgment.EvidenceCause{Kind: judgment.EvidenceCauseMissingProof},
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
