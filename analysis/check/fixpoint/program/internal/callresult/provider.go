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
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/proof"
	valuerefinement "github.com/wippyai/go-lua/analysis/domain/value/refinement"
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
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	luatypeprojection "github.com/wippyai/go-lua/analysis/lua/typeprojection"
	"github.com/wippyai/go-lua/analysis/symbol"
	typenormalize "github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/refinement"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typecall"
)

// KeyFunc maps one call site in context to an exact summary key.
type KeyFunc func(ctx transfer.NodeContext, site factflow.CallSiteView) (summary.SummaryKey, bool)

// CalleeValueFunc resolves the current callee expression value at a call site.
type CalleeValueFunc func(ctx transfer.NodeContext, site factflow.CallSiteView, in state.State, read func(cfg.Point) state.State) (product.Value, bool)

// ReceiverCallableFunc resolves a callable method from language-owned receiver
// declaration evidence when flow has lost the receiver's precise contract but
// the receiver root has not been replaced.
type ReceiverCallableFunc func(ctx transfer.NodeContext, site factflow.CallSiteView) (*typ.Function, bool)

// ReturnPresenceRelationsForPathFunc resolves an in-scope access path to
// lowered return-presence relations, when the path has a stable imported/global
// signature identity.
type ReturnPresenceRelationsForPathFunc func(point cfg.Point, p pathdom.Path) []callpayload.CallReturnPresenceRelation

// ProviderConfig configures summary-backed call outcomes.
type ProviderConfig struct {
	Summaries               summary.Reader
	ContextKeyFor           KeyFunc
	KeyFor                  KeyFunc
	CalleeValue             CalleeValueFunc
	ReceiverCallable        ReceiverCallableFunc
	Facts                   factflow.Facts
	Index                   *SummaryIndex
	FunctionKeys            map[symbol.ID]summary.SummaryKey
	FunctionExpressionKeys  map[factflow.ExprRef]summary.SummaryKey
	FunctionIDs             map[identity.ID]summary.SummaryKey
	PathKeys                map[factflow.CalleePathKey]summary.SummaryKey
	PathMultiKeys           map[factflow.CalleePathKey][]summary.SummaryKey
	FunctionTypes           map[summary.SummaryKey]*typ.Function
	Sources                 sourcevalue.SourceValues
	ReturnPresenceRelations ReturnPresenceRelationsForPathFunc
	// KeySpace is the consuming (caller) analysis keyspace. Summaries read at a
	// call site carry heap objects interned under the callee's keyspace; they are
	// rebased into this keyspace before any heap member is read or written.
	KeySpace *keyspace.KeySpace
	// TypeValues is the caller-owned query cache for pure type-to-value
	// materialization. Summary application may rebuild the same declared return
	// and obligation values at many call sites; keeping this surface explicit
	// avoids hidden package globals while preserving canonical products.
	TypeValues *typevalue.Cache
}

