package containment

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	flowrole "github.com/wippyai/go-lua/analysis/program/flow/role"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// emitFlowExecution emits only the execution-owned Write -> Assign edges in
// the canonical containment relation. It deliberately does not attach a row
// to its lexical Body, attach construct Bodies to their declaring constructs,
// or resolve any reference (Cell, receiver, capture, or Label). Call,
// Branch, and Loop expression edges belong to the expression lane, which has
// the source context needed to prove their lexical ownership.
//
// The pass is also the final fail-closed check for the authored execution
// rows.  A row is validated even when it contributes no edge: an absent or
// malformed row must never disappear merely because a later lane happens not
// to observe it.
func emitFlowExecution(view authored.View, counts [keyspace.FamilyCount]uint32, result *emission) error {
	if result == nil {
		return errors.New("program/flow/containment: nil execution emission")
	}
	if !view.Cold().ContentID().Available() {
		return errors.New("program/flow/containment: authored Flow expired")
	}
	if err := flowExecutionCounts(view, counts); err != nil {
		return err
	}

	storage := view.Storage()
	cells := storage.Cells()
	if err := flowExecutionCells(cells, counts); err != nil {
		return err
	}
	if err := flowExecutionReads(storage.Reads(), cells, counts); err != nil {
		return err
	}
	if err := flowExecutionVarargs(storage.Varargs(), cells, counts); err != nil {
		return err
	}
	if err := flowExecutionBinds(storage.Binds(), counts); err != nil {
		return err
	}

	if err := flowExecutionAssigns(storage, counts, &result.edges); err != nil {
		return err
	}

	functions := view.Functions()
	if err := flowExecutionFunctions(functions, cells, counts); err != nil {
		return err
	}
	if err := flowExecutionCalls(view.Calls(), counts); err != nil {
		return err
	}
	if err := flowExecutionControl(view.Control(), cells, counts); err != nil {
		return err
	}
	return nil
}

func flowExecutionCounts(view authored.View, counts [keyspace.FamilyCount]uint32) error {
	if counts[keyspace.FamilyInvalid] != 0 || counts[keyspace.FamilyOutcome] != 0 {
		return errors.New("program/flow/containment: invalid execution denominator")
	}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		if counts[family] > keyspace.MaxTermOrdinal {
			return errors.New("program/flow/containment: execution family cardinality overflow")
		}
	}
	checks := [...]struct {
		family keyspace.Family
		count  int
	}{
		{keyspace.FamilyValues, view.Values().Count()},
		{keyspace.FamilyLensExact, view.Access().Exact().Count()},
		{keyspace.FamilyLensKey, view.Access().Dynamic().Count()},
		{keyspace.FamilyCell, view.Storage().Cells().Count()},
		{keyspace.FamilyRead, view.Storage().Reads().Count()},
		{keyspace.FamilyVararg, view.Storage().Varargs().Count()},
		{keyspace.FamilyBind, view.Storage().Binds().Count()},
		{keyspace.FamilyAssign, view.Storage().Assigns().Count()},
		{keyspace.FamilyWrite, view.Storage().Writes().Count()},
		{keyspace.FamilyFunction, view.Functions().Count()},
		{keyspace.FamilyCall, view.Calls().Count()},
		{keyspace.FamilyReturn, view.Control().Returns().Count()},
		{keyspace.FamilyBreak, view.Control().Breaks().Count()},
		{keyspace.FamilyLabel, view.Control().Labels().Count()},
		{keyspace.FamilyGoto, view.Control().Gotos().Count()},
		{keyspace.FamilyBranch, view.Control().Branches().Count()},
		{keyspace.FamilyLoop, view.Control().Loops().Count()},
	}
	for _, check := range checks {
		if check.count < 0 || uint64(check.count) != uint64(counts[check.family]) ||
			!flowExecutionCountFits(check.count) {
			return errors.New("program/flow/containment: execution family cardinality mismatch")
		}
	}
	return nil
}

func flowExecutionCountFits(count int) bool {
	return count >= 0 && uint64(count) <= uint64(keyspace.MaxTermOrdinal)
}

