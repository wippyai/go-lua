package directfunction

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/binding"
	"github.com/wippyai/go-lua/analysis/program/flow/body"
	"github.com/wippyai/go-lua/analysis/program/flow/containment"
	"github.com/wippyai/go-lua/analysis/program/flow/executable"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	flowrole "github.com/wippyai/go-lua/analysis/program/flow/role"
	"github.com/wippyai/go-lua/analysis/program/flow/sourcecontrol"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

func validateOwners(
	sourceView source.View,
	flow authored.View,
	staticID identity.ContentID,
	moduleID identity.ContentID,
	bodies *body.Result,
	bindings binding.Result,
	forest *containment.Result,
	control *sourcecontrol.Result,
	executableResult *executable.Result,
) ([keyspace.FamilyCount]uint32, error) {
	var counts [keyspace.FamilyCount]uint32
	identity := sourceView.Identity()
	sourceID := identity.ContentID()
	flowID := flow.Cold().ContentID()
	if !identity.ContentID().Available() || identity.Name() == "" || identity.TermCount() == 0 ||
		!flowID.Available() || !staticID.Available() || !moduleID.Available() || bodies == nil || forest == nil || control == nil || executableResult == nil {
		return counts, errors.New("program/flow/directfunction: one or more owners are unavailable")
	}
	if !body.Matches(bodies, sourceID, flowID) ||
		!binding.Matches(&bindings, sourceID, flowID) ||
		!executable.Matches(executableResult, sourceID, flowID, staticID, moduleID) ||
		!containment.Matches(forest, sourceID, flowID, staticID, moduleID) ||
		!sourcecontrol.Matches(control, sourceID, flowID, staticID, moduleID) {
		return counts, errors.New("program/flow/directfunction: owner provenance disagrees with Source, Flow, Static, or Module")
	}
	var total uint64
	var outcomes uint64
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		count := identity.FamilyCount(family)
		if count < 0 || !keyspace.TermOrdinalFits(count) {
			return counts, errors.New("program/flow/directfunction: invalid Source family denominator")
		}
		if family == keyspace.FamilyOutcome {
			outcomes = uint64(count)
			continue
		}
		counts[family] = uint32(count)
		total += uint64(count)
	}
	if total+outcomes != uint64(identity.TermCount()) || uint64(forest.Count()) != total ||
		bodies.BodyCount() != int(counts[keyspace.FamilyBody]) || counts[keyspace.FamilyBody] == 0 ||
		bindings.CellCount() != int(counts[keyspace.FamilyCell]) || control.NodeCount() == 0 {
		return counts, errors.New("program/flow/directfunction: structural owner denominator mismatch")
	}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		if executableResult.FamilyCount(family) != int(counts[family]) {
			return counts, errors.New("program/flow/directfunction: executable family denominator mismatch")
		}
	}
	storage := flow.Storage()
	access := flow.Access()
	controlView := flow.Control()
	operators := flow.Operators()
	checks := [...]struct {
		actual int
		family keyspace.Family
	}{
		{flow.Values().Count(), keyspace.FamilyValues},
		{access.Exact().Count(), keyspace.FamilyLensExact},
		{access.Dynamic().Count(), keyspace.FamilyLensKey},
		{storage.Cells().Count(), keyspace.FamilyCell},
		{storage.Reads().Count(), keyspace.FamilyRead},
		{storage.Varargs().Count(), keyspace.FamilyVararg},
		{storage.Binds().Count(), keyspace.FamilyBind},
		{storage.Assigns().Count(), keyspace.FamilyAssign},
		{storage.Writes().Count(), keyspace.FamilyWrite},
		{flow.Tables().Count(), keyspace.FamilyTable},
		{flow.Fields().Count(), keyspace.FamilyTableField},
		{operators.Unaries().Count(), keyspace.FamilyUnary},
		{operators.Binaries().Count(), keyspace.FamilyBinary},
		{operators.Selects().Count(), keyspace.FamilySelect},
		{flow.Functions().Count(), keyspace.FamilyFunction},
		{flow.Calls().Count(), keyspace.FamilyCall},
		{controlView.Returns().Count(), keyspace.FamilyReturn},
		{controlView.Breaks().Count(), keyspace.FamilyBreak},
		{controlView.Labels().Count(), keyspace.FamilyLabel},
		{controlView.Gotos().Count(), keyspace.FamilyGoto},
		{controlView.Branches().Count(), keyspace.FamilyBranch},
		{controlView.Loops().Count(), keyspace.FamilyLoop},
		{flow.Claims().Count(), keyspace.FamilyValueClaim},
		{flow.TypeValues().Count(), keyspace.FamilyTypeValue},
	}
	for _, check := range checks {
		if check.actual != int(counts[check.family]) {
			return counts, errors.New("program/flow/directfunction: authored family denominator mismatch")
		}
	}
	return counts, nil
}

