package flow

import (
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// ReturnRelationsFromFunctionType projects caller-visible return relations from
// an annotated function type. A non-function possibility makes the projection
// Top because callers cannot soundly consume a relation that is not proven for
// every runtime callee.
func ReturnRelationsFromFunctionType(fnType typ.Type) ReturnRelations {
	if fnType == nil {
		return ReturnRelationsDomain.Top()
	}
	if u, ok := unwrap.Alias(fnType).(*typ.Union); ok {
		first := true
		acc := ReturnRelationsDomain.Bottom()
		for _, member := range u.Members {
			aliased := unwrap.Alias(member)
			if aliased == nil || aliased.Kind() == kind.Nil {
				continue
			}
			if unwrap.Function(member) == nil {
				return ReturnRelationsDomain.Top()
			}
			rels := ReturnRelationsFromSpec(contract.ExtractSpec(member))
			if first {
				acc = rels
				first = false
				continue
			}
			acc = ReturnRelationsDomain.Join(acc, rels)
		}
		if first {
			return ReturnRelationsDomain.Top()
		}
		return acc
	}
	if unwrap.Function(fnType) == nil {
		return ReturnRelationsDomain.Top()
	}
	return ReturnRelationsFromSpec(contract.ExtractSpec(fnType))
}

// ReturnRelationsFromSpec projects explicit return-relation labels from a
// contract. A missing contract is Top: it proves no finite caller-visible
// relation.
func ReturnRelationsFromSpec(spec *contract.Spec) ReturnRelations {
	if spec == nil {
		return ReturnRelationsDomain.Top()
	}
	var out []ReturnCorrelation
	for _, label := range spec.Effects.Labels {
		if er, ok := label.(effect.ErrorReturn); ok {
			out = append(out, ReturnCorrelation{ValueIndex: er.ValueIndex, ErrorIndex: er.ErrorIndex})
		}
	}
	return ReturnRelationsOfErrorReturns(out)
}
