package pass

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/check/readmodel"
)

// ConcatOperands emits obligations for `..` operands that may be nil at runtime.
type ConcatOperands struct{}

func (ConcatOperands) Name() string {
	return "operator.concat.operand"
}

func (ConcatOperands) Produce(ctx Context) []judgment.Judgment {
	functionKey := ctx.FunctionKey
	if functionKey == "" {
		functionKey = "body"
	}
	var out []judgment.Judgment
	ctx.Reader.ForEachConcatOperand(func(operand readmodel.ConcatOperand) bool {
		if !operand.NilRisk() {
			return true
		}
		out = append(out, concatOperandJudgment(ctx, functionKey, operand))
		return true
	})
	return out
}

func concatOperandJudgment(ctx Context, functionKey string, operand readmodel.ConcatOperand) judgment.Judgment {
	label := operand.OperandLabel
	if label == "" {
		label = "operand"
	}
	span := spanFromReadModel(ctx.SourceFile, operand.OperandSpan)
	evidence := judgment.EvidenceChain{
		{
			Kind:  judgment.EvidenceAbstractFact,
			Trust: judgment.EvidenceTrustProven,
			Origin: judgment.OriginRef{
				Point: operand.Point,
				Key:   "concat:operand",
			},
			Detail: judgment.EvidenceDetail{
				Kind:  judgment.EvidenceDetailConcatOperand,
				Field: operand.Side,
				Cause: judgment.EvidenceCause{Kind: judgment.EvidenceCauseFlowAssign},
			},
			Span: span,
		},
	}
	evidence = appendNilabilityProvenance(evidence, operand.Point, ctx.SourceFile, label, operand.OperandSpan, operand.Nilability)
	return judgment.Judgment{
		Code:  judgment.CodeConcatOperand,
		Point: operand.Point,
		Subject: judgment.NewSubjectRef(
			functionKey,
			judgment.SubjectExpression,
			fmt.Sprintf("concat:%d:%s", operand.Point, operand.OperandKey),
		).WithLabel(label),
		Actual:   judgment.NewValueRef(0, operand.TypeWithPresence).WithLabel(label),
		Verdict:  judgment.VerdictRefuted,
		Evidence: evidence,
		Spans:    []judgment.SpanRef{span},
	}
}
