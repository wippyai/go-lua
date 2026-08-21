package compiler

// Module rows are a compiler-owned projection of the authored Flow import
// and chunk-entry relations.  The old imports component was a useful
// semantic reference while this projection was being moved, but it is not an
// input here: Source, authored Flow, and Static are the only owner views used
// by this pass.

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/artifact/compiler/internal/rowidentity"
	"github.com/wippyai/go-lua/analysis/program/flow"
	flowauthored "github.com/wippyai/go-lua/analysis/program/flow/authored"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/denominator"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
	programconstruction "github.com/wippyai/go-lua/analysis/schema/program/construction"
	"github.com/wippyai/go-lua/analysis/schema/program/heapallocation"
)

// copyModuleRowsFailure compiles both Module families in their authored
// order.  The import pass intentionally repeats the old semantic validation
// at this owner boundary: a malformed authored Import, Call, Values, Read,
// Cell, Source String, or exact Key fails closed rather than becoming a
// partially useful publication row.
func (compiler *compiler) copyModuleRowsFailure() programconstruction.Fault {
	if compiler == nil || compiler.input == nil || !compiler.input.Available() {
		return programconstruction.New(programcatalog.ModuleEntry(), programconstruction.IssueModuleUnavailable, -1, -1)
	}
	sourceView := compiler.input.Source()
	flowView := compiler.input.Flow()
	staticView := compiler.input.Static()
	sourceID := sourceView.Identity().ContentID()
	flowID := flowView.ContentID()
	staticID := staticView.ContentID()
	authored := flowView.Authored()
	imports := authored.Imports()
	moduleID := imports.ContentID()
	provenance := flowView.Provenance()
	if !sourceID.Available() || !flowID.Available() || !staticID.Available() ||
		!moduleID.Available() || !provenance.Available() || provenance.Source != sourceID ||
		provenance.Flow != flowID || provenance.Static != staticID || provenance.Module != moduleID {
		return programconstruction.New(programcatalog.ModuleEntry(), programconstruction.IssueModuleUnavailable, -1, -1)
	}
	if imports.Count() != sourceView.Identity().FamilyCount(keyspace.FamilyImport) {
		return programconstruction.New(programcatalog.ModuleImport(), programconstruction.IssueModuleImport, -1, -1)
	}

	compiler.publication.ModuleImports = make([]programschema.ModuleImport, 0, imports.Count())
	compiler.publication.ModuleRequests = make([]programschema.ModuleRequest, 0, imports.Count())
	compiler.publication.ModuleEntries = compiler.publication.ModuleEntries[:0]
	compiler.publication.ModuleEntryRootCells = compiler.publication.ModuleEntryRootCells[:0]
	compiler.publication.ModuleEntryRootFunctions = compiler.publication.ModuleEntryRootFunctions[:0]
	compiler.publication.ModuleEntryMembers = compiler.publication.ModuleEntryMembers[:0]

	values := authored.Values()
	reads := authored.Storage().Reads()
	cells := authored.Storage().Cells()
	strings := sourceView.Literals().Strings()
	keys := sourceView.Keys()
	seenCalls := make(map[keyspace.Term]struct{}, authored.Calls().Count())
	for index := 0; index < imports.Count(); index++ {
		importTerm, importTermOK := imports.At(index)
		row, rowOK := imports.ImportAt(index)
		if !importTermOK || !rowOK || row.Term != importTerm ||
			keyspace.TermFamily(importTerm) != keyspace.FamilyImport ||
			keyspace.TermOrdinal(importTerm) == 0 ||
			keyspace.TermFamily(row.Call) != keyspace.FamilyCall ||
			keyspace.TermOrdinal(row.Call) == 0 ||
			(row.Alias != 0 && (keyspace.TermFamily(row.Alias) != keyspace.FamilyCell || keyspace.TermOrdinal(row.Alias) == 0)) ||
			keyspace.TermFamily(row.Request) != keyspace.FamilyString ||
			keyspace.TermOrdinal(row.Request) == 0 {
			return programconstruction.New(programcatalog.ModuleImport(), programconstruction.IssueModuleImport, index, -1)
		}
		if _, duplicate := seenCalls[row.Call]; duplicate {
			return programconstruction.New(programcatalog.ModuleImport(), programconstruction.IssueModuleImport, index, -1)
		}
		seenCalls[row.Call] = struct{}{}
		callOrdinal := keyspace.TermOrdinal(row.Call)
		callTerm, callTermOK := authored.Calls().At(int(callOrdinal - 1))
		construction, constructionOK := compiler.callConstruction(int(callOrdinal - 1))
		owner, callee, receiver, actuals, callOK := authored.Calls().Get(row.Call)
		if !callTermOK || callTerm != row.Call || !constructionOK || !construction.id.Available() || !callOK ||
			keyspace.TermFamily(owner) != keyspace.FamilyBody || keyspace.TermOrdinal(owner) == 0 ||
			receiver != 0 || keyspace.TermFamily(callee) != keyspace.FamilyRead ||
			keyspace.TermOrdinal(callee) == 0 || keyspace.TermFamily(actuals) != keyspace.FamilyValues ||
			keyspace.TermOrdinal(actuals) == 0 {
			return programconstruction.New(programcatalog.ModuleImport(), programconstruction.IssueModuleImport, index, -1)
		}
		actualOwner, tail, valuesOK := values.Get(actuals)
		length, lengthOK := values.Len(actuals)
		position, positionOK := values.Position(actuals, 0)
		if !valuesOK || actualOwner != owner || tail != 0 || !lengthOK || length != 1 ||
			!positionOK || position.Fixed != row.Request ||
			keyspace.TermFamily(position.Fixed) != keyspace.FamilyString || keyspace.TermOrdinal(position.Fixed) == 0 {
			return programconstruction.New(programcatalog.ModuleImport(), programconstruction.IssueModuleImport, index, -1)
		}
		readOwner, readSource, _, readOK := reads.Get(callee)
		if !readOK || readOwner != owner || keyspace.TermFamily(readSource) != keyspace.FamilyCell || keyspace.TermOrdinal(readSource) == 0 {
			return programconstruction.New(programcatalog.ModuleImport(), programconstruction.IssueModuleImport, index, -1)
		}
		cellKind, cellBody, cellKey, cellOK := cells.Get(readSource)
		if !cellOK || cellKind != flowauthored.CellGlobal || cellBody != 0 || cellKey == 0 {
			return programconstruction.New(programcatalog.ModuleImport(), programconstruction.IssueModuleImport, index, -1)
		}
		atom, atomOK := keys.Exact(cellKey)
		if !atomOK || atom != (keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "require"}) {
			return programconstruction.New(programcatalog.ModuleImport(), programconstruction.IssueModuleImport, index, -1)
		}
		requestOrdinal := keyspace.TermOrdinal(row.Request)
		if requestOrdinal == 0 || requestOrdinal > uint32(strings.Count()) {
			return programconstruction.New(programcatalog.ModuleRequest(), programconstruction.IssueModuleRequest, index, -1)
		}
		requestTerm, requestOwner, text, requestOK := strings.At(int(requestOrdinal - 1))
		if !requestOK || requestTerm != row.Request || requestOwner != owner || text == "" {
			return programconstruction.New(programcatalog.ModuleRequest(), programconstruction.IssueModuleRequest, index, -1)
		}
		requestKey, requestKeyOK := keys.Find(keyspace.LiteralValue{Kind: keyspace.LiteralString, String: text})
		if !requestKeyOK || requestKey == 0 {
			return programconstruction.New(programcatalog.ModuleRequest(), programconstruction.IssueModuleRequest, index, -1)
		}
		valueIndex := int(requestOrdinal) - 1
		valueSource, valueSourceOK := compiler.valueSourceAt(5, valueIndex)
		if !valueSourceOK || valueSource.term != row.Request || valueSource.id == (identity.ContentID{}) || !valueSource.id.Available() {
			return programconstruction.New(programcatalog.ModuleRequest(), programconstruction.IssueModuleRequest, index, -1)
		}
		importID, importIDOK := flowView.SemanticTermPath(importTerm)
		if !importIDOK || !importID.Available() {
			return programconstruction.New(programcatalog.ModuleImport(), programconstruction.IssueModuleImport, index, -1)
		}
		aliasID := identity.ContentID{}
		hasAlias := row.Alias != 0
		if hasAlias {
			var aliasOK bool
			aliasID, aliasOK = rowidentity.StorageCellID(compiler.key.ProgramID(), flowView, row.Alias)
			if !aliasOK || !aliasID.Available() {
				return programconstruction.New(programcatalog.ModuleImport(), programconstruction.IssueModuleImport, index, -1)
			}
		}
		if !fitsUint32(len(compiler.publication.ModuleRequests)) {
			return programconstruction.New(programcatalog.ModuleRequest(), programconstruction.IssueModuleRequest, index, -1)
		}
		requestRow, requestRowOK := programschema.NewModuleRequest(valueSource.id, importID, valueSource.id, requestKey)
		if !requestRowOK {
			return programconstruction.New(programcatalog.ModuleRequest(), programconstruction.IssueModuleRequest, index, -1)
		}
		importRow, importRowOK := programschema.NewModuleImport(importID, construction.id, aliasID, uint32(len(compiler.publication.ModuleRequests)), 1, hasAlias)
		if !importRowOK {
			return programconstruction.New(programcatalog.ModuleImport(), programconstruction.IssueModuleImport, index, -1)
		}
		compiler.publication.ModuleRequests = append(compiler.publication.ModuleRequests, requestRow)
		compiler.publication.ModuleImports = append(compiler.publication.ModuleImports, importRow)
	}

	if failure := compiler.copyModuleEntriesFailure(); failure.Available() {
		return failure
	}
	if !compiler.installModuleCounts() {
		return programconstruction.New(programcatalog.ModuleEntry(), programconstruction.IssueModuleUnavailable, -1, -1)
	}
	return programconstruction.Fault{}
}