func validateFunctions(
	flow authored.View,
	bodies *body.Result,
	bindings binding.Result,
	counts [keyspace.FamilyCount]uint32,
	captureOuter []keyspace.Term,
) error {
	if uint64(len(captureOuter)) != uint64(counts[keyspace.FamilyCell])+1 {
		return errors.New("program/flow/directfunction: Capture Cell denominator mismatch")
	}
	functions := flow.Functions()
	cells := flow.Storage().Cells()
	for index := 0; index < functions.Count(); index++ {
		function, ok := functions.At(index)
		if !ok || function != keyspace.MakeTerm(keyspace.FamilyFunction, uint32(index+1)) {
			return errors.New("program/flow/directfunction: Function ordinal is not dense")
		}
		owner, functionBody, vararg, rowOK := functions.Get(function)
		if !rowOK || !validBody(owner, counts) || !validBody(functionBody, counts) || owner == functionBody {
			return errors.New("program/flow/directfunction: malformed Function Body authority")
		}
		parent, parentOK := bodies.Parent(functionBody)
		activation, activationOK := bodies.Activation(functionBody)
		if !parentOK || parent != owner || !activationOK || activation != function {
			return errors.New("program/flow/directfunction: Function Body topology disagrees")
		}
		if vararg != 0 {
			cellKind, cellBody, key, cellOK := cells.Get(vararg)
			role, roleOK := bindings.Role(vararg)
			host, hostOK := bindings.Host(vararg)
			if !cellOK || cellKind != authored.CellLocal || key != 0 || cellBody != functionBody ||
				!roleOK || role != kind.CellFunctionVararg || !hostOK || host != function {
				return errors.New("program/flow/directfunction: Function Vararg binding disagrees")
			}
		}
		captureCount, countOK := functions.CaptureCount(function)
		if !countOK || captureCount < 0 {
			return errors.New("program/flow/directfunction: Function capture range is unavailable")
		}
		for at := 0; at < captureCount; at++ {
			inner, outer, captureOK := functions.CaptureAt(function, at)
			if !captureOK || !validCell(inner, counts) || !validCell(outer, counts) || inner == outer {
				return errors.New("program/flow/directfunction: malformed Capture")
			}
			innerKind, innerBody, innerKey, innerRowOK := cells.Get(inner)
			outerKind, outerBody, outerKey, outerRowOK := cells.Get(outer)
			innerRole, innerRoleOK := bindings.Role(inner)
			innerHost, innerHostOK := bindings.Host(inner)
			if !innerRowOK || innerKind != authored.CellLocal || innerKey != 0 || innerBody != functionBody ||
				!innerRoleOK || innerRole != kind.CellCapture || !innerHostOK || innerHost != function ||
				!outerRowOK || outerKind != authored.CellLocal || outerKey != 0 || !validBody(outerBody, counts) ||
				outerBody == functionBody ||
				!visibleBody(bodies, owner, outerBody) {
				return errors.New("program/flow/directfunction: Capture visibility or role disagrees")
			}
			innerOrdinal := keyspace.TermOrdinal(inner)
			if captureOuter[innerOrdinal] != 0 {
				return errors.New("program/flow/directfunction: Capture Inner has multiple outers")
			}
			captureOuter[innerOrdinal] = outer
		}
	}
	return nil
}

