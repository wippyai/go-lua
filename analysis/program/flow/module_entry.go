// Package flow contains the top-level assembly-local Flow projections.
//
// This file is deliberately private to the Flow assembler.  Module entry is
// not a fourth semantic owner and it is not an intermediate graph: the
// assembler gives this helper committed Source/Flow views and sealed proof
// projections, and receives one transient imports.CommitInput to pass directly
// to the Module finalizer.
package flow

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/body"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/directfunction"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/executable"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/imports"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

// sealModuleEntry derives the chunk-entry projection and the Module import
// resolutions from the one set of already-sealed owners.  staticID is passed
// explicitly because the Executable and DirectFunction proofs retain only
// their four scalar provenance fences; no Static owner or composite build ID
// is retained here.
//
// The returned input is assembly scratch.  It must be consumed immediately by
// imports.Finalizer.Commit; this helper never returns a Module Result, stores a
// string-key table, or exposes a second entry authority.
func sealModuleEntry(
	sourceView source.View,
	flowView authored.View,
	moduleView imports.View,
	bodies *body.Result,
	executableResult *executable.Result,
	directFunctions *directfunction.Result,
	staticID identity.ContentID,
	entry keyspace.Term,
) (imports.CommitInput, error) {
	var empty imports.CommitInput
	sourceID := sourceView.Identity().ContentID()
	flowID := flowView.Cold().ContentID()
	moduleID := moduleView.ContentID()
	if !sourceID.Available() || !flowID.Available() || !moduleID.Available() || !staticID.Available() {
		return empty, errors.New("program/flow: module entry owner identity is unavailable")
	}
	if !body.Matches(bodies, sourceID, flowID) {
		return empty, errors.New("program/flow: module entry Body provenance disagrees")
	}
	if !executable.Matches(executableResult, sourceID, flowID, staticID, moduleID) {
		return empty, errors.New("program/flow: module entry Executable provenance disagrees")
	}
	if !directfunction.Matches(directFunctions, sourceID, flowID, staticID, moduleID) {
		return empty, errors.New("program/flow: module entry DirectFunction provenance disagrees")
	}

	identity := sourceView.Identity()
	if identity.FamilyCount(keyspace.FamilyImport) != moduleView.Count() {
		return empty, errors.New("program/flow: module entry Import denominator disagrees")
	}
	if keyspace.TermFamily(entry) != keyspace.FamilyBody || keyspace.TermOrdinal(entry) == 0 ||
		uint64(keyspace.TermOrdinal(entry)) > uint64(bodies.BodyCount()) {
		return empty, errors.New("program/flow: module entry is not a Body")
	}
	if parent, hasParent := bodies.Parent(entry); hasParent || parent != 0 {
		return empty, errors.New("program/flow: module entry Body has a parent")
	}
	if activation, ok := bodies.Activation(entry); !ok || activation != 0 {
		return empty, errors.New("program/flow: module entry Body is not the chunk activation")
	}

	resolutions, err := sealModuleImportResolutions(sourceView, flowView, moduleView)
	if err != nil {
		return empty, err
	}
	entryInput, err := sealChunkEntry(sourceView, flowView, bodies, executableResult, directFunctions, entry)
	if err != nil {
		return empty, err
	}
	return imports.CommitInput{Resolutions: resolutions, Entry: entryInput}, nil
}

