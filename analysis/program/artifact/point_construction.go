package artifact

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow"
)

func (compiler *compiler) copyLocalWTOFailure() CompileFailure {
	wto := compiler.input.Flow().Local().WTO()
	regions := make(map[identity.ContentID]int, wto.Count())
	for index := 0; index < wto.Count(); index++ {
		parent, ok := wto.At(index)
		if !ok || !parent.Available() || !parent.ID().Available() {
			return compileFailure(CompileStageLocalWTO, CompileRowRegion, index, -1, CompileReasonRegionUnavailable)
		}
		if _, exists := regions[parent.ID()]; exists {
			return compileFailure(CompileStageLocalWTO, CompileRowRegion, index, -1, CompileReasonRegionDuplicate)
		}
		header, headerOK := parent.HeaderPoint()
		if !headerOK || !compiler.installPoint(header) {
			return compileFailure(CompileStageLocalWTO, CompileRowRegion, index, -1, CompileReasonRegionHeaderUnavailable)
		}
		members := make([]identity.ContentID, parent.PointCount())
		if len(members) == 0 {
			return compileFailure(CompileStageLocalWTO, CompileRowRegion, index, -1, CompileReasonRegionEmpty)
		}
		for pointIndex := range members {
			point, pointOK := parent.PointAt(pointIndex)
			if !pointOK || !compiler.installPoint(point) {
				return compileFailure(CompileStageLocalWTO, CompileRowRegion, index, pointIndex, CompileReasonRegionMemberUnavailable)
			}
			members[pointIndex] = point.PathID()
			if pointIndex != 0 && members[pointIndex] == members[pointIndex-1] {
				return compileFailure(CompileStageLocalWTO, CompileRowRegion, index, pointIndex, CompileReasonRegionMemberDuplicate)
			}
		}
		if members[0] != header.PathID() {
			return compileFailure(CompileStageLocalWTO, CompileRowRegion, index, 0, CompileReasonRegionHeaderMismatch)
		}
		regions[parent.ID()] = len(compiler.regions)
		compiler.regions = append(compiler.regions, Region{
			id: parent.ID(), head: header.PathID(), sourceHead: header.PathID(), parent: parent.ParentID(), cyclic: parent.Cyclic(), members: members,
		})
	}

	pointEvents := make(map[identity.ContentID]struct{}, len(compiler.points))
	entered := make([]bool, len(compiler.regions))
	exited := make([]bool, len(compiler.regions))
	type frame struct {
		region int
		next   int
	}
	stack := make([]frame, 0, len(compiler.regions))
	for index := 0; index < wto.EventCount(); index++ {
		parent, ok := wto.EventAt(index)
		if !ok || !parent.Available() {
			return compileFailure(CompileStageLocalWTO, CompileRowWTOEvent, index, -1, CompileReasonEventUnavailable)
		}
		event := WTOEvent{}
		switch parent.Kind() {
		case flow.WTOEventEnter:
			region, regionOK := parent.Region()
			if !regionOK || !region.Available() {
				return compileFailure(CompileStageLocalWTO, CompileRowWTOEvent, index, -1, CompileReasonEventRegionUnavailable)
			}
			regionIndex, exists := regions[region.ID()]
			if !exists {
				return compileFailure(CompileStageLocalWTO, CompileRowWTOEvent, index, -1, CompileReasonEventRegionUnknown)
			}
			if entered[regionIndex] || exited[regionIndex] {
				return compileFailure(CompileStageLocalWTO, CompileRowWTOEvent, index, regionIndex, CompileReasonEventRegionRepeated)
			}
			row := compiler.regions[regionIndex]
			if len(stack) == 0 {
				if row.parent.Available() {
					return compileFailure(CompileStageLocalWTO, CompileRowWTOEvent, index, regionIndex, CompileReasonEventRootParent)
				}
			} else {
				parentFrame := stack[len(stack)-1]
				if !entered[parentFrame.region] || parentFrame.next == 0 || row.parent != compiler.regions[parentFrame.region].id {
					return compileFailure(CompileStageLocalWTO, CompileRowWTOEvent, index, regionIndex, CompileReasonEventParentMismatch)
				}
			}
			entered[regionIndex] = true
			stack = append(stack, frame{region: regionIndex})
			event.kind, event.region = WTOEventEnter, region.ID()
		case flow.WTOEventPoint:
			point, pointOK := parent.Point()
			// Parent LocalWTO may schedule an acyclic phase vertex outside every
			// cyclic Region.  It is still a total parent-issued point and must be
			// retained, rather than being treated as malformed merely because the
			// region stack is empty.
			if !pointOK || !compiler.installPoint(point) {
				return compileFailure(CompileStageLocalWTO, CompileRowWTOEvent, index, -1, CompileReasonEventPointUnavailable)
			}
			id := point.PathID()
			if _, exists := pointEvents[id]; exists {
				return compileFailure(CompileStageLocalWTO, CompileRowWTOEvent, index, -1, CompileReasonEventPointRepeated)
			}
			if len(stack) != 0 {
				current := &stack[len(stack)-1]
				row := compiler.regions[current.region]
				if current.next >= len(row.members) || row.members[current.next] != id || current.next == 0 && row.head != id {
					return compileFailure(CompileStageLocalWTO, CompileRowWTOEvent, index, current.next, CompileReasonEventPointOrder)
				}
				current.next++
			}
			pointEvents[id] = struct{}{}
			event.kind, event.point = WTOEventPoint, id
		case flow.WTOEventExit:
			region, regionOK := parent.Region()
			if !regionOK || !region.Available() || len(stack) == 0 {
				return compileFailure(CompileStageLocalWTO, CompileRowWTOEvent, index, -1, CompileReasonEventExitUnavailable)
			}
			current := stack[len(stack)-1]
			if compiler.regions[current.region].id != region.ID() || current.next != len(compiler.regions[current.region].members) || exited[current.region] {
				return compileFailure(CompileStageLocalWTO, CompileRowWTOEvent, index, current.region, CompileReasonEventExitMismatch)
			}
			exited[current.region] = true
			stack = stack[:len(stack)-1]
			event.kind, event.region = WTOEventExit, region.ID()
		default:
			return compileFailure(CompileStageLocalWTO, CompileRowWTOEvent, index, -1, CompileReasonEventKindUnknown)
		}
		if !event.Available() {
			return compileFailure(CompileStageLocalWTO, CompileRowWTOEvent, index, -1, CompileReasonEventUnavailable)
		}
		compiler.events = append(compiler.events, event)
	}
	if len(stack) != 0 {
		return compileFailure(CompileStageLocalWTO, CompileRowWTOEvent, wto.EventCount(), len(stack), CompileReasonEventUnbalanced)
	}
	for index := range compiler.regions {
		if !entered[index] || !exited[index] {
			return compileFailure(CompileStageLocalWTO, CompileRowRegion, index, -1, CompileReasonRegionIncomplete)
		}
	}
	for point := range compiler.points {
		if _, exists := pointEvents[point]; !exists {
			return compileFailure(CompileStageLocalWTO, CompileRowPoint, -1, -1, CompileReasonPointUnscheduled)
		}
	}
	return CompileFailure{}
}

