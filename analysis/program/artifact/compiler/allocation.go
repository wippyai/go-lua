package compiler

import (
	"crypto/sha256"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/flow"
	"github.com/wippyai/go-lua/analysis/program/flow/causal"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/internal/framing"
)

// allocationCompileRow is the short-lived construction join for one exact
// authored allocation. It exists only while this compiler copies the immutable
// artifact rows; no Flow proof object or authored Term is retained by Artifact.
type allocationCompileRow struct {
	term       keyspace.Term
	occurrence identity.ContentID
	role       allocationRole
	form       allocationForm
	template   identity.ContentID
	root       program.Span
	entry      causal.Site
	finish     causal.Site
	fields     []allocationFieldCompileRow
}

type allocationFieldCompileRow struct {
	term         keyspace.Term
	kind         flowkind.FieldKind
	selector     keyspace.Term
	values       keyspace.Term
	width        int
	finalOpen    bool
	normalized   keyspace.Key
	normalizedOK bool
	valuesRow    programschema.Values
	fieldSpan    program.Span
	selectorSpan program.Span
	id           identity.ContentID
	shares       bool
	// memberTerm is authored.Values().Member(values, 0): the single fixed
	// member a width-1, non-open field carries. It is 0 when the field is
	// open or has width other than 1, mirroring authored.Values().Member's
	// own zero-term failure convention.
	memberTerm keyspace.Term
}