// sealModuleImportResolutions cross-validates each authored Module Import and
// derives only its Source-owned Key. Request is already part of the authored
// row; this pass never discovers or overwrites it from generic Call/Values
// shape. Static typeof calls carry an ordinary (non-implicit) Read and are
// valid alongside runtime implicit Reads.
func sealModuleImportResolutions(
	sourceView source.View,
	flowView authored.View,
	moduleView imports.View,
) ([]imports.Resolution, error) {
	resolutions := make([]imports.Resolution, moduleView.Count())
	if moduleView.Count() == 0 {
		return resolutions, nil
	}
	calls := flowView.Calls()
	callCount := calls.Count()
	if callCount < 0 || !keyspace.TermOrdinalFits(callCount) {
		return nil, errors.New("program/flow: Module Import Call denominator is unavailable")
	}
	seenCalls := make([]bool, callCount+1)
	values := flowView.Values()
	reads := flowView.Storage().Reads()
	cells := flowView.Storage().Cells()
	strings := sourceView.Literals().Strings()
	keys := sourceView.Keys()
	for index := 0; index < moduleView.Count(); index++ {
		importTerm := keyspace.MakeTerm(keyspace.FamilyImport, uint32(index+1))
		row, ok := moduleView.Import(importTerm)
		if !ok || row.Term != importTerm || keyspace.TermFamily(row.Call) != keyspace.FamilyCall || keyspace.TermOrdinal(row.Call) == 0 {
			return nil, errors.New("program/flow: malformed Module Import")
		}
		callOrdinal := keyspace.TermOrdinal(row.Call)
		if uint64(callOrdinal) > uint64(callCount) {
			return nil, errors.New("program/flow: Module Import Call is outside Flow")
		}
		if seenCalls[callOrdinal] {
			return nil, errors.New("program/flow: duplicate Module Import Call")
		}
		seenCalls[callOrdinal] = true
		if row.Request == 0 || keyspace.TermFamily(row.Request) != keyspace.FamilyString || keyspace.TermOrdinal(row.Request) == 0 || row.Key != 0 {
			return nil, errors.New("program/flow: Module Import authored Request is malformed")
		}
		owner, callee, receiver, actuals, ok := calls.Get(row.Call)
		if !ok || keyspace.TermFamily(owner) != keyspace.FamilyBody || receiver != 0 ||
			keyspace.TermFamily(callee) != keyspace.FamilyRead || keyspace.TermOrdinal(callee) == 0 ||
			keyspace.TermFamily(actuals) != keyspace.FamilyValues || keyspace.TermOrdinal(actuals) == 0 {
			return nil, errors.New("program/flow: Module Import is not a plain require Read Call")
		}
		actualOwner, tail, valuesOK := values.Get(actuals)
		if !valuesOK || actualOwner != owner || tail != 0 {
			return nil, errors.New("program/flow: Module Import actual Values are not one fixed argument")
		}
		if length, lengthOK := values.Len(actuals); !lengthOK || length != 1 {
			return nil, errors.New("program/flow: Module Import actual Values are not exactly one fixed argument")
		}
		position, positionOK := values.Position(actuals, 0)
		if !positionOK || position.Fixed != row.Request || keyspace.TermFamily(position.Fixed) != keyspace.FamilyString || keyspace.TermOrdinal(position.Fixed) == 0 {
			return nil, errors.New("program/flow: Module Import Request disagrees with Call Values")
		}
		readOwner, readSource, _, readOK := reads.Get(callee)
		if !readOK || readOwner != owner || keyspace.TermFamily(readSource) != keyspace.FamilyCell || keyspace.TermOrdinal(readSource) == 0 {
			return nil, errors.New("program/flow: Module Import callee Read owner/source disagrees")
		}
		cellKind, cellBody, cellKey, cellOK := cells.Get(readSource)
		if !cellOK || cellKind != authored.CellGlobal || cellBody != 0 || cellKey == 0 {
			return nil, errors.New("program/flow: Module Import callee Read is not a global Cell")
		}
		atom, atomOK := keys.Exact(cellKey)
		if !atomOK || atom != (keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "require"}) {
			return nil, errors.New("program/flow: Module Import callee Read is not canonical require")
		}
		requestOrdinal := keyspace.TermOrdinal(row.Request)
		if requestOrdinal == 0 || requestOrdinal > uint32(strings.Count()) {
			return nil, errors.New("program/flow: Module Import Request is outside Source")
		}
		requestTerm, requestOwner, text, requestOK := strings.At(int(requestOrdinal - 1))
		if !requestOK || requestTerm != row.Request || requestOwner != owner {
			return nil, errors.New("program/flow: Module Import Request String owner disagrees")
		}
		if text == "" {
			return nil, errors.New("program/flow: Module Import Request String is empty")
		}
		key, keyOK := keys.Find(keyspace.LiteralValue{Kind: keyspace.LiteralString, String: text})
		if !keyOK || key == 0 {
			return nil, errors.New("program/flow: Module Import authored Request has no Source exact key")
		}
		resolutions[index] = imports.Resolution{Request: row.Request, Key: key}
	}
	return resolutions, nil
}

