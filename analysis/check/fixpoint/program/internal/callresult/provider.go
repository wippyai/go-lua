// Package callresult adapts fixpoint summaries into factflow call outcomes.
package callresult

import (
	"github.com/wippyai/go-lua/analysis/check/fixpoint/program/internal/memberaccess"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/typewitness"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/calloutcome"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/factquery"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/internal/mapedit"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/typecall"
	luatypeprojection "github.com/wippyai/go-lua/analysis/lua/typeprojection"
	"github.com/wippyai/go-lua/analysis/symbol"
	typenormalize "github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/refinement"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// KeyFunc maps one call site in context to an exact summary key.
type KeyFunc func(ctx transfer.NodeContext, site factflow.CallSiteView) (summary.SummaryKey, bool)

// CalleeValueFunc resolves the current callee expression value at a call site.
type CalleeValueFunc func(ctx transfer.NodeContext, site factflow.CallSiteView, in state.State, read func(cfg.Point) state.State) (product.Value, bool)

// ReturnPresenceRelationsForPathFunc resolves an in-scope access path to
// lowered return-presence relations, when the path has a stable imported/global
// signature identity.
type ReturnPresenceRelationsForPathFunc func(point cfg.Point, p pathdom.Path) []callpayload.CallReturnPresenceRelation

// ProviderConfig configures summary-backed call outcomes.
type ProviderConfig struct {
	Summaries               summary.Reader
	KeyFor                  KeyFunc
	CalleeValue             CalleeValueFunc
	Facts                   factflow.Facts
	FunctionKeys            map[symbol.ID]summary.SummaryKey
	FunctionExpressionKeys  map[factflow.ExprRef]summary.SummaryKey
	FunctionIDs             map[identity.ID]summary.SummaryKey
	PathKeys                map[pathdom.PathKey]summary.SummaryKey
	PathMultiKeys           map[pathdom.PathKey][]summary.SummaryKey
	FunctionTypes           map[summary.SummaryKey]*typ.Function
	Sources                 sourcevalue.SourceValues
	ReturnPresenceRelations ReturnPresenceRelationsForPathFunc
	// KeySpace is the consuming (caller) analysis keyspace. Summaries read at a
	// call site carry heap objects interned under the callee's keyspace; they are
	// rebased into this keyspace before any heap member is read or written.
	KeySpace *keyspace.KeySpace
}

// OutcomeProvider returns a generic call-boundary outcome provider backed by
// exact summary reads.
func OutcomeProvider(config ProviderConfig) callpayload.CallOutcomeProvider {
	summaries := config.Summaries
	keyFor := config.KeyFor
	calleeValue := config.CalleeValue
	facts := config.Facts
	functionKeys := mapedit.Clone(config.FunctionKeys)
	functionExpressionKeys := mapedit.Clone(config.FunctionExpressionKeys)
	functionIDs := mapedit.Clone(config.FunctionIDs)
	pathKeys := mapedit.Clone(config.PathKeys)
	pathMultiKeys := clonePathMultiKeys(config.PathMultiKeys)
	functionTypes := mapedit.Clone(config.FunctionTypes)
	sources := config.Sources
	returnPresenceRelations := config.ReturnPresenceRelations
	callerKeySpace := config.KeySpace
	return func(ctx transfer.NodeContext, site factflow.CallSiteView, in state.State, read func(cfg.Point) state.State) callpayload.CallOutcome {
		if summaries == nil {
			return callpayload.CallOutcome{}
		}
		key, ok := summaryKeyForCall(ctx, site, in, read, keyFor, calleeValue, functionIDs, pathKeys)
		var got summary.Summary
		var fn *typ.Function
		if ok {
			var readOK bool
			got, readOK = summaries.Read(key)
			if !readOK {
				return callpayload.CallOutcome{}
			}
			got = got.RekeyHeapTableObjects(callerKeySpace)
			fn = functionTypes[key]
			got = applyDeclaredSummaryReturns(ctx.Registry, got, fn)
			if summaryNeedsGenericInstantiation(ctx, fn, sources) {
				got = specializeGenericSummary(ctx, site, got, fn, summaries, facts, functionKeys, functionExpressionKeys, functionTypes, sources, in, read)
			}
			got = materializeReturnRootTypesFromFacts(ctx.Registry, got)
		} else if joined, joinedOK := joinedSummaryForDefinitionPath(ctx, site, in, read, calleeValue, summaries, pathMultiKeys, functionTypes, facts, functionKeys, functionExpressionKeys, sources, callerKeySpace); joinedOK {
			got = joined
		} else {
			if out, ok := unresolvedFunctionCallOutcome(ctx, site, in, read, calleeValue); ok {
				return out
			}
			return callpayload.CallOutcome{}
		}
		if len(got.ReturnParamPathAliases) != 0 {
			got = materializeReturnParamPathAliases(ctx, callerKeySpace, site, got, sources, in, read)
		}
		out := outcomeFromSummary(got, func(index int) bool {
			if index < 0 || index >= len(got.ParamObligations) {
				return false
			}
			return summary.UsefulParamObligation(ctx.Registry, got.ParamObligations[index])
		}, func(index int) bool {
			if index < 0 || index >= len(got.NormalReturnParams) {
				return false
			}
			return summary.UsefulNormalReturnParam(ctx.Registry, got.NormalReturnParams[index])
		})
		if returnPresenceRelations != nil && len(got.ParamMemberReturnSlots) != 0 {
			out.ReturnPresenceRelations = append(
				out.ReturnPresenceRelations,
				paramMemberReturnPresenceRelations(ctx, site, got, facts, returnPresenceRelations)...,
			)
		}
		if fn != nil && len(got.ReturnParamPathAliases) != 0 {
			out.ParamExposures = append(out.ParamExposures, paramReturnExposures(ctx.Registry, site.ArgumentSourceCount(), got, fn)...)
		}
		if fn != nil && len(got.NormalReturnFacts.StoreRelations) != 0 {
			out.ParamExposures = append(out.ParamExposures, paramStoreRelationExposures(ctx.Registry, site.ArgumentSourceCount(), got, fn)...)
		}
		if len(got.ParamSinkExposures) != 0 {
			out.ParamExposures = append(out.ParamExposures, paramSinkExposures(ctx.Registry, site.ArgumentSourceCount(), got)...)
		}
		if fn != nil {
			out.Results = padMissingResultsToNil(ctx.Registry, site, out.Results, len(fn.Returns))
		}
		out.PostReturnAuthority = calloutcome.HasAuthoritativePostReturnEvidence(ctx.Registry, out)
		if fn != nil && len(fn.Params) != 0 {
			out.ParamObligations = append(out.ParamObligations, functionTypeParamObligations(ctx.Registry, site.ArgumentSourceCount(), fn)...)
		}
		if len(got.ParamMemberCallObligations) != 0 {
			out.ParamObligations = append(out.ParamObligations, memberCallParamObligations(ctx, site, got, sources, in, read)...)
		}
		return out
	}
}

func joinedSummaryForDefinitionPath(
	ctx transfer.NodeContext,
	site factflow.CallSiteView,
	in state.State,
	read func(cfg.Point) state.State,
	calleeValue CalleeValueFunc,
	summaries summary.Reader,
	pathMultiKeys map[pathdom.PathKey][]summary.SummaryKey,
	functionTypes map[summary.SummaryKey]*typ.Function,
	facts factflow.Facts,
	functionKeys map[symbol.ID]summary.SummaryKey,
	functionExpressionKeys map[factflow.ExprRef]summary.SummaryKey,
	sources sourcevalue.SourceValues,
	callerKeySpace *keyspace.KeySpace,
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
		got, ok := summaries.Read(key)
		if !ok {
			continue
		}
		fn := functionTypes[key]
		got = applyDeclaredSummaryReturns(ctx.Registry, got, fn)
		if summaryNeedsGenericInstantiation(ctx, fn, sources) {
			got = specializeGenericSummary(ctx, site, got, fn, summaries, facts, functionKeys, functionExpressionKeys, functionTypes, sources, in, read)
		}
		got = got.RekeyHeapTableObjects(callerKeySpace)
		if !seen {
			out = got
			seen = true
			continue
		}
		out = joinPossibleCallSummaries(ctx.Registry, out, got)
	}
	return out, seen
}

