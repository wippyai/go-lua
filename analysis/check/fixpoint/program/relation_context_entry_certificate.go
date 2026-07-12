package program

import (
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

// relationContextEntryCertificate is a generation-scoped proof that one
// contextual entry can be represented exactly by symbolic parameter inputs.
// It is deliberately inert: producing a certificate does not select relation
// execution or alter summary routing.
//
// A State is reconstructed from params plus same-value root refinements and
// compared through state.Domain, which covers every registered State lane.
// Consequently a newly added lane fails closed without this producer needing
// a parallel list of forbidden facts.
type relationContextEntryCertificate struct {
	context             summary.SummaryKey
	base                summary.SummaryKey
	preparedBodyDigest  uint64
	discoveryGeneration uint64
	params              []relationContextEntryParam
	// rootRefinements records which parameter roots were physically present
	// as same-value path refinements in the certified concrete entry. A
	// symbolic preservation proof may project a return fact only for these
	// roots; parameter values alone do not authorize inventing one.
	rootRefinements []bool
}

// relationContextEntryParam binds one lexical parameter slot to the immutable
// canonical product value used to construct the contextual entry.
type relationContextEntryParam struct {
	slot   statekey.Value
	symbol symbol.ID
	value  product.Value
}

func certifyRelationContextEntry(
	reg *axis.Registry,
	bindings *bind.Result,
	fn *ast.FunctionExpr,
	context, base summary.SummaryKey,
	bodyDigest, generation uint64,
	entry state.State,
	keys *keyspace.KeySpace,
) *relationContextEntryCertificate {
	if reg == nil || bindings == nil || fn == nil || keys == nil ||
		context.Entry == (summary.EntryKey{}) || bodyDigest == 0 || generation == 0 ||
		bindings.HasEntryCaptures(fn) {
		return nil
	}
	if origin, ok := bindings.FunctionOrigin(fn); ok && origin.Kind == bind.FunctionOriginMethod {
		return nil
	}

	slots := bindings.ParamSlots(fn)
	if len(slots) == 0 {
		return nil
	}
	params := make([]relationContextEntryParam, 0, len(slots))
	paramBySymbol := make(map[symbol.ID]product.Value, len(slots))
	paramIndexBySymbol := make(map[symbol.ID]int, len(slots))
	rootRefinements := make([]bool, len(slots))
	exact := state.State{}
	for index, param := range slots {
		if param.Symbol == 0 || param.Vararg || param.ImplicitSelf {
			return nil
		}
		slot := statekey.SymbolValue(param.Symbol)
		value := entry.ReadValue(reg, slot)
		if slot == 0 || !contextEntryParamValueUseful(reg, value) {
			return nil
		}
		params = append(params, relationContextEntryParam{slot: slot, symbol: param.Symbol, value: value})
		paramBySymbol[param.Symbol] = value
		paramIndexBySymbol[param.Symbol] = index
		exact = exact.WriteValue(reg, slot, value)
	}

	validPaths := true
	entry.ForEachPathRefinement(func(pathKey keyspace.Key, value product.Value) bool {
		paramValue, ok := paramBySymbol[pathKey.Sym]
		if !ok || pathKey.Segs != 0 || !product.Equal(reg, value, paramValue) {
			validPaths = false
			return false
		}
		rootRefinements[paramIndexBySymbol[pathKey.Sym]] = true
		exact = exact.WriteLocalPathKey(reg, pathKey, value)
		return true
	})
	if !validPaths || !state.Domain(reg).Equal(entry, exact) {
		return nil
	}

	return &relationContextEntryCertificate{
		context:             context,
		base:                base,
		preparedBodyDigest:  bodyDigest,
		discoveryGeneration: generation,
		params:              params,
		rootRefinements:     rootRefinements,
	}
}

// certifyFinalRelationContextEntries publishes certificates only after every
// context-entry transform has completed. Opportunistic discovery proofs are
// replaced because TransformEntries cannot prove their preservation.
func (k *programKeys) certifyFinalRelationContextEntries(reg *axis.Registry, prepared preparedBodies) {
	if k == nil || !k.certifyRelationContexts || reg == nil || k.bindings == nil {
		return
	}
	generation := k.contexts.nextEntryDiscoveryGeneration()
	for i := range k.contexts.entries {
		context := &k.contexts.entries[i]
		context.relationContextEntry = nil
		if context.funcExpr == nil || !context.hasEntryState {
			continue
		}
		base, ok := k.summaryKeyForFunction(context.funcExpr)
		static := prepared.function(context.funcExpr)
		if !ok || static == nil {
			continue
		}
		context.relationContextEntry = certifyRelationContextEntry(
			reg, k.bindings, context.funcExpr, context.key, base,
			static.IdentityDigest(), generation, context.entryState, context.entryKeys,
		)
	}
}