// sealChunkEntry derives fixed Return slots and the exact table surface of
// every executable chunk Return.  The fixed slot width is preserved even
// when the direct Function proof is absent: slot position is part of the
// module boundary contract.
func sealChunkEntry(
	sourceView source.View,
	flowView authored.View,
	bodies *body.Result,
	executableResult *executable.Result,
	directFunctions *directfunction.Result,
	entry keyspace.Term,
) (imports.Entry, error) {
	returns := flowView.Control().Returns()
	values := flowView.Values()
	fields := flowView.Fields()
	returnCount := returns.Count()
	if returnCount < 0 || !keyspace.TermOrdinalFits(returnCount) || fields.Count() < 0 || !keyspace.TermOrdinalFits(fields.Count()) {
		return imports.Entry{}, errors.New("program/flow: module entry denominator is unavailable")
	}
	keyScratch, err := newEntryKeyScratch(sourceView)
	if err != nil {
		return imports.Entry{}, err
	}
	input := imports.Entry{
		ReturnIndex:  make([]uint32, returnCount+1),
		RootRanges:   make([]imports.EntryRange, returnCount+1),
		MemberRanges: make([]imports.EntryRange, returnCount+1),
		MemberIndex:  make([]uint32, fields.Count()+1),
	}

	// All three dense authored relations are checked before their rows are
	// consumed. This keeps malformed owner rows fail-closed instead of letting
	// a partial Entry become a second semantic authority.
	for index := 0; index < returnCount; index++ {
		term, ok := returns.At(index)
		if !ok || term != keyspace.MakeTerm(keyspace.FamilyReturn, uint32(index+1)) {
			return imports.Entry{}, errors.New("program/flow: malformed Return denominator")
		}
	}
	for index := 0; index < fields.Count(); index++ {
		term, ok := fields.At(index)
		if !ok || term != keyspace.MakeTerm(keyspace.FamilyTableField, uint32(index+1)) {
			return imports.Entry{}, errors.New("program/flow: malformed TableField denominator")
		}
	}

	for index := 0; index < returnCount; index++ {
		returned := keyspace.MakeTerm(keyspace.FamilyReturn, uint32(index+1))
		owner, valuesTerm, ok := returns.Get(returned)
		if !ok || keyspace.TermFamily(owner) != keyspace.FamilyBody || keyspace.TermOrdinal(owner) == 0 ||
			keyspace.TermFamily(valuesTerm) != keyspace.FamilyValues || keyspace.TermOrdinal(valuesTerm) == 0 {
			return imports.Entry{}, errors.New("program/flow: malformed Return row")
		}
		ownerActivation, ok := bodies.Activation(owner)
		if !ok {
			return imports.Entry{}, errors.New("program/flow: Return Body activation is unavailable")
		}
		if ownerActivation != 0 || !executableResult.Executable(returned) {
			continue
		}
		if valuesOwner, _, valuesOK := values.Get(valuesTerm); !valuesOK || valuesOwner != owner {
			return imports.Entry{}, errors.New("program/flow: Return Values owner disagrees")
		}

		fixedCount, ok := values.Len(valuesTerm)
		if !ok || fixedCount < 0 || !keyspace.TermOrdinalFits(fixedCount) {
			return imports.Entry{}, errors.New("program/flow: Return fixed Pack is unavailable")
		}
		start := uint32(len(input.Roots))
		memberStart := uint32(len(input.Members))
		for position := 0; position < fixedCount; position++ {
			value, ok := values.Member(valuesTerm, position)
			if !ok {
				return imports.Entry{}, errors.New("program/flow: Return fixed Pack position is unavailable")
			}
			input.Roots = append(input.Roots, entryDirectFunction(value, executableResult, directFunctions))
			input.RootCells = append(input.RootCells, entryDirectCell(flowView, value))
			if keyspace.TermFamily(value) == keyspace.FamilyTable && executableResult.Executable(value) {
				if err := appendEntryTable(sourceView, flowView, executableResult, directFunctions, value, value, returned, uint32(position), &keyScratch, &input); err != nil {
					return imports.Entry{}, err
				}
			}
		}
		input.RootRanges[index+1] = imports.EntryRange{Start: start, End: uint32(len(input.Roots))}
		input.MemberRanges[index+1] = imports.EntryRange{Start: memberStart, End: uint32(len(input.Members))}
		input.ReturnTerms = append(input.ReturnTerms, returned)
		input.ReturnIndex[index+1] = uint32(len(input.ReturnTerms))
	}
	return input, nil
}

// entryDirectFunction admits only an occurrence that is in the executable
// closure and has a DirectFunction witness. DirectFunction's standalone
// Function query is intentionally broader (it is identity-based even for a
// dead Function), so the executable fence belongs here at the boundary.
func entryDirectFunction(value keyspace.Term, executableResult *executable.Result, directFunctions *directfunction.Result) keyspace.Term {
	if executableResult == nil || directFunctions == nil || !executableResult.Executable(value) {
		return 0
	}
	function, ok := directFunctions.DirectFunction(value)
	if !ok || !executableResult.Executable(function) {
		return 0
	}
	return function
}