func joinPossibleCallSummaries(reg *axis.Registry, left, right summary.Summary) summary.Summary {
	left = materializeReturnRootTypesFromFacts(reg, left)
	right = materializeReturnRootTypesFromFacts(reg, right)
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
		value := typevalue.FromType(reg, t)
		out.Returns[i] = typevalue.WithWitness(reg, value, t)
	}
	return dropDescendantFactsBelowMaybeAbsentReturns(out)
}

func materializeReturnRootTypesFromFacts(reg *axis.Registry, sum summary.Summary) summary.Summary {
	if reg == nil || len(sum.NormalReturnFacts.PathRefinements) == 0 && len(sum.NormalReturnFacts.PathStaticMembers) == 0 {
		return sum
	}
	maxIndex := len(sum.Returns) - 1
	for _, fact := range sum.NormalReturnFacts.PathRefinements {
		if index := fact.Path.PlaceholderIndex(); index > maxIndex {
			maxIndex = index
		}
	}
	for _, fact := range sum.NormalReturnFacts.PathStaticMembers {
		if index := fact.Path.PlaceholderIndex(); index > maxIndex {
			maxIndex = index
		}
	}
	if maxIndex < 0 {
		return sum
	}
	if len(sum.Returns) <= maxIndex {
		expanded := make([]product.Value, maxIndex+1)
		copy(expanded, sum.Returns)
		sum.Returns = expanded
	}
	for index := 0; index <= maxIndex; index++ {
		if !returnSlotNeedsFactType(reg, sum.Returns[index]) {
			continue
		}
		t, ok := returnRecordTypeFromFacts(reg, sum.NormalReturnFacts, index)
		if !ok {
			continue
		}
		value := typevalue.FromType(reg, t)
		sum.Returns[index] = typevalue.WithWitness(reg, value, t)
	}
	return sum
}

func returnSlotNeedsFactType(reg *axis.Registry, value product.Value) bool {
	if product.Equal(reg, value, product.Bottom(reg)) {
		return true
	}
	_, ok := typevalue.TypeOf(reg, value)
	return !ok
}

func returnRecordTypeFromFacts(reg *axis.Registry, facts callboundary.NormalReturnFacts, index int) (typ.Type, bool) {
	var parts typ.RecordParts
	seen := false
	add := func(path pathdom.Path, value product.Value) {
		if path.PlaceholderIndex() != index || len(path.Segments) != 1 {
			return
		}
		t, ok := typevalue.TypeOf(reg, value)
		if !ok || t == nil {
			return
		}
		if name, ok := path.DirectFieldName(); ok {
			payload, optional := typetable.EntryValueShape(t)
			parts.Fields = append(parts.Fields, typ.Field{Name: name, Type: payload, Optional: optional})
			seen = true
			return
		}
		if index, ok := path.DirectIntIndex(); ok {
			payload, optional := typetable.EntryValueShape(t)
			parts.StaticMembers = append(parts.StaticMembers, typ.StaticMember{Kind: typ.StaticMemberIntIndex, Index: int64(index), Type: payload, Optional: optional})
			seen = true
		}
	}
	for _, fact := range facts.PathRefinements {
		add(fact.Path, fact.Value)
	}
	for _, fact := range facts.PathStaticMembers {
		add(fact.Path, fact.Value)
	}
	if !seen {
		return nil, false
	}
	return typetable.RebuildRecord(parts), true
}

func materializeReturnParamPathAliases(
	ctx transfer.NodeContext,
	ks *keyspace.KeySpace,
	site factflow.CallSiteView,
	got summary.Summary,
	sources sourcevalue.SourceValues,
	in state.State,
	read func(cfg.Point) state.State,
) summary.Summary {
	if ctx.Registry == nil || sources == nil || len(got.ReturnParamPathAliases) == 0 {
		return got
	}
	objects := got.HeapTableObjects
	if len(objects) == 0 {
		return got
	}
	changed := false
	for _, alias := range got.ReturnParamPathAliases {
		value, ok := returnParamAliasSourceValue(ctx, ks, site, alias.Source, sources, in, read)
		if !ok {
			continue
		}
		if writeReturnParamAliasMember(ctx.Registry, ks, objects, got.Returns, alias.ReturnIndex, alias.Member, value) {
			changed = true
		}
	}
	if !changed {
		return got
	}
	got.HeapTableObjects = objects
	return summary.NormalizeOwned(ctx.Registry, got)
}

func returnParamAliasSourceValue(
	ctx transfer.NodeContext,
	ks *keyspace.KeySpace,
	site factflow.CallSiteView,
	sourceKey pathdom.PathKey,
	sources sourcevalue.SourceValues,
	in state.State,
	read func(cfg.Point) state.State,
) (product.Value, bool) {
	sourcePath, ok := pathaddr.PlaceholderPathFromKey(sourceKey)
	if !ok {
		return product.Value{}, false
	}
	index := sourcePath.PlaceholderIndex()
	source, ok := site.ArgumentSourceAt(index)
	if !ok {
		return product.Value{}, false
	}
	value, ok := sources.ValueOfSource(ctx.Point, source, in, read)
	if !ok {
		return product.Value{}, false
	}
	if len(sourcePath.Segments) == 0 {
		return value, true
	}
	return sourcevalue.HeapMemberFromValue(ctx.Registry, ks, in, value, sourcePath.Segments)
}

func writeReturnParamAliasMember(
	reg *axis.Registry,
	ks *keyspace.KeySpace,
	objects map[identity.ID]heapidentity.TableObject,
	returns []product.Value,
	returnIndex int,
	memberKey pathaddr.SuffixKey,
	value product.Value,
) bool {
	rootID := returnIdentityAt(reg, returns, returnIndex)
	if rootID == (identity.ID{}) {
		return false
	}
	segments, ok := pathaddr.RelativeStaticMemberSuffixSegments(memberKey)
	if !ok || len(segments) == 0 {
		return false
	}
	key, ok := ks.FromRootlessSuffix(segments)
	if !ok {
		return false
	}
	changed := writeHeapObjectStaticMember(reg, objects, rootID, returns[returnIndex], key, value)
	if writeNestedHeapObjectStaticMember(reg, ks, objects, rootID, returns[returnIndex], segments, value) {
		changed = true
	}
	return changed
}

