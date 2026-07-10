package program

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/program/internal/callresult"
	"github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	valueref "github.com/wippyai/go-lua/analysis/domain/value/refinement"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/typeresolve"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

func applyCallArgumentParamEntryState(
	reg *axis.Registry,
	bindings *bind.Result,
	prepass *body.Result,
	keys *programKeys,
	point cfg.Point,
	site factflow.CallSiteView,
	fn *ast.FunctionExpr,
	contextualFn *typ.Function,
	entry state.State,
) (state.State, bool) {
	if reg == nil || bindings == nil || prepass == nil || fn == nil {
		return entry, false
	}
	slots := bindings.ParamSlots(fn)
	if len(slots) == 0 {
		return entry, false
	}
	seen := false
	caller, hasCaller := prepass.StateAtBoundary(point)
	capturedArgumentRoots := callArgumentCapturedRootSymbols(bindings, prepass, site)
	nextParam := 0
	if receiver, ok := callReceiverValue(reg, prepass, point, site); ok {
		if slot, ok := paramValueSlot(slots, nextParam); ok {
			value, contractOK := contextualParamEntryValue(reg, contextualFn, nextParam)
			if !contractOK {
				value, contractOK = declaredParamEntryValue(reg, prepass.TypeResolver(), slots[nextParam])
			}
			value, valueOK := callContextParamEntryValue(reg, receiver, true, value, contractOK)
			if !valueOK {
				value = receiver
			}
			entry = entry.WriteValue(reg, slot, value)
			entry = writeFiniteParamRootPathValue(reg, prepass.KeySpace(), entry, slots[nextParam], value)
			if hasCaller {
				if updated, ok := seedEntryHeapObjectsForValue(reg, caller, entry, receiver); ok {
					entry = updated
				}
			}
			seen = true
			nextParam++
		}
	}
	site.ForEachArgumentSource(func(i int, source factflow.ValueSource) bool {
		slot, ok := paramValueSlot(slots, i+nextParam)
		if !ok {
			return false
		}
		actual, actualOK := callArgumentEntryValue(reg, prepass, keys, point, source)
		value, contractOK := contextualParamEntryValue(reg, contextualFn, i+nextParam)
		if !contractOK {
			value, contractOK = declaredParamEntryValue(reg, prepass.TypeResolver(), slots[i+nextParam])
		}
		var valueOK bool
		topLikeContract := paramSlotContainsTopLikeContract(prepass.TypeResolver(), contextualFn, slots[i+nextParam], i+nextParam)
		rootTopLikeContract := paramSlotRootTopLikeContract(prepass.TypeResolver(), contextualFn, slots[i+nextParam], i+nextParam)
		entryActual, entryActualOK := actual, actualOK
		if source.Kind == factflow.ValueSourceCall && contractOK {
			entryActualOK = false
		}
		value, valueOK = callContextParamEntryValue(reg, entryActual, entryActualOK, value, contractOK)
		if !valueOK {
			return true
		}
		var callableEntry state.State
		var callableValue product.Value
		var hasCallableValue bool
		if topLikeContract && actualOK && hasCaller {
			callableEntry, callableValue, hasCallableValue = seedEntryCallableHeapObjectsForValue(reg, caller, entry, actual)
			if hasCallableValue {
				value = callableValue
				entry = callableEntry
			}
		}
		entry = entry.WriteValue(reg, slot, value)
		entry = writeFiniteParamRootPathValue(reg, prepass.KeySpace(), entry, slots[i+nextParam], value)
		if !hasCallableValue && actualOK && hasCaller {
			if updated, ok := seedEntryHeapObjectsForValue(reg, caller, entry, actual); ok {
				entry = updated
			}
		}
		if !rootTopLikeContract {
			if updated, ok := applyCallArgumentPathEntryState(reg, prepass, point, source, slots[i+nextParam], capturedArgumentRoots, entry); ok {
				entry = updated
			}
		}
		seen = true
		return true
	})
	return entry, seen
}

