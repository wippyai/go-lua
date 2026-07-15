package transformer

import (
	"context"
	"fmt"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
)

func (r Relation) validateCallbackFreeEvaluatedSummaryTerms(ctx context.Context) error {
	if r.arena == nil {
		return fmt.Errorf("transformer: evaluated summary has no term arena")
	}
	seen := make(map[ValueTerm]bool)
	steps := uint64(0)
	check := func(term ValueTerm) error { return r.arena.validateCallbackFreeValueTerm(ctx, term, seen, &steps) }
	for rowIndex, row := range r.rows {
		if rowIndex&63 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		if len(row.Effects) != 0 {
			return fmt.Errorf("transformer: evaluated summary rejects effects")
		}
		if err := r.arena.validateCallbackFreeGuard(ctx, row.Guard, seen, &steps); err != nil {
			return err
		}
		for _, operation := range row.Ops {
			if err := check(operation.Value); err != nil {
				return err
			}
		}
		for _, proof := range row.Proofs {
			if proof.Key != 0 {
				if err := check(proof.Key); err != nil {
					return err
				}
			}
		}
		for _, refinement := range row.PathRefinements {
			if err := check(refinement.Value); err != nil {
				return err
			}
		}
	}
	return ctx.Err()
}

// evaluateOwnerSummary specializes the owner summary with the same memoized,
// cancellable evaluator used for every selector and boundary product. It is a
// deliberately narrow copy of the established specialization semantics: the
// shadow slice rejects effects and callbacks before entering this function.
func (r Relation) evaluateOwnerSummary(ctx context.Context, evaluator *evaluatedTermEvaluator) (summary.Summary, error) {
	if evaluator == nil || evaluator.arena != r.arena {
		return summary.Summary{}, fmt.Errorf("transformer: evaluated summary has foreign evaluator")
	}
	descriptors := r.descriptors
	if descriptors == nil {
		descriptors = DefaultDescriptorRegistry()
	}
	reg := r.arena.reg
	var accumulated summary.Summary
	var candidates []summary.Summary
	var rawCandidateReturns [][]product.Value
	have := false
	for rowIndex, row := range r.rows {
		if rowIndex&63 == 0 {
			if err := ctx.Err(); err != nil {
				return summary.Summary{}, err
			}
		}
		feasible, err := evaluator.guard(row.Guard)
		if err != nil {
			return summary.Summary{}, err
		}
		if !feasible {
			continue
		}
		if len(row.Effects) != 0 {
			return summary.Summary{}, fmt.Errorf("transformer: evaluated summary rejects effects")
		}
		candidate := row.Output.Clone()
		var rawReturns []product.Value
		if r.inferReturnCorrelations {
			rawReturns = append(rawReturns, row.Output.Returns...)
		}
		for _, operation := range row.Ops {
			handler := descriptors.handlers[operation.Descriptor]
			if handler == nil || row.Guard != r.arena.True() && !handler.ConditionalAllowed() {
				return summary.Summary{}, fmt.Errorf("transformer: evaluated summary descriptor rejected")
			}
			value, err := evaluator.value(operation.Value)
			if err != nil {
				return summary.Summary{}, err
			}
			if err := handler.Apply(reg, &candidate, operation.Slot, value); err != nil {
				return summary.Summary{}, err
			}
			if operation.Descriptor != DescriptorReturn {
				continue
			}
			if r.inferReturnCorrelations {
				priorLen := len(rawReturns)
				for len(rawReturns) <= int(operation.Slot) {
					rawReturns = append(rawReturns, product.Bottom(reg))
				}
				if int(operation.Slot) >= priorLen {
					rawReturns[operation.Slot] = value
				} else {
					rawReturns[operation.Slot] = summary.JoinReturnValue(reg, rawReturns[operation.Slot], value)
				}
			}
			if param, exact := r.arena.directParamRoot(operation.Value); exact {
				if r.authority.allowsSummary("ReturnFlows") && r.authority.allowsSummary("ReturnParamPathAliases") {
					placeholder, ok := pathaddr.PlaceholderKeyFromPath(pathdom.NewPlaceholder(param))
					if !ok {
						return summary.Summary{}, fmt.Errorf("transformer: evaluated summary cannot encode return parameter")
					}
					candidate.ReturnFlows = append(candidate.ReturnFlows, summary.ReturnFlow{
						ReturnIndex: int(operation.Slot), Kind: summary.ReturnFlowParam, Param: param,
					})
					candidate.ReturnParamPathAliases = append(candidate.ReturnParamPathAliases, summary.ReturnParamPathAlias{
						ReturnIndex: int(operation.Slot), Source: placeholder,
					})
				}
			} else if param, exact := r.arena.refinedParamRoot(operation.Value); exact && r.authority.allowsSummary("ReturnParamPathAliases") {
				placeholder, ok := pathaddr.PlaceholderKeyFromPath(pathdom.NewPlaceholder(param))
				if !ok {
					return summary.Summary{}, fmt.Errorf("transformer: evaluated summary cannot encode refined return parameter")
				}
				candidate.ReturnParamPathAliases = append(candidate.ReturnParamPathAliases, summary.ReturnParamPathAlias{
					ReturnIndex: int(operation.Slot), Source: placeholder,
				})
			}
		}
		if (len(row.Proofs) != 0 || len(row.PathRefinements) != 0) && !r.authority.allowsSummary("NormalReturnFacts") {
			return summary.Summary{}, fmt.Errorf("transformer: evaluated summary lacks normal-return-fact authority")
		}
		for _, proof := range row.Proofs {
			tablePath, valid := proof.placeholderPath(r.arena)
			if !valid {
				return summary.Summary{}, fmt.Errorf("transformer: evaluated summary has invalid proof path")
			}
			proofPath := tablePath.Clone()
			if proof.Key != 0 {
				key, err := evaluator.value(proof.Key)
				if err != nil {
					return summary.Summary{}, err
				}
				keySegment, valid := typevalue.ExactScalarKeySegment(reg, nil, key)
				if !valid {
					return summary.Summary{}, fmt.Errorf("transformer: evaluated summary proof key is not exact")
				}
				proofPath.Segments = append(proofPath.Segments, keySegment)
			}
			candidate.NormalReturnFacts.BranchProofs = append(candidate.NormalReturnFacts.BranchProofs, callboundary.BranchProof{
				Kind: proof.Kind, Path: proofPath, Presence: proof.Presence,
			})
		}
		for _, refinement := range row.PathRefinements {
			root, valid := refinement.preservedRoot(r.arena)
			if !valid || root.Kind != RootParam {
				return summary.Summary{}, fmt.Errorf("transformer: evaluated summary has unsupported preserved root")
			}
			value, err := evaluator.value(refinement.Value)
			if err != nil {
				return summary.Summary{}, err
			}
			value, keep := callboundary.ProjectPathRefinementValue(reg, value)
			if !keep {
				continue
			}
			candidate.NormalReturnFacts.PathRefinements = append(candidate.NormalReturnFacts.PathRefinements, callboundary.PathValueFact{
				Path: pathdom.NewPlaceholder(int(root.Index)), Value: value,
			})
		}
		if !have {
			accumulated, have = candidate, true
		} else {
			accumulated = summary.Join(reg, accumulated, candidate)
		}
		if r.inferReturnCorrelations {
			candidates = append(candidates, candidate)
			rawCandidateReturns = append(rawCandidateReturns, rawReturns)
		}
	}
	if !have {
		return summary.Summary{}, nil
	}
	accumulated.ReturnParamPathAliases = append(accumulated.ReturnParamPathAliases, r.projection.returnParamPathAliases...)
	if r.inferReturnCorrelations {
		var declared []product.Value
		if handler, ok := descriptors.handlers[DescriptorReturn].(returnHandler); ok {
			declared = handler.declared
		}
		conditions, relations, exact := inferReturnRowCorrelations(reg, candidates, rawCandidateReturns, declared)
		if !exact {
			return summary.Summary{}, fmt.Errorf("transformer: evaluated summary return correlation is inexact")
		}
		if len(conditions) != 0 && !r.authority.allowsSummary("ReturnConditionSlotRefinements") {
			return summary.Summary{}, fmt.Errorf("transformer: evaluated summary lacks return-condition authority")
		}
		if len(relations) != 0 && !r.authority.allowsSummary("ReturnPresenceRelations") {
			return summary.Summary{}, fmt.Errorf("transformer: evaluated summary lacks return-presence authority")
		}
		accumulated.ReturnConditionSlotRefinements = append(accumulated.ReturnConditionSlotRefinements, conditions...)
		accumulated.ReturnPresenceRelations = append(accumulated.ReturnPresenceRelations, relations...)
	}
	return summary.NormalizeContext(ctx, reg, accumulated)
}
