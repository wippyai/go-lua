package effectlowering

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/type/ambient"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
)

const (
	// ChannelLifecycleProtocol is the typestate protocol used for runtime
	// channel lifecycle facts.
	ChannelLifecycleProtocol typestate.Protocol = "channel.lifecycle"

	// ChannelStateOpen and ChannelStateClosed are the runtime channel states
	// modeled by the checker. Channels carry no exit obligation because the
	// scheduler owns their lifetime.
	ChannelStateOpen   typestate.State = "open"
	ChannelStateClosed typestate.State = "closed"
)

// ChannelLifecycleDefinition is the ambient runtime's declared protocol. The
// lifecycle machinery consumes it exactly like a manifest-declared FSM: send
// is an open-state-preserving operation and close finalizes an open channel.
var ChannelLifecycleDefinition = typestate.Definition{
	Protocol:    ChannelLifecycleProtocol,
	States:      []typestate.State{ChannelStateOpen, ChannelStateClosed},
	FinalStates: []typestate.State{ChannelStateClosed},
	Transitions: []typestate.TransitionDecl{
		{From: ChannelStateOpen, To: ChannelStateOpen},
		{From: ChannelStateOpen, To: ChannelStateClosed},
	},
}

// ReceiverTypeFunc resolves a colon-call receiver type at the call boundary.
type ReceiverTypeFunc func(ctx transfer.NodeContext, site factflow.CallSiteView, in state.State, read func(cfg.Point) state.State) (typ.Type, bool)

type AmbientChannelSendOutcomeProviderConfig struct {
	ReceiverType ReceiverTypeFunc
	KeySpace     *keyspace.KeySpace
}

type AmbientChannelLifecycleOutcomeProviderConfig struct {
	ReceiverType ReceiverTypeFunc
	NameForSite  SignatureSiteNameFunc
	KeySpace     *keyspace.KeySpace
	Resolver     *visibility.Resolver
}

