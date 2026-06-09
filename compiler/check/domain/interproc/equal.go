package interproc

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/functionfact"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/domain/value/product"
)

// ProjectionProductEqual checks whether two postflow projection products are equal.
func ProjectionProductEqual(a, b ProjectionProduct) bool {
	if !FunctionFactsEqual(a.FunctionFacts, b.FunctionFacts) {
		return false
	}
	if !symbolTypeMapEqual(a.CapturedTypes, b.CapturedTypes) {
		return false
	}
	if !CapturedFieldAssignsEqual(a.CapturedFields, b.CapturedFields) {
		return false
	}
	if !ConstructorFieldsEqual(a.ConstructorFields, b.ConstructorFields) {
		return false
	}
	return true
}

// FunctionFactsEqual checks if two FunctionFacts projection maps are equal.
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

// FunctionFactEqual checks one FunctionFacts projection product slot. The vector
// carriers are interned product.AbstractValue, so their convergence no-op
// equality is the value-domain product.Equal per slot (pointer-fast through
// interning), the same relation the flow store uses for its fixpoint.
func FunctionFactEqual(a, b api.FunctionFact) bool {
	if !product.EqualVector(a.Call.Params, b.Call.Params) {
		return false
	}
	if !product.EqualVector(a.Body.Params, b.Body.Params) {
		return false
	}
	if !product.EqualVector(a.Entry.Params, b.Entry.Params) {
		return false
	}
	if !product.EqualVector(a.Returns.Preflow, b.Returns.Preflow) {
		return false
	}
	if !product.EqualVector(a.Returns.Postflow, b.Returns.Postflow) {
		return false
	}
	if !value.FactTypeEqual(a.Public.Signature, b.Public.Signature) {
		return false
	}
	if !RefinementEqual(a.Effects.Refinement, b.Effects.Refinement) {
		return false
	}
	if !functionfact.EnvReturnsEqual(a.Export.EnvReturns, b.Export.EnvReturns) {
		return false
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
			for _, key := range SortedFieldKeys(fields) {
				left := fields[key]
				right := otherFields[key]
				if !product.Equal(left, right) {
					return false
				}
			}
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
		for _, key := range SortedFieldKeys(fields) {
			left := fields[key]
			right := other[key]
			if !product.Equal(left, right) {
				return false
			}
		}
	}
	return true
}
