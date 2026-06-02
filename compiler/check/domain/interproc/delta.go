package interproc

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
)

// FunctionFactsDelta returns a canonical product delta for function facts.
func FunctionFactsDelta(facts api.FunctionFacts) api.Facts {
	if len(facts) == 0 {
		return api.Facts{}
	}
	return JoinFacts(api.Facts{}, api.Facts{FunctionFacts: facts})
}

// LiteralSigsDelta returns a canonical product delta for literal signatures.
func LiteralSigsDelta(sigs api.LiteralSigs) api.Facts {
	if len(sigs) == 0 {
		return api.Facts{}
	}
	return JoinFacts(api.Facts{}, api.Facts{LiteralSigs: sigs})
}

// CapturedTypesDelta returns a canonical product delta for captured symbol types.
func CapturedTypesDelta(types api.CapturedTypes) api.Facts {
	if len(types) == 0 {
		return api.Facts{}
	}
	return JoinFacts(api.Facts{}, api.Facts{CapturedTypes: types})
}

// CapturedFieldAssignsDelta returns a canonical product delta for field writes
// performed by one nested function.
func CapturedFieldAssignsDelta(
	fnSym cfg.SymbolID,
	fields map[cfg.SymbolID]FieldValues,
) api.Facts {
	if fnSym == 0 || len(fields) == 0 {
		return api.Facts{}
	}
	normalized := make(map[cfg.SymbolID]FieldValues, len(fields))
	for sym, byField := range fields {
		if len(byField) > 0 {
			normalized[sym] = byField
		}
	}
	if len(normalized) == 0 {
		return api.Facts{}
	}
	return JoinFacts(api.Facts{}, api.Facts{
		CapturedFields: api.CapturedFieldAssigns{fnSym: normalized},
	})
}

// ConstructorFieldsDelta returns a canonical module product delta for one class.
func ConstructorFieldsDelta(classSym cfg.SymbolID, fields FieldValues) api.Facts {
	if classSym == 0 || len(fields) == 0 {
		return api.Facts{}
	}
	return JoinFacts(api.Facts{}, api.Facts{
		ConstructorFields: api.ConstructorFields{classSym: fields},
	})
}
