package program

import (
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/kind"
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
	captures            []relationContextEntryCapture
	// rootRefinements records which parameter roots were physically present
	// as same-value path refinements in the certified concrete entry. A
	// symbolic preservation proof may project a return fact only for these
	// roots; parameter values alone do not authorize inventing one.
	rootRefinements []bool
	// captureRootRefinements is the same physical-entry proof for ordered
	// capture roots. Projection uses the capture's concrete lexical path, never
	// a parameter placeholder.
	captureRootRefinements []bool
}

// relationContextEntryParam binds one lexical parameter slot to the immutable
// canonical product value used to construct the contextual entry.
type relationContextEntryParam struct {
	slot   statekey.Value
	symbol symbol.ID
	value  product.Value
}

type relationContextEntryCapture struct {
	slot   statekey.Value
	symbol symbol.ID
	name   string
	value  product.Value
	path   pathdom.Path
}

func certifyRelationContextEntry(
	reg *axis.Registry,
	bindings *bind.Result,
	fn *ast.FunctionExpr,
	boundaryParams, boundaryCaptures []symbol.ID,
	context, base summary.SummaryKey,
	bodyDigest, generation uint64,
	entry state.State,
	keys *keyspace.KeySpace,
) *relationContextEntryCertificate {
	if reg == nil || bindings == nil || fn == nil || keys == nil ||
		context.Entry == (summary.EntryKey{}) || bodyDigest == 0 || generation == 0 {
		return nil
	}
	if origin, ok := bindings.FunctionOrigin(fn); ok && origin.Kind == bind.FunctionOriginMethod {
		return nil
	}

	slots := bindings.ParamSlots(fn)
	if len(slots) != len(boundaryParams) {
		return nil
	}
	if len(slots) == 0 && len(boundaryCaptures) == 0 {
		return nil
	}
	params := make([]relationContextEntryParam, 0, len(slots))
	paramBySymbol := make(map[symbol.ID]product.Value, len(slots))
	paramIndexBySymbol := make(map[symbol.ID]int, len(slots))
	rootRefinements := make([]bool, len(slots))
	directCaptures := bindings.DirectCaptures(fn)
	directBySymbol := make(map[symbol.ID]bind.Capture, len(directCaptures))
	for _, capture := range directCaptures {
		directBySymbol[capture.Captured] = capture
	}
	captures := make([]relationContextEntryCapture, 0, len(boundaryCaptures))
	captureRootRefinements := make([]bool, len(boundaryCaptures))
	exact := state.State{}
	for index, param := range slots {
		if param.Symbol == 0 || param.Symbol != boundaryParams[index] || param.Vararg || param.ImplicitSelf {
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
	seenCaptures := make(map[symbol.ID]struct{}, len(boundaryCaptures))
	for index, captureSymbol := range boundaryCaptures {
		capture, present := directBySymbol[captureSymbol]
		if !present || capture.Captured == 0 || capture.CapturedName == "" || bindings.HasWrite(capture.Captured) {
			return nil
		}
		if _, duplicate := seenCaptures[capture.Captured]; duplicate {
			return nil
		}
		seenCaptures[capture.Captured] = struct{}{}
		slot := statekey.SymbolValue(capture.Captured)
		value := entry.ReadValue(reg, slot)
		if slot == 0 || !contextEntryCaptureValueUseful(reg, value) {
			return nil
		}
		if _, duplicate := paramBySymbol[capture.Captured]; duplicate {
			return nil
		}
		captures = append(captures, relationContextEntryCapture{slot: slot, symbol: capture.Captured, name: capture.CapturedName, value: value})
		paramBySymbol[capture.Captured] = value
		paramIndexBySymbol[capture.Captured] = len(slots) + index
		exact = exact.WriteValue(reg, slot, value)
	}

	validPaths := true
	entry.ForEachPathRefinement(func(pathKey keyspace.Key, value product.Value) bool {
		paramValue, ok := paramBySymbol[pathKey.Sym]
		if !ok || pathKey.Segs != 0 || !product.Equal(reg, value, paramValue) {
			validPaths = false
			return false
		}
		index := paramIndexBySymbol[pathKey.Sym]
		if index < len(rootRefinements) {
			rootRefinements[index] = true
		} else {
			captureIndex := index - len(rootRefinements)
			// Capture paths cross a lexical boundary and therefore retain their
			// exact certified key identity. Versioned/stable/named spellings are
			// not interchangeable with an unversioned lexical capture root.
			if (pathKey.Kind != keyspace.KindUnversionedSym && pathKey.Kind != keyspace.KindResolverSym) || pathKey.Segs != 0 || pathKey.Canon {
				validPaths = false
				return false
			}
			capturePath, pathOK := keys.StatePath(pathKey)
			if !pathOK || capturePath.Symbol != pathKey.Sym || uint32(capturePath.Version) != pathKey.Ver || len(capturePath.Segments) != 0 {
				validPaths = false
				return false
			}
			captures[captureIndex].path = capturePath
			captureRootRefinements[captureIndex] = true
		}
		exact = exact.WriteLocalPathKey(reg, pathKey, value)
		return true
	})
	if !validPaths || !state.Domain(reg).Equal(entry, exact) {
		return nil
	}

	return &relationContextEntryCertificate{
		context:                context,
		base:                   base,
		preparedBodyDigest:     bodyDigest,
		discoveryGeneration:    generation,
		params:                 params,
		captures:               captures,
		rootRefinements:        rootRefinements,
		captureRootRefinements: captureRootRefinements,
	}
}

func contextEntryCaptureValueUseful(reg *axis.Registry, value product.Value) bool {
	if !contextEntryValueUseful(reg, value) {
		return false
	}
	t, ok := typevalue.TypeOf(reg, value)
	if !ok || t == nil {
		return false
	}
	switch t.Kind() {
	case kind.Nil, kind.Boolean, kind.Number, kind.Integer, kind.String, kind.Literal:
		return true
	default:
		return false
	}
}

func (c *relationContextEntryCertificate) matchesBoundary(params, captures []symbol.ID) bool {
	if c == nil || len(c.params) != len(params) || len(c.captures) != len(captures) {
		return false
	}
	for i := range params {
		if c.params[i].symbol != params[i] {
			return false
		}
	}
	for i := range captures {
		if c.captures[i].symbol != captures[i] {
			return false
		}
	}
	return true
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
			reg, k.bindings, context.funcExpr, static.OperationPlan().BoundaryParams(), static.OperationPlan().BoundaryCaptures(), context.key, base,
			static.IdentityDigest(), generation, context.entryState, context.entryKeys,
		)
	}
}