func writeNestedHeapObjectStaticMember(
	reg *axis.Registry,
	ks *keyspace.KeySpace,
	objects map[identity.ID]heapidentity.TableObject,
	rootID identity.ID,
	rootValue product.Value,
	segments []segment.Segment,
	value product.Value,
) bool {
	if len(segments) < 2 {
		return false
	}
	currentID := rootID
	currentValue := rootValue
	for len(segments) > 1 {
		key, ok := ks.FromRootlessSuffix(segments[:1])
		if !ok {
			return false
		}
		object, ok := objects[currentID]
		if !ok {
			return false
		}
		nextValue, ok := object.StaticMember(key)
		if !ok {
			return false
		}
		nextID, ok := product.Get(reg, nextValue, identity.Key).ID()
		if !ok {
			return false
		}
		currentID = nextID
		currentValue = nextValue
		segments = segments[1:]
	}
	key, ok := ks.FromRootlessSuffix(segments)
	if !ok {
		return false
	}
	return writeHeapObjectStaticMember(reg, objects, currentID, currentValue, key, value)
}

func writeHeapObjectStaticMember(
	reg *axis.Registry,
	objects map[identity.ID]heapidentity.TableObject,
	id identity.ID,
	root product.Value,
	key keyspace.Key,
	value product.Value,
) bool {
	if id == (identity.ID{}) || key.Kind == keyspace.KindInvalid {
		return false
	}
	object := objects[id]
	staticMembers := object.StaticMembers()
	if staticMembers == nil {
		staticMembers = make(map[keyspace.Key]product.Value, 1)
	}
	if existing, ok := staticMembers[key]; ok && product.Equal(reg, existing, value) {
		return false
	}
	staticMembers[key] = value
	objectRoot := object.Root()
	if product.Equal(reg, objectRoot, product.Bottom(reg)) && !product.Equal(reg, root, product.Bottom(reg)) {
		objectRoot = root
	}
	objects[id] = heapidentity.NewTableObject(heapidentity.TableObjectConfig{
		Root:              objectRoot,
		StaticMembers:     staticMembers,
		DynamicIndexFacts: object.DynamicIndexFacts(),
	})
	return true
}

func returnIdentityAt(reg *axis.Registry, returns []product.Value, index int) identity.ID {
	if index < 0 || index >= len(returns) {
		return identity.ID{}
	}
	id, ok := product.Get(reg, returns[index], identity.Key).ID()
	if !ok {
		return identity.ID{}
	}
	return id
}

func summaryReturnValueAt(reg *axis.Registry, sum summary.Summary, index int) (product.Value, bool) {
	if index < 0 || index >= len(sum.Returns) {
		return product.Value{}, false
	}
	value := sum.Returns[index]
	if product.Equal(reg, value, product.Bottom(reg)) {
		return product.Value{}, false
	}
	return value, true
}

func dropDescendantFactsBelowMaybeAbsentReturns(sum summary.Summary) summary.Summary {
	if len(sum.Returns) == 0 {
		return sum
	}
	maybeAbsent := make(map[int]struct{})
	for i, value := range sum.Returns {
		if !product.DefinitelyPresent(value) {
			maybeAbsent[i] = struct{}{}
		}
	}
	if len(maybeAbsent) == 0 {
		return sum
	}
	facts := sum.NormalReturnFacts
	facts.PathRefinements = filterPathValueFactsBelowReturns(facts.PathRefinements, maybeAbsent)
	facts.PathStaticMembers = filterPathStaticMemberFactsBelowReturns(facts.PathStaticMembers, maybeAbsent)
	facts.PathInvalidations = filterPathInvalidationFactsBelowReturns(facts.PathInvalidations, maybeAbsent)
	facts.DynamicIndexFacts = filterDynamicIndexFactsBelowReturns(facts.DynamicIndexFacts, maybeAbsent)
	facts.BranchProofs = filterBranchProofsBelowReturns(facts.BranchProofs, maybeAbsent)
	facts.ChannelSelects = filterChannelSelectsBelowReturns(facts.ChannelSelects, maybeAbsent)
	facts.FrozenTables = filterFrozenTablesBelowReturns(facts.FrozenTables, maybeAbsent)
	facts.EffectDeltas = filterEffectDeltasBelowReturns(facts.EffectDeltas, maybeAbsent)
	facts.EscapeEvents = filterEscapeEventsBelowReturns(facts.EscapeEvents, maybeAbsent)
	facts.StoreRelations = filterStoreRelationsBelowReturns(facts.StoreRelations, maybeAbsent)
	facts.LifecycleFacts = filterLifecycleFactsBelowReturns(facts.LifecycleFacts, maybeAbsent)
	sum.NormalReturnFacts = facts
	return sum
}

func filterPathValueFactsBelowReturns(in []callboundary.PathValueFact, roots map[int]struct{}) []callboundary.PathValueFact {
	out := in[:0]
	for _, fact := range in {
		if strictPlaceholderDescendant(fact.Path, roots) {
			continue
		}
		out = append(out, fact)
	}
	return out
}

func filterPathStaticMemberFactsBelowReturns(in []callboundary.PathStaticMemberFact, roots map[int]struct{}) []callboundary.PathStaticMemberFact {
	out := in[:0]
	for _, fact := range in {
		if strictPlaceholderDescendant(fact.Path, roots) {
			continue
		}
		out = append(out, fact)
	}
	return out
}

func filterPathInvalidationFactsBelowReturns(in []callboundary.PathInvalidationFact, roots map[int]struct{}) []callboundary.PathInvalidationFact {
	out := in[:0]
	for _, fact := range in {
		if strictPlaceholderDescendant(fact.Path, roots) {
			continue
		}
		out = append(out, fact)
	}
	return out
}

func filterDynamicIndexFactsBelowReturns(in []callboundary.DynamicIndexFact, roots map[int]struct{}) []callboundary.DynamicIndexFact {
	out := in[:0]
	for _, fact := range in {
		if strictPlaceholderDescendant(fact.Table, roots) {
			continue
		}
		out = append(out, fact)
	}
	return out
}

func filterBranchProofsBelowReturns(in []callboundary.BranchProof, roots map[int]struct{}) []callboundary.BranchProof {
	out := in[:0]
	for _, fact := range in {
		if strictPlaceholderDescendant(fact.Path, roots) || strictPlaceholderDescendant(fact.Other, roots) {
			continue
		}
		out = append(out, fact)
	}
	return out
}

func filterChannelSelectsBelowReturns(in []callboundary.ChannelSelectFact, roots map[int]struct{}) []callboundary.ChannelSelectFact {
	out := in[:0]
	for _, fact := range in {
		if strictPlaceholderDescendant(fact.Result, roots) || strictPlaceholderDescendant(fact.Case, roots) {
			continue
		}
		out = append(out, fact)
	}
	return out
}

