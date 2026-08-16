package functionboundary

import "github.com/wippyai/go-lua/program/keyspace"

// Count reports the number of existing Function rows in the relation.
func (result *Result) Count() int {
	if !result.valid() {
		return 0
	}
	return len(result.functions)
}

// At returns one canonical FunctionBoundary by dense Function ordinal.
func (result *Result) At(index int) (Boundary, bool) {
	if !result.valid() || index < 0 || index >= len(result.functions) {
		return Boundary{}, false
	}
	boundary := Boundary{result: result, index: uint32(index)}
	return boundary, boundary.available()
}

// For resolves an existing Function without scanning child rows.
func (result *Result) For(function keyspace.Term) (Boundary, bool) {
	if !result.valid() || keyspace.TermFamily(function) != keyspace.FamilyFunction || keyspace.TermOrdinal(function) == 0 {
		return Boundary{}, false
	}
	index := keyspace.TermOrdinal(function) - 1
	if uint64(index) >= uint64(len(result.functions)) || result.functions[index].function != function {
		return Boundary{}, false
	}
	return result.At(int(index))
}

// ForFunctionBody resolves the Function owning an existing Function Body.
func (result *Result) ForFunctionBody(body keyspace.Term) (Boundary, bool) {
	if !result.valid() || keyspace.TermFamily(body) != keyspace.FamilyBody || keyspace.TermOrdinal(body) == 0 ||
		uint64(keyspace.TermOrdinal(body)) >= uint64(len(result.byBody)) {
		return Boundary{}, false
	}
	index := result.byBody[keyspace.TermOrdinal(body)]
	if index == 0 || uint64(index) > uint64(len(result.functions)) {
		return Boundary{}, false
	}
	return result.At(int(index - 1))
}

// ForFunctionOutcome resolves the Function owning an existing Outcome.
func (result *Result) ForFunctionOutcome(outcome keyspace.Term) (Boundary, bool) {
	if !result.valid() || keyspace.TermFamily(outcome) != keyspace.FamilyOutcome || keyspace.TermOrdinal(outcome) == 0 ||
		uint64(keyspace.TermOrdinal(outcome)) >= uint64(len(result.byOutcome)) {
		return Boundary{}, false
	}
	index := result.byOutcome[keyspace.TermOrdinal(outcome)]
	if index == 0 || uint64(index) > uint64(len(result.functions)) {
		return Boundary{}, false
	}
	row := result.functions[index-1]
	poolIndex := result.outcomeAt[keyspace.TermOrdinal(outcome)]
	if poolIndex == 0 || !rangeContains(row.outcomes, poolIndex-1, len(result.outcomes)) ||
		result.outcomes[poolIndex-1].term != outcome {
		return Boundary{}, false
	}
	return result.At(int(index - 1))
}

// ForBody resolves any existing Body, including the root/chunk Body, without
// scanning Function rows. It never fabricates a Function for the root.
func (result *Result) ForBody(body keyspace.Term) (BodyBoundary, bool) {
	if !result.valid() || keyspace.TermFamily(body) != keyspace.FamilyBody || keyspace.TermOrdinal(body) == 0 ||
		uint64(keyspace.TermOrdinal(body)) >= uint64(len(result.bodies)) {
		return BodyBoundary{}, false
	}
	boundary := BodyBoundary{result: result, index: keyspace.TermOrdinal(body)}
	return boundary, boundary.available()
}

// ForOutcome resolves the Body that owns any existing Outcome, including a
// top-level Outcome. The returned BodyBoundary is the only owner relation.
func (result *Result) ForOutcome(outcome keyspace.Term) (BodyBoundary, bool) {
	if !result.valid() || keyspace.TermFamily(outcome) != keyspace.FamilyOutcome || keyspace.TermOrdinal(outcome) == 0 ||
		uint64(keyspace.TermOrdinal(outcome)) >= uint64(len(result.bodyByOutcome)) {
		return BodyBoundary{}, false
	}
	bodyOrdinal := result.bodyByOutcome[keyspace.TermOrdinal(outcome)]
	if bodyOrdinal == 0 || uint64(bodyOrdinal) >= uint64(len(result.bodies)) {
		return BodyBoundary{}, false
	}
	row := result.bodies[bodyOrdinal]
	poolIndex := result.outcomeAt[keyspace.TermOrdinal(outcome)]
	if poolIndex == 0 || !rangeContains(row.outcomes, poolIndex-1, len(result.outcomes)) ||
		result.outcomes[poolIndex-1].term != outcome {
		return BodyBoundary{}, false
	}
	boundary := BodyBoundary{result: result, index: bodyOrdinal}
	return boundary, boundary.available()
}

