package effectlowering

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/calloutcome"
	"github.com/wippyai/go-lua/analysis/engine/callproducer"
	factapply "github.com/wippyai/go-lua/analysis/engine/factapply"
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
	Sources               sourcevalue.SourceValues
	ExpressionRefinements map[factflow.ExprRef]factflow.ExpressionRefinement
}

// ModuleLoadOutcomeProvider materializes require("exact-path") slot zero from
// manifest export metadata. Non-require calls, non-single-argument calls,
// dynamic paths, and missing manifests fail closed with no result.
func ModuleLoadOutcomeProvider(config ModuleLoadOutcomeProviderConfig) factapply.CallOutcomeProvider {
	exports := config.Exports
	nameFor := config.NameFor
	sources := config.Sources
	expressionRefinements := config.ExpressionRefinements
	return func(ctx transfer.NodeContext, site factflow.CallSiteView, in state.State, read func(cfg.Point) state.State) factapply.CallOutcome {
		if exports == nil || nameFor == nil || sources == nil {
			return factapply.CallOutcome{}
		}
		name, ok := nameFor(ctx, callproducer.FromView(site))
		if !ok || name != "require" {
			return factapply.CallOutcome{}
		}
		args := site.ArgumentSources()
		if len(args) != 1 {
			return factapply.CallOutcome{}
		}
		value, ok := sourcevalue.WithExpressionRefinements(ctx.Registry, sources, expressionRefinements).ValueOfSource(ctx.Point, args[0], in, read)
		if !ok {
			return factapply.CallOutcome{}
		}
		path, ok := exactStringLiteral(ctx.Registry, value)
		if !ok {
			return factapply.CallOutcome{}
		}
		exportType, ok := exports.LookupExport(path)
		if !ok || exportType == nil {
			return factapply.CallOutcome{}
		}
		out := factapply.CallOutcome{
			Results: []factapply.CallResult{{
				Index: 0,
				Value: returnValueFromType(ctx.Registry, exportType),
			}},
		}
		out.PostReturnAuthority = calloutcome.HasPostReturnEvidence(ctx.Registry, out)
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
