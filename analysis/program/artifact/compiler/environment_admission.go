package compiler

import (
	"crypto/sha256"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/causal"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/internal/framing"
)

// admitPointDecision joins an exact guarded final-route decision into the
// already parent-issued source point scope. pointDraft Site attachments describe
// ambient continuation guards, while the route proof is the sole authority
// for its edge-local guard. Keeping both sources in one canonical set avoids
// reconstructing scope from Terms at Link time.
func (compiler *compiler) admitPointDecision(point, decision identity.ContentID) bool {
	geometry, exists := compiler.pointGeometry[point]
	if !exists || !geometry.Available() || !decision.Available() {
		return false
	}
	if compiler.pointDecisionAdds == nil {
		compiler.pointDecisionAdds = make(map[identity.ContentID][]identity.ContentID)
	}
	compiler.pointDecisionAdds[point] = append(compiler.pointDecisionAdds[point], decision)
	return true
}

// canonicalizePointDecisionsFailure batches route-local decision admission by
// owner point. The old path inserted each decision into a sorted slice, moving
// O(D) tail elements for every route. One radix pass and in-place duplicate
// removal gives the same canonical owner-local order in O(D) work.
func (compiler *compiler) canonicalizePointDecisionsFailure() CompileFailure {
	for point, additions := range compiler.pointDecisionAdds {
		geometry, known := compiler.pointGeometry[point]
		if !known || !geometry.Available() {
			return compileFailure(CompileStageRoutes, CompileRowRoute, -1, -1, CompileReasonRouteGuard)
		}
		if len(additions) == 0 {
			continue
		}
		decisions := make([]identity.ContentID, 0, len(geometry.decisions)+len(additions))
		decisions = append(decisions, geometry.decisions...)
		decisions = append(decisions, additions...)
		identity.SortContentIDs(decisions)
		unique := 0
		for _, decision := range decisions {
			if !decision.Available() {
				return compileFailure(CompileStageRoutes, CompileRowRoute, -1, -1, CompileReasonRouteGuard)
			}
			if unique == 0 || decisions[unique-1] != decision {
				decisions[unique] = decision
				unique++
			}
		}
		geometry.decisions = decisions[:unique]
		compiler.pointGeometry[point] = geometry
	}
	return CompileFailure{}
}

func (compiler *compiler) emitRoutesFailure() CompileFailure {
	if compiler == nil || !compiler.input.Available() {
		return compileFailure(CompileStageRoutes, CompileRowRoute, -1, -1, CompileReasonRouteUnavailable)
	}
	routes := compiler.input.Flow().Causal().Successors()
	for index := 0; index < routes.TotalCount(); index++ {
		route, ok := routes.FinalAt(index)
		if !ok {
			return compileFailure(CompileStageRoutes, CompileRowRoute, index, -1, CompileReasonRouteUnavailable)
		}
		if failure := compiler.admitEnvironmentFailure(route, index); failure.Available() {
			return failure
		}
	}
	return CompileFailure{}
}

