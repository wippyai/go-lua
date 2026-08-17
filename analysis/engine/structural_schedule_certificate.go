package engine

import (
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/internal/schedule"
	"github.com/wippyai/go-lua/analysis/engine/rows"
	"github.com/wippyai/go-lua/analysis/identity"
)

// validateMountedArtifactSchedule is the one composition gate between the
// parent WTO certificate and the composed equation graph. Program order is a
// stable rank input to the one equation scheduler, not a second final-schedule
// authority: Link-local factor and activation edges may legally constrain that
// order or merge structural SCCs. Publication therefore proves exact point
// coverage and monotonic cycle preservation. Added edges may merge parent
// regions, but can never split or erase a Program-issued cyclic region.
type ReceiptScheduleFailure uint8

const (
	ReceiptScheduleFailureNone ReceiptScheduleFailure = iota
	ReceiptScheduleFailureInput
	ReceiptScheduleFailurePoint
	ReceiptScheduleFailureBootstrap
	ReceiptScheduleFailureEvent
	ReceiptScheduleFailureCoverage
	ReceiptScheduleFailureOrder
	ReceiptScheduleFailureRegionCount
	ReceiptScheduleFailureRegionPoint
	ReceiptScheduleFailureRegionMatch
	ReceiptScheduleFailureRegionParent
	ReceiptScheduleFailureStage
)

