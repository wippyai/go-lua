package selectapply

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	programsource "github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/domain/type/channelselect"
)

// Handler is the compile-time if/elseif chain that switches on one select
// result's channel field. Facts stay in the Snapshot; this names the handled
// ordinals and the source span of the first arm.
type Handler struct {
	Site          identity.ContentID
	Location      programsource.Span
	Result        string
	Handled       []int
	Names         map[int]string
	SelectDefault bool
	ElseDefault   bool
}

// Handlers projects each select application's result-channel if-chain.
func Handlers(prog *program.Program, apps []Application) []Handler {
	if prog == nil || len(apps) == 0 {
		return nil
	}
	var handlers []Handler
	for _, app := range apps {
		handler, ok := handlerFor(prog, app)
		if !ok {
			continue
		}
		handlers = append(handlers, handler)
	}
	return handlers
}

func handlerFor(prog *program.Program, app Application) (Handler, bool) {
	call, callOK := prog.Flow().Authored().Calls().At(app.Index)
	if !callOK {
		return Handler{}, false
	}
	resultCell, resultOK := bindCellOf(prog, call)
	if !resultOK {
		return Handler{}, false
	}
	_, _, _, actuals, rowOK := prog.Flow().Authored().Calls().Get(call)
	if !rowOK {
		return Handler{}, false
	}
	cells := caseCells(prog, actuals)
	tableType, tableOK := selectArgumentType(prog, actuals)
	selectDefault := false
	if tableOK {
		_, selectDefault, _ = channelselect.CasesFromTable(tableType)
	}
	resultName, named := prog.Source().Spellings().CellName(resultCell)
	if !named || resultName == "" {
		return Handler{}, false
	}
	head, handled, _, elseDefault, headOK := channelChain(prog, resultCell, cells)
	if !headOK {
		return Handler{}, false
	}
	span, spanOK := chainLocation(prog, head)
	if !spanOK {
		return Handler{}, false
	}
	names := make(map[int]string, len(cells))
	for ordinal, cell := range cells {
		if name, cellNamed := prog.Source().Spellings().CellName(cell); cellNamed && name != "" {
			names[ordinal] = name
		}
	}
	return Handler{
		Site: app.Site, Location: span, Result: resultName, Handled: handled, Names: names,
		SelectDefault: selectDefault, ElseDefault: elseDefault,
	}, true
}

func chainLocation(prog *program.Program, head keyspace.Term) (programsource.Span, bool) {
	_, condition, _, _, ok := prog.Flow().Authored().Control().Branches().Get(head)
	if !ok {
		return programsource.Span{}, false
	}
	span, spanOK := prog.Source().Identity().Span(condition)
	if spanOK {
		return span, true
	}
	return prog.Source().Identity().Span(head)
}

func bindCellOf(prog *program.Program, value keyspace.Term) (keyspace.Term, bool) {
	binds := prog.Flow().Authored().Storage().Binds()
	writes := prog.Flow().Authored().Storage().Writes()
	for index := 0; index < binds.Count(); index++ {
		bind, bindOK := binds.At(index)
		_, values, rowOK := binds.Get(bind)
		if !bindOK || !rowOK {
			continue
		}
		if !valuesContains(prog, values, value) {
			continue
		}
		if n, ok := prog.Source().Binds().Len(bind); ok && n > 0 {
			if target, targetOK := prog.Source().Binds().At(bind, 0); targetOK && target != 0 {
				return target, true
			}
		}
		for writeIndex := 0; writeIndex < writes.Count(); writeIndex++ {
			write, writeOK := writes.At(writeIndex)
			assign, target, writeRowOK := writes.Get(write)
			if writeOK && writeRowOK && assign == bind && target != 0 {
				return target, true
			}
		}
	}
	assigns := prog.Flow().Authored().Storage().Assigns()
	for index := 0; index < assigns.Count(); index++ {
		assign, assignOK := assigns.At(index)
		_, values, rowOK := assigns.Get(assign)
		if !assignOK || !rowOK || !valuesContains(prog, values, value) {
			continue
		}
		for writeIndex := 0; writeIndex < writes.Count(); writeIndex++ {
			write, writeOK := writes.At(writeIndex)
			owner, target, writeRowOK := writes.Get(write)
			if writeOK && writeRowOK && owner == assign && target != 0 {
				return target, true
			}
		}
	}
	return 0, false
}

