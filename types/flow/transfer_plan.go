package flow

import (
	"github.com/wippyai/go-lua/types/cfg"
)

func (s *Solution) buildTransferPlan(size int) {
	if s == nil || s.inputs == nil || size <= 0 {
		return
	}
	s.assignmentsByPoint = make([][]UnifiedAssignment, size)
	for _, assign := range s.inputs.Assignments {
		if idx := int(assign.Point); idx >= 0 && idx < size {
			s.assignmentsByPoint[idx] = append(s.assignmentsByPoint[idx], assign)
		}
	}
	if s.inputs.Graph != nil {
		s.phisByPoint = make([][]cfg.PhiNode, size)
		for _, phi := range s.inputs.Graph.PhiNodes() {
			if idx := int(phi.Point); idx >= 0 && idx < size {
				s.phisByPoint[idx] = append(s.phisByPoint[idx], phi)
			}
		}
	}
	s.mapMutatorAssignmentsByPoint = make([][]MapMutatorAssignment, size)
	for _, assign := range s.inputs.MapMutatorAssignments {
		if idx := int(assign.Point); idx >= 0 && idx < size {
			s.mapMutatorAssignmentsByPoint[idx] = append(s.mapMutatorAssignmentsByPoint[idx], assign)
		}
	}
	s.tableMutatorByPoint = make([][]TableMutatorAssignment, size)
	for _, assign := range s.inputs.TableMutatorAssignments {
		if idx := int(assign.Point); idx >= 0 && idx < size {
			s.tableMutatorByPoint[idx] = append(s.tableMutatorByPoint[idx], assign)
		}
	}
	s.containerMutatorByPoint = make([][]ContainerMutatorAssignment, size)
	for _, assign := range s.inputs.ContainerMutatorAssignments {
		if idx := int(assign.Point); idx >= 0 && idx < size {
			s.containerMutatorByPoint[idx] = append(s.containerMutatorByPoint[idx], assign)
		}
	}
}

func (s *Solution) phisAt(p cfg.Point) []cfg.PhiNode {
	if s == nil {
		return nil
	}
	idx := int(p)
	if idx < 0 || idx >= len(s.phisByPoint) {
		return nil
	}
	return s.phisByPoint[idx]
}

func (s *Solution) assignmentsAt(p cfg.Point) []UnifiedAssignment {
	if s == nil {
		return nil
	}
	idx := int(p)
	if idx < 0 || idx >= len(s.assignmentsByPoint) {
		return nil
	}
	return s.assignmentsByPoint[idx]
}

func (s *Solution) mapMutatorAssignmentsAt(p cfg.Point) []MapMutatorAssignment {
	if s == nil {
		return nil
	}
	idx := int(p)
	if idx < 0 || idx >= len(s.mapMutatorAssignmentsByPoint) {
		return nil
	}
	return s.mapMutatorAssignmentsByPoint[idx]
}

func (s *Solution) tableMutatorAssignmentsAt(p cfg.Point) []TableMutatorAssignment {
	if s == nil {
		return nil
	}
	idx := int(p)
	if idx < 0 || idx >= len(s.tableMutatorByPoint) {
		return nil
	}
	return s.tableMutatorByPoint[idx]
}

func (s *Solution) containerMutatorAssignmentsAt(p cfg.Point) []ContainerMutatorAssignment {
	if s == nil {
		return nil
	}
	idx := int(p)
	if idx < 0 || idx >= len(s.containerMutatorByPoint) {
		return nil
	}
	return s.containerMutatorByPoint[idx]
}
