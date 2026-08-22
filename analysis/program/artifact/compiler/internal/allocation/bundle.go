// Package allocation owns the compiler's private allocation construction
// bundle. It derives canonical heap rows and retains only the source joins
// still needed by later compiler phases; no Artifact or compiler authority is
// imported here.
package allocation

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/flow"
	"github.com/wippyai/go-lua/analysis/program/flow/causal"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
	programconstruction "github.com/wippyai/go-lua/analysis/schema/program/construction"
	"github.com/wippyai/go-lua/analysis/schema/program/heapallocation"
)

// Input is the authority needed to construct allocation rows. Values are
// already canonical compiler rows, ordered by their Values-family ordinal.
type Input struct {
	Program *program.Program
	Values  []programschema.Values
}

func (input Input) valueForTerm(term keyspace.Term) (programschema.Values, bool) {
	if keyspace.TermFamily(term) != keyspace.FamilyValues || keyspace.TermOrdinal(term) == 0 {
		return programschema.Values{}, false
	}
	index := int(keyspace.TermOrdinal(term)) - 1
	if index < 0 || index >= len(input.Values) {
		return programschema.Values{}, false
	}
	value := input.Values[index]
	return value, value.Available()
}

type row struct {
	term       keyspace.Term
	occurrence identity.ContentID
	canonical  uint32
	entry      causal.Site
	finish     causal.Site
	fields     []field
}

type field struct {
	canonical  uint32
	values     programschema.Values
	memberTerm keyspace.Term
}

// Bundle retains the exact scratch needed by occurrence, call-target,
// structural-diagnostic, and module projections, alongside canonical heap
// rows for publication. Its maps are private to this child package.
type Bundle struct {
	rows        []row
	byTerm      map[keyspace.Term]int
	fieldIDs    map[keyspace.Term]uint32
	heapRows    []heapallocation.Allocation
	heapFields  []heapallocation.Field
	byID        map[identity.ContentID]uint32
	transferred bool
}

// Row is an opaque handle to one source allocation held by a Bundle.
type Row struct {
	bundle *Bundle
	index  int
}

// Field is an opaque handle to one source field held by a Bundle.
type Field struct {
	bundle *Bundle
	row    int
	index  int
}

func (bundle *Bundle) validRow(index int) bool {
	return bundle != nil && index >= 0 && index < len(bundle.rows)
}

func (bundle *Bundle) validField(rowIndex, fieldIndex int) bool {
	return bundle.validRow(rowIndex) && fieldIndex >= 0 && fieldIndex < len(bundle.rows[rowIndex].fields)
}

func (bundle *Bundle) Count() int {
	if bundle == nil {
		return 0
	}
	return len(bundle.rows)
}

func (bundle *Bundle) RowAt(index int) (Row, bool) {
	if !bundle.validRow(index) {
		return Row{}, false
	}
	return Row{bundle: bundle, index: index}, true
}

func (bundle *Bundle) ForTerm(term keyspace.Term) (Row, bool) {
	if bundle == nil || term == 0 {
		return Row{}, false
	}
	index, ok := bundle.byTerm[term]
	if !ok {
		return Row{}, false
	}
	return bundle.RowAt(index)
}

func (bundle *Bundle) FieldID(term keyspace.Term) (identity.ContentID, bool) {
	if bundle == nil || term == 0 {
		return identity.ContentID{}, false
	}
	ordinal, ok := bundle.fieldIDs[term]
	field, fieldOK := bundle.canonicalField(ordinal)
	return field.ID(), ok && fieldOK && field.ID().Available()
}

func (bundle *Bundle) AllocationForID(id identity.ContentID) (heapallocation.Allocation, bool) {
	if bundle == nil || !id.Available() {
		return heapallocation.Allocation{}, false
	}
	ordinal, ok := bundle.byID[id]
	allocation, allocationOK := bundle.canonicalAllocation(ordinal)
	return allocation, ok && allocationOK
}

func (bundle *Bundle) canonicalAllocation(ordinal uint32) (heapallocation.Allocation, bool) {
	if bundle == nil || bundle.transferred || uint64(ordinal) >= uint64(len(bundle.heapRows)) {
		return heapallocation.Allocation{}, false
	}
	allocation := bundle.heapRows[ordinal]
	return allocation, allocation.Available()
}

func (bundle *Bundle) canonicalField(ordinal uint32) (heapallocation.Field, bool) {
	if bundle == nil || bundle.transferred || uint64(ordinal) >= uint64(len(bundle.heapFields)) {
		return heapallocation.Field{}, false
	}
	field := bundle.heapFields[ordinal]
	return field, field.Available()
}

