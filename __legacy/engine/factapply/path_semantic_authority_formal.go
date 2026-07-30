package factapply

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
)

// ProjectFormal transports this authority's already-frozen visibility answers
// into one lexical formal-root keyspace. It changes only structural identity:
// every transaction continues to use the same resolver and semantic laws.
func (a *PathSemanticAuthority) ProjectFormal(
	domain state.ProductDomain,
	rekey state.CoordinateFormalRootRekey,
	target *keyspace.KeySpace,
) (*PathSemanticAuthority, error) {
	if a == nil || !a.Valid() || !domain.Valid() || target == nil || !target.Valid() {
		return nil, fmt.Errorf("factapply: invalid formal path semantic authority")
	}
	resolver, ok := a.resolver.ProjectKeySpace(target, func(source keyspace.Key) (keyspace.Key, bool) {
		mapped, err := domain.RekeyStructuralKeyFormal(rekey, source)
		return mapped, err == nil
	})
	if !ok {
		return nil, fmt.Errorf("factapply: formal path resolver projection failed")
	}
	formal := *a
	formal.resolver = resolver
	return &formal, nil
}

// CallBoundaryPathBindings freezes the sole placeholder/return-path mapping
// used by call effects. Concrete and formal factor programs consume this same
// binding; a formal authority projects the resolved keys without rebuilding
// call-site syntax in the transformer.
func (a *PathSemanticAuthority) CallBoundaryPathBindings(
	facts factflow.Facts,
	site factflow.CallSiteView,
) (callboundary.PathBindings, error) {
	if a == nil || !a.Valid() {
		return callboundary.PathBindings{}, fmt.Errorf("factapply: call-boundary paths have no semantic authority")
	}
	return callboundary.NewPathBindings(
		callPlaceholderBindings(facts, a.resolver, site),
		callReturnSlotBindings(site),
	), nil
}
