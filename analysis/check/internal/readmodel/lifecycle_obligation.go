package readmodel

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	readapi "github.com/wippyai/go-lua/analysis/check/readmodel"
)

// ForEachLifecycleObligation visits typestate resources whose obligations remain
// open at function exit, with reachable lifecycle fact sites attached as
// renderer-independent evidence.
func (r Reader) ForEachLifecycleObligation(visit func(LifecycleObligation) bool) bool {
	if r.result == nil || visit == nil {
		return false
	}
	proofs := r.result.LifecycleObligationProofs()
	for _, proof := range proofs {
		if !visit(lifecycleObligationFromBody(proof)) {
			return true
		}
	}
	return len(proofs) > 0
}

func lifecycleObligationFromBody(proof body.LifecycleObligationProof) LifecycleObligation {
	return LifecycleObligation{
		Point:    proof.Point,
		Resource: proof.Resource,
		Protocol: proof.Protocol,
		Current:  proof.Current,
		Finals:   proof.Finals,
		Sites:    lifecycleSitesFromBody(proof.Sites),
	}
}

func lifecycleSitesFromBody(sites []body.LifecycleSite) []readapi.LifecycleSite {
	if len(sites) == 0 {
		return nil
	}
	out := make([]readapi.LifecycleSite, 0, len(sites))
	for _, site := range sites {
		kind, ok := lifecycleSiteKindFromBody(site.Kind)
		if !ok {
			continue
		}
		out = append(out, readapi.LifecycleSite{
			Point:       site.Point,
			Kind:        kind,
			Resource:    site.Resource,
			Protocol:    site.Protocol,
			From:        site.From,
			To:          site.To,
			TargetLabel: site.TargetLabel,
			Span:        sourceSpanFromBody(site.Span),
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func lifecycleSiteKindFromBody(kind body.LifecycleSiteKind) (readapi.LifecycleSiteKind, bool) {
	switch kind {
	case body.LifecycleSiteAcquire:
		return readapi.LifecycleSiteAcquire, true
	case body.LifecycleSiteTransition:
		return readapi.LifecycleSiteTransition, true
	case body.LifecycleSiteEscape:
		return readapi.LifecycleSiteEscape, true
	default:
		return 0, false
	}
}
