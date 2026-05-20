package interproc

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/typ"
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
	fields map[cfg.SymbolID]map[string]typ.Type,
) api.Facts {
	if fnSym == 0 || len(fields) == 0 {
		return api.Facts{}
	}
	return JoinFacts(api.Facts{}, api.Facts{
		CapturedFields: api.CapturedFieldAssigns{fnSym: fields},
	})
}

// CapturedContainerMutationsDelta returns a canonical product delta for
// container writes performed by one nested function.
func CapturedContainerMutationsDelta(
	fnSym cfg.SymbolID,
	mutations map[cfg.SymbolID][]api.ContainerMutation,
) api.Facts {
	if fnSym == 0 || len(mutations) == 0 {
		return api.Facts{}
	}
	return JoinFacts(api.Facts{}, api.Facts{
		CapturedContainers: api.CapturedContainerMutations{fnSym: mutations},
	})
}

// ConstructorFieldsDelta returns a canonical module product delta for one class.
func ConstructorFieldsDelta(classSym cfg.SymbolID, fields map[string]typ.Type) api.Facts {
	if classSym == 0 || len(fields) == 0 {
		return api.Facts{}
	}
	return JoinFacts(api.Facts{}, api.Facts{
		ConstructorFields: api.ConstructorFields{classSym: fields},
	})
}
