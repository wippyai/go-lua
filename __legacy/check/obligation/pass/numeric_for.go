package pass

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/check/readmodel"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// NumericForOperands emits obligations for numeric-for init/limit/step operands
// that are statically known not to be numbers.
type NumericForOperands struct{}

func (NumericForOperands) Name() string {
	return "for.numeric.operand"
}

func (NumericForOperands) Produce(ctx Context) []judgment.Judgment {
	functionKey := ctx.FunctionKey
	if functionKey == "" {
		functionKey = "body"
	}
	var out []judgment.Judgment
	ctx.Reader.ForEachNumericForOperand(func(operand readmodel.NumericForOperand) bool {
		if !operand.DefinitelyNotNumber {
			return true
		}
		out = append(out, numericForOperandJudgment(ctx, functionKey, operand))
		return true
	})
	return out
}

func numericForOperandJudgment(ctx Context, functionKey string, operand readmodel.NumericForOperand) judgment.Judgment {
	label := operand.OperandLabel
	if label == "" {
		label = operand.Role
	}
	evidence := judgment.EvidenceChain{
		{
			Kind:  judgment.EvidenceAbstractFact,
			Trust: judgment.EvidenceTrustProven,
			Origin: judgment.OriginRef{
				Point: operand.Point,
				Key:   "numeric-for:operand",
			},
			Span: spanFromReadModel(ctx.SourceFile, operand.OperandSpan),
		},
	}
	if operand.ExplicitTopLikeCast {
		evidence = append(evidence,
			judgment.Evidence{
				Kind:   judgment.EvidenceUserAssertion,
				Trust:  judgment.EvidenceTrustClaimed,
				Detail: judgment.UserAssertedAnyEvidenceDetail(label),
				Origin: judgment.OriginRef{
					Point: operand.Point,
					Key:   "numeric-for:explicit-top",
				},
				Span: spanFromReadModel(ctx.SourceFile, operand.OperandSpan),
			},
			judgment.Evidence{
				Kind:  judgment.EvidencePrecisionBoundary,
				Trust: judgment.EvidenceTrustUnknown,
				Origin: judgment.OriginRef{
					Point: operand.Point,
					Key:   "numeric-for:precision-boundary",
				},
				Span: spanFromReadModel(ctx.SourceFile, operand.OperandSpan),
			},
			judgment.Evidence{
				Kind:  judgment.EvidenceMissingProof,
				Trust: judgment.EvidenceTrustUnknown,
				Origin: judgment.OriginRef{
					Point: operand.Point,
					Key:   "numeric-for:missing-proof",
				},
				Span: spanFromReadModel(ctx.SourceFile, operand.OperandSpan),
			},
		)
	}
	return judgment.Judgment{
		Code:  judgment.CodeNumericForOperand,
		Point: operand.Point,
		Subject: judgment.NewSubjectRef(
			functionKey,
			judgment.SubjectExpression,
			fmt.Sprintf("numeric-for:%d:%s", operand.Point, operand.OperandKey),
		).WithLabel(label),
		Expected: judgment.NewTypeRef(typ.Number).WithLabel(operand.Role),
		Actual:   judgment.NewValueRef(0, operand.TypeWithPresence).WithLabel(label),
		Verdict:  judgment.VerdictRefuted,
		Evidence: evidence,
		Spans:    []judgment.SpanRef{spanFromReadModel(ctx.SourceFile, operand.OperandSpan)},
	}
}