// installModuleCounts replaces any stale authored-module contribution in the
// Program count set with the exact six canonical publication planes emitted by
// this compiler.  Program no longer owns a Module component, so this is the
// first point at which those derived denominator cardinalities exist.
func (compiler *compiler) installModuleCounts() bool {
	if compiler == nil || !compiler.counts.Available() {
		return false
	}
	ids := denominator.GeneratedProgramModuleIDs()
	counts := []struct {
		id    schema.EntryID
		value int
	}{
		{ids.ProgramModuleImport, len(compiler.publication.ModuleImports)},
		{ids.ProgramModuleRequest, len(compiler.publication.ModuleRequests)},
		{ids.ProgramModuleEntry, len(compiler.publication.ModuleEntries)},
		{ids.ProgramModuleEntryRootCell, len(compiler.publication.ModuleEntryRootCells)},
		{ids.ProgramModuleEntryMember, len(compiler.publication.ModuleEntryMembers)},
		{ids.ProgramModuleEntryRootFunction, len(compiler.publication.ModuleEntryRootFunctions)},
	}
	moduleRows := make([]denominator.CountRow, 0, len(counts))
	for _, count := range counts {
		if !fitsUint32(count.value) {
			return false
		}
		row, rowOK := denominator.NewCountRow(count.id, uint64(count.value))
		if !rowOK {
			return false
		}
		moduleRows = append(moduleRows, row)
	}
	moduleCounts, moduleOK := denominator.NewCountRows(moduleRows)
	merged, mergedOK := denominator.MergeCountRows(compiler.counts, moduleCounts)
	if !moduleOK || !mergedOK || !denominator.GeneratedCountRowsCompleteForOwners(merged,
		denominator.RelationOwnerProgramSource,
		denominator.RelationOwnerProgramFlow,
		denominator.RelationOwnerProgramStatic,
		denominator.RelationOwnerProgramModule,
	) {
		return false
	}
	compiler.counts = merged
	return true
}