// OutcomeProvider returns a generic call-boundary outcome provider backed by
// exact summary reads.
func OutcomeProvider(config ProviderConfig) callpayload.CallOutcomeProvider {
	summaries := config.Summaries
	contextKeyFor := config.ContextKeyFor
	keyFor := config.KeyFor
	calleeValue := config.CalleeValue
	receiverCallable := config.ReceiverCallable
	facts := config.Facts
	index := summaryIndexFromProviderConfig(config)
	functionKeys := index.functionKeys
	functionExpressionKeys := index.functionExpressionKeys
	functionIDs := index.functionIDs
	pathKeys := index.pathKeys
	pathMultiKeys := index.pathMultiKeys
	functionTypes := index.functionTypes
	sources := config.Sources
	returnPresenceRelations := config.ReturnPresenceRelations
	callerKeySpace := config.KeySpace
	typeValues := config.TypeValues
	preparedSummaries := providerPreparedSummaryCache{
		summaries:      summaries,
		functionTypes:  functionTypes,
		callerKeySpace: callerKeySpace,
	}
	return func(ctx transfer.NodeContext, site factflow.CallSiteView, in state.State, read func(cfg.Point) state.State) callpayload.CallOutcome {
		if summaries == nil {
			return callpayload.CallOutcome{}
		}
		var got summary.Summary
		var fn *typ.Function
		summaryOwned := false
		currentIdentityKey, hasCurrentIdentityKey := currentFunctionIdentityKey(ctx, site, in, read, calleeValue, functionIDs)
		matchedContext := false
		matchedContextKey := summary.SummaryKey{}
		readSummary := func(key summary.SummaryKey) bool {
			var ok bool
			got, fn, ok = preparedSummaries.read(ctx.Registry, typeValues, key)
			if !ok {
				return false
			}
			summaryOwned = true
			if summaryNeedsGenericInstantiation(ctx, fn, sources) {
				got = specializeGenericSummary(ctx, site, got, fn, summaries, facts, functionKeys, functionExpressionKeys, functionTypes, sources, in, read, typeValues)
			}
			got = materializeReturnRootTypesFromFacts(ctx.Registry, typeValues, got)
			return true
		}
		if contextKeyFor != nil {
			if key, ok := contextKeyFor(ctx, site); ok &&
				contextKeyMatchesCurrentIdentity(key, currentIdentityKey, hasCurrentIdentityKey) &&
				readSummary(key) {
				matchedContext = true
				matchedContextKey = key
				goto matched
			}
		}
		if hasCurrentIdentityKey {
			if !readSummary(currentIdentityKey) {
				return callpayload.CallOutcome{}
			}
			goto matched
		}
		if keyFor != nil {
			if key, ok := keyFor(ctx, site); ok {
				if !readSummary(key) {
					return callpayload.CallOutcome{}
				}
				goto matched
			}
		}
		if key, ok := summaryKeyForDefinitionPath(ctx, site, in, read, calleeValue, pathKeys); ok {
			if !readSummary(key) {
				return callpayload.CallOutcome{}
			}
			goto matched
		}
		if joined, joinedOK := joinedSummaryForDefinitionPath(ctx, site, in, read, calleeValue, summaries, &preparedSummaries, pathMultiKeys, functionTypes, facts, functionKeys, functionExpressionKeys, sources, typeValues); joinedOK {
			got = joined
		} else {
			if out, ok := unresolvedFunctionCallOutcome(ctx, site, in, read, calleeValue); ok {
				return refineOutcomeResultsFromCurrentCallable(ctx, site, sources, in, read, calleeValue, receiverCallable, typeValues, out)
			}
			return callpayload.CallOutcome{}
		}
	matched:
		if matchedContext {
			got = inheritSameIdentityContextBaseSummary(ctx.Registry, typeValues, &preparedSummaries, currentIdentityKey, hasCurrentIdentityKey, matchedContextKey, got)
		}
		if len(got.ReturnParamPathAliases) != 0 {
			if summaryOwned {
				got = got.Clone()
				summaryOwned = false
			}
			got = materializeReturnParamPathAliases(ctx, callerKeySpace, site, got, sources, in, read, typeValues)
		}
		if summaryOwned && len(got.HeapTableObjects) != 0 {
			got.HeapTableObjects = heapidentity.CloneMap(got.HeapTableObjects)
		}
		summaryParamOffset := summaryParamObligationOffset(ctx, site, fn, in, read, calleeValue)
		out := outcomeFromSummary(ctx.Registry, got, summaryParamOffset, func(index int) bool {
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
		out = refineOutcomeResultsFromCurrentCallable(ctx, site, sources, in, read, calleeValue, receiverCallable, typeValues, out)
		if returnPresenceRelations != nil && len(got.ParamMemberReturnSlots) != 0 {
			out.ReturnPresenceRelations = append(
				out.ReturnPresenceRelations,
				paramMemberReturnPresenceRelations(ctx, site, got, facts, returnPresenceRelations)...,
			)
		}
		if fn != nil && len(got.ReturnParamPathAliases) != 0 {
			out.ParamExposures = append(out.ParamExposures, paramReturnExposures(ctx.Registry, typeValues, site.ArgumentSourceCount(), got, fn)...)
		}
		if fn != nil && len(got.NormalReturnFacts.StoreRelations) != 0 {
			out.ParamExposures = append(out.ParamExposures, paramStoreRelationExposures(ctx.Registry, typeValues, site.ArgumentSourceCount(), got, fn)...)
		}
		if fn != nil && len(got.NormalReturnFacts.PathInvalidations) != 0 {
			out.ParamExposures = append(out.ParamExposures, paramDirectMutationExposures(ctx.Registry, typeValues, site.ArgumentSourceCount(), got, fn)...)
		}
		if len(got.ParamSinkExposures) != 0 {
			out.ParamExposures = append(out.ParamExposures, paramSinkExposures(ctx.Registry, typeValues, site.ArgumentSourceCount(), got)...)
		}
		if fn != nil {
			out.Results = padMissingResultsToNil(ctx.Registry, site, out.Results, len(fn.Returns))
		}
		out.PostReturnAuthority = calloutcome.HasAuthoritativePostReturnEvidence(ctx.Registry, out)
		if fn != nil && len(fn.Params) != 0 {
			out.ParamObligations = append(out.ParamObligations, functionTypeParamObligationsForSite(ctx, typeValues, site, fn, sources, in, read)...)
		}
		if len(got.ParamMemberCallObligations) != 0 {
			out.ParamObligations = append(out.ParamObligations, memberCallParamObligations(ctx, site, got, sources, in, read, typeValues)...)
		}
		return out
	}
}

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

func materializeReturnRootTypesFromFacts(reg *axis.Registry, typeValues *typevalue.Cache, sum summary.Summary) summary.Summary {
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
	returnsOwned := false
	if len(sum.Returns) <= maxIndex {
		expanded := make([]product.Value, maxIndex+1)
		copy(expanded, sum.Returns)
		sum.Returns = expanded
		returnsOwned = true
	}
	ensureReturnsOwned := func() {
		if returnsOwned {
			return
		}
		next := make([]product.Value, len(sum.Returns))
		copy(next, sum.Returns)
		sum.Returns = next
		returnsOwned = true
	}
	for index := 0; index <= maxIndex; index++ {
		factType, ok := returnRecordTypeFromFacts(reg, sum.NormalReturnFacts, index)
		if !ok {
			continue
		}
		t := factType
		if existing, existingOK := typevalue.TypeOf(reg, sum.Returns[index]); existingOK {
			merged, mergedOK := mergeReturnRecordFactType(existing, factType)
			if !mergedOK {
				continue
			}
			t = merged
		} else if !returnSlotNeedsFactType(reg, sum.Returns[index]) {
			continue
		}
		next := typeWitnessValue(reg, typeValues, t)
		if product.Equal(reg, sum.Returns[index], next) {
			continue
		}
		ensureReturnsOwned()
		sum.Returns[index] = next
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

func mergeReturnRecordFactType(existing typ.Type, factType typ.Type) (typ.Type, bool) {
	if existing == nil {
		return factType, true
	}
	base, ok := existing.(*typ.Record)
	if !ok {
		return nil, false
	}
	facts, ok := factType.(*typ.Record)
	if !ok {
		return nil, false
	}
	parts := typ.RecordParts{
		Fields:        append([]typ.Field(nil), base.Fields...),
		StaticMembers: append([]typ.StaticMember(nil), base.StaticMembers...),
		Metatable:     base.Metatable,
		MapKey:        base.MapKey,
		MapValue:      base.MapValue,
		Open:          base.Open,
	}
	for _, field := range facts.Fields {
		parts.Fields = upsertReturnRecordField(parts.Fields, field)
	}
	for _, member := range facts.StaticMembers {
		parts.StaticMembers = upsertReturnRecordStaticMember(parts.StaticMembers, member)
	}
	return typetable.RebuildRecord(parts), true
}

func upsertReturnRecordField(fields []typ.Field, field typ.Field) []typ.Field {
	for i := range fields {
		if fields[i].Name == field.Name {
			fields[i] = field
			return fields
		}
	}
	return append(fields, field)
}

func upsertReturnRecordStaticMember(members []typ.StaticMember, member typ.StaticMember) []typ.StaticMember {
	for i := range members {
		if typ.CompareStaticMembers(members[i], member) == 0 {
			members[i] = member
			return members
		}
	}
	return append(members, member)
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
	typeValues *typevalue.Cache,
) summary.Summary {
	if ctx.Registry == nil || sources == nil || len(got.ReturnParamPathAliases) == 0 {
		return got
	}
	objects := got.HeapTableObjects
	changed := false
	for _, alias := range got.ReturnParamPathAliases {
		value, ok := returnParamAliasSourceValue(ctx, ks, site, alias.Source, sources, in, read)
		if !ok {
			continue
		}
		if alias.Member == "" {
			if writeDirectReturnParamAliasValue(ctx.Registry, got.Returns, alias.ReturnIndex, value) {
				changed = true
			}
			continue
		}
		if len(objects) == 0 {
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

func writeDirectReturnParamAliasValue(reg *axis.Registry, returns []product.Value, returnIndex int, value product.Value) bool {
	if reg == nil || returnIndex < 0 || returnIndex >= len(returns) {
		return false
	}
	current := returns[returnIndex]
	var next product.Value
	if product.Equal(reg, current, product.Bottom(reg)) || product.Equal(reg, current, product.Top()) {
		next = value
	} else if product.Equal(reg, current, value) {
		return false
	} else if valuerefinement.DeclaredContractAlreadySatisfied(reg, value, current) {
		next = value
	} else if mergedPresence, ok := typevalue.DeclaredTypeFactsPresenceOnly(reg, value, current); ok {
		next = product.WithPresence(reg, value, mergedPresence)
	} else {
		next = valuerefinement.MergeDeclaredContract(reg, value, current)
		if currentPresence := product.PresenceOf(current); !presence.Equal(product.PresenceOf(next), currentPresence) {
			next = product.WithPresence(reg, next, currentPresence)
		}
	}
	if product.Equal(reg, current, next) {
		return false
	}
	returns[returnIndex] = next
	return true
}

func returnParamAliasSourceValue(
	ctx transfer.NodeContext,
	ks *keyspace.KeySpace,
	site factflow.CallSiteView,
	sourceKey pathaddr.PlaceholderKey,
	sources sourcevalue.SourceValues,
	in state.State,
	read func(cfg.Point) state.State,
) (product.Value, bool) {
	sourcePath, ok := sourceKey.Path()
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
	sum.NormalReturnFacts = sum.NormalReturnFacts.DropFactsTouchingPaths(func(p pathdom.Path) bool {
		return strictPlaceholderDescendant(p, maybeAbsent)
	})
	return sum
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
	results := make([]callpayload.CallResult, 0, len(fn.Returns))
	for i, ret := range fn.Returns {
		if ret == nil || typ.IsAny(ret) || typ.IsUnknown(ret) || refinement.ContainsFreeTypeParam(ret) {
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
	return calloutcome.MergeSupplemental(ctx.Registry, out, callpayload.CallOutcome{Results: results})
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
	if first.Name == "self" {
		return true
	}
	receiverType := fallback
	if receiverType == nil {
		receiverType, _ = methodReceiverType(ctx, site, sources, in, read)
	}
	return typecall.ParamConsumesReceiver(first.Name, first.Type, receiverType)
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
	contextKeyFor KeyFunc,
	keyFor KeyFunc,
	calleeValue CalleeValueFunc,
	functionIDs map[identity.ID]summary.SummaryKey,
	pathKeys map[factflow.CalleePathKey]summary.SummaryKey,
) (summary.SummaryKey, bool) {
	if contextKeyFor != nil {
		if key, ok := contextKeyFor(ctx, site); ok {
			return key, true
		}
	}
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

// SummaryArgumentTypeProvider resolves call argument types using summary-backed
// function expression identities, then falls back to generic source-value
// projection. It is used by public signature lowering when imported generic
// calls receive local callbacks whose return types are known only after the
// program fixed point.
func SummaryArgumentTypeProvider(config ProviderConfig) func(transfer.NodeContext, factflow.ValueSource, state.State, func(cfg.Point) state.State) (typ.Type, bool) {
	summaries := config.Summaries
	facts := config.Facts
	index := summaryIndexFromProviderConfig(config)
	functionKeys := index.functionKeys
	functionExpressionKeys := index.functionExpressionKeys
	functionTypes := index.functionTypes
	sources := config.Sources
	return func(ctx transfer.NodeContext, source factflow.ValueSource, in state.State, read func(cfg.Point) state.State) (typ.Type, bool) {
		rootDeclarations := rootDeclarationQueryProvider(facts, ctx.Graph)
		return callArgumentType(ctx, source, summaries, facts, rootDeclarations, functionKeys, functionExpressionKeys, functionTypes, sources, in, read)
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
	typeValues *typevalue.Cache,
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
	return specializeSummaryReturns(ctx.Registry, typeValues, got, fn.Returns, instantiated.Returns)
}

func summaryNeedsGenericInstantiation(ctx transfer.NodeContext, fn *typ.Function, sources sourcevalue.SourceValues) bool {
	return ctx.Registry != nil && fn != nil && len(fn.TypeParams) != 0 && sources != nil
}

func applyDeclaredSummaryReturns(reg *axis.Registry, typeValues *typevalue.Cache, got summary.Summary, fn *typ.Function) summary.Summary {
	if reg == nil || fn == nil || len(fn.TypeParams) != 0 || len(fn.Returns) == 0 {
		return got
	}
	return specializeSummaryReturns(reg, typeValues, got, fn.Returns, fn.Returns)
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
	rootDeclarations := rootDeclarationQueryProvider(facts, ctx.Graph)
	var args []typ.Type
	for i := 0; i < argCount; i++ {
		source, ok := site.ArgumentSourceAt(i)
		if !ok {
			continue
		}
		t, ok := callArgumentType(ctx, source, summaries, facts, rootDeclarations, functionKeys, functionExpressionKeys, functionTypes, sources, in, read)
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

func rootDeclarationQueryProvider(
	facts factflow.Facts,
	graph cfg.Graph,
) func() factquery.RootDeclarationQuery {
	var query factquery.RootDeclarationQuery
	var ready bool
	return func() factquery.RootDeclarationQuery {
		if !ready {
			query = factquery.NewRootDeclarationQuery(facts, graph)
			ready = true
		}
		return query
	}
}

func sourceValueAtArgument(
	reg *axis.Registry,
	point cfg.Point,
	source factflow.ValueSource,
	facts factflow.Facts,
	rootDeclarations func() factquery.RootDeclarationQuery,
	sources sourcevalue.SourceValues,
	in state.State,
	read func(cfg.Point) state.State,
) (product.Value, bool) {
	value, ok := valueOfSource(point, source, sources, in, read)
	if t, okType := inferenceTypeFromValue(reg, value); okType && UsableType(reg, value, t) {
		return value, true
	}
	if source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
		return value, ok
	}
	if value, ok := valueFromRootDeclarationSource(reg, point, source.ExprRef, facts, rootDeclarations, sources, in, read); ok {
		return value, true
	}
	return value, ok
}

func valueFromRootDeclarationSource(
	reg *axis.Registry,
	point cfg.Point,
	expr factflow.ExprRef,
	facts factflow.Facts,
	rootDeclarations func() factquery.RootDeclarationQuery,
	sources sourcevalue.SourceValues,
	in state.State,
	read func(cfg.Point) state.State,
) (product.Value, bool) {
	if reg == nil {
		return product.Value{}, false
	}
	exprPath, ok := facts.ExpressionPathRef(expr)
	if !ok || exprPath.Symbol == 0 || len(exprPath.Segments) != 0 {
		return product.Value{}, false
	}
	if rootDeclarations == nil {
		return product.Value{}, false
	}
	decl, ok := rootDeclarations().DominatingRootDeclarationSource(point, exprPath.Symbol)
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
	rootDeclarations func() factquery.RootDeclarationQuery,
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
	value, ok := sourceValueAtArgument(ctx.Registry, ctx.Point, source, facts, rootDeclarations, sources, in, read)
	if !ok {
		return nil, false
	}
	t, ok := inferenceTypeFromValue(ctx.Registry, value)
	if !ok || !UsableType(ctx.Registry, value, t) {
		return nil, false
	}
	return t, true
}

func UsableType(reg *axis.Registry, value product.Value, t typ.Type) bool {
	if reg == nil || t == nil || typ.IsAny(t) || typ.IsUnknown(t) || typ.IsNever(t) || refinement.ContainsFreeTypeParam(t) {
		return false
	}
	return proof.New(reg, nil).TypeEvidenceUsableForInference(value)
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
	sum, ok, _ := readProviderSummary(summaries, key)
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

func inferenceTypeFromValue(reg *axis.Registry, value product.Value) (typ.Type, bool) {
	if reg == nil {
		return nil, false
	}
	return proof.New(reg, nil).ValueTypeWithPresence(value)
}

func typeWitnessValue(reg *axis.Registry, typeValues *typevalue.Cache, t typ.Type) product.Value {
	if typeValues != nil {
		return typeValues.FromTypeWithWitness(reg, t)
	}
	return typevalue.WithWitness(reg, typevalue.FromType(reg, t), t)
}

func specializeSummaryReturns(reg *axis.Registry, typeValues *typevalue.Cache, got summary.Summary, originalReturns []typ.Type, returns []typ.Type) summary.Summary {
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
	heapObjects := got.HeapTableObjects
	heapChanged := false
	for i := range nextReturns {
		if i >= len(returns) {
			break
		}
		ret := returns[i]
		if ret == nil || refinement.ContainsFreeTypeParam(ret) {
			continue
		}
		declared := typeWitnessValue(reg, typeValues, ret)
		if i >= len(got.Returns) {
			// No body return for this slot: adopt the declared return directly.
			nextReturns[i] = declared
			changed = true
			continue
		}
		next := joinInstantiatedReturnValue(reg, nextReturns[i], declared, originalReturnTypeAt(originalReturns, i))
		if nextObjects, ok := clampDeclaredReturnHeapMembers(reg, typeValues, got.HeapKeySpace, heapObjects, next, ret); ok {
			heapObjects = nextObjects
			heapChanged = true
		}
		if product.Equal(reg, nextReturns[i], next) {
			continue
		}
		nextReturns[i] = next
		changed = true
	}
	if !changed && !heapChanged {
		return got
	}
	got.Returns = nextReturns
	if heapChanged {
		got.HeapTableObjects = heapObjects
	}
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
	if typ.IsAny(original) || typ.IsUnknown(original) {
		return value
	}
	if refinement.ContainsFreeTypeParam(original) || valueContainsFreeTypeParam(reg, value) {
		return declared
	}
	if returnValueCarriesUntrustedTopEvidence(reg, value) && declaredReturnHasConcreteContract(reg, declared) {
		return declared
	}
	merged := valuerefinement.MergeDeclaredContract(reg, value, declared)
	return product.WithPresence(reg, merged, product.PresenceOf(declared))
}

func returnValueCarriesUntrustedTopEvidence(reg *axis.Registry, value product.Value) bool {
	ev := product.Get(reg, value, evidence.Key)
	return ev.IsExplicitTop() || ev.IsGradualTop()
}

func declaredReturnHasConcreteContract(reg *axis.Registry, value product.Value) bool {
	t, ok := typevalue.TypeOf(reg, value)
	return ok && t != nil && !typ.IsAny(t) && !typ.IsUnknown(t) && !typ.IsNever(t) && !refinement.ContainsFreeTypeParam(t)
}

func clampDeclaredReturnHeapMembers(
	reg *axis.Registry,
	typeValues *typevalue.Cache,
	ks *keyspace.KeySpace,
	objects map[identity.ID]heapidentity.TableObject,
	root product.Value,
	declared typ.Type,
) (map[identity.ID]heapidentity.TableObject, bool) {
	if reg == nil || ks == nil || len(objects) == 0 || declared == nil {
		return objects, false
	}
	id, ok := product.Get(reg, root, identity.Key).ID()
	if !ok {
		return objects, false
	}
	object, ok := objects[id]
	if !ok {
		return objects, false
	}
	contracts := declaredReturnMemberBoundaryContracts(reg, typeValues, ks, declared, object.StaticMembers())
	if len(contracts) == 0 {
		return objects, false
	}
	staticMembers := object.StaticMembers()
	changed := false
	for key, contract := range contracts {
		if existing, ok := staticMembers[key]; ok && product.Equal(reg, existing, contract) {
			continue
		}
		if staticMembers == nil {
			staticMembers = make(map[keyspace.Key]product.Value, len(contracts))
		}
		staticMembers[key] = contract
		changed = true
	}
	if !changed {
		return objects, false
	}
	next := make(map[identity.ID]heapidentity.TableObject, len(objects))
	for objectID, object := range objects {
		next[objectID] = object
	}
	next[id] = heapidentity.NewTableObject(heapidentity.TableObjectConfig{
		Root:              object.Root(),
		StaticMembers:     staticMembers,
		DynamicIndexFacts: object.DynamicIndexFacts(),
	})
	return next, true
}

func declaredReturnMemberBoundaryContracts(reg *axis.Registry, typeValues *typevalue.Cache, ks *keyspace.KeySpace, declared typ.Type, existing map[keyspace.Key]product.Value) map[keyspace.Key]product.Value {
	if len(existing) == 0 {
		return nil
	}
	out := make(map[keyspace.Key]product.Value)
	for key, current := range existing {
		segments, ok := ks.SuffixSegmentsView(key)
		if !ok || len(segments) == 0 {
			continue
		}
		t, ok := luatypeprojection.ApplySegments(declared, segments)
		if !ok || t == nil {
			continue
		}
		currentType, currentTypeOK := typevalue.TypeOf(reg, current)
		if !declaredTypeContainsBoundaryTop(t) && currentTypeOK && declaredMemberAcceptsCurrent(typeValues, currentType, t) {
			continue
		}
		out[key] = typeWitnessValue(reg, typeValues, t)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func declaredMemberAcceptsCurrent(typeValues *typevalue.Cache, current, declared typ.Type) bool {
	if typeValues != nil {
		return typeValues.IsSubtype(current, declared)
	}
	return typevalue.NewCache().IsSubtype(current, declared)
}

func declaredReturnRecord(t typ.Type) (*typ.Record, bool) {
	if optional, ok := t.(*typ.Optional); ok {
		t = optional.Inner
	}
	record, ok := t.(*typ.Record)
	return record, ok
}

func declaredTypeContainsBoundaryTop(t typ.Type) bool {
	return refinement.ContainsBoundaryTop(t)
}

func valueContainsFreeTypeParam(reg *axis.Registry, value product.Value) bool {
	t, ok := typevalue.TypeOf(reg, value)
	return ok && refinement.ContainsFreeTypeParam(t)
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
	reg *axis.Registry,
	got summary.Summary,
	paramOffset int,
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
		paramIndex := i - paramOffset
		if paramIndex < 0 {
			continue
		}
		out.ParamObligations = append(out.ParamObligations, callpayload.CallParamObligation{
			ParamIndex: paramIndex,
			Value:      value,
		})
	}
	for _, obligation := range got.CapturedPathObligations {
		if !summary.UsefulParamObligation(reg, obligation.Value) {
			continue
		}
		stable, ok := pathaddr.StableFromKey(obligation.Path.PathKey())
		if !ok {
			continue
		}
		path, ok := stable.Path()
		if !ok {
			continue
		}
		out.PathObligations = append(out.PathObligations, callpayload.CallPathObligation{
			Path:  path,
			Value: obligation.Value,
		})
	}
	for i, value := range got.NormalReturnParams {
		if usefulNormalReturnParam == nil || !usefulNormalReturnParam(i) {
			continue
		}
		value = normalReturnParamCallRefinement(reg, value)
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
	if len(got.ReturnConditionSlotRefinements) != 0 {
		out.ReturnConditionSlots = make([]callpayload.CallReturnConditionSlotRefinement, len(got.ReturnConditionSlotRefinements))
		for i, refinement := range got.ReturnConditionSlotRefinements {
			out.ReturnConditionSlots[i] = callpayload.CallReturnConditionSlotRefinement{
				ReturnIndex: refinement.ReturnIndex,
				ReturnValue: refinement.ReturnValue,
				TargetIndex: refinement.TargetIndex,
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

func normalReturnParamCallRefinement(reg *axis.Registry, value product.Value) product.Value {
	if reg == nil {
		return value
	}
	p := product.PresenceOf(value)
	if !presence.Equal(p, presence.Present()) && !presence.Equal(p, presence.Absent()) {
		return value
	}
	t, ok := typevalue.TypeOf(reg, value)
	if ok && t != nil && !typ.IsAny(t) && !typ.IsUnknown(t) {
		return value
	}
	kind := product.Get(reg, value, runtimekind.Key)
	if !kind.IsTop() && !kind.IsBottom() {
		return value
	}
	return product.NewWithPresence(reg, product.ShapeTop, p)
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
	p, ok := facts.ExpressionPathRef(source.ExprRef)
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
	typeValues *typevalue.Cache,
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
		value := typeWitnessValue(ctx.Registry, typeValues, want)
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

func projectMemberObligationReceiver(receiver typ.Type, receiverPath pathaddr.SuffixKey) (typ.Type, bool) {
	if receiver == nil {
		return nil, false
	}
	if receiverPath == "" {
		return receiver, true
	}
	segments, ok := pathaddr.RelativeStaticMemberSuffixSegments(receiverPath)
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
func paramReturnExposures(reg *axis.Registry, typeValues *typevalue.Cache, argCount int, got summary.Summary, fn *typ.Function) []callpayload.CallParamExposure {
	if reg == nil || fn == nil || len(got.ReturnParamPathAliases) == 0 {
		return nil
	}
	returns := callResultReturnTypes(got, fn.Returns)
	var out []callpayload.CallParamExposure
	for _, alias := range got.ReturnParamPathAliases {
		paramIndex, ok := alias.Source.RootPlaceholderIndex()
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
		exposure, ok := newParamExposure(reg, typeValues, pathdom.NewPlaceholder(paramIndex), contract)
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
func paramStoreRelationExposures(reg *axis.Registry, typeValues *typevalue.Cache, argCount int, got summary.Summary, fn *typ.Function) []callpayload.CallParamExposure {
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
		exposure, ok := newParamExposure(reg, typeValues, pathdom.NewPlaceholder(sourceIndex), contract)
		if !ok {
			continue
		}
		out = append(out, exposure)
	}
	return out
}

// paramDirectMutationExposures lowers direct callee-side writes through a
// parameter path into covariant call-boundary exposures. A normal-return
// invalidation below $N proves the callee may have mutated the caller's argument
// through its declared parameter view, so the caller must widen that argument to
// the callee's parameter contract before later narrow reads.
func paramDirectMutationExposures(reg *axis.Registry, typeValues *typevalue.Cache, argCount int, got summary.Summary, fn *typ.Function) []callpayload.CallParamExposure {
	if reg == nil || fn == nil || len(got.NormalReturnFacts.PathInvalidations) == 0 {
		return nil
	}
	var out []callpayload.CallParamExposure
	for _, invalidation := range got.NormalReturnFacts.PathInvalidations {
		if !invalidation.Path.IsPlaceholder() || len(invalidation.Path.Segments) == 0 {
			continue
		}
		paramIndex := invalidation.Path.PlaceholderIndex()
		if paramIndex < 0 || paramIndex >= argCount || paramIndex >= len(fn.Params) {
			continue
		}
		contract := fn.Params[paramIndex].Type
		if contract == nil {
			continue
		}
		exposure, ok := newParamExposure(reg, typeValues, pathdom.NewPlaceholder(paramIndex), contract)
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
func paramSinkExposures(reg *axis.Registry, typeValues *typevalue.Cache, argCount int, got summary.Summary) []callpayload.CallParamExposure {
	if reg == nil || len(got.ParamSinkExposures) == 0 {
		return nil
	}
	var out []callpayload.CallParamExposure
	for _, sink := range got.ParamSinkExposures {
		paramIndex, ok := sink.Source.PlaceholderIndex()
		if !ok || paramIndex < 0 || paramIndex >= argCount {
			continue
		}
		contract, ok := typevalue.TypeOf(reg, sink.Contract)
		if !ok {
			continue
		}
		exposure, ok := newParamExposure(reg, typeValues, pathdom.NewPlaceholder(paramIndex), contract)
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
func newParamExposure(reg *axis.Registry, typeValues *typevalue.Cache, source pathdom.Path, contract typ.Type) (callpayload.CallParamExposure, bool) {
	if contract == nil || typ.IsAny(contract) || typ.IsUnknown(contract) || refinement.ContainsFreeTypeParam(contract) {
		return callpayload.CallParamExposure{}, false
	}
	kind, ok := covariantExposureKind(contract)
	if !ok {
		return callpayload.CallParamExposure{}, false
	}
	value := typeWitnessValue(reg, typeValues, contract)
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

func functionTypeParamObligations(reg *axis.Registry, typeValues *typevalue.Cache, argCount int, fn *typ.Function) []callpayload.CallParamObligation {
	if reg == nil || fn == nil || len(fn.Params) == 0 {
		return nil
	}
	return functionTypeParamObligationsFrom(reg, typeValues, argCount, fn, 0)
}

func functionTypeParamObligationsForSite(
	ctx transfer.NodeContext,
	typeValues *typevalue.Cache,
	site factflow.CallSiteView,
	fn *typ.Function,
	sources sourcevalue.SourceValues,
	in state.State,
	read func(cfg.Point) state.State,
) []callpayload.CallParamObligation {
	if ctx.Registry == nil || fn == nil || len(fn.Params) == 0 {
		return nil
	}
	paramOffset := 0
	if site.MethodName() != "" && callableConsumesMethodReceiver(ctx, site, fn, sources, in, read, nil) {
		paramOffset = 1
	}
	return functionTypeParamObligationsFrom(ctx.Registry, typeValues, site.ArgumentSourceCount(), fn, paramOffset)
}

func methodReceiverType(
	ctx transfer.NodeContext,
	site factflow.CallSiteView,
	sources sourcevalue.SourceValues,
	in state.State,
	read func(cfg.Point) state.State,
) (typ.Type, bool) {
	if ctx.Registry == nil {
		return nil, false
	}
	method := site.MethodName()
	var rootType typ.Type
	var hasRootType bool
	if receiverPath, ok := site.ReceiverPath(); ok && receiverPath.Symbol != 0 && len(receiverPath.Segments) == 0 {
		value := in.ReadSymbolValue(ctx.Registry, receiverPath.Symbol)
		if t, ok := typeFromValue(ctx.Registry, value); ok {
			rootType, hasRootType = t, true
			if receiverTypeHasCallableMember(t, method) {
				return t, true
			}
		}
	}
	if sources != nil {
		source, ok := site.ReceiverSource()
		if ok {
			value, ok := sources.ValueOfSource(ctx.Point, source, in, read)
			if ok {
				if t, ok := typeFromValue(ctx.Registry, value); ok {
					return t, true
				}
			}
		}
	}
	if hasRootType {
		return rootType, true
	}
	return nil, false
}

func receiverTypeHasCallableMember(receiverType typ.Type, method string) bool {
	if receiverType == nil || method == "" {
		return false
	}
	fn, status, ok := typecall.MemberCallable(receiverType, method)
	return ok && status == typecall.MemberCallOK && fn != nil
}

func functionTypeParamObligationsFrom(reg *axis.Registry, typeValues *typevalue.Cache, argCount int, fn *typ.Function, paramOffset int) []callpayload.CallParamObligation {
	if reg == nil || fn == nil || len(fn.Params) == 0 || paramOffset >= len(fn.Params) {
		return nil
	}
	limit := argCount
	if limit > len(fn.Params)-paramOffset {
		limit = len(fn.Params) - paramOffset
	}
	var out []callpayload.CallParamObligation
	for i := 0; i < limit; i++ {
		want := fn.Params[i+paramOffset].Type
		if want == nil || typ.IsAny(want) || typ.IsUnknown(want) || refinement.ContainsFreeTypeParam(want) {
			continue
		}
		value := typeWitnessValue(reg, typeValues, want)
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
