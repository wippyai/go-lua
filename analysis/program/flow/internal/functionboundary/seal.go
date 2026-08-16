package functionboundary

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/body"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/evaluation"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/outcome"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

// Seal joins the already-existing Source formal order, authored Flow closure
// rows, Body ownership, evaluation entries, and canonical Outcome exits. It
// retains no input owner and introduces no new semantic identity. All Body
// and Outcome rows are materialized once in canonical dense order, including
// the assembly root/chunk Body.
func Seal(
	preimage source.Preimage,
	view authored.View,
	bodies *body.Result,
	ports *evaluation.Ports,
	outcomes *outcome.Result,
	staticID identity.ContentID,
	moduleID identity.ContentID,
	entry keyspace.Term,
) (*Result, error) {
	sourceID := preimage.Identity().ContentID()
	flowID := view.Cold().ContentID()
	if !sourceID.Available() || !flowID.Available() || !staticID.Available() || !moduleID.Available() {
		return nil, errors.New("program/flow/functionboundary: owner identity unavailable")
	}
	if !body.Matches(bodies, sourceID, flowID) || !evaluation.Matches(ports, sourceID, flowID, staticID, moduleID) ||
		!outcome.Matches(outcomes, sourceID, flowID, staticID, moduleID) {
		return nil, errors.New("program/flow/functionboundary: prerequisite provenance disagrees")
	}
	if keyspace.TermFamily(entry) != keyspace.FamilyBody || keyspace.TermOrdinal(entry) == 0 {
		return nil, errors.New("program/flow/functionboundary: invalid assembly Entry")
	}
	if parent, hasParent := bodies.Parent(entry); hasParent || parent != 0 {
		return nil, errors.New("program/flow/functionboundary: assembly Entry has a Body parent")
	}
	if _, ok := bodies.Activation(entry); !ok {
		return nil, errors.New("program/flow/functionboundary: assembly Entry is not sealed")
	}

	bodyCount := bodies.BodyCount()
	functionView := view.Functions()
	result := &Result{
		sourceID: sourceID, flowID: flowID, staticID: staticID, moduleID: moduleID, entry: entry,
		functions: make([]functionRow, functionView.Count()), bodies: make([]bodyRow, bodyCount+1),
		byBody: make([]uint32, bodyCount+1), byOutcome: make([]uint32, outcomes.Count()+1),
		bodyByOutcome: make([]uint32, outcomes.Count()+1), outcomeAt: make([]uint32, outcomes.Count()+1),
		contexts:     make(map[identity.ContentID]uint32, functionView.Count()),
		bodyContexts: make(map[identity.ContentID]uint32, bodyCount),
	}

	// Build every Body row directly from the sealed dense Body/Ports/Outcome
	// authorities. This includes the root Body and never infers it by a scan.
	for ordinal := 1; ordinal <= bodyCount; ordinal++ {
		bodyTerm := keyspace.MakeTerm(keyspace.FamilyBody, uint32(ordinal))
		bodyEntry, ok := ports.Entry(bodyTerm)
		if !ok || bodyEntry == 0 {
			return nil, errors.New("program/flow/functionboundary: Body entry is unavailable")
		}
		start, end, ok := outcomes.BodyRange(bodyTerm)
		if !ok || start < 0 || end < start || end > outcomes.Count() {
			return nil, errors.New("program/flow/functionboundary: Body Outcome range is unavailable")
		}
		row := bodyRow{body: bodyTerm, entry: bodyEntry, outcomes: range32{start: uint32(len(result.outcomes))}}
		for outcomeIndex := start; outcomeIndex < end; outcomeIndex++ {
			outcomeTerm, ok := outcomes.At(outcomeIndex)
			if !ok {
				return nil, errors.New("program/flow/functionboundary: Outcome row disappeared")
			}
			bodyOwner, outcomeKind, target, ok := outcomes.Get(outcomeTerm)
			if !ok || bodyOwner != bodyTerm {
				return nil, errors.New("program/flow/functionboundary: Outcome owner disagrees with Body")
			}
			outcomeOrdinal := keyspace.TermOrdinal(outcomeTerm)
			if keyspace.TermFamily(outcomeTerm) != keyspace.FamilyOutcome || outcomeOrdinal == 0 ||
				uint64(outcomeOrdinal) >= uint64(len(result.outcomeAt)) || result.outcomeAt[outcomeOrdinal] != 0 {
				return nil, errors.New("program/flow/functionboundary: Outcome denominator is not canonical")
			}
			result.outcomeAt[outcomeOrdinal] = uint32(len(result.outcomes) + 1)
			result.bodyByOutcome[outcomeOrdinal] = uint32(ordinal)
			result.outcomes = append(result.outcomes, outcomeRow{term: outcomeTerm, body: bodyOwner, kind: outcomeKind, target: target})
		}
		row.outcomes.end = uint32(len(result.outcomes))
		result.bodies[ordinal] = row
	}
	if len(result.outcomes) != outcomes.Count() {
		return nil, errors.New("program/flow/functionboundary: Body ranges do not cover Outcome rows")
	}

	cells := view.Storage().Cells()
	formalView := preimage.Formals()
	for index := 0; index < functionView.Count(); index++ {
		function, ok := functionView.At(index)
		if !ok || keyspace.TermFamily(function) != keyspace.FamilyFunction || keyspace.TermOrdinal(function) != uint32(index+1) {
			return nil, errors.New("program/flow/functionboundary: Function denominator is not canonical")
		}
		owner, functionBody, vararg, ok := functionView.Get(function)
		if !ok || keyspace.TermFamily(owner) != keyspace.FamilyBody || keyspace.TermOrdinal(owner) == 0 ||
			keyspace.TermFamily(functionBody) != keyspace.FamilyBody || keyspace.TermOrdinal(functionBody) == 0 || owner == functionBody {
			return nil, errors.New("program/flow/functionboundary: malformed Function Body authority")
		}
		parent, hasParent := bodies.Parent(functionBody)
		if !hasParent || parent != owner {
			return nil, errors.New("program/flow/functionboundary: Function owner disagrees with Body parent")
		}
		activation, ok := bodies.Activation(functionBody)
		if !ok || activation != function {
			return nil, errors.New("program/flow/functionboundary: Function activation disagrees with Body")
		}
		bodyOrdinal := keyspace.TermOrdinal(functionBody)
		if uint64(bodyOrdinal) >= uint64(len(result.bodies)) || result.bodies[bodyOrdinal].body != functionBody || result.bodies[bodyOrdinal].function != 0 {
			return nil, errors.New("program/flow/functionboundary: Function Body inverse is not unique")
		}
		bodyRow := result.bodies[bodyOrdinal]
		row := functionRow{function: function, owner: owner, body: functionBody, entry: bodyRow.entry, vararg: vararg, outcomes: bodyRow.outcomes}
		formalCount, ok := formalView.Len(function)
		if !ok || formalCount < 0 {
			return nil, errors.New("program/flow/functionboundary: formal order is unavailable")
		}
		row.formals.start = uint32(len(result.formals))
		for formalIndex := 0; formalIndex < formalCount; formalIndex++ {
			cell, ok := formalView.At(function, formalIndex)
			if !ok || !localCell(cells, cell, functionBody) {
				return nil, errors.New("program/flow/functionboundary: malformed formal Cell")
			}
			result.formals = append(result.formals, cell)
		}
		row.formals.end = uint32(len(result.formals))
		if vararg != 0 && !localCell(cells, vararg, functionBody) {
			return nil, errors.New("program/flow/functionboundary: malformed vararg Cell")
		}
		captureCount, ok := functionView.CaptureCount(function)
		if !ok || captureCount < 0 {
			return nil, errors.New("program/flow/functionboundary: capture order is unavailable")
		}
		row.captures.start = uint32(len(result.captures))
		for captureIndex := 0; captureIndex < captureCount; captureIndex++ {
			inner, outer, ok := functionView.CaptureAt(function, captureIndex)
			if !ok || !localCell(cells, inner, functionBody) {
				return nil, errors.New("program/flow/functionboundary: malformed capture inner Cell")
			}
			outerBody, outerOK := localCellBody(cells, outer)
			if !outerOK || outerBody == functionBody || !bodies.AncestorOrSelf(outerBody, functionBody) {
				return nil, errors.New("program/flow/functionboundary: malformed capture outer Cell")
			}
			result.captures = append(result.captures, captureRow{inner: inner, outer: outer, innerBody: functionBody, outerBody: outerBody})
		}
		row.captures.end = uint32(len(result.captures))
		result.functions[index] = row
		result.byBody[bodyOrdinal] = uint32(index + 1)
		result.bodies[bodyOrdinal].function = uint32(index + 1)
	}
	if result.byBody[keyspace.TermOrdinal(entry)] != 0 {
		return nil, errors.New("program/flow/functionboundary: root Body was assigned a Function")
	}

	// Hash all semantic rows after their canonical pools are complete, then
	// build both inverse identity maps linearly with collision rejection.
	for ordinal := 1; ordinal <= bodyCount; ordinal++ {
		row := result.bodies[ordinal]
		row.context = hashBodyContext(result, row)
		if !row.context.Available() {
			return nil, errors.New("program/flow/functionboundary: Body context is unavailable")
		}
		if _, exists := result.bodyContexts[row.context]; exists {
			return nil, errors.New("program/flow/functionboundary: Body context collision")
		}
		result.bodyContexts[row.context] = uint32(ordinal)
		result.bodies[ordinal] = row
	}
	for index := range result.functions {
		row := result.functions[index]
		row.context = hashContext(result, row)
		if !row.context.Available() {
			return nil, errors.New("program/flow/functionboundary: Function context is unavailable")
		}
		if _, exists := result.contexts[row.context]; exists {
			return nil, errors.New("program/flow/functionboundary: Function context collision")
		}
		if _, exists := result.bodyContexts[row.context]; exists {
			return nil, errors.New("program/flow/functionboundary: Function context collision")
		}
		result.contexts[row.context] = uint32(index + 1)
		result.functions[index] = row
	}
	for ordinal := uint32(1); ordinal < uint32(len(result.bodyByOutcome)); ordinal++ {
		bodyOrdinal := result.bodyByOutcome[ordinal]
		if bodyOrdinal == 0 || uint64(bodyOrdinal) >= uint64(len(result.bodies)) {
			return nil, errors.New("program/flow/functionboundary: Outcome Body inverse is unavailable")
		}
		result.byOutcome[ordinal] = result.bodies[bodyOrdinal].function
	}
	if !result.validateResult() {
		return nil, errors.New("program/flow/functionboundary: malformed sealed relation")
	}
	result.sealed = true
	return result, nil
}

func localCell(cells authored.Cells, cell, expectedBody keyspace.Term) bool {
	body, ok := localCellBody(cells, cell)
	return ok && body == expectedBody
}

func localCellBody(cells authored.Cells, cell keyspace.Term) (keyspace.Term, bool) {
	if keyspace.TermFamily(cell) != keyspace.FamilyCell || keyspace.TermOrdinal(cell) == 0 {
		return 0, false
	}
	cellKind, body, _, ok := cells.Get(cell)
	if !ok || cellKind != authored.CellLocal || keyspace.TermFamily(body) != keyspace.FamilyBody || keyspace.TermOrdinal(body) == 0 {
		return 0, false
	}
	return body, true
}