func filterFrozenTablesBelowReturns(in []callboundary.FrozenTableFact, roots map[int]struct{}) []callboundary.FrozenTableFact {
	out := in[:0]
	for _, fact := range in {
		if strictPlaceholderDescendant(fact.Target, roots) {
			continue
		}
		out = append(out, fact)
	}
	return out
}

func filterEffectDeltasBelowReturns(in []callboundary.EffectDelta, roots map[int]struct{}) []callboundary.EffectDelta {
	out := in[:0]
	for _, fact := range in {
		if strictPlaceholderDescendant(fact.Target, roots) {
			continue
		}
		out = append(out, fact)
	}
	return out
}

func filterEscapeEventsBelowReturns(in []callboundary.EscapeEventFact, roots map[int]struct{}) []callboundary.EscapeEventFact {
	out := in[:0]
	for _, fact := range in {
		if strictPlaceholderDescendant(fact.Target, roots) {
			continue
		}
		out = append(out, fact)
	}
	return out
}

func filterStoreRelationsBelowReturns(in []callboundary.StoreRelationFact, roots map[int]struct{}) []callboundary.StoreRelationFact {
	out := in[:0]
	for _, fact := range in {
		if strictPlaceholderDescendant(fact.Source, roots) || strictPlaceholderDescendant(fact.Into, roots) {
			continue
		}
		out = append(out, fact)
	}
	return out
}

func filterLifecycleFactsBelowReturns(in []callboundary.LifecycleFact, roots map[int]struct{}) []callboundary.LifecycleFact {
	out := in[:0]
	for _, fact := range in {
		if strictPlaceholderDescendant(fact.Target, roots) {
			continue
		}
		out = append(out, fact)
	}
	return out
}