func validateCells(flow authored.View, bindings binding.Result, counts [keyspace.FamilyCount]uint32) error {
	cells := flow.Storage().Cells()
	for index := 0; index < cells.Count(); index++ {
		cell, ok := cells.At(index)
		if !ok || cell != keyspace.MakeTerm(keyspace.FamilyCell, uint32(index+1)) {
			return errors.New("program/flow/directfunction: Cell ordinal is not dense")
		}
		cellKind, body, key, rowOK := cells.Get(cell)
		role, roleOK := bindings.Role(cell)
		host, hostOK := bindings.Host(cell)
		if !rowOK || !roleOK || !hostOK {
			return errors.New("program/flow/directfunction: Cell binding is unavailable")
		}
		if cellKind == authored.CellGlobal {
			if body != 0 || key == 0 || role != kind.CellGlobal || host != 0 {
				return errors.New("program/flow/directfunction: malformed global Cell binding")
			}
			continue
		}
		if cellKind != authored.CellLocal || !validBody(body, counts) || key != 0 || role == kind.CellGlobal || host == 0 {
			return errors.New("program/flow/directfunction: malformed lexical Cell binding")
		}
		var hostFamily keyspace.Family
		switch role {
		case kind.CellLocal:
			hostFamily = keyspace.FamilyBind
		case kind.CellFormal, kind.CellFunctionVararg, kind.CellCapture:
			hostFamily = keyspace.FamilyFunction
		case kind.CellLoop:
			hostFamily = keyspace.FamilyLoop
		case kind.CellChunkVararg:
			hostFamily = keyspace.FamilyBody
		default:
			return errors.New("program/flow/directfunction: unknown lexical Cell role")
		}
		if keyspace.TermFamily(host) != hostFamily || keyspace.TermOrdinal(host) == 0 || keyspace.TermOrdinal(host) > counts[hostFamily] {
			return errors.New("program/flow/directfunction: lexical Cell host has the wrong family")
		}
	}
	return nil
}

func validateOccurrences(
	flow authored.View,
	bodies *body.Result,
	bindings binding.Result,
	forest *containment.Result,
	counts [keyspace.FamilyCount]uint32,
) error {
	storage := flow.Storage()
	cells := storage.Cells()
	reads := storage.Reads()
	for index := 0; index < reads.Count(); index++ {
		read, ok := reads.At(index)
		if !ok || read != keyspace.MakeTerm(keyspace.FamilyRead, uint32(index+1)) {
			return errors.New("program/flow/directfunction: Read ordinal is not dense")
		}
		owner, sourceTerm, _, rowOK := reads.Get(read)
		if !rowOK || !validBody(owner, counts) || !validTerm(sourceTerm, counts) || !forest.Contains(read, read) {
			return errors.New("program/flow/directfunction: malformed Read occurrence")
		}
		switch keyspace.TermFamily(sourceTerm) {
		case keyspace.FamilyCell:
			cellKind, cellBody, key, cellOK := cells.Get(sourceTerm)
			role, roleOK := bindings.Role(sourceTerm)
			if !cellOK || !roleOK {
				return errors.New("program/flow/directfunction: Read Cell binding is unavailable")
			}
			if cellKind == authored.CellGlobal {
				if cellBody != 0 || key == 0 || role != kind.CellGlobal {
					return errors.New("program/flow/directfunction: malformed global Read Cell")
				}
			} else if cellKind != authored.CellLocal || !validBody(cellBody, counts) || key != 0 || role == kind.CellGlobal || !visibleBody(bodies, owner, cellBody) {
				return errors.New("program/flow/directfunction: Read Cell is not visible")
			}
		case keyspace.FamilyLensExact:
			lensOwner, _, _, _, lensOK := flow.Access().Exact().Get(sourceTerm)
			if !lensOK || lensOwner != owner {
				return errors.New("program/flow/directfunction: Read Exact Lens owner disagrees")
			}
		case keyspace.FamilyLensKey:
			lensOwner, _, _, lensOK := flow.Access().Dynamic().Get(sourceTerm)
			if !lensOK || lensOwner != owner {
				return errors.New("program/flow/directfunction: Read Dynamic Lens owner disagrees")
			}
		default:
			return errors.New("program/flow/directfunction: Read source is not lexical or lens")
		}
	}

	calls := flow.Calls()
	for index := 0; index < calls.Count(); index++ {
		call, ok := calls.At(index)
		if !ok || call != keyspace.MakeTerm(keyspace.FamilyCall, uint32(index+1)) {
			return errors.New("program/flow/directfunction: Call ordinal is not dense")
		}
		owner, callee, _, actuals, rowOK := calls.Get(call)
		if !rowOK || !validBody(owner, counts) || !flowrole.ValueOccurrence(counts, callee) || !validFamilyTerm(actuals, counts, keyspace.FamilyValues) || !forest.Contains(call, call) {
			return errors.New("program/flow/directfunction: malformed Call occurrence")
		}
	}

	loops := flow.Control().Loops()
	values := flow.Values()
	for index := 0; index < loops.Count(); index++ {
		loop, ok := loops.At(index)
		if !ok || loop != keyspace.MakeTerm(keyspace.FamilyLoop, uint32(index+1)) {
			return errors.New("program/flow/directfunction: Loop ordinal is not dense")
		}
		owner, child, loopKind, control, rowOK := loops.Get(loop)
		parent, parentOK := bodies.Parent(child)
		if !rowOK || !validBody(owner, counts) || !validBody(child, counts) || !parentOK || parent != owner ||
			(loopKind < kind.LoopWhile || loopKind > kind.LoopGenericFor) || !validTerm(control, counts) ||
			!validLoopControl(values, owner, control, loopKind, counts) || !forest.Contains(loop, loop) {
			return errors.New("program/flow/directfunction: malformed Loop occurrence")
		}
		cellCount, cellCountOK := loops.CellCount(loop)
		if !cellCountOK || cellCount < 0 {
			return errors.New("program/flow/directfunction: Loop Cell range is unavailable")
		}
		for cellIndex := 0; cellIndex < cellCount; cellIndex++ {
			cell, cellOK := loops.CellAt(loop, cellIndex)
			cellKind, cellBody, key, rowCellOK := cells.Get(cell)
			role, roleOK := bindings.Role(cell)
			host, hostOK := bindings.Host(cell)
			if !cellOK || !rowCellOK || cellKind != authored.CellLocal || cellBody != child || key != 0 ||
				!roleOK || role != kind.CellLoop || !hostOK || host != loop {
				return errors.New("program/flow/directfunction: Loop Cell binding disagrees")
			}
		}
	}
	return nil
}

