package pass

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/check/readmodel"
)

// TypestateRequirements checks declarative lifecycle preconditions at every
// resolved call boundary. Unlike a transition, a requirement never mutates the
// abstract store.
type TypestateRequirements struct{}

func (TypestateRequirements) Name() string { return "typestate.requirement" }

func (TypestateRequirements) Produce(ctx Context) []judgment.Judgment {
	functionKey := ctx.FunctionKey
	if functionKey == "" {
		functionKey = "body"
	}
	var out []judgment.Judgment
	ctx.Reader.ForEachTypestateRequirement(func(item readmodel.TypestateRequirement) bool {
		out = append(out, typestateRequirementJudgment(ctx, functionKey, item))
		return true
	})
	return out
}

func typestateRequirementJudgment(ctx Context, functionKey string, item readmodel.TypestateRequirement) judgment.Judgment {
	resource := item.Target
	if resource == "" {
		resource = item.Resource
	}
	code := judgment.CodeTypestateUnprovenRequirement
	verdict := judgment.VerdictRefuted
	evidenceKind := judgment.EvidenceMissingProof
	trust := judgment.EvidenceTrustRefuted
	detailKind := judgment.EvidenceDetailLifecycleMissingProof
	origin := "typestate:requirement:missing"
	if item.Refuted {
		code = judgment.CodeTypestateInvalidRequirement
		evidenceKind = judgment.EvidenceAbstractFact
		trust = judgment.EvidenceTrustProven
		detailKind = judgment.EvidenceDetailTypestateInvalidTransition
		origin = "typestate:requirement:invalid"
	}
	span := spanFromReadModel(ctx.SourceFile, item.Span)
	return judgment.Judgment{
		Code: code, Point: item.Point,
		Subject: judgment.NewSubjectRef(functionKey, judgment.SubjectExpression,
			fmt.Sprintf("typestate-requirement:%d:%s:%s:%s", item.Point, item.Protocol, item.Resource, item.Expected)).WithLabel(resource),
		Expected: judgment.TypeRef{Label: item.Expected}, Actual: judgment.ValueRef{Label: item.Found}, Verdict: verdict,
		Evidence: judgment.EvidenceChain{{
			Kind: evidenceKind, Trust: trust,
			Detail: judgment.EvidenceDetail{Kind: detailKind, Resource: resource, Protocol: item.Protocol, FromState: item.Expected, CurrentState: item.Found},
			Origin: judgment.OriginRef{Point: item.Point, Key: origin}, Span: span,
		}}, Spans: []judgment.SpanRef{span},
	}
}
