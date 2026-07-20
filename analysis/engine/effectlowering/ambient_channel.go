package effectlowering

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typecall"
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

type AmbientChannelSendOutcomeProviderConfig struct {
	KeySpace *keyspace.KeySpace
}

type AmbientChannelLifecycleOutcomeProviderConfig struct {
	NameForSite SignatureSiteNameFunc
	KeySpace    *keyspace.KeySpace
	Resolver    *visibility.Resolver
	Domain      state.ProductDomain
}

// AmbientChannelSendOutcomeProvider lowers Channel<T>:send(payload) into the
// same send-escape fact used by manifest-backed process.send.
func AmbientChannelSendOutcomeProvider(config AmbientChannelSendOutcomeProviderConfig) callpayload.CallOutcomeProgram {
	args := signatureArgumentReader{keySpace: config.KeySpace}
	shape := func(_ transfer.NodeContext, site factflow.CallSiteView) (callpayload.CallOutcomeSiteShape, error) {
		if site.MethodName() != "send" {
			return callpayload.CallOutcomeSiteShape{}, nil
		}
		if _, present := site.ReceiverSource(); !present {
			return callpayload.CallOutcomeSiteShape{}, nil
		}
		return callpayload.CallOutcomeSiteShape{FieldNames: []string{"SuspensionKnown", "MaySuspend", "NormalReturnFacts"}}, nil
	}
	evaluate := func(ctx transfer.NodeContext, site factflow.CallSiteView, input callpayload.CallOutcomeInput) (callpayload.CallOutcome, error) {
		if site.MethodName() != "send" {
			return callpayload.CallOutcome{}, nil
		}
		receiver, ok := input.Receiver()
		if !ok {
			return callpayload.CallOutcome{}, nil
		}
		t, ok := receiverTypeFromValue(ctx.Registry, receiver)
		if !ok {
			return callpayload.CallOutcome{}, nil
		}
		if _, ok := typecall.AmbientChannelPayloadType(t); !ok {
			return callpayload.CallOutcome{}, nil
		}
		arg, ok := site.ArgumentSourceAt(0)
		if !ok || !args.callArgumentSourceCanBindPath(arg) {
			return callpayload.CallOutcome{}, nil
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
		}, nil
	}
	return callpayload.SealCallOutcomeProgram("ambient channel send outcome", []string{"SuspensionKnown", "MaySuspend", "NormalReturnFacts"}, state.LaneSet{}, state.LaneSet{}, shape, nil, evaluate)
}