func strictPlaceholderDescendant(p pathdom.Path, roots map[int]struct{}) bool {
	if len(p.Segments) == 0 {
		return false
	}
	index := p.PlaceholderIndex()
	if index < 0 {
		return false
	}
	_, ok := roots[index]
	return ok
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

func summaryKeyForCall(
	ctx transfer.NodeContext,
	site factflow.CallSiteView,
	in state.State,
	read func(cfg.Point) state.State,
	keyFor KeyFunc,
	calleeValue CalleeValueFunc,
	functionIDs map[identity.ID]summary.SummaryKey,
	pathKeys map[pathdom.PathKey]summary.SummaryKey,
) (summary.SummaryKey, bool) {
	if id, hasID := currentCalleeIdentity(ctx, site, in, read, calleeValue); hasID {
		key, ok := functionIDs[id]
		return key, ok
	}
	if keyFor != nil {
		if key, ok := keyFor(ctx, site); ok {
			return key, true
		}
	}
	return summaryKeyForDefinitionPath(ctx, site, in, read, calleeValue, pathKeys)
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
	pathKeys map[pathdom.PathKey]summary.SummaryKey,
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

// SummaryArgumentTypeProvider resolves call argument types using summary-backed
// function expression identities, then falls back to generic source-value
// projection. It is used by public signature lowering when imported generic
// calls receive local callbacks whose return types are known only after the
// program fixed point.
func SummaryArgumentTypeProvider(config ProviderConfig) func(transfer.NodeContext, factflow.ValueSource, state.State, func(cfg.Point) state.State) (typ.Type, bool) {
	summaries := config.Summaries
	facts := config.Facts
	functionKeys := mapedit.Clone(config.FunctionKeys)
	functionExpressionKeys := mapedit.Clone(config.FunctionExpressionKeys)
	functionTypes := mapedit.Clone(config.FunctionTypes)
	sources := config.Sources
	return func(ctx transfer.NodeContext, source factflow.ValueSource, in state.State, read func(cfg.Point) state.State) (typ.Type, bool) {
		return callArgumentType(ctx, source, summaries, facts, functionKeys, functionExpressionKeys, functionTypes, sources, in, read)
	}
}

func specializeGenericSummary(
	ctx transfer.NodeContext,
	site factflow.CallSiteView,
	got summary.Summary,
	fn *typ.Function,
	summaries summary.Reader,
	facts factflow.Facts,
	functionKeys map[symbol.ID]summary.SummaryKey,
	functionExpressionKeys map[factflow.ExprRef]summary.SummaryKey,
	functionTypes map[summary.SummaryKey]*typ.Function,
	sources sourcevalue.SourceValues,
	in state.State,
	read func(cfg.Point) state.State,
) summary.Summary {
	if ctx.Registry == nil || fn == nil || len(fn.TypeParams) == 0 || sources == nil {
		return got
	}
	args := callArgumentTypes(ctx, site, summaries, facts, functionKeys, functionExpressionKeys, functionTypes, sources, in, read)
	if len(args) == 0 {
		return got
	}
	instantiated, violations := typecall.InstantiateGenericCall(fn, args)
	if len(violations) != 0 || instantiated == nil || instantiated == fn {
		return got
	}
	return specializeSummaryReturns(ctx.Registry, got, fn.Returns, instantiated.Returns)
}

func summaryNeedsGenericInstantiation(ctx transfer.NodeContext, fn *typ.Function, sources sourcevalue.SourceValues) bool {
	return ctx.Registry != nil && fn != nil && len(fn.TypeParams) != 0 && sources != nil
}

func applyDeclaredSummaryReturns(reg *axis.Registry, got summary.Summary, fn *typ.Function) summary.Summary {
	if reg == nil || fn == nil || len(fn.TypeParams) != 0 || len(fn.Returns) == 0 {
		return got
	}
	return specializeSummaryReturns(reg, got, fn.Returns, fn.Returns)
}

func callArgumentTypes(
	ctx transfer.NodeContext,
	site factflow.CallSiteView,
	summaries summary.Reader,
	facts factflow.Facts,
	functionKeys map[symbol.ID]summary.SummaryKey,
	functionExpressionKeys map[factflow.ExprRef]summary.SummaryKey,
	functionTypes map[summary.SummaryKey]*typ.Function,
	sources sourcevalue.SourceValues,
	in state.State,
	read func(cfg.Point) state.State,
) []typ.Type {
	if sources == nil {
		return nil
	}
	argCount := site.ArgumentSourceCount()
	if argCount == 0 {
		return nil
	}
	var args []typ.Type
	for i := 0; i < argCount; i++ {
		source, ok := site.ArgumentSourceAt(i)
		if !ok {
			continue
		}
		t, ok := callArgumentType(ctx, source, summaries, facts, functionKeys, functionExpressionKeys, functionTypes, sources, in, read)
		if !ok {
			continue
		}
		if args == nil {
			args = make([]typ.Type, argCount)
		}
		args[i] = t
	}
	return args
}

func sourceValueAtArgument(
	reg *axis.Registry,
	point cfg.Point,
	source factflow.ValueSource,
	facts factflow.Facts,
	graph cfg.Graph,
	sources sourcevalue.SourceValues,
	in state.State,
	read func(cfg.Point) state.State,
) (product.Value, bool) {
	value, ok := valueOfSource(point, source, sources, in, read)
	if t, okType := typeFromValue(reg, value); okType && UsableType(reg, value, t) {
		return value, true
	}
	if source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
		return value, ok
	}
	if value, ok := valueFromRootDeclarationSource(reg, point, source.ExprRef, facts, graph, sources, in, read); ok {
		return value, true
	}
	return value, ok
}

func valueFromRootDeclarationSource(
	reg *axis.Registry,
	point cfg.Point,
	expr factflow.ExprRef,
	facts factflow.Facts,
	graph cfg.Graph,
	sources sourcevalue.SourceValues,
	in state.State,
	read func(cfg.Point) state.State,
) (product.Value, bool) {
	if reg == nil {
		return product.Value{}, false
	}
	exprPath, ok := facts.ExpressionPath(expr)
	if !ok || exprPath.Symbol == 0 || len(exprPath.Segments) != 0 {
		return product.Value{}, false
	}
	decl, ok := factquery.DominatingRootDeclarationSource(point, exprPath.Symbol, facts, graph)
	if !ok {
		return product.Value{}, false
	}
	declState := in
	if read != nil {
		declState = read(decl.Point)
	}
	if decl.Symbol != 0 {
		v := declState.ReadSymbolValue(reg, decl.Symbol)
		if !product.Equal(reg, v, product.Bottom(reg)) {
			return v, true
		}
	}
	return valueOfSource(decl.Point, decl.Source, sources, declState, read)
}

func valueOfSource(
	point cfg.Point,
	source factflow.ValueSource,
	sources sourcevalue.SourceValues,
	in state.State,
	read func(cfg.Point) state.State,
) (product.Value, bool) {
	if sources == nil {
		return product.Value{}, false
	}
	value, ok := sources.ValueOfSource(point, source, in, read)
	if !ok {
		return product.Value{}, false
	}
	return value, true
}

func callArgumentType(
	ctx transfer.NodeContext,
	source factflow.ValueSource,
	summaries summary.Reader,
	facts factflow.Facts,
	functionKeys map[symbol.ID]summary.SummaryKey,
	functionExpressionKeys map[factflow.ExprRef]summary.SummaryKey,
	functionTypes map[summary.SummaryKey]*typ.Function,
	sources sourcevalue.SourceValues,
	in state.State,
	read func(cfg.Point) state.State,
) (typ.Type, bool) {
	if t, ok := functionExpressionTypeFromSummary(ctx, source, summaries, facts, functionKeys, functionExpressionKeys, functionTypes); ok {
		return t, true
	}
	if sources == nil {
		return nil, false
	}
	value, ok := sourceValueAtArgument(ctx.Registry, ctx.Point, source, facts, ctx.Graph, sources, in, read)
	if !ok {
		return nil, false
	}
	t, ok := typeFromValue(ctx.Registry, value)
	if !ok || !UsableType(ctx.Registry, value, t) {
		return nil, false
	}
	return t, true
}

func UsableType(reg *axis.Registry, value product.Value, t typ.Type) bool {
	if reg == nil || t == nil || typ.IsAny(t) || typ.IsUnknown(t) || typ.IsNever(t) || refinement.ContainsFreeTypeParam(t) {
		return false
	}
	ev := product.Get(reg, value, evidence.Key)
	return !ev.IsExplicitTop() && !ev.IsGradualTop()
}

func functionExpressionTypeFromSummary(
	ctx transfer.NodeContext,
	source factflow.ValueSource,
	summaries summary.Reader,
	facts factflow.Facts,
	functionKeys map[symbol.ID]summary.SummaryKey,
	functionExpressionKeys map[factflow.ExprRef]summary.SummaryKey,
	functionTypes map[summary.SummaryKey]*typ.Function,
) (typ.Type, bool) {
	if !source.HasExpr || summaries == nil {
		return nil, false
	}
	key, ok := functionExpressionKeys[source.ExprRef]
	if !ok {
		functionSymbol, ok := facts.ExpressionFunction(source.ExprRef)
		if !ok || functionSymbol == 0 {
			return nil, false
		}
		key, ok = functionKeys[functionSymbol]
		if !ok {
			return nil, false
		}
	}
	fn := functionTypes[key]
	if fn == nil {
		return nil, false
	}
	sum, ok := summaries.Read(key)
	if !ok || len(sum.Returns) == 0 {
		return fn, true
	}
	out := functionTypeWithSummaryReturns(ctx.Registry, fn, sum.Returns)
	return out, true
}

func functionTypeWithSummaryReturns(reg *axis.Registry, fn *typ.Function, returns []product.Value) *typ.Function {
	if reg == nil || fn == nil || len(returns) == 0 {
		return fn
	}
	outReturns := make([]typ.Type, 0, len(returns))
	for _, ret := range returns {
		t, ok := typeFromValue(reg, ret)
		if !ok || t == nil || typ.IsAny(t) || typ.IsUnknown(t) || refinement.ContainsFreeTypeParam(t) {
			return fn
		}
		outReturns = append(outReturns, t)
	}
	if len(outReturns) == 0 {
		return fn
	}
	return typ.RebuildFunction(typ.FunctionParts{
		TypeParams: fn.TypeParams,
		Params:     fn.Params,
		Variadic:   fn.Variadic,
		Returns:    outReturns,
	})
}

func typeFromValue(reg *axis.Registry, value product.Value) (typ.Type, bool) {
	return typevalue.TypeOf(reg, value)
}

func specializeSummaryReturns(reg *axis.Registry, got summary.Summary, originalReturns []typ.Type, returns []typ.Type) summary.Summary {
	if reg == nil || len(returns) == 0 {
		return got
	}
	originalReturns = callResultReturnTypes(got, originalReturns)
	returns = callResultReturnTypes(got, returns)
	// A function whose body never returns normally (e.g. a stub `function f(): T
	// error("nyi") end`) has no summary return values, but its declared signature
	// is still the contract callers see. Size the slots to the declared returns and
	// adopt the declared type for any slot the body left empty.
	width := len(got.Returns)
	if len(returns) > width {
		width = len(returns)
	}
	nextReturns := make([]product.Value, width)
	copy(nextReturns, got.Returns)
	changed := width != len(got.Returns)
	for i := range nextReturns {
		if i >= len(returns) {
			break
		}
		ret := returns[i]
		if ret == nil || typ.IsAny(ret) || typ.IsUnknown(ret) || refinement.ContainsFreeTypeParam(ret) {
			continue
		}
		declared := typevalue.WithWitness(reg, typevalue.FromType(reg, ret), ret)
		if i >= len(got.Returns) {
			// No body return for this slot: adopt the declared return directly.
			nextReturns[i] = declared
			changed = true
			continue
		}
		next := joinInstantiatedReturnValue(reg, nextReturns[i], declared, originalReturnTypeAt(originalReturns, i))
		if product.Equal(reg, nextReturns[i], next) {
			continue
		}
		nextReturns[i] = next
		changed = true
	}
	if !changed {
		return got
	}
	got.Returns = nextReturns
	return summary.NormalizeOwned(reg, got)
}

func originalReturnTypeAt(returns []typ.Type, index int) typ.Type {
	if index < 0 || index >= len(returns) {
		return nil
	}
	return returns[index]
}

func callResultReturnTypes(got summary.Summary, returns []typ.Type) []typ.Type {
	if len(returns) == 1 && len(got.Returns) > 1 {
		if tuple, ok := returns[0].(*typ.Tuple); ok {
			return append([]typ.Type(nil), tuple.Elements...)
		}
	}
	return returns
}

func joinInstantiatedReturnValue(reg *axis.Registry, value product.Value, declared product.Value, original typ.Type) product.Value {
	if product.Equal(reg, value, product.Top()) {
		return declared
	}
	if refinement.ContainsFreeTypeParam(original) || valueContainsFreeTypeParam(reg, value) {
		return declared
	}
	if product.Equal(reg, value, product.Bottom(reg)) {
		return declared
	}
	ed := product.Edit(reg, value)
	ed.SetPresence(presence.Join(product.PresenceOf(value), product.PresenceOf(declared)))
	declaredKind := product.Get(reg, declared, runtimekind.Key)
	if !declaredKind.IsTop() {
		product.EditSet(&ed, runtimekind.Key, runtimekind.Join(product.Get(reg, value, runtimekind.Key), declaredKind))
	}
	declaredEvidence := product.Get(reg, declared, evidence.Key)
	if !evidence.Equal(declaredEvidence, evidence.Top()) {
		product.EditSet(&ed, evidence.Key, evidence.Join(product.Get(reg, value, evidence.Key), declaredEvidence))
	}
	declaredWitness := product.Get(reg, declared, typewitness.Key)
	if !declaredWitness.IsBottom() && !declaredWitness.IsTop() {
		product.EditSet(&ed, typewitness.Key, declaredWitness)
	}
	declaredOrigin := product.Get(reg, declared, variantorigin.Key)
	if !declaredOrigin.IsBottom() && !declaredOrigin.IsTop() {
		product.EditSet(&ed, variantorigin.Key, declaredOrigin)
	}
	return ed.Done()
}

func valueContainsFreeTypeParam(reg *axis.Registry, value product.Value) bool {
	t, ok := typevalue.TypeOf(reg, value)
	return ok && refinement.ContainsFreeTypeParam(t)
}

func clonePathMultiKeys(in map[pathdom.PathKey][]summary.SummaryKey) map[pathdom.PathKey][]summary.SummaryKey {
	if len(in) == 0 {
		return nil
	}
	out := make(map[pathdom.PathKey][]summary.SummaryKey, len(in))
	for pathKey, keys := range in {
		if len(keys) == 0 {
			continue
		}
		out[pathKey] = append([]summary.SummaryKey(nil), keys...)
	}
	return out
}

// padMissingResultsToNil fills nil for result slots a call site consumes beyond
// the callee's declared return arity. A callee declared to return declaredCount
// values yields nil for any further destructuring target, matching Lua runtime
// semantics. It applies only when the callee's declared return arity is known
// and finite (declaredCount comes from the resolved function type); an
// unresolved or unknown-arity callee never reaches here.
func padMissingResultsToNil(reg *axis.Registry, site factflow.CallSiteView, results []callpayload.CallResult, declaredCount int) []callpayload.CallResult {
	if reg == nil || declaredCount < 0 {
		return results
	}
	present := make(map[int]struct{}, len(results))
	for _, result := range results {
		present[result.Index] = struct{}{}
	}
	nilValue := typevalue.Nil(reg)
	site.ForEachResultTarget(func(target factflow.CallResultTargetView) bool {
		index := target.ResultIndex()
		if index < declaredCount {
			return true
		}
		if _, ok := present[index]; ok {
			return true
		}
		present[index] = struct{}{}
		results = append(results, callpayload.CallResult{Index: index, Value: nilValue})
		return true
	})
	return results
}

func outcomeFromSummary(
	got summary.Summary,
	usefulParamObligation func(int) bool,
	usefulNormalReturnParam func(int) bool,
) callpayload.CallOutcome {
	out := callpayload.CallOutcome{}
	if len(got.Returns) != 0 {
		out.Results = make([]callpayload.CallResult, len(got.Returns))
		for i, value := range got.Returns {
			out.Results[i] = callpayload.CallResult{Index: i, Value: value}
		}
	}
	for i, value := range got.ParamObligations {
		if usefulParamObligation == nil || !usefulParamObligation(i) {
			continue
		}
		out.ParamObligations = append(out.ParamObligations, callpayload.CallParamObligation{
			ParamIndex: i,
			Value:      value,
		})
	}
	for i, value := range got.NormalReturnParams {
		if usefulNormalReturnParam == nil || !usefulNormalReturnParam(i) {
			continue
		}
		out.ParamPathRefinements = append(out.ParamPathRefinements, callpayload.CallParamPathRefinement{
			Path:  pathdom.NewPlaceholder(i),
			Value: value,
		})
	}
	for i, condition := range got.NormalReturnParamConditions {
		value, ok := paramConditionValue(condition)
		if !ok {
			continue
		}
		out.ParamConditions = append(out.ParamConditions, callpayload.CallParamCondition{
			ParamIndex: i,
			Value:      value,
		})
	}
	for _, equality := range got.NormalReturnParamEqualities {
		if equality.Left < 0 || equality.Right < 0 || equality.Left == equality.Right {
			continue
		}
		out.ParamPathRelations = append(out.ParamPathRelations, callpayload.CallParamPathRelation{
			Kind:  callpayload.CallPathRelationEqual,
			Left:  pathdom.NewPlaceholder(equality.Left),
			Right: pathdom.NewPlaceholder(equality.Right),
		})
	}
	out.NormalReturnFacts = summary.CloneNormalReturnFacts(got.NormalReturnFacts)
	// outcomeFromSummary only receives caller-owned summaries. Passing the heap
	// map through avoids cloning the snapshot-read copy again.
	out.HeapTableObjects = got.HeapTableObjects
	if len(got.ReturnConditionParamRefinements) != 0 {
		out.ReturnConditionRefinements = make([]callpayload.CallReturnConditionRefinement, len(got.ReturnConditionParamRefinements))
		for i, refinement := range got.ReturnConditionParamRefinements {
			out.ReturnConditionRefinements[i] = callpayload.CallReturnConditionRefinement{
				ReturnIndex: refinement.ReturnIndex,
				ReturnValue: refinement.ReturnValue,
				Target:      refinement.Target.Clone(),
				Value:       refinement.Value,
			}
		}
	}
	if len(got.ReturnPresenceRelations) != 0 {
		out.ReturnPresenceRelations = make([]callpayload.CallReturnPresenceRelation, len(got.ReturnPresenceRelations))
		for i, relation := range got.ReturnPresenceRelations {
			out.ReturnPresenceRelations[i] = callpayload.CallReturnPresenceRelation{
				TriggerIndex:    relation.TriggerIndex,
				TriggerPresence: relation.TriggerPresence,
				TargetIndex:     relation.TargetIndex,
				TargetPresence:  relation.TargetPresence,
			}
		}
	}
	return out
}

func paramMemberReturnPresenceRelations(
	ctx transfer.NodeContext,
	site factflow.CallSiteView,
	got summary.Summary,
	facts factflow.Facts,
	returnPresenceRelations ReturnPresenceRelationsForPathFunc,
) []callpayload.CallReturnPresenceRelation {
	if returnPresenceRelations == nil || len(got.ParamMemberReturnSlots) == 0 {
		return nil
	}
	type memberKey struct {
		receiver int
		member   segment.Segment
	}
	argCount := site.ArgumentSourceCount()
	slotsByMember := make(map[memberKey]map[int]int)
	for _, slot := range got.ParamMemberReturnSlots {
		if slot.ReceiverParam < 0 || slot.ReceiverParam >= argCount ||
			!memberaccess.Valid(slot.Member) || slot.ReturnIndex < 0 || slot.MemberResultIndex < 0 {
			continue
		}
		key := memberKey{receiver: slot.ReceiverParam, member: slot.Member}
		slots := slotsByMember[key]
		if slots == nil {
			slots = make(map[int]int)
			slotsByMember[key] = slots
		}
		slots[slot.MemberResultIndex] = slot.ReturnIndex
	}
	if len(slotsByMember) == 0 {
		return nil
	}
	var out []callpayload.CallReturnPresenceRelation
	for key, slots := range slotsByMember {
		source, ok := site.ArgumentSourceAt(key.receiver)
		if !ok {
			continue
		}
		memberPaths := argumentMemberPaths(facts, source, key.member)
		if len(memberPaths) == 0 {
			continue
		}
		for _, memberPath := range memberPaths {
			for _, relation := range returnPresenceRelations(ctx.Point, memberPath) {
				trigger, ok := slots[relation.TriggerIndex]
				if !ok {
					continue
				}
				target, ok := slots[relation.TargetIndex]
				if !ok {
					continue
				}
				out = append(out, callpayload.CallReturnPresenceRelation{
					TriggerIndex:    trigger,
					TriggerPresence: relation.TriggerPresence,
					TargetIndex:     target,
					TargetPresence:  relation.TargetPresence,
				})
			}
		}
	}
	return out
}

func argumentMemberPaths(facts factflow.Facts, source factflow.ValueSource, member segment.Segment) []pathdom.Path {
	if !memberaccess.Valid(member) || source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
		return nil
	}
	p, ok := facts.ExpressionPath(source.ExprRef)
	if !ok || p.IsEmpty() {
		return nil
	}
	return memberaccess.Paths(p, member)
}

