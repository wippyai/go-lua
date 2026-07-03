package diagnostics

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func (p discriminatedUnionExhaustiveness) registrationInvalidatedBeforeDispatch(result *body.Result, graph cfg.Graph, open []openRegistrationMutation, reg registrationCall, dispatch dispatchCall) bool {
	for _, mutation := range open {
		if !mutation.hasKey || mutation.key != reg.key {
			continue
		}
		if mutation.point == reg.point || mutation.point == dispatch.point ||
			!result.PointCanReach(reg.point, mutation.point) ||
			!result.PointCanReach(mutation.point, dispatch.point) {
			continue
		}
		if result.PathsAliasAtBoundary(mutation.point, mutation.registry, reg.registry) {
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
		if mutation.point == dispatch.point || !result.PointCanReach(mutation.point, dispatch.point) {
			continue
		}
		if mutation.hasKey {
			if result.PathsAliasAtBoundary(mutation.point, mutation.registry, dispatch.registry) &&
				mutation.key == c.key {
				return true
			}
			continue
		}
		if mutation.opensAll {
			if mutation.path.Overlaps(dispatch.registry) {
				return true
			}
			if mutation.aliasSensitive && result.PathsAliasAtBoundary(mutation.point, mutation.path, dispatch.registry) {
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