func flowExecutionCells(view authored.Cells, counts [keyspace.FamilyCount]uint32) error {
	for ordinal := uint32(1); ordinal <= counts[keyspace.FamilyCell]; ordinal++ {
		term := keyspace.MakeTerm(keyspace.FamilyCell, ordinal)
		got, ok := view.At(int(ordinal - 1))
		if !ok || got != term {
			return errors.New("program/flow/containment: noncanonical Cell ordinal")
		}
		cellKind, body, key, ok := view.Get(term)
		if !ok || (cellKind != authored.CellLocal && cellKind != authored.CellGlobal) {
			return errors.New("program/flow/containment: invalid Cell row")
		}
		switch cellKind {
		case authored.CellLocal:
			if !flowExecutionTerm(counts, body, keyspace.FamilyBody) || key != 0 {
				return errors.New("program/flow/containment: invalid local Cell reference")
			}
		case authored.CellGlobal:
			if body != 0 || key == 0 {
				return errors.New("program/flow/containment: invalid global Cell reference")
			}
		}
	}
	return nil
}

func flowExecutionReads(view authored.Reads, cells authored.Cells, counts [keyspace.FamilyCount]uint32) error {
	for ordinal := uint32(1); ordinal <= counts[keyspace.FamilyRead]; ordinal++ {
		term := keyspace.MakeTerm(keyspace.FamilyRead, ordinal)
		got, ok := view.At(int(ordinal - 1))
		if !ok || got != term {
			return errors.New("program/flow/containment: noncanonical Read ordinal")
		}
		owner, source, implicit, ok := view.Get(term)
		if !ok || !flowExecutionTerm(counts, owner, keyspace.FamilyBody) ||
			(!flowExecutionTerm(counts, source, keyspace.FamilyCell) &&
				!flowExecutionTerm(counts, source, keyspace.FamilyLensExact) &&
				!flowExecutionTerm(counts, source, keyspace.FamilyLensKey)) {
			return errors.New("program/flow/containment: invalid Read row")
		}
		if implicit {
			cellKind, _, _, cellOK := cells.Get(source)
			if !cellOK || cellKind != authored.CellGlobal {
				return errors.New("program/flow/containment: implicit Read is not global")
			}
		}
	}
	return nil
}

func flowExecutionVarargs(view authored.Varargs, cells authored.Cells, counts [keyspace.FamilyCount]uint32) error {
	for ordinal := uint32(1); ordinal <= counts[keyspace.FamilyVararg]; ordinal++ {
		term := keyspace.MakeTerm(keyspace.FamilyVararg, ordinal)
		got, ok := view.At(int(ordinal - 1))
		if !ok || got != term {
			return errors.New("program/flow/containment: noncanonical Vararg ordinal")
		}
		owner, cell, ok := view.Get(term)
		if !ok || !flowExecutionTerm(counts, owner, keyspace.FamilyBody) ||
			!flowExecutionCell(cells, counts, cell, false, 0) {
			return errors.New("program/flow/containment: invalid Vararg row")
		}
	}
	return nil
}

func flowExecutionBinds(view authored.Binds, counts [keyspace.FamilyCount]uint32) error {
	for ordinal := uint32(1); ordinal <= counts[keyspace.FamilyBind]; ordinal++ {
		term := keyspace.MakeTerm(keyspace.FamilyBind, ordinal)
		got, ok := view.At(int(ordinal - 1))
		if !ok || got != term {
			return errors.New("program/flow/containment: noncanonical Bind ordinal")
		}
		owner, values, ok := view.Get(term)
		if !ok || !flowExecutionTerm(counts, owner, keyspace.FamilyBody) ||
			!flowExecutionTerm(counts, values, keyspace.FamilyValues) {
			return errors.New("program/flow/containment: invalid Bind row")
		}
	}
	return nil
}

