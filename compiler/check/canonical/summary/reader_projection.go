package summary

import (
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

// ReturnValues reads ref's summary through the live-or-converged summary
// boundary and projects its abstract return tuple defensively.
func (r Reader) ReturnValues(ref FuncRef) []product.AbstractValue {
	return ReturnValues(r.Summarize(ref))
}

// ReturnTypes reads ref through the summary boundary and projects its
// caller-visible concrete return tuple.
func (r Reader) ReturnTypes(ref FuncRef) []typ.Type {
	return ReturnTypes(r.Summarize(ref))
}

// ParamTypes reads ref's solved parameter-contract cell and projects it to the
// concrete projection shape keyed by parameter slot.
func (r Reader) ParamTypes(ref FuncRef) map[int]typ.Type {
	return paramevidence.ContractTypes(r.Summarize(ref).Params)
}

// ReturnRelations reads ref's caller-visible return-relation cell.
func (r Reader) ReturnRelations(ref FuncRef) flow.ReturnRelations {
	return r.Summarize(ref).Relations
}

// CallEntryPublication reads caller-to-callee entry evidence dep published for
// callee. Missing facts deliberately mean no finite aggregate proof.
func (r Reader) CallEntryPublication(dep FuncRef, callee FuncRef) CallEntryPublication {
	pub, ok := r.Summarize(dep).CallEntryPublication[callee]
	if !ok {
		return CallEntryPublication{Facts: flow.BoundaryFactsDomain.Top()}
	}
	return CallEntryPublication{
		Values: cloneEntryValues(pub.Values),
		Facts:  pub.Facts.Clone(),
	}
}

// PrototypeSelf reads dep's published prototype-self relation through the
// Reader boundary.
func (r Reader) PrototypeSelf(dep FuncRef) flow.PrototypeSelf {
	return flow.PrototypeSelfDomain.Join(r.Summarize(dep).PrototypeSelf, flow.PrototypeSelfDomain.Bottom())
}