func memberCallParamObligations(
	ctx transfer.NodeContext,
	site factflow.CallSiteView,
	got summary.Summary,
	sources sourcevalue.SourceValues,
	in state.State,
	read func(cfg.Point) state.State,
) []callpayload.CallParamObligation {
	if ctx.Registry == nil || sources == nil || len(got.ParamMemberCallObligations) == 0 {
		return nil
	}
	argCount := site.ArgumentSourceCount()
	if argCount == 0 {
		return nil
	}
	var out []callpayload.CallParamObligation
	for _, obligation := range got.ParamMemberCallObligations {
		if obligation.ReceiverParam < 0 || obligation.ReceiverParam >= argCount ||
			obligation.ArgParam < 0 || obligation.ArgParam >= argCount ||
			obligation.MemberParamIndex < 0 || !memberaccess.Valid(obligation.Member) {
			continue
		}
		receiverSource, ok := site.ArgumentSourceAt(obligation.ReceiverParam)
		if !ok {
			continue
		}
		receiverValue, ok := sources.ValueOfSource(ctx.Point, receiverSource, in, read)
		if !ok {
			continue
		}
		receiverType, ok := typeFromValue(ctx.Registry, receiverValue)
		if !ok {
			continue
		}
		receiverType, ok = projectMemberObligationReceiver(receiverType, obligation.ReceiverPath)
		if !ok {
			continue
		}
		callable, status, ok := memberaccess.Callable(receiverType, obligation.Member)
		if status != typecall.MemberCallOK || !ok || callable == nil || obligation.MemberParamIndex >= len(callable.Params) {
			continue
		}
		want := callable.Params[obligation.MemberParamIndex].Type
		if want == nil || typ.IsAny(want) || typ.IsUnknown(want) || refinement.ContainsFreeTypeParam(want) {
			continue
		}
		value := typevalue.WithWitness(ctx.Registry, typevalue.FromType(ctx.Registry, want), want)
		if !summary.UsefulParamObligation(ctx.Registry, value) {
			continue
		}
		out = append(out, callpayload.CallParamObligation{
			ParamIndex: obligation.ArgParam,
			Value:      value,
			Origin: callpayload.CallParamObligationOrigin{
				HasOrigin:        true,
				ReceiverParam:    obligation.ReceiverParam,
				ReceiverPath:     obligation.ReceiverPath,
				Member:           obligation.Member,
				ArgParam:         obligation.ArgParam,
				MemberParamIndex: obligation.MemberParamIndex,
			},
		})
	}
	return out
}