func (compiler *compiler) copyModuleEntriesFailure() programconstruction.Fault {
	if compiler == nil || compiler.input == nil {
		return programconstruction.New(programcatalog.ModuleEntry(), programconstruction.IssueModuleUnavailable, -1, -1)
	}
	flowView := compiler.input.Flow()
	authored := flowView.Authored()
	bodies := flowView.Body()
	executable := flowView.Executable()
	directFunctions := flowView.DirectFunctions()
	if bodies == nil || executable == nil || directFunctions == nil {
		return programconstruction.New(programcatalog.ModuleEntry(), programconstruction.IssueModuleEntry, -1, -1)
	}
	entry, entryOK := bodies.Entry()
	if !entryOK || keyspace.TermFamily(entry) != keyspace.FamilyBody || keyspace.TermOrdinal(entry) == 0 {
		return programconstruction.New(programcatalog.ModuleEntry(), programconstruction.IssueModuleEntry, -1, -1)
	}
	if parent, hasParent := bodies.Parent(entry); hasParent || parent != 0 {
		return programconstruction.New(programcatalog.ModuleEntry(), programconstruction.IssueModuleEntry, -1, -1)
	}
	if activation, activationOK := bodies.Activation(entry); !activationOK || activation != 0 {
		return programconstruction.New(programcatalog.ModuleEntry(), programconstruction.IssueModuleEntry, -1, -1)
	}

	returns := authored.Control().Returns()
	fields := authored.Fields()
	returnCount := returns.Count()
	fieldCount := fields.Count()
	if !fitsUint32(returnCount) || !fitsUint32(fieldCount) {
		return programconstruction.New(programcatalog.ModuleEntry(), programconstruction.IssueModuleEntry, -1, -1)
	}
	for index := 0; index < returnCount; index++ {
		term, ok := returns.At(index)
		if !ok || term != keyspace.MakeTerm(keyspace.FamilyReturn, uint32(index+1)) {
			return programconstruction.New(programcatalog.ModuleEntry(), programconstruction.IssueModuleEntry, index, -1)
		}
	}
	for index := 0; index < fieldCount; index++ {
		term, ok := fields.At(index)
		if !ok || term != keyspace.MakeTerm(keyspace.FamilyTableField, uint32(index+1)) {
			return programconstruction.New(programcatalog.ModuleEntry(), programconstruction.IssueModuleEntry, index, -1)
		}
	}
	exactCount := compiler.input.Source().Keys().ExactCount()
	if exactCount < 0 || !fitsUint32(exactCount) {
		return programconstruction.New(programcatalog.ModuleEntry(), programconstruction.IssueModuleEntry, -1, -1)
	}
	keyScratch := moduleKeyScratch{marks: make([]uint32, exactCount+1)}
	if len(keyScratch.marks) == 0 {
		return programconstruction.New(programcatalog.ModuleEntry(), programconstruction.IssueModuleEntry, -1, -1)
	}
	for index := 0; index < returnCount; index++ {
		returned := keyspace.MakeTerm(keyspace.FamilyReturn, uint32(index+1))
		owner, valuesTerm, returnOK := returns.Get(returned)
		if !returnOK || keyspace.TermFamily(owner) != keyspace.FamilyBody || keyspace.TermOrdinal(owner) == 0 ||
			keyspace.TermFamily(valuesTerm) != keyspace.FamilyValues || keyspace.TermOrdinal(valuesTerm) == 0 {
			return programconstruction.New(programcatalog.ModuleEntry(), programconstruction.IssueModuleEntry, index, -1)
		}
		ownerActivation, ownerActivationOK := bodies.Activation(owner)
		if !ownerActivationOK {
			return programconstruction.New(programcatalog.ModuleEntry(), programconstruction.IssueModuleEntry, index, -1)
		}
		if ownerActivation != 0 || !executable.Contains(returned) {
			continue
		}
		valuesOwner, tail, valuesOwnerOK := authored.Values().Get(valuesTerm)
		fixedCount, fixedOK := authored.Values().Len(valuesTerm)
		if !valuesOwnerOK || valuesOwner != owner || tail != 0 || !fixedOK || fixedCount < 0 || !fitsUint32(fixedCount) {
			return programconstruction.New(programcatalog.ModuleEntry(), programconstruction.IssueModuleEntry, index, -1)
		}
		// The entry identity names the authored Return occurrence. ReturnID
		// instead joins the root activation's canonical Return Outcome. A
		// Return in a nested root-activation Body first reaches a local Outcome
		// and then propagates here; the Module plane must not expose that
		// intermediate boundary. More than one authored Return may therefore
		// share one ReturnID without collapsing their ordered entry rows.
		entryID, entryIDOK := flowView.SemanticTermPath(returned)
		returnOutcome, _, returnOutcomeOK := flowView.ReturnProjection().ForBody(entry)
		returnedID, returnedIDOK := flowView.SemanticTermPath(returnOutcome)
		if !entryIDOK || !entryID.Available() || !returnOutcomeOK ||
			keyspace.TermFamily(returnOutcome) != keyspace.FamilyOutcome ||
			!returnedIDOK || !returnedID.Available() {
			return programconstruction.New(programcatalog.ModuleEntry(), programconstruction.IssueModuleEntry, index, -1)
		}
		rootCellOffset, rootFunctionOffset, memberOffset := len(compiler.publication.ModuleEntryRootCells), len(compiler.publication.ModuleEntryRootFunctions), len(compiler.publication.ModuleEntryMembers)
		for position := 0; position < fixedCount; position++ {
			value, valueOK := authored.Values().Member(valuesTerm, position)
			if !valueOK || !fitsUint32(position) {
				return programconstruction.New(programcatalog.ModuleEntry(), programconstruction.IssueModuleEntry, index, position)
			}
			memberID, memberIDOK := flowView.ValuesMemberID(valuesTerm, position)
			if !memberIDOK || !memberID.Available() {
				if _, functionOK := moduleEntryDirectFunction(value, executable, directFunctions); functionOK {
					return programconstruction.New(programcatalog.ModuleEntryRootFunction(), programconstruction.IssueModuleRootFunction, index, position)
				}
				if _, cellOK := moduleEntryDirectCell(flowView, value); cellOK {
					return programconstruction.New(programcatalog.ModuleEntryRootCell(), programconstruction.IssueModuleRootCell, index, position)
				}
			}
			if function, functionOK := moduleEntryDirectFunction(value, executable, directFunctions); functionOK {
				functionID, functionIDOK := compiler.moduleFunctionID(function)
				if !functionIDOK {
					return programconstruction.New(programcatalog.ModuleEntryRootFunction(), programconstruction.IssueModuleRootFunction, index, position)
				}
				child, childOK := programschema.NewModuleEntryRootFunction(memberID, entryID, functionID, uint32(position))
				if !childOK {
					return programconstruction.New(programcatalog.ModuleEntryRootFunction(), programconstruction.IssueModuleRootFunction, index, position)
				}
				compiler.publication.ModuleEntryRootFunctions = append(compiler.publication.ModuleEntryRootFunctions, child)
			}
			if cell, cellOK := moduleEntryDirectCell(flowView, value); cellOK {
				cellID, cellIDOK := rowidentity.StorageCellID(compiler.key.ProgramID(), flowView, cell)
				if !cellIDOK || !cellID.Available() {
					return programconstruction.New(programcatalog.ModuleEntryRootCell(), programconstruction.IssueModuleRootCell, index, position)
				}
				child, childOK := programschema.NewModuleEntryRootCell(memberID, entryID, cellID, uint32(position))
				if !childOK {
					return programconstruction.New(programcatalog.ModuleEntryRootCell(), programconstruction.IssueModuleRootCell, index, position)
				}
				compiler.publication.ModuleEntryRootCells = append(compiler.publication.ModuleEntryRootCells, child)
			}
			if keyspace.TermFamily(value) == keyspace.FamilyTable && executable.Contains(value) {
				if failure := compiler.appendModuleTable(value, value, entryID, uint32(position), &keyScratch); failure.Available() {
					return failure
				}
			}
		}
		if !fitsUint32(rootCellOffset) || !fitsUint32(rootFunctionOffset) || !fitsUint32(memberOffset) ||
			!fitsUint32(len(compiler.publication.ModuleEntryRootCells)-rootCellOffset) ||
			!fitsUint32(len(compiler.publication.ModuleEntryRootFunctions)-rootFunctionOffset) ||
			!fitsUint32(len(compiler.publication.ModuleEntryMembers)-memberOffset) {
			return programconstruction.New(programcatalog.ModuleEntry(), programconstruction.IssueModuleEntry, index, -1)
		}
		row, rowOK := programschema.NewModuleEntry(entryID, returnedID, uint32(index+1), uint32(fixedCount),
			uint32(rootCellOffset), uint32(len(compiler.publication.ModuleEntryRootCells)-rootCellOffset),
			uint32(rootFunctionOffset), uint32(len(compiler.publication.ModuleEntryRootFunctions)-rootFunctionOffset),
			uint32(memberOffset), uint32(len(compiler.publication.ModuleEntryMembers)-memberOffset))
		if !rowOK {
			return programconstruction.New(programcatalog.ModuleEntry(), programconstruction.IssueModuleEntry, index, -1)
		}
		compiler.publication.ModuleEntries = append(compiler.publication.ModuleEntries, row)
	}
	return programconstruction.Fault{}
}

