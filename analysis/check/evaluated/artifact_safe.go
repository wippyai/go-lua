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

func canonicalArtifactValue(ctx context.Context, reg *axis.Registry, value product.Value) error {
	artifact, err := product.SealCanonical(ctx, reg, value)
	if err != nil {
		return err
	}
	if !artifact.Valid() {
		return product.ErrCanonicalMaterializationUnavailable
	}
	return nil
}

func normalizeArtifactSafeSummary(ctx context.Context, reg *axis.Registry, in summary.Summary) (summary.Summary, error) {
	out, err := summary.NormalizeContext(ctx, reg, in)
	if err != nil {
		return summary.Summary{}, err
	}
	if _, err := summary.SealCanonical(ctx, reg, out); err != nil {
		return summary.Summary{}, err
	}
	return out, nil
}