// Root returns the explicit assembly-entry Body boundary.
func (result *Result) Root() (RootBoundary, bool) {
	if !result.valid() {
		return RootBoundary{}, false
	}
	boundary := RootBoundary{result: result, index: keyspace.TermOrdinal(result.entry)}
	return boundary, boundary.available()
}

// ResolveContextID performs exact-quartet-fenced Function lookup through the
// immutable map built during sealing.
func (result *Result) ResolveContextID(id keyspace.ContentID) (Boundary, bool) {
	if !result.valid() || !id.Available() {
		return Boundary{}, false
	}
	index := result.contexts[id]
	if index == 0 || uint64(index) > uint64(len(result.functions)) {
		return Boundary{}, false
	}
	row := result.functions[index-1]
	if row.context != id {
		return Boundary{}, false
	}
	return result.At(int(index - 1))
}

// ResolveBodyContextID performs exact-quartet-fenced Body lookup through the
// immutable map built during sealing.
func (result *Result) ResolveBodyContextID(id keyspace.ContentID) (BodyBoundary, bool) {
	if !result.valid() || !id.Available() {
		return BodyBoundary{}, false
	}
	index := result.bodyContexts[id]
	if index == 0 || uint64(index) >= uint64(len(result.bodies)) {
		return BodyBoundary{}, false
	}
	row := result.bodies[index]
	if row.context != id {
		return BodyBoundary{}, false
	}
	boundary := BodyBoundary{result: result, index: index}
	return boundary, boundary.available()
}

func (boundary Boundary) available() bool {
	if boundary.result == nil || !boundary.result.valid() || uint64(boundary.index) >= uint64(len(boundary.result.functions)) {
		return false
	}
	row := boundary.result.functions[boundary.index]
	return functionRowAvailable(boundary.result, boundary.index, row)
}

func functionRowAvailable(result *Result, index uint32, row functionRow) bool {
	if keyspace.TermFamily(row.function) != keyspace.FamilyFunction || keyspace.TermOrdinal(row.function) != index+1 ||
		keyspace.TermFamily(row.owner) != keyspace.FamilyBody || keyspace.TermOrdinal(row.owner) == 0 ||
		keyspace.TermFamily(row.body) != keyspace.FamilyBody || keyspace.TermOrdinal(row.body) == 0 || row.entry == 0 ||
		(row.vararg != 0 && (keyspace.TermFamily(row.vararg) != keyspace.FamilyCell || keyspace.TermOrdinal(row.vararg) == 0)) ||
		!validRange(row.formals, len(result.formals)) || !validRange(row.captures, len(result.captures)) ||
		!validRange(row.outcomes, len(result.outcomes)) || !row.context.Available() {
		return false
	}
	bodyOrdinal := keyspace.TermOrdinal(row.body)
	return uint64(bodyOrdinal) < uint64(len(result.bodies)) && result.byBody[bodyOrdinal] == index+1 &&
		result.bodies[bodyOrdinal].function == index+1
}

func (boundary BodyBoundary) available() bool {
	if boundary.result == nil || !boundary.result.valid() || boundary.index == 0 || uint64(boundary.index) >= uint64(len(boundary.result.bodies)) {
		return false
	}
	row := boundary.result.bodies[boundary.index]
	return bodyRowAvailable(boundary.result, boundary.index, row)
}

func bodyRowAvailable(result *Result, index uint32, row bodyRow) bool {
	return keyspace.TermFamily(row.body) == keyspace.FamilyBody && keyspace.TermOrdinal(row.body) == index && row.entry != 0 &&
		validRange(row.outcomes, len(result.outcomes)) && row.context.Available()
}

func (boundary RootBoundary) available() bool {
	if boundary.result == nil || !boundary.result.valid() || boundary.index == 0 {
		return false
	}
	body := BodyBoundary{result: boundary.result, index: boundary.index}
	return boundary.index == keyspace.TermOrdinal(boundary.result.entry) && body.available()
}

// Available reports whether this Function handle belongs to its exact sealed quartet.
func (boundary Boundary) Available() bool { return boundary.available() }

