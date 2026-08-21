// Package functionboundary owns Flow's immutable function/body-boundary join.
//
// The relation contains only existing Source/Flow/Outcome terms. It is a
// compact owner-fenced index, not a second semantic model and not a generic
// port abstraction.
package functionboundary

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/flow/outcome"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// Result is the sealed FunctionBoundary relation. It also carries the one
// explicit root/chunk Body boundary, so consumers never have to reopen Source
// to recover the assembly entry or its Outcomes.
type Result struct {
	sourceID identity.ContentID
	flowID   identity.ContentID
	staticID identity.ContentID
	moduleID identity.ContentID
	sealed   bool

	entry keyspace.Term

	functions []functionRow
	bodies    []bodyRow
	byBody    []uint32 // Body ordinal -> Function row + 1, zero for root/non-Function Bodies.
	formals   []keyspace.Term
	captures  []captureRow
	// outcomes remains the canonical owner. FunctionBoundary retains only
	// ranges into that owner; it never copies Outcome rows or builds inverse
	// sidecars over them.
	outcomes *outcome.Result

	// These maps are lookup-only indexes over one sealed Result. They are
	// built linearly and reject collisions; no sorted row copy is retained.
	contexts     map[identity.ContentID]uint32 // Function context -> Function row + 1.
	bodyContexts map[identity.ContentID]uint32 // Body context -> Body ordinal.
}

type range32 struct{ start, end uint32 }

type functionRow struct {
	function keyspace.Term
	owner    keyspace.Term
	body     keyspace.Term
	entry    keyspace.Term
	vararg   keyspace.Term
	formals  range32
	captures range32
	outcomes range32
	context  identity.ContentID
}

type bodyRow struct {
	body     keyspace.Term
	entry    keyspace.Term
	outcomes range32
	function uint32
	context  identity.ContentID
}

type captureRow struct {
	inner     keyspace.Term
	outer     keyspace.Term
	innerBody keyspace.Term
	outerBody keyspace.Term
}

// Boundary is an opaque handle into one immutable Function row.
type Boundary struct {
	result *Result
	index  uint32
}

// BodyBoundary is an opaque handle into one existing Body row. It deliberately
// exposes no Function/formal/capture fields: a root Body is not a Function.
type BodyBoundary struct {
	result *Result
	index  uint32
}

// RootBoundary is the explicit assembly-entry Body boundary.
type RootBoundary struct {
	result *Result
	index  uint32
}

// Capture is one ordered pair of existing captured Cells.
type Capture struct {
	Inner     keyspace.Term
	Outer     keyspace.Term
	InnerBody keyspace.Term
	OuterBody keyspace.Term
}

// OutcomeExit is one existing owning-Body Outcome row.
type OutcomeExit struct {
	Outcome keyspace.Term
	Body    keyspace.Term
	Kind    kind.OutcomeKind
	Target  keyspace.Term
}

// Matches reports exact four-owner provenance after Seal has published the
// one-time structural validation fence.
func Matches(result *Result, sourceID, flowID, staticID, moduleID identity.ContentID) bool {
	return result != nil && result.sealed && sourceID.Available() && flowID.Available() && staticID.Available() && moduleID.Available() &&
		result.sourceID == sourceID && result.flowID == flowID && result.staticID == staticID && result.moduleID == moduleID
}

func (result *Result) available() bool {
	return result != nil && result.sourceID.Available() && result.flowID.Available() &&
		result.staticID.Available() && result.moduleID.Available() && result.outcomes != nil
}

// valid is the O(1) publication fence. Complete structural validation runs
// once in Seal before sealed is published; immutable query paths never rescan
// or rehash the relation.
func (result *Result) valid() bool {
	return result != nil && result.sealed && result.available()
}

