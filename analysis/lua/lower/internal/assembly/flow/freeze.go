package flow

import (
	"errors"
	"fmt"

	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	programsource "github.com/wippyai/go-lua/analysis/program/source"
)

func cloneTerms(values []keyspace.Term) []keyspace.Term {
	return append([]keyspace.Term(nil), values...)
}

// Freeze copies the complete Flow rows and resolves only the binder-owned
// global Cell keys through the Source preimage supplied by assembly core.
func (r *Rows) Freeze(preimage programsource.Preimage, counts [keyspace.FamilyCount]uint32) (authored.Input, error) {
	if r == nil || !preimage.Identity().ContentID().Available() {
		return authored.Input{}, errors.New("program/lower/collector: unavailable Source Preimage")
	}
	if counts[keyspace.FamilyInvalid] != 0 || counts[keyspace.FamilyOutcome] != 0 {
		return authored.Input{}, errors.New("program/lower/collector: invalid Flow count denominator")
	}
	if err := r.checkCounts(counts); err != nil {
		return authored.Input{}, err
	}
	if err := r.resolveBreakTargets(preimage, counts); err != nil {
		return authored.Input{}, err
	}
	input := authored.Input{Counts: counts}
	input.Values = authored.ValuesInput{Rows: append([]authored.Value(nil), r.values.rows...), Terms: cloneTerms(r.values.terms)}
	input.Access = authored.AccessInput{Exact: append([]authored.ExactLens(nil), r.access.exact...), Dynamic: append([]authored.DynamicLens(nil), r.access.dynamic...)}
	input.Storage = authored.StorageInput{
		Cells: append([]authored.Cell(nil), r.storage.cells...), Reads: append([]authored.Read(nil), r.storage.reads...), Varargs: append([]authored.Vararg(nil), r.storage.varargs...), Binds: append([]authored.Bind(nil), r.storage.binds...), Assigns: append([]authored.Assign(nil), r.storage.assigns...), Writes: append([]authored.Write(nil), r.storage.writes...),
	}
	input.Tables = authored.TablesInput{Rows: append([]authored.Table(nil), r.tables.rows...), Fields: append([]authored.Field(nil), r.tables.fields...), Order: cloneTerms(r.tables.order)}
	input.Functions = authored.FunctionsInput{Rows: append([]authored.Function(nil), r.functions.rows...), Captures: append([]authored.Capture(nil), r.functions.captures...)}
	input.Calls = append([]authored.Call(nil), r.calls.rows...)
	input.Control = authored.ControlInput{Returns: append([]authored.Return(nil), r.control.returns...), Breaks: append([]authored.Break(nil), r.control.breaks...), Labels: append([]authored.Label(nil), r.control.labels...), Gotos: append([]authored.Goto(nil), r.control.gotos...), Branches: append([]authored.Branch(nil), r.control.branches...), Loops: append([]authored.Loop(nil), r.control.loops...), Cells: cloneTerms(r.control.loopCells)}
	input.Operators = authored.OperatorsInput{Unaries: append([]authored.Unary(nil), r.operators.unaries...), Binaries: append([]authored.Binary(nil), r.operators.binaries...), Selects: append([]authored.Select(nil), r.operators.selects...)}
	input.Claims = append([]authored.ValueClaim(nil), r.operands.claims...)
	input.TypeValues = append([]authored.TypeValue(nil), r.operands.typeValues...)
	keys := preimage.Keys()
	for index := 0; index < r.storage.globalCensus.Len(); index++ {
		row, ok := r.storage.globalCensus.At(index)
		if !ok || index >= len(input.Storage.Cells) || row.Slot() != uint32(index) || input.Storage.Cells[index].Kind != authored.CellGlobal {
			return authored.Input{}, fmt.Errorf("program/lower/collector: malformed global Cell prefix at %d", index+1)
		}
		key, ok := keys.Find(keyspace.LiteralValue{Kind: keyspace.LiteralString, String: row.Name()})
		if !ok || key == 0 {
			return authored.Input{}, fmt.Errorf("program/lower/collector: Source is missing global atom %q", row.Name())
		}
		input.Storage.Cells[index].Key = key
		input.Storage.Cells[index].Body = 0
	}
	for index := r.storage.globalCensus.Len(); index < len(input.Storage.Cells); index++ {
		if input.Storage.Cells[index].Kind == authored.CellGlobal {
			return authored.Input{}, fmt.Errorf("program/lower/collector: global Cell outside reserved census prefix at %d", index+1)
		}
	}
	return input, nil
}

