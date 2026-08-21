package compiler

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/artifact/issuance"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/analysis/schema/program/calltarget"
	"github.com/wippyai/go-lua/analysis/schema/program/heapallocation"
)

// matching selects the subscriptions one compiled occurrence row issues: the
// family it belongs to, the payload code it carries, and the operand shape the
// row itself has. The requirement is decided here rather than after placement,
// so a row an owner cannot seal an operand for is never placed.
func (compiler *compiler) matching(row programschema.Occurrence) ([]issuance.Placement, bool) {
	if !row.Kind().Valid() {
		return nil, false
	}
	matched := make([]issuance.Placement, 0)
	for index := 0; index < compiler.issuance.Count(); index++ {
		placement, present := compiler.issuance.At(index)
		if !present || !placement.Available() || placement.Occurrence != row.Kind() {
			continue
		}
		if placement.HasCode && placement.Code != row.Code() {
			continue
		}
		admissible, decided := compiler.requirementAdmits(placement.Requirement, row)
		if !decided {
			return nil, false
		}
		if admissible {
			matched = append(matched, placement)
		}
	}
	return matched, true
}

// requirementAdmits decides one declared operand shape against one compiled
// row. The second result is whether the shape could be decided at all: a
// requirement naming a geometry the row's family does not carry is a
// declaration the artifact cannot honor, and it refuses the compile rather
// than placing the row on an unstated reading.
func (compiler *compiler) requirementAdmits(requirement issuance.Requirement, row programschema.Occurrence) (bool, bool) {
	switch requirement {
	case issuance.RequirementUnrestricted:
		return true, true
	case issuance.RequirementCallPlainUnary:
		call, found := compiler.callForID(row.ID())
		if !found {
			return false, false
		}
		_, hasReceiver := call.ReceiverID()
		_, hasTail := call.TailID()
		return call.Form() == programschema.CallFormPlain && call.ArgumentCount() == 1 && !hasReceiver && !hasTail, true
	case issuance.RequirementCallResultSlot:
		if row.Kind() == programschema.OccurrenceStorageBindTransfer {
			valueID, valueOK := programschema.OccurrenceInputID(row, compiler.publication.OccurrenceInputs, 1)
			cellID, cellOK := programschema.OccurrenceInputID(row, compiler.publication.OccurrenceInputs, 2)
			if !valueOK || !cellOK {
				return false, false
			}
			for _, slot := range compiler.publication.CallResultSlots {
				slotValue, hasValue := slot.ValueID()
				if slot.Available() && hasValue && slotValue == valueID &&
					slot.SourceKind() == programschema.CallResultSlotSourceValuesTail &&
					slot.ConsumerKind() == programschema.CallResultSlotConsumerCell && slot.ConsumerID() == cellID {
					return true, true
				}
			}
			return false, true
		}
		call, found := compiler.callForID(row.ID())
		if !found {
			return false, false
		}
		_, hasReceiver := call.ReceiverID()
		_, hasTail := call.TailID()
		if call.Form() != programschema.CallFormPlain || call.ArgumentCount() != 1 || hasReceiver || hasTail {
			return false, true
		}
		slot, slotOK := compiler.callResultSlotFor(call.ID(), 0)
		if !slotOK {
			// A valid Call with no consumer-side CallResult is a normal
			// non-admission (for example a discarded statement call), not a
			// malformed declaration.
			return false, true
		}
		_, hasValue := slot.ValueID()
		return hasValue, true
	case issuance.RequirementClosureCapture:
		return compiler.closureCaptureAdmits(row)
	default:
		return false, false
	}
}

