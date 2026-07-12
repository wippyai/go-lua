package callresult

import (
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/calloutcome"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	typenormalize "github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typecall"
)

type providerPreparedSummaryCacheKey struct {
	reg *axis.Registry
	key summary.SummaryKey
}

type providerPreparedSummary struct {
	sum summary.Summary
	fn  *typ.Function
}

type providerPreparedSummaryCache struct {
	summaries      summary.Reader
	functionTypes  map[summary.SummaryKey]*typ.Function
	callerKeySpace *keyspace.KeySpace
	entries        map[providerPreparedSummaryCacheKey]providerPreparedSummary
}

func (c *providerPreparedSummaryCache) read(
	reg *axis.Registry,
	typeValues *typevalue.Cache,
	key summary.SummaryKey,
) (summary.Summary, *typ.Function, bool) {
	if c == nil || c.summaries == nil {
		return summary.Summary{}, nil, false
	}
	cacheKey := providerPreparedSummaryCacheKey{reg: reg, key: key}
	if cached, ok := c.entries[cacheKey]; ok {
		return cached.sum, cached.fn, true
	}
	got, ok, _ := readProviderSummary(c.summaries, key)
	if !ok {
		return summary.Summary{}, nil, false
	}
	fn := c.functionTypes[key]
	got = got.RekeyHeapTableObjects(c.callerKeySpace)
	got = applyDeclaredSummaryReturns(reg, typeValues, got, fn)
	if c.entries == nil {
		c.entries = make(map[providerPreparedSummaryCacheKey]providerPreparedSummary)
	}
	c.entries[cacheKey] = providerPreparedSummary{sum: got, fn: fn}
	return got, fn, true
}

func joinedSummaryForDefinitionPath(
	ctx transfer.NodeContext,
	site factflow.CallSiteView,
	in state.State,
	read func(cfg.Point) state.State,
	calleeValue CalleeValueFunc,
	summaries summary.Reader,
	preparedSummaries *providerPreparedSummaryCache,
	pathMultiKeys map[factflow.CalleePathKey][]summary.SummaryKey,
	functionTypes map[summary.SummaryKey]*typ.Function,
	facts factflow.Facts,
	functionKeys map[symbol.ID]summary.SummaryKey,
	functionExpressionKeys map[factflow.ExprRef]summary.SummaryKey,
	sources sourcevalue.SourceValues,
	typeValues *typevalue.Cache,
) (summary.Summary, bool) {
	if ctx.Registry == nil || summaries == nil || len(pathMultiKeys) == 0 {
		return summary.Summary{}, false
	}
	calleeKey := site.CalleePathKey()
	if calleeKey == "" {
		return summary.Summary{}, false
	}
	keys := pathMultiKeys[calleeKey]
	if len(keys) < 2 {
		return summary.Summary{}, false
	}
	if currentCalleeBlocksDefinitionPath(ctx, site, in, read, calleeValue) {
		return summary.Summary{}, false
	}
	var out summary.Summary
	seen := false
	for _, key := range keys {
		got, fn, ok := preparedSummaries.read(ctx.Registry, typeValues, key)
		if !ok {
			continue
		}
		if summaryNeedsGenericInstantiation(ctx, fn, sources) {
			got = specializeGenericSummary(ctx, site, got, fn, summaries, facts, functionKeys, functionExpressionKeys, functionTypes, sources, in, read, typeValues)
		}
		if !seen {
			out = got
			seen = true
			continue
		}
		out = joinPossibleCallSummaries(ctx.Registry, typeValues, out, got)
	}
	return out, seen
}

func joinPossibleCallSummaries(reg *axis.Registry, typeValues *typevalue.Cache, left, right summary.Summary) summary.Summary {
	left = materializeReturnRootTypesFromFacts(reg, typeValues, left)
	right = materializeReturnRootTypesFromFacts(reg, typeValues, right)
	out := summary.Join(reg, left, right)
	for i := range out.Returns {
		leftValue, leftOK := summaryReturnValueAt(reg, left, i)
		rightValue, rightOK := summaryReturnValueAt(reg, right, i)
		if !leftOK || !rightOK {
			continue
		}
		leftType, leftTypeOK := typevalue.TypeOf(reg, leftValue)
		rightType, rightTypeOK := typevalue.TypeOf(reg, rightValue)
		if !leftTypeOK || !rightTypeOK {
			continue
		}
		t := typenormalize.UnionForEvidence(leftType, rightType)
		out.Returns[i] = typeWitnessValue(reg, typeValues, t)
	}
	return dropDescendantFactsBelowMaybeAbsentReturns(out)
}

