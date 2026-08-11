package collector

import (
	"errors"
	"fmt"

	"github.com/wippyai/go-lua/program/flow"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/source"
)

// freeze copies complete authored Flow rows and resolves only global Cell
// keys through Source's live Preimage. The returned Input contains no maps,
// raw key payloads, spans, or derived candidate/index state.
func (rows *flowRows) freeze(preimage source.Preimage, counts [keyspace.FamilyCount]uint32) (flow.Input, error) {
	if rows == nil || !preimage.Identity().ContentID().Available() {
		return flow.Input{}, errors.New("program/lower/collector: unavailable Source Preimage")
	}
	if counts[keyspace.FamilyInvalid] != 0 || counts[keyspace.FamilyOutcome] != 0 {
		return flow.Input{}, errors.New("program/lower/collector: invalid Flow count denominator")
	}
	if err := rows.checkCounts(counts); err != nil {
		return flow.Input{}, err
	}

	input := flow.Input{Counts: counts}
	input.Values = flow.ValuesInput{
		Rows:  append([]flow.Value(nil), rows.values.values...),
		Terms: cloneTerms(rows.values.valueTerms),
	}
	input.Access = flow.AccessInput{
		Exact:   append([]flow.ExactLens(nil), rows.access.exactLenses...),
		Dynamic: append([]flow.DynamicLens(nil), rows.access.dynamicLenses...),
	}
	input.Storage = flow.StorageInput{
		Cells:   append([]flow.Cell(nil), rows.storage.cells...),
		Reads:   append([]flow.Read(nil), rows.storage.reads...),
		Varargs: append([]flow.Vararg(nil), rows.storage.varargs...),
		Binds:   append([]flow.Bind(nil), rows.storage.binds...),
		Assigns: append([]flow.Assign(nil), rows.storage.assigns...),
		Writes:  append([]flow.Write(nil), rows.storage.writes...),
	}
	input.Tables = flow.TablesInput{
		Rows:   append([]flow.Table(nil), rows.tables.tables...),
		Fields: append([]flow.Field(nil), rows.tables.tableFields...),
		Order:  cloneTerms(rows.tables.tableOrder),
	}
	input.Functions = flow.FunctionsInput{
		Rows:     append([]flow.Function(nil), rows.functions.functions...),
		Captures: append([]flow.Capture(nil), rows.functions.captures...),
	}
	input.Calls = append([]flow.Call(nil), rows.calls.calls...)
	input.Control = flow.ControlInput{
		Returns:  append([]flow.Return(nil), rows.control.returns...),
		Breaks:   append([]flow.Break(nil), rows.control.breaks...),
		Labels:   append([]flow.Label(nil), rows.control.labels...),
		Gotos:    append([]flow.Goto(nil), rows.control.gotos...),
		Branches: append([]flow.Branch(nil), rows.control.branches...),
		Loops:    append([]flow.Loop(nil), rows.control.loops...),
		Cells:    cloneTerms(rows.control.loopCells),
	}
	input.Operators = flow.OperatorsInput{
		Unaries:  append([]flow.Unary(nil), rows.operators.unaries...),
		Binaries: append([]flow.Binary(nil), rows.operators.binaries...),
		Selects:  append([]flow.Select(nil), rows.operators.selects...),
	}
	input.Claims = append([]flow.ValueClaim(nil), rows.operands.claims...)
	input.TypeValues = append([]flow.TypeValue(nil), rows.operands.typeValues...)

	keys := preimage.Keys()
	for index := 0; index < rows.storage.globalCensus.Len(); index++ {
		row, ok := rows.storage.globalCensus.At(index)
		if !ok || index >= len(input.Storage.Cells) || row.Slot() != uint32(index) ||
			input.Storage.Cells[index].Kind != flow.CellGlobal {
			return flow.Input{}, fmt.Errorf("program/lower/collector: malformed global Cell prefix at %d", index+1)
		}
		name := row.Name()
		key, ok := keys.Find(keyspace.LiteralValue{Kind: keyspace.LiteralString, String: name})
		if !ok || key == 0 {
			return flow.Input{}, fmt.Errorf("program/lower/collector: Source is missing global atom %q", name)
		}
		input.Storage.Cells[index].Key = key
		input.Storage.Cells[index].Body = 0
	}
	for index := rows.storage.globalCensus.Len(); index < len(input.Storage.Cells); index++ {
		if input.Storage.Cells[index].Kind == flow.CellGlobal {
			return flow.Input{}, fmt.Errorf("program/lower/collector: global Cell outside reserved census prefix at %d", index+1)
		}
	}
	return input, nil
}