// validLoopControl applies the loop-kind family law without asking Values for
// scalar While/Repeat controls. Numeric/Generic controls are Values rows and
// must retain the loop owner's exact Body; the row query is deliberately
// confined to those two kinds.
func validLoopControl(values authored.Values, owner, control keyspace.Term, loopKind kind.LoopKind, counts [keyspace.FamilyCount]uint32) bool {
	if !flowrole.LoopControlFamily(counts, control, loopKind) {
		return false
	}
	switch loopKind {
	case kind.LoopWhile, kind.LoopRepeat:
		return true
	case kind.LoopNumericFor, kind.LoopGenericFor:
		valuesOwner, _, ok := values.Get(control)
		return ok && valuesOwner == owner
	default:
		return false
	}
}

func visibleCell(flow authored.View, bodies *body.Result, owner, cell keyspace.Term, counts [keyspace.FamilyCount]uint32) bool {
	if keyspace.TermFamily(cell) != keyspace.FamilyCell || !validBody(owner, counts) {
		return false
	}
	_, cellBody, _, ok := flow.Storage().Cells().Get(cell)
	return ok && visibleBody(bodies, owner, cellBody)
}

func visibleBody(bodies *body.Result, owner, cellBody keyspace.Term) bool {
	if bodies == nil || keyspace.TermFamily(owner) != keyspace.FamilyBody || keyspace.TermFamily(cellBody) != keyspace.FamilyBody {
		return false
	}
	ownerActivation, ownerOK := bodies.Activation(owner)
	cellActivation, cellOK := bodies.Activation(cellBody)
	return ownerOK && cellOK && ownerActivation == cellActivation && bodies.AncestorOrSelf(cellBody, owner)
}

func validTerm(term keyspace.Term, counts [keyspace.FamilyCount]uint32) bool {
	family, ordinal := keyspace.TermFamily(term), keyspace.TermOrdinal(term)
	return family > keyspace.FamilyInvalid && family < keyspace.FamilyCount && ordinal != 0 && ordinal <= counts[family]
}

func validFamilyTerm(term keyspace.Term, counts [keyspace.FamilyCount]uint32, family keyspace.Family) bool {
	return keyspace.TermFamily(term) == family && keyspace.TermOrdinal(term) != 0 && keyspace.TermOrdinal(term) <= counts[family]
}

func validBody(term keyspace.Term, counts [keyspace.FamilyCount]uint32) bool {
	return keyspace.TermFamily(term) == keyspace.FamilyBody && keyspace.TermOrdinal(term) != 0 && keyspace.TermOrdinal(term) <= counts[keyspace.FamilyBody]
}

func validCell(term keyspace.Term, counts [keyspace.FamilyCount]uint32) bool {
	return keyspace.TermFamily(term) == keyspace.FamilyCell && keyspace.TermOrdinal(term) != 0 && keyspace.TermOrdinal(term) <= counts[keyspace.FamilyCell]
}