// Equal compares Function contextual identity and exact owner provenance.
func (boundary Boundary) Equal(other Boundary) bool {
	if !boundary.available() || !other.available() {
		return false
	}
	left, right := boundary.result.functions[boundary.index], other.result.functions[other.index]
	return left.context == right.context && boundary.result.sourceID == other.result.sourceID &&
		boundary.result.flowID == other.result.flowID && boundary.result.staticID == other.result.staticID &&
		boundary.result.moduleID == other.result.moduleID
}

// ContextID returns the replay-stable exact-quartet-fenced Function identity.
func (boundary Boundary) ContextID() keyspace.ContentID {
	if !boundary.available() {
		return keyspace.ContentID{}
	}
	return boundary.result.functions[boundary.index].context
}

func (boundary Boundary) row() (functionRow, bool) {
	if !boundary.available() {
		return functionRow{}, false
	}
	return boundary.result.functions[boundary.index], true
}

func (boundary Boundary) Function() (keyspace.Term, bool) {
	row, ok := boundary.row()
	return row.function, ok
}
func (boundary Boundary) Owner() (keyspace.Term, bool) {
	row, ok := boundary.row()
	return row.owner, ok
}
func (boundary Boundary) Body() (keyspace.Term, bool) {
	row, ok := boundary.row()
	return row.body, ok
}
func (boundary Boundary) Entry() (keyspace.Term, bool) {
	row, ok := boundary.row()
	return row.entry, ok
}

// Vararg returns the existing optional vararg Cell. A function without one
// returns false; no synthetic Cell is created.
func (boundary Boundary) Vararg() (keyspace.Term, bool) {
	row, ok := boundary.row()
	return row.vararg, ok && row.vararg != 0
}
func (boundary Boundary) FormalCount() int {
	row, ok := boundary.row()
	if !ok {
		return 0
	}
	return int(row.formals.end - row.formals.start)
}
func (boundary Boundary) FormalAt(index int) (keyspace.Term, bool) {
	row, ok := boundary.row()
	if !ok || index < 0 || uint64(index) >= uint64(row.formals.end-row.formals.start) {
		return 0, false
	}
	return boundary.result.formals[row.formals.start+uint32(index)], true
}
func (boundary Boundary) CaptureCount() int {
	row, ok := boundary.row()
	if !ok {
		return 0
	}
	return int(row.captures.end - row.captures.start)
}
func (boundary Boundary) CaptureAt(index int) (Capture, bool) {
	row, ok := boundary.row()
	if !ok || index < 0 || uint64(index) >= uint64(row.captures.end-row.captures.start) {
		return Capture{}, false
	}
	pair := boundary.result.captures[row.captures.start+uint32(index)]
	return Capture{Inner: pair.inner, Outer: pair.outer, InnerBody: pair.innerBody, OuterBody: pair.outerBody}, true
}
func (boundary Boundary) OutcomeCount() int {
	row, ok := boundary.row()
	if !ok {
		return 0
	}
	return int(row.outcomes.end - row.outcomes.start)
}
func (boundary Boundary) OutcomeAt(index int) (OutcomeExit, bool) {
	row, ok := boundary.row()
	if !ok || index < 0 || uint64(index) >= uint64(row.outcomes.end-row.outcomes.start) {
		return OutcomeExit{}, false
	}
	exit := boundary.result.outcomes[row.outcomes.start+uint32(index)]
	return OutcomeExit{Outcome: exit.term, Body: exit.body, Kind: exit.kind, Target: exit.target}, true
}

// Available reports whether this Body handle belongs to its exact quartet.
func (boundary BodyBoundary) Available() bool { return boundary.available() }

// OwnsBody is the exact hot-owner fence for a Body handle. Equal intentionally
// admits equivalent artifact replay; owner-local projections must use this
// predicate before indexing their own dense rows.
func (result *Result) OwnsBody(boundary BodyBoundary) bool {
	return result != nil && result.valid() && boundary.available() && boundary.result == result
}

// OwnsFunction is the exact hot-owner fence for a Function boundary. Equal
// permits equivalent artifact replay; owner-local consumers must use this
// predicate before consuming formal/vararg/capture rows.
func (result *Result) OwnsFunction(boundary Boundary) bool {
	return result != nil && result.valid() && boundary.available() && boundary.result == result
}

