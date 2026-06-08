package flow

import (
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// BoundaryFactsFromFunctionType projects boundary-relative postconditions from
// an annotated function type. A non-function possibility makes the projection
// Top because callers cannot soundly consume a fact that is not proven for every
// runtime callee.
func BoundaryFactsFromFunctionType(fnType typ.Type) BoundaryFacts {
	if fnType == nil {
		return BoundaryFactsDomain.Top()
	}
	if u, ok := unwrap.Alias(fnType).(*typ.Union); ok {
		first := true
		acc := BoundaryFactsDomain.Bottom()
		for _, member := range u.Members {
			aliased := unwrap.Alias(member)
			if aliased == nil || aliased.Kind() == kind.Nil {
				continue
			}
			if unwrap.Function(member) == nil {
				return BoundaryFactsDomain.Top()
			}
			facts := BoundaryFactsFromSpec(contract.ExtractSpec(member))
			if first {
				acc = facts
				first = false
				continue
			}
			acc = BoundaryFactsDomain.Join(acc, facts)
		}
		if first {
			return BoundaryFactsDomain.Top()
		}
		return acc
	}
	if unwrap.Function(fnType) == nil {
		return BoundaryFactsDomain.Top()
	}
	return BoundaryFactsFromSpec(contract.ExtractSpec(fnType))
}

// BoundaryFactsFromSpec projects explicit boundary-relative labels from a
// contract. A missing contract is Top: it proves no finite caller-visible fact.
func BoundaryFactsFromSpec(spec *contract.Spec) BoundaryFacts {
	if spec == nil {
		return BoundaryFactsDomain.Top()
	}
	var lengthRelations []BoundaryLengthRelationFact
	for _, label := range spec.Effects.Labels {
		if rl, ok := label.(effect.ReturnLength); ok {
			if param, ok := rl.Length.(constraint.ParamLen); ok {
				lengthRelations = append(lengthRelations, BoundaryLengthRelationFact{
					Target: BoundaryPath{Kind: BoundaryPathReturn, Index: rl.ReturnIndex},
					Source: BoundaryPath{Kind: BoundaryPathParam, Index: param.Index},
				})
			}
		}
	}
	for _, ensure := range spec.ExprEnsures {
		if rel, ok := boundaryLengthRelationFromExprCompare(ensure); ok {
			lengthRelations = append(lengthRelations, rel)
		}
	}
	return BoundaryFactsDomain.Top().WithLengthRelations(lengthRelations)
}

func boundaryLengthRelationFromExprCompare(c constraint.ExprCompare) (BoundaryLengthRelationFact, bool) {
	if c.Rel != constraint.ExprGe && c.Rel != constraint.ExprEq {
		return BoundaryLengthRelationFact{}, false
	}
	left, ok := c.Left.(constraint.RetLen)
	if !ok {
		return BoundaryLengthRelationFact{}, false
	}
	right, ok := c.Right.(constraint.ParamLen)
	if !ok {
		return BoundaryLengthRelationFact{}, false
	}
	return BoundaryLengthRelationFact{
		Target: BoundaryPath{Kind: BoundaryPathReturn, Index: left.Index},
		Source: BoundaryPath{Kind: BoundaryPathParam, Index: right.Index},
	}, true
}
