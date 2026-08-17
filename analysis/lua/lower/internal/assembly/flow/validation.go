package flow

import (
	"errors"
	"fmt"

	programflow "github.com/wippyai/go-lua/analysis/program/flow"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func (r *Rows) checkCounts(counts [keyspace.FamilyCount]uint32) error {
	checks := []struct {
		family keyspace.Family
		got    int
		name   string
	}{
		{keyspace.FamilyValues, len(r.values.rows), "Values"}, {keyspace.FamilyLensExact, len(r.access.exact), "LensExact"}, {keyspace.FamilyLensKey, len(r.access.dynamic), "LensKey"}, {keyspace.FamilyCell, len(r.storage.cells), "Cell"}, {keyspace.FamilyRead, len(r.storage.reads), "Read"}, {keyspace.FamilyVararg, len(r.storage.varargs), "Vararg"}, {keyspace.FamilyBind, len(r.storage.binds), "Bind"}, {keyspace.FamilyAssign, len(r.storage.assigns), "Assign"}, {keyspace.FamilyWrite, len(r.storage.writes), "Write"}, {keyspace.FamilyTable, len(r.tables.rows), "Table"}, {keyspace.FamilyTableField, len(r.tables.fields), "TableField"}, {keyspace.FamilyFunction, len(r.functions.rows), "Function"}, {keyspace.FamilyCall, len(r.calls.rows), "Call"}, {keyspace.FamilyReturn, len(r.control.returns), "Return"}, {keyspace.FamilyBreak, len(r.control.breaks), "Break"}, {keyspace.FamilyLabel, len(r.control.labels), "Label"}, {keyspace.FamilyGoto, len(r.control.gotos), "Goto"}, {keyspace.FamilyBranch, len(r.control.branches), "Branch"}, {keyspace.FamilyLoop, len(r.control.loops), "Loop"}, {keyspace.FamilyUnary, len(r.operators.unaries), "Unary"}, {keyspace.FamilyBinary, len(r.operators.binaries), "Binary"}, {keyspace.FamilySelect, len(r.operators.selects), "Select"}, {keyspace.FamilyValueClaim, len(r.operands.claims), "ValueClaim"}, {keyspace.FamilyTypeValue, len(r.operands.typeValues), "TypeValue"},
	}
	for _, check := range checks {
		if check.got < 0 || uint64(check.got) > uint64(keyspace.MaxTermOrdinal) || uint64(check.got) != uint64(counts[check.family]) {
			return fmt.Errorf("program/lower/collector: %s denominator mismatch", check.name)
		}
	}
	globalCount := r.storage.globalCensus.Len()
	if globalCount > len(r.storage.cells) {
		return errors.New("program/lower/collector: global census rows exceed Cell rows")
	}
	for index := 0; index < globalCount; index++ {
		row, ok := r.storage.globalCensus.At(index)
		if !ok || row.Slot() != uint32(index) || row.Ordinal() != uint32(index+1) || r.storage.cells[index].Kind != programflow.CellGlobal {
			return fmt.Errorf("program/lower/collector: invalid global Cell prefix at %d", index+1)
		}
	}
	for index := globalCount; index < len(r.storage.cells); index++ {
		if r.storage.cells[index].Kind == programflow.CellGlobal {
			return fmt.Errorf("program/lower/collector: unreserved global Cell at %d", index+1)
		}
	}
	if len(r.values.rows) == 0 && len(r.values.terms) != 0 {
		return errors.New("program/lower/collector: orphan Values terms")
	}
	if len(r.tables.filled) != len(r.tables.rows) {
		return errors.New("program/lower/collector: Table fill denominator mismatch")
	}
	for index, filled := range r.tables.filled {
		if !filled {
			return fmt.Errorf("program/lower/collector: Table %d was not filled", index+1)
		}
	}
	if len(r.tables.fields) == 0 && len(r.tables.order) != 0 {
		return errors.New("program/lower/collector: orphan Table order")
	}
	if uint64(len(r.functions.captures)) > uint64(keyspace.MaxTermOrdinal) || uint64(len(r.control.loopCells)) > uint64(keyspace.MaxTermOrdinal) {
		return errors.New("program/lower/collector: dense pool overflow")
	}
	return nil
}