// copyAllocationRowsFailure compiles the complete authored allocation
// denominator directly from Flow. The resulting heapAllocationDraft values are
// the only allocation/field rows retained by Artifact; the Flow proofs and
// source terms in allocationRows are construction scratch only.
func (compiler *compiler) copyAllocationRowsFailure() CompileFailure {
	if compiler == nil || !compiler.input.Available() {
		return compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceAllocation)
	}
	flowView := compiler.input.Flow()
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
	rows := make([]allocationCompileRow, 0, count)
	heapRows := make([]heapAllocationDraft, 0, count)
	seenTemplates := make(map[identity.ContentID]struct{}, count)
	var failure CompileFailure
	setFailure := func(row, field int) bool {
		failure = compileFailure(CompileStageOccurrences, CompileRowOccurrence, row, field, CompileReasonOccurrenceAllocation)
		return false
	}
	compile := func(term keyspace.Term, role allocationRole) bool {
		rowIndex := len(rows)
		occurrence, occurrenceOK := flowView.AllocationID(term)
		form := allocationFormForFields(flowView, term, role)
		root, rootOK := compiler.input.Span(term)
		entry, entryOK := root.Entry()
		finish, finishOK := root.Finish()
		if !occurrenceOK || !role.Valid() || !form.Valid() || !rootOK || !entryOK || !finishOK || !compiler.input.OwnsSpan(root) {
			return setFailure(rowIndex, -1)
		}
		fieldCount := 0
		if role == allocationTable {
			var fieldCountOK bool
			fieldCount, fieldCountOK = authored.Tables().FieldCount(term)
			if !fieldCountOK {
				return setFailure(rowIndex, -1)
			}
		}
		if role == allocationClosure && fieldCount != 0 {
			return setFailure(rowIndex, -1)
		}
		fields := make([]allocationFieldCompileRow, 0, fieldCount)
		for fieldIndex := 0; fieldIndex < fieldCount; fieldIndex++ {
			fieldTerm, fieldTermOK := authored.Tables().FieldAt(term, fieldIndex)
			table, selector, values, kind, authoredFieldOK := authored.Fields().Get(fieldTerm)
			resolved, finalOpen, valuesOK := authored.Fields().Values(fieldTerm)
			width, widthOK := authored.Values().Len(values)
			valueRow, valueRowOK := compiler.valueRowForTerm(values)
			_, valueSpanOK := valueRow.RootSpanID()
			fieldSpan, fieldSpanOK := compiler.input.Span(fieldTerm)
			normalized, normalizedOK := flowView.AccessGeometry().TableFields().Get(fieldTerm)
			fieldProof, fieldProofOK := flowView.AllocationFieldID(term, fieldTerm)
			fieldID := allocationFieldID(compiler.input.ContentID(), fieldProof)
			var memberTerm keyspace.Term
			if !finalOpen && width == 1 {
				memberTerm, _ = authored.Values().Member(values, 0)
			}
			fieldRow := allocationFieldCompileRow{
				term: fieldTerm, kind: kind, selector: selector, values: values,
				width: width, finalOpen: finalOpen, normalized: normalized, normalizedOK: normalizedOK,
				valuesRow: valueRow, fieldSpan: fieldSpan, id: fieldID, memberTerm: memberTerm,
			}
			if kind == flowkind.FieldKey {
				fieldRow.selectorSpan, _ = compiler.input.Span(selector)
			}
			fieldRow.shares = allocationSharesFirstValueCell(flowView, fieldTerm, kind, selector, values, width)
			if !fieldTermOK || !authoredFieldOK || table != term || selector == 0 && kind == flowkind.FieldKey || values == 0 || !valuesOK || resolved != values || !widthOK || width < 0 || !valueRowOK || !valueRow.Available() || !valueSpanOK || !fieldSpanOK || !compiler.input.OwnsSpan(fieldSpan) || !fieldProofOK || !fieldID.Available() {
				return setFailure(rowIndex, fieldIndex)
			}
			if kind == flowkind.FieldKey && (!fieldRow.selectorSpan.Available() || !compiler.input.OwnsSpan(fieldRow.selectorSpan)) {
				return setFailure(rowIndex, fieldIndex)
			}
			fields = append(fields, fieldRow)
		}
		template := allocationTemplateID(occurrence, role, form, fields)
		if !template.Available() {
			return setFailure(rowIndex, -1)
		}
		if _, duplicate := seenTemplates[template]; duplicate {
			return setFailure(rowIndex, -1)
		}
		seenTemplates[template] = struct{}{}
		row := allocationCompileRow{term: term, occurrence: occurrence, role: role, form: form, template: template, root: root, entry: entry, finish: finish, fields: fields}
		heapRow := heapAllocationDraft{id: template, role: role, form: form, rootSpan: root.ContextID(), fields: make([]heapFieldDraft, 0, len(fields))}
		for fieldIndex, field := range fields {
			valueSpan, valueSpanOK := field.valuesRow.RootSpanID()
			heapField := heapFieldDraft{id: field.id, kind: field.kind, fieldSpan: field.fieldSpan.ContextID(), valuesSpan: valueSpan, valuesID: field.valuesRow.ID(), width: field.width, finalOpen: field.finalOpen, sharesFirstValueCell: field.shares, normalized: field.normalized, normalizedOK: field.normalizedOK}
			if field.kind == flowkind.FieldKey {
				heapField.selectorSpan = field.selectorSpan.ContextID()
			}
			if !valueSpanOK || !heapField.Available() {
				return setFailure(rowIndex, fieldIndex)
			}
			heapRow.fields = append(heapRow.fields, heapField)
		}
		if !heapRow.Available() {
			return setFailure(rowIndex, -1)
		}
		rows = append(rows, row)
		heapRows = append(heapRows, heapRow)
		return true
	}
	for index := 0; index < authored.Tables().Count(); index++ {
		term, ok := authored.Tables().At(index)
		if ok && executable.Contains(term) && !compile(term, allocationTable) {
			break
		}
	}
	for index := 0; index < authored.Functions().Count() && !failure.Available(); index++ {
		term, ok := authored.Functions().At(index)
		if ok && executable.Contains(term) && !compile(term, allocationClosure) {
			break
		}
	}
	if failure.Available() {
		return failure
	}
	if failure.Available() || len(rows) != count {
		if failure.Available() {
			return failure
		}
		return compileFailure(CompileStageOccurrences, CompileRowOccurrence, len(rows), -1, CompileReasonOccurrenceAllocation)
	}
	compiler.allocationRows = rows
	compiler.heapAllocations = heapRows
	return CompileFailure{}
}

