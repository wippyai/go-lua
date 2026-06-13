// Package effectlowering lowers declared signature effects into factflow facts
// and factapply call outcomes.
package effectlowering

import (
	"github.com/wippyai/go-lua/analysis/domain/effect/signature"
	factapply "github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// SignatureNameFunc maps one call producer in context to a stable signature name.
type SignatureNameFunc func(ctx transfer.NodeContext, call factflow.CallProducer) (string, bool)

// SignatureLookup is the bounded read view required for signature-backed call
// results.
type SignatureLookup interface {
	Lookup(name string) (signature.Function, bool)
}

// SignatureOutcomeProviderConfig carries the signature/effect lookup plus the generic
// fact/source read models needed to resolve call argument values.
type SignatureOutcomeProviderConfig struct {
	Signatures SignatureLookup
	NameFor    SignatureNameFunc
	Facts      factflow.Facts
	Sources    sourcevalue.SourceValues
}

// SignatureOutcomeProvider materializes declared signature return types into
// call outcome return slots.
func SignatureOutcomeProvider(config SignatureOutcomeProviderConfig) factapply.CallOutcomeProvider {
	signatures := config.Signatures
	nameFor := config.NameFor
	facts := config.Facts
	sources := config.Sources
	return func(ctx transfer.NodeContext, site factflow.CallSite, in state.State, read func(cfg.Point) state.State) factapply.CallOutcome {
		if signatures == nil || nameFor == nil {
			return factapply.CallOutcome{}
		}
		call := factflow.CallProducerFromSite(site)
		name, ok := nameFor(ctx, call)
		if !ok {
			return factapply.CallOutcome{}
		}
		sig, ok := signatures.Lookup(name)
		if !ok {
			return factapply.CallOutcome{}
		}
		out := factapply.CallOutcome{
			ReturnPresenceRelations: signatureReturnPresenceRelations(sig),
			ParamPathRefinements:    signatureParamPathRefinements(ctx, sig, site),
			ParamPathInvalidations:  signatureParamPathInvalidations(sig, site),
		}
		if sig.Type == nil || len(sig.Type.Returns) == 0 {
			return out
		}
		results := make([]factapply.CallResult, 0, len(sig.Type.Returns))
		for i, ret := range sig.Type.Returns {
			value, ok := signatureReturnValue(ctx, facts, sources, sig, i, in, read)
			if !ok && ret != nil {
				value, ok = returnValueFromType(ctx.Registry, ret), true
			}
			if !ok {
				continue
			}
			results = append(results, factapply.CallResult{
				Index: i,
				Value: value,
			})
		}
		out.Results = results
		return out
	}
}
