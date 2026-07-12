// Package effectlowering lowers declared signature effects into factflow facts
// and factapply call outcomes.
package effectlowering

import (
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/calloutcome"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/module/signature"
	"github.com/wippyai/go-lua/analysis/type/subst"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// SignatureNameFunc maps one call producer in context to a stable signature name.
type SignatureNameFunc func(ctx transfer.NodeContext, call factflow.CallProducer) (string, bool)

// SignatureSiteNameFunc maps read-only call-site evidence in context to a
// stable signature name without materializing a producer DTO.
type SignatureSiteNameFunc func(ctx transfer.NodeContext, site factflow.CallSiteView) (string, bool)

// SignatureArgumentTypeFunc resolves a call argument source to a type when the
// caller owns stronger evidence than the generic source-value projection.
type SignatureArgumentTypeFunc func(ctx transfer.NodeContext, source factflow.ValueSource, in state.State, read func(cfg.Point) state.State) (typ.Type, bool)

// SignatureReturnValueFunc optionally materializes argument-sensitive return
// evidence for a named signature before generic return transforms and declared
// return types are used.
type SignatureReturnValueFunc func(SignatureReturnContext) (product.Value, bool)

// SignatureReturnContext is the read-only call-site context passed to a
// signature return override.
type SignatureReturnContext struct {
	Node  transfer.NodeContext
	Site  factflow.CallSiteView
	Name  string
	Index int
	In    state.State
	Read  func(cfg.Point) state.State
}

// SignatureLookup is the bounded read view required for signature-backed call
// results.
type SignatureLookup interface {
	Lookup(name string) (signature.Function, bool)
}

// immutableSignatureLookup is implemented by bounded sources that can lend a
// read-only signature. SignatureOutcomeProvider never mutates a looked-up
// signature: receiver binding and generic substitution replace carrier fields
// with newly constructed values. Borrowing therefore removes deep ownership
// clones from fixed-point transfers while preserving SignatureLookup.Lookup's
// ownership contract for all other users.
type immutableSignatureLookup interface {
	LookupView(name string) (signature.Function, bool)
}

// SignatureOutcomeProviderConfig carries the signature/effect lookup plus the generic
// fact/source read models needed to resolve call argument values.
type SignatureOutcomeProviderConfig struct {
	Signatures    SignatureLookup
	NameFor       SignatureNameFunc
	NameForSite   SignatureSiteNameFunc
	ReturnTypeOps ReturnTypeOps
	TypeValues    *typevalue.Cache
	Facts         factflow.Facts
	Sources       sourcevalue.SourceValues
	ArgumentType  SignatureArgumentTypeFunc
	ReturnValue   SignatureReturnValueFunc
	// KeySpace is the consuming (caller) analysis keyspace into which rehydrated
	// heap allocation templates intern their rootless static-member keys.
	KeySpace *keyspace.KeySpace
}

// SignatureOutcomeProvider materializes declared signature return types into
// call outcome return slots.
func SignatureOutcomeProvider(config SignatureOutcomeProviderConfig) callpayload.CallOutcomeProvider {
	signatures := config.Signatures
	nameFor := config.NameFor
	nameForSite := config.NameForSite
	returnTypeOps := config.ReturnTypeOps
	typeValues := config.TypeValues
	facts := config.Facts
	sources := config.Sources
	argumentType := config.ArgumentType
	returnValue := config.ReturnValue
	providerKeySpace := config.KeySpace
	expressionRefinements := sourcevalue.NewExpressionRefinementsFromReader(facts)
	lookupSignature := signatures.Lookup
	if views, ok := signatures.(immutableSignatureLookup); ok {
		lookupSignature = views.LookupView
	}
	return func(ctx transfer.NodeContext, site factflow.CallSiteView, in state.State, read func(cfg.Point) state.State) callpayload.CallOutcome {
		if signatures == nil {
			return callpayload.CallOutcome{}
		}
		refinedSources := expressionRefinements.Bind(ctx.Registry, sources)
		var receiverType typ.Type
		name, ok := signatureNameForSite(ctx, site, nameForSite, nameFor)
		if ok && site.MethodName() != "" {
			receiverType, _ = receiverTypeForMethodSite(ctx, site, in, refinedSources, read)
		}
		if !ok {
			name, receiverType, ok = receiverMethodSignatureName(ctx, site, in, refinedSources, read)
			if !ok {
				return callpayload.CallOutcome{}
			}
		}
		sig, ok := lookupSignature(name)
		if !ok {
			return callpayload.CallOutcome{}
		}
		sig = bindReceiverSelfSignature(sig, receiverType)
		argSources := signatureArgumentSources(ctx, facts, site, providerKeySpace)
		if site.MethodName() != "" && signatureConsumesReceiver(sig.Type, receiverType) {
			argSources = signatureMethodArgumentSources(ctx, facts, site, providerKeySpace)
		}
		sig = instantiateSignatureForCall(ctx, sources, expressionRefinements, argumentType, sig, argSources, in, read, returnTypeOps)
		out := callpayload.CallOutcome{}
		if sig.OperationalEffects != nil {
			out.SuspensionKnown = sig.OperationalEffects.SuspensionKnown
			out.MaySuspend = sig.OperationalEffects.MaySuspend
		}
		if sig.OperationalEffects != nil && !sig.OperationalEffects.IsEmpty() && !operationalEffectsOnlyMaySuspend(sig.OperationalEffects) {
			out = applyOperationalEffects(ctx, out, operationalEffectContext{
				effects:               sig.OperationalEffects,
				signatureType:         sig.Type,
				argSources:            argSources,
				sources:               sources,
				expressionRefinements: expressionRefinements,
				in:                    in,
				read:                  read,
				keySpace:              providerKeySpace,
				typeValues:            typeValues,
			})
		} else {
			invalidations := signatureParamPathInvalidationsForReader(sig, argSources)
			lengthFloors := signatureParamLengthFloorsForReader(sig, argSources)
			mutations := signatureParamMutationEffectsForReader(ctx, sig, argSources, refinedSources, argumentType, in, read, typeValues)
			out.ReturnPresenceRelations = signatureReturnPresenceRelations(sig)
			out.ParamObligations = mutations.Obligations
			out.ParamPathRefinements = signatureParamPathRefinementsForReader(ctx, sig, argSources)
			out.ParamPathWrites = mutations.Writes
			out.ParamLengthFloors = lengthFloors
			out.ParamPathInvalidations = invalidations
			applySignatureNormalReturnFacts(sig, argSources, invalidations, mutations, &out.NormalReturnFacts)
		}
		if sig.Type == nil || len(sig.Type.Returns) == 0 {
			out.PostReturnAuthority = calloutcome.HasAuthoritativePostReturnEvidence(ctx.Registry, out)
			return out
		}
		results := make([]callpayload.CallResult, 0, len(sig.Type.Returns))
		for i, ret := range sig.Type.Returns {
			value, ok := product.Value{}, false
			if returnValue != nil {
				value, ok = returnValue(SignatureReturnContext{
					Node:  ctx,
					Site:  site,
					Name:  name,
					Index: i,
					In:    in,
					Read:  read,
				})
			}
			if !ok {
				value, ok = operationalReturnFlowValue(ctx, facts, sources, expressionRefinements, providerKeySpace, sig, i, argSources, in, read, typeValues)
			}
			if !ok {
				value, ok = signatureReturnValue(ctx, sources, expressionRefinements, sig, i, argSources, in, read, returnTypeOps, typeValues)
			}
			if !ok && ret != nil {
				value, ok = returnValueFromSignatureTypeCached(ctx.Registry, typeValues, sig.Type, ret), true
			}
			if !ok {
				continue
			}
			value = operationalReturnAllocationValue(ctx.Registry, typeValues, sig.OperationalEffects, sig.Type, ctx.Point, i, value)
			results = append(results, callpayload.CallResult{
				Index: i,
				Value: value,
			})
		}
		if returnValue != nil {
			present := make(map[int]struct{}, len(results))
			for _, result := range results {
				present[result.Index] = struct{}{}
			}
			site.ForEachResultTarget(func(target factflow.CallResultTargetView) bool {
				index := target.ResultIndex()
				if index < len(sig.Type.Returns) {
					return true
				}
				if _, ok := present[index]; ok {
					return true
				}
				value, ok := returnValue(SignatureReturnContext{
					Node:  ctx,
					Site:  site,
					Name:  name,
					Index: index,
					In:    in,
					Read:  read,
				})
				if !ok {
					return true
				}
				present[index] = struct{}{}
				results = append(results, callpayload.CallResult{
					Index: index,
					Value: value,
				})
				return true
			})
		}
		out.Results = results
		out.PostReturnAuthority = calloutcome.HasAuthoritativePostReturnEvidence(ctx.Registry, out)
		return out
	}
}

func operationalEffectsOnlyMaySuspend(effects *signature.OperationalEffects) bool {
	if effects == nil || !effects.MaySuspend {
		return false
	}
	withoutSuspension := effects.Clone()
	withoutSuspension.MaySuspend = false
	return withoutSuspension.IsEmpty()
}

type signatureNormalReturnLaneContext struct {
	sig           signature.Function
	args          signatureArgumentReader
	invalidations []callpayload.CallParamPathInvalidation
	mutations     signatureParamMutationEffects
}

type signatureNormalReturnLaneHandler func(signatureNormalReturnLaneContext, *callboundary.NormalReturnFacts)

var signatureNormalReturnLanes = callboundary.BindNormalReturnFactLanes(
	"signature normal-return",
	map[callboundary.NormalReturnFactLaneID]signatureNormalReturnLaneHandler{
		callboundary.LanePathRefinements:          signatureNormalReturnNoop,
		callboundary.LanePersistentPathWrites:     signatureNormalReturnNoop,
		callboundary.LanePathStaticMembers:        signatureNormalReturnNoop,
		callboundary.LanePathStaticMemberDeltas:   signatureNormalReturnNoop,
		callboundary.LanePathPresenceImplications: signatureNormalReturnNoop,
		callboundary.LanePathInvalidations:        signatureNormalReturnPathInvalidationLane,
		callboundary.LaneDynamicIndexFacts:        signatureNormalReturnDynamicIndexLane,
		callboundary.LaneKeyMemberships:           signatureNormalReturnNoop,
		callboundary.LaneDynamicValueKeys:         signatureNormalReturnNoop,
		callboundary.LaneDynamicAllValues:         signatureNormalReturnNoop,
		callboundary.LaneBranchProofs:             signatureNormalReturnNoop,
		callboundary.LaneChannelSelects:           signatureNormalReturnNoop,
		callboundary.LaneFrozenTables:             signatureNormalReturnFrozenTableLane,
		callboundary.LaneEffectDeltas:             signatureNormalReturnNoop,
		callboundary.LaneEscapeEvents:             signatureNormalReturnEscapeEventLane,
		callboundary.LaneStoreRelations:           signatureNormalReturnStoreRelationLane,
		callboundary.LaneLifecycleFacts:           signatureNormalReturnLifecycleLane,
		callboundary.LaneNumFloors:                signatureNormalReturnNoop,
		callboundary.LaneRelConstraints:           signatureNormalReturnNoop,
	},
	func(handler signatureNormalReturnLaneHandler) bool { return handler != nil },
)

func signatureNormalReturnNoop(signatureNormalReturnLaneContext, *callboundary.NormalReturnFacts) {
}

func signatureNormalReturnPathInvalidationLane(ctx signatureNormalReturnLaneContext, out *callboundary.NormalReturnFacts) {
	out.PathInvalidations = signatureNormalReturnPathInvalidations(ctx.invalidations)
}

func signatureNormalReturnDynamicIndexLane(ctx signatureNormalReturnLaneContext, out *callboundary.NormalReturnFacts) {
	out.DynamicIndexFacts = ctx.mutations.DynamicIndexFacts
}

func signatureNormalReturnFrozenTableLane(ctx signatureNormalReturnLaneContext, out *callboundary.NormalReturnFacts) {
	out.FrozenTables = signatureFrozenTablesForReader(ctx.sig, ctx.args)
}

func signatureNormalReturnEscapeEventLane(ctx signatureNormalReturnLaneContext, out *callboundary.NormalReturnFacts) {
	out.EscapeEvents = signatureEscapeEventsForReader(ctx.sig, ctx.args)
}

func signatureNormalReturnStoreRelationLane(ctx signatureNormalReturnLaneContext, out *callboundary.NormalReturnFacts) {
	out.StoreRelations = signatureStoreRelationsForReader(ctx.sig, ctx.args)
}

func signatureNormalReturnLifecycleLane(ctx signatureNormalReturnLaneContext, out *callboundary.NormalReturnFacts) {
	out.LifecycleFacts = signatureLifecycleFactsForReader(ctx.sig, ctx.args)
}

func applySignatureNormalReturnFacts(
	sig signature.Function,
	args signatureArgumentReader,
	invalidations []callpayload.CallParamPathInvalidation,
	mutations signatureParamMutationEffects,
	out *callboundary.NormalReturnFacts,
) {
	ctx := signatureNormalReturnLaneContext{
		sig:           sig,
		args:          args,
		invalidations: invalidations,
		mutations:     mutations,
	}
	for _, lane := range signatureNormalReturnLanes {
		lane.Value(ctx, out)
	}
}

func signatureNameForSite(
	ctx transfer.NodeContext,
	site factflow.CallSiteView,
	nameForSite SignatureSiteNameFunc,
	nameFor SignatureNameFunc,
) (string, bool) {
	if nameForSite != nil {
		if name, ok := nameForSite(ctx, site); ok {
			return name, true
		}
	}
	if nameFor == nil {
		return "", false
	}
	return nameFor(ctx, factflow.NewCallProducerFromView(site))
}

func bindReceiverSelfSignature(sig signature.Function, receiverType typ.Type) signature.Function {
	if sig.Type == nil || receiverType == nil {
		return sig
	}
	if bound, ok := subst.Self(sig.Type, receiverType).(*typ.Function); ok && bound != nil {
		sig.Type = bound
	}
	return sig
}

func signatureConsumesReceiver(fn *typ.Function, receiverType typ.Type) bool {
	if fn == nil || len(fn.Params) == 0 {
		return false
	}
	first := fn.Params[0]
	if first.Receiver {
		return true
	}
	if first.Type == nil || receiverType == nil || typ.IsAny(first.Type) || typ.IsUnknown(first.Type) {
		return false
	}
	return subtype.IsSubtype(receiverType, first.Type)
}

func receiverMethodSignatureName(ctx transfer.NodeContext, site factflow.CallSiteView, in state.State, sources sourcevalue.SourceValues, read func(cfg.Point) state.State) (string, typ.Type, bool) {
	method := site.MethodName()
	if method == "" || ctx.Registry == nil {
		return "", nil, false
	}
	receiverType, ok := receiverTypeForMethodSite(ctx, site, in, sources, read)
	if !ok {
		return "", nil, false
	}
	if subtype.IsSubtype(receiverType, typ.String) {
		return "string." + method, receiverType, true
	}
	iface, ok := receiverSignatureInterface(receiverType)
	if !ok {
		return "", nil, false
	}
	return iface.Name + "." + method, receiverType, true
}

func receiverTypeForMethodSite(ctx transfer.NodeContext, site factflow.CallSiteView, in state.State, sources sourcevalue.SourceValues, read func(cfg.Point) state.State) (typ.Type, bool) {
	if ctx.Registry == nil {
		return nil, false
	}
	receiverPath, ok := site.ReceiverPath()
	if ok && receiverPath.Symbol != 0 && len(receiverPath.Segments) == 0 {
		if receiverType, ok := receiverTypeFromValue(ctx.Registry, in.ReadSymbolValue(ctx.Registry, receiverPath.Symbol)); ok {
			return receiverType, true
		}
	}

	if sources == nil {
		return nil, false
	}
	receiverSource, ok := site.ReceiverSource()
	if !ok {
		return nil, false
	}
	value, ok := sources.ValueOfSource(ctx.Point, receiverSource, in, read)
	if !ok {
		return nil, false
	}
	return receiverTypeFromValue(ctx.Registry, value)
}

func receiverTypeFromValue(reg *axis.Registry, value product.Value) (typ.Type, bool) {
	receiverType, ok := typevalue.TypeOf(reg, value)
	if !ok || receiverType == nil || typ.IsAny(receiverType) || typ.IsUnknown(receiverType) || typ.IsNever(receiverType) {
		return nil, false
	}
	return receiverType, true
}

func receiverSignatureInterface(receiverType typ.Type) (*typ.Interface, bool) {
	iface, ok := typ.UnwrapTransparentWrappers(receiverType).(*typ.Interface)
	if !ok || iface == nil || iface.Name == "" {
		return nil, false
	}
	return iface, true
}

func signatureArgumentSources(ctx transfer.NodeContext, facts factflow.Facts, site factflow.CallSiteView, ks *keyspace.KeySpace) signatureArgumentReader {
	if site.ArgumentSourceCount() != 0 {
		return signatureArgumentsFromViewWithKeySpace(site, ks)
	}
	if factSite, ok := facts.CallSiteView(ctx.Point); ok {
		return signatureArgumentsFromViewWithKeySpace(factSite, ks)
	}
	return signatureArgumentReader{keySpace: ks}
}

func signatureMethodArgumentSources(ctx transfer.NodeContext, facts factflow.Facts, site factflow.CallSiteView, ks *keyspace.KeySpace) signatureArgumentReader {
	if site.ArgumentSourceCount() != 0 {
		return signatureArgumentsFromMethodViewWithKeySpace(site, ks)
	}
	if factSite, ok := facts.CallSiteView(ctx.Point); ok {
		return signatureArgumentsFromMethodViewWithKeySpace(factSite, ks)
	}
	return signatureArgumentReader{keySpace: ks}
}
