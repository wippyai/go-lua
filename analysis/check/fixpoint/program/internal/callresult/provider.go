// Package callresult adapts fixpoint summaries into factflow call outcomes.
package callresult

import (
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callpayload"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/typ"
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
	ProtectedCall           func(transfer.NodeContext, factflow.CallSiteView) bool
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
	callerKeySpace := config.KeySpace
	typeValues := config.TypeValues
	preparedSummaries := providerPreparedSummaryCache{
		summaries:      summaries,
		functionTypes:  functionTypes,
		callerKeySpace: callerKeySpace,
	}
	transaction := newPreparedSummaryTransaction(config, index)
	return func(ctx transfer.NodeContext, site factflow.CallSiteView, in state.State, read func(cfg.Point) state.State) callpayload.CallOutcome {
		if summaries == nil {
			return refineOutcomeResultsFromCurrentCallable(ctx, site, sources, in, read, calleeValue, receiverCallable, typeValues, callpayload.CallOutcome{})
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
			if out := refineOutcomeResultsFromCurrentCallable(ctx, site, sources, in, read, calleeValue, receiverCallable, typeValues, callpayload.CallOutcome{}); !out.Empty() {
				return out
			}
			if out, ok := unresolvedFunctionCallOutcome(ctx, site, in, read, calleeValue); ok {
				return refineOutcomeResultsFromCurrentCallable(ctx, site, sources, in, read, calleeValue, receiverCallable, typeValues, out)
			}
			return callpayload.CallOutcome{}
		}
	matched:
		if matchedContext {
			got = inheritSameIdentityContextBaseSummary(ctx.Registry, typeValues, &preparedSummaries, currentIdentityKey, hasCurrentIdentityKey, matchedContextKey, got)
		}
		return transaction.Apply(ctx, site, in, read, got, fn, summaryOwned)
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
