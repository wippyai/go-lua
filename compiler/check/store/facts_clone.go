package store

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/typ"
)

func cloneFacts(f api.Facts) api.Facts {
	if factsEmpty(f) {
		return api.Facts{}
	}
	return api.Facts{
		FunctionFacts:      cloneFunctionFacts(f.FunctionFacts),
		LiteralSigs:        cloneLiteralSigs(f.LiteralSigs),
		CapturedTypes:      cloneCapturedTypes(f.CapturedTypes),
		CapturedFields:     cloneCapturedFieldAssigns(f.CapturedFields),
		CapturedContainers: cloneCapturedContainerMutations(f.CapturedContainers),
		ConstructorFields:  cloneConstructorFields(f.ConstructorFields),
	}
}

func cloneFunctionFacts(src api.FunctionFacts) api.FunctionFacts {
	if len(src) == 0 {
		return nil
	}
	out := make(api.FunctionFacts, len(src))
	for sym, fact := range src {
		fact.Params = cloneTypeSlice(fact.Params)
		fact.Summary = cloneTypeSlice(fact.Summary)
		fact.Narrow = cloneTypeSlice(fact.Narrow)
		out[sym] = fact
	}
	return out
}

func cloneLiteralSigs(src api.LiteralSigs) api.LiteralSigs {
	if len(src) == 0 {
		return nil
	}
	out := make(map[*ast.FunctionExpr]*typ.Function, len(src))
	for fn, sig := range src {
		out[fn] = sig
	}
	return out
}

func cloneCapturedTypes(src api.CapturedTypes) api.CapturedTypes {
	if len(src) == 0 {
		return nil
	}
	out := make(api.CapturedTypes, len(src))
	for sym, t := range src {
		out[sym] = t
	}
	return out
}

func cloneCapturedFieldAssigns(src api.CapturedFieldAssigns) api.CapturedFieldAssigns {
	if len(src) == 0 {
		return nil
	}
	out := make(api.CapturedFieldAssigns, len(src))
	for callee, bySym := range src {
		if len(bySym) == 0 {
			continue
		}
		bySymOut := make(map[cfg.SymbolID]map[string]typ.Type, len(bySym))
		for sym, fields := range bySym {
			if len(fields) == 0 {
				continue
			}
			fieldOut := make(map[string]typ.Type, len(fields))
			for name, t := range fields {
				fieldOut[name] = t
			}
			bySymOut[sym] = fieldOut
		}
		if len(bySymOut) > 0 {
			out[callee] = bySymOut
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func cloneCapturedContainerMutations(src api.CapturedContainerMutations) api.CapturedContainerMutations {
	if len(src) == 0 {
		return nil
	}
	out := make(api.CapturedContainerMutations, len(src))
	for callee, bySym := range src {
		if len(bySym) == 0 {
			continue
		}
		bySymOut := make(map[cfg.SymbolID][]api.ContainerMutation, len(bySym))
		for sym, muts := range bySym {
			if len(muts) == 0 {
				continue
			}
			mutsOut := make([]api.ContainerMutation, len(muts))
			copy(mutsOut, muts)
			for i := range mutsOut {
				if len(mutsOut[i].Segments) > 0 {
					mutsOut[i].Segments = append(mutsOut[i].Segments[:0:0], mutsOut[i].Segments...)
				}
			}
			bySymOut[sym] = mutsOut
		}
		if len(bySymOut) > 0 {
			out[callee] = bySymOut
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func cloneConstructorFields(src api.ConstructorFields) api.ConstructorFields {
	if len(src) == 0 {
		return nil
	}
	out := make(api.ConstructorFields, len(src))
	for sym, fields := range src {
		if len(fields) == 0 {
			continue
		}
		fieldOut := make(map[string]typ.Type, len(fields))
		for name, t := range fields {
			fieldOut[name] = t
		}
		out[sym] = fieldOut
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func cloneTypeSlice(src []typ.Type) []typ.Type {
	if len(src) == 0 {
		return nil
	}
	out := make([]typ.Type, len(src))
	copy(out, src)
	return out
}
