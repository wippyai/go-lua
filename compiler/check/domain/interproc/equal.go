package interproc

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/functionfact"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/domain/value/product"
)

// FactsEqual checks if two canonical interproc fact bundles are equal.
func FactsEqual(a, b api.Facts) bool {
	if !FunctionFactsEqual(a.FunctionFacts, b.FunctionFacts) {
		return false
	}
	if !LiteralSigsEqual(a.LiteralSigs, b.LiteralSigs) {
		return false
	}
	if !symbolTypeMapEqual(a.CapturedTypes, b.CapturedTypes) {
		return false
	}
	if !CapturedFieldAssignsEqual(a.CapturedFields, b.CapturedFields) {
		return false
	}
	if !CapturedContainerMutationsEqual(a.CapturedContainers, b.CapturedContainers) {
		return false
	}
	if !ConstructorFieldsEqual(a.ConstructorFields, b.ConstructorFields) {
		return false
	}
	return true
}

// FunctionFactsEqual checks if two canonical function-fact maps are equal.
func FunctionFactsEqual(a, b api.FunctionFacts) bool {
	if len(a) != len(b) {
		return false
	}
	for _, sym := range cfg.SortedSymbolIDs(a) {
		af := a[sym]
		bf, ok := b[sym]
		if !ok {
			return false
		}
		if !FunctionFactEqual(af, bf) {
			return false
		}
	}
	return true
}

// FunctionFactEqual checks one canonical function-fact product slot. The vector
// carriers are interned product.AbstractValue, so their convergence no-op
// equality is the value-domain product.Equal per slot (pointer-fast through
// interning), the same relation the flow store uses for its fixpoint.
func FunctionFactEqual(a, b api.FunctionFact) bool {
	if !product.EqualVector(a.Params, b.Params) {
		return false
	}
	if !product.EqualVector(a.BodyParams, b.BodyParams) {
		return false
	}
	if !product.EqualVector(a.EntryParams, b.EntryParams) {
		return false
	}
	if !product.EqualVector(a.Summary, b.Summary) {
		return false
	}
	if !product.EqualVector(a.Narrow, b.Narrow) {
		return false
	}
	if !value.FactTypeEqual(a.Signature, b.Signature) {
		return false
	}
	if !RefinementEqual(a.Refinement, b.Refinement) {
		return false
	}
	if !functionfact.EnvReturnsEqual(a.EnvReturns, b.EnvReturns) {
		return false
	}
	return true
}

// LiteralSigsEqual checks if two literal signature maps are equal.
func LiteralSigsEqual(a, b api.LiteralSigs) bool {
	if len(a) != len(b) {
		return false
	}
	for fn, sig := range a {
		other, ok := b[fn]
		if !ok || !value.FactTypeEqual(sig, other) {
			return false
		}
	}
	return true
}

func symbolTypeMapEqual(a map[cfg.SymbolID]product.AbstractValue, b map[cfg.SymbolID]product.AbstractValue) bool {
	if len(a) != len(b) {
		return false
	}
	for _, sym := range cfg.SortedSymbolIDs(a) {
		left := a[sym]
		right, ok := b[sym]
		if !ok || !product.Equal(left, right) {
			return false
		}
	}
	return true
}

// CapturedFieldAssignsEqual checks if two captured field assignment maps are equal.
func CapturedFieldAssignsEqual(a, b api.CapturedFieldAssigns) bool {
	if len(a) != len(b) {
		return false
	}
	for _, callee := range cfg.SortedSymbolIDs(a) {
		fieldsBySym := a[callee]
		other := b[callee]
		if len(fieldsBySym) != len(other) {
			return false
		}
		for _, sym := range cfg.SortedSymbolIDs(fieldsBySym) {
			fields := fieldsBySym[sym]
			otherFields := other[sym]
			if len(fields) != len(otherFields) {
				return false
			}
			for _, name := range cfg.SortedFieldNames(fields) {
				left := fields[name]
				right := otherFields[name]
				if !product.Equal(left, right) {
					return false
				}
			}
		}
	}
	return true
}

// CapturedContainerMutationsEqual checks if two captured container mutation maps are equal.
func CapturedContainerMutationsEqual(a, b api.CapturedContainerMutations) bool {
	if len(a) != len(b) {
		return false
	}
	for _, callee := range cfg.SortedSymbolIDs(a) {
		baseMap := a[callee]
		otherBase := b[callee]
		if len(baseMap) != len(otherBase) {
			return false
		}
		for _, sym := range cfg.SortedSymbolIDs(baseMap) {
			muts := baseMap[sym]
			otherMuts := otherBase[sym]
			if !containerMutationSlicesEqual(muts, otherMuts) {
				return false
			}
		}
	}
	return true
}

func containerMutationSlicesEqual(a, b []api.ContainerMutation) bool {
	if len(a) != len(b) {
		return false
	}
	if len(a) == 0 {
		return true
	}
	index := make(map[string]api.ContainerMutation, len(a))
	for _, m := range a {
		index[api.ContainerMutationKey(m)] = m
	}
	for _, m := range b {
		key := api.ContainerMutationKey(m)
		other, ok := index[key]
		if !ok ||
			!product.Equal(other.KeyType, m.KeyType) ||
			!product.Equal(other.ValueType, m.ValueType) {
			return false
		}
	}
	return true
}

// ConstructorFieldsEqual checks if two constructor field maps are equal.
func ConstructorFieldsEqual(a, b api.ConstructorFields) bool {
	if len(a) != len(b) {
		return false
	}
	for _, sym := range cfg.SortedSymbolIDs(a) {
		fields := a[sym]
		other := b[sym]
		if len(fields) != len(other) {
			return false
		}
		for _, name := range cfg.SortedFieldNames(fields) {
			left := fields[name]
			right := other[name]
			if !product.Equal(left, right) {
				return false
			}
		}
	}
	return true
}
