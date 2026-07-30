package pass

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/check/readmodel"
)

// TypestateInvalidTransitions emits generic, declared-protocol lifecycle
// precondition failures. A protocol-specific surface may adapt these judgments
// (as channels do) without reimplementing typestate state inspection.
type TypestateInvalidTransitions struct{}

func (TypestateInvalidTransitions) Name() string {
	return "typestate.invalid_transition"
}

func (TypestateInvalidTransitions) Produce(ctx Context) []judgment.Judgment {
	functionKey := ctx.FunctionKey
	if functionKey == "" {
		functionKey = "body"
	}
	var out []judgment.Judgment
	ctx.Reader.ForEachTypestateInvalidTransition(func(item readmodel.TypestateInvalidTransition) bool {
		// The ambient channel runtime adapts the same solved failure facts to
		// its established channel.*.closed codes. Do not emit a second generic
		// diagnostic for that presentation-only protocol.
		if item.Protocol != "channel.lifecycle" {
			out = append(out, typestateInvalidTransitionJudgment(ctx, functionKey, item))
		}
		return true
	})
	return out
}

func typestateInvalidTransitionJudgment(ctx Context, functionKey string, item readmodel.TypestateInvalidTransition) judgment.Judgment {
	span := spanFromReadModel(ctx.SourceFile, item.Span)
	resource := item.Target
	if resource == "" {
		resource = item.Resource
	}
	return judgment.Judgment{
		Code:  judgment.CodeTypestateInvalidTransition,
		Point: item.Point,
		Subject: judgment.NewSubjectRef(
			functionKey,
			judgment.SubjectExpression,
			fmt.Sprintf("typestate-transition:%d:%s:%s:%s", item.Point, item.Protocol, item.Resource, item.Expected),
		).WithLabel(resource),
		Expected: judgment.TypeRef{Label: item.Expected},
		Actual:   judgment.ValueRef{Label: item.Found},
		Verdict:  judgment.VerdictRefuted,
		Evidence: judgment.EvidenceChain{{
			Kind:  judgment.EvidenceAbstractFact,
			Trust: judgment.EvidenceTrustProven,
			Detail: judgment.EvidenceDetail{
				Kind:         judgment.EvidenceDetailTypestateInvalidTransition,
				Resource:     resource,
				Protocol:     item.Protocol,
				FromState:    item.Expected,
				CurrentState: item.Found,
			},
			Origin: judgment.OriginRef{Point: item.Point, Key: "typestate:invalid-transition"},
			Span:   span,
		}},
		Spans: []judgment.SpanRef{span},
	}
}
