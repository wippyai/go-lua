// Package directfunction seals occurrence-specific direct Function evidence.
//
// The proof retains only dense Read, Call, and GenericFor Loop projections.
// Function Terms denote themselves; every other accepted value is a lexical
// Read whose terminal Cell has one exact Function installation.  The package
// consumes already sealed structural owners and retains no source, authored,
// containment, binding, or control graph.
package directfunction

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/binding"
	"github.com/wippyai/go-lua/analysis/program/flow/body"
	"github.com/wippyai/go-lua/analysis/program/flow/containment"
	"github.com/wippyai/go-lua/analysis/program/flow/executable"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/flow/sourcecontrol"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

type solver struct {
	source     source.View
	flow       authored.View
	bodies     *body.Result
	bindings   binding.Result
	forest     *containment.Result
	control    *sourcecontrol.Result
	executable *executable.Result
	counts     [keyspace.FamilyCount]uint32

	terminal        []keyspace.Term
	functionForCell []keyspace.Term
	functionOrigin  []keyspace.Term
	cellForFunction []keyspace.Term
	recursiveSelf   []keyspace.Term
}

func (s *solver) populate(result *Result) error {
	reads := s.flow.Storage().Reads()
	for index := 0; index < reads.Count(); index++ {
		read, ok := reads.At(index)
		if !ok {
			return errors.New("program/flow/directfunction: Read view is unavailable")
		}
		owner, _, _, rowOK := reads.Get(read)
		if !rowOK {
			return errors.New("program/flow/directfunction: Read row is unavailable")
		}
		if s.forest.Static(read) || !s.executable.Contains(read) {
			continue
		}
		node, nodeOK := s.coordinate(read)
		if !nodeOK {
			return errors.New("program/flow/directfunction: dynamic Read lacks a coordinate")
		}
		if !s.control.Reachable(node) {
			continue
		}
		function := s.candidate(read, owner, node)
		if function != 0 {
			result.readFunctions[keyspace.TermOrdinal(read)] = function
		}
	}

	calls := s.flow.Calls()
	for index := 0; index < calls.Count(); index++ {
		call, ok := calls.At(index)
		if !ok {
			return errors.New("program/flow/directfunction: Call view is unavailable")
		}
		owner, callee, _, actuals, rowOK := calls.Get(call)
		if !rowOK {
			return errors.New("program/flow/directfunction: Call row is unavailable")
		}
		if !s.executable.Contains(call) {
			continue
		}
		node, nodeOK := s.coordinate(call)
		if !nodeOK {
			return errors.New("program/flow/directfunction: executable Call lacks a coordinate")
		}
		if !s.control.Reachable(node) {
			continue
		}
		if _, _, ok := s.flow.Values().Get(actuals); !ok {
			return errors.New("program/flow/directfunction: Call actual Values row is unavailable")
		}
		function := s.candidate(callee, owner, node)
		if function != 0 {
			result.callFunctions[keyspace.TermOrdinal(call)] = function
		}
	}

	loops := s.flow.Control().Loops()
	values := s.flow.Values()
	for index := 0; index < loops.Count(); index++ {
		loop, ok := loops.At(index)
		if !ok {
			return errors.New("program/flow/directfunction: Loop view is unavailable")
		}
		owner, _, loopKind, control, rowOK := loops.Get(loop)
		if !rowOK || loopKind != kind.LoopGenericFor || !s.executable.Contains(loop) {
			continue
		}
		node, nodeOK := s.coordinate(loop)
		if !nodeOK {
			return errors.New("program/flow/directfunction: executable GenericFor lacks a coordinate")
		}
		if !s.control.Reachable(node) {
			continue
		}
		position, positionOK := values.Position(control, 0)
		if !positionOK || position.Fixed == 0 {
			// A tail-only or NilFill first header has no fixed occurrence and
			// therefore cannot carry a direct Function proof.
			continue
		}
		function := s.candidate(position.Fixed, owner, node)
		if function != 0 {
			result.loopFunctions[keyspace.TermOrdinal(loop)] = function
		}
	}
	return nil
}

