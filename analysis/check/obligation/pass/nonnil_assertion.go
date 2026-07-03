package pass

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/check/readmodel"
)

// NonNilAssertions emits obligations for `expr!` assertions that are proven to
// fail because the operand is nil on every normally reachable path.
type NonNilAssertions struct{}

func (NonNilAssertions) Name() string {
	return "assertion.nonnil"
}

func (NonNilAssertions) Produce(ctx Context) []judgment.Judgment {
	functionKey := ctx.FunctionKey
	if functionKey == "" {
		functionKey = "body"
	}
	var out []judgment.Judgment
	ctx.Reader.ForEachNonNilAssertion(func(assertion readmodel.NonNilAssertion) bool {
		if !assertion.OperandNilOnly {
			return true
		}
		out = append(out, nonNilAssertionJudgment(ctx, functionKey, assertion))
		return true
	})
	return out
}

func nonNilAssertionJudgment(ctx Context, functionKey string, assertion readmodel.NonNilAssertion) judgment.Judgment {
	label := assertion.OperandLabel
	if label == "" {
		label = "value"
	}
	return judgment.Judgment{
		Code:  judgment.CodeNonNilAssertion,
		Point: assertion.Point,
		Subject: judgment.NewSubjectRef(
			functionKey,
			judgment.SubjectExpression,
			fmt.Sprintf("nonnil:%d:%s", assertion.Point, assertion.OperandKey),
		).WithLabel(label),
		Actual:  judgment.NewValueRef(assertion.ValueHash, assertion.TypeWithPresence).WithLabel(label),
		Verdict: judgment.VerdictRefuted,
		Evidence: judgment.EvidenceChain{
			{
				Kind:  judgment.EvidenceAbstractFact,
				Trust: judgment.EvidenceTrustProven,
				Origin: judgment.OriginRef{
					Point: assertion.Point,
					Key:   "nonnil:operand",
				},
				Span: spanFromReadModel(ctx.SourceFile, assertion.OperandSpan),
			},
		},
		Spans: []judgment.SpanRef{spanFromReadModel(ctx.SourceFile, assertion.OperandSpan)},
	}
}