// resolveBreakTargets is the lower assembly's one canonical owner/target
// boundary. Break rows retain their lexical Body owner, while the authored
// target is selected from immutable Source body nesting and authored
// Loop-to-Body rows before Flow is built.
func (r *Rows) resolveBreakTargets(preimage programsource.Preimage, counts [keyspace.FamilyCount]uint32) error {
	if r == nil {
		return errors.New("program/lower/collector: nil Flow rows")
	}
	bodyCount := int(counts[keyspace.FamilyBody])
	parents := make([]keyspace.Term, bodyCount+1)
	for ordinal := 1; ordinal <= bodyCount; ordinal++ {
		body := keyspace.MakeTerm(keyspace.FamilyBody, uint32(ordinal))
		length, ok := preimage.Order().BodyLen(body)
		if !ok {
			return fmt.Errorf("program/lower/collector: missing Source order for Body %v", body)
		}
		for index := 0; index < length; index++ {
			child, childOK := preimage.Order().BodyAt(body, index)
			if !childOK || keyspace.TermFamily(child) != keyspace.FamilyBody {
				continue
			}
			childOrdinal := keyspace.TermOrdinal(child)
			if childOrdinal == 0 || int(childOrdinal) > bodyCount || (parents[childOrdinal] != 0 && parents[childOrdinal] != body) {
				return fmt.Errorf("program/lower/collector: invalid Source Body parent for %v", child)
			}
			parents[childOrdinal] = body
		}
	}
	setParent := func(child, parent keyspace.Term) error {
		childOrdinal := keyspace.TermOrdinal(child)
		parentOrdinal := keyspace.TermOrdinal(parent)
		if keyspace.TermFamily(child) != keyspace.FamilyBody || childOrdinal == 0 || int(childOrdinal) > bodyCount ||
			keyspace.TermFamily(parent) != keyspace.FamilyBody || parentOrdinal == 0 || int(parentOrdinal) > bodyCount {
			return fmt.Errorf("program/lower/collector: invalid Body parent relation %v -> %v", parent, child)
		}
		if parents[childOrdinal] != 0 && parents[childOrdinal] != parent {
			return fmt.Errorf("program/lower/collector: conflicting Body parents for %v", child)
		}
		parents[childOrdinal] = parent
		return nil
	}
	for _, row := range r.control.branches {
		if err := setParent(row.WhenTrue, row.Owner); err != nil {
			return err
		}
		if err := setParent(row.WhenFalse, row.Owner); err != nil {
			return err
		}
	}

	loopByBody := make([]keyspace.Term, bodyCount+1)
	for index, row := range r.control.loops {
		bodyOrdinal := keyspace.TermOrdinal(row.Body)
		if bodyOrdinal == 0 || int(bodyOrdinal) > bodyCount {
			return fmt.Errorf("program/lower/collector: Loop %d has invalid Body", index+1)
		}
		if loopByBody[bodyOrdinal] != 0 {
			return fmt.Errorf("program/lower/collector: Body %v has duplicate Loop owners", row.Body)
		}
		loopByBody[bodyOrdinal] = keyspace.MakeTerm(keyspace.FamilyLoop, uint32(index+1))
		if err := setParent(row.Body, row.Owner); err != nil {
			return err
		}
	}
	functionBody := make([]bool, bodyCount+1)
	for _, row := range r.functions.rows {
		bodyOrdinal := keyspace.TermOrdinal(row.Body)
		if bodyOrdinal == 0 || int(bodyOrdinal) > bodyCount {
			return errors.New("program/lower/collector: Function has invalid Body")
		}
		functionBody[bodyOrdinal] = true
		if err := setParent(row.Body, row.Owner); err != nil {
			return err
		}
	}

	for index := range r.control.breaks {
		owner := r.control.breaks[index].Owner
		ownerOrdinal := keyspace.TermOrdinal(owner)
		if keyspace.TermFamily(owner) != keyspace.FamilyBody || ownerOrdinal == 0 || int(ownerOrdinal) > bodyCount {
			return fmt.Errorf("program/lower/collector: Break %d has invalid Body owner", index+1)
		}
		current := owner
		target := keyspace.Term(0)
		for current != 0 {
			currentOrdinal := keyspace.TermOrdinal(current)
			if functionBody[currentOrdinal] {
				break
			}
			if loop := loopByBody[currentOrdinal]; loop != 0 {
				target = loop
				break
			}
			parent := parents[currentOrdinal]
			if parent == 0 {
				break
			}
			current = parent
		}
		if target == 0 {
			return fmt.Errorf("program/lower/collector: Break %d has no same-function Loop target", index+1)
		}
		if r.control.breaks[index].Target != 0 && r.control.breaks[index].Target != target {
			return fmt.Errorf("program/lower/collector: Break %d target disagrees with Body topology", index+1)
		}
		r.control.breaks[index].Target = target
	}
	return nil
}

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
		if !ok || row.Slot() != uint32(index) || row.Ordinal() != uint32(index+1) || r.storage.cells[index].Kind != authored.CellGlobal {
			return fmt.Errorf("program/lower/collector: invalid global Cell prefix at %d", index+1)
		}
	}
	for index := globalCount; index < len(r.storage.cells); index++ {
		if r.storage.cells[index].Kind == authored.CellGlobal {
			return fmt.Errorf("program/lower/collector: global Cell outside reserved census prefix at %d", index+1)
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
