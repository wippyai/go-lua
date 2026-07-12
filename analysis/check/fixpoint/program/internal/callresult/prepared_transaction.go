package callresult

import (
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/calloutcome"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// PreparedSummaryTransaction is the canonical already-matched
// Summary-to-CallOutcome transaction. Routing/read caches remain owned by
// OutcomeProvider; relation composition calls this directly after exact
// specialization, avoiding a per-call Snapshot and provider closure.
type PreparedSummaryTransaction struct {
	callerBodyID            lexicalidentity.StableLexicalBodyID
	returnedAllocations     *returnedAllocationIdentityCache
	index                   *SummaryIndex
	facts                   factflow.Facts
	sources                 sourcevalue.SourceValues
	calleeValue             CalleeValueFunc
	receiverCallable        ReceiverCallableFunc
	returnPresenceRelations ReturnPresenceRelationsForPathFunc
	callerKeySpace          *keyspace.KeySpace
	typeValues              *typevalue.Cache
}

func newPreparedSummaryTransaction(config ProviderConfig, index *SummaryIndex) PreparedSummaryTransaction {
	return PreparedSummaryTransaction{
		callerBodyID:        config.CallerBodyID,
		returnedAllocations: &returnedAllocationIdentityCache{},
		index:               index, facts: config.Facts, sources: config.Sources,
		calleeValue: config.CalleeValue, receiverCallable: config.ReceiverCallable,
		returnPresenceRelations: config.ReturnPresenceRelations,
		callerKeySpace:          config.KeySpace, typeValues: config.TypeValues,
	}
}

func NewPreparedSummaryTransaction(config ProviderConfig) PreparedSummaryTransaction {
	return newPreparedSummaryTransaction(config, summaryIndexFromProviderConfig(config))
}

func (t PreparedSummaryTransaction) FunctionType(key summary.SummaryKey) *typ.Function {
	if t.index == nil {
		return nil
	}
	return t.index.functionTypes[key]
}

// Apply consumes an exact matched Summary. summaryOwned means the caller lends
// immutable backing storage which must be cloned before mutation.
func (t PreparedSummaryTransaction) Apply(ctx transfer.NodeContext, site factflow.CallSiteView, in state.State, read func(cfg.Point) state.State, got summary.Summary, fn *typ.Function, summaryOwned bool) callpayload.CallOutcome {
	got = materializeReturnRootTypesFromFacts(ctx.Registry, t.typeValues, got)
	if len(got.ReturnParamPathAliases) != 0 {
		if summaryOwned {
			got = got.Clone()
			summaryOwned = false
		}
		got = materializeReturnParamPathAliases(ctx, t.callerKeySpace, site, got, t.sources, in, read, t.typeValues)
	}
	if summaryOwned && len(got.FreshHeapAllocations) != 0 {
		got = got.Clone()
		summaryOwned = false
	} else if summaryOwned && len(got.HeapTableObjects) != 0 {
		got.HeapTableObjects = heapidentity.CloneMap(got.HeapTableObjects)
	}
	if t.index != nil {
		got = instantiateReturnedAllocations(ctx, t.callerBodyID, t.index.owner, got, t.returnedAllocations)
	}
	summaryParamOffset := summaryParamObligationOffset(ctx, site, fn, in, read, t.calleeValue)
	out := outcomeFromSummary(ctx.Registry, got, summaryParamOffset, func(index int) bool {
		return index >= 0 && index < len(got.ParamObligations) && summary.UsefulParamObligation(ctx.Registry, got.ParamObligations[index])
	}, func(index int) bool {
		return index >= 0 && index < len(got.NormalReturnParams) && summary.UsefulNormalReturnParam(ctx.Registry, got.NormalReturnParams[index])
	})
	out = refineOutcomeResultsFromCurrentCallable(ctx, site, t.sources, in, read, t.calleeValue, t.receiverCallable, t.typeValues, out)
	if t.returnPresenceRelations != nil && len(got.ParamMemberReturnSlots) != 0 {
		out.ReturnPresenceRelations = append(out.ReturnPresenceRelations,
			paramMemberReturnPresenceRelations(ctx, t.callerKeySpace, site, got, t.facts, t.returnPresenceRelations)...)
	}
	if fn != nil && len(got.ReturnParamPathAliases) != 0 {
		out.ParamExposures = append(out.ParamExposures, paramReturnExposures(ctx.Registry, t.typeValues, site.ArgumentSourceCount(), got, fn)...)
	}
	if fn != nil && len(got.NormalReturnFacts.StoreRelations) != 0 {
		out.ParamExposures = append(out.ParamExposures, paramStoreRelationExposures(ctx.Registry, t.typeValues, site.ArgumentSourceCount(), got, fn)...)
	}
	if fn != nil && len(got.NormalReturnFacts.PathInvalidations) != 0 {
		out.ParamExposures = append(out.ParamExposures, paramDirectMutationExposures(ctx.Registry, t.typeValues, site.ArgumentSourceCount(), got, fn)...)
	}
	if len(got.ParamSinkExposures) != 0 {
		out.ParamExposures = append(out.ParamExposures, paramSinkExposures(ctx.Registry, t.typeValues, site.ArgumentSourceCount(), got)...)
	}
	if fn != nil {
		out.Results = padMissingResultsToNil(ctx.Registry, site, out.Results, len(fn.Returns))
	}
	out.PostReturnAuthority = calloutcome.HasAuthoritativePostReturnEvidence(ctx.Registry, out)
	if fn != nil && len(fn.Params) != 0 {
		out.ParamObligations = append(out.ParamObligations, functionTypeParamObligationsForSite(ctx, t.typeValues, site, fn, t.sources, in, read)...)
	}
	if len(got.ParamMemberCallObligations) != 0 {
		out.ParamObligations = append(out.ParamObligations, memberCallParamObligations(ctx, site, got, fn, t.sources, in, read, t.typeValues)...)
		out.ParamObligations = append(out.ParamObligations, memberCallParamObligationOriginsFromSummary(ctx.Registry, got, fn)...)
	}
	return out
}
