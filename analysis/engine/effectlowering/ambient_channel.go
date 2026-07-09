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
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
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

// ReceiverTypeFunc resolves a colon-call receiver type at the call boundary.
type ReceiverTypeFunc func(ctx transfer.NodeContext, site factflow.CallSiteView, in state.State, read func(cfg.Point) state.State) (typ.Type, bool)

type AmbientChannelSendOutcomeProviderConfig struct {
	ReceiverType ReceiverTypeFunc
	KeySpace     *keyspace.KeySpace
}

type AmbientChannelLifecycleOutcomeProviderConfig struct {
	ReceiverType ReceiverTypeFunc
	NameForSite  SignatureSiteNameFunc
	Signatures   SignatureLookup
	ArgumentType SignatureArgumentTypeFunc
	Sources      sourcevalue.SourceValues
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
// surface into typestate facts over the existing lifecycle axis. Unknown calls
// that receive a channel argument escape that local proof, so closed-state
// evidence is not reused after potential aliasing by opaque code.
func AmbientChannelLifecycleOutcomeProvider(config AmbientChannelLifecycleOutcomeProviderConfig) callpayload.CallOutcomeProvider {
	receiverType := config.ReceiverType
	nameForSite := config.NameForSite
	signatures := config.Signatures
	argumentType := config.ArgumentType
	sources := config.Sources
	args := signatureArgumentReader{keySpace: config.KeySpace}
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
				case "close":
					return channelLifecycleOutcome(callboundary.LifecycleFact{
						Target:   pathdom.NewPlaceholder(0),
						Kind:     callboundary.LifecycleTransition,
						Protocol: ChannelLifecycleProtocol,
						From:     ChannelStateOpen,
						To:       ChannelStateClosed,
					})
				case "receive":
					if slot, ok := channelReceiverLifecycleSlot(ctx, site, in, config.Resolver, config.KeySpace); channelSlotIsProvablyClosed(slot, ok) {
						return channelClosedReceiveOutcome(ctx, receiverPayload)
					}
					return callpayload.CallOutcome{}
				case "send", "case_receive", "case_send":
					return callpayload.CallOutcome{}
				default:
					return callpayload.CallOutcome{}
				}
			}
		}
		if channelCallHasKnownSignature(ctx, site, nameForSite, signatures) {
			return callpayload.CallOutcome{}
		}
		facts := channelOpaqueEscapeFacts(ctx, site, in, read, argumentType, sources, args, receiverIsChannel)
		if len(facts) == 0 {
			return callpayload.CallOutcome{}
		}
		return callpayload.CallOutcome{
			NormalReturnFacts: callboundary.NormalReturnFacts{LifecycleFacts: facts},
		}
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

func channelSlotIsProvablyClosed(slot typestate.Slot, ok bool) bool {
	if !ok || slot.Current != ChannelStateClosed {
		return false
	}
	return slot.Locality != typestate.LocalityUnknown &&
		slot.Locality != typestate.LocalityEscaped &&
		slot.Locality != typestate.LocalityBottom
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
	switch name {
	case "channel.select":
		return true
	default:
		return isChannelNewSignatureName(name)
	}
}

func channelCallHasKnownSignature(ctx transfer.NodeContext, site factflow.CallSiteView, nameForSite SignatureSiteNameFunc, signatures SignatureLookup) bool {
	if signatures == nil {
		return false
	}
	name, ok := channelSignatureName(ctx, site, nameForSite)
	if !ok || name == "" {
		return false
	}
	if isChannelKnownModuleSignatureName(name) {
		return true
	}
	_, ok = signatures.Lookup(name)
	return ok
}

func channelOpaqueEscapeFacts(
	ctx transfer.NodeContext,
	site factflow.CallSiteView,
	in state.State,
	read func(cfg.Point) state.State,
	argumentType SignatureArgumentTypeFunc,
	sources sourcevalue.SourceValues,
	args signatureArgumentReader,
	receiverIsChannel bool,
) []callboundary.LifecycleFact {
	var out []callboundary.LifecycleFact
	if receiverIsChannel {
		out = append(out, channelLifecycleEscapeFact(pathdom.NewPlaceholder(0)))
	}
	offset := 0
	if _, ok := site.ReceiverPath(); ok {
		offset = 1
	}
	site.ForEachArgumentSource(func(i int, source factflow.ValueSource) bool {
		if !args.callArgumentSourceCanBindPath(source) {
			return true
		}
		t, ok := channelArgumentSourceType(ctx, source, in, read, argumentType, sources)
		if !ok {
			return true
		}
		if _, ok := ambient.ChannelPayloadType(t); !ok {
			return true
		}
		out = append(out, channelLifecycleEscapeFact(pathdom.NewPlaceholder(i+offset)))
		return true
	})
	return out
}

func channelLifecycleEscapeFact(target pathdom.Path) callboundary.LifecycleFact {
	return callboundary.LifecycleFact{
		Target:   target,
		Kind:     callboundary.LifecycleEscape,
		Protocol: ChannelLifecycleProtocol,
	}
}

func channelArgumentSourceType(
	ctx transfer.NodeContext,
	source factflow.ValueSource,
	in state.State,
	read func(cfg.Point) state.State,
	argumentType SignatureArgumentTypeFunc,
	sources sourcevalue.SourceValues,
) (typ.Type, bool) {
	if argumentType != nil {
		if t, ok := argumentType(ctx, source, in, read); ok {
			return t, true
		}
	}
	if sources == nil {
		return nil, false
	}
	value, ok := sources.ValueOfSource(ctx.Point, source, in, read)
	if !ok {
		return nil, false
	}
	return typevalue.WitnessOf(ctx.Registry, value)
}