// TakeCanonicalPlanes validates and transfers the one canonical allocation
// ledger to publication. Once taken, no source handle can reopen or read the
// canonical planes again.
func (bundle *Bundle) TakeCanonicalPlanes() ([]heapallocation.Allocation, []heapallocation.Field, bool) {
	if bundle == nil || bundle.transferred || len(bundle.byID) != len(bundle.heapRows) {
		return nil, nil, false
	}
	for index, allocation := range bundle.heapRows {
		if !allocation.Available() || !fitsUint32(index) || bundle.byID[allocation.ID()] != uint32(index) {
			return nil, nil, false
		}
		offset, count, spanOK := allocation.FieldSpan()
		if !spanOK || uint64(offset)+uint64(count) > uint64(len(bundle.heapFields)) {
			return nil, nil, false
		}
	}
	for index, field := range bundle.heapFields {
		if !field.Available() || !fitsUint32(index) {
			return nil, nil, false
		}
	}
	for term, ordinal := range bundle.fieldIDs {
		field, fieldOK := bundle.canonicalField(ordinal)
		if term == 0 || !fieldOK || !field.ID().Available() {
			return nil, nil, false
		}
	}
	for _, source := range bundle.rows {
		if _, allocationOK := bundle.canonicalAllocation(source.canonical); !allocationOK {
			return nil, nil, false
		}
		for _, sourceField := range source.fields {
			if _, fieldOK := bundle.canonicalField(sourceField.canonical); !fieldOK {
				return nil, nil, false
			}
		}
	}
	allocations, fields := bundle.heapRows, bundle.heapFields
	bundle.heapRows, bundle.heapFields = nil, nil
	bundle.byID, bundle.fieldIDs = nil, nil
	bundle.transferred = true
	return allocations, fields, true
}

func (handle Row) source() (row, bool) {
	if !handle.bundle.validRow(handle.index) {
		return row{}, false
	}
	return handle.bundle.rows[handle.index], true
}

func (row Row) Term() (keyspace.Term, bool) {
	source, ok := row.source()
	return source.term, ok && source.term != 0
}

func (row Row) Occurrence() (identity.ContentID, bool) {
	source, ok := row.source()
	return source.occurrence, ok && source.occurrence.Available()
}

func (row Row) canonical() (heapallocation.Allocation, bool) {
	source, ok := row.source()
	if !ok {
		return heapallocation.Allocation{}, false
	}
	return row.bundle.canonicalAllocation(source.canonical)
}

func (row Row) Role() (heapallocation.Role, bool) {
	allocation, ok := row.canonical()
	return allocation.Role(), ok && allocation.Role().Valid()
}

func (row Row) Form() (heapallocation.Form, bool) {
	allocation, ok := row.canonical()
	return allocation.Form(), ok && allocation.Form().Valid()
}

func (row Row) Template() (identity.ContentID, bool) {
	allocation, ok := row.canonical()
	return allocation.ID(), ok && allocation.ID().Available()
}

func (row Row) Entry() (causal.Site, bool) {
	source, ok := row.source()
	return source.entry, ok && source.entry.Available()
}

func (row Row) Finish() (causal.Site, bool) {
	source, ok := row.source()
	return source.finish, ok && source.finish.Available()
}

func (row Row) FieldCount() int {
	source, ok := row.source()
	if !ok {
		return 0
	}
	return len(source.fields)
}

func (row Row) FieldAt(index int) (Field, bool) {
	if !row.bundle.validField(row.index, index) {
		return Field{}, false
	}
	return Field{bundle: row.bundle, row: row.index, index: index}, true
}

func (handle Field) source() (field, bool) {
	if !handle.bundle.validField(handle.row, handle.index) {
		return field{}, false
	}
	return handle.bundle.rows[handle.row].fields[handle.index], true
}

func (field Field) canonical() (heapallocation.Field, bool) {
	source, ok := field.source()
	if !ok {
		return heapallocation.Field{}, false
	}
	return field.bundle.canonicalField(source.canonical)
}

func (field Field) Kind() (flowkind.FieldKind, bool) {
	canonical, ok := field.canonical()
	kind, kindOK := flowFieldKind(canonical.Kind())
	return kind, ok && kindOK
}

func (field Field) Width() (int, bool) {
	canonical, ok := field.canonical()
	return canonical.Width(), ok && canonical.Width() >= 0
}