func projectMemberObligationReceiver(receiver typ.Type, receiverPath pathdom.PathKey) (typ.Type, bool) {
	if receiver == nil {
		return nil, false
	}
	if receiverPath == "" {
		return receiver, true
	}
	segments, ok := segment.ParseFormattedSegments(string(receiverPath))
	if !ok {
		return nil, false
	}
	return luatypeprojection.ApplySegments(receiver, segments)
}

// paramReturnExposures lowers return-param aliasing into covariant call-boundary
// exposures. For each ReturnParamPathAlias the callee records (a parameter stored
// into a returned container slot), it emits an exposure whose contract is the
// callee's declared return type projected at the aliased return slot member. That
// projected member type, not the parameter's own declared type, is the sound
// contract: a callee that covariantly stores a narrow parameter into a wider
// return field exposes the argument object at the wider field type, so a write
// through the caller's returned view can launder a wider value back into the
// argument. The caller eager-widens the argument's source object toward this
// contract, mirroring the in-body covariant mutable-view exposure.
func paramReturnExposures(reg *axis.Registry, argCount int, got summary.Summary, fn *typ.Function) []callpayload.CallParamExposure {
	if reg == nil || fn == nil || len(got.ReturnParamPathAliases) == 0 {
		return nil
	}
	returns := callResultReturnTypes(got, fn.Returns)
	var out []callpayload.CallParamExposure
	for _, alias := range got.ReturnParamPathAliases {
		paramIndex, ok := rootPlaceholderIndex(alias.Source)
		if !ok || paramIndex < 0 || paramIndex >= argCount {
			continue
		}
		if alias.ReturnIndex < 0 || alias.ReturnIndex >= len(returns) {
			continue
		}
		returnType := returns[alias.ReturnIndex]
		if returnType == nil {
			continue
		}
		contract := returnType
		if alias.Member != "" {
			memberSegments, ok := pathaddr.RelativeStaticMemberSuffixSegments(alias.Member)
			if !ok || len(memberSegments) == 0 {
				continue
			}
			contract, ok = luatypeprojection.ApplySegments(returnType, memberSegments)
			if !ok {
				continue
			}
		}
		exposure, ok := newParamExposure(reg, pathdom.NewPlaceholder(paramIndex), contract)
		if !ok {
			continue
		}
		out = append(out, exposure)
	}
	return out
}

