package flow

import (
	"errors"

	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
)

var errFlowCounts = errors.New("program/flow: invalid denominator counts")

// CountRows derives Flow's native denominator rows from its authored and
// sealed projections. The result contains only neutral schema identities;
// Program combines it with the other owner sets without re-walking Flow.
func CountRows(view View) (denominator.CountRows, error) {
	if !view.ContentID().Available() {
		return denominator.CountRows{}, errFlowCounts
	}
	authored := view.Authored()
	values := authored.Values()
	valueCount := values.Count()
	occurrences := 0
	for index := 0; index < valueCount; index++ {
		term, ok := values.At(index)
		if !ok {
			return denominator.CountRows{}, errFlowCounts
		}
		fixed, ok := values.Len(term)
		if !ok || !addFlowCount(&occurrences, fixed) {
			return denominator.CountRows{}, errFlowCounts
		}
		_, tail, ok := values.Get(term)
		if !ok {
			return denominator.CountRows{}, errFlowCounts
		}
		if tail != 0 && !addFlowCount(&occurrences, 1) {
			return denominator.CountRows{}, errFlowCounts
		}
	}
	access := authored.Access()
	storage := authored.Storage()
	cells, reads, assigns, writes := storage.Cells().Count(), storage.Reads().Count(), storage.Assigns().Count(), storage.Writes().Count()
	varargs, binds := storage.Varargs().Count(), storage.Binds().Count()
	globals := 0
	for index := 0; index < cells; index++ {
		term, ok := storage.Cells().At(index)
		if !ok {
			return denominator.CountRows{}, errFlowCounts
		}
		cellKind, _, _, ok := storage.Cells().Get(term)
		if !ok {
			return denominator.CountRows{}, errFlowCounts
		}
		if cellKind == CellGlobal {
			globals++
		}
	}
	storagePrimary, ok := denominator.SumInts(cells, globals, reads, assigns, writes, varargs, binds)
	if !ok || !flowCountFits(storagePrimary) {
		return denominator.CountRows{}, errFlowCounts
	}
	tables, fields := authored.Tables().Count(), authored.Fields().Count()
	operators := authored.Operators()
	unaries, binaries, selects := operators.Unaries().Count(), operators.Binaries().Count(), operators.Selects().Count()
	operatorPrimary, ok := denominator.SumInts(unaries, binaries, selects)
	if !ok || !flowCountFits(operatorPrimary) {
		return denominator.CountRows{}, errFlowCounts
	}
	accessPrimary, ok := denominator.SumInts(access.Exact().Count(), access.Dynamic().Count())
	if !ok || !flowCountFits(accessPrimary) {
		return denominator.CountRows{}, errFlowCounts
	}
	constructorPrimary, ok := denominator.SumInts(tables, fields)
	if !ok || !flowCountFits(constructorPrimary) {
		return denominator.CountRows{}, errFlowCounts
	}
	candidates := view.Candidates()
	functions := authored.Functions()
	captures := 0
	for index := 0; index < functions.Count(); index++ {
		term, ok := functions.At(index)
		if !ok {
			return denominator.CountRows{}, errFlowCounts
		}
		captureCount, ok := functions.CaptureCount(term)
		if !ok || !addFlowCount(&captures, captureCount) {
			return denominator.CountRows{}, errFlowCounts
		}
	}
	calls := authored.Calls().Count()
	directCalls := 0
	for index := 0; index < calls; index++ {
		term, ok := authored.Calls().At(index)
		if !ok {
			return denominator.CountRows{}, errFlowCounts
		}
		if _, _, ok := view.AccessGeometry().DirectCall(term); ok {
			directCalls++
		}
	}
	control := authored.Control()
	returns, breaks := control.Returns().Count(), control.Breaks().Count()
	labels, gotos := control.Labels().Count(), control.Gotos().Count()
	branches, loops := control.Branches().Count(), control.Loops().Count()
	genericFor := 0
	for index := 0; index < loops; index++ {
		term, ok := control.Loops().At(index)
		if !ok {
			return denominator.CountRows{}, errFlowCounts
		}
		_, _, loopKind, _, ok := control.Loops().Get(term)
		if !ok {
			return denominator.CountRows{}, errFlowCounts
		}
		if loopKind == flowkind.LoopGenericFor {
			genericFor++
		}
	}
	controlPrimary, ok := denominator.SumInts(returns, breaks, labels, gotos, branches, loops)
	if !ok || !flowCountFits(controlPrimary) {
		return denominator.CountRows{}, errFlowCounts
	}
	transfers := view.Causal().Edges().Count()
	ids := denominator.GeneratedProgramFlowIDs()
	valuesToPublish := []struct {
		id    schema.EntryID
		value int
	}{
		{ids.ProgramFlowValues, valueCount},
		{ids.ProgramFlowValueOccurrence, occurrences},
		{ids.ProgramFlowLens, accessPrimary},
		{ids.ProgramFlowStorage, storagePrimary},
		{ids.ProgramFlowStorageCell, cells},
		{ids.ProgramFlowStorageGlobal, globals},
		{ids.ProgramFlowStorageRead, reads},
		{ids.ProgramFlowStorageAssign, assigns},
		{ids.ProgramFlowStorageWrite, writes},
		{ids.ProgramFlowStorageVararg, varargs},
		{ids.ProgramFlowStorageBind, binds},
		{ids.ProgramFlowConstructors, constructorPrimary},
		{ids.ProgramFlowConstructorField, fields},
		{ids.ProgramFlowOperators, operatorPrimary},
		{ids.ProgramFlowUnaryNumeric, candidates.Unary().NumericCount()},
		{ids.ProgramFlowLength, candidates.Unary().LengthCount()},
		{ids.ProgramFlowArithmetic, candidates.Binary().ArithmeticCount()},
		{ids.ProgramFlowBitwise, candidates.Binary().BitwiseCount()},
		{ids.ProgramFlowConcat, candidates.Binary().ConcatCount()},
		{ids.ProgramFlowEquality, candidates.Binary().EqualityCount()},
		{ids.ProgramFlowOrder, candidates.Binary().OrderCount()},
		{ids.ProgramFlowIndexGet, candidates.Access().GetCount()},
		{ids.ProgramFlowIndexSet, candidates.Access().SetCount()},
		{ids.ProgramFlowFunction, functions.Count()},
		{ids.ProgramFlowFunctionCapture, captures},
		{ids.ProgramFlowCall, calls},
		{ids.ProgramFlowDirectCallBinding, directCalls},
		{ids.ProgramFlowControl, controlPrimary},
		{ids.ProgramFlowGenericFor, genericFor},
		{ids.ProgramFlowClaim, authored.Claims().Count()},
		{ids.ProgramFlowTypeValue, authored.TypeValues().Count()},
		{ids.ProgramFlowOutcome, view.Outcomes().Count()},
		{ids.ProgramFlowTransfer, transfers},
	}
	rows := make([]denominator.CountRow, 0, len(valuesToPublish))
	for _, value := range valuesToPublish {
		row, ok := flowCountRow(value.id, value.value)
		if !ok {
			return denominator.CountRows{}, errFlowCounts
		}
		rows = append(rows, row)
	}
	sealed, ok := denominator.NewCountRows(rows)
	if !ok {
		return denominator.CountRows{}, errFlowCounts
	}
	return sealed, nil
}

func flowCountRow(id schema.EntryID, value int) (denominator.CountRow, bool) {
	if !flowCountFits(value) {
		return denominator.CountRow{}, false
	}
	return denominator.NewCountRow(id, uint64(value))
}

func flowCountFits(value int) bool {
	return value >= 0 && uint64(value) <= uint64(keyspace.MaxTermOrdinal)
}

func addFlowCount(total *int, value int) bool {
	if total == nil || !flowCountFits(value) {
		return false
	}
	sum, ok := denominator.SumInts(*total, value)
	if !ok || !flowCountFits(sum) {
		return false
	}
	*total = sum
	return true
}