// entryDirectCell preserves only the immediate Read -> Cell witness. It
// deliberately does not follow aliases, lenses, assignments, or table paths.
func entryDirectCell(flowView authored.View, value keyspace.Term) keyspace.Term {
	if keyspace.TermFamily(value) != keyspace.FamilyRead {
		return 0
	}
	_, sourceTerm, _, ok := flowView.Storage().Reads().Get(value)
	if !ok || keyspace.TermFamily(sourceTerm) != keyspace.FamilyCell || keyspace.TermOrdinal(sourceTerm) == 0 {
		return 0
	}
	return sourceTerm
}

type entryField struct {
	field keyspace.Term
	value keyspace.Term
	key   keyspace.Key
}

// entryKeyScratch is the one exact-key duplicate plane for a complete chunk
// entry seal. Each Table advances the epoch instead of allocating or clearing
// a key-sized bitmap, so nested surfaces cost O(keys+fields) storage/work.
type entryKeyScratch struct {
	marks []uint32
	epoch uint32
}

func newEntryKeyScratch(sourceView source.View) (entryKeyScratch, error) {
	keyCount := sourceView.Keys().ExactCount()
	if keyCount < 0 || !keyspace.TermOrdinalFits(keyCount) {
		return entryKeyScratch{}, errors.New("program/flow: Source exact-key denominator is unavailable")
	}
	return entryKeyScratch{marks: make([]uint32, keyCount+1)}, nil
}

