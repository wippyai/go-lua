package summary

import (
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
	typejoin "github.com/wippyai/go-lua/types/typ/join"
)

// ReturnTypes projects the abstract return tuple in s to caller-visible concrete
// types. It is summary algebra: callers should not inspect Summary.Returns
// directly when they only need the public return tuple.
func ReturnTypes(s Summary) []typ.Type {
	if len(s.Returns) == 0 {
		return nil
	}
	out := make([]typ.Type, len(s.Returns))
	for i, av := range s.Returns {
		out[i] = product.ProjectValueOrUnknown(av)
	}
	return out
}

// ReturnValues exposes the abstract return tuple in s without leaking mutable
// backing storage. Transfer-level consumers use this when they need the full
// product carrier rather than the concrete-type projection.
func ReturnValues(s Summary) []product.AbstractValue {
	if len(s.Returns) == 0 {
		return nil
	}
	out := make([]product.AbstractValue, len(s.Returns))
	copy(out, s.Returns)
	return out
}

// FunctionSignatureWithProjectedReturns returns sig unchanged when source
// declarations already own the return contract; otherwise it splices the
// caller-visible return tuple projected from s. Parameter contracts and other
// summary axes must not mutate the public signature shape here.
func FunctionSignatureWithProjectedReturns(sig *typ.Function, hasDeclaredReturns bool, s Summary) *typ.Function {
	if sig == nil || hasDeclaredReturns {
		return sig
	}
	return functionSignatureWithReturnTypes(sig, ReturnTypes(s))
}

// FunctionSignatureWithEntryParamsAndProjectedReturns is the callable-value
// boundary for exact entry contexts. EntryValues are caller-provided product
// facts, so they can refine gradual source parameters before return projection
// feeds higher-order generic inference.
func FunctionSignatureWithEntryParamsAndProjectedReturns(sig *typ.Function, hasDeclaredReturns bool, s Summary, values EntryValues) *typ.Function {
	if sig == nil {
		return nil
	}
	out := functionSignatureWithEntryParams(sig, values)
	out = functionSignatureWithProjectedParams(out, s)
	if hasDeclaredReturns {
		return out
	}
	return functionSignatureWithReturnTypes(out, ReturnTypes(s))
}

// functionSignatureWithProjectedParams refines only gradual source parameter
// slots from solved parameter contracts. Concrete annotations remain
// authoritative.
func functionSignatureWithProjectedParams(sig *typ.Function, s Summary) *typ.Function {
	if sig == nil || len(sig.Params) == 0 || len(s.Params) == 0 {
		return sig
	}
	paramTypes := paramevidence.ContractTypes(s.Params)
	if len(paramTypes) == 0 {
		return sig
	}
	params := append([]typ.Param(nil), sig.Params...)
	changed := false
	for slot, t := range paramTypes {
		if slot < 0 || slot >= len(params) || t == nil || typ.IsAbsentOrUnknown(t) || typ.IsAny(t) || typ.ContainsTypeParam(t) {
			continue
		}
		current := params[slot].Type
		if current != nil && !typ.IsAbsentOrUnknown(current) && !typ.IsAny(current) {
			continue
		}
		params[slot].Type = t
		changed = true
	}
	if !changed {
		return sig
	}
	return functionSignatureWithParams(sig, params)
}

// functionSignatureWithEntryParams refines gradual parameter slots from exact
// entry values. The EntryValues producer already suppresses fixed source
// annotations, so this helper only has to preserve non-gradual signature slots.
func functionSignatureWithEntryParams(sig *typ.Function, values EntryValues) *typ.Function {
	if sig == nil || len(sig.Params) == 0 || len(values) == 0 {
		return sig
	}
	params := append([]typ.Param(nil), sig.Params...)
	changed := false
	for slot, av := range values {
		if slot < 0 || slot >= len(params) || av.IsZero() {
			continue
		}
		t := product.ProjectValueOrUnknown(av)
		if t == nil || typ.IsAbsentOrUnknown(t) || typ.IsAny(t) || typ.ContainsTypeParam(t) {
			continue
		}
		current := params[slot].Type
		if current != nil && !typ.IsAbsentOrUnknown(current) && !typ.IsAny(current) {
			continue
		}
		params[slot].Type = t
		changed = true
	}
	if !changed {
		return sig
	}
	return functionSignatureWithParams(sig, params)
}

func functionSignatureWithReturnTypes(sig *typ.Function, returns []typ.Type) *typ.Function {
	if sig == nil {
		return nil
	}
	if len(returns) == 0 {
		return sig
	}
	return typejoin.WithReturns(sig, returns)
}

func functionSignatureWithParams(sig *typ.Function, params []typ.Param) *typ.Function {
	if sig == nil {
		return nil
	}
	builder := typ.Func().ReserveParams(len(params))
	for _, tp := range sig.TypeParams {
		builder = builder.TypeParamRef(tp)
	}
	for _, param := range params {
		if param.Optional {
			builder = builder.OptParam(param.Name, param.Type)
		} else {
			builder = builder.Param(param.Name, param.Type)
		}
	}
	if sig.Variadic != nil {
		builder = builder.Variadic(sig.Variadic)
	}
	if len(sig.Returns) > 0 {
		builder = builder.Returns(sig.Returns...)
	}
	if sig.Effects != nil {
		builder = builder.Effects(sig.Effects)
	}
	if sig.Spec != nil {
		builder = builder.Spec(sig.Spec)
	}
	if sig.Refinement != nil {
		builder = builder.WithRefinement(sig.Refinement)
	}
	return builder.Build()
}