func (compiler *compiler) installPoint(point flow.WTOPoint) bool {
	if !point.Available() || !point.PathID().Available() {
		return false
	}
	if compiler.pointGeometry == nil {
		compiler.pointGeometry = make(map[identity.ContentID]Point)
	}
	if existing, exists := compiler.pointGeometry[point.PathID()]; exists {
		return existing.Available()
	}
	entryBody, bodyOK := compiler.input.BodyAt(0)
	entrySite, entryOK := entryBody.EntrySite()
	if !bodyOK || !entryOK || !entrySite.Available() {
		return false
	}
	decisions := make(map[identity.ContentID]struct{})
	initial := false
	for index := 0; index < point.SiteCount(); index++ {
		site, siteOK := point.SiteAt(index)
		if !siteOK || !site.Available() || !compiler.input.OwnsSite(site) {
			return false
		}
		if site.ContextID() == entrySite.ContextID() {
			initial = true
		}
		subject, subjectOK := site.Term()
		count, countOK := compiler.input.Flow().Continuation().GuardCount(subject)
		if !subjectOK || !countOK {
			return false
		}
		for guardIndex := 0; guardIndex < count; guardIndex++ {
			guard, guardOK := compiler.input.Flow().Continuation().GuardAt(subject, guardIndex)
			decisionID, decisionOK := compiler.input.Flow().SemanticTermPath(guard)
			if !guardOK || !decisionOK || !decisionID.Available() {
				return false
			}
			decisions[decisionID] = struct{}{}
		}
	}
	ordered := make([]identity.ContentID, 0, len(decisions))
	for decision := range decisions {
		ordered = append(ordered, decision)
	}
	identity.SortContentIDs(ordered)
	compiler.points[point.PathID()] = struct{}{}
	compiler.pointGeometry[point.PathID()] = Point{id: point.PathID(), decisions: ordered, initial: initial}
	return true
}

func (compiler *compiler) containsPoint(point flow.WTOPoint) bool {
	if !point.Available() || !point.PathID().Available() {
		return false
	}
	_, exists := compiler.points[point.PathID()]
	return exists
}
