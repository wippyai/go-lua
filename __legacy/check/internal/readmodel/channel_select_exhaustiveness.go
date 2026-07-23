package readmodel

import "github.com/wippyai/go-lua/analysis/check/body"

// ForEachChannelSelectExhaustiveness visits channel.select elseif chains that
// do not handle every selectable case and do not have a select default.
func (r Reader) ForEachChannelSelectExhaustiveness(visit func(ChannelSelectExhaustiveness) bool) bool {
	if r.result == nil || visit == nil {
		return false
	}
	proofs := r.result.ChannelSelectExhaustivenessProofs()
	for _, proof := range proofs {
		if !visit(channelSelectExhaustivenessFromBody(proof)) {
			return true
		}
	}
	return len(proofs) > 0
}

func channelSelectExhaustivenessFromBody(proof body.ChannelSelectExhaustivenessProof) ChannelSelectExhaustiveness {
	return ChannelSelectExhaustiveness{
		Point:         proof.Point,
		Span:          sourceSpanFromBody(proof.Span),
		ResultChannel: proof.ResultChannel,
		Handled:       proof.Handled,
		Missing:       proof.Missing,
		HasDefault:    proof.HasDefault,
	}
}
