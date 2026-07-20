package factapply

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/symbol"
)

// PrepareRootAssignmentStablePathEvidence is the sole N4 semantic projection
// from an implication snapshot and assigned value to the representation-owned
// stable-root rewrite. Concrete State and guarded coordinate execution both
// call this function before applying the state-owned transaction.
func PrepareRootAssignmentStablePathEvidence(
	reg *axis.Registry,
	domain state.ProductDomain,
	keys *keyspace.KeySpace,
	snapshot pathevidence.PathPresenceImplicationsSnapshot,
	target symbol.ID,
	value product.Value,
	idempotent bool,
) (state.StableRootPathEvidenceMutation, error) {
	if reg == nil || !domain.Valid() || domain.Registry() != reg || keys == nil || !keys.Valid() || target == 0 || !product.BelongsToRegistry(reg, value) {
		return state.StableRootPathEvidenceMutation{}, fmt.Errorf("factapply: invalid stable-root path-evidence input")
	}
	preserve := make([]pathevidence.PathPresenceImplication, 0)
	if !idempotent && !snapshot.Bottom {
		for _, implication := range snapshot.Implications {
			matchesTrigger := stableRootImplicationEndpointMatchesSymbol(implication.Trigger, target)
			matchesTarget := stableRootImplicationEndpointMatchesSymbol(implication.Target, target)
			if !matchesTrigger && matchesTarget && rootAssignmentValueSatisfiesImplicationTarget(reg, value, implication) {
				preserve = append(preserve, implication)
			}
		}
	}
	return domain.SealStableRootPathEvidenceMutation(keys, target, idempotent, preserve)
}

func stableRootImplicationEndpointMatchesSymbol(candidate keyspace.Key, target symbol.ID) bool {
	if candidate.Sym != target || candidate.Segs != 0 {
		return false
	}
	switch candidate.Kind {
	case keyspace.KindUnversionedSym, keyspace.KindStableSym:
		return true
	default:
		return false
	}
}

func rootAssignmentValueSatisfiesImplicationTarget(
	reg *axis.Registry,
	value product.Value,
	implication pathevidence.PathPresenceImplication,
) bool {
	if implication.HasTargetValue {
		return product.Domain(reg).LessOrEq(value, implication.TargetValue)
	}
	if !presence.Equal(implication.TargetPresence, presence.Present()) && !presence.Equal(implication.TargetPresence, presence.Absent()) {
		return false
	}
	constraint := product.NewWithPresence(reg, product.ShapeTop, implication.TargetPresence)
	return product.Domain(reg).LessOrEq(value, constraint)
}
