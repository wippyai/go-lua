// Package effectlowering lowers declared signature effects into factflow facts
// and factapply call outcomes.
package effectlowering

import (
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/engine/calloutcome"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/module/signature"
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

// SignatureLookup is the bounded read view required for signature-backed call
// results.
type SignatureLookup interface {
	Lookup(name string) (signature.Function, bool)
}

// SignatureOutcomeProviderConfig carries the signature/effect lookup plus the generic
// fact/source read models needed to resolve call argument values.
type SignatureOutcomeProviderConfig struct {
	Signatures    SignatureLookup
	NameFor       SignatureNameFunc
	NameForSite   SignatureSiteNameFunc
	ReturnTypeOps ReturnTypeOps
	Facts         factflow.Facts
	Sources       sourcevalue.SourceValues
	ArgumentType  SignatureArgumentTypeFunc
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
	facts := config.Facts
	sources := config.Sources
	argumentType := config.ArgumentType
	providerKeySpace := config.KeySpace
	expressionRefinements := facts.ExpressionRefinements()
	return func(ctx transfer.NodeContext, site factflow.CallSiteView, in state.State, read func(cfg.Point) state.State) callpayload.CallOutcome {
		if signatures == nil || (nameFor == nil && nameForSite == nil) {
			return callpayload.CallOutcome{}
		}
		name, ok := signatureNameForSite(ctx, site, nameForSite, nameFor)
		if !ok {
			return callpayload.CallOutcome{}
		}
		sig, ok := signatures.Lookup(name)
		if !ok {
			return callpayload.CallOutcome{}
		}
		argSources := signatureArgumentSources(ctx, facts, site)
		sig = instantiateSignatureForCall(ctx, sources, expressionRefinements, argumentType, sig, argSources, in, read, returnTypeOps)
		var out callpayload.CallOutcome
		if sig.OperationalEffects != nil && !sig.OperationalEffects.IsEmpty() {
			out = applyOperationalEffects(ctx, out, operationalEffectContext{
				effects:               sig.OperationalEffects,
				argSources:            argSources,
				sources:               sources,
				expressionRefinements: expressionRefinements,
				in:                    in,
				read:                  read,
				keySpace:              providerKeySpace,
			})
		} else {
			invalidations := signatureParamPathInvalidationsForReader(sig, argSources)
			lengthFloors := signatureParamLengthFloorsForReader(sig, argSources)
			out.ReturnPresenceRelations = signatureReturnPresenceRelations(sig)
			out.ParamPathRefinements = signatureParamPathRefinementsForReader(ctx, sig, argSources)
			out.ParamLengthFloors = lengthFloors
			out.ParamPathInvalidations = invalidations
			out.NormalReturnFacts.PathInvalidations = signatureNormalReturnPathInvalidations(invalidations)
			out.NormalReturnFacts.EscapeEvents = signatureEscapeEventsForReader(sig, argSources)
			out.NormalReturnFacts.FrozenTables = signatureFrozenTablesForReader(sig, argSources)
			out.NormalReturnFacts.StoreRelations = signatureStoreRelationsForReader(sig, argSources)
			out.NormalReturnFacts.LifecycleFacts = signatureLifecycleFactsForReader(sig, argSources)
		}
		if sig.Type == nil || len(sig.Type.Returns) == 0 {
			out.PostReturnAuthority = calloutcome.HasAuthoritativePostReturnEvidence(ctx.Registry, out)
			return out
		}
		results := make([]callpayload.CallResult, 0, len(sig.Type.Returns))
		for i, ret := range sig.Type.Returns {
			value, ok := signatureReturnValue(ctx, sources, expressionRefinements, sig, i, argSources, in, read, returnTypeOps)
			if !ok && ret != nil {
				value, ok = returnValueFromType(ctx.Registry, ret), true
			}
			if !ok {
				continue
			}
			value = operationalReturnAllocationValue(ctx.Registry, sig.OperationalEffects, i, value)
			results = append(results, callpayload.CallResult{
				Index: i,
				Value: value,
			})
		}
		out.Results = results
		out.PostReturnAuthority = calloutcome.HasAuthoritativePostReturnEvidence(ctx.Registry, out)
		return out
	}
}

func signatureNameForSite(
	ctx transfer.NodeContext,
	site factflow.CallSiteView,
	nameForSite SignatureSiteNameFunc,
	nameFor SignatureNameFunc,
) (string, bool) {
	if nameForSite != nil {
		return nameForSite(ctx, site)
	}
	if nameFor == nil {
		return "", false
	}
	return nameFor(ctx, factflow.NewCallProducerFromView(site))
}

func signatureArgumentSources(ctx transfer.NodeContext, facts factflow.Facts, site factflow.CallSiteView) signatureArgumentReader {
	if site.ArgumentSourceCount() != 0 {
		return signatureArgumentsFromView(site)
	}
	if factSite, ok := facts.CallSiteView(ctx.Point); ok {
		return signatureArgumentsFromView(factSite)
	}
	return signatureArgumentReader{}
}