func (rows *flowRows) checkCounts(counts [keyspace.FamilyCount]uint32) error {
	pairs := []struct {
		family keyspace.Family
		got    int
		name   string
	}{
		{keyspace.FamilyValues, len(rows.values.values), "Values"},
		{keyspace.FamilyLensExact, len(rows.access.exactLenses), "LensExact"},
		{keyspace.FamilyLensKey, len(rows.access.dynamicLenses), "LensKey"},
		{keyspace.FamilyCell, len(rows.storage.cells), "Cell"},
		{keyspace.FamilyRead, len(rows.storage.reads), "Read"},
		{keyspace.FamilyVararg, len(rows.storage.varargs), "Vararg"},
		{keyspace.FamilyBind, len(rows.storage.binds), "Bind"},
		{keyspace.FamilyAssign, len(rows.storage.assigns), "Assign"},
		{keyspace.FamilyWrite, len(rows.storage.writes), "Write"},
		{keyspace.FamilyTable, len(rows.tables.tables), "Table"},
		{keyspace.FamilyTableField, len(rows.tables.tableFields), "TableField"},
		{keyspace.FamilyFunction, len(rows.functions.functions), "Function"},
		{keyspace.FamilyCall, len(rows.calls.calls), "Call"},
		{keyspace.FamilyReturn, len(rows.control.returns), "Return"},
		{keyspace.FamilyBreak, len(rows.control.breaks), "Break"},
		{keyspace.FamilyLabel, len(rows.control.labels), "Label"},
		{keyspace.FamilyGoto, len(rows.control.gotos), "Goto"},
		{keyspace.FamilyBranch, len(rows.control.branches), "Branch"},
		{keyspace.FamilyLoop, len(rows.control.loops), "Loop"},
		{keyspace.FamilyUnary, len(rows.operators.unaries), "Unary"},
		{keyspace.FamilyBinary, len(rows.operators.binaries), "Binary"},
		{keyspace.FamilySelect, len(rows.operators.selects), "Select"},
		{keyspace.FamilyValueClaim, len(rows.operands.claims), "ValueClaim"},
		{keyspace.FamilyTypeValue, len(rows.operands.typeValues), "TypeValue"},
	}
	for _, pair := range pairs {
		if pair.got < 0 || uint64(pair.got) > uint64(keyspace.MaxTermOrdinal) || uint64(pair.got) != uint64(counts[pair.family]) {
			return fmt.Errorf("program/lower/collector: %s denominator mismatch", pair.name)
		}
	}
	globalCount := rows.storage.globalCensus.Len()
	if globalCount > len(rows.storage.cells) {
		return errors.New("program/lower/collector: global census rows exceed Cell rows")
	}
	for index := 0; index < globalCount; index++ {
		row, ok := rows.storage.globalCensus.At(index)
		if !ok || row.Slot() != uint32(index) || row.Ordinal() != uint32(index+1) ||
			rows.storage.cells[index].Kind != flow.CellGlobal {
			return fmt.Errorf("program/lower/collector: invalid global Cell prefix at %d", index+1)
		}
	}
	for index := globalCount; index < len(rows.storage.cells); index++ {
		if rows.storage.cells[index].Kind == flow.CellGlobal {
			return fmt.Errorf("program/lower/collector: unreserved global Cell at %d", index+1)
		}
	}
	if len(rows.values.values) == 0 && len(rows.values.valueTerms) != 0 {
		return errors.New("program/lower/collector: orphan Values terms")
	}
	if len(rows.tables.tableFilled) != len(rows.tables.tables) {
		return errors.New("program/lower/collector: Table fill denominator mismatch")
	}
	for index, filled := range rows.tables.tableFilled {
		if !filled {
			return fmt.Errorf("program/lower/collector: Table %d was not filled", index+1)
		}
	}
	if len(rows.tables.tableFields) == 0 && len(rows.tables.tableOrder) != 0 {
		return errors.New("program/lower/collector: orphan Table order")
	}
	if uint64(len(rows.functions.captures)) > uint64(keyspace.MaxTermOrdinal) || uint64(len(rows.control.loopCells)) > uint64(keyspace.MaxTermOrdinal) {
		return errors.New("program/lower/collector: dense pool overflow")
	}
	return nil
}