func moduleEntryDirectFunction(value keyspace.Term, executable interface{ Contains(keyspace.Term) bool }, directFunctions interface {
	For(keyspace.Term) (keyspace.Term, bool)
}) (keyspace.Term, bool) {
	if executable == nil || directFunctions == nil || !executable.Contains(value) {
		return 0, false
	}
	function, ok := directFunctions.For(value)
	return function, ok && executable.Contains(function)
}

func moduleEntryDirectCell(flowView flow.View, value keyspace.Term) (keyspace.Term, bool) {
	if keyspace.TermFamily(value) != keyspace.FamilyRead {
		return 0, false
	}
	_, source, _, ok := flowView.Authored().Storage().Reads().Get(value)
	return source, ok && keyspace.TermFamily(source) == keyspace.FamilyCell && keyspace.TermOrdinal(source) != 0
}

func (compiler *compiler) moduleFunctionID(function keyspace.Term) (identity.ContentID, bool) {
	if compiler == nil || compiler.input == nil || keyspace.TermFamily(function) != keyspace.FamilyFunction || keyspace.TermOrdinal(function) == 0 {
		return identity.ContentID{}, false
	}
	if compiler.bodyBoundary == nil {
		return identity.ContentID{}, false
	}
	return compiler.bodyBoundary.FunctionID(function)
}

