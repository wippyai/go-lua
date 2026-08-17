package artifact

import (
	"crypto/sha256"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/flow"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/internal/framing"
)

// allocationCompileRow is the short-lived construction join for one exact
// Flow allocation. It exists only while this compiler copies the immutable
// artifact rows; no Flow allocation or authored Term is retained by Artifact.
type allocationCompileRow struct {
	allocation flow.Allocation
	term       keyspace.Term
	occurrence flow.AllocationOccurrence
	role       flow.AllocationRole
	form       flow.AllocationForm
	template   identity.ContentID
	root       program.Span
	entry      flow.Site
	finish     flow.Site
	fields     []allocationFieldCompileRow
}

type allocationFieldCompileRow struct {
	field        flow.AllocationField
	term         keyspace.Term
	kind         flowkind.FieldKind
	selector     keyspace.Term
	values       keyspace.Term
	width        int
	finalOpen    bool
	normalized   keyspace.Key
	normalizedOK bool
	valuesRow    ValuesRow
	fieldSpan    program.Span
	selectorSpan program.Span
	id           identity.ContentID
	shares       bool
}

// copyAllocationRowsFailure compiles the complete authored allocation
// denominator directly from Flow. The resulting HeapAllocationRow values are
// the only allocation/field rows retained by Artifact; the Flow proofs and
// source terms in allocationRows are construction scratch only.
func (compiler *compiler) copyAllocationRowsFailure() CompileFailure {
	if compiler == nil || !compiler.input.Available() {
		return compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceAllocation)
	}
	flowView := compiler.input.Flow()
	allocations := flowView.Allocations()
	count := allocations.Count()
	if count < 0 {
		return compileFailure(CompileStageOccurrences, CompileRowOccurrence, -1, -1, CompileReasonOccurrenceAllocation)
	}
	rows := make([]allocationCompileRow, 0, count)
	heapRows := make([]HeapAllocationRow, 0, count)
	seenTemplates := make(map[identity.ContentID]struct{}, count)
	authored := flowView.Authored()
	executable := flowView.Executable()
	tableIndex, functionIndex := 0, 0
	var failure CompileFailure
	setFailure := func(row, field int) bool {
		failure = compileFailure(CompileStageOccurrences, CompileRowOccurrence, row, field, CompileReasonOccurrenceAllocation)
		return false
	}
	if !allocations.Visit(func(allocation flow.Allocation) bool {
		rowIndex := len(rows)
		if !allocation.Available() || !allocations.Owns(allocation) {
			return setFailure(rowIndex, -1)
		}
		// Allocations deliberately keep their source coordinate private. Recover
		// the canonical term from the owner column in the same order used by
		// Allocations.Visit; this is construction-only and does not recreate an
		// allocation identity or retain a foreign proof coordinate.
		var term keyspace.Term
		var termOK bool
		switch allocation.Role() {
		case flow.AllocationTable:
			for tableIndex < authored.Tables().Count() {
				candidate, candidateOK := authored.Tables().At(tableIndex)
				tableIndex++
				if !candidateOK {
					break
				}
				if executable.Contains(candidate) {
					term, termOK = candidate, true
					break
				}
			}
		case flow.AllocationClosure:
			for functionIndex < authored.Functions().Count() {
				candidate, candidateOK := authored.Functions().At(functionIndex)
				functionIndex++
				if !candidateOK {
					break
				}
				if executable.Contains(candidate) {
					term, termOK = candidate, true
					break
				}
			}
		}
		occurrence := allocation.Occurrence()
		role, form := allocation.Role(), allocation.Form()
		root, rootOK := compiler.input.Span(term)
		entry, entryOK := root.Entry()
		finish, finishOK := root.Finish()
		if !termOK || !occurrence.Available() || !role.Valid() || !form.Valid() || !rootOK || !entryOK || !finishOK || !compiler.input.OwnsSpan(root) {
			return setFailure(rowIndex, -1)
		}
		fieldCount := allocation.FieldCount()
		if fieldCount < 0 || (role == flow.AllocationClosure && fieldCount != 0) {
			return setFailure(rowIndex, -1)
		}
		fields := make([]allocationFieldCompileRow, 0, fieldCount)
		for fieldIndex := 0; fieldIndex < fieldCount; fieldIndex++ {
			field, fieldOK := allocation.FieldAt(fieldIndex)
			fieldTerm, fieldTermOK := authored.Tables().FieldAt(term, fieldIndex)
			table, selector, values, kind, authoredFieldOK := authored.Fields().Get(fieldTerm)
			resolved, finalOpen, valuesOK := authored.Fields().Values(fieldTerm)
			width, widthOK := authored.Values().Len(values)
			valueRow, valueRowOK := compiler.valuesOccurrence(values)
			_, valueSpanOK := valueRow.RootSpanID()
			fieldSpan, fieldSpanOK := compiler.input.Span(fieldTerm)
			normalized, normalizedOK := flowView.AccessGeometry().TableFields().Get(fieldTerm)
			fieldID := allocationFieldID(compiler.input.ContentID(), field)
			fieldRow := allocationFieldCompileRow{
				field: field, term: fieldTerm, kind: kind, selector: selector, values: values,
				width: width, finalOpen: finalOpen, normalized: normalized, normalizedOK: normalizedOK,
				valuesRow: valueRow, fieldSpan: fieldSpan, id: fieldID,
			}
			if kind == flowkind.FieldKey {
				fieldRow.selectorSpan, _ = compiler.input.Span(selector)
			}
			fieldRow.shares = allocationSharesFirstValueCell(flowView, fieldTerm, kind, selector, values, width)
			if !fieldOK || !allocations.OwnsField(field) || !fieldTermOK || !authoredFieldOK || table != term || selector == 0 && kind == flowkind.FieldKey || values == 0 || !valuesOK || resolved != values || !widthOK || width < 0 || !valueRowOK || !valueRow.Available() || !valueSpanOK || !fieldSpanOK || !compiler.input.OwnsSpan(fieldSpan) || !fieldID.Available() {
				return setFailure(rowIndex, fieldIndex)
			}
			if kind == flowkind.FieldKey && (!fieldRow.selectorSpan.Available() || !compiler.input.OwnsSpan(fieldRow.selectorSpan)) {
				return setFailure(rowIndex, fieldIndex)
			}
			fields = append(fields, fieldRow)
		}
		template := allocationTemplateID(occurrence.ID(), role, form, fields)
		if !template.Available() {
			return setFailure(rowIndex, -1)
		}
		if _, duplicate := seenTemplates[template]; duplicate {
			return setFailure(rowIndex, -1)
		}
		seenTemplates[template] = struct{}{}
		row := allocationCompileRow{allocation: allocation, term: term, occurrence: occurrence, role: role, form: form, template: template, root: root, entry: entry, finish: finish, fields: fields}
		heapRow := HeapAllocationRow{id: template, role: role, form: form, rootSpan: root.ContextID(), fields: make([]HeapFieldRow, 0, len(fields))}
		for fieldIndex, field := range fields {
			valueSpan, valueSpanOK := field.valuesRow.RootSpanID()
			heapField := HeapFieldRow{id: field.id, kind: field.kind, fieldSpan: field.fieldSpan.ContextID(), valuesSpan: valueSpan, valuesID: field.valuesRow.ID(), width: field.width, finalOpen: field.finalOpen, sharesFirstValueCell: field.shares, normalized: field.normalized, normalizedOK: field.normalizedOK}
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
	}) {
		if failure.Available() {
			return failure
		}
		return compileFailure(CompileStageOccurrences, CompileRowOccurrence, len(rows), -1, CompileReasonOccurrenceAllocation)
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

func allocationTemplateID(occurrence identity.ContentID, role flow.AllocationRole, form flow.AllocationForm, fields []allocationFieldCompileRow) identity.ContentID {
	if !occurrence.Available() || !role.Valid() || !form.Valid() || (role == flow.AllocationClosure && len(fields) != 0) {
		return identity.ContentID{}
	}
	hash := sha256.New()
	var writer framing.Writer
	if writer.Reset(hash, "program/allocation-template", 2) != nil || writer.Record(1) != nil || writer.Bytes(occurrence[:]) != nil || writer.Uint(uint64(role)) != nil || writer.Uint(uint64(form)) != nil {
		return identity.ContentID{}
	}
	if role == flow.AllocationTable {
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

func allocationFieldID(programID identity.ContentID, field flow.AllocationField) identity.ContentID {
	if !programID.Available() || !field.Available() {
		return identity.ContentID{}
	}
	child := field.ID()
	if !child.Available() {
		return identity.ContentID{}
	}
	const prefix = "program-allocation-field-v1"
	var payload [len(prefix) + sha256.Size + sha256.Size]byte
	copy(payload[:len(prefix)], prefix)
	copy(payload[len(prefix):len(prefix)+sha256.Size], programID[:])
	copy(payload[len(prefix)+sha256.Size:], child[:])
	return sha256.Sum256(payload[:])
}

func allocationSharesFirstValueCell(view flow.View, field keyspace.Term, kind flowkind.FieldKind, selector, values keyspace.Term, width int) bool {
	if !view.AccessGeometry().Available() || kind != flowkind.FieldKey || field == 0 || selector == 0 || values == 0 || width <= 0 {
		return false
	}
	table, source, _, _, fieldOK := view.Authored().Fields().Get(field)
	member, memberOK := view.Authored().Values().Member(values, 0)
	reads := view.Authored().Storage().Reads()
	leftOwner, leftCell, _, leftOK := reads.Get(selector)
	rightOwner, rightCell, _, rightOK := reads.Get(member)
	return fieldOK && source == selector && memberOK && leftOK && rightOK && leftOwner == table && rightOwner == table &&
		keyspace.TermFamily(leftCell) == keyspace.FamilyCell && keyspace.TermOrdinal(leftCell) != 0 &&
		keyspace.TermFamily(rightCell) == keyspace.FamilyCell && keyspace.TermOrdinal(rightCell) != 0 && leftCell == rightCell
}
