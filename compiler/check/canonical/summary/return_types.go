package summary

import (
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

// FunctionSignatureWithSummaryReturns returns sig with the concrete return tuple
// projected from s. If s proves no return tuple, sig is returned unchanged.
func FunctionSignatureWithSummaryReturns(sig *typ.Function, s Summary) *typ.Function {
	return FunctionSignatureWithReturnTypes(sig, ReturnTypes(s))
}

// FunctionSignatureWithProjectedReturns returns sig unchanged when source
// declarations already own the return contract; otherwise it splices the
// caller-visible return tuple projected from s. Parameter contracts and other
// summary axes must not mutate the public signature shape here.
func FunctionSignatureWithProjectedReturns(sig *typ.Function, hasDeclaredReturns bool, s Summary) *typ.Function {
	if sig == nil || hasDeclaredReturns {
		return sig
	}
	return FunctionSignatureWithSummaryReturns(sig, s)
}

// FunctionSignatureWithReturnTypes returns sig with returns installed, preserving
// every other function component. Empty returns leave sig unchanged.
func FunctionSignatureWithReturnTypes(sig *typ.Function, returns []typ.Type) *typ.Function {
	if sig == nil {
		return nil
	}
	if len(returns) == 0 {
		return sig
	}
	return typejoin.WithReturns(sig, returns)
}