type moduleEntryField struct {
	field keyspace.Term
	value keyspace.Term
	key   keyspace.Key
}

type moduleKeyScratch struct {
	marks []uint32
	epoch uint32
}

func (scratch *moduleKeyScratch) next() (uint32, bool) {
	if scratch == nil || len(scratch.marks) == 0 {
		return 0, false
	}
	scratch.epoch++
	if scratch.epoch == 0 {
		for index := range scratch.marks {
			scratch.marks[index] = 0
		}
		scratch.epoch = 1
	}
	return scratch.epoch, true
}

func (compiler *compiler) selectModuleFields(table keyspace.Term, scratch *moduleKeyScratch) ([]moduleEntryField, bool) {
	if compiler == nil || scratch == nil || keyspace.TermFamily(table) != keyspace.FamilyTable || keyspace.TermOrdinal(table) == 0 {
		return nil, false
	}
	tables := compiler.input.Flow().Authored().Tables()
	fields := compiler.input.Flow().Authored().Fields()
	values := compiler.input.Flow().Authored().Values()
	fieldCount, ok := tables.FieldCount(table)
	if !ok || fieldCount < 0 || !fitsUint32(fieldCount) {
		return nil, false
	}
	epoch, ok := scratch.next()
	if !ok {
		return nil, false
	}
	selected := make([]moduleEntryField, 0, fieldCount)
	fenced := false
	sourceView := compiler.input.Source()
	for index := fieldCount - 1; index >= 0; index-- {
		field, fieldOK := tables.FieldAt(table, index)
		fieldTable, fieldKey, valuesTerm, fieldKind, rowOK := fields.Get(field)
		if !fieldOK || !rowOK || fieldTable != table || keyspace.TermFamily(field) != keyspace.FamilyTableField ||
			keyspace.TermOrdinal(field) == 0 || keyspace.TermFamily(valuesTerm) != keyspace.FamilyValues || keyspace.TermOrdinal(valuesTerm) == 0 {
			return nil, false
		}
		if fieldKind == flowkind.FieldKey {
			fenced = true
			continue
		}
		if fieldKind != flowkind.FieldName && fieldKind != flowkind.FieldExact {
			continue
		}
		key, exact := moduleEntryFieldKey(sourceView, fieldKind, fieldKey)
		if !exact || key == 0 || uint64(key) >= uint64(len(scratch.marks)) {
			continue
		}
		if scratch.marks[key] == epoch {
			continue
		}
		scratch.marks[key] = epoch
		if fenced {
			continue
		}
		fixed, fixedOK := values.Len(valuesTerm)
		_, tail, valuesOK := values.Get(valuesTerm)
		if !valuesOK || !fixedOK || fixed != 1 || tail != 0 {
			continue
		}
		value, valueOK := values.Member(valuesTerm, 0)
		if !valueOK {
			return nil, false
		}
		selected = append(selected, moduleEntryField{field: field, value: value, key: key})
	}
	for left, right := 0, len(selected)-1; left < right; left, right = left+1, right-1 {
		selected[left], selected[right] = selected[right], selected[left]
	}
	return selected, true
}

