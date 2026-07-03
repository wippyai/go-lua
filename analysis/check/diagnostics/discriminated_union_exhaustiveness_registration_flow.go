package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func (p discriminatedUnionExhaustiveness) registrationInvalidatedBeforeDispatch(result *body.Result, graph cfg.Graph, open []openRegistrationMutation, reg registrationCall, dispatch dispatchCall) bool {
	for _, mutation := range open {
		if !mutation.hasKey || mutation.key != reg.key {
			continue
		}
		if mutation.point == reg.point || mutation.point == dispatch.point ||
			!diagnosticCanReach(p.flow, graph, reg.point, mutation.point) ||
			!diagnosticCanReach(p.flow, graph, mutation.point, dispatch.point) {
			continue
		}
		if registrationRegistryMatchesAt(result, mutation.point, mutation.registry, reg.registry) {
			return true
		}
	}
	return false
}

func (p discriminatedUnionExhaustiveness) openRegistrationCanCoverCase(result *body.Result, graph cfg.Graph, open []openRegistrationMutation, dispatch dispatchCall, c discriminantCase) bool {
	for _, mutation := range open {
		if !mutation.mayRegister {
			continue
		}
		if mutation.point == dispatch.point || !diagnosticCanReach(p.flow, graph, mutation.point, dispatch.point) {
			continue
		}
		if mutation.hasKey {
			if registrationRegistryMatchesAt(result, mutation.point, mutation.registry, dispatch.registry) &&
				mutation.key == c.key {
				return true
			}
			continue
		}
		if mutation.opensAll {
			if mutation.path.Overlaps(dispatch.registry) {
				return true
			}
			if mutation.aliasSensitive && registrationRegistryMatchesAt(result, mutation.point, mutation.path, dispatch.registry) {
				return true
			}
			continue
		}
		if dispatch.registry.HasPrefix(mutation.path) {
			return true
		}
	}
	return false
}

func registrationRegistryMatchesAt(result *body.Result, point cfg.Point, left, right pathdom.Path) bool {
	if left.Equal(right) {
		return true
	}
	return result != nil && result.PathsEquivalentAtBoundary(point, left, right)
}