// allocationRowForTerm looks up one allocation's compile row by its own
// authored term. The term -> row index is built once, on first use, so a
// program whose diagnostic population never descends a structural member
// (no declared record/array/map initializer) pays nothing for it. A term
// that is not a compiled allocation -- not FamilyTable, or FamilyTable but
// not Executable -- is simply absent from the index.
func (compiler *compiler) allocationRowForTerm(term keyspace.Term) (allocationCompileRow, bool) {
	if compiler == nil || term == 0 {
		return allocationCompileRow{}, false
	}
	if compiler.allocationRowsByTerm == nil {
		byTerm := make(map[keyspace.Term]int, len(compiler.allocationRows))
		for index, row := range compiler.allocationRows {
			byTerm[row.term] = index
		}
		compiler.allocationRowsByTerm = byTerm
	}
	index, found := compiler.allocationRowsByTerm[term]
	if !found || index < 0 || index >= len(compiler.allocationRows) {
		return allocationCompileRow{}, false
	}
	return compiler.allocationRows[index], true
}

func allocationTemplateID(occurrence identity.ContentID, role allocationRole, form allocationForm, fields []allocationFieldCompileRow) identity.ContentID {
	if !occurrence.Available() || !role.Valid() || !form.Valid() || (role == allocationClosure && len(fields) != 0) {
		return identity.ContentID{}
	}
	hash := sha256.New()
	var writer framing.Writer
	if writer.Reset(hash, "program/allocation-template", 2) != nil || writer.Record(1) != nil || writer.Bytes(occurrence[:]) != nil || writer.Uint(uint64(role)) != nil || writer.Uint(uint64(form)) != nil {
		return identity.ContentID{}
	}
	if role == allocationTable {
		if writer.Count(uint64(len(fields))) != nil {
			return identity.ContentID{}
		}
		for _, field := range fields {
			if writer.Record(1) != nil || writer.Uint(uint64(field.kind)) != nil || writer.Bool(field.normalizedOK) != nil {
				return identity.ContentID{}
			}
			if field.normalizedOK && writer.Uint(uint64(field.normalized)) != nil {
				return identity.ContentID{}
			}
			if writer.Uint(uint64(field.width)) != nil || writer.Bool(field.finalOpen) != nil {
				return identity.ContentID{}
			}
		}
	}
	if writer.Finish() != nil {
		return identity.ContentID{}
	}
	var id identity.ContentID
	copy(id[:], hash.Sum(nil))
	return id
}

func allocationFormForFields(view flow.View, term keyspace.Term, role allocationRole) allocationForm {
	if !view.ContentID().Available() || !role.Valid() {
		return allocationFormInvalid
	}
	if role == allocationClosure {
		return allocationFormEmpty
	}
	count, ok := view.Authored().Tables().FieldCount(term)
	if !ok {
		return allocationFormInvalid
	}
	if count == 0 {
		return allocationFormEmpty
	}
	open := false
	for index := 0; index < count; index++ {
		field, fieldOK := view.Authored().Tables().FieldAt(term, index)
		table, _, values, _, rowOK := view.Authored().Fields().Get(field)
		resolved, fieldOpen, valuesOK := view.Authored().Fields().Values(field)
		if !fieldOK || !rowOK || table != term || !valuesOK || resolved != values {
			return allocationFormInvalid
		}
		if fieldOpen {
			open = true
			continue
		}
		width, widthOK := view.Authored().Values().Len(values)
		if !widthOK || width != 1 {
			return allocationFormInvalid
		}
	}
	if open {
		return allocationFormFinalOpen
	}
	return allocationFormClosed
}

func allocationFieldID(programID, fieldProof identity.ContentID) identity.ContentID {
	if !programID.Available() || !fieldProof.Available() {
		return identity.ContentID{}
	}
	const prefix = "program-allocation-field-v1"
	var payload [len(prefix) + sha256.Size + sha256.Size]byte
	copy(payload[:len(prefix)], prefix)
	copy(payload[len(prefix):len(prefix)+sha256.Size], programID[:])
	copy(payload[len(prefix)+sha256.Size:], fieldProof[:])
	return sha256.Sum256(payload[:])
}

func allocationSharesFirstValueCell(view flow.View, field keyspace.Term, kind flowkind.FieldKind, selector, values keyspace.Term, width int) bool {
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