func flowExecutionAssigns(view authored.Storage, counts [keyspace.FamilyCount]uint32, edges *[]kernelEdge) error {
	if edges == nil {
		return errors.New("program/flow/containment: nil execution edge emission")
	}
	assigns, writes := view.Assigns(), view.Writes()
	writeCursor := uint32(0)
	for ordinal := uint32(1); ordinal <= counts[keyspace.FamilyAssign]; ordinal++ {
		assign := keyspace.MakeTerm(keyspace.FamilyAssign, ordinal)
		got, ok := assigns.At(int(ordinal - 1))
		if !ok || got != assign {
			return errors.New("program/flow/containment: noncanonical Assign ordinal")
		}
		owner, values, ok := assigns.Get(assign)
		if !ok || !flowExecutionTerm(counts, owner, keyspace.FamilyBody) ||
			!flowExecutionTerm(counts, values, keyspace.FamilyValues) {
			return errors.New("program/flow/containment: invalid Assign row")
		}
		writeCount, ok := assigns.WriteCount(assign)
		if !ok || writeCount <= 0 || !flowExecutionCountFits(writeCount) {
			return errors.New("program/flow/containment: invalid Assign Write range")
		}
		for index := 0; index < writeCount; index++ {
			write, writeOK := assigns.WriteAt(assign, index)
			expected := keyspace.MakeTerm(keyspace.FamilyWrite, writeCursor+1)
			if !writeOK || write != expected {
				return errors.New("program/flow/containment: Assign WriteAt order mismatch")
			}
			writeAssign, target, rowOK := writes.Get(write)
			if !rowOK || writeAssign != assign || !flowExecutionWritable(counts, target) {
				return errors.New("program/flow/containment: invalid Write row")
			}
			*edges = append(*edges, kernelEdge{child: write, parent: assign})
			writeCursor++
		}
	}
	if writeCursor != counts[keyspace.FamilyWrite] {
		return errors.New("program/flow/containment: Assign Write ranges do not partition Writes")
	}
	return nil
}

func flowExecutionFunctions(view authored.Functions, cells authored.Cells, counts [keyspace.FamilyCount]uint32) error {
	innerSeen := make([]bool, int(counts[keyspace.FamilyCell])+1)
	outerSeen := make([]uint32, int(counts[keyspace.FamilyCell])+1)
	for ordinal := uint32(1); ordinal <= counts[keyspace.FamilyFunction]; ordinal++ {
		term := keyspace.MakeTerm(keyspace.FamilyFunction, ordinal)
		got, ok := view.At(int(ordinal - 1))
		if !ok || got != term {
			return errors.New("program/flow/containment: noncanonical Function ordinal")
		}
		owner, body, vararg, ok := view.Get(term)
		if !ok || !flowExecutionTerm(counts, owner, keyspace.FamilyBody) ||
			!flowExecutionTerm(counts, body, keyspace.FamilyBody) || owner == body {
			return errors.New("program/flow/containment: invalid Function row")
		}
		if vararg != 0 && !flowExecutionCell(cells, counts, vararg, true, body) {
			return errors.New("program/flow/containment: invalid Function Vararg")
		}
		captureCount, ok := view.CaptureCount(term)
		if !ok || captureCount < 0 || !flowExecutionCountFits(captureCount) {
			return errors.New("program/flow/containment: invalid Function Capture range")
		}
		for index := 0; index < captureCount; index++ {
			inner, outer, captureOK := view.CaptureAt(term, index)
			if !captureOK || !flowExecutionTerm(counts, inner, keyspace.FamilyCell) ||
				!flowExecutionTerm(counts, outer, keyspace.FamilyCell) {
				return errors.New("program/flow/containment: invalid Function Capture")
			}
			if !flowExecutionCell(cells, counts, inner, true, body) {
				return errors.New("program/flow/containment: invalid Capture inner Cell")
			}
			if !flowExecutionCell(cells, counts, outer, false, body) {
				return errors.New("program/flow/containment: invalid Capture outer Cell")
			}
			_, innerBody, _, innerOK := cells.Get(inner)
			_, outerBody, _, outerOK := cells.Get(outer)
			if !innerOK || !outerOK || outerBody == body || innerBody == outerBody {
				return errors.New("program/flow/containment: Capture does not cross Body")
			}
			innerOrdinal, outerOrdinal := keyspace.TermOrdinal(inner), keyspace.TermOrdinal(outer)
			if innerSeen[innerOrdinal] || outerSeen[outerOrdinal] == ordinal {
				return errors.New("program/flow/containment: duplicate Function Capture")
			}
			innerSeen[innerOrdinal] = true
			outerSeen[outerOrdinal] = ordinal
			if inner == outer {
				return errors.New("program/flow/containment: Capture aliases itself")
			}
		}
	}
	return nil
}

