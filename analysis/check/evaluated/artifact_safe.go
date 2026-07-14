package evaluated

import (
	"context"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

func artifactSafeValue(reg *axis.Registry, value product.Value) bool {
	return product.RetentionSafe(reg, value)
}

func normalizeArtifactSafeSummary(ctx context.Context, reg *axis.Registry, in summary.Summary) (summary.Summary, error) {
	return summary.NormalizeArtifactContext(ctx, reg, in)
}
