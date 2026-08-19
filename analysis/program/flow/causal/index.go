package causal

import (
	"errors"
	"fmt"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func (s *indexState) activationRoot(bodyTerm keyspace.Term) (keyspace.Term, bool) {
	function, ok := s.bodies.Activation(bodyTerm)
	if !ok {
		return 0, false
	}
	if function == 0 {
		return s.entry, true
	}
	_, functionBody, _, functionOK := s.flow.Functions().Get(function)
	if !functionOK {
		return 0, false
	}
	return functionBody, true
}

func (s *indexState) validateArcCoverage() error {
	s.arcCoverageValidated = false
	if s.graph == nil || s.graph.ArcCount() < 0 || len(s.arcDisposition) != s.graph.ArcCount() {
		return errors.New("program/flow/causal: Arc disposition denominator is unavailable")
	}
	for index := 0; index < s.graph.ArcCount(); index++ {
		if s.arcDisposition[index] != arcUndisposed {
			continue
		}
		row, ok := s.graph.ArcAt(index)
		if !ok {
			return fmt.Errorf("program/flow/causal: sourcecontrol Arc %d is unavailable", index)
		}
		sourceLive := s.live(row.Source)
		targetLive := s.live(row.Target)
		if !sourceLive || !targetLive || s.static(row.Source) || s.static(row.Target) {
			// Explicit dead/static/liveness-only disposition. It is not a
			// causal Edge and therefore must never be fabricated as one.
			s.arcDisposition[index] = arcDeadStatic
			continue
		}
		return fmt.Errorf("program/flow/causal: live sourcecontrol Arc %d (%v -> %v) has no disposition", index, row.Source, row.Target)
	}
	s.arcCoverageValidated = true
	return nil
}

func (s *indexState) finish() error {
	if !s.arcCoverageValidated {
		return errors.New("program/flow/causal: Arc coverage was not validated before finalization")
	}
	if len(s.edgeRows) != len(s.edgeOwners) || len(s.boundaryRows) != len(s.boundaryOwners) {
		return errors.New("program/flow/causal: typed row owner planes are misaligned")
	}
	for _, edge := range s.edgeRows {
		if keyspace.TermFamily(edge.From) == keyspace.FamilyCall {
			return errors.New("program/flow/causal: local Edge originates at Call")
		}
		if edge.From == 0 || edge.To == 0 || !s.validOutcomeOrLive(edge.From) || !s.validOutcomeOrLive(edge.To) {
			return errors.New("program/flow/causal: malformed local Edge row")
		}
		// A self-edge is retained only when the claimed recurrence witness
		// supplied a nonzero Mu, including an empty reset interval.
		if edge.From == edge.To && edge.Mu == 0 {
			return errors.New("program/flow/causal: Mu-less Edge is self-referential")
		}
		if edge.Decision == 0 && edge.Truth || edge.Decision != 0 && (!isDecision(edge.Decision) || !s.live(edge.Decision)) {
			return errors.New("program/flow/causal: malformed local Edge decision")
		}
	}

	// Rows were materialized directly in their final typed planes. Only the
	// parallel owner slices remain seal-local for projection construction.
	s.result.edges.rows = s.edgeRows
	s.result.boundaries.rows = s.boundaryRows
	for _, edge := range s.edgeRows {
		if !s.result.validEdgeRow(edge) {
			return errors.New("program/flow/causal: malformed local Edge combination")
		}
	}
	for _, boundary := range s.boundaryRows {
		if !s.result.validBoundaryRow(boundary) {
			return errors.New("program/flow/causal: malformed CallBoundary combination")
		}
	}
	s.result.boundaries.callSlots = make([]uint32, s.counts[keyspace.FamilyCall]+1)
	for index, row := range s.boundaryRows {
		callOrdinal := keyspace.TermOrdinal(row.Call)
		if callOrdinal == 0 || uint64(callOrdinal) >= uint64(len(s.result.boundaries.callSlots)) ||
			s.result.boundaries.callSlots[callOrdinal] != 0 {
			return errors.New("program/flow/causal: duplicate CallBoundary ordinal")
		}
		s.result.boundaries.callSlots[callOrdinal] = uint32(index + 1)
	}

	if err := s.buildEdgeIndexes(); err != nil {
		return err
	}
	if err := s.buildSuccessorIndex(); err != nil {
		return err
	}
	if err := s.result.buildRouteIndex(); err != nil {
		return err
	}
	if err := s.buildWriteCommitIndex(); err != nil {
		return err
	}
	return nil
}

// buildWriteCommitIndex projects the seal-local assignment commit witnesses
// onto the existing combined Successor refs. It publishes no new Edge rows or
// predecessor relation: each entry is only a capability to one already
// indexed local Edge.
func (s *indexState) buildWriteCommitIndex() error {
	writeCount := s.counts[keyspace.FamilyWrite]
	if uint64(len(s.writeCommitEdges)) != uint64(writeCount)+1 || uint64(len(s.writeCommitSet)) != uint64(writeCount)+1 {
		return errors.New("program/flow/causal: assignment commit witness denominator is unavailable")
	}
	refs := make([]successorRef, writeCount+1)
	if writeCount == 0 {
		s.result.index.writeCommitRefs = refs
		return nil
	}
	edgeRefs := make([]successorRef, len(s.edgeRows))
	for _, ref := range s.result.index.refs {
		if !ref.local {
			continue
		}
		if uint64(ref.index) >= uint64(len(edgeRefs)) || edgeRefs[ref.index].local {
			return errors.New("program/flow/causal: assignment commit Edge ref is ambiguous")
		}
		edgeRefs[ref.index] = ref
	}
	for ordinal := uint32(1); ordinal <= writeCount; ordinal++ {
		if !s.writeCommitSet[ordinal] {
			continue
		}
		edgeIndex := s.writeCommitEdges[ordinal]
		if uint64(edgeIndex) >= uint64(len(s.edgeRows)) || !edgeRefs[edgeIndex].local {
			return errors.New("program/flow/causal: assignment commit Edge disappeared from Successors")
		}
		write := keyspace.MakeTerm(keyspace.FamilyWrite, ordinal)
		if s.edgeRows[edgeIndex].To != write {
			return errors.New("program/flow/causal: assignment commit Successor target disagrees with Write")
		}
		refs[ordinal] = edgeRefs[edgeIndex]
	}
	s.result.index.writeCommitRefs = refs
	return nil
}

func (s *indexState) buildEdgeIndexes() error {
	bodyCount := s.counts[keyspace.FamilyBody]
	s.result.edges.bodyRanges = make([]range32, bodyCount+1)
	bodyCounts := make([]uint32, bodyCount+1)
	activationCounts := make([]uint32, bodyCount+1)
	activationRoot := make([]bool, bodyCount+1)
	for ordinal := uint32(1); ordinal <= bodyCount; ordinal++ {
		body := keyspace.MakeTerm(keyspace.FamilyBody, ordinal)
		root, rootOK := s.activationRoot(body)
		if !rootOK || keyspace.TermFamily(root) != keyspace.FamilyBody {
			return errors.New("program/flow/causal: Edge activation root is unavailable")
		}
		rootOrdinal := keyspace.TermOrdinal(root)
		if rootOrdinal == 0 || rootOrdinal > bodyCount {
			return errors.New("program/flow/causal: Edge activation root ordinal is invalid")
		}
		if body == root {
			activationRoot[rootOrdinal] = true
		}
	}
	for index, owner := range s.edgeOwners {
		bodyOrdinal := keyspace.TermOrdinal(owner)
		if keyspace.TermFamily(owner) != keyspace.FamilyBody || bodyOrdinal == 0 || bodyOrdinal > bodyCount {
			return fmt.Errorf("program/flow/causal: Edge %d owner is invalid", index)
		}
		if bodyCounts[bodyOrdinal] == ^uint32(0) {
			return errors.New("program/flow/causal: Body Edge counter overflows")
		}
		bodyCounts[bodyOrdinal]++
		root, rootOK := s.activationRoot(owner)
		if !rootOK {
			return errors.New("program/flow/causal: Edge activation is unavailable")
		}
		rootOrdinal := keyspace.TermOrdinal(root)
		if rootOrdinal == 0 || rootOrdinal > bodyCount || activationCounts[rootOrdinal] == ^uint32(0) {
			return errors.New("program/flow/causal: activation Edge counter overflows")
		}
		activationCounts[rootOrdinal]++
	}
	for ordinal := uint32(1); ordinal <= bodyCount; ordinal++ {
		s.result.edges.bodyRanges[ordinal].start = s.result.edges.bodyRanges[ordinal-1].end
		if uint64(s.result.edges.bodyRanges[ordinal].start)+uint64(bodyCounts[ordinal]) > uint64(^uint32(0)) {
			return errors.New("program/flow/causal: Body Edge range overflows")
		}
		s.result.edges.bodyRanges[ordinal].end = s.result.edges.bodyRanges[ordinal].start + bodyCounts[ordinal]
	}
	s.result.edges.bodyIndexes = make([]uint32, len(s.edgeRows))
	bodyNext := make([]uint32, bodyCount+1)
	for ordinal := uint32(1); ordinal <= bodyCount; ordinal++ {
		bodyNext[ordinal] = s.result.edges.bodyRanges[ordinal].start
	}
	for index, owner := range s.edgeOwners {
		ordinal := keyspace.TermOrdinal(owner)
		at := bodyNext[ordinal]
		if uint64(at) >= uint64(len(s.result.edges.bodyIndexes)) {
			return errors.New("program/flow/causal: Body Edge index overflow")
		}
		s.result.edges.bodyIndexes[at] = uint32(index)
		bodyNext[ordinal]++
	}

	s.result.edges.activationRanges = make([]range32, bodyCount+1)
	s.result.edges.activationRoots = activationRoot
	for ordinal := uint32(1); ordinal <= bodyCount; ordinal++ {
		s.result.edges.activationRanges[ordinal].start = s.result.edges.activationRanges[ordinal-1].end
		if uint64(s.result.edges.activationRanges[ordinal].start)+uint64(activationCounts[ordinal]) > uint64(^uint32(0)) {
			return errors.New("program/flow/causal: activation Edge range overflows")
		}
		s.result.edges.activationRanges[ordinal].end = s.result.edges.activationRanges[ordinal].start + activationCounts[ordinal]
	}
	s.result.edges.activationIndexes = make([]uint32, len(s.edgeRows))
	activationNext := make([]uint32, bodyCount+1)
	for ordinal := uint32(1); ordinal <= bodyCount; ordinal++ {
		activationNext[ordinal] = s.result.edges.activationRanges[ordinal].start
	}
	for index, owner := range s.edgeOwners {
		root, rootOK := s.activationRoot(owner)
		if !rootOK {
			return errors.New("program/flow/causal: Edge activation root disappeared")
		}
		ordinal := keyspace.TermOrdinal(root)
		at := activationNext[ordinal]
		if uint64(at) >= uint64(len(s.result.edges.activationIndexes)) {
			return errors.New("program/flow/causal: activation Edge index overflow")
		}
		s.result.edges.activationIndexes[at] = uint32(index)
		activationNext[ordinal]++
	}
	return nil
}

func (s *indexState) buildSuccessorIndex() error {
	// Each source family gets a dense ordinal plane only when at least one
	// retained route originates there. Families with no outgoing route retain
	// no denominator-sized offsets at all.
	s.result.index.planes = [keyspace.FamilyCount]successorPlane{}
	active := [keyspace.FamilyCount]bool{}
	for _, edge := range s.edgeRows {
		family, ordinal := keyspace.TermFamily(edge.From), keyspace.TermOrdinal(edge.From)
		if family <= keyspace.FamilyInvalid || family >= keyspace.FamilyCount || ordinal == 0 || ordinal > s.counts[family] {
			return errors.New("program/flow/causal: Edge source is outside successor denominator")
		}
		active[family] = true
	}
	for _, boundary := range s.boundaryRows {
		family, ordinal := keyspace.TermFamily(boundary.Call), keyspace.TermOrdinal(boundary.Call)
		if family != keyspace.FamilyCall || ordinal == 0 || ordinal > s.counts[family] {
			return errors.New("program/flow/causal: CallBoundary source is outside successor denominator")
		}
		active[family] = true
	}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		if active[family] {
			s.result.index.planes[family] = successorPlane{denominator: s.counts[family], ranges: make([]range32, s.counts[family]+1)}
		}
	}
	inc := func(family keyspace.Family, ordinal uint32, amount uint64) error {
		plane := &s.result.index.planes[family]
		if ordinal == 0 || uint64(ordinal) >= uint64(len(plane.ranges)) || amount > uint64(^uint32(0))-uint64(plane.ranges[ordinal].end) {
			return errors.New("program/flow/causal: successor source counter overflows")
		}
		plane.ranges[ordinal].end += uint32(amount)
		return nil
	}
	for _, edge := range s.edgeRows {
		if err := inc(keyspace.TermFamily(edge.From), keyspace.TermOrdinal(edge.From), 1); err != nil {
			return err
		}
	}
	for _, boundary := range s.boundaryRows {
		if err := inc(keyspace.FamilyCall, keyspace.TermOrdinal(boundary.Call), uint64(boundaryArmCount(boundary))); err != nil {
			return err
		}
	}
	var total uint64
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		plane := &s.result.index.planes[family]
		for ordinal := uint32(1); ordinal < uint32(len(plane.ranges)); ordinal++ {
			count := uint64(plane.ranges[ordinal].end)
			if total+count > uint64(^uint32(0)) {
				return errors.New("program/flow/causal: successor index overflows")
			}
			plane.ranges[ordinal].start = uint32(total)
			plane.ranges[ordinal].end = uint32(total + count)
			total += count
		}
	}
	if total > uint64(^uint(0)>>1) {
		return errors.New("program/flow/causal: successor reference pool exceeds host index range")
	}
	refs := make([]successorRef, int(total))
	fillEdge := func(edge Edge, index uint32, ordinal int) error {
		if ordinal >= int(^uint32(0)) {
			return errors.New("program/flow/causal: successor Plan ordinal is invalid")
		}
		plane := &s.result.index.planes[keyspace.TermFamily(edge.From)]
		rangeValue := &plane.ranges[keyspace.TermOrdinal(edge.From)]
		if rangeValue.start >= rangeValue.end || uint64(rangeValue.start) >= uint64(len(refs)) {
			return errors.New("program/flow/causal: successor fill range is invalid")
		}
		ref := successorRef{index: index, local: true, arm: BoundaryLocal}
		if ordinal >= 0 {
			ref.planOrdinal, ref.planOrdinalSet = uint32(ordinal), true
		}
		refs[rangeValue.start] = ref
		rangeValue.start++
		return nil
	}
	if len(s.edgeRowsScratch.planOrdinals) != 0 && len(s.edgeRowsScratch.planOrdinals) != len(s.edgeRows) {
		return errors.New("program/flow/causal: Edge Plan ordinal scratch is malformed")
	}
	for index, edge := range s.edgeRows {
		ordinal := -1
		if len(s.edgeRowsScratch.planOrdinals) != 0 {
			if index >= len(s.edgeRowsScratch.planOrdinals) {
				return errors.New("program/flow/causal: Edge Plan ordinal disappeared")
			}
			ordinal = s.edgeRowsScratch.planOrdinals[index]
		}
		if err := fillEdge(edge.Edge, uint32(index), ordinal); err != nil {
			return err
		}
	}
	if len(s.boundaryRowsScratch.planOrdinals) != 0 && len(s.boundaryRowsScratch.planOrdinals) != len(s.boundaryRows) {
		return errors.New("program/flow/causal: CallBoundary Plan ordinal scratch is malformed")
	}
	for index, boundary := range s.boundaryRows {
		var planned boundaryPlanOrdinals
		planBound := len(s.boundaryRowsScratch.planOrdinals) != 0
		if planBound {
			planned = s.boundaryRowsScratch.planOrdinals[index]
		}
		plane := &s.result.index.planes[keyspace.FamilyCall]
		rangeValue := &plane.ranges[keyspace.TermOrdinal(boundary.Call)]
		for _, arm := range [...]BoundaryArmKind{BoundaryResume, BoundarySelectTrue, BoundarySelectFalse, BoundaryTail, BoundaryThrow, BoundaryYield, BoundaryCancel} {
			if !boundaryArmPresent(boundary, arm) {
				continue
			}
			if rangeValue.start >= rangeValue.end || uint64(rangeValue.start) >= uint64(len(refs)) {
				return errors.New("program/flow/causal: successor boundary fill range is invalid")
			}
			if planBound && (!planned.present[arm] || planned.ordinals[arm] < 0 || planned.ordinals[arm] >= int(^uint32(0))) {
				return errors.New("program/flow/causal: CallBoundary arm Plan ordinal is invalid")
			}
			ref := successorRef{index: uint32(index), local: false, arm: arm}
			if planBound {
				ref.planOrdinal, ref.planOrdinalSet = uint32(planned.ordinals[arm]), true
			}
			refs[rangeValue.start] = ref
			rangeValue.start++
		}
	}
	// Restore each range start without a second denominator-sized cursor plane.
	for _, edge := range s.edgeRows {
		plane := &s.result.index.planes[keyspace.TermFamily(edge.From)]
		rangeValue := &plane.ranges[keyspace.TermOrdinal(edge.From)]
		if rangeValue.start == 0 {
			return errors.New("program/flow/causal: successor edge prefix underflows")
		}
		rangeValue.start--
	}
	for _, boundary := range s.boundaryRows {
		plane := &s.result.index.planes[keyspace.FamilyCall]
		rangeValue := &plane.ranges[keyspace.TermOrdinal(boundary.Call)]
		for _, arm := range [...]BoundaryArmKind{BoundaryResume, BoundarySelectTrue, BoundarySelectFalse, BoundaryTail, BoundaryThrow, BoundaryYield, BoundaryCancel} {
			if boundaryArmPresent(boundary, arm) {
				if rangeValue.start == 0 {
					return errors.New("program/flow/causal: successor boundary prefix underflows")
				}
				rangeValue.start--
			}
		}
	}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		plane := &s.result.index.planes[family]
		for ordinal := uint32(1); ordinal < uint32(len(plane.ranges)); ordinal++ {
			if plane.ranges[ordinal].start > plane.ranges[ordinal].end {
				return errors.New("program/flow/causal: successor prefix/fill mismatch")
			}
		}
	}
	s.result.index.refs = refs
	return nil
}

func boundaryArmPresent(row boundaryRow, arm BoundaryArmKind) bool {
	b := row.CallBoundary
	switch arm {
	case BoundaryResume:
		return b.mode == boundaryDirect && b.Normal != 0
	case BoundarySelectTrue, BoundarySelectFalse:
		return (b.mode == boundarySelectAnd || b.mode == boundarySelectOr) && b.Normal != 0 && b.Other != 0
	case BoundaryTail:
		return b.mode == boundaryTail && b.TailReturn != 0
	case BoundaryThrow:
		return b.Throw != 0
	case BoundaryYield:
		return b.Yield != 0
	case BoundaryCancel:
		return b.Cancel != 0
	default:
		return false
	}
}

func boundaryArmCount(row boundaryRow) int {
	count := 0
	for _, arm := range [...]BoundaryArmKind{
		BoundaryResume, BoundarySelectTrue, BoundarySelectFalse, BoundaryTail,
		BoundaryThrow, BoundaryYield, BoundaryCancel,
	} {
		if boundaryArmPresent(row, arm) {
			count++
		}
	}
	return count
}