func callArgumentCapturedRootSymbols(bindings *bind.Result, prepass *body.Result, site factflow.CallSiteView) map[symbol.ID]struct{} {
	if bindings == nil || prepass == nil {
		return nil
	}
	var out map[symbol.ID]struct{}
	resolver := closureCaptureSeeder{bindings: bindings}
	site.ForEachArgumentSource(func(_ int, source factflow.ValueSource) bool {
		if source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
			return true
		}
		fnSym, ok := prepass.ExpressionFunction(source.ExprRef)
		if !ok || fnSym == 0 {
			if p, pathOK := prepass.ExpressionPathRef(source.ExprRef); pathOK {
				fnSym = p.Symbol
			}
		}
		if fnSym == 0 {
			return true
		}
		fn, ok := resolver.functionForCapturedSymbol(fnSym)
		if !ok || fn == nil {
			return true
		}
		for _, capture := range bindings.DirectCaptures(fn) {
			if capture.Captured == 0 {
				continue
			}
			if out == nil {
				out = make(map[symbol.ID]struct{})
			}
			out[capture.Captured] = struct{}{}
		}
		return true
	})
	return out
}

func writeFiniteParamRootPathValue(reg *axis.Registry, ks *keyspace.KeySpace, entry state.State, slot bind.ParamSlot, value product.Value) state.State {
	if reg == nil || ks == nil || slot.Symbol == 0 {
		return entry
	}
	t, ok := typevalue.TypeOf(reg, value)
	if !ok || t == nil || typ.ContainsRecursive(t) {
		return entry
	}
	paramRoot := path.NewPath(slot.Symbol, slot.Name)
	paramRoot.Version = 1
	return entry.WritePathKey(reg, ks, paramRoot.Key(), value)
}

func callContextParamEntryValue(reg *axis.Registry, actual product.Value, actualOK bool, contract product.Value, contractOK bool) (product.Value, bool) {
	if contractOK && declaredContractHasExplicitTopBoundary(reg, contract) {
		return contract, true
	}
	if actualOK && contextEntryParamValueUseful(reg, actual) {
		if !contractOK {
			return actual, true
		}
		if !actualParamEntryValueSatisfiesContract(reg, actual, contract) {
			return contract, true
		}
		if actualParamEntryValueIsMorePrecise(reg, actual, contract) {
			return actual, true
		}
		return valueref.MergeDeclaredContract(reg, actual, contract), true
	}
	if contractOK {
		return contract, true
	}
	return product.Value{}, false
}

func actualParamEntryValueSatisfiesContract(reg *axis.Registry, actual, contract product.Value) bool {
	if reg == nil {
		return false
	}
	actualType, actualOK := typevalue.TypeOf(reg, actual)
	contractType, contractOK := typevalue.TypeOf(reg, contract)
	if !actualOK || !contractOK || actualType == nil || contractType == nil {
		return false
	}
	return subtype.IsSubtype(actualType, contractType)
}

func actualParamEntryValueIsMorePrecise(reg *axis.Registry, actual, contextual product.Value) bool {
	if reg == nil {
		return false
	}
	actualType, actualOK := typevalue.TypeOf(reg, actual)
	contextType, contextOK := typevalue.TypeOf(reg, contextual)
	if !actualOK || !contextOK || actualType == nil || contextType == nil {
		return false
	}
	if typetable.IsBuiltinTopMarker(contextType) && subtype.IsSubtype(actualType, contextType) {
		return true
	}
	if actualLiteral, ok := typ.UnwrapTransparentWrappers(actualType).(*typ.Literal); ok &&
		subtype.IsSubtype(actualType, contextType) &&
		finiteLiteralParamDomainContains(contextType, actualLiteral) {
		return true
	}
	return false
}

