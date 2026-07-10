package body

import (
	"github.com/wippyai/go-lua/analysis/engine/effectlowering"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// ChannelLifecycleOperation classifies a runtime channel operation that is
// invalid when the receiver is proven closed.
type ChannelLifecycleOperation string

const (
	ChannelLifecycleSend  ChannelLifecycleOperation = "send"
	ChannelLifecycleClose ChannelLifecycleOperation = "close"
)

// ChannelLifecycleMisuseProof records a channel operation whose receiver is
// provably closed at the call entry.
type ChannelLifecycleMisuseProof struct {
	Point     cfg.Point
	Operation ChannelLifecycleOperation
	Channel   string
	State     string
	Span      SourceSpan
}

// ChannelLifecycleMisuseProofs adapts generic invalid-transition facts for the
// ambient channel presentation. State inspection and invalidity proof live in
// TypestateInvalidTransitionProofs; this layer only preserves the historic
// channel diagnostic vocabulary.
func (r *Result) ChannelLifecycleMisuseProofs() []ChannelLifecycleMisuseProof {
	if r == nil {
		return nil
	}
	var out []ChannelLifecycleMisuseProof
	for _, proof := range r.TypestateInvalidTransitionProofs() {
		if proof.Protocol != string(effectlowering.ChannelLifecycleProtocol) {
			continue
		}
		site, ok := r.CallSiteView(proof.Point)
		if !ok {
			continue
		}
		operation, ok := channelLifecycleOperation(site.MethodName())
		if !ok {
			continue
		}
		out = append(out, ChannelLifecycleMisuseProof{
			Point:     proof.Point,
			Operation: operation,
			Channel:   proof.Target,
			State:     proof.Found,
			Span:      proof.Span,
		})
	}
	return out
}

func channelLifecycleOperation(method string) (ChannelLifecycleOperation, bool) {
	switch method {
	case "send":
		return ChannelLifecycleSend, true
	case "close":
		return ChannelLifecycleClose, true
	default:
		return "", false
	}
}
