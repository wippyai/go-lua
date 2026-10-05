package check

import (
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

// exportArgumentContracts retains explicit declarations separately from inferred
// exports. Contracts follow function source identities, not exported field names.
func (s *Session) exportArgumentContracts() map[string]*io.ArgumentContract {
	var contracts map[string]*io.ArgumentContract
	for fn, result := range s.Results {
		if fn == nil || fn.ParList == nil || result == nil || result.Graph == nil || result.NarrowSynth == nil {
			continue
		}
		builder := typ.Func()
		declared := false
		for _, slot := range result.Graph.ParamSlotsReadOnly() {
			t := typ.Any
			if slot.TypeAnnotation != nil {
				t = result.NarrowSynth.ResolveType(slot.TypeAnnotation, result.BaseScope)
				declared = true
			}
			builder.Param(slot.Name, t)
		}
		if fn.ParList.VarargType != nil {
			builder.Variadic(result.NarrowSynth.ResolveType(fn.ParList.VarargType, result.BaseScope))
			declared = true
		}
		if !declared {
			continue
		}
		if contracts == nil {
			contracts = make(map[string]*io.ArgumentContract)
		}
		contracts[fn.SourceKey()] = &io.ArgumentContract{Signature: builder.Build(), Types: result.BaseScope.AllTypes()}
	}
	return contracts
}
