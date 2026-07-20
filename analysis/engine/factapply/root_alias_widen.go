package factapply

import (
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// covariantExposureSuppressesPathProof reports whether a bare-root covariant
// record exposure at point widens the root symbol named by source. Such an
// exposure suppresses the wide==narrow path-equality proof: a record's per-field
// facts flow through reference-equality member congruence, so the equality would
// reset the narrow source to Top on a write through the alias. An array exposure
// carries no per-member congruence and relies on the equality proof for its
// existing read-back diagnostics, so it keeps the equality. A sub-path exposure
// adds no such equality, so it needs no suppression.
func covariantExposureSuppressesPathProof(facts factflow.Facts, resolver *visibility.Resolver, point cfg.Point, source factflow.ValueSource) bool {
	sourcePath, ok := sourcePathFromValueSource(resolver, facts, source)
	if !ok || sourcePath.Symbol == 0 || len(sourcePath.Segments) != 0 {
		return false
	}
	for _, exposure := range facts.CovariantExposures(point) {
		if exposure.Kind() != factflow.CovariantExposureRecord {
			continue
		}
		ep := exposure.SourcePath()
		if ep.Symbol == sourcePath.Symbol && len(ep.Segments) == 0 {
			return true
		}
	}
	return false
}