func flowExecutionCalls(view authored.Calls, counts [keyspace.FamilyCount]uint32) error {
	for ordinal := uint32(1); ordinal <= counts[keyspace.FamilyCall]; ordinal++ {
		term := keyspace.MakeTerm(keyspace.FamilyCall, ordinal)
		got, ok := view.At(int(ordinal - 1))
		if !ok || got != term {
			return errors.New("program/flow/containment: noncanonical Call ordinal")
		}
		owner, callee, receiver, actuals, ok := view.Get(term)
		if !ok || !flowExecutionTerm(counts, owner, keyspace.FamilyBody) ||
			!flowExecutionValue(counts, callee) || !flowExecutionTerm(counts, actuals, keyspace.FamilyValues) ||
			(receiver != 0 && !flowExecutionValue(counts, receiver)) {
			return errors.New("program/flow/containment: invalid Call row")
		}
	}
	return nil
}

func flowExecutionControl(view authored.Control, cells authored.Cells, counts [keyspace.FamilyCount]uint32) error {
	if err := flowExecutionReturns(view.Returns(), counts); err != nil {
		return err
	}
	if err := flowExecutionBreaks(view.Breaks(), counts); err != nil {
		return err
	}
	if err := flowExecutionLabels(view.Labels(), counts); err != nil {
		return err
	}
	if err := flowExecutionGotos(view.Gotos(), counts); err != nil {
		return err
	}
	if err := flowExecutionBranches(view.Branches(), counts); err != nil {
		return err
	}
	return flowExecutionLoops(view.Loops(), cells, counts)
}

func flowExecutionReturns(view authored.Returns, counts [keyspace.FamilyCount]uint32) error {
	for ordinal := uint32(1); ordinal <= counts[keyspace.FamilyReturn]; ordinal++ {
		term := keyspace.MakeTerm(keyspace.FamilyReturn, ordinal)
		got, ok := view.At(int(ordinal - 1))

		owner, values, rowOK := view.Get(term)
		if !ok || got != term || !rowOK || !flowExecutionTerm(counts, owner, keyspace.FamilyBody) ||
			!flowExecutionTerm(counts, values, keyspace.FamilyValues) {
			return errors.New("program/flow/containment: invalid Return row")
		}
	}
	return nil
}

func flowExecutionBreaks(view authored.Breaks, counts [keyspace.FamilyCount]uint32) error {
	for ordinal := uint32(1); ordinal <= counts[keyspace.FamilyBreak]; ordinal++ {
		term := keyspace.MakeTerm(keyspace.FamilyBreak, ordinal)
		got, ok := view.At(int(ordinal - 1))
		owner, _, rowOK := view.Get(term)
		if !ok || got != term || !rowOK || !flowExecutionTerm(counts, owner, keyspace.FamilyBody) {
			return errors.New("program/flow/containment: invalid Break row")
		}
	}
	return nil
}

func flowExecutionLabels(view authored.Labels, counts [keyspace.FamilyCount]uint32) error {
	for ordinal := uint32(1); ordinal <= counts[keyspace.FamilyLabel]; ordinal++ {
		term := keyspace.MakeTerm(keyspace.FamilyLabel, ordinal)
		got, ok := view.At(int(ordinal - 1))
		owner, rowOK := view.Get(term)
		if !ok || got != term || !rowOK || !flowExecutionTerm(counts, owner, keyspace.FamilyBody) {
			return errors.New("program/flow/containment: invalid Label row")
		}
	}
	return nil
}

func flowExecutionGotos(view authored.Gotos, counts [keyspace.FamilyCount]uint32) error {
	for ordinal := uint32(1); ordinal <= counts[keyspace.FamilyGoto]; ordinal++ {
		term := keyspace.MakeTerm(keyspace.FamilyGoto, ordinal)
		got, ok := view.At(int(ordinal - 1))
		owner, target, rowOK := view.Get(term)
		if !ok || got != term || !rowOK || !flowExecutionTerm(counts, owner, keyspace.FamilyBody) ||
			!flowExecutionTerm(counts, target, keyspace.FamilyLabel) {
			return errors.New("program/flow/containment: invalid Goto row")
		}
	}
	return nil
}

