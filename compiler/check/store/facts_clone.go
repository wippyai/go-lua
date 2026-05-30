package store

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

func cloneFunctionFacts(src api.FunctionFacts) api.FunctionFacts {
	if len(src) == 0 {
		return nil
	}
	out := make(api.FunctionFacts, len(src))
	for sym, fact := range src {
		out[sym] = cloneFunctionFact(fact)
	}
	return out
}

func cloneFunctionFact(fact api.FunctionFact) api.FunctionFact {
	fact.Params = cloneAbstractValueSlice(fact.Params)
	fact.BodyParams = cloneAbstractValueSlice(fact.BodyParams)
	fact.EntryParams = cloneAbstractValueSlice(fact.EntryParams)
	fact.Summary = cloneAbstractValueSlice(fact.Summary)
	fact.Narrow = cloneAbstractValueSlice(fact.Narrow)
	fact.EnvReturns = cloneEnvReturnSpecs(fact.EnvReturns)
	return fact
}

func cloneAbstractValueSlice(src []product.AbstractValue) []product.AbstractValue {
	if len(src) == 0 {
		return nil
	}
	out := make([]product.AbstractValue, len(src))
	copy(out, src)
	return out
}

func cloneEnvReturnSpecs(src []contract.EnvReturnSpec) []contract.EnvReturnSpec {
	if len(src) == 0 {
		return nil
	}
	out := make([]contract.EnvReturnSpec, len(src))
	for i, spec := range src {
		out[i] = spec
		out[i].Path = cloneSegments(spec.Path)
		out[i].Args = cloneTypeSlice(spec.Args)
	}
	return out
}

func cloneSegments(src []constraint.Segment) []constraint.Segment {
	if len(src) == 0 {
		return nil
	}
	out := make([]constraint.Segment, len(src))
	copy(out, src)
	return out
}

func cloneAbstractValueFieldMap(src map[string]product.AbstractValue) map[string]product.AbstractValue {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]product.AbstractValue, len(src))
	for name, v := range src {
		out[name] = v
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
		bySymOut := make(map[cfg.SymbolID]map[string]product.AbstractValue, len(bySym))
		for sym, fields := range bySym {
			if len(fields) == 0 {
				continue
			}
			bySymOut[sym] = cloneAbstractValueFieldMap(fields)
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

func cloneConstructorFieldMap(src map[string]product.AbstractValue) map[string]product.AbstractValue {
	return cloneAbstractValueFieldMap(src)
}

func cloneTypeSlice(src []typ.Type) []typ.Type {
	if len(src) == 0 {
		return nil
	}
	out := make([]typ.Type, len(src))
	copy(out, src)
	return out
}
