package compiler

import (
	"bytes"
	"sort"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/causal"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/relation/schema/region"
)

func (compiler *compiler) copyLocalWTOFailure() CompileFailure {
	wto := compiler.input.Flow().LocalWTO()
	for index := 0; index < wto.Count(); index++ {
		parent, ok := wto.At(index)
		if !ok || !parent.Available() || !parent.ID().Available() {
			return compileFailure(CompileStageLocalWTO, CompileRowRegion, index, -1, CompileReasonRegionUnavailable)
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
		compiler.regions = append(compiler.regions, regionDraft{
			id: parent.ID(), parent: parent.ParentID(), cyclic: parent.Cyclic(), members: members,
		})
	}

	pointEvents := make(map[identity.ContentID]struct{}, len(compiler.pointGeometry))
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
		event := wtoEventDraft{}
		switch parent.Kind() {
		case causal.WTOEventEnter:
			region, regionOK := parent.Region()
			if !regionOK || !region.Available() {
				return compileFailure(CompileStageLocalWTO, CompileRowWTOEvent, index, -1, CompileReasonEventRegionUnavailable)
			}
			ordinal, exists := region.Ordinal()
			regionIndex := int(ordinal)
			if !exists || regionIndex < 0 || regionIndex >= len(compiler.regions) || compiler.regions[regionIndex].id != region.ID() {
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
			event.kind, event.region = wtoEventEnter, region.ID()
		case causal.WTOEventPoint:
			point, pointOK := parent.Point()
			// Parent LocalWTO may schedule an acyclic phase vertex outside every
			// cyclic regionDraft.  It is still a total parent-issued point and must be
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
				if current.next >= len(row.members) || row.members[current.next] != id {
					return compileFailure(CompileStageLocalWTO, CompileRowWTOEvent, index, current.next, CompileReasonEventPointOrder)
				}
				current.next++
			}
			pointEvents[id] = struct{}{}
			event.kind, event.point = wtoEventPoint, id
		case causal.WTOEventExit:
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
			event.kind, event.region = wtoEventExit, region.ID()
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
	for point := range compiler.pointGeometry {
		if _, exists := pointEvents[point]; !exists {
			return compileFailure(CompileStageLocalWTO, CompileRowPoint, -1, -1, CompileReasonPointUnscheduled)
		}
	}
	return CompileFailure{}
}

func (compiler *compiler) installPoint(point causal.WTOPoint) bool {
	if !point.Available() || !point.PathID().Available() {
		return false
	}
	if compiler.pointGeometry == nil {
		compiler.pointGeometry = make(map[identity.ContentID]pointDraft)
	}
	if existing, exists := compiler.pointGeometry[point.PathID()]; exists {
		return existing.Available()
	}
	rootBoundary, rootOK := compiler.input.Flow().FunctionBoundaries().Root()
	rootBody, rootBodyOK := rootBoundary.Body()
	rootBodyOrdinal := keyspace.TermOrdinal(rootBody)
	if !rootOK || !rootBoundary.Available() || !rootBodyOK || keyspace.TermFamily(rootBody) != keyspace.FamilyBody || rootBodyOrdinal == 0 ||
		uint64(rootBodyOrdinal) > uint64(compiler.input.BodyCount()) {
		return false
	}
	entryBody, bodyOK := compiler.input.BodyAt(int(rootBodyOrdinal) - 1)
	entrySite, entryOK := entryBody.EntrySite()
	if !bodyOK || !entryOK || !entrySite.Available() {
		return false
	}
	decisions := make(map[identity.ContentID]region.Atom)
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
			decisionAtom, atomOK := compiler.input.Flow().SemanticTermAtom(guard)
			if !guardOK || !decisionOK || !decisionID.Available() || !atomOK || !decisionAtom.Available() {
				return false
			}
			if prior, exists := decisions[decisionID]; exists && prior != decisionAtom {
				return false
			}
			decisions[decisionID] = decisionAtom
		}
	}
	ordered := make([]pointDecisionDraft, 0, len(decisions))
	for decision, atom := range decisions {
		ordered = append(ordered, pointDecisionDraft{semantic: decision, atom: atom})
	}
	sort.Slice(ordered, func(left, right int) bool {
		return bytes.Compare(ordered[left].semantic[:], ordered[right].semantic[:]) < 0
	})
	compiler.pointGeometry[point.PathID()] = pointDraft{id: point.PathID(), decisionScope: point.PathID(), decisions: ordered, initial: initial}
	return true
}

func (compiler *compiler) containsPoint(point causal.WTOPoint) bool {
	if !point.Available() || !point.PathID().Available() {
		return false
	}
	row, exists := compiler.pointGeometry[point.PathID()]
	return exists && row.Available()
}