type moduleTableFrame struct {
	table       keyspace.Term
	parent      keyspace.Term
	selected    []moduleEntryField
	next        int
	containerAt int
}

func (compiler *compiler) appendModuleTable(table, parent keyspace.Term, entryID identity.ContentID, rootPosition uint32, scratch *moduleKeyScratch) programconstruction.Fault {
	selected, ok := compiler.selectModuleFields(table, scratch)
	if !ok {
		return programconstruction.New(programcatalog.ModuleEntryMember(), programconstruction.IssueModuleMember, -1, -1)
	}
	stack := []moduleTableFrame{{table: table, parent: parent, selected: selected, containerAt: -1}}
	for len(stack) != 0 {
		at := len(stack) - 1
		frame := &stack[at]
		if frame.next >= len(frame.selected) {
			if frame.containerAt >= 0 && len(compiler.publication.ModuleEntryMembers) == frame.containerAt+1 {
				compiler.publication.ModuleEntryMembers = compiler.publication.ModuleEntryMembers[:frame.containerAt]
			}
			stack = stack[:at]
			continue
		}
		candidate := frame.selected[frame.next]
		frame.next++
		fieldID, fieldOK := compiler.moduleFieldID(candidate.field)
		tableID, tableOK := compiler.moduleTableID(frame.table)
		parentID, parentOK := compiler.moduleTableOrFieldID(frame.parent)
		if !fieldOK || !tableOK || !parentOK {
			return programconstruction.New(programcatalog.ModuleEntryMember(), programconstruction.IssueModuleMember, -1, -1)
		}
		function, functionOK := moduleEntryDirectFunction(candidate.value, compiler.input.Flow().Executable(), compiler.input.Flow().DirectFunctions())
		if functionOK {
			valueID, valueOK := compiler.moduleFunctionID(function)
			if !valueOK {
				return programconstruction.New(programcatalog.ModuleEntryMember(), programconstruction.IssueModuleMember, -1, -1)
			}
			member, memberOK := programschema.NewModuleEntryMember(fieldID, fieldID, parentID, valueID, entryID, tableID, candidate.key, rootPosition, true)
			if !memberOK {
				return programconstruction.New(programcatalog.ModuleEntryMember(), programconstruction.IssueModuleMember, -1, -1)
			}
			compiler.publication.ModuleEntryMembers = append(compiler.publication.ModuleEntryMembers, member)
			continue
		}
		if keyspace.TermFamily(candidate.value) != keyspace.FamilyTable || !compiler.input.Flow().Executable().Contains(candidate.value) {
			continue
		}
		nested, nestedOK := compiler.selectModuleFields(candidate.value, scratch)
		if !nestedOK {
			return programconstruction.New(programcatalog.ModuleEntryMember(), programconstruction.IssueModuleMember, -1, -1)
		}
		member, memberOK := programschema.NewModuleEntryMember(fieldID, fieldID, parentID, identity.ContentID{}, entryID, tableID, candidate.key, rootPosition, false)
		if !memberOK {
			return programconstruction.New(programcatalog.ModuleEntryMember(), programconstruction.IssueModuleMember, -1, -1)
		}
		compiler.publication.ModuleEntryMembers = append(compiler.publication.ModuleEntryMembers, member)
		stack = append(stack, moduleTableFrame{table: candidate.value, parent: candidate.field, selected: nested, containerAt: len(compiler.publication.ModuleEntryMembers) - 1})
	}
	return programconstruction.Fault{}
}