// validateResult is the complete fail-closed structural proof performed once
// before publication. It checks every dense row, range, term, inverse, map,
// and semantic context preimage.
func (result *Result) validateResult() bool {
	if result == nil || !result.available() || keyspace.TermFamily(result.entry) != keyspace.FamilyBody || keyspace.TermOrdinal(result.entry) == 0 ||
		len(result.bodies) < 2 || len(result.byBody) != len(result.bodies) ||
		len(result.contexts) != len(result.functions) || len(result.bodyContexts) != len(result.bodies)-1 {
		return false
	}
	outcomeCount := result.outcomes.Count()
	entryOrdinal := keyspace.TermOrdinal(result.entry)
	if uint64(entryOrdinal) >= uint64(len(result.bodies)) || result.bodies[entryOrdinal].body != result.entry {
		return false
	}
	bodyOutcomeCursor := uint32(0)
	for ordinal := uint32(1); ordinal < uint32(len(result.bodies)); ordinal++ {
		row := result.bodies[ordinal]
		start, end, rangeOK := result.outcomes.BodyRange(row.body)
		if keyspace.TermFamily(row.body) != keyspace.FamilyBody || keyspace.TermOrdinal(row.body) != ordinal ||
			!validExistingTerm(row.entry) || !rangeOK || start < 0 || end < start || end > outcomeCount ||
			row.outcomes.start != bodyOutcomeCursor || row.outcomes.start != uint32(start) || row.outcomes.end != uint32(end) || !row.context.Available() {
			return false
		}
		bodyOutcomeCursor = row.outcomes.end
		if ordinal == entryOrdinal {
			if row.function != 0 {
				return false
			}
		} else if row.function != 0 {
			if uint64(row.function) > uint64(len(result.functions)) || keyspace.TermOrdinal(result.functions[row.function-1].body) != ordinal {
				return false
			}
		}
		contextIndex, ok := result.bodyContexts[row.context]
		if !ok || contextIndex != ordinal || hashBodyContext(result, row) != row.context {
			return false
		}
	}
	if bodyOutcomeCursor != uint32(outcomeCount) {
		return false
	}
	formalCursor, captureCursor := uint32(0), uint32(0)
	for index, row := range result.functions {
		if keyspace.TermFamily(row.function) != keyspace.FamilyFunction || keyspace.TermOrdinal(row.function) != uint32(index+1) ||
			keyspace.TermFamily(row.owner) != keyspace.FamilyBody || keyspace.TermOrdinal(row.owner) == 0 ||
			keyspace.TermFamily(row.body) != keyspace.FamilyBody || keyspace.TermOrdinal(row.body) == 0 || !validExistingTerm(row.entry) ||
			(row.vararg != 0 && (keyspace.TermFamily(row.vararg) != keyspace.FamilyCell || keyspace.TermOrdinal(row.vararg) == 0)) ||
			!validRange(row.formals, len(result.formals)) || row.formals.start != formalCursor || !validRange(row.captures, len(result.captures)) || row.captures.start != captureCursor ||
			!validRange(row.outcomes, outcomeCount) || !row.context.Available() {
			return false
		}
		bodyOrdinal := keyspace.TermOrdinal(row.body)
		ownerOrdinal := keyspace.TermOrdinal(row.owner)
		if uint64(ownerOrdinal) >= uint64(len(result.bodies)) || uint64(bodyOrdinal) >= uint64(len(result.bodies)) || result.byBody[bodyOrdinal] != uint32(index+1) {
			return false
		}
		body := result.bodies[bodyOrdinal]
		if body.function != uint32(index+1) || body.entry != row.entry || body.outcomes != row.outcomes {
			return false
		}
		formalCursor, captureCursor = row.formals.end, row.captures.end
		contextIndex, ok := result.contexts[row.context]
		if !ok || contextIndex != uint32(index+1) || hashContext(result, row) != row.context {
			return false
		}
	}
	if formalCursor != uint32(len(result.formals)) || captureCursor != uint32(len(result.captures)) {
		return false
	}
	for _, term := range result.formals {
		if keyspace.TermFamily(term) != keyspace.FamilyCell || keyspace.TermOrdinal(term) == 0 {
			return false
		}
	}
	for _, row := range result.captures {
		if keyspace.TermFamily(row.inner) != keyspace.FamilyCell || keyspace.TermOrdinal(row.inner) == 0 ||
			keyspace.TermFamily(row.outer) != keyspace.FamilyCell || keyspace.TermOrdinal(row.outer) == 0 {
			return false
		}
	}
	for index := 0; index < outcomeCount; index++ {
		exit, ok := result.outcomeAt(uint32(index))
		if !ok || keyspace.TermOrdinal(exit.Outcome) != uint32(index+1) ||
			uint64(keyspace.TermOrdinal(exit.Body)) >= uint64(len(result.bodies)) ||
			exit.Kind < kind.OutcomeNormal || exit.Kind > kind.OutcomeCancel || !validTarget(exit.Kind, exit.Target) ||
			!rangeContains(result.bodies[keyspace.TermOrdinal(exit.Body)].outcomes, uint32(index), outcomeCount) {
			return false
		}
		body := result.bodies[keyspace.TermOrdinal(exit.Body)]
		if body.function != 0 && !rangeContains(result.functions[body.function-1].outcomes, uint32(index), outcomeCount) {
			return false
		}
	}
	for ordinal := uint32(1); ordinal < uint32(len(result.byBody)); ordinal++ {
		functionIndex := result.byBody[ordinal]
		if functionIndex == 0 {
			if result.bodies[ordinal].function != 0 {
				return false
			}
			continue
		}
		if uint64(functionIndex) > uint64(len(result.functions)) || result.functions[functionIndex-1].body != result.bodies[ordinal].body {
			return false
		}
	}
	if result.byBody[entryOrdinal] != 0 || result.bodies[entryOrdinal].function != 0 {
		return false
	}
	for functionIndex := range result.functions {
		if result.byBody[keyspace.TermOrdinal(result.functions[functionIndex].body)] != uint32(functionIndex+1) {
			return false
		}
	}
	for context := range result.contexts {
		if _, collision := result.bodyContexts[context]; collision {
			return false
		}
	}
	return true
}

