package pass

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/check/readmodel"
	"github.com/wippyai/go-lua/analysis/type/typ"
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
	return out
}

func assignmentJudgment(ctx Context, functionKey string, assignment readmodel.Assignment) judgment.Judgment {
	verdict := judgment.VerdictUnknown
	if assignment.Check.ProvenMismatch {
		verdict = judgment.VerdictRefuted
	}
	actualTrust := judgment.EvidenceTrustUnknown
	if !typ.TypeEquals(assignment.TypeWithPresence, nil) {
		actualTrust = judgment.EvidenceTrustProven
	}
	actual := assignment.TypeWithPresence
	if typ.TypeEquals(actual, nil) {
		actual = typ.Unknown
	}
	if verdict == judgment.VerdictUnknown &&
		assignment.UntrustedTopOrigin &&
		(typ.IsAny(actual) || typ.IsUnknown(actual) || typ.TypeEquals(actual, assignment.Expected)) {
		actual = typ.Any
	}
	missingProofTrust := judgment.EvidenceTrustUnknown
	if verdict == judgment.VerdictRefuted {
		missingProofTrust = judgment.EvidenceTrustRefuted
	}
	var missingProofDetail judgment.EvidenceDetail
	if assignment.Check.Mismatch.Kind == readmodel.AssignmentMismatchMissingRequiredField {
		missingProofDetail = judgment.MissingRequiredFieldTypeEvidenceDetail(
			assignment.Check.Mismatch.Field,
			assignment.Check.Mismatch.Type,
		)
	}
	evidence := judgment.EvidenceChain{
		{
			Kind:  judgment.EvidenceAbstractFact,
			Trust: actualTrust,
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
	return judgment.Judgment{
		Code:  judgment.CodeAssignment,
		Point: assignment.Point,
		Subject: judgment.NewSubjectRef(
			functionKey,
			judgment.SubjectPath,
			fmt.Sprintf("assignment:%d:%s", assignment.Point, assignment.TargetKey),
		).WithLabel(assignment.TargetLabel),
		Expected: judgment.NewTypeRef(assignment.Expected).WithLabel(assignment.ExpectedLabel),
		Actual:   judgment.NewValueRef(assignment.ValueHash, actual).WithLabel(assignment.SourceLabel),
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