// AmbientChannelSendOutcomeProvider lowers Channel<T>:send(payload) into the
// same send-escape fact used by manifest-backed process.send.
func AmbientChannelSendOutcomeProvider(config AmbientChannelSendOutcomeProviderConfig) callpayload.CallOutcomeProvider {
	receiverType := config.ReceiverType
	args := signatureArgumentReader{keySpace: config.KeySpace}
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
		if !ok || !args.callArgumentSourceCanBindPath(arg) {
			return callpayload.CallOutcome{}
		}
		target := 0
		if _, ok := site.ReceiverPath(); ok {
			target = 1
		}
		return callpayload.CallOutcome{
			SuspensionKnown: true,
			MaySuspend:      true,
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

// AmbientChannelLifecycleOutcomeProvider lowers the ambient channel runtime
// surface into declared typestate facts. Protocol-independent opaque-call
// escape is handled by AmbientTypestateEscapeOutcomeProvider.
func AmbientChannelLifecycleOutcomeProvider(config AmbientChannelLifecycleOutcomeProviderConfig) callpayload.CallOutcomeProvider {
	receiverType := config.ReceiverType
	nameForSite := config.NameForSite
	return func(ctx transfer.NodeContext, site factflow.CallSiteView, in state.State, read func(cfg.Point) state.State) callpayload.CallOutcome {
		if name, ok := channelSignatureName(ctx, site, nameForSite); ok {
			if isChannelNewSignatureName(name) {
				return channelLifecycleOutcome(callboundary.LifecycleFact{
					Target:   pathdom.Path{Root: "ret[0]"},
					Kind:     callboundary.LifecycleAcquire,
					Protocol: ChannelLifecycleProtocol,
					To:       ChannelStateOpen,
				})
			}
			if isChannelKnownModuleSignatureName(name) {
				return callpayload.CallOutcome{}
			}
		}
		receiverIsChannel := false
		var receiverPayload typ.Type
		if receiverType != nil && site.MethodName() != "" {
			if t, ok := receiverType(ctx, site, in, read); ok {
				if payload, ok := ambient.ChannelPayloadType(t); ok {
					receiverIsChannel = true
					receiverPayload = payload
				}
			}
			if receiverIsChannel {
				switch site.MethodName() {
				case "send":
					return channelLifecycleOutcome(callboundary.LifecycleFact{
						Target:   pathdom.NewPlaceholder(0),
						Kind:     callboundary.LifecycleTransition,
						Protocol: ChannelLifecycleProtocol,
						From:     ChannelStateOpen,
						To:       ChannelStateOpen,
					})
				case "close":
					return channelLifecycleOutcome(callboundary.LifecycleFact{
						Target:   pathdom.NewPlaceholder(0),
						Kind:     callboundary.LifecycleTransition,
						Protocol: ChannelLifecycleProtocol,
						From:     ChannelStateOpen,
						To:       ChannelStateClosed,
					})
				case "receive":
					if slot, ok := channelReceiverLifecycleSlot(ctx, site, in, config.Resolver, config.KeySpace); ok &&
						slot.Current == ChannelStateClosed &&
						slot.Locality != typestate.LocalityUnknown &&
						slot.Locality != typestate.LocalityEscaped &&
						slot.Locality != typestate.LocalityBottom {
						return channelClosedReceiveOutcome(ctx, receiverPayload)
					}
				}
				return callpayload.CallOutcome{}
			}
		}
		return callpayload.CallOutcome{}
	}
}

func channelClosedReceiveOutcome(ctx transfer.NodeContext, payload typ.Type) callpayload.CallOutcome {
	if payload == nil {
		payload = typ.Unknown
	}
	payloadResult := typeexpr.Optional(payload)
	return callpayload.CallOutcome{
		Results: []callpayload.CallResult{
			{Index: 0, Value: typevalue.WithWitness(ctx.Registry, typevalue.FromType(ctx.Registry, payloadResult), payloadResult)},
			{Index: 1, Value: typevalue.WithWitness(ctx.Registry, typevalue.FromType(ctx.Registry, typ.Boolean), typ.Boolean)},
		},
		PostReturnAuthority: true,
	}
}

func channelReceiverLifecycleSlot(
	ctx transfer.NodeContext,
	site factflow.CallSiteView,
	in state.State,
	resolver *visibility.Resolver,
	ks *keyspace.KeySpace,
) (typestate.Slot, bool) {
	receiver, ok := site.ReceiverPath()
	if !ok || receiver.IsEmpty() {
		return typestate.Slot{}, false
	}
	key, ok := channelReceiverTypestateStateKey(ctx.Point, resolver, receiver)
	if !ok {
		return typestate.Slot{}, false
	}
	resource := in.CanonicalTypestateResource(ks, key, ChannelLifecycleProtocol)
	return in.TypestateSlot(resource)
}

func channelReceiverTypestateStateKey(point cfg.Point, resolver *visibility.Resolver, receiver pathdom.Path) (pathaddr.StateKey, bool) {
	address := visibility.AddressAt(resolver, point, receiver)
	if receiver.Root == "" && receiver.Symbol != 0 && len(receiver.Segments) == 0 {
		return address.RootOrVisibleStateKey()
	}
	return address.VisibleStateKey()
}

func channelLifecycleOutcome(fact callboundary.LifecycleFact) callpayload.CallOutcome {
	return callpayload.CallOutcome{
		SuspensionKnown: true,
		NormalReturnFacts: callboundary.NormalReturnFacts{
			LifecycleFacts: []callboundary.LifecycleFact{fact},
		},
	}
}

func channelSignatureName(ctx transfer.NodeContext, site factflow.CallSiteView, nameForSite SignatureSiteNameFunc) (string, bool) {
	if nameForSite == nil {
		return "", false
	}
	return nameForSite(ctx, site)
}

func isChannelNewSignatureName(name string) bool {
	return name == "channel.new"
}

func isChannelKnownModuleSignatureName(name string) bool {
	return name == "channel.select" || isChannelNewSignatureName(name)
}