func (compiler *compiler) moduleTableID(table keyspace.Term) (identity.ContentID, bool) {
	if compiler == nil || compiler.allocations == nil {
		return identity.ContentID{}, false
	}
	row, rowOK := compiler.allocations.ForTerm(table)
	role, roleOK := row.Role()
	template, templateOK := row.Template()
	return template, rowOK && roleOK && role == heapallocation.RoleTable && templateOK
}

func (compiler *compiler) moduleFieldID(field keyspace.Term) (identity.ContentID, bool) {
	if compiler == nil || compiler.allocations == nil || field == 0 {
		return identity.ContentID{}, false
	}
	return compiler.allocations.FieldID(field)
}

func (compiler *compiler) moduleTableOrFieldID(term keyspace.Term) (identity.ContentID, bool) {
	if keyspace.TermFamily(term) == keyspace.FamilyTable {
		return compiler.moduleTableID(term)
	}
	if keyspace.TermFamily(term) == keyspace.FamilyTableField {
		return compiler.moduleFieldID(term)
	}
	return identity.ContentID{}, false
}

func moduleEntryFieldKey(sourceView source.View, fieldKind flowkind.FieldKind, term keyspace.Term) (keyspace.Key, bool) {
	keys := sourceView.Keys()
	switch fieldKind {
	case flowkind.FieldName:
		_, _, key, ok := keys.Name(term)
		if !ok || key == 0 {
			return 0, false
		}
		atom, atomOK := keys.Exact(key)
		return key, atomOK && atom.Kind == keyspace.LiteralString
	case flowkind.FieldExact:
		ordinal := keyspace.TermOrdinal(term)
		if keyspace.TermFamily(term) != keyspace.FamilyString || ordinal == 0 {
			return 0, false
		}
		observed, _, value, ok := sourceView.Literals().Strings().At(int(ordinal - 1))
		if !ok || observed != term {
			return 0, false
		}
		return keys.Find(keyspace.LiteralValue{Kind: keyspace.LiteralString, String: value})
	default:
		return 0, false
	}
}