func finiteLiteralParamDomainContains(domain typ.Type, actual *typ.Literal) bool {
	if domain == nil || actual == nil {
		return false
	}
	switch tt := typ.UnwrapTransparentWrappers(domain).(type) {
	case *typ.Literal:
		return typ.TypeEquals(tt, actual)
	case *typ.Union:
		for _, member := range tt.Members {
			lit, ok := typ.UnwrapTransparentWrappers(member).(*typ.Literal)
			if !ok {
				return false
			}
			if typ.TypeEquals(lit, actual) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func declaredParamEntryValue(reg *axis.Registry, resolver *typeresolve.Resolver, slot bind.ParamSlot) (product.Value, bool) {
	if reg == nil || resolver == nil || slot.Type == nil {
		return product.Value{}, false
	}
	t, ok := resolver.Type(slot.Type)
	if !ok {
		return product.Value{}, false
	}
	return paramContractEntryValue(reg, t)
}

func contextualParamEntryValue(reg *axis.Registry, fn *typ.Function, index int) (product.Value, bool) {
	t, ok := callParamType(fn, index)
	if !ok {
		return product.Value{}, false
	}
	return paramContractEntryValue(reg, t)
}

func paramContractEntryValue(reg *axis.Registry, t typ.Type) (product.Value, bool) {
	if reg == nil || t == nil {
		return product.Value{}, false
	}
	if param, ok := typ.UnwrapTransparentWrappers(t).(*typ.TypeParam); ok {
		if param.Constraint == nil {
			return product.Value{}, false
		}
		t = param.Constraint
	}
	if rootTopLikeAnnotationType(t) {
		value := typevalue.WithWitness(reg, typevalue.FromType(reg, t), t)
		value = product.Set(reg, value, evidence.Key, evidence.ExplicitTop())
		value = product.Set(reg, value, assertion.Key, assertion.Any())
		return value, true
	}
	if !usableContextualTypeOnly(t) {
		return product.Value{}, false
	}
	return typevalue.WithWitness(reg, typevalue.FromType(reg, t), t), true
}

func rootTopLikeAnnotationType(t typ.Type) bool {
	if t == nil {
		return false
	}
	t = typ.UnwrapTransparentWrappers(t)
	if typ.IsAny(t) || typ.IsUnknown(t) {
		return true
	}
	opt, ok := t.(*typ.Optional)
	if !ok || opt == nil {
		return rootDynamicAnyContainerType(t)
	}
	inner := typ.UnwrapTransparentWrappers(opt.Inner)
	return typ.IsAny(inner) || typ.IsUnknown(inner) || rootDynamicAnyContainerType(inner)
}

func rootDynamicAnyContainerType(t typ.Type) bool {
	switch v := typ.UnwrapTransparentWrappers(t).(type) {
	case *typ.Map:
		return typ.IsAny(typ.UnwrapTransparentWrappers(v.Value)) || typ.IsUnknown(typ.UnwrapTransparentWrappers(v.Value))
	case *typ.ReadonlyMap:
		return typ.IsAny(typ.UnwrapTransparentWrappers(v.Value)) || typ.IsUnknown(typ.UnwrapTransparentWrappers(v.Value))
	case *typ.Record:
		if v == nil || !v.HasMapComponent() || len(v.Fields) != 0 || len(v.StaticMembers) != 0 {
			return false
		}
		value := typ.UnwrapTransparentWrappers(v.MapValue)
		return typ.IsAny(value) || typ.IsUnknown(value)
	default:
		return false
	}
}

func declaredContractHasExplicitTopBoundary(reg *axis.Registry, value product.Value) bool {
	return reg != nil && product.Get(reg, value, evidence.Key).IsExplicitTop()
}

func paramSlotContainsTopLikeContract(resolver *typeresolve.Resolver, fn *typ.Function, slot bind.ParamSlot, index int) bool {
	if t, ok := callParamType(fn, index); ok && containsTopLikeAnnotationType(t) {
		return true
	}
	if resolver == nil || slot.Type == nil {
		return false
	}
	t, ok := resolver.Type(slot.Type)
	return ok && containsTopLikeAnnotationType(t)
}

func paramSlotRootTopLikeContract(resolver *typeresolve.Resolver, fn *typ.Function, slot bind.ParamSlot, index int) bool {
	if t, ok := callParamType(fn, index); ok && rootTopLikeAnnotationType(t) {
		return true
	}
	if resolver == nil || slot.Type == nil {
		return false
	}
	t, ok := resolver.Type(slot.Type)
	return ok && rootTopLikeAnnotationType(t)
}

func applyCallArgumentPathEntryState(
	reg *axis.Registry,
	prepass *body.Result,
	point cfg.Point,
	source factflow.ValueSource,
	slot bind.ParamSlot,
	capturedArgumentRoots map[symbol.ID]struct{},
	entry state.State,
) (state.State, bool) {
	if reg == nil || prepass == nil || slot.Symbol == 0 || source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
		return entry, false
	}
	actualPath, ok := prepass.ExpressionPathRef(source.ExprRef)
	if !ok || actualPath.IsEmpty() {
		return entry, false
	}
	actualRootKey, ok := prepass.PathKeyAtBoundary(point, actualPath)
	if !ok || actualRootKey == "" {
		return entry, false
	}
	paramRoot := path.NewPath(slot.Symbol, slot.Name)
	paramRoot.Version = 1
	paramRootKey := paramRoot.Key()
	if paramRootKey == "" {
		return entry, false
	}
	caller, ok := prepass.StateAt(point)
	if !ok {
		return entry, false
	}
	ks := prepass.KeySpace()
	out := entry
	seen := false
	if _, captured := capturedArgumentRoots[actualPath.RootOnly().Symbol]; captured {
		if actualKey, actualOK := ks.FromPathKey(actualRootKey); actualOK {
			if paramKey, paramOK := ks.FromPathKey(paramRootKey); paramOK && actualKey != paramKey {
				out = out.AddBranchProof(pathevidence.BranchProof{
					Kind:  pathevidence.BranchProofPathEqual,
					Path:  paramKey,
					Other: actualKey,
				})
				seen = true
			}
		}
	}
	edit := out.EditPathEvidence(reg)
	bottom := product.Bottom(reg)
	if snapshot := caller.PathRefinementsSnapshot(ks); !snapshot.Top {
		for pathKey, value := range snapshot.Refinements {
			if pathKey == "" || product.Equal(reg, value, bottom) {
				continue
			}
			rebased, ok := pathaddr.RebasePathKey(pathKey, actualRootKey, paramRootKey)
			if !ok || rebased == "" {
				continue
			}
			edit.WritePathKey(ks, rebased, value)
			seen = true
		}
	}
	if snapshot := caller.PathStaticMembersSnapshot(ks); !snapshot.Bottom && !snapshot.Top {
		for pathKey, value := range snapshot.Members {
			if pathKey == "" || product.Equal(reg, value, bottom) {
				continue
			}
			rebased, ok := pathaddr.RebasePathKey(pathKey, actualRootKey, paramRootKey)
			if !ok || rebased == "" {
				continue
			}
			edit.WritePathStaticMember(ks, rebased, value)
			seen = true
		}
	}
	return edit.DoneOn(out), seen
}

func contextEntryParamValueUseful(reg *axis.Registry, value product.Value) bool {
	if !contextEntryValueUseful(reg, value) {
		return false
	}
	t, ok := typevalue.TypeOf(reg, value)
	return ok && callresult.UsableType(reg, value, t)
}

func callArgumentEntryValue(reg *axis.Registry, prepass *body.Result, keys *programKeys, point cfg.Point, source factflow.ValueSource) (product.Value, bool) {
	if reg == nil || prepass == nil {
		return product.Value{}, false
	}
	objectLiteralValue := func() (product.Value, typ.Type, bool) {
		t, ok := contextualObjectLiteralArgumentType(reg, prepass, point, source)
		if !ok || t == nil {
			return product.Value{}, nil, false
		}
		value := typevalue.WithWitness(reg, typevalue.FromType(reg, t), t)
		if !contextEntryValueUseful(reg, value) {
			return product.Value{}, nil, false
		}
		return value, t, true
	}
	if value, ok := prepass.SourceValueAtBoundary(point, source); ok && contextEntryValueUseful(reg, value) {
		if litValue, litType, litOK := objectLiteralValue(); litOK {
			if valueType, typeOK := typevalue.TypeOf(reg, value); !typeOK || subtype.IsSubtype(litType, valueType) {
				return valueref.MergeDeclaredContract(reg, value, litValue), true
			}
		}
		return value, true
	}
	if source.Kind == factflow.ValueSourceExpression && source.HasExpr {
		if p, ok := prepass.ExpressionPathRef(source.ExprRef); ok {
			if value, ok := prepass.PathValueAtBoundary(point, p); ok && contextEntryValueUseful(reg, value) {
				if litValue, litType, litOK := objectLiteralValue(); litOK {
					if valueType, typeOK := typevalue.TypeOf(reg, value); !typeOK || subtype.IsSubtype(litType, valueType) {
						return valueref.MergeDeclaredContract(reg, value, litValue), true
					}
				}
				return value, true
			}
		}
	}
	if litValue, _, litOK := objectLiteralValue(); litOK {
		return litValue, true
	}
	if value, ok := callResultSourceContractValue(reg, prepass, keys, source); ok && contextEntryValueUseful(reg, value) {
		return value, true
	}
	return product.Value{}, false
}

func callResultSourceContractValue(reg *axis.Registry, prepass *body.Result, keys *programKeys, source factflow.ValueSource) (product.Value, bool) {
	if reg == nil || prepass == nil ||
		keys == nil ||
		source.Kind != factflow.ValueSourceCall ||
		!source.HasCallPoint ||
		source.ResultIndex < 0 {
		return product.Value{}, false
	}
	site, ok := prepass.CallSiteView(source.CallPoint)
	if !ok {
		return product.Value{}, false
	}
	key, ok := prepassCallSummaryKey(reg, prepass, source.CallPoint, site, keys)
	if !ok {
		return product.Value{}, false
	}
	fn := instantiateSignatureTypeForContext(reg, prepass, source.CallPoint, site, keys.functionTypes[key], keys)
	if fn == nil || source.ResultIndex >= len(fn.Returns) {
		return product.Value{}, false
	}
	ret := fn.Returns[source.ResultIndex]
	if !usableContextualTypeOnly(ret) {
		return product.Value{}, false
	}
	return typevalue.WithWitness(reg, typevalue.FromType(reg, ret), ret), true
}

func callReceiverValue(reg *axis.Registry, prepass *body.Result, point cfg.Point, site factflow.CallSiteView) (product.Value, bool) {
	if source, ok := site.ReceiverSource(); ok {
		value, ok := prepass.SourceValueAtBoundary(point, source)
		if ok && contextEntryValueUseful(reg, value) {
			return value, true
		}
	}
	if receiverPath, ok := site.ReceiverPath(); ok {
		value, ok := prepass.PathValueAtBoundary(point, receiverPath)
		if ok && contextEntryValueUseful(reg, value) {
			return value, true
		}
	}
	return product.Value{}, false
}

func paramValueSlot(slots []bind.ParamSlot, index int) (statekey.Value, bool) {
	if index < 0 || index >= len(slots) || slots[index].Symbol == 0 || slots[index].Vararg {
		return 0, false
	}
	slot := statekey.SymbolValue(slots[index].Symbol)
	return slot, slot != 0
}

func contextEntryValueUseful(reg *axis.Registry, value product.Value) bool {
	return reg != nil &&
		!product.Equal(reg, value, product.Bottom(reg)) &&
		!product.Equal(reg, value, product.Top())
}