func currentFunctionIdentityKey(
	ctx transfer.NodeContext,
	site factflow.CallSiteView,
	in state.State,
	read func(cfg.Point) state.State,
	calleeValue CalleeValueFunc,
	functionIDs map[identity.ID]summary.SummaryKey,
) (summary.SummaryKey, bool) {
	id, ok := currentCalleeIdentity(ctx, site, in, read, calleeValue)
	if !ok {
		return summary.SummaryKey{}, false
	}
	key, ok := functionIDs[id]
	if !ok {
		return summary.SummaryKey{}, false
	}
	return key, true
}

func contextKeyMatchesCurrentIdentity(contextKey, currentIdentityKey summary.SummaryKey, hasCurrentIdentityKey bool) bool {
	return !hasCurrentIdentityKey || contextKey.Ref == currentIdentityKey.Ref
}

func readProviderSummary(reader summary.Reader, key summary.SummaryKey) (summary.Summary, bool, bool) {
	if reader == nil {
		return summary.Summary{}, false, false
	}
	if owned, ok := reader.(summary.OwnedNormalizedReader); ok {
		got, readOK := owned.ReadOwnedNormalized(key)
		return got, readOK, readOK
	}
	got, ok := reader.Read(key)
	return got, ok, false
}

func inheritSameIdentityContextBaseSummary(
	reg *axis.Registry,
	typeValues *typevalue.Cache,
	preparedSummaries *providerPreparedSummaryCache,
	currentIdentityKey summary.SummaryKey,
	hasCurrentIdentityKey bool,
	matchedContextKey summary.SummaryKey,
	got summary.Summary,
) summary.Summary {
	if reg == nil || preparedSummaries == nil || !hasCurrentIdentityKey ||
		currentIdentityKey == matchedContextKey ||
		currentIdentityKey.Ref != matchedContextKey.Ref {
		return got
	}
	base, _, ok := preparedSummaries.read(reg, typeValues, currentIdentityKey)
	if !ok {
		return got
	}
	got = inheritMissingSameIdentityReturns(reg, got, base)
	got.NormalReturnFacts = got.NormalReturnFacts.Append(base.NormalReturnFacts)
	return summary.NormalizeOwned(reg, got)
}

func inheritMissingSameIdentityReturns(reg *axis.Registry, got, base summary.Summary) summary.Summary {
	if reg == nil || len(base.Returns) == 0 {
		return got
	}
	bottom := product.Bottom(reg)
	width := len(got.Returns)
	if len(base.Returns) > width {
		width = len(base.Returns)
	}
	var next []product.Value
	for i := 0; i < width; i++ {
		var value product.Value
		if i < len(got.Returns) {
			value = got.Returns[i]
		} else {
			value = bottom
		}
		if product.Equal(reg, value, bottom) && i < len(base.Returns) {
			if next == nil {
				next = make([]product.Value, width)
				copy(next, got.Returns)
				for j := len(got.Returns); j < width; j++ {
					next[j] = bottom
				}
			}
			next[i] = base.Returns[i]
		}
	}
	if next == nil {
		return got
	}
	got.Returns = next
	return got
}

func unresolvedFunctionCallOutcome(
	ctx transfer.NodeContext,
	site factflow.CallSiteView,
	in state.State,
	read func(cfg.Point) state.State,
	calleeValue CalleeValueFunc,
) (callpayload.CallOutcome, bool) {
	if ctx.Registry == nil || calleeValue == nil {
		return callpayload.CallOutcome{}, false
	}
	value, ok := calleeValue(ctx, site, in, read)
	if !ok {
		return callpayload.CallOutcome{}, false
	}
	if functionWitnessHasUsableReturns(ctx.Registry, value) {
		return callpayload.CallOutcome{}, false
	}
	if !product.Get(ctx.Registry, value, runtimekind.Key).Contains(runtimekind.Function) {
		return callpayload.CallOutcome{}, false
	}
	results := unknownResultSlots(ctx.Registry, site)
	if len(results) == 0 {
		return callpayload.CallOutcome{}, false
	}
	return callpayload.CallOutcome{Results: results}, true
}

