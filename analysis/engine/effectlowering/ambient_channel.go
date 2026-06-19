package effectlowering

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/type/ambient"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// ReceiverTypeFunc resolves a colon-call receiver type at the call boundary.
type ReceiverTypeFunc func(ctx transfer.NodeContext, site factflow.CallSiteView, in state.State, read func(cfg.Point) state.State) (typ.Type, bool)

type AmbientChannelSendOutcomeProviderConfig struct {
	ReceiverType ReceiverTypeFunc
}

// AmbientChannelSendOutcomeProvider lowers Channel<T>:send(payload) into the
// same send-escape fact used by manifest-backed process.send.
func AmbientChannelSendOutcomeProvider(config AmbientChannelSendOutcomeProviderConfig) callpayload.CallOutcomeProvider {
	receiverType := config.ReceiverType
	return func(ctx transfer.NodeContext, site factflow.CallSiteView, in state.State, read func(cfg.Point) state.State) callpayload.CallOutcome {
		if receiverType == nil || site.MethodName() != "send" {
			return callpayload.CallOutcome{}
		}
		t, ok := receiverType(ctx, site, in, read)
		if !ok {
			return callpayload.CallOutcome{}
		}
		if _, ok := ambient.ChannelPayloadType(t); !ok {
			return callpayload.CallOutcome{}
		}
		arg, ok := site.ArgumentSourceAt(0)
		if !ok || !callArgumentSourceCanBindPath(arg) {
			return callpayload.CallOutcome{}
		}
		target := 0
		if _, ok := site.ReceiverPath(); ok {
			target = 1
		}
		return callpayload.CallOutcome{
			NormalReturnFacts: callboundary.NormalReturnFacts{
				EscapeEvents: []callboundary.EscapeEventFact{{
					Target:    pathdom.NewPlaceholder(target),
					Kind:      callboundary.EscapeEventSend,
					Recursive: true,
				}},
			},
		}
	}
}