func valuesContains(prog *program.Program, values, want keyspace.Term) bool {
	if values == want {
		return true
	}
	if member, ok := prog.Flow().Authored().Values().Member(values, 0); ok && member == want {
		return true
	}
	pos, ok := prog.Flow().Authored().Values().Position(values, 0)
	return ok && (pos.Fixed == want || pos.Tail == want)
}

func caseCells(prog *program.Program, actuals keyspace.Term) map[int]keyspace.Term {
	first, ok := prog.Flow().Authored().Values().Member(actuals, 0)
	if !ok || keyspace.TermFamily(first) != keyspace.FamilyTable {
		return nil
	}
	tables := prog.Flow().Authored().Tables()
	fields := prog.Flow().Authored().Fields()
	count, countOK := tables.FieldCount(first)
	if !countOK {
		return nil
	}
	cells := make(map[int]keyspace.Term)
	listOrdinal := 0
	for index := 0; index < count; index++ {
		field, fieldOK := tables.FieldAt(first, index)
		if !fieldOK {
			continue
		}
		_, _, values, fieldKind, fieldRowOK := fields.Get(field)
		if !fieldRowOK || fieldKind != flowkind.FieldList {
			continue
		}
		value, valueOK := valuesAt(prog, values, 0)
		if !valueOK || keyspace.TermFamily(value) != keyspace.FamilyCall {
			listOrdinal++
			continue
		}
		name, named := prog.Source().Spellings().CallName(value)
		_, _, receiver, _, rowOK := prog.Flow().Authored().Calls().Get(value)
		if !named || !rowOK || (name != caseReceiveMethod && name != caseSendMethod) || receiver == 0 {
			listOrdinal++
			continue
		}
		cell, cellOK := cellOf(prog, receiver, 0)
		if cellOK {
			cells[listOrdinal] = cell
		}
		listOrdinal++
	}
	return cells
}

func channelChain(prog *program.Program, resultCell keyspace.Term, cells map[int]keyspace.Term) (keyspace.Term, []int, map[int]string, bool, bool) {
	branches := prog.Flow().Authored().Control().Branches()
	matching := make(map[keyspace.Term]int)
	var order []keyspace.Term
	for index := 0; index < branches.Count(); index++ {
		branch, branchOK := branches.At(index)
		_, condition, _, _, rowOK := branches.Get(branch)
		if !branchOK || !rowOK {
			continue
		}
		channel, ok := channelEqCell(prog, condition, resultCell)
		if !ok {
			continue
		}
		ordinal, named := ordinalOfCell(cells, channel)
		if !named {
			continue
		}
		matching[branch] = ordinal
		order = append(order, branch)
	}
	if len(order) == 0 {
		return 0, nil, nil, false, false
	}
	successor := make(map[keyspace.Term]keyspace.Term)
	referenced := make(map[keyspace.Term]struct{})
	covering := make(map[keyspace.Term]struct{})
	for branch := range matching {
		_, _, _, whenFalse, ok := branches.Get(branch)
		if !ok {
			continue
		}
		next, hasElse := chainSuccessor(prog, whenFalse, matching)
		if next != 0 {
			successor[branch] = next
			referenced[next] = struct{}{}
			continue
		}
		if hasElse {
			covering[branch] = struct{}{}
		}
	}
	var head keyspace.Term
	for _, branch := range order {
		if _, used := referenced[branch]; !used {
			head = branch
			break
		}
	}
	if head == 0 {
		return 0, nil, nil, false, false
	}
	var handled []int
	names := make(map[int]string)
	seen := make(map[keyspace.Term]struct{})
	last := head
	for branch := head; branch != 0; {
		if _, loop := seen[branch]; loop {
			return 0, nil, nil, false, false
		}
		seen[branch] = struct{}{}
		last = branch
		ordinal := matching[branch]
		handled = append(handled, ordinal)
		if name, named := prog.Source().Spellings().CellName(cells[ordinal]); named {
			names[ordinal] = name
		}
		next, chained := successor[branch]
		if !chained {
			break
		}
		branch = next
	}
	_, elseDefault := covering[last]
	return head, handled, names, elseDefault, true
}

