package rows

import (
	"bytes"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	schemaissuance "github.com/wippyai/go-lua/analysis/schema/issuance"
)

// ValidArtifactScalarEdgeProof is the scalar route proof of one structural
// edge. It reads only the declared edge, so the same proof holds before the
// edge is mounted and after a mounting owner has substituted its identities.
func ValidArtifactScalarEdgeProof(edge ArtifactScalarEdge) bool {
	if !edge.ID.Available() || !edge.Route.Available() || !edge.Arm.Valid() {
		return false
	}
	if edge.Guarded != edge.Guard.Available() || edge.Guarded != edge.Decision.Available() || edge.HasReset != edge.Reset.Available() || edge.Mu.Available() != edge.HasReset {
		return false
	}
	for index, reset := range edge.Resets {
		if !reset.Available() || index > 0 && bytes.Compare(edge.Resets[index-1][:], reset[:]) >= 0 {
			return false
		}
	}
	return true
}

// validArtifactScalarSpec is the sole template relational admission fence. Builder
// methods protect storage ownership and basic scalar availability; this final
// pass still rejects forged cross-row structure before ownership transfers to
// the sealed template. Every later phase then proves only role/mount substitution.
func validArtifactScalarSpec(spec *ArtifactScalarSpec) bool {
	state, open := spec.writable()
	if !open || !state.ArtifactID.Available() || !state.ProgramID.Available() || !state.SchemaID.Available() || len(state.Points) == 0 || len(state.Events) == 0 || len(state.Bodies) == 0 {
		return false
	}
	points := make(map[identity.ContentID]struct{}, len(state.Points))
	for _, point := range state.Points {
		if !point.ID.Available() {
			return false
		}
		if _, duplicate := points[point.ID]; duplicate {
			return false
		}
		points[point.ID] = struct{}{}
		for _, decision := range point.Decisions {
			if !decision.Available() {
				return false
			}
		}
		for index := 1; index < len(point.Decisions); index++ {
			if bytes.Compare(point.Decisions[index-1][:], point.Decisions[index][:]) >= 0 {
				return false
			}
		}
	}

	edgeIDs := make(map[identity.ContentID]struct{}, len(state.Edges)+len(state.Transfers))
	routes := make(map[identity.ContentID]ArtifactScalarEdge, len(state.Edges))
	duplicateRoutes := make(map[identity.ContentID]struct{})
	pointDecisions := make(map[identity.ContentID]map[identity.ContentID]struct{}, len(state.Points))
	for _, point := range state.Points {
		decisions := make(map[identity.ContentID]struct{}, len(point.Decisions))
		for _, decision := range point.Decisions {
			decisions[decision] = struct{}{}
		}
		pointDecisions[point.ID] = decisions
	}
	for _, edge := range state.Edges {
		if !ValidArtifactScalarEdgeProof(edge) {
			return false
		}
		if _, fromOK := points[edge.From]; !fromOK {
			return false
		}
		if _, toOK := points[edge.To]; !toOK {
			return false
		}
		if edge.Guarded {
			if _, decisionOK := pointDecisions[edge.From][edge.Decision]; !decisionOK {
				return false
			}
		}
		if _, duplicate := edgeIDs[edge.ID]; duplicate {
			return false
		}
		if _, duplicate := routes[edge.Route]; duplicate {
			duplicateRoutes[edge.Route] = struct{}{}
		} else {
			routes[edge.Route] = edge
		}
		edgeIDs[edge.ID] = struct{}{}
	}
	for _, edge := range state.Transfers {
		if !edge.ID.Available() || !edge.From.Available() || !edge.To.Available() || edge.From == edge.To || edge.Full == (len(edge.Factors) != 0) {
			return false
		}
		if _, duplicate := edgeIDs[edge.ID]; duplicate {
			return false
		}
		edgeIDs[edge.ID] = struct{}{}
		for _, factor := range edge.Factors {
			if !scalarSpecOwnsFactor(state, factor) {
				return false
			}
		}
	}

	regions := make(map[identity.ContentID]ArtifactScalarRegion, len(state.Regions))
	for _, region := range state.Regions {
		if !region.ID.Available() || !region.Head.Available() || len(region.Members) == 0 || region.Members[0] != region.Head {
			return false
		}
		if _, duplicate := regions[region.ID]; duplicate {
			return false
		}
		members := make(map[identity.ContentID]struct{}, len(region.Members))
		for _, member := range region.Members {
			if _, pointOK := points[member]; !pointOK {
				return false
			}
			if _, duplicate := members[member]; duplicate {
				return false
			}
			members[member] = struct{}{}
		}
		regions[region.ID] = region
	}
	for _, region := range state.Regions {
		if region.Parent.Available() {
			if _, parentOK := regions[region.Parent]; !parentOK || region.Parent == region.ID {
				return false
			}
		}
	}

	if !validArtifactScalarSchedule(points, regions, state.Events) {
		return false
	}

	pointRank := make(map[identity.ContentID]int, len(points))
	for _, event := range state.Events {
		if event.Kind == ArtifactEventPoint {
			pointRank[event.Point] = len(pointRank)
		}
	}
	type artifactStageGeometry struct {
		stage schema.Key
		input identity.ContentID
	}
	stageGeometry := make(map[identity.ContentID]artifactStageGeometry)
	type artifactTemplateRuleOccurrence struct {
		role ArtifactScalarRole
		id   identity.ContentID
	}
	nativeOccurrences := make(map[artifactTemplateRuleOccurrence]struct{})
	for _, rule := range state.Rules {
		if !scalarSpecOwnsRole(state, rule.Role) || !rule.Stage.Available() || !rule.Point.Available() || !rule.ID.Available() {
			return false
		}
		stage, stageOK := state.stages.Entry(rule.Stage, schemaissuance.KindStage)
		if state.stagesSet {
			if !stageOK || stage.Native() != rule.Native || stage.ConsumesInput() != rule.Input.Available() {
				return false
			}
		} else if rule.Native {
			// Native rows are interpreted by the engine after mounting. They
			// therefore require the canonical schema witness at this seal; an
			// opaque key plus a caller-supplied bool is not authority.
			return false
		}
		if _, pointOK := points[rule.Point]; !pointOK {
			return false
		}
		if rule.Input.Available() {
			if _, inputOK := points[rule.Input]; !inputOK {
				return false
			}
		}
		if !rule.Input.Available() {
			if rule.Native || rule.Route.Available() {
				return false
			}
		} else {
			if rule.Input == rule.Point {
				return false
			}
		}
		if rule.Native {
			inputRank, inputOK := pointRank[rule.Input]
			outputRank, pointOK := pointRank[rule.Point]
			if !rule.Input.Available() || rule.Input == rule.Point || !inputOK || !pointOK || inputRank >= outputRank {
				return false
			}
			geometry := artifactStageGeometry{stage: rule.Stage, input: rule.Input}
			if prior, duplicate := stageGeometry[rule.Point]; duplicate && prior != geometry {
				return false
			}
			stageGeometry[rule.Point] = geometry
		}
		if rule.Native {
			key := artifactTemplateRuleOccurrence{role: rule.Role, id: rule.ID}
			if _, duplicate := nativeOccurrences[key]; duplicate {
				return false
			}
			nativeOccurrences[key] = struct{}{}
		}
		if rule.Route.Available() {
			if _, duplicate := duplicateRoutes[rule.Route]; duplicate {
				return false
			}
			predecessor, predecessorOK := routes[rule.Route]
			if !predecessorOK || predecessor.To != rule.Input {
				return false
			}
		}
	}
	for _, geometry := range stageGeometry {
		stage, stageOK := state.stages.Entry(geometry.stage, schemaissuance.KindStage)
		if !stageOK || !stage.Native() {
			return false
		}
		predecessors := stage.Predecessors()
		if len(predecessors) == 0 {
			return false
		}
		owner, inputIsNative := stageGeometry[geometry.input]
		admitted := false
		for _, predecessorKey := range predecessors {
			predecessor, predecessorOK := state.stages.Entry(predecessorKey, schemaissuance.KindStage)
			if !predecessorOK {
				return false
			}
			if predecessor.Native() {
				admitted = admitted || inputIsNative && owner.stage == predecessorKey
			} else {
				admitted = admitted || !inputIsNative
			}
		}
		if !admitted {
			return false
		}
	}
	bodies := make(map[identity.ContentID]struct{}, len(state.Bodies))
	for _, body := range state.Bodies {
		if !body.ID.Available() || len(body.Entry) == 0 || len(body.Exits) == 0 {
			return false
		}
		if _, duplicate := bodies[body.ID]; duplicate {
			return false
		}
		bodies[body.ID] = struct{}{}
		seenEntry := make(map[identity.ContentID]struct{}, len(body.Entry))
		for _, point := range body.Entry {
			if _, pointOK := points[point]; !pointOK {
				return false
			}
			if _, duplicate := seenEntry[point]; duplicate {
				return false
			}
			seenEntry[point] = struct{}{}
		}
		seenExit := make(map[identity.ContentID]struct{}, len(body.Exits))
		for _, point := range body.Exits {
			if _, pointOK := points[point]; !pointOK {
				return false
			}
			if _, duplicate := seenExit[point]; duplicate {
				return false
			}
			seenExit[point] = struct{}{}
		}
	}
	return true
}