func (field Field) FinalOpen() (bool, bool) {
	canonical, ok := field.canonical()
	return canonical.FinalOpen(), ok
}

func (field Field) Normalized() (keyspace.Key, bool) {
	canonical, ok := field.canonical()
	key, keyOK := canonical.NormalizedKey()
	return keyspace.Key(key), ok && keyOK
}

func (field Field) Values() (programschema.Values, bool) {
	source, ok := field.source()
	return source.values, ok && source.values.Available()
}

func (field Field) ID() (identity.ContentID, bool) {
	canonical, ok := field.canonical()
	return canonical.ID(), ok && canonical.ID().Available()
}

func (field Field) MemberTerm() (keyspace.Term, bool) {
	source, ok := field.source()
	return source.memberTerm, ok && source.memberTerm != 0
}

// Build compiles every executable table and closure allocation into one
// private bundle. All failure coordinates are expressed in canonical
// allocation-row and field order for the parent compiler to map directly.
func Build(input Input) (*Bundle, programconstruction.Fault) {
	if input.Program == nil || !input.Program.Available() {
		return nil, programconstruction.New(programcatalog.HeapAllocation(), programconstruction.IssueHeapAllocationUnavailable, -1, -1)
	}
	flowView := input.Program.Flow()
	authored := flowView.Authored()
	executable := flowView.Executable()
	count := 0
	for index := 0; index < authored.Tables().Count(); index++ {
		term, ok := authored.Tables().At(index)
		if ok && executable.Contains(term) {
			count++
		}
	}
	for index := 0; index < authored.Functions().Count(); index++ {
		term, ok := authored.Functions().At(index)
		if ok && executable.Contains(term) {
			count++
		}
	}
	bundle := &Bundle{
		rows:     make([]row, 0, count),
		byTerm:   make(map[keyspace.Term]int, count),
		fieldIDs: make(map[keyspace.Term]uint32),
		heapRows: make([]heapallocation.Allocation, 0, count),
		byID:     make(map[identity.ContentID]uint32, count),
	}
	seenTemplates := make(map[identity.ContentID]struct{}, count)
	compile := func(term keyspace.Term, role heapallocation.Role) programconstruction.Fault {
		rowIndex := len(bundle.heapRows)
		occurrence, occurrenceOK := flowView.AllocationID(term)
		form := formForFields(flowView, term, role)
		root, rootOK := input.Program.Span(term)
		entry, entryOK := root.Entry()
		finish, finishOK := root.Finish()
		if !occurrenceOK || !role.Valid() || !form.Valid() || !rootOK || !entryOK || !finishOK || !input.Program.OwnsSpan(root) {
			return programconstruction.New(programcatalog.HeapAllocation(), programconstruction.IssueHeapAllocationUnavailable, rowIndex, -1)
		}
		fieldCount := 0
		if role == heapallocation.RoleTable {
			var fieldsOK bool
			fieldCount, fieldsOK = authored.Tables().FieldCount(term)
			if !fieldsOK {
				return programconstruction.New(programcatalog.HeapAllocation(), programconstruction.IssueHeapAllocationUnavailable, rowIndex, -1)
			}
		}
		if role == heapallocation.RoleClosure && fieldCount != 0 {
			return programconstruction.New(programcatalog.HeapAllocation(), programconstruction.IssueHeapAllocationUnavailable, rowIndex, -1)
		}
		fields := make([]field, 0, fieldCount)
		canonicalFields := make([]heapallocation.Field, 0, fieldCount)
		fieldTerms := make([]keyspace.Term, 0, fieldCount)
		seenFields := make(map[keyspace.Term]struct{}, fieldCount)
		for fieldIndex := 0; fieldIndex < fieldCount; fieldIndex++ {
			fieldOrdinal := len(bundle.heapFields) + fieldIndex
			fieldTerm, fieldTermOK := authored.Tables().FieldAt(term, fieldIndex)
			table, selector, valuesTerm, kind, authoredFieldOK := authored.Fields().Get(fieldTerm)
			resolved, finalOpen, valuesOK := authored.Fields().Values(fieldTerm)
			width, widthOK := authored.Values().Len(valuesTerm)
			valueRow, valueRowOK := input.valueForTerm(valuesTerm)
			_, valueSpanOK := valueRow.RootSpanID()
			fieldSpan, fieldSpanOK := input.Program.Span(fieldTerm)
			normalized, normalizedOK := flowView.AccessGeometry().TableFields().Get(fieldTerm)
			fieldProof, fieldProofOK := flowView.AllocationFieldID(term, fieldTerm)
			fieldID := heapallocation.FieldID(input.Program.ContentID(), fieldProof)
			var memberTerm keyspace.Term
			if !finalOpen && width == 1 {
				memberTerm, _ = authored.Values().Member(valuesTerm, 0)
			}
			if !fieldTermOK || !authoredFieldOK || table != term || selector == 0 && kind == flowkind.FieldKey || valuesTerm == 0 || !valuesOK || resolved != valuesTerm || !widthOK || width < 0 || !valueRowOK || !valueRow.Available() || !valueSpanOK || !fieldSpanOK || !input.Program.OwnsSpan(fieldSpan) || !fieldProofOK || !fieldID.Available() {
				return programconstruction.New(programcatalog.HeapField(), programconstruction.IssueHeapFieldUnavailable, fieldOrdinal, -1)
			}
			selectorSpan := program.Span{}
			if kind == flowkind.FieldKey {
				selectorSpan, _ = input.Program.Span(selector)
				if !selectorSpan.Available() || !input.Program.OwnsSpan(selectorSpan) {
					return programconstruction.New(programcatalog.HeapField(), programconstruction.IssueHeapFieldUnavailable, fieldOrdinal, -1)
				}
			}
			if _, duplicate := bundle.fieldIDs[fieldTerm]; duplicate {
				return programconstruction.New(programcatalog.HeapField(), programconstruction.IssueHeapFieldDuplicate, fieldOrdinal, -1)
			}
			if _, duplicate := seenFields[fieldTerm]; duplicate {
				return programconstruction.New(programcatalog.HeapField(), programconstruction.IssueHeapFieldDuplicate, fieldOrdinal, -1)
			}
			canonicalKind, kindOK := heapFieldKind(kind)
			valuesSpan, valuesSpanOK := valueRow.RootSpanID()
			selectorSpanID := identity.ContentID{}
			if canonicalKind == heapallocation.FieldKindKey {
				selectorSpanID = selectorSpan.ContextID()
			}
			canonical, canonicalOK := heapallocation.NewField(fieldID, canonicalKind, fieldSpan.ContextID(), selectorSpanID, valuesSpan, valueRow.ID(), width, finalOpen, sharesFirstValueCell(flowView, fieldTerm, kind, selector, valuesTerm, width), uint64(normalized), normalizedOK)
			if !kindOK || !valuesSpanOK || !canonicalOK {
				return programconstruction.New(programcatalog.HeapField(), programconstruction.IssueHeapFieldUnavailable, fieldOrdinal, -1)
			}
			fields = append(fields, field{values: valueRow, memberTerm: memberTerm})
			canonicalFields = append(canonicalFields, canonical)
			fieldTerms = append(fieldTerms, fieldTerm)
			seenFields[fieldTerm] = struct{}{}
		}
		fieldOffset := len(bundle.heapFields)
		template := heapallocation.TemplateID(occurrence, role, form, canonicalFields)
		if !template.Available() {
			return programconstruction.New(programcatalog.HeapAllocation(), programconstruction.IssueHeapAllocationUnavailable, rowIndex, -1)
		}
		if _, duplicate := seenTemplates[template]; duplicate {
			return programconstruction.New(programcatalog.HeapAllocation(), programconstruction.IssueHeapAllocationDuplicate, rowIndex, -1)
		}
		if _, duplicate := bundle.byTerm[term]; duplicate {
			return programconstruction.New(programcatalog.HeapAllocation(), programconstruction.IssueHeapAllocationDuplicate, rowIndex, -1)
		}
		if !fitsUint32(fieldOffset) || !fitsUint32(len(canonicalFields)) || !fitsUint32(len(bundle.heapRows)) {
			return programconstruction.New(programcatalog.HeapAllocation(), programconstruction.IssueHeapAllocationUnavailable, rowIndex, -1)
		}
		heapRow, heapRowOK := heapallocation.NewAllocation(template, role, form, root.ContextID(), uint32(fieldOffset), uint32(len(canonicalFields)))
		if !heapRowOK {
			return programconstruction.New(programcatalog.HeapAllocation(), programconstruction.IssueHeapAllocationUnavailable, rowIndex, -1)
		}
		seenTemplates[template] = struct{}{}
		for fieldIndex, fieldTerm := range fieldTerms {
			ordinal := uint32(fieldOffset + fieldIndex)
			bundle.fieldIDs[fieldTerm] = ordinal
			fields[fieldIndex].canonical = ordinal
		}
		bundle.byTerm[term] = rowIndex
		bundle.rows = append(bundle.rows, row{term: term, occurrence: occurrence, canonical: uint32(len(bundle.heapRows)), entry: entry, finish: finish, fields: fields})
		bundle.heapFields = append(bundle.heapFields, canonicalFields...)
		bundle.heapRows = append(bundle.heapRows, heapRow)
		bundle.byID[template] = uint32(len(bundle.heapRows) - 1)
		return programconstruction.Fault{}
	}
	for index := 0; index < authored.Tables().Count(); index++ {
		term, ok := authored.Tables().At(index)
		if ok && executable.Contains(term) {
			if fault := compile(term, heapallocation.RoleTable); fault.Available() {
				return nil, fault
			}
		}
	}
	for index := 0; index < authored.Functions().Count(); index++ {
		term, ok := authored.Functions().At(index)
		if ok && executable.Contains(term) {
			if fault := compile(term, heapallocation.RoleClosure); fault.Available() {
				return nil, fault
			}
		}
	}
	if len(bundle.rows) != count || len(bundle.heapRows) != count || len(bundle.byID) != count {
		return nil, programconstruction.New(programcatalog.HeapAllocation(), programconstruction.IssueHeapAllocationUnavailable, len(bundle.heapRows), -1)
	}
	return bundle, programconstruction.Fault{}
}

