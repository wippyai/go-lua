package state

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
)

const LanePathEvidence LaneID = "path-evidence"

var pathEvidenceLaneSpec = laneSpec{
	id:           LanePathEvidence,
	keySpaceMode: laneKeySpaceOwned,
	boundary:     boundaryLaneOps{project: projectPathEvidenceBoundary, rebase: rebasePathEvidenceBoundary, postRebase: postRebasePathEvidenceBoundary, equal: equalPathEvidenceBoundary},
	rekey: func(s State, from, to *keyspace.KeySpace) (State, bool) {
		lane, ok := s.pathEvidence.RekeyValueLanes(from, to)
		if !ok {
			return s, false
		}
		s.pathEvidence = lane
		return s, true
	},
	valueDependencies: enumeratedValueDependencies(visitPathEvidenceValueDependencies),
	identitySupport: laneIdentitySupportPolicy{
		kind: laneIdentitiesEnumerated,
		visit: func(reg *axis.Registry, payload laneFactorPayload, visit func(identity.Term) bool) bool {
			return visitPathEvidenceFactorIdentities(reg, typedLaneFactorValue[pathevidence.Lane](payload), visit)
		},
		visitState: func(reg *axis.Registry, source State, visit func(identity.Term) bool) bool {
			return visitPathEvidenceLaneIdentities(reg, nil, source.pathEvidence, visit)
		},
		visitStateKeys: func(reg *axis.Registry, source State, keys *keyspace.KeySpace, visit func(identity.Term) bool) bool {
			return visitPathEvidenceLaneIdentities(reg, keys, source.pathEvidence, visit)
		},
		image: IdentityImageEmbeddedValue,
	},
	numericConsistency: numericConsistencyIndependent(),
	semanticLaws: []laneSemanticLaw{pathSubtreeMutationLane(
		func(s State) pathevidence.Lane { return s.pathEvidence },
		func(out *State, lane pathevidence.Lane) { out.pathEvidence = lane },
		func(lane pathevidence.Lane, keys *keyspace.KeySpace, prefixes []pathdom.PathKey, _ pathdom.PathKey) (pathevidence.Lane, bool, bool) {
			if keys == nil || !keys.Valid() {
				return lane, false, false
			}
			next, changed := lane.InvalidatePathKeySubtreePrefixesChanged(keys, prefixes)
			return next, changed, true
		},
	), pathDescendantMutationLane(
		func(s State) pathevidence.Lane { return s.pathEvidence },
		func(out *State, lane pathevidence.Lane) { out.pathEvidence = lane },
		func(lane pathevidence.Lane, keys *keyspace.KeySpace, prefixes pathevidence.PathKeyDescendantInvalidationPrefixes, _ pathdom.PathKey) (pathevidence.Lane, bool, bool) {
			if keys == nil || !keys.Valid() {
				return lane, false, false
			}
			next, changed := lane.InvalidatePathKeyDescendantPrefixesChanged(keys, prefixes)
			return next, changed, true
		},
	), pathResolutionIndependent(), pathEqualityQuotientIndependent(), genericForPathEvidenceBinding(), pathReplacementLane[pathevidence.Lane](false, true, true, applyPathReplacementPathLane), effectFactorIndependent(), callBoundaryIndependent()},
	boundaryClosureCompanion: noBoundaryClosureCompanion(),
	rootAssignment:           rootAssignmentUnchanged(true, true, true),
	dynamicRead:              dynamicReadIndependent(),
	coordinateFamilies:       []coordinateFamilySpec{pathEvidenceCoordinateFamilySpec},
	fingerprint:              fingerprintPathEvidence,
	markReachable: func(s State) State {
		s.pathEvidence = s.pathEvidence.Reachable()
		return s
	},
	build: func(reg *axis.Registry, _ DomainOptions) laneOps {
		domain := pathevidence.Domain(reg)
		return stateLaneWithBoundary(domain,
			func(s State) pathevidence.Lane { return s.pathEvidence },
			func(out *State, lane pathevidence.Lane) { out.pathEvidence = lane },
			typedLaneFactorRepresentation[pathevidence.Lane]{equal: domain.Equal},
			typedBoundaryFactorOps[pathevidence.Lane]{
				apply: applyPathEvidenceBoundaryLane, roots: boundaryRootsPathValuesAndReachability(applyPathEvidenceBoundaryRoots),
				project: projectPathEvidenceBoundaryFactor, rebase: rebasePathEvidenceBoundaryFactor,
				postRebase: postRebasePathEvidenceBoundaryFactor, reachability: emitPathEvidenceReachability,
			},
		)
	},
}

func visitPathEvidenceValueDependencies(source State, keys *keyspace.KeySpace, visit func(statekey.ValueDependency)) {
	visitPath := func(path keyspace.Key) {
		if dependency, ok := pathevidence.PathValueDependency(keys, path); ok {
			visit(dependency)
		}
	}
	source.pathEvidence.ForEachPathRefinement(keys, func(path keyspace.Key, _ product.Value) bool {
		visitPath(path)
		return true
	})
	source.pathEvidence.ForEachPathStaticMember(keys, func(path keyspace.Key, _ product.Value) bool {
		visitPath(path)
		return true
	})
	source.pathEvidence.ForEachBranchProof(func(proof pathevidence.BranchProof) bool {
		visitPath(proof.Path)
		visitPath(proof.Other)
		return true
	})
	source.pathEvidence.ForEachPathPresenceImplication(func(implication pathevidence.PathPresenceImplication) bool {
		visitPath(implication.Trigger)
		visitPath(implication.TriggerOther)
		visitPath(implication.Target)
		return true
	})
}

func emitPathEvidenceReachability(program *boundaryReachabilityProgramBuilder, lane pathevidence.Lane) {
	lane.ForEachPathRefinement(program.keys, func(path keyspace.Key, value product.Value) bool {
		if program.pathCone(false, path) {
			program.addValue(value)
		}
		return true
	})
	lane.ForEachPathStaticMember(program.keys, func(path keyspace.Key, value product.Value) bool {
		if program.pathCone(false, path) {
			program.addValue(value)
		}
		return true
	})
	lane.ForEachBranchProof(func(proof pathevidence.BranchProof) bool {
		program.pathCone(false, proof.Path, proof.Other)
		return true
	})
	lane.ForEachPathPresenceImplication(func(implication pathevidence.PathPresenceImplication) bool {
		if program.pathCone(false, implication.Trigger, implication.TriggerOther, implication.Target) {
			if implication.HasTriggerValue {
				program.addValue(implication.TriggerValue)
			}
			if implication.HasTargetValue {
				program.addValue(implication.TargetValue)
			}
		}
		return true
	})
}