// paramStoreRelationExposures lowers param-to-param store relations into covariant
// call-boundary exposures (Route 1). Each StoreRelationFact records that the
// callee stores one parameter (Source, a bare placeholder) into a member slot of
// another parameter (Into, a placeholder with member segments). The destination
// slot type, not the source parameter's own declared type, is the sound contract:
// a callee that covariantly stores a narrow parameter into a wider destination
// slot exposes the argument object at the wider slot type, so a write through the
// caller's destination view can launder a wider value back into the source
// argument. The contract is the destination parameter's declared type projected at
// the store member, and the caller eager-widens the source argument toward it.
func paramStoreRelationExposures(reg *axis.Registry, argCount int, got summary.Summary, fn *typ.Function) []callpayload.CallParamExposure {
	if reg == nil || fn == nil || len(got.NormalReturnFacts.StoreRelations) == 0 {
		return nil
	}
	var out []callpayload.CallParamExposure
	for _, relation := range got.NormalReturnFacts.StoreRelations {
		if !relation.Source.IsPlaceholder() || len(relation.Source.Segments) != 0 {
			continue
		}
		sourceIndex := relation.Source.PlaceholderIndex()
		if sourceIndex < 0 || sourceIndex >= argCount {
			continue
		}
		if !relation.Into.IsPlaceholder() || len(relation.Into.Segments) == 0 {
			continue
		}
		intoIndex := relation.Into.PlaceholderIndex()
		if intoIndex < 0 || intoIndex >= len(fn.Params) {
			continue
		}
		destType := fn.Params[intoIndex].Type
		if destType == nil {
			continue
		}
		contract, ok := luatypeprojection.ApplySegments(destType, relation.Into.Segments)
		if !ok {
			continue
		}
		exposure, ok := newParamExposure(reg, pathdom.NewPlaceholder(sourceIndex), contract)
		if !ok {
			continue
		}
		out = append(out, exposure)
	}
	return out
}

// paramSinkExposures lowers param-to-sink store exposures into covariant
// call-boundary exposures (Route 2). Each ParamSinkExposure records that the
// callee stores one parameter (Source, a bare placeholder) into a member slot of a
// persistent sink (a captured upvalue or a global) the caller cannot track writes
// back through. The carried Contract is the sink's slot type, computed in-body
// where the sink's container type is available: it is the real exposure type,
// since a covariant store of a narrow parameter into a wider sink slot is
// well-typed and a later write through the sink launders a wider value back into
// the argument. The caller eager-widens the source argument toward the carried
// slot type.
func paramSinkExposures(reg *axis.Registry, argCount int, got summary.Summary) []callpayload.CallParamExposure {
	if reg == nil || len(got.ParamSinkExposures) == 0 {
		return nil
	}
	var out []callpayload.CallParamExposure
	for _, sink := range got.ParamSinkExposures {
		paramIndex, ok := rootPlaceholderIndex(sink.Source)
		if !ok || paramIndex < 0 || paramIndex >= argCount {
			continue
		}
		contract, ok := typevalue.TypeOf(reg, sink.Contract)
		if !ok {
			continue
		}
		exposure, ok := newParamExposure(reg, pathdom.NewPlaceholder(paramIndex), contract)
		if !ok {
			continue
		}
		out = append(out, exposure)
	}
	return out
}

// newParamExposure builds a unified call-boundary exposure for a callee-relative
// source placeholder toward a destination-slot contract type. It gates on a
// mutable record/array contract and carries the contract's witness-bearing value.
func newParamExposure(reg *axis.Registry, source pathdom.Path, contract typ.Type) (callpayload.CallParamExposure, bool) {
	if contract == nil || typ.IsAny(contract) || typ.IsUnknown(contract) || refinement.ContainsFreeTypeParam(contract) {
		return callpayload.CallParamExposure{}, false
	}
	kind, ok := covariantExposureKind(contract)
	if !ok {
		return callpayload.CallParamExposure{}, false
	}
	value := typevalue.WithWitness(reg, typevalue.FromType(reg, contract), contract)
	return callpayload.CallParamExposure{
		Source:   source,
		Contract: value,
		Kind:     kind,
	}, true
}

// covariantExposureKind selects the widening kind for a mutable container
// contract: an opaque-array element widen for an array, a record field rebuild
// for a record. Any other shape is not a mutable container view and emits no
// exposure. The lowering twin transferfacts.covariantExposureKind must classify
// identically; the layered architecture keeps factflow type-independent and this
// package's imports bounded, so the two cannot share one helper.
func covariantExposureKind(contract typ.Type) (factflow.CovariantExposureKind, bool) {
	switch unaliasType(contract).(type) {
	case *typ.Array:
		return factflow.CovariantExposureArray, true
	case *typ.Record:
		return factflow.CovariantExposureRecord, true
	default:
		return 0, false
	}
}

func unaliasType(t typ.Type) typ.Type {
	for {
		alias, ok := t.(*typ.Alias)
		if !ok || alias == nil {
			return t
		}
		t = alias.UnaliasedTarget()
	}
}

// rootPlaceholderIndex returns the parameter index of a root placeholder source
// key ($i with no member segments). Sources with member segments name a sub-path
// of a parameter, which this exposure lane does not handle.
func rootPlaceholderIndex(source pathdom.PathKey) (int, bool) {
	p, ok := pathaddr.PlaceholderPathFromKey(source)
	if !ok || len(p.Segments) != 0 {
		return 0, false
	}
	index := p.PlaceholderIndex()
	if index < 0 {
		return 0, false
	}
	return index, true
}

func functionTypeParamObligations(reg *axis.Registry, argCount int, fn *typ.Function) []callpayload.CallParamObligation {
	if reg == nil || fn == nil || len(fn.Params) == 0 {
		return nil
	}
	limit := argCount
	if limit > len(fn.Params) {
		limit = len(fn.Params)
	}
	var out []callpayload.CallParamObligation
	for i := 0; i < limit; i++ {
		want := fn.Params[i].Type
		if want == nil || typ.IsAny(want) || typ.IsUnknown(want) || refinement.ContainsFreeTypeParam(want) {
			continue
		}
		value := typevalue.WithWitness(reg, typevalue.FromType(reg, want), want)
		if !summary.UsefulParamObligation(reg, value) {
			continue
		}
		out = append(out, callpayload.CallParamObligation{
			ParamIndex: i,
			Value:      value,
		})
	}
	return out
}

func paramConditionValue(condition summary.ParamCondition) (bool, bool) {
	switch condition {
	case summary.ParamConditionTruthy:
		return true, true
	case summary.ParamConditionFalsy:
		return false, true
	default:
		return false, false
	}
}

// ByCalleeIdentity maps direct callee symbols to summary keys. Mutable callee
// paths are intentionally not resolved here; path calls must go through current
// value identity so reassignments and non-dominating writes stay sound.
func ByCalleeIdentity(symbolKeys map[symbol.ID]summary.SummaryKey) KeyFunc {
	clonedSymbols := make(map[symbol.ID]summary.SummaryKey, len(symbolKeys))
	for id, key := range symbolKeys {
		clonedSymbols[id] = key
	}
	return func(_ transfer.NodeContext, site factflow.CallSiteView) (summary.SummaryKey, bool) {
		if key, ok := clonedSymbols[site.CalleeSymbol()]; ok {
			return key, true
		}
		return summary.SummaryKey{}, false
	}
}