func (compiler *compiler) admitEnvironmentFailure(route causal.FinalRoute, rowIndex int) CompileFailure {
	if compiler == nil || !compiler.input.Available() || !route.Available() || !compiler.input.Flow().Causal().OwnsFinalRoute(route) {
		return compileFailure(CompileStageRoutes, CompileRowRoute, rowIndex, -1, CompileReasonRouteForeign)
	}
	from, fromOK := route.FromPoint()
	to, toOK := route.ToPoint()
	_, fromSiteOK := route.From()
	_, toSiteOK := route.To()
	routeID, routeOK := route.ID()
	arm, armOK := route.Arm()
	if !fromOK || !toOK || !fromSiteOK || !toSiteOK || !compiler.containsPoint(from) || !compiler.containsPoint(to) {
		return compileFailure(CompileStageRoutes, CompileRowRoute, rowIndex, -1, CompileReasonRouteEndpoints)
	}
	if !routeOK {
		return compileFailure(CompileStageRoutes, CompileRowRoute, rowIndex, -1, CompileReasonRouteIdentity)
	}
	if !armOK || arm < causal.BoundaryLocal || arm > causal.BoundaryCancel {
		return compileFailure(CompileStageRoutes, CompileRowRoute, rowIndex, -1, CompileReasonRouteArm)
	}
	occurrenceID := environmentRouteOccurrenceID(compiler.input.ContentID(), routeID, arm)
	if !occurrenceID.Available() {
		return compileFailure(CompileStageRoutes, CompileRowRoute, rowIndex, -1, CompileReasonRouteIdentity)
	}
	guardID, conditionID, guarded, truth := identity.ContentID{}, identity.ContentID{}, false, false
	decisionID := identity.ContentID{}
	if route.Guarded() {
		guard, guardOK := route.GuardProof()
		truthValue, truthOK := guard.Truth()
		if !guardOK || !truthOK || !guard.Available() {
			return compileFailure(CompileStageRoutes, CompileRowRoute, rowIndex, -1, CompileReasonRouteGuard)
		}
		guardID, guarded, truth = guard.ContextID(), true, truthValue
		if !guardID.Available() {
			return compileFailure(CompileStageRoutes, CompileRowRoute, rowIndex, -1, CompileReasonRouteGuard)
		}
		var decisionOK bool
		decisionID, decisionOK = guard.DecisionPathID()
		if !decisionOK || !decisionID.Available() || !compiler.admitPointDecision(from.PathID(), decisionID) {
			return compileFailure(CompileStageRoutes, CompileRowRoute, rowIndex, -1, CompileReasonRouteGuard)
		}
		identityValue, identityOK := route.Identity()
		if !identityOK {
			return compileFailure(CompileStageRoutes, CompileRowRoute, rowIndex, -1, CompileReasonRouteGuard)
		}
		conditionID = compiler.conditionValueSpanID(identityValue.Decision)
	}

	component, resetDigest := identity.ContentID{}, identity.ContentID{}
	hasReset := route.HasResetWitness()
	mu, hasMu := identity.ContentID{}, route.HasMu()
	if hasMu {
		var muOK bool
		mu, muOK = route.MuPathID()
		if !muOK || !mu.Available() {
			return compileFailure(CompileStageRoutes, CompileRowRoute, rowIndex, -1, CompileReasonRouteMu)
		}
	}
	resets := []identity.ContentID(nil)
	if recurrence, recurrenceOK := route.Recurrence(); recurrenceOK {
		proofRouteID, proofRouteIDOK := recurrence.RouteID()
		if !proofRouteIDOK || proofRouteID != routeID {
			return compileFailure(CompileStageRoutes, CompileRowRoute, rowIndex, -1, CompileReasonRouteRecurrence)
		}
		component = recurrence.ComponentID()
		if !component.Available() {
			return compileFailure(CompileStageRoutes, CompileRowRoute, rowIndex, -1, CompileReasonRouteRecurrence)
		}
		if hasReset {
			// ResetCount is a Mu/reset-witness capability. Ordinary
			// intra-component routes intentionally have no witness and their
			// parent proof returns (0,false), rather than fabricating an empty
			// reset interval. Only consume the count when the witness exists.
			count, countOK := recurrence.ResetCount()
			if !countOK || count < 0 {
				return compileFailure(CompileStageRoutes, CompileRowRoute, rowIndex, -1, CompileReasonRouteReset)
			}
			var resetOK bool
			resetDigest, resetOK = recurrence.ResetDigest()
			routeResetCount, routeResetOK := route.ResetCount()
			if !resetOK || !resetDigest.Available() || !routeResetOK || routeResetCount != count {
				return compileFailure(CompileStageRoutes, CompileRowRoute, rowIndex, -1, CompileReasonRouteReset)
			}
			resets = make([]identity.ContentID, count)
			for index := range resets {
				path, pathOK := route.ResetPathAt(index)
				if !pathOK || !path.Available() {
					return compileFailure(CompileStageRoutes, CompileRowRoute, rowIndex, index, CompileReasonRouteResetMember)
				}
				resets[index] = path
				if index != 0 && !contentIDBefore(resets[index-1], resets[index]) {
					return compileFailure(CompileStageRoutes, CompileRowRoute, rowIndex, index, CompileReasonRouteResetOrder)
				}
			}
		}
	} else if hasMu || hasReset {
		return compileFailure(CompileStageRoutes, CompileRowRoute, rowIndex, -1, CompileReasonRouteRecurrence)
	}
	if hasMu && !component.Available() || hasMu != hasReset {
		return compileFailure(CompileStageRoutes, CompileRowRoute, rowIndex, -1, CompileReasonRouteMuResetMismatch)
	}

	row := environmentEdgeDraft{
		id: occurrenceID, from: from.PathID(), to: to.PathID(), route: routeID,
		guard: guardID, decision: decisionID, condition: conditionID, guarded: guarded, truth: truth, component: component,
		mu: mu, hasMu: hasMu, reset: resetDigest, resets: resets, hasReset: hasReset, arm: arm,
	}
	if !row.Available() {
		return compileFailure(CompileStageRoutes, CompileRowEnvironment, rowIndex, -1, CompileReasonEnvironmentUnavailable)
	}
	compiler.environment = append(compiler.environment, row)
	if prior, exists := compiler.environmentByRoute[row.route]; exists {
		if prior.id != occurrenceID {
			return compileFailure(CompileStageRoutes, CompileRowRoute, rowIndex, -1, CompileReasonRouteIdentity)
		}
		compiler.environmentRouteDuplicates[row.route] = struct{}{}
	} else {
		compiler.environmentByRoute[row.route] = row
	}
	return CompileFailure{}
}

// environmentRouteOccurrenceID is the artifact-owned construction identity
// for one emitted environment row. It preserves the established route
// occurrence equation while consuming only the sealed route digest and arm
// supplied by Flow.
func environmentRouteOccurrenceID(programID, routeID identity.ContentID, arm causal.BoundaryArmKind) identity.ContentID {
	if !programID.Available() || !routeID.Available() {
		return identity.ContentID{}
	}
	hash := sha256.New()
	var writer framing.Writer
	if writer.Reset(hash, "program/transformer/structural-route", 1) != nil ||
		writer.Record(1) != nil || writer.Bytes(programID[:]) != nil ||
		writer.Bytes(routeID[:]) != nil || writer.Uint(uint64(arm)) != nil || writer.Finish() != nil {
		return identity.ContentID{}
	}
	var result identity.ContentID
	if sum := hash.Sum(result[:0]); len(sum) != len(result) {
		return identity.ContentID{}
	}
	return result
}

// conditionValueSpanID joins a Branch guard's canonical authored condition to
// its existing transformer Span only while the artifact row is being built.
// The resulting Span identity is copied into the immutable edge column; no
// authored term or live span is retained by Artifact.
func (compiler *compiler) conditionValueSpanID(decision keyspace.Term) identity.ContentID {
	if compiler == nil || !compiler.input.Available() || keyspace.TermFamily(decision) != keyspace.FamilyBranch {
		return identity.ContentID{}
	}
	_, condition, _, _, ok := compiler.input.Flow().Authored().Control().Branches().Get(decision)
	span, spanOK := compiler.input.Span(condition)
	if !ok || !spanOK || !compiler.input.OwnsSpan(span) {
		return identity.ContentID{}
	}
	return span.ContextID()
}
