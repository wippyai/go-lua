package activation

import (
	"github.com/wippyai/go-lua/analysis/identity"
	calldomain "github.com/wippyai/go-lua/domain/call"
)

// routeKeys is activation's immutable output for one canonical Call body.
// Semantic keys are computed once during binding and indexed by Call's body
// order; no Body, artifact, Program, role, or mount descriptor is mirrored.
type routeKeys struct {
	target   identity.SemanticKey
	endpoint identity.SemanticKey
}

func sealTargetRoutes(algebra *calldomain.Algebra) ([]routeKeys, bool) {
	if algebra == nil || !algebra.Valid() {
		return nil, false
	}
	bodies := algebra.Bodies()
	routes := make([]routeKeys, bodies.Count())
	for index := range routes {
		body, bodyOK := bodies.At(index)
		role, roleOK := body.RoleID()
		target, endpoint, semanticOK := callActivationRoleSemantics(role)
		if !bodyOK || !roleOK || !semanticOK {
			return nil, false
		}
		routes[index] = routeKeys{target: target, endpoint: endpoint}
	}
	return routes, true
}

const (
	callActivationTargetFrame   = uint64(1)
	callActivationEndpointFrame = uint64(2)
)

// callActivationRoleSemantics issues the two activation-owned semantic axes
// from Call's exact portable role. It is called only while sealing the private
// route column, never from a fold or admission projection.
func callActivationRoleSemantics(role calldomain.TargetRoleID) (identity.SemanticKey, identity.SemanticKey, bool) {
	id, ok := role.ContentID()
	if !ok {
		return identity.SemanticKey{}, identity.SemanticKey{}, false
	}
	target, targetOK := identity.NewSemanticKey([32]byte(id), callActivationTargetFrame)
	endpoint, endpointOK := identity.NewSemanticKey([32]byte(id), callActivationEndpointFrame)
	return target, endpoint, targetOK && endpointOK && target != endpoint
}