func (result *Result) outcomeCount() int {
	if result == nil || result.outcomes == nil {
		return 0
	}
	return result.outcomes.Count()
}

func (result *Result) outcomeAt(index uint32) (OutcomeExit, bool) {
	if result == nil || result.outcomes == nil || uint64(index) >= uint64(result.outcomes.Count()) {
		return OutcomeExit{}, false
	}
	term, ok := result.outcomes.At(int(index))
	if !ok || keyspace.TermFamily(term) != keyspace.FamilyOutcome || keyspace.TermOrdinal(term) != index+1 {
		return OutcomeExit{}, false
	}
	body, outcomeKind, target, ok := result.outcomes.Get(term)
	if !ok || keyspace.TermFamily(body) != keyspace.FamilyBody || keyspace.TermOrdinal(body) == 0 {
		return OutcomeExit{}, false
	}
	return OutcomeExit{Outcome: term, Body: body, Kind: outcomeKind, Target: target}, true
}

func validExistingTerm(term keyspace.Term) bool {
	return keyspace.TermFamily(term) > keyspace.FamilyInvalid && keyspace.TermFamily(term) < keyspace.FamilyCount && keyspace.TermOrdinal(term) != 0
}

func rangeContains(value range32, index uint32, length int) bool {
	return validRange(value, length) && index >= value.start && index < value.end
}

func validRange(value range32, length int) bool {
	return value.start <= value.end && uint64(value.end) <= uint64(length)
}

func validTarget(outcomeKind kind.OutcomeKind, target keyspace.Term) bool {
	switch outcomeKind {
	case kind.OutcomeBreak:
		return keyspace.TermFamily(target) == keyspace.FamilyLoop && keyspace.TermOrdinal(target) != 0
	case kind.OutcomeGoto:
		return keyspace.TermFamily(target) == keyspace.FamilyLabel && keyspace.TermOrdinal(target) != 0
	default:
		return target == 0
	}
}