func (boundary BodyBoundary) Equal(other BodyBoundary) bool {
	if !boundary.available() || !other.available() {
		return false
	}
	left, right := boundary.result.bodies[boundary.index], other.result.bodies[other.index]
	return left.context == right.context && boundary.result.sourceID == other.result.sourceID &&
		boundary.result.flowID == other.result.flowID && boundary.result.staticID == other.result.staticID &&
		boundary.result.moduleID == other.result.moduleID
}
func (boundary BodyBoundary) ContextID() keyspace.ContentID {
	if !boundary.available() {
		return keyspace.ContentID{}
	}
	return boundary.result.bodies[boundary.index].context
}
func (boundary BodyBoundary) Body() (keyspace.Term, bool) {
	if !boundary.available() {
		return 0, false
	}
	return boundary.result.bodies[boundary.index].body, true
}
func (boundary BodyBoundary) Entry() (keyspace.Term, bool) {
	if !boundary.available() {
		return 0, false
	}
	return boundary.result.bodies[boundary.index].entry, true
}
func (boundary BodyBoundary) OutcomeCount() int {
	if !boundary.available() {
		return 0
	}
	row := boundary.result.bodies[boundary.index]
	return int(row.outcomes.end - row.outcomes.start)
}
func (boundary BodyBoundary) OutcomeAt(index int) (OutcomeExit, bool) {
	if !boundary.available() || index < 0 {
		return OutcomeExit{}, false
	}
	row := boundary.result.bodies[boundary.index]
	if uint64(index) >= uint64(row.outcomes.end-row.outcomes.start) {
		return OutcomeExit{}, false
	}
	exit := boundary.result.outcomes[row.outcomes.start+uint32(index)]
	return OutcomeExit{Outcome: exit.term, Body: exit.body, Kind: exit.kind, Target: exit.target}, true
}

// OutcomeForTerm resolves one exact member of this Body's already-sealed
// ordered Outcome range. The returned ordinal is relative to this Body and is
// derived from Result's sole dense inverse; no range scan is performed.
func (boundary BodyBoundary) OutcomeForTerm(outcome keyspace.Term) (OutcomeExit, int, bool) {
	if !boundary.available() || keyspace.TermFamily(outcome) != keyspace.FamilyOutcome || keyspace.TermOrdinal(outcome) == 0 ||
		uint64(keyspace.TermOrdinal(outcome)) >= uint64(len(boundary.result.outcomeAt)) {
		return OutcomeExit{}, 0, false
	}
	row := boundary.result.bodies[boundary.index]
	poolIndex := boundary.result.outcomeAt[keyspace.TermOrdinal(outcome)]
	if poolIndex == 0 || !rangeContains(row.outcomes, poolIndex-1, len(boundary.result.outcomes)) {
		return OutcomeExit{}, 0, false
	}
	exit := boundary.result.outcomes[poolIndex-1]
	if exit.term != outcome || exit.body != row.body {
		return OutcomeExit{}, 0, false
	}
	ordinal := int(poolIndex - 1 - row.outcomes.start)
	return OutcomeExit{Outcome: exit.term, Body: exit.body, Kind: exit.kind, Target: exit.target}, ordinal, true
}

// Available reports whether this Root handle is the exact assembly entry.
func (boundary RootBoundary) Available() bool { return boundary.available() }
func (boundary RootBoundary) Equal(other RootBoundary) bool {
	if !boundary.available() || !other.available() {
		return false
	}
	return BodyBoundary{result: boundary.result, index: boundary.index}.Equal(BodyBoundary{result: other.result, index: other.index})
}
func (boundary RootBoundary) ContextID() keyspace.ContentID {
	if !boundary.available() {
		return keyspace.ContentID{}
	}
	return boundary.result.bodies[boundary.index].context
}
func (boundary RootBoundary) Body() (keyspace.Term, bool) {
	return BodyBoundary{result: boundary.result, index: boundary.index}.Body()
}
func (boundary RootBoundary) Entry() (keyspace.Term, bool) {
	return BodyBoundary{result: boundary.result, index: boundary.index}.Entry()
}
func (boundary RootBoundary) OutcomeCount() int {
	return BodyBoundary{result: boundary.result, index: boundary.index}.OutcomeCount()
}
func (boundary RootBoundary) OutcomeAt(index int) (OutcomeExit, bool) {
	return BodyBoundary{result: boundary.result, index: boundary.index}.OutcomeAt(index)
}