func refineOutcomeResultsFromCurrentCallable(
	ctx transfer.NodeContext,
	site factflow.CallSiteView,
	sources sourcevalue.SourceValues,
	in state.State,
	read func(cfg.Point) state.State,
	calleeValue CalleeValueFunc,
	receiverCallable ReceiverCallableFunc,
	typeValues *typevalue.Cache,
	out callpayload.CallOutcome,
) callpayload.CallOutcome {
	if ctx.Registry == nil {
		return out
	}
	fn, ok := currentCallableFunctionType(ctx, site, sources, in, read, calleeValue, receiverCallable)
	if !ok || fn == nil || len(fn.TypeParams) != 0 || len(fn.Returns) == 0 {
		return out
	}
	if currentCallableChannelMethodMaySuspend(site, fn) {
		out.SuspensionKnown = true
		out.MaySuspend = true
	}
	results := make([]callpayload.CallResult, 0, len(fn.Returns))
	for i, ret := range fn.Returns {
		if ret == nil || typ.IsAny(ret) || typ.IsUnknown(ret) {
			continue
		}
		results = append(results, callpayload.CallResult{
			Index: i,
			Value: typeWitnessValue(ctx.Registry, typeValues, ret),
		})
	}
	if len(results) == 0 {
		return out
	}
	supplemental := callpayload.CallOutcome{Results: results}
	supplemental.ReturnConditionSlots = channelReceiveReturnConditionSlots(ctx.Registry, typeValues, site, fn)
	return calloutcome.MergeSupplemental(ctx.Registry, out, supplemental)
}

func currentCallableChannelMethodMaySuspend(site factflow.CallSiteView, fn *typ.Function) bool {
	switch site.MethodName() {
	case "receive", "send":
	default:
		return false
	}
	if fn == nil || len(fn.Params) == 0 || fn.Params[0].Type == nil {
		return false
	}
	_, ok := typecall.AmbientChannelPayloadType(fn.Params[0].Type)
	return ok
}

func channelReceiveReturnConditionSlots(
	reg *axis.Registry,
	typeValues *typevalue.Cache,
	site factflow.CallSiteView,
	fn *typ.Function,
) []callpayload.CallReturnConditionSlotRefinement {
	if reg == nil || site.MethodName() != "receive" || fn == nil || len(fn.Returns) < 2 {
		return nil
	}
	payload := fn.Returns[0]
	okType := fn.Returns[1]
	if payload == nil || okType == nil || !typ.IsBooleanType(okType) {
		return nil
	}
	return []callpayload.CallReturnConditionSlotRefinement{{
		ReturnIndex: 1,
		ReturnValue: true,
		TargetIndex: 0,
		Value:       product.WithPresence(reg, typeWitnessValue(reg, typeValues, payload), presence.Present()),
	}}
}

func summaryParamObligationOffset(
	ctx transfer.NodeContext,
	site factflow.CallSiteView,
	fn *typ.Function,
	in state.State,
	read func(cfg.Point) state.State,
	calleeValue CalleeValueFunc,
) int {
	if site.MethodName() == "" {
		return 0
	}
	if fn == nil {
		fn, _ = currentCallableFunctionType(ctx, site, nil, in, read, calleeValue, nil)
	}
	if !callableConsumesMethodReceiver(ctx, site, fn, nil, in, read, nil) {
		return 0
	}
	return 1
}

func currentCallableFunctionType(
	ctx transfer.NodeContext,
	site factflow.CallSiteView,
	sources sourcevalue.SourceValues,
	in state.State,
	read func(cfg.Point) state.State,
	calleeValue CalleeValueFunc,
	receiverCallable ReceiverCallableFunc,
) (*typ.Function, bool) {
	if ctx.Registry == nil || calleeValue == nil {
		if receiverCallable != nil {
			if fn, ok := receiverCallable(ctx, site); ok {
				return fn, true
			}
		}
		return receiverMemberCallableFunctionType(ctx, site, sources, in, read)
	}
	value, ok := calleeValue(ctx, site, in, read)
	if !ok {
		if receiverCallable != nil {
			if fn, ok := receiverCallable(ctx, site); ok {
				return fn, true
			}
		}
		return receiverMemberCallableFunctionType(ctx, site, sources, in, read)
	}
	if t, ok := typevalue.TypeOf(ctx.Registry, value); ok {
		if fn, callableOK := typecall.Callable(t); callableOK && fn != nil {
			return fn, true
		}
	}
	if receiverCallable != nil {
		if fn, ok := receiverCallable(ctx, site); ok {
			return fn, true
		}
	}
	return receiverMemberCallableFunctionType(ctx, site, sources, in, read)
}

func receiverMemberCallableFunctionType(
	ctx transfer.NodeContext,
	site factflow.CallSiteView,
	sources sourcevalue.SourceValues,
	in state.State,
	read func(cfg.Point) state.State,
) (*typ.Function, bool) {
	method := site.MethodName()
	if method == "" {
		return nil, false
	}
	receiverType, ok := methodReceiverType(ctx, site, sources, in, read)
	if !ok || receiverType == nil || typ.IsAny(receiverType) || typ.IsUnknown(receiverType) || typ.IsNever(receiverType) {
		return nil, false
	}
	fn, status, ok := typecall.MemberCallable(receiverType, method)
	return fn, status == typecall.MemberCallOK && ok && fn != nil
}