// closureCaptureAdmits recognizes the one mounted capture denominator from
// the compiler's canonical artifact rows. It deliberately scans the sealed
// construction slices directly: no Flow handle, inverse map, or domain
// coordinate is needed to decide whether an allocation has a positive
// capture boundary.
func (compiler *compiler) closureCaptureAdmits(row programschema.Occurrence) (bool, bool) {
	if compiler == nil || !row.Available() || row.Kind() != programschema.OccurrenceAllocation {
		return false, false
	}
	if compiler.allocations == nil {
		return false, false
	}
	allocation, allocationOK := compiler.allocations.AllocationForID(row.ID())
	if !allocationOK {
		return false, false
	}
	if allocation.Role() != heapallocation.RoleClosure {
		return false, true
	}

	var target calltarget.Target
	foundTarget := false
	for _, candidate := range compiler.publication.CallTargets {
		if !candidate.Available() || candidate.AllocationID() != row.ID() {
			continue
		}
		if foundTarget {
			return false, false
		}
		target = candidate
		foundTarget = true
	}
	if !foundTarget {
		return false, false
	}
	boundary, boundaryOK := compiler.bodyBoundary.FunctionBoundaryForBody(target.BodyID())
	if !boundaryOK {
		return false, false
	}
	if boundary.CaptureCount() == 0 {
		return false, true
	}
	offset, count, spanOK := boundary.CaptureSpan()
	if !spanOK || int(count) != boundary.CaptureCount() || uint64(offset)+uint64(count) > uint64(len(compiler.bodyBoundary.FunctionCaptures())) {
		return false, false
	}
	for index := 0; index < int(count); index++ {
		capture := compiler.bodyBoundary.FunctionCaptures()[int(offset)+index]
		position, positionOK := capture.Position()
		if !capture.Available() || !positionOK || position != index || capture.InnerBodyID() != boundary.BodyID() {
			return false, false
		}
	}
	return true, true
}

// callForID resolves one authored call row by the parent-issued identity an
// occurrence row carries. The canonical Call column is the sole authority;
// this construction-only lookup scans it directly and retains no inverse.
func (compiler *compiler) callForID(id identity.ContentID) (programschema.Call, bool) {
	if compiler == nil || !id.Available() {
		return programschema.Call{}, false
	}
	var found programschema.Call
	for _, row := range compiler.publication.Calls {
		if !row.Available() || row.ID() != id {
			continue
		}
		if found.Available() {
			return programschema.Call{}, false
		}
		found = row
	}
	return found, found.Available()
}

func (compiler *compiler) callResultSlotFor(call identity.ContentID, ordinal uint32) (programschema.CallResultSlot, bool) {
	if compiler == nil || !call.Available() {
		return programschema.CallResultSlot{}, false
	}
	var found programschema.CallResultSlot
	for _, row := range compiler.publication.CallResultSlots {
		rowOrdinal, ordinalOK := row.Ordinal()
		if !row.Available() || !ordinalOK || row.CallID() != call || rowOrdinal != ordinal {
			continue
		}
		if found.Available() {
			return programschema.CallResultSlot{}, false
		}
		found = row
	}
	return found, found.Available()
}

func (compiler *compiler) applyIssuance(row programschema.Occurrence, ordinal uint32, geometry occurrenceSpanGeometry, finish []identity.ContentID, placement issuance.Placement) bool {
	if !placement.Available() {
		return false
	}
	if row.Kind() == programschema.OccurrenceValues {
		_, pointCount, pointOK := row.PointSpan()
		if !pointOK {
			return false
		}
		if pointCount == 0 {
			return true
		}
	}
	switch placement.Form {
	case issuance.FormBase:
		return compiler.appendBaseIssuance(ordinal, finish, placement)
	case issuance.FormLocal:
		return compiler.appendLocalIssuance(ordinal, geometry, finish, placement)
	case issuance.FormLocalSuccessor:
		return compiler.appendLocalSuccessorIssuance(ordinal, finish, placement)
	case issuance.FormComputation:
		return compiler.appendComputationIssuance(row, ordinal, finish, placement)
	case issuance.FormLocalPredecessor:
		return compiler.appendLocalPredecessorIssuance(ordinal, geometry, finish, placement)
	case issuance.FormCallStage:
		return compiler.appendCallStageIssuance(ordinal, finish, placement)
	default:
		return false
	}
}

func (compiler *compiler) appendLocalSuccessorIssuance(ordinal uint32, finish []identity.ContentID, issued issuance.Placement) bool {
	if len(finish) == 0 || issued.Input != programschema.RuleInputFinish {
		return false
	}
	for _, base := range finish {
		local, stage, stageOK := compiler.localSuccessorStage(base)
		if !stageOK || !compiler.appendRuleOccurrence(issued.Key, issued.Writes, ordinal, stage, local, programschema.RuleStageLocal, issued.Input, identity.ContentID{}) {
			return false
		}
	}
	return true
}

