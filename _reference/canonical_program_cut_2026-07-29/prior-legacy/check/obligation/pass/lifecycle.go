package pass

import (
	"fmt"
	"strings"

	"github.com/wippyai/go-lua/analysis/check/judgment"
	"github.com/wippyai/go-lua/analysis/check/readmodel"
)

// LifecycleObligations emits obligations for resources that remain in a
// non-final typestate at function exit.
type LifecycleObligations struct{}

func (LifecycleObligations) Name() string {
	return "effect.lifecycle.unreleased"
}

func (LifecycleObligations) Produce(ctx Context) []judgment.Judgment {
	functionKey := ctx.FunctionKey
	if functionKey == "" {
		functionKey = "body"
	}
	var out []judgment.Judgment
	ctx.Reader.ForEachLifecycleObligation(func(obligation readmodel.LifecycleObligation) bool {
		out = append(out, lifecycleObligationJudgment(ctx, functionKey, obligation))
		return true
	})
	return out
}

func lifecycleObligationJudgment(ctx Context, functionKey string, obligation readmodel.LifecycleObligation) judgment.Judgment {
	resource := lifecycleResourceLabel(obligation)
	final := strings.Join(obligation.Finals, " or ")
	finalKey := lifecycleFinalStateKey(obligation.Finals)
	evidence := make(judgment.EvidenceChain, 0, len(obligation.Sites)+1)
	for _, site := range obligation.Sites {
		evidence = append(evidence, lifecycleSiteEvidence(ctx, site, resource, finalKey))
	}
	evidence = append(evidence, judgment.Evidence{
		Kind:  judgment.EvidenceMissingProof,
		Trust: judgment.EvidenceTrustRefuted,
		Detail: judgment.EvidenceDetail{
			Kind:         judgment.EvidenceDetailLifecycleMissingProof,
			Resource:     resource,
			Protocol:     obligation.Protocol,
			CurrentState: obligation.Current,
			FinalState:   finalKey,
		},
		Origin: judgment.OriginRef{
			Point: obligation.Point,
			Key:   "lifecycle:missing-proof",
		},
	})
	return judgment.Judgment{
		Code:  judgment.CodeLifecycle,
		Point: obligation.Point,
		Subject: judgment.NewSubjectRef(
			functionKey,
			judgment.SubjectPath,
			fmt.Sprintf("lifecycle:%s:%s", obligation.Protocol, obligation.Resource),
		).WithLabel(resource),
		Expected: judgment.TypeRef{Label: final},
		Actual:   judgment.ValueRef{Label: obligation.Current},
		Verdict:  judgment.VerdictRefuted,
		Evidence: evidence,
	}
}

func lifecycleResourceLabel(obligation readmodel.LifecycleObligation) string {
	for _, site := range obligation.Sites {
		if site.Kind == readmodel.LifecycleSiteAcquire && site.TargetLabel != "" {
			return site.TargetLabel
		}
	}
	if obligation.Resource != "" {
		return obligation.Resource
	}
	return "resource"
}

func lifecycleSiteEvidence(ctx Context, site readmodel.LifecycleSite, resource, final string) judgment.Evidence {
	detailKind := judgment.EvidenceDetailLifecycleAcquire
	switch site.Kind {
	case readmodel.LifecycleSiteTransition:
		detailKind = judgment.EvidenceDetailLifecycleTransition
	case readmodel.LifecycleSiteEscape:
		detailKind = judgment.EvidenceDetailLifecycleEscape
	}
	resourceName := resource
	if site.TargetLabel != "" {
		resourceName = site.TargetLabel
	}
	return judgment.Evidence{
		Kind:  judgment.EvidenceAbstractFact,
		Trust: judgment.EvidenceTrustProven,
		Detail: judgment.EvidenceDetail{
			Kind:       detailKind,
			Resource:   resourceName,
			Protocol:   site.Protocol,
			FromState:  site.From,
			ToState:    site.To,
			FinalState: final,
		},
		Origin: judgment.OriginRef{
			Point: site.Point,
			Key:   "lifecycle:site",
		},
		Span: spanFromReadModel(ctx.SourceFile, site.Span),
	}
}

func lifecycleFinalStateKey(finals []string) string {
	if len(finals) == 0 {
		return ""
	}
	return strings.Join(finals, "\x1f")
}
