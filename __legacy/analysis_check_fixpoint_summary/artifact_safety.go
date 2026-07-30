package summary

import (
	"context"
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
)

// NormalizeArtifactContext uses the owner-maintained summary lane inventory to
// prove that every populated lane has an explicit immutable-retention rule.
// New lanes fail closed until their descriptor supplies that rule.
func NormalizeArtifactContext(ctx context.Context, reg *axis.Registry, in Summary) (Summary, error) {
	out, err := NormalizeContext(ctx, reg, in)
	if err != nil {
		return Summary{}, err
	}
	if !summaryRetentionSafe(reg, out) {
		return Summary{}, fmt.Errorf("summary: populated lane has no artifact-retention proof")
	}
	return out, nil
}

func productsRetentionSafe(reg *axis.Registry, values []product.Value) bool {
	for _, value := range values {
		if !product.RetentionSafe(reg, value) {
			return false
		}
	}
	return true
}

func normalReturnFactsRetentionSafe(reg *axis.Registry, facts callboundary.NormalReturnFacts) bool {
	// The shadow slice admits only cloned structural branch proofs and portable
	// placeholder refinements. Every other nested fact lane remains fail-closed
	// until its owner adds a typed retention rule.
	branchProofs := facts.BranchProofs
	pathRefinements := facts.PathRefinements
	facts.BranchProofs = nil
	facts.PathRefinements = nil
	if len(branchProofs) == 0 && len(pathRefinements) == 0 || !facts.Empty() {
		return false
	}
	for _, fact := range pathRefinements {
		if !fact.Path.IsPlaceholder() || !product.RetentionSafe(reg, fact.Value) {
			return false
		}
	}
	return true
}
