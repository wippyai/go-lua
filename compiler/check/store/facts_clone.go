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
	fact.Call.Params = cloneAbstractValueSlice(fact.Call.Params)
	fact.Body.Params = cloneAbstractValueSlice(fact.Body.Params)
	fact.Entry.Params = cloneAbstractValueSlice(fact.Entry.Params)
	fact.Returns.Preflow = cloneAbstractValueSlice(fact.Returns.Preflow)
	fact.Returns.Postflow = cloneAbstractValueSlice(fact.Returns.Postflow)
	fact.Export.EnvReturns = cloneEnvReturnSpecs(fact.Export.EnvReturns)
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

func cloneAbstractValueFieldMap(src api.FieldValues) api.FieldValues {
	if len(src) == 0 {
		return nil
	}
	out := make(api.FieldValues, len(src))
	for key, v := range src {
		out[key] = v
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
		bySymOut := make(map[cfg.SymbolID]api.FieldValues, len(bySym))
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

func cloneConstructorFieldMap(src api.FieldValues) api.FieldValues {
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
