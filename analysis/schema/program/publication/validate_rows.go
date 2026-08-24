package publication

import (
	"bytes"

	"github.com/wippyai/go-lua/analysis/identity"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/program"
)

func (validator *validator) validateSealRows(state *validationState) bool {
	if validator == nil || state == nil {
		return false
	}
	outcomeCount, outcomesPublished := programschema.OutcomeFamily().Count(&validator.frozen, validator.catalog)
	returnValueCount, returnsPublished := programschema.OutcomeReturnValueFamily().Count(&validator.frozen, validator.catalog)
	outcomePointCount, pointsPublished := programschema.OutcomePointFamily().Count(&validator.frozen, validator.catalog)
	if !outcomesPublished || !returnsPublished || !pointsPublished {
		return false
	}
	returnCursor, pointCursor := uint32(0), uint32(0)
	for index := 0; index < outcomeCount; index++ {
		row, held := programschema.OutcomeFamily().At(&validator.frozen, validator.catalog, index)
		returnOffset, returnWidth, returnSpanOK := row.ReturnValueSpan()
		pointOffset, pointWidth, pointSpanOK := row.PointSpan()
		if !held || !returnSpanOK || !pointSpanOK {
			return false
		}
		if _, exists := state.bodyRows[row.BodyID()]; !exists {
			return false
		}
		if _, duplicate := state.outcomeRows[row.ID()]; duplicate {
			return false
		}
		if pointOffset != pointCursor || uint64(pointOffset)+uint64(pointWidth) > uint64(outcomePointCount) {
			return false
		}
		outcomePoints := make(map[identity.ContentID]struct{}, pointWidth)
		for pointIndex := uint32(0); pointIndex < pointWidth; pointIndex++ {
			child, childHeld := programschema.OutcomePointFamily().At(&validator.frozen, validator.catalog, int(pointOffset+pointIndex))
			point := child.PointID()
			if !childHeld || child.OutcomeID() != row.ID() {
				return false
			}
			if _, known := state.pointRows[point]; !known {
				return false
			}
			if _, duplicate := outcomePoints[point]; duplicate {
				return false
			}
			outcomePoints[point] = struct{}{}
		}
		if returnOffset != returnCursor || uint64(returnOffset)+uint64(returnWidth) > uint64(returnValueCount) {
			return false
		}
		for valueIndex := uint32(0); valueIndex < returnWidth; valueIndex++ {
			value, valueHeld := programschema.OutcomeReturnValueFamily().At(&validator.frozen, validator.catalog, int(returnOffset+valueIndex))
			if !valueHeld || value.OutcomeID() != row.ID() {
				return false
			}
			if _, exists := state.valueRows[value.ValuesID()]; !exists {
				return false
			}
		}
		state.outcomeRows[row.ID()] = index
		returnCursor += returnWidth
		pointCursor += pointWidth
	}
	if int(returnCursor) != returnValueCount || int(pointCursor) != outcomePointCount {
		return false
	}
	for index := 0; index < outcomeCount; index++ {
		row, _ := programschema.OutcomeFamily().At(&validator.frozen, validator.catalog, index)
		propagation, propagated := row.PropagationID()
		if !propagated {
			continue
		}
		nextIndex, exists := state.outcomeRows[propagation]
		if !exists || nextIndex == index {
			return false
		}
		next, nextHeld := programschema.OutcomeFamily().At(&validator.frozen, validator.catalog, nextIndex)
		target, hasTarget := row.TargetID()
		nextTarget, nextHasTarget := next.TargetID()
		if !nextHeld || next.Kind() != row.Kind() || nextHasTarget != hasTarget || nextTarget != target {
			return false
		}
	}
	edgeCount, edgesPublished := programschema.EnvironmentEdgeFamily().Count(&validator.frozen, validator.catalog)
	if !edgesPublished {
		return false
	}
	for index := 0; index < edgeCount; index++ {
		row, held := programschema.EnvironmentEdgeFamily().At(&validator.frozen, validator.catalog, index)
		resetOffset, resetCount, spanOK := row.ResetSpan()
		if !held || !spanOK {
			return false
		}
		if _, exists := state.pointRows[row.From()]; !exists {
			return false
		}
		if _, exists := state.pointRows[row.To()]; !exists {
			return false
		}
		var previousReset identity.ContentID
		for resetIndex := uint32(0); resetIndex < resetCount; resetIndex++ {
			reset, resetHeld := programschema.EnvironmentResetFamily().At(&validator.frozen, validator.catalog, int(resetOffset+resetIndex))
			if !resetHeld {
				return false
			}
			if resetIndex != 0 && !contentIDBefore(previousReset, reset.ID()) {
				return false
			}
			previousReset = reset.ID()
		}
	}
	localTransferCount, localTransfersPublished := programschema.LocalTransferFamily().Count(&validator.frozen, validator.catalog)
	localTransferWriteCount, localTransferWritesPublished := programschema.LocalTransferWriteFamily().Count(&validator.frozen, validator.catalog)
	if !localTransfersPublished || !localTransferWritesPublished {
		return false
	}
	seenLocalTransfers := make(map[identity.ContentID]struct{}, localTransferCount)
	consumedTransferWrites := uint32(0)
	for index := 0; index < localTransferCount; index++ {
		row, held := programschema.LocalTransferFamily().At(&validator.frozen, validator.catalog, index)
		offset, writeCount, spanOK := row.WriteSpan()
		if !held || !spanOK || offset != consumedTransferWrites || uint64(offset)+uint64(writeCount) > uint64(localTransferWriteCount) {
			return false
		}
		if _, exists := state.pointRows[row.From()]; !exists {
			return false
		}
		if _, exists := state.pointRows[row.To()]; !exists {
			return false
		}
		if _, duplicate := seenLocalTransfers[row.ID()]; duplicate {
			return false
		}
		seenLocalTransfers[row.ID()] = struct{}{}
		var prior schema.Key
		for child := uint32(0); child < writeCount; child++ {
			write, writeHeld := programschema.LocalTransferWriteFamily().At(&validator.frozen, validator.catalog, int(offset+child))
			key, keyOK := write.Key()
			if !writeHeld || !keyOK || child != 0 && prior >= key {
				return false
			}
			prior = key
		}
		consumedTransferWrites += writeCount
	}
	if uint64(consumedTransferWrites) != uint64(localTransferWriteCount) {
		return false
	}
	// Storage-cell lifetime rows are a neutral bridge consumed by mounted
	// domains. Artifact sealing authenticates only their own identity/value
	// uniqueness here; the compiler is the authority that proved the Flow
	// ownership classification before this publication was built.
	lifecycleView, lifecycleViewOK := validator.lifecycle, validator.lifecycle.Available()
	if !lifecycleViewOK {
		return false
	}
	lifetimeCount, lifetimesPublished := lifecycleView.StorageCellLifetimeCount()
	if !lifetimesPublished {
		return false
	}
	seenLifetimes := make(map[identity.ContentID]struct{}, lifetimeCount)
	for index := 0; index < lifetimeCount; index++ {
		row, held := lifecycleView.StorageCellLifetimeAt(index)
		if !held || !row.Available() {
			return false
		}
		if _, duplicate := seenLifetimes[row.ID()]; duplicate {
			return false
		}
		seenLifetimes[row.ID()] = struct{}{}
	}
	// The suspension plane is published as an ordered boundary sequence plus
	// per-subject live ranges over it. Their constructors authenticate the
	// derived row identity; sealing owns the publication-wide uniqueness gate
	// so a malformed or repeated row cannot cross the Artifact boundary.
	boundaryCount, boundariesPublished := lifecycleView.SubjectYieldBoundaryCount()
	if !boundariesPublished {
		return false
	}
	seenBoundaries := make(map[identity.ContentID]struct{}, boundaryCount)
	seenOrdinals := make(map[uint32]struct{}, boundaryCount)
	for index := 0; index < boundaryCount; index++ {
		row, held := lifecycleView.SubjectYieldBoundaryAt(index)
		if !held || !row.Available() {
			return false
		}
		if _, duplicate := seenBoundaries[row.ID()]; duplicate {
			return false
		}
		// The ordinal is the coordinate every span is a range over. Two
		// boundaries at one ordinal would make a range ambiguous.
		if _, duplicate := seenOrdinals[row.Ordinal()]; duplicate {
			return false
		}
		seenBoundaries[row.ID()] = struct{}{}
		seenOrdinals[row.Ordinal()] = struct{}{}
	}
	spanCount, spansPublished := lifecycleView.SubjectLivenessSpanCount()
	if !spansPublished {
		return false
	}
	seenSpans := make(map[identity.ContentID]struct{}, spanCount)
	for index := 0; index < spanCount; index++ {
		row, held := lifecycleView.SubjectLivenessSpanAt(index)
		if !held || !row.Available() {
			return false
		}
		if _, duplicate := seenSpans[row.ID()]; duplicate {
			return false
		}
		seenSpans[row.ID()] = struct{}{}
	}
	// Alias and Unknown events are the authenticated local boundary facts.
	// Their Flow semantic paths and causal route identities are checked at
	// compiler admission; sealing owns the Artifact-wide row uniqueness gate.
	subjectEventCount, subjectEventsPublished := lifecycleView.SubjectEventCount()
	if !subjectEventsPublished {
		return false
	}
	seenEvents := make(map[identity.ContentID]struct{}, subjectEventCount)
	for index := 0; index < subjectEventCount; index++ {
		row, held := lifecycleView.SubjectEventAt(index)
		if !held || !row.Available() {
			return false
		}
		if _, duplicate := seenEvents[row.ID()]; duplicate {
			return false
		}
		seenEvents[row.ID()] = struct{}{}
	}
	scopeCount, scopesPublished := lifecycleView.AliasRouteScopeCount()
	memberCount, membersPublished := lifecycleView.AliasRouteScopeMemberCount()
	if !scopesPublished || !membersPublished {
		return false
	}
	seenScopes := make(map[identity.ContentID]struct{}, scopeCount)
	seenScopeKeys := make(map[[2]identity.ContentID]struct{}, scopeCount)
	memberCursor := uint32(0)
	for index := 0; index < scopeCount; index++ {
		row, held := lifecycleView.AliasRouteScopeAt(index)
		offset, count, spanOK := row.MemberSpan()
		if !held || !row.Available() || !spanOK || offset != memberCursor || uint64(offset)+uint64(count) > uint64(memberCount) {
			return false
		}
		if _, duplicate := seenScopes[row.ID()]; duplicate {
			return false
		}
		kindKey := identity.ContentID{byte(row.Kind())}
		scopeKey := [2]identity.ContentID{kindKey, row.BodyID()}
		if _, duplicate := seenScopeKeys[scopeKey]; duplicate {
			return false
		}
		seenScopes[row.ID()] = struct{}{}
		seenScopeKeys[scopeKey] = struct{}{}
		previous := identity.ContentID{}
		for memberIndex := uint32(0); memberIndex < count; memberIndex++ {
			member, memberHeld := lifecycleView.AliasRouteScopeMemberAt(int(offset + memberIndex))
			ordinal, ordinalOK := member.Ordinal()
			route := member.RouteID()
			if !memberHeld || !member.Available() || !ordinalOK || member.ScopeID() != row.ID() || ordinal != memberIndex ||
				memberIndex > 0 && bytes.Compare(previous[:], route[:]) >= 0 {
				return false
			}
			previous = route
		}
		memberCursor += count
	}
	if uint64(memberCursor) != uint64(memberCount) {
		return false
	}
	candidateCount, candidatesPublished := lifecycleView.AliasCandidateCount()
	if !candidatesPublished {
		return false
	}
	seenCandidates := make(map[[2]identity.ContentID]struct{}, candidateCount)
	for index := 0; index < candidateCount; index++ {
		row, held := lifecycleView.AliasCandidateAt(index)
		if !held || !row.Available() {
			return false
		}
		if _, scopeHeld := seenScopes[row.ScopeID()]; !scopeHeld {
			return false
		}
		kindKey := identity.ContentID{byte(row.CandidateKind())}
		candidateKey := [2]identity.ContentID{kindKey, row.CandidateID()}
		if _, duplicate := seenCandidates[candidateKey]; duplicate {
			return false
		}
		seenCandidates[candidateKey] = struct{}{}
	}
	regionCount, regionsPublished := programschema.RegionFamily().Count(&validator.frozen, validator.catalog)
	if !regionsPublished {
		return false
	}
	regionRows := make(map[identity.ContentID]struct{}, regionCount)
	for index := 0; index < regionCount; index++ {
		row, held := programschema.RegionFamily().At(&validator.frozen, validator.catalog, index)
		memberOffset, memberCount, spanOK := row.MemberSpan()
		if !held || !spanOK {
			return false
		}
		if _, exists := regionRows[row.ID()]; exists {
			return false
		}
		regionRows[row.ID()] = struct{}{}
		var previousMember identity.ContentID
		for memberIndex := uint32(0); memberIndex < memberCount; memberIndex++ {
			member, memberHeld := programschema.RegionMemberFamily().At(&validator.frozen, validator.catalog, int(memberOffset+memberIndex))
			if !memberHeld {
				return false
			}
			if _, exists := state.pointRows[member.ID()]; !exists || memberIndex != 0 && member.ID() == previousMember {
				return false
			}
			previousMember = member.ID()
		}
	}
	for index := 0; index < regionCount; index++ {
		row, held := programschema.RegionFamily().At(&validator.frozen, validator.catalog, index)
		if !held {
			return false
		}
		if row.ParentID().Available() {
			if _, exists := regionRows[row.ParentID()]; !exists {
				return false
			}
		}
	}
	eventCount, eventsPublished := programschema.WTOEventFamily().Count(&validator.frozen, validator.catalog)
	if !eventsPublished {
		return false
	}
	for index := 0; index < eventCount; index++ {
		event, held := programschema.WTOEventFamily().At(&validator.frozen, validator.catalog, index)
		if !held {
			return false
		}
		if event.Kind() == programschema.WTOEventPoint {
			if _, exists := state.pointRows[event.PointID()]; !exists {
				return false
			}
		} else if _, exists := regionRows[event.RegionID()]; !exists {
			return false
		}
	}
	program := validator.program
	exactCount, exactPublished := programschema.ExactScalarSummaryFamily().Count(&validator.frozen, validator.catalog)
	if !exactPublished {
		return false
	}
	var priorExact identity.ContentID
	for index := 0; index < exactCount; index++ {
		row, rowOK := programschema.ExactScalarSummaryFamily().At(&validator.frozen, validator.catalog, index)
		if !rowOK || index > 0 && !contentIDBefore(priorExact, row.ID()) {
			return false
		}
		priorExact = row.ID()
		entry, found := state.occurrence(programschema.OccurrenceBinaryArithmetic, row.OccurrenceID())
		if !found {
			return false
		}
		binary := entry.row
		leftRow, leftOK := program.OccurrenceInputFor(entry.ordinal, 0)
		rightRow, rightOK := program.OccurrenceInputFor(entry.ordinal, 1)
		left, right, endpointsOK := leftRow.InputID(), rightRow.InputID(), leftOK && rightOK
		body, bodyOK := binary.BodyID()
		endpointsOK = endpointsOK && bodyOK && body == row.BodyPathID()
		wantSubject := binary.ID()
		switch row.Role() {
		case programschema.ExactScalarSummaryLeft:
			wantSubject = left
		case programschema.ExactScalarSummaryRight:
			wantSubject = right
		case programschema.ExactScalarSummaryResult:
		default:
			endpointsOK = false
		}
		if !endpointsOK || row.SubjectID() != wantSubject {
			return false
		}
	}
	arithmeticCount, arithmeticPublished := programschema.ArithmeticSummaryFamily().Count(&validator.frozen, validator.catalog)
	if !arithmeticPublished {
		return false
	}
	var priorArithmetic identity.ContentID
	for index := 0; index < arithmeticCount; index++ {
		row, rowOK := programschema.ArithmeticSummaryFamily().At(&validator.frozen, validator.catalog, index)
		if !rowOK || index > 0 && !contentIDBefore(priorArithmetic, row.ID()) {
			return false
		}
		priorArithmetic = row.ID()
		entry, found := state.occurrence(programschema.OccurrenceBinaryArithmetic, row.OccurrenceID())
		if !found {
			return false
		}
		binary := entry.row
		body, bodyOK := binary.BodyID()
		if !bodyOK || body != row.BodyPathID() || flowkind.BinaryOp(binary.Code()) != flowkind.BinaryOp(row.Operator()) {
			return false
		}
	}
	unaryCount, unaryPublished := programschema.UnarySummaryFamily().Count(&validator.frozen, validator.catalog)
	if !unaryPublished {
		return false
	}
	var priorUnary identity.ContentID
	for index := 0; index < unaryCount; index++ {
		row, rowOK := programschema.UnarySummaryFamily().At(&validator.frozen, validator.catalog, index)
		if !rowOK || index > 0 && !contentIDBefore(priorUnary, row.ID()) {
			return false
		}
		priorUnary = row.ID()
		entry, found := state.occurrence(programschema.OccurrenceUnary, row.OccurrenceID())
		if !found {
			return false
		}
		unary := entry.row
		body, bodyOK := unary.BodyID()
		if !bodyOK || body != row.BodyPathID() {
			return false
		}
		if flowkind.UnaryOp(unary.Code()) != flowkind.UnaryOp(row.Operator()) {
			return false
		}
		pointFound := false
		pointOffset, pointCount, pointSpanOK := unary.PointSpan()
		for position := uint32(0); pointSpanOK && position < pointCount; position++ {
			point, pointOK := program.OccurrencePointAt(int(pointOffset + position))
			pointFound = pointFound || pointOK && point.PointID() == row.OutputPointID()
		}
		if !pointFound {
			return false
		}
	}

	return true
}