func formForFields(view flow.View, term keyspace.Term, role heapallocation.Role) heapallocation.Form {
	if !view.ContentID().Available() || !role.Valid() {
		return 0
	}
	if role == heapallocation.RoleClosure {
		return heapallocation.FormEmpty
	}
	count, ok := view.Authored().Tables().FieldCount(term)
	if !ok {
		return 0
	}
	if count == 0 {
		return heapallocation.FormEmpty
	}
	open := false
	for index := 0; index < count; index++ {
		field, fieldOK := view.Authored().Tables().FieldAt(term, index)
		table, _, values, _, rowOK := view.Authored().Fields().Get(field)
		resolved, fieldOpen, valuesOK := view.Authored().Fields().Values(field)
		if !fieldOK || !rowOK || table != term || !valuesOK || resolved != values {
			return 0
		}
		if fieldOpen {
			open = true
			continue
		}
		width, widthOK := view.Authored().Values().Len(values)
		if !widthOK || width != 1 {
			return 0
		}
	}
	if open {
		return heapallocation.FormFinalOpen
	}
	return heapallocation.FormClosed
}

func heapFieldKind(kind flowkind.FieldKind) (heapallocation.FieldKind, bool) {
	switch kind {
	case flowkind.FieldList:
		return heapallocation.FieldKindList, true
	case flowkind.FieldName:
		return heapallocation.FieldKindName, true
	case flowkind.FieldExact:
		return heapallocation.FieldKindExact, true
	case flowkind.FieldKey:
		return heapallocation.FieldKindKey, true
	default:
		return 0, false
	}
}