func (s *solver) candidate(value, owner keyspace.Term, occurrence uint32) keyspace.Term {
	if s == nil || !validTerm(value, s.counts) || !validBody(owner, s.counts) {
		return 0
	}
	if keyspace.TermFamily(value) == keyspace.FamilyFunction {
		// A Function term is not self-authenticating when it appears as a
		// Call/GenericFor value.  Its authored owner and the containment proof
		// must place that occurrence in the carrier Body being evaluated.  The
		// standalone Result.For(Function) query intentionally has a
		// different contract and remains an identity projection even for dead
		// Functions.
		functionOwner, _, _, functionOK := s.flow.Functions().Get(value)
		if !functionOK || functionOwner != owner || s.forest == nil || s.executable == nil ||
			s.forest.Static(value) || !s.executable.Contains(value) || !s.forest.Contains(owner, value) {
			return 0
		}
		return value
	}
	if keyspace.TermFamily(value) != keyspace.FamilyRead {
		return 0
	}

	reads := s.flow.Storage().Reads()
	readOwner, cellOrLens, _, ok := reads.Get(value)
	if !ok || readOwner != owner || s.forest.Static(value) || !s.executable.Contains(value) {
		return 0
	}
	readNode, readOK := s.liveCoordinate(value)
	if !readOK || !s.control.Reachable(readNode) {
		return 0
	}
	cells := s.flow.Storage().Cells()
	cellKind, cellBody, _, cellOK := cells.Get(cellOrLens)
	if !cellOK || cellKind != authored.CellLocal || !validBody(cellBody, s.counts) ||
		!s.visible(owner, cellOrLens) {
		return 0
	}
	base, baseOK := s.terminalCell(cellOrLens)
	if !baseOK {
		return 0
	}
	baseOrdinal := keyspace.TermOrdinal(base)
	if uint64(baseOrdinal) >= uint64(len(s.functionForCell)) || uint64(baseOrdinal) >= uint64(len(s.functionOrigin)) {
		return 0
	}
	function := s.functionForCell[baseOrdinal]
	if !s.validFunction(function) {
		return 0
	}

	origin := s.functionOrigin[baseOrdinal]
	originNode, originOK := s.liveCoordinate(origin)
	if originOK && originNode != occurrence && s.control.Reachable(originNode) && s.control.Dominates(originNode, occurrence) {
		return function
	}

	// Function Bodies are independent dominance roots.  The only route around
	// strict dominance is a self witness through the exact reverse
	// Function-to-terminal-Cell mapping and a Capture whose outer resolves to
	// that Cell.
	activation, activationOK := s.bodies.Activation(owner)
	functionOrdinal := keyspace.TermOrdinal(function)
	originFamily := keyspace.TermFamily(origin)
	if originFamily == keyspace.FamilyBind {
		// Binding.FunctionCell is the narrow reverse proof for the ordinary
		// local-function Bind route. It must not be required for an Assign,
		// because local f; f = function() f() end has no Function-valued Bind.
		boundCell, boundOK := s.bindings.FunctionCell(function)
		boundBase, terminalOK := s.terminalCell(boundCell)
		if !boundOK || !terminalOK || boundBase != base {
			return 0
		}
	} else if originFamily != keyspace.FamilyAssign {
		return 0
	}
	if uint64(functionOrdinal) >= uint64(len(s.cellForFunction)) || s.cellForFunction[functionOrdinal] != base {
		return 0
	}
	if activationOK && activation == function &&
		keyspace.TermFamily(function) == keyspace.FamilyFunction &&
		functionOrdinal < uint32(len(s.recursiveSelf)) && s.recursiveSelf[functionOrdinal] == base {
		return function
	}
	return 0
}

func (s *solver) terminalCell(cell keyspace.Term) (keyspace.Term, bool) {
	if s == nil || keyspace.TermFamily(cell) != keyspace.FamilyCell {
		return 0, false
	}
	ordinal := keyspace.TermOrdinal(cell)
	if ordinal == 0 || uint64(ordinal) >= uint64(len(s.terminal)) {
		return 0, false
	}
	base := s.terminal[ordinal]
	if keyspace.TermFamily(base) != keyspace.FamilyCell || keyspace.TermOrdinal(base) == 0 {
		return 0, false
	}
	return base, true
}

func (s *solver) visible(owner, cell keyspace.Term) bool {
	if s == nil || !validBody(owner, s.counts) || keyspace.TermFamily(cell) != keyspace.FamilyCell {
		return false
	}
	_, cellBody, _, ok := s.flow.Storage().Cells().Get(cell)
	if !ok || !validBody(cellBody, s.counts) {
		return false
	}
	ownerActivation, ownerOK := s.bodies.Activation(owner)
	cellActivation, cellOK := s.bodies.Activation(cellBody)
	return ownerOK && cellOK && ownerActivation == cellActivation && s.bodies.AncestorOrSelf(cellBody, owner)
}

func (s *solver) liveCoordinate(term keyspace.Term) (uint32, bool) {
	if s == nil || s.control == nil {
		return 0, false
	}
	node, ok := s.control.Coordinate(s.source, term)
	return node, ok && s.control.Reachable(node)
}

func (s *solver) coordinate(term keyspace.Term) (uint32, bool) {
	if s == nil || s.control == nil {
		return 0, false
	}
	return s.control.Coordinate(s.source, term)
}

func (s *solver) validFunction(function keyspace.Term) bool {
	return s != nil && keyspace.TermFamily(function) == keyspace.FamilyFunction &&
		keyspace.TermOrdinal(function) != 0 && keyspace.TermOrdinal(function) <= s.counts[keyspace.FamilyFunction]
}
