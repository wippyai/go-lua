package effectlowering

import (
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/calloutcome"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
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
	Exports     ModuleExportLookup
	NameFor     SignatureNameFunc
	NameForSite SignatureSiteNameFunc
	TypeValues  *typevalue.Cache
}

// ModuleLoadOutcomeProvider materializes require("exact-path") slot zero from
// manifest export metadata. Only an exact literal path may rehydrate a manifest
// export. A literal miss returns untrusted any without post-return authority;
// dynamic paths still produce no result so the require signature can reject an
// unproven argument.
func ModuleLoadOutcomeProvider(config ModuleLoadOutcomeProviderConfig) callpayload.CallOutcomeProgram {
	exports := config.Exports
	nameFor := config.NameFor
	nameForSite := config.NameForSite
	typeValues := config.TypeValues
	shape := func(ctx transfer.NodeContext, site factflow.CallSiteView) (callpayload.CallOutcomeSiteShape, error) {
		name, ok := signatureNameForSite(ctx, site, nameForSite, nameFor)
		if !ok || name != "require" || site.ArgumentSourceCount() != 1 {
			return callpayload.CallOutcomeSiteShape{}, nil
		}
		return callpayload.CallOutcomeSiteShape{FieldNames: []string{"Results", "PostReturnAuthority"}}, nil
	}
	evaluate := func(ctx transfer.NodeContext, site factflow.CallSiteView, input callpayload.CallOutcomeInput) (callpayload.CallOutcome, error) {
		if exports == nil || (nameFor == nil && nameForSite == nil) {
			return callpayload.CallOutcome{}, nil
		}
		name, ok := signatureNameForSite(ctx, site, nameForSite, nameFor)
		if !ok || name != "require" {
			return callpayload.CallOutcome{}, nil
		}
		if site.ArgumentSourceCount() != 1 {
			return callpayload.CallOutcome{}, nil
		}
		value, ok := input.Argument(0)
		if !ok {
			return callpayload.CallOutcome{}, nil
		}
		path, ok := typevalue.StringLiteralOf(ctx.Registry, value)
		if !ok {
			return callpayload.CallOutcome{}, nil
		}
		exportType, exact := exports.LookupExport(path)
		if !exact || exportType == nil {
			return callpayload.CallOutcome{Results: []callpayload.CallResult{{
				Index: 0,
				Value: returnValueFromTypeCached(ctx.Registry, typeValues, typ.Any),
			}}}, nil
		}
		out := callpayload.CallOutcome{
			Results: []callpayload.CallResult{{
				Index: 0,
				Value: returnValueFromTypeCached(ctx.Registry, typeValues, exportType),
			}},
		}
		out.PostReturnAuthority = calloutcome.HasAuthoritativePostReturnEvidence(ctx.Registry, out)
		return out, nil
	}
	return callpayload.SealObservedCallOutcomeProgram(
		"module-load outcome", []string{"Results", "PostReturnAuthority"},
		state.LaneSet{}, state.LaneSet{}, callpayload.ObserveCallOutcomeOperands(false, false, 0), shape, nil, evaluate,
	)
}