func flowExecutionBranches(view authored.Branches, counts [keyspace.FamilyCount]uint32) error {
	for ordinal := uint32(1); ordinal <= counts[keyspace.FamilyBranch]; ordinal++ {
		term := keyspace.MakeTerm(keyspace.FamilyBranch, ordinal)
		got, ok := view.At(int(ordinal - 1))
		owner, condition, whenTrue, whenFalse, rowOK := view.Get(term)
		if !ok || got != term || !rowOK || !flowExecutionTerm(counts, owner, keyspace.FamilyBody) ||
			!flowExecutionValue(counts, condition) || !flowExecutionTerm(counts, whenTrue, keyspace.FamilyBody) ||
			!flowExecutionTerm(counts, whenFalse, keyspace.FamilyBody) || owner == whenTrue || owner == whenFalse ||
			whenTrue == whenFalse {
			return errors.New("program/flow/containment: invalid Branch row")
		}
	}
	return nil
}

func flowExecutionLoops(view authored.Loops, cells authored.Cells, counts [keyspace.FamilyCount]uint32) error {
	seen := make([]bool, int(counts[keyspace.FamilyCell])+1)
	for ordinal := uint32(1); ordinal <= counts[keyspace.FamilyLoop]; ordinal++ {
		term := keyspace.MakeTerm(keyspace.FamilyLoop, ordinal)
		got, ok := view.At(int(ordinal - 1))
		owner, body, loopKind, control, rowOK := view.Get(term)
		validControl := false
		switch loopKind {
		case kind.LoopWhile, kind.LoopRepeat:
			validControl = flowExecutionValue(counts, control)
		case kind.LoopNumericFor, kind.LoopGenericFor:
			validControl = flowExecutionTerm(counts, control, keyspace.FamilyValues)
		}
		if !ok || got != term || !rowOK || !flowExecutionTerm(counts, owner, keyspace.FamilyBody) ||
			!flowExecutionTerm(counts, body, keyspace.FamilyBody) || owner == body || !flowExecutionLoopKind(loopKind) ||
			!validControl {
			return errors.New("program/flow/containment: invalid Loop row")
		}
		cellCount, countOK := view.CellCount(term)
		if !countOK || cellCount < 0 || !flowExecutionCountFits(cellCount) {
			return errors.New("program/flow/containment: invalid Loop Cell range")
		}
		for index := 0; index < cellCount; index++ {
			cell, cellOK := view.CellAt(term, index)
			if !cellOK || !flowExecutionCell(cells, counts, cell, true, body) {
				return errors.New("program/flow/containment: invalid Loop Cell")
			}
			cellOrdinal := keyspace.TermOrdinal(cell)
			if seen[cellOrdinal] {
				return errors.New("program/flow/containment: duplicate Loop Cell")
			}
			seen[cellOrdinal] = true
		}
	}
	return nil
}

func flowExecutionCell(view authored.Cells, counts [keyspace.FamilyCount]uint32, term keyspace.Term, sameBody bool, body keyspace.Term) bool {
	if !flowExecutionTerm(counts, term, keyspace.FamilyCell) {
		return false
	}
	kind, cellBody, key, ok := view.Get(term)
	if !ok || kind != authored.CellLocal || key != 0 || !flowExecutionTerm(counts, cellBody, keyspace.FamilyBody) {
		return false
	}
	return !sameBody || cellBody == body
}

func flowExecutionWritable(counts [keyspace.FamilyCount]uint32, term keyspace.Term) bool {
	return flowrole.Addressable(counts, term)
}

func flowExecutionValue(counts [keyspace.FamilyCount]uint32, term keyspace.Term) bool {
	return flowrole.ValueOccurrence(counts, term)
}

func flowExecutionTerm(counts [keyspace.FamilyCount]uint32, term keyspace.Term, family keyspace.Family) bool {
	return keyspace.TermFamily(term) == family && keyspace.TermOrdinal(term) != 0 && keyspace.TermOrdinal(term) <= counts[family]
}

func flowExecutionLoopKind(value kind.LoopKind) bool {
	return value >= kind.LoopWhile && value <= kind.LoopGenericFor
}
