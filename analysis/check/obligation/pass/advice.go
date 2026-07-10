package pass

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/check/readmodel"
)

// AdviceRedundantClaims emits hint-level judgments for removable runtime type
// claims whose operands are already proven to satisfy the claimed type.
type AdviceRedundantClaims struct{}

func (AdviceRedundantClaims) Name() string {
	return "advice.redundant_claim"
}

func (AdviceRedundantClaims) Produce(ctx Context) []judgment.Judgment {
	return produceReaderJudgments(ctx, readmodel.Reader.ForEachRedundantClaim, adviceRedundantClaimJudgment)
}

// AdviceAlwaysTrueGuards emits hint-level judgments for reachable branch
// conditions whose value is a singleton boolean.
type AdviceAlwaysTrueGuards struct{}

func (AdviceAlwaysTrueGuards) Name() string {
	return "advice.always_true_guard"
}

func (AdviceAlwaysTrueGuards) Produce(ctx Context) []judgment.Judgment {
	return produceReaderJudgments(ctx, readmodel.Reader.ForEachAlwaysTrueGuard, adviceAlwaysTrueGuardJudgment)
}

// AdviceInvariantLoopReads emits hint-level judgments for hoistable static
// member/index reads inside loops.
type AdviceInvariantLoopReads struct{}

func (AdviceInvariantLoopReads) Name() string {
	return "advice.invariant_loop_read"
}

func (AdviceInvariantLoopReads) Produce(ctx Context) []judgment.Judgment {
	return produceReaderJudgments(ctx, readmodel.Reader.ForEachInvariantLoopRead, adviceInvariantLoopReadJudgment)
}

// AdviceSplitBirthDiscriminants emits hint-level judgments for locally born
// record variants whose discriminant tag and payload fields are assigned apart.
type AdviceSplitBirthDiscriminants struct{}

func (AdviceSplitBirthDiscriminants) Name() string {
	return "advice.split_birth_discriminant"
}

func (AdviceSplitBirthDiscriminants) Produce(ctx Context) []judgment.Judgment {
	return produceReaderJudgments(ctx, readmodel.Reader.ForEachSplitBirthDiscriminant, adviceSplitBirthDiscriminantJudgment)
}

func adviceRedundantClaimJudgment(ctx Context, functionKey string, claim readmodel.RedundantClaim) judgment.Judgment {
	claimSpan := spanFromReadModel(ctx.SourceFile, claim.ClaimSpan)
	operandSpan := spanFromReadModel(ctx.SourceFile, claim.OperandSpan)
	return judgment.Judgment{
		Code:  judgment.CodeAdviceRedundantClaim,
		Point: claim.Point,
		Subject: subjectRef(functionKey, judgment.SubjectExpression,
			fmt.Sprintf("advice-claim:%d:%d:%d", claim.Point, claim.ClaimSpan.StartLine, claim.ClaimSpan.StartCol),
			claim.ClaimLabel),
		Expected: judgment.NewTypeRef(claim.ClaimedType),
		Actual:   judgment.NewValueRef(0, claim.OperandType).WithLabel(claim.OperandLabel),
		Verdict:  judgment.VerdictProven,
		Evidence: judgment.EvidenceChain{
			provenAbstractEvidence(claim.Point, "advice:claim:operand-type", judgment.AdviceProvenTypeEvidenceDetail(claim.OperandLabel, claim.ClaimedType), operandSpan),
			provenAbstractEvidence(claim.Point, "advice:claim:site", judgment.AdviceClaimSiteEvidenceDetail(claim.ClaimedType), claimSpan),
		},
		Spans: []judgment.SpanRef{claimSpan, operandSpan},
	}
}

