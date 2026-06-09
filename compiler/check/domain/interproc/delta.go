package interproc

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
)

// FunctionFactProjectionDelta returns a canonical product delta for function facts.
func FunctionFactProjectionDelta(facts api.FunctionFacts) ProjectionProduct {
	if len(facts) == 0 {
		return ProjectionProduct{}
	}
	return JoinProjectionProduct(ProjectionProduct{}, ProjectionProduct{FunctionFacts: facts})
}

// CapturedTypeProjectionDelta returns a canonical product delta for captured symbol types.
func CapturedTypeProjectionDelta(types api.CapturedTypes) ProjectionProduct {
	if len(types) == 0 {
		return ProjectionProduct{}
	}
	return JoinProjectionProduct(ProjectionProduct{}, ProjectionProduct{CapturedTypes: types})
}

// CapturedFieldProjectionDelta returns a canonical product delta for field writes
// performed by one nested function.
func CapturedFieldProjectionDelta(
	fnSym cfg.SymbolID,
	fields map[cfg.SymbolID]FieldValues,
) ProjectionProduct {
	if fnSym == 0 || len(fields) == 0 {
		return ProjectionProduct{}
	}
	normalized := make(map[cfg.SymbolID]FieldValues, len(fields))
	for sym, byField := range fields {
		if len(byField) > 0 {
			normalized[sym] = byField
		}
	}
	if len(normalized) == 0 {
		return ProjectionProduct{}
	}
	return JoinProjectionProduct(ProjectionProduct{}, ProjectionProduct{
		CapturedFields: api.CapturedFieldAssigns{fnSym: normalized},
	})
}

// ConstructorFieldProjectionDelta returns a canonical module product delta for one class.
func ConstructorFieldProjectionDelta(classSym cfg.SymbolID, fields FieldValues) ProjectionProduct {
	if classSym == 0 || len(fields) == 0 {
		return ProjectionProduct{}
	}
	return JoinProjectionProduct(ProjectionProduct{}, ProjectionProduct{
		ConstructorFields: api.ConstructorFields{classSym: fields},
	})
}
