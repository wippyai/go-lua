package transformer

import (
	"context"
	"fmt"

	"github.com/wippyai/go-lua/analysis/engine/callpayload"
)

func (a *formalTupleAlgebra) callOutcomeAlternatives(
	ctx context.Context,
	tuple formalRelationTuple,
	fiber formalCallOutcomeFiber,
) (callpayload.CallOutcomeAlternativeSet, error) {
	if ctx == nil || a == nil || !fiber.valid() || tuple.bottom() {
		return callpayload.CallOutcomeAlternativeSet{}, nil
	}
	span, directory, authority, ok := a.span(tuple.variable)
	ordinal, exact := span.ordinal(fiber.descriptor)
	if !ok || !exact || ordinal != fiber.ordinal || tuple.root.owner != directory || fiber.descriptor.variable != tuple.variable {
		return callpayload.CallOutcomeAlternativeSet{}, fmt.Errorf("transformer: formal call-outcome read has foreign ownership")
	}
	care, err := a.care(tuple)
	if err != nil || care == decisionFalse {
		return callpayload.CallOutcomeAlternativeSet{}, err
	}
	value, err := directory.valueAt(tuple.root, ordinal)
	if err != nil {
		return callpayload.CallOutcomeAlternativeSet{}, err
	}
	rows, err := a.decisions.partitionLeafTuplesUnderCare(ctx, care, []decisionRef{decisionRef(value)})
	if err != nil {
		return callpayload.CallOutcomeAlternativeSet{}, err
	}
	var out callpayload.CallOutcomeAlternativeSet
	for _, row := range rows {
		if row.care == decisionFalse || len(row.leaves) != 1 {
			return callpayload.CallOutcomeAlternativeSet{}, errDecisionMalformed
		}
		leaf, leafErr := a.componentLeaf(authority, fiber.descriptor, row.leaves[0])
		if leafErr != nil {
			return callpayload.CallOutcomeAlternativeSet{}, leafErr
		}
		terminal, leafErr := authority.terminal(leaf)
		if leafErr != nil || terminal.kind != formalComponentCallOutcomes {
			if leafErr != nil {
				return callpayload.CallOutcomeAlternativeSet{}, leafErr
			}
			return callpayload.CallOutcomeAlternativeSet{}, errFormalComponentMalformed
		}
		out = out.Join(authority.product.Registry(), terminal.callOutcomes)
	}
	return out.Normalize(authority.product.Registry()), nil
}