func adviceAlwaysTrueGuardJudgment(ctx Context, functionKey string, guard readmodel.AlwaysTrueGuard) judgment.Judgment {
	span := spanFromReadModel(ctx.SourceFile, guard.ConditionSpan)
	return judgment.Judgment{
		Code:  judgment.CodeAdviceAlwaysTrueGuard,
		Point: guard.Point,
		Subject: subjectRef(functionKey, judgment.SubjectExpression,
			fmt.Sprintf("advice-guard:%d:%d:%d", guard.Point, guard.ConditionSpan.StartLine, guard.ConditionSpan.StartCol),
			guard.ConditionLabel),
		Actual:  judgment.NewValueRef(0, guard.ConditionType).WithLabel(guard.ConditionLabel),
		Verdict: judgment.VerdictProven,
		Evidence: judgment.EvidenceChain{
			provenAbstractEvidence(guard.Point, "advice:guard:value", judgment.AdviceGuardValueEvidenceDetail(guard.Always), span),
		},
		Spans: []judgment.SpanRef{span},
	}
}

func adviceInvariantLoopReadJudgment(ctx Context, functionKey string, read readmodel.InvariantLoopRead) judgment.Judgment {
	readSpan := spanFromReadModel(ctx.SourceFile, read.ReadSpan)
	loopSpan := spanFromReadModel(ctx.SourceFile, read.LoopSpan)
	return judgment.Judgment{
		Code:  judgment.CodeAdviceInvariantLoopRead,
		Point: read.Point,
		Subject: subjectRef(functionKey, judgment.SubjectExpression,
			fmt.Sprintf("advice-loop-read:%d:%s", read.Point, read.ReadPath.String()),
			read.ReadLabel),
		Actual:  judgment.NewValueRef(0, read.ReceiverType).WithLabel(read.ReceiverLabel),
		Verdict: judgment.VerdictProven,
		Evidence: judgment.EvidenceChain{
			provenAbstractEvidence(read.Point, "advice:loop-read:stable", judgment.AdviceLoopInvariantEvidenceDetail(read.ReadPath.String()), readSpan),
			provenAbstractEvidence(read.Point, "advice:loop-read:receiver-non-nil", judgment.AdviceReceiverNonNilEvidenceDetail(read.ReceiverLabel), readSpan),
		},
		Spans: []judgment.SpanRef{readSpan, loopSpan},
	}
}

func adviceSplitBirthDiscriminantJudgment(ctx Context, functionKey string, item readmodel.SplitBirthDiscriminant) judgment.Judgment {
	tagSpan := spanFromReadModel(ctx.SourceFile, item.TagWriteSpan)
	birthSpan := spanFromReadModel(ctx.SourceFile, item.BirthSpan)
	useSpan := spanFromReadModel(ctx.SourceFile, item.DiscriminantUseSpan)
	evidence := judgment.EvidenceChain{
		provenAbstractEvidence(item.BirthPoint, "advice:split-birth:birth", judgment.AdviceTableBirthEvidenceDetail(item.ReceiverLabel), birthSpan),
		provenAbstractEvidence(item.Point, "advice:split-birth:tag", judgment.AdviceTagWriteEvidenceDetail(item.TagLabel, item.TagValue), tagSpan),
	}
	spans := []judgment.SpanRef{tagSpan, birthSpan}
	for _, payload := range item.PayloadWrites {
		payloadSpan := spanFromReadModel(ctx.SourceFile, payload.Span)
		evidence = append(evidence, provenAbstractEvidence(payload.Point, "advice:split-birth:payload", judgment.AdvicePayloadWriteEvidenceDetail(payload.Label), payloadSpan))
		spans = append(spans, payloadSpan)
	}
	evidence = append(evidence, provenAbstractEvidence(item.DiscriminantUsePoint, "advice:split-birth:use", judgment.AdviceDiscriminantUseEvidenceDetail(item.TagLabel), useSpan))
	spans = append(spans, useSpan)
	return judgment.Judgment{
		Code:  judgment.CodeAdviceSplitBirthDiscriminant,
		Point: item.Point,
		Subject: subjectRef(functionKey, judgment.SubjectExpression,
			fmt.Sprintf("advice-split-birth:%d:%s", item.Point, item.TagLabel),
			item.TagLabel),
		Actual:   judgment.NewValueRef(0, nil).WithLabel(item.ReceiverLabel),
		Verdict:  judgment.VerdictProven,
		Evidence: evidence,
		Spans:    spans,
	}
}