func validArtifactScalarSchedule(points map[identity.ContentID]struct{}, regions map[identity.ContentID]ArtifactScalarRegion, events []ArtifactScalarEvent) bool {
	if len(events) == 0 {
		return false
	}
	entered := make(map[identity.ContentID]bool, len(regions))
	exited := make(map[identity.ContentID]bool, len(regions))
	seenPoint := make(map[identity.ContentID]struct{}, len(points))
	type frame struct {
		region identity.ContentID
		next   int
	}
	stack := make([]frame, 0, len(regions))
	for _, event := range events {
		switch event.Kind {
		case ArtifactEventEnter:
			region, regionOK := regions[event.Region]
			if !regionOK || event.Point.Available() || entered[event.Region] || exited[event.Region] {
				return false
			}
			if len(stack) == 0 {
				if region.Parent.Available() {
					return false
				}
			} else {
				parent := stack[len(stack)-1]
				if parent.next == 0 || region.Parent != parent.region {
					return false
				}
			}
			entered[event.Region] = true
			stack = append(stack, frame{region: event.Region})
		case ArtifactEventPoint:
			if event.Region.Available() {
				return false
			}
			if _, pointOK := points[event.Point]; !pointOK {
				return false
			}
			if _, duplicate := seenPoint[event.Point]; duplicate {
				return false
			}
			if len(stack) != 0 {
				current := &stack[len(stack)-1]
				members := regions[current.region].Members
				if current.next >= len(members) || members[current.next] != event.Point {
					return false
				}
				current.next++
			}
			seenPoint[event.Point] = struct{}{}
		case ArtifactEventExit:
			region, regionOK := regions[event.Region]
			if !regionOK || event.Point.Available() || len(stack) == 0 || stack[len(stack)-1].region != event.Region || exited[event.Region] {
				return false
			}
			if stack[len(stack)-1].next != len(region.Members) {
				return false
			}
			exited[event.Region] = true
			stack = stack[:len(stack)-1]
		default:
			return false
		}
	}
	if len(stack) != 0 || len(seenPoint) != len(points) || len(entered) != len(regions) || len(exited) != len(regions) {
		return false
	}
	for region := range regions {
		if !entered[region] || !exited[region] {
			return false
		}
	}
	return true
}