func (compiler *compiler) appendBaseIssuance(ordinal uint32, finish []identity.ContentID, issued issuance.Placement) bool {
	if len(finish) == 0 || issued.Input != programschema.RuleInputNone {
		return false
	}
	for _, point := range finish {
		if !compiler.appendRuleOccurrence(issued.Key, issued.Writes, ordinal, point, identity.ContentID{}, programschema.RuleStageBase, issued.Input, identity.ContentID{}) {
			return false
		}
	}
	return true
}

func (compiler *compiler) appendLocalIssuance(ordinal uint32, geometry occurrenceSpanGeometry, finish []identity.ContentID, issued issuance.Placement) bool {
	if len(finish) == 0 || issued.Input == programschema.RuleInputNone || issued.Input == programschema.RuleInputPredecessor || issued.Input == programschema.RuleInputEntry && len(geometry.entry) != 1 {
		return false
	}
	for _, base := range finish {
		stage, stageOK := compiler.localStage(base)
		if !stageOK {
			return false
		}
		input := base
		if issued.Input == programschema.RuleInputEntry {
			input = geometry.entry[0]
		}
		if !compiler.appendRuleOccurrence(issued.Key, issued.Writes, ordinal, stage, input, programschema.RuleStageLocal, issued.Input, identity.ContentID{}) {
			return false
		}
	}
	return true
}

func (compiler *compiler) appendComputationIssuance(row programschema.Occurrence, ordinal uint32, finish []identity.ContentID, issued issuance.Placement) bool {
	left, leftOK := programschema.OccurrenceInputID(row, compiler.publication.OccurrenceInputs, 0)
	right, rightOK := programschema.OccurrenceInputID(row, compiler.publication.OccurrenceInputs, 1)
	if len(finish) == 0 || !leftOK || !rightOK {
		return false
	}
	for _, base := range finish {
		stage, stageOK := compiler.localComputationStage(base, issued.Key, row.ID(), left, right)
		if !stageOK || !compiler.appendRuleOccurrence(issued.Key, issued.Writes, ordinal, stage, base, programschema.RuleStageLocal, programschema.RuleInputFinish, identity.ContentID{}) {
			return false
		}
	}
	return true
}

func (compiler *compiler) appendLocalPredecessorIssuance(ordinal uint32, geometry occurrenceSpanGeometry, finish []identity.ContentID, issued issuance.Placement) bool {
	if !geometry.route.Available() {
		return false
	}
	routeIndex, found := compiler.environmentByRoute[geometry.route]
	predecessorOrdinal, unique := routeIndex.uniqueAt(len(compiler.environment))
	if !found || !unique {
		return false
	}
	predecessor := compiler.environment[predecessorOrdinal]
	expectedID := environmentRouteOccurrenceID(compiler.input.ContentID(), geometry.route, predecessor.arm)
	if !predecessor.Available() || predecessor.route != geometry.route || predecessor.id != expectedID {
		return false
	}
	finishMember := false
	for _, point := range finish {
		if point == predecessor.to {
			finishMember = true
			break
		}
	}
	stage, stageOK := compiler.predecessorStage(predecessor.to, issued.Writes)
	return finishMember && stageOK && compiler.appendRuleOccurrence(issued.Key, issued.Writes, ordinal, stage, predecessor.to, programschema.RuleStageLocal, programschema.RuleInputPredecessor, geometry.route)
}

func (compiler *compiler) appendCallStageIssuance(ordinal uint32, finish []identity.ContentID, issued issuance.Placement) bool {
	if len(finish) == 0 || issued.Stage < programschema.RuleStageCallDispatch || issued.Stage > programschema.RuleStageCallEffect {
		return false
	}
	for _, base := range finish {
		stages, stagesOK := compiler.callStage(base)
		if !stagesOK {
			return false
		}
		point, input := stages.Dispatch(), base
		switch issued.Stage {
		case programschema.RuleStageCallSummary:
			point, input = stages.Summary(), stages.Dispatch()
		case programschema.RuleStageCallEffect:
			point, input = stages.Effect(), stages.Summary()
		}
		if !compiler.appendRuleOccurrence(issued.Key, issued.Writes, ordinal, point, input, issued.Stage, programschema.RuleInputFinish, identity.ContentID{}) {
			return false
		}
	}
	return true
}