// AmbientChannelLifecycleOutcomeProvider lowers the ambient channel runtime
// surface into declared typestate facts. Protocol-independent opaque-call
// escape is handled by AmbientTypestateEscapeOutcomeProvider.
func AmbientChannelLifecycleOutcomeProvider(config AmbientChannelLifecycleOutcomeProviderConfig) callpayload.CallOutcomeProgram {
	nameForSite := config.NameForSite
	typestateQuery, queryErr := config.Domain.SealTypestateQueryCapability(config.KeySpace)
	if queryErr != nil {
		panic(queryErr)
	}
	shape := func(ctx transfer.NodeContext, site factflow.CallSiteView) (callpayload.CallOutcomeSiteShape, error) {
		if name, ok := channelSignatureName(ctx, site, nameForSite); ok {
			if isChannelNewSignatureName(name) {
				return callpayload.CallOutcomeSiteShape{FieldNames: []string{"SuspensionKnown", "NormalReturnFacts"}}, nil
			}
			if isChannelKnownModuleSignatureName(name) {
				return callpayload.CallOutcomeSiteShape{}, nil
			}
		}
		switch site.MethodName() {
		case "send", "close":
			if _, present := site.ReceiverSource(); !present {
				return callpayload.CallOutcomeSiteShape{}, nil
			}
			return callpayload.CallOutcomeSiteShape{FieldNames: []string{"SuspensionKnown", "NormalReturnFacts"}}, nil
		case "receive":
			if _, present := site.ReceiverSource(); !present {
				return callpayload.CallOutcomeSiteShape{}, nil
			}
			var queries []state.TypestateResourceQuery
			if receiver, present := site.ReceiverPath(); present && !receiver.IsEmpty() {
				if receiverKey, resolved := channelReceiverTypestateStateKey(ctx.Point, config.Resolver, receiver); resolved {
					query, err := state.SealTypestateResourceQuery(config.Domain, typestateQuery, receiverKey, ChannelLifecycleProtocol)
					if err != nil {
						return callpayload.CallOutcomeSiteShape{}, err
					}
					queries = []state.TypestateResourceQuery{query}
				}
			}
			return callpayload.CallOutcomeSiteShape{
				FieldNames: []string{"Results", "PostReturnAuthority", "ReturnConditionSlots"}, TypestateResourceQueries: queries,
				Correlations: []callpayload.CallOutcomeCorrelationShape{
					callpayload.ReturnConditionSlotShape(1, false, 0), callpayload.ReturnConditionSlotShape(1, true, 0),
				},
			}, nil
		default:
			return callpayload.CallOutcomeSiteShape{}, nil
		}
	}
	evaluate := func(ctx transfer.NodeContext, site factflow.CallSiteView, input callpayload.CallOutcomeInput, query state.TypestateResourceQuery) (callpayload.CallOutcome, error) {
		if name, ok := channelSignatureName(ctx, site, nameForSite); ok {
			if isChannelNewSignatureName(name) {
				return channelLifecycleOutcome(callboundary.LifecycleFact{
					Target:   pathdom.Path{Root: "ret[0]"},
					Kind:     callboundary.LifecycleAcquire,
					Protocol: ChannelLifecycleProtocol,
					To:       ChannelStateOpen,
				}), nil
			}
			if isChannelKnownModuleSignatureName(name) {
				return callpayload.CallOutcome{}, nil
			}
		}
		receiverIsChannel := false
		var receiverPayload typ.Type
		if site.MethodName() != "" {
			if receiver, present := input.Receiver(); present {
				if t, ok := receiverTypeFromValue(ctx.Registry, receiver); ok {
					if payload, ok := typecall.AmbientChannelPayloadType(t); ok {
						receiverIsChannel = true
						receiverPayload = payload
					}
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
					}), nil
				case "close":
					return channelLifecycleOutcome(callboundary.LifecycleFact{
						Target:   pathdom.NewPlaceholder(0),
						Kind:     callboundary.LifecycleTransition,
						Protocol: ChannelLifecycleProtocol,
						From:     ChannelStateOpen,
						To:       ChannelStateClosed,
					}), nil
				case "receive":
					correlation := channelReceiveCorrelationOutcome(ctx)
					slot, ok := channelReceiverLifecycleSlot(input, query)
					if ok &&
						slot.Current == ChannelStateClosed &&
						slot.Locality != typestate.LocalityUnknown &&
						slot.Locality != typestate.LocalityEscaped &&
						slot.Locality != typestate.LocalityBottom {
						closed := channelClosedReceiveOutcome(ctx, receiverPayload)
						closed.ReturnConditionSlots = correlation.ReturnConditionSlots
						return closed, nil
					}
					return correlation, nil
				}
				return callpayload.CallOutcome{}, nil
			}
		}
		return callpayload.CallOutcome{}, nil
	}
	prepare := func(ctx transfer.NodeContext, site factflow.CallSiteView) (callpayload.CallOutcomeSitePreparation, error) {
		shaped, err := shape(ctx, site)
		if err != nil {
			return callpayload.CallOutcomeSitePreparation{}, err
		}
		var query state.TypestateResourceQuery
		if len(shaped.TypestateResourceQueries) != 0 {
			query = shaped.TypestateResourceQueries[0]
		}
		return callpayload.CallOutcomeSitePreparation{
			Shape: shaped,
			Evaluate: func(evalCtx transfer.NodeContext, input callpayload.CallOutcomeInput) (callpayload.CallOutcome, error) {
				return evaluate(evalCtx, site, input, query)
			},
		}, nil
	}
	return callpayload.SealPreparedCallOutcomeProgram(
		"ambient channel lifecycle outcome",
		[]string{"Results", "PostReturnAuthority", "SuspensionKnown", "NormalReturnFacts", "ReturnConditionSlots"},
		typestateQuery.Lanes(), state.LaneSet{}, prepare,
	)
}

// channelReceiveCorrelationOutcome is the ambient Channel<T> ABI relation:
// the boolean result is the discriminator for the payload slot. It is not a
// budgeted inference or a signature-name heuristic; receiver type authority
// has already proved the runtime channel operation above.
func channelReceiveCorrelationOutcome(ctx transfer.NodeContext) callpayload.CallOutcome {
	return callpayload.CallOutcome{ReturnConditionSlots: []callpayload.CallReturnConditionSlotRefinement{
		{
			ReturnIndex: 1, ReturnValue: true, TargetIndex: 0,
			Value: product.NewWithPresence(ctx.Registry, product.ShapeTop, presence.Present()),
		},
		{
			ReturnIndex: 1, ReturnValue: false, TargetIndex: 0,
			Value: product.NewWithPresence(ctx.Registry, product.ShapeTop, presence.Absent()),
		},
	}}
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

func channelReceiverLifecycleSlot(input callpayload.CallOutcomeInput, query state.TypestateResourceQuery) (typestate.Slot, bool) {
	observation, ok := input.Primary().ObserveTypestateResource(query)
	if !ok {
		return typestate.Slot{}, false
	}
	return observation.Slot()
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
