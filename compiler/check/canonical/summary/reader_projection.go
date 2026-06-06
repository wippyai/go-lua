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

// ReturnValuesWithKey reads an exact summary key and projects its abstract
// return tuple defensively.
func (r Reader) ReturnValuesWithKey(key Key) []product.AbstractValue {
	return ReturnValues(r.SummarizeWithKey(key))
}

// ReturnTypes reads ref through the summary boundary and projects its
// caller-visible concrete return tuple.
func (r Reader) ReturnTypes(ref FuncRef) []typ.Type {
	return ReturnTypes(r.Summarize(ref))
}

// ReturnTypesWithKey reads an exact summary key and projects its caller-visible
// concrete return tuple.
func (r Reader) ReturnTypesWithKey(key Key) []typ.Type {
	return ReturnTypes(r.SummarizeWithKey(key))
}

// ParamTypes reads ref's solved parameter-contract cell and projects it to the
// concrete bridge shape keyed by parameter slot.
func (r Reader) ParamTypes(ref FuncRef) map[int]typ.Type {
	return paramevidence.ContractTypes(r.Summarize(ref).Params)
}

// ReturnRelations reads ref's caller-visible return-relation cell.
func (r Reader) ReturnRelations(ref FuncRef) flow.ReturnRelations {
	return r.Summarize(ref).Relations
}

// CallEntryValues reads the caller-to-callee entry-value evidence dep published
// for callee. This lets diagnostic and projection observers consume
// EntryValueDependencies through the same live-or-converged Reader boundary
// instead of indexing Summary axes directly.
func (r Reader) CallEntryValues(dep FuncRef, callee FuncRef) EntryValues {
	return cloneEntryValues(r.Summarize(dep).CallEntryValues[callee])
}

// PrototypeSelf reads dep's published prototype-self relation through the
// Reader boundary.
func (r Reader) PrototypeSelf(dep FuncRef) flow.PrototypeSelf {
	return flow.PrototypeSelfDomain.Join(r.Summarize(dep).PrototypeSelf, flow.PrototypeSelfDomain.Bottom())
}