func flowFieldKind(kind heapallocation.FieldKind) (flowkind.FieldKind, bool) {
	switch kind {
	case heapallocation.FieldKindList:
		return flowkind.FieldList, true
	case heapallocation.FieldKindName:
		return flowkind.FieldName, true
	case heapallocation.FieldKindExact:
		return flowkind.FieldExact, true
	case heapallocation.FieldKindKey:
		return flowkind.FieldKey, true
	default:
		return 0, false
	}
}

func sharesFirstValueCell(view flow.View, field keyspace.Term, kind flowkind.FieldKind, selector, values keyspace.Term, width int) bool {
	if !view.AccessGeometry().Available() || kind != flowkind.FieldKey || field == 0 || selector == 0 || values == 0 || width <= 0 {
		return false
	}
	_, source, _, _, fieldOK := view.Authored().Fields().Get(field)
	member, memberOK := view.Authored().Values().Member(values, 0)
	reads := view.Authored().Storage().Reads()
	_, leftSource, _, leftOK := reads.Get(selector)
	_, rightSource, _, rightOK := reads.Get(member)
	return fieldOK && source == selector && memberOK && leftOK && rightOK &&
		keyspace.TermFamily(leftSource) == keyspace.FamilyCell && keyspace.TermOrdinal(leftSource) != 0 &&
		keyspace.TermFamily(rightSource) == keyspace.FamilyCell && keyspace.TermOrdinal(rightSource) != 0 &&
		leftSource == rightSource
}

func fitsUint32(value int) bool { return value >= 0 && uint64(value) <= uint64(^uint32(0)) }