func validateMountedArtifactSchedule(artifactRows *artifactReceiptTopology, topology *equation.Topology, graph *equation.Graph) (ReceiptScheduleFailure, uint32, bool) {
	expectedPoints := 0
	if artifactRows != nil {
		expectedPoints = len(artifactRows.points)
		if artifactRows.bootstrap != nil {
			expectedPoints++
		}
	}
	if artifactRows == nil || topology == nil || graph == nil || !topology.OwnsGraph(graph) || !artifactRows.valid(nil) || graph.Schedule() == nil || graph.PointCount() != expectedPoints {
		return ReceiptScheduleFailureInput, 0, false
	}
	pointByKey := make(map[composition.Key]identity.ContentID, len(artifactRows.points))
	for _, id := range artifactRows.points {
		ref, refOK := artifactRows.pointRef[id]
		locator, locatorOK := topology.PointRow(ref)
		point, pointOK := locator.Resolve(graph)
		if !refOK || !locatorOK || !pointOK || !point.Available() {
			return ReceiptScheduleFailurePoint, uint32(len(pointByKey)), false
		}
		if _, duplicate := pointByKey[point.Key()]; duplicate {
			return ReceiptScheduleFailurePoint, uint32(len(pointByKey)), false
		}
		pointByKey[point.Key()] = id
	}
	bootstrapKey := composition.Key{}
	if artifactRows.bootstrap != nil {
		locator, locatorOK := topology.PointRow(artifactRows.bootstrap.ref)
		point, pointOK := locator.Resolve(graph)
		if !locatorOK || !pointOK || !point.Available() {
			return ReceiptScheduleFailureBootstrap, 0, false
		}
		bootstrapKey = point.Key()
		if _, duplicate := pointByKey[bootstrapKey]; duplicate {
			return ReceiptScheduleFailureBootstrap, 0, false
		}
	}
	pointRank := make(map[identity.ContentID]int, len(artifactRows.points))
	bootstrapSeen := false
	compiled := graph.Schedule()
	for index := 0; index < compiled.EventCount(); index++ {
		event, eventOK := compiled.EventAt(index)
		if !eventOK {
			return ReceiptScheduleFailureEvent, uint32(index), false
		}
		if event.Kind != schedule.EventNode {
			continue
		}
		point, pointOK := graph.PointAt(event.Node)
		if !pointOK {
			return ReceiptScheduleFailureEvent, uint32(index), false
		}
		id, found := pointByKey[point.Key()]
		if !found {
			if bootstrapKey.Available() && point.Key() == bootstrapKey && !bootstrapSeen {
				bootstrapSeen = true
				continue
			}
			return ReceiptScheduleFailureEvent, uint32(index), false
		}
		if _, duplicate := pointRank[id]; duplicate {
			return ReceiptScheduleFailureEvent, uint32(index), false
		}
		pointRank[id] = len(pointRank)
	}
	if len(pointRank) != len(artifactRows.points) || artifactRows.bootstrap != nil && !bootstrapSeen {
		return ReceiptScheduleFailureCoverage, uint32(len(pointRank)), false
	}
	// Native Call ownership comes only from ProgramArtifact's sealed rule
	// placements. Summary and Effect intentionally have multiple ingress
	// transports, so choosing an arbitrary LocalTransfer would make ownership
	// depend on row order. All roles sharing a native stage must attest the same
	// exact input.
	stageBase := make(map[identity.ContentID]identity.ContentID)
	stageKind := make(map[identity.ContentID]rows.ArtifactRuleStage)
	for _, placement := range artifactRows.callStages {
		base, stage := placement.mountedInput, placement.mountedPoint
		baseRank, baseOK := pointRank[base]
		stageRank, stageOK := pointRank[stage]
		if !placement.stage.NativeCall() || !baseOK || !stageOK || baseRank >= stageRank {
			return ReceiptScheduleFailureStage, uint32(stageRank), false
		}
		if prior, duplicate := stageBase[stage]; duplicate && prior != base {
			return ReceiptScheduleFailureStage, uint32(stageRank), false
		}
		if prior, duplicate := stageKind[stage]; duplicate && prior != placement.stage {
			return ReceiptScheduleFailureStage, uint32(stageRank), false
		}
		stageBase[stage], stageKind[stage] = base, placement.stage
	}
	localStages := make(map[identity.ContentID]struct{})
	for _, placement := range artifactRows.ruleSet {
		switch placement.stage {
		case rows.ArtifactRuleStageBase:
		case rows.ArtifactRuleStageLocal:
			if _, native := stageKind[placement.mountedPoint]; native {
				return ReceiptScheduleFailureStage, uint32(pointRank[placement.mountedPoint]), false
			}
			localStages[placement.mountedPoint] = struct{}{}
		case rows.ArtifactRuleStageCallDispatch, rows.ArtifactRuleStageCallSummary, rows.ArtifactRuleStageCallEffect:
			if owner, native := stageKind[placement.mountedPoint]; !native || owner != placement.stage {
				return ReceiptScheduleFailureStage, uint32(pointRank[placement.mountedPoint]), false
			}
		default:
			return ReceiptScheduleFailureStage, 0, false
		}
	}
	localOwners := make(map[identity.ContentID]identity.ContentID, len(localStages))
	for edgeIndex, edge := range artifactRows.edges {
		if !edge.local {
			continue
		}
		base, stage := edge.from, edge.to
		baseRank, baseOK := pointRank[base]
		stageRank, stageOK := pointRank[stage]
		ordered := baseRank < stageRank
		if !baseOK || !stageOK || !ordered {
			return ReceiptScheduleFailureStage, uint32(edgeIndex), false
		}
		if _, native := stageKind[stage]; native {
			continue
		}
		if _, local := localStages[stage]; !local || !edge.full {
			continue
		}
		if _, duplicate := localOwners[stage]; duplicate {
			return ReceiptScheduleFailureStage, uint32(edgeIndex), false
		}
		localOwners[stage], stageBase[stage] = base, base
	}
	if len(localOwners) != len(localStages) {
		return ReceiptScheduleFailureStage, uint32(len(localOwners)), false
	}
	// Every parent point event must resolve to the exact scheduled point. The
	// dense parent rank has already been supplied through TopologySpec.PointRanks;
	// dependencies in the composed graph are allowed to override that tie-break.
	for _, event := range artifactRows.events {
		if event.kind != rows.ArtifactEventPoint {
			continue
		}
		if _, rankOK := pointRank[event.point]; !rankOK {
			return ReceiptScheduleFailureOrder, 0, false
		}
	}
	graphRegions := make([]map[identity.ContentID]struct{}, compiled.RegionCount())
	for graphIndex := range graphRegions {
		view, viewOK := graph.RegionAt(graphIndex)
		if !viewOK {
			return ReceiptScheduleFailureRegionPoint, uint32(graphIndex), false
		}
		if view.PointCount() == 0 {
			return ReceiptScheduleFailureRegionPoint, uint32(graphIndex), false
		}
		members := make(map[identity.ContentID]struct{}, view.PointCount())
		for memberIndex := 0; memberIndex < view.PointCount(); memberIndex++ {
			point, pointOK := view.PointAt(memberIndex)
			id, idOK := pointByKey[point.Key()]
			if !pointOK || !idOK {
				return ReceiptScheduleFailureRegionPoint, uint32(graphIndex), false
			}
			members[id] = struct{}{}
		}
		graphRegions[graphIndex] = members
	}
	for _, parent := range artifactRows.regions {
		if !parent.cyclic {
			continue
		}
		contained := false
		for _, region := range graphRegions {
			containsAll := true
			for _, member := range parent.members {
				seenStages := make(map[identity.ContentID]struct{}, 4)
				for {
					base, staged := stageBase[member]
					if !staged {
						break
					}
					if _, cycle := seenStages[member]; cycle {
						return ReceiptScheduleFailureStage, uint32(len(seenStages)), false
					}
					seenStages[member] = struct{}{}
					member = base
				}
				if _, exists := region[member]; !exists {
					containsAll = false
					break
				}
			}
			if containsAll {
				contained = true
				break
			}
		}
		if !contained {
			return ReceiptScheduleFailureRegionMatch, 0, false
		}
	}
	return ReceiptScheduleFailureNone, 0, true
}
