package flow

import (
	"errors"
	"fmt"

	programflow "github.com/wippyai/go-lua/analysis/program/flow"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	programsource "github.com/wippyai/go-lua/analysis/program/source"
)

func cloneTerms(values []keyspace.Term) []keyspace.Term {
	return append([]keyspace.Term(nil), values...)
}

// Freeze copies the complete Flow rows and resolves only the binder-owned
// global Cell keys through the Source preimage supplied by assembly core.
func (r *Rows) Freeze(preimage programsource.Preimage, counts [keyspace.FamilyCount]uint32) (programflow.Input, error) {
	if r == nil || !preimage.Identity().ContentID().Available() {
		return programflow.Input{}, errors.New("program/lower/collector: unavailable Source Preimage")
	}
	if counts[keyspace.FamilyInvalid] != 0 || counts[keyspace.FamilyOutcome] != 0 {
		return programflow.Input{}, errors.New("program/lower/collector: invalid Flow count denominator")
	}
	if err := r.checkCounts(counts); err != nil {
		return programflow.Input{}, err
	}
	if err := r.resolveBreakTargets(preimage, counts); err != nil {
		return programflow.Input{}, err
	}
	input := programflow.Input{Counts: counts}
	input.Values = programflow.ValuesInput{Rows: append([]programflow.Value(nil), r.values.rows...), Terms: cloneTerms(r.values.terms)}
	input.Access = programflow.AccessInput{Exact: append([]programflow.ExactLens(nil), r.access.exact...), Dynamic: append([]programflow.DynamicLens(nil), r.access.dynamic...)}
	input.Storage = programflow.StorageInput{
		Cells: append([]programflow.Cell(nil), r.storage.cells...), Reads: append([]programflow.Read(nil), r.storage.reads...), Varargs: append([]programflow.Vararg(nil), r.storage.varargs...), Binds: append([]programflow.Bind(nil), r.storage.binds...), Assigns: append([]programflow.Assign(nil), r.storage.assigns...), Writes: append([]programflow.Write(nil), r.storage.writes...),
	}
	input.Tables = programflow.TablesInput{Rows: append([]programflow.Table(nil), r.tables.rows...), Fields: append([]programflow.Field(nil), r.tables.fields...), Order: cloneTerms(r.tables.order)}
	input.Functions = programflow.FunctionsInput{Rows: append([]programflow.Function(nil), r.functions.rows...), Captures: append([]programflow.Capture(nil), r.functions.captures...)}
	input.Calls = append([]programflow.Call(nil), r.calls.rows...)
	input.Control = programflow.ControlInput{Returns: append([]programflow.Return(nil), r.control.returns...), Breaks: append([]programflow.Break(nil), r.control.breaks...), Labels: append([]programflow.Label(nil), r.control.labels...), Gotos: append([]programflow.Goto(nil), r.control.gotos...), Branches: append([]programflow.Branch(nil), r.control.branches...), Loops: append([]programflow.Loop(nil), r.control.loops...), Cells: cloneTerms(r.control.loopCells)}
	input.Operators = programflow.OperatorsInput{Unaries: append([]programflow.Unary(nil), r.operators.unaries...), Binaries: append([]programflow.Binary(nil), r.operators.binaries...), Selects: append([]programflow.Select(nil), r.operators.selects...)}
	input.Claims = append([]programflow.ValueClaim(nil), r.operands.claims...)
	input.TypeValues = append([]programflow.TypeValue(nil), r.operands.typeValues...)
	keys := preimage.Keys()
	for index := 0; index < r.storage.globalCensus.Len(); index++ {
		row, ok := r.storage.globalCensus.At(index)
		if !ok || index >= len(input.Storage.Cells) || row.Slot() != uint32(index) || input.Storage.Cells[index].Kind != programflow.CellGlobal {
			return programflow.Input{}, fmt.Errorf("program/lower/collector: malformed global Cell prefix at %d", index+1)
		}
		key, ok := keys.Find(keyspace.LiteralValue{Kind: keyspace.LiteralString, String: row.Name()})
		if !ok || key == 0 {
			return programflow.Input{}, fmt.Errorf("program/lower/collector: Source is missing global atom %q", row.Name())
		}
		input.Storage.Cells[index].Key = key
		input.Storage.Cells[index].Body = 0
	}
	for index := r.storage.globalCensus.Len(); index < len(input.Storage.Cells); index++ {
		if input.Storage.Cells[index].Kind == programflow.CellGlobal {
			return programflow.Input{}, fmt.Errorf("program/lower/collector: global Cell outside reserved census prefix at %d", index+1)
		}
	}
	return input, nil
}