func (scratch *entryKeyScratch) next() (uint32, bool) {
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

type entryTableFrame struct {
	table       keyspace.Term
	parent      keyspace.Term
	selected    []entryField
	next        int
	containerAt int
}

// appendEntryTable is the iterative table-member derivation. Every selected
// field is a final exact Source-key write; a FieldKey row is a dynamic fence,
// and duplicate exact keys use the last authored write. Container rows are
// provisional and are removed when no exported descendant survives.
func appendEntryTable(
	sourceView source.View,
	flowView authored.View,
	executableResult *executable.Result,
	directFunctions *directfunction.Result,
	table keyspace.Term,
	parent keyspace.Term,
	returned keyspace.Term,
	rootOrdinal uint32,
	keyScratch *entryKeyScratch,
	input *imports.Entry,
) error {
	if keyScratch == nil || input == nil || keyspace.TermFamily(table) != keyspace.FamilyTable || keyspace.TermOrdinal(table) == 0 {
		return errors.New("program/flow: malformed module-entry Table")
	}
	selected, err := selectEntryFieldsWithScratch(sourceView, flowView, table, keyScratch)
	if err != nil {
		return err
	}
	stack := []entryTableFrame{{table: table, parent: parent, selected: selected, containerAt: -1}}
	for len(stack) != 0 {
		at := len(stack) - 1
		frame := &stack[at]
		if frame.next >= len(frame.selected) {
			if frame.containerAt >= 0 && len(input.Members) == frame.containerAt+1 {
				field := input.Members[frame.containerAt].Field
				input.Members = input.Members[:frame.containerAt]
				if ordinal := keyspace.TermOrdinal(field); ordinal < uint32(len(input.MemberIndex)) {
					input.MemberIndex[ordinal] = 0
				}
			}
			stack = stack[:at]
			continue
		}
		candidate := frame.selected[frame.next]
		frame.next++
		member := imports.EntryMember{
			Field: candidate.field, Parent: frame.parent, Returned: returned,
			Table: frame.table, Suffix: candidate.key, Ordinal: rootOrdinal,
		}
		if function := entryDirectFunction(candidate.value, executableResult, directFunctions); function != 0 {
			member.Value = function
			input.Members = append(input.Members, member)
			input.MemberIndex[keyspace.TermOrdinal(candidate.field)] = uint32(len(input.Members))
			continue
		}
		if keyspace.TermFamily(candidate.value) != keyspace.FamilyTable || !executableResult.Executable(candidate.value) {
			continue
		}
		nested, err := selectEntryFieldsWithScratch(sourceView, flowView, candidate.value, keyScratch)
		if err != nil {
			return err
		}
		input.Members = append(input.Members, member)
		input.MemberIndex[keyspace.TermOrdinal(candidate.field)] = uint32(len(input.Members))
		containerAt := len(input.Members) - 1
		stack = append(stack, entryTableFrame{
			table: candidate.value, parent: candidate.field, selected: nested,
			containerAt: containerAt,
		})
	}
	return nil
}

// selectEntryFields is the isolated-query wrapper used by focused laws. The
// full chunk seal constructs one scratch and calls selectEntryFieldsWithScratch
// for every root and nested Table.
func selectEntryFields(sourceView source.View, flowView authored.View, table keyspace.Term) ([]entryField, error) {
	scratch, err := newEntryKeyScratch(sourceView)
	if err != nil {
		return nil, err
	}
	return selectEntryFieldsWithScratch(sourceView, flowView, table, &scratch)
}

// selectEntryFieldsWithScratch walks one Table's authored field order
// backwards. Epoch marks are indexed by Source-owned Key handles, never by
// raw strings or a second key map.
func selectEntryFieldsWithScratch(
	sourceView source.View,
	flowView authored.View,
	table keyspace.Term,
	keyScratch *entryKeyScratch,
) ([]entryField, error) {
	tables := flowView.Tables()
	fields := flowView.Fields()
	fieldCount, ok := tables.FieldCount(table)
	if !ok || fieldCount < 0 || !keyspace.TermOrdinalFits(fieldCount) {
		return nil, errors.New("program/flow: Table field range is unavailable")
	}
	epoch, ok := keyScratch.next()
	if !ok {
		return nil, errors.New("program/flow: module-entry exact-key scratch is unavailable")
	}
	selected := make([]entryField, 0, fieldCount)
	fenced := false
	for index := fieldCount - 1; index >= 0; index-- {
		field, ok := tables.FieldAt(table, index)
		if !ok || keyspace.TermFamily(field) != keyspace.FamilyTableField || keyspace.TermOrdinal(field) == 0 {
			return nil, errors.New("program/flow: Table field order is unavailable")
		}
		fieldTable, fieldKey, valuesTerm, fieldKind, ok := fields.Get(field)
		if !ok || fieldTable != table || keyspace.TermFamily(valuesTerm) != keyspace.FamilyValues || keyspace.TermOrdinal(valuesTerm) == 0 {
			return nil, errors.New("program/flow: malformed TableField")
		}
		if fieldKind == kind.FieldKey {
			fenced = true
			continue
		}
		if fieldKind != kind.FieldName && fieldKind != kind.FieldExact {
			continue
		}
		key, exact := entryFieldKey(sourceView, fieldKind, fieldKey)
		if !exact || key == 0 || uint64(key) >= uint64(len(keyScratch.marks)) {
			continue
		}
		if keyScratch.marks[key] == epoch {
			continue
		}
		keyScratch.marks[key] = epoch
		if fenced {
			continue
		}
		values := flowView.Values()
		fixed, fixedOK := values.Len(valuesTerm)
		_, tail, valuesOK := values.Get(valuesTerm)
		if !valuesOK || !fixedOK || fixed != 1 || tail != 0 {
			continue
		}
		value, valueOK := values.Member(valuesTerm, 0)
		if !valueOK {
			return nil, errors.New("program/flow: TableField value position is unavailable")
		}
		selected = append(selected, entryField{field: field, value: value, key: key})
	}
	for left, right := 0, len(selected)-1; left < right; left, right = left+1, right-1 {
		selected[left], selected[right] = selected[right], selected[left]
	}
	return selected, nil
}

func entryFieldKey(sourceView source.View, fieldKind kind.FieldKind, term keyspace.Term) (keyspace.Key, bool) {
	keys := sourceView.Keys()
	switch fieldKind {
	case kind.FieldName:
		_, _, key, ok := keys.Name(term)
		if !ok || key == 0 {
			return 0, false
		}
		atom, ok := keys.Exact(key)
		return key, ok && atom.Kind == keyspace.LiteralString
	case kind.FieldExact:
		value, ok := entryExactString(sourceView, term)
		if !ok {
			return 0, false
		}
		return keys.Find(keyspace.LiteralValue{Kind: keyspace.LiteralString, String: value})
	default:
		return 0, false
	}
}

// entryExactString resolves only a direct Source String occurrence. Module
// entry publishes named string members exclusively; numeric equality,
// normalization, and Unary evaluation remain Source-private concerns and have
// no authority at this boundary.
func entryExactString(sourceView source.View, term keyspace.Term) (string, bool) {
	if keyspace.TermFamily(term) != keyspace.FamilyString {
		return "", false
	}
	ordinal := keyspace.TermOrdinal(term)
	if ordinal == 0 {
		return "", false
	}
	observed, _, value, ok := sourceView.Literals().Strings().At(int(ordinal - 1))
	return value, ok && observed == term
}