func callableConsumesMethodReceiver(
	ctx transfer.NodeContext,
	site factflow.CallSiteView,
	fn *typ.Function,
	sources sourcevalue.SourceValues,
	in state.State,
	read func(cfg.Point) state.State,
	fallback typ.Type,
) bool {
	if fn == nil || len(fn.Params) == 0 {
		return false
	}
	first := fn.Params[0]
	if first.Receiver {
		return true
	}
	receiverType := fallback
	if receiverType == nil {
		receiverType, _ = methodReceiverType(ctx, site, sources, in, read)
	}
	return typecall.ParamConsumesReceiver(first.Receiver, first.Type, receiverType)
}

func functionWitnessHasUsableReturns(reg *axis.Registry, value product.Value) bool {
	if reg == nil {
		return false
	}
	t, ok := product.Get(reg, value, typewitness.Key).Type()
	if !ok {
		return false
	}
	fn, ok := typecall.Callable(t)
	return ok && fn != nil && len(fn.Returns) != 0
}

func unknownResultSlots(reg *axis.Registry, site factflow.CallSiteView) []callpayload.CallResult {
	if reg == nil {
		return nil
	}
	unknown := product.Set(reg, product.Top(), evidence.Key, evidence.GradualTop())
	seen := make(map[int]struct{})
	var out []callpayload.CallResult
	site.ForEachResultTarget(func(target factflow.CallResultTargetView) bool {
		index := target.ResultIndex()
		if index < 0 {
			return true
		}
		if _, ok := seen[index]; ok {
			return true
		}
		seen[index] = struct{}{}
		out = append(out, callpayload.CallResult{Index: index, Value: unknown})
		return true
	})
	return out
}

// summaryKeyForDefinitionPath resolves a member-call callee to its summary by the
// callee's syntactic definition path. It is the sound fallback for a callee value
// rehydrated across a closure boundary, where the function value carries no
// runtime identity: the path resolves only when the current callee value holds no
// conflicting identity, so a later reassignment (whose value carries the new
// identity) is resolved by the current-identity route instead.
func summaryKeyForDefinitionPath(
	ctx transfer.NodeContext,
	site factflow.CallSiteView,
	in state.State,
	read func(cfg.Point) state.State,
	calleeValue CalleeValueFunc,
	pathKeys map[factflow.CalleePathKey]summary.SummaryKey,
) (summary.SummaryKey, bool) {
	if len(pathKeys) == 0 {
		return summary.SummaryKey{}, false
	}
	calleeKey := site.CalleePathKey()
	if calleeKey == "" {
		return summary.SummaryKey{}, false
	}
	key, ok := pathKeys[calleeKey]
	if !ok {
		return summary.SummaryKey{}, false
	}
	if currentCalleeBlocksDefinitionPath(ctx, site, in, read, calleeValue) {
		return summary.SummaryKey{}, false
	}
	return key, true
}

func currentCalleeBlocksDefinitionPath(
	ctx transfer.NodeContext,
	site factflow.CallSiteView,
	in state.State,
	read func(cfg.Point) state.State,
	calleeValue CalleeValueFunc,
) bool {
	if ctx.Registry == nil || calleeValue == nil {
		return false
	}
	value, ok := calleeValue(ctx, site, in, read)
	if !ok {
		return false
	}
	if _, hasID := product.Get(ctx.Registry, value, identity.Key).ID(); hasID {
		return true
	}
	return !product.Get(ctx.Registry, value, runtimekind.Key).Contains(runtimekind.Function)
}

func currentCalleeIdentity(
	ctx transfer.NodeContext,
	site factflow.CallSiteView,
	in state.State,
	read func(cfg.Point) state.State,
	calleeValue CalleeValueFunc,
) (identity.ID, bool) {
	if calleeValue == nil {
		return identity.ID{}, false
	}
	value, ok := calleeValue(ctx, site, in, read)
	if !ok {
		return identity.ID{}, false
	}
	id, ok := product.Get(ctx.Registry, value, identity.Key).ID()
	if !ok {
		return identity.ID{}, false
	}
	return id, true
}

func clonePathMultiKeys(in map[factflow.CalleePathKey][]summary.SummaryKey) map[factflow.CalleePathKey][]summary.SummaryKey {
	if len(in) == 0 {
		return nil
	}
	out := make(map[factflow.CalleePathKey][]summary.SummaryKey, len(in))
	for pathKey, keys := range in {
		if len(keys) == 0 {
			continue
		}
		out[pathKey] = append([]summary.SummaryKey(nil), keys...)
	}
	return out
}
