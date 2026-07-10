package readmodel

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	readapi "github.com/wippyai/go-lua/analysis/check/readmodel"
)

// ForEachChannelLifecycleMisuse visits runtime channel operations whose
// receiver is provably closed.
func (r Reader) ForEachChannelLifecycleMisuse(visit func(ChannelLifecycleMisuse) bool) bool {
	if r.result == nil || visit == nil {
		return false
	}
	proofs := r.result.ChannelLifecycleMisuseProofs()
	for _, proof := range proofs {
		if !visit(channelLifecycleMisuseFromBody(proof)) {
			return true
		}
	}
	return len(proofs) > 0
}

func channelLifecycleMisuseFromBody(proof body.ChannelLifecycleMisuseProof) ChannelLifecycleMisuse {
	return ChannelLifecycleMisuse{
		Point:     proof.Point,
		Span:      sourceSpanFromBody(proof.Span),
		Operation: readapi.ChannelLifecycleOperation(proof.Operation),
		Channel:   proof.Channel,
		State:     proof.State,
	}
}