func chainSuccessor(prog *program.Program, whenFalse keyspace.Term, matching map[keyspace.Term]int) (keyspace.Term, bool) {
	if whenFalse == 0 {
		return 0, false
	}
	if _, ok := matching[whenFalse]; ok {
		return whenFalse, false
	}
	if keyspace.TermFamily(whenFalse) != keyspace.FamilyBody {
		return 0, true
	}
	count, countOK := prog.Source().Order().BodyLen(whenFalse)
	if !countOK || count == 0 {
		return 0, false
	}
	first, firstOK := prog.Source().Order().BodyAt(whenFalse, 0)
	if firstOK {
		if _, ok := matching[first]; ok {
			return first, false
		}
	}
	return 0, true
}

func ordinalOfCell(cells map[int]keyspace.Term, cell keyspace.Term) (int, bool) {
	for ordinal, candidate := range cells {
		if candidate == cell {
			return ordinal, true
		}
	}
	return 0, false
}

func channelEqCell(prog *program.Program, condition, resultCell keyspace.Term) (keyspace.Term, bool) {
	condition = unwrapRead(prog, condition, 0)
	if keyspace.TermFamily(condition) != keyspace.FamilyBinary {
		return 0, false
	}
	_, op, left, right, ok := prog.Flow().Authored().Operators().Binaries().Get(condition)
	if !ok || op != flowkind.BinaryEqual {
		return 0, false
	}
	if _, match := fieldChannel(prog, left, resultCell); match {
		return cellOf(prog, right, 0)
	}
	if _, match := fieldChannel(prog, right, resultCell); match {
		return cellOf(prog, left, 0)
	}
	return 0, false
}

func fieldChannel(prog *program.Program, term, resultCell keyspace.Term) (keyspace.Term, bool) {
	term = unwrapRead(prog, term, 0)
	if keyspace.TermFamily(term) != keyspace.FamilyLensExact {
		return 0, false
	}
	_, base, source, fieldKind, ok := prog.Flow().Authored().Access().Exact().Get(term)
	if !ok || fieldKind != flowkind.FieldName {
		return 0, false
	}
	name, named := fieldName(prog, source)
	if !named || name != channelselect.ResultChannelField {
		return 0, false
	}
	baseCell, baseOK := cellOf(prog, base, 0)
	if !baseOK || baseCell != resultCell {
		return 0, false
	}
	return resultCell, true
}

func fieldName(prog *program.Program, key keyspace.Term) (string, bool) {
	_, name, _, ok := prog.Source().Keys().Name(key)
	if ok && name != "" {
		return name, true
	}
	value, exact := prog.Source().Keys().Exact(keyspace.Key(key))
	return value.String, exact && value.Kind == keyspace.LiteralString && value.String != ""
}

func unwrapRead(prog *program.Program, term keyspace.Term, depth int) keyspace.Term {
	if prog == nil || term == 0 || depth > walkLimit {
		return term
	}
	if keyspace.TermFamily(term) != keyspace.FamilyRead {
		return term
	}
	_, source, _, ok := prog.Flow().Authored().Storage().Reads().Get(term)
	if !ok {
		return term
	}
	return unwrapRead(prog, source, depth+1)
}
