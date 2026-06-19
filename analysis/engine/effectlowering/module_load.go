package effectlowering

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/calloutcome"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// ModuleExportLookup is the bounded exact-path read view required for
// require/import rehydration.
type ModuleExportLookup interface {
	LookupExport(path string) (typ.Type, bool)
}

// ModuleLoadOutcomeProviderConfig carries module export lookup plus the generic
// fact/source read models needed to resolve require's module path argument.
type ModuleLoadOutcomeProviderConfig struct {
	Exports               ModuleExportLookup
	NameFor               SignatureNameFunc
	NameForSite           SignatureSiteNameFunc
	Sources               sourcevalue.SourceValues
	ExpressionRefinements map[factflow.ExprRef]factflow.ExpressionRefinement
}

// ModuleLoadOutcomeProvider materializes require("exact-path") slot zero from
// manifest export metadata. Non-require calls, non-single-argument calls,
// dynamic paths, and missing manifests fail closed with no result.
func ModuleLoadOutcomeProvider(config ModuleLoadOutcomeProviderConfig) callpayload.CallOutcomeProvider {
	exports := config.Exports
	nameFor := config.NameFor
	nameForSite := config.NameForSite
	sources := config.Sources
	expressionRefinements := config.ExpressionRefinements
	return func(ctx transfer.NodeContext, site factflow.CallSiteView, in state.State, read func(cfg.Point) state.State) callpayload.CallOutcome {
		if exports == nil || (nameFor == nil && nameForSite == nil) || sources == nil {
			return callpayload.CallOutcome{}
		}
		name, ok := signatureNameForSite(ctx, site, nameForSite, nameFor)
		if !ok || name != "require" {
			return callpayload.CallOutcome{}
		}
		if site.ArgumentSourceCount() != 1 {
			return callpayload.CallOutcome{}
		}
		arg, ok := site.ArgumentSourceAt(0)
		if !ok {
			return callpayload.CallOutcome{}
		}
		value, ok := sourcevalue.WithExpressionRefinements(ctx.Registry, sources, expressionRefinements).ValueOfSource(ctx.Point, arg, in, read)
		if !ok {
			return callpayload.CallOutcome{}
		}
		path, ok := exactStringLiteral(ctx.Registry, value)
		if !ok {
			return callpayload.CallOutcome{}
		}
		exportType, ok := exports.LookupExport(path)
		if !ok || exportType == nil {
			return callpayload.CallOutcome{}
		}
		out := callpayload.CallOutcome{
			Results: []callpayload.CallResult{{
				Index: 0,
				Value: returnValueFromType(ctx.Registry, exportType),
			}},
		}
		out.PostReturnAuthority = calloutcome.HasAuthoritativePostReturnEvidence(ctx.Registry, out)
		return out
	}
}

func exactStringLiteral(reg *axis.Registry, value product.Value) (string, bool) {
	if reg == nil {
		return "", false
	}
	witness := product.Get(reg, value, typewitness.Key)
	t, ok := witness.Type()
	if !ok {
		return "", false
	}
	return stringLiteralFromWitness(t)
}

func stringLiteralFromWitness(t typ.Type) (string, bool) {
	lit, ok := t.(*typ.Literal)
	if !ok || lit.Base != kind.String {
		return "", false
	}
	value, ok := lit.Value.(string)
	return value, ok
}
