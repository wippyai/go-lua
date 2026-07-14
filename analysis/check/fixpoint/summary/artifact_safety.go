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

func normalReturnFactsRetentionSafe(_ *axis.Registry, facts callboundary.NormalReturnFacts) bool {
	// The first shadow slice admits cloned structural branch proofs only. Every
	// other nested fact lane remains fail-closed until its owner adds a typed
	// retention rule.
	branchProofs := facts.BranchProofs
	facts.BranchProofs = nil
	return len(branchProofs) != 0 && facts.Empty()
}
