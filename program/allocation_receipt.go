package program

import (
	"crypto/sha256"
	"errors"
	"fmt"

	"github.com/wippyai/go-lua/program/flow"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/internal/canonical"
	"github.com/wippyai/go-lua/program/keyspace"
)

// allocationReceipt is the one cold publication product for authored
// allocation geometry. All rows are immutable and owner-fenced; hot queries
// only index these already-sealed rows and never reopen Flow.
type allocationReceipt struct {
	owner *Program
	rows  []allocationReceiptRow
}

type allocationReceiptRow struct {
	allocation flow.Allocation
	term       keyspace.Term
	entry      flow.Site
	finish     flow.Site
	role       AllocationRole
	form       AllocationForm
	template   AllocationTemplate
	fields     []allocationFieldReceipt
	target     allocationTargetReceipt
}

type allocationTargetReceipt struct {
	body       Body
	function   Function
	callTarget CallBodyTarget
}

type allocationFieldReceipt struct {
	field        flow.AllocationField
	term         keyspace.Term
	kind         flowkind.FieldKind
	selector     keyspace.Term
	normalized   keyspace.Key
	normalizedOK bool
	values       keyspace.Term
	width        int
	finalOpen    bool
	valuesOK     bool
}

func buildAllocationReceipt(owner *Program) (*allocationReceipt, error) {
	if owner == nil || owner.flow == nil || !owner.id.Available() {
		return nil, errors.New("owner-unavailable")
	}
	view := owner.Flow()
	allocations := view.Allocations()
	receipt := &allocationReceipt{owner: owner}
	var failure error
	fail := func(stage string, field int) bool {
		failure = fmt.Errorf("stage=%s row=%d field=%d", stage, len(receipt.rows), field)
		return false
	}
	input := TransformerInput{
		owner: owner, programID: owner.id,
		sourceID: owner.source.Cold().ContentID(), flowID: owner.flow.ContentID(),
		staticID: owner.static.Cold().ContentID(), moduleID: owner.module.Cold().ContentID(),
		allocationReceipt: receipt,
		pointAttachments:  owner.pointAttachments,
	}
	seenTemplates := make(map[keyspace.ContentID]struct{})
	if !allocations.Visit(func(allocation flow.Allocation) bool {
		if !allocation.Available() {
			return fail("allocation", -1)
		}
		term, termOK := allocation.SourceTerm()
		if !termOK {
			return fail("source", -1)
		}
		entryTerm, entryOK := view.Ports().Entry(term)
		finishTerm, finishOK := view.Ports().Finish(term)
		entry, entrySiteOK := view.Causal().Sites().ForTerm(entryTerm)
		finish, finishSiteOK := view.Causal().Sites().ForTerm(finishTerm)
		if !entryOK || !finishOK || !entrySiteOK || !finishSiteOK {
			return fail("span", -1)
		}
		occurrence := allocation.Occurrence()
		if !occurrence.Available() {
			return fail("occurrence", -1)
		}
		row := allocationReceiptRow{allocation: allocation, term: term, entry: entry, finish: finish, role: allocation.Role(), form: allocation.Form()}
		if !row.role.Valid() || !row.form.Valid() {
			return fail("shape", -1)
		}
		if row.role == AllocationClosure {
			boundary, boundaryOK := view.FunctionBoundaries().For(term)
			if !boundaryOK {
				return fail("closure-boundary", -1)
			}
			bodyTerm, bodyTermOK := boundary.Body()
			if !bodyTermOK {
				return fail("closure-body-term", -1)
			}
			body, bodyOK := input.Body(bodyTerm)
			if !bodyOK {
				return fail("closure-body", -1)
			}
			function, functionOK := body.TransformerFunction()
			if !functionOK {
				return fail("closure-function", -1)
			}
			callTarget, targetOK := body.CallTarget()
			if !targetOK {
				return fail("closure-target", -1)
			}
			row.target = allocationTargetReceipt{body: body, function: function, callTarget: callTarget}
		}
		var templateOK bool
		fieldCount := allocation.FieldCount()
		if fieldCount < 0 {
			return fail("field-count", -1)
		}
		row.fields = make([]allocationFieldReceipt, fieldCount)
		for fieldIndex := 0; fieldIndex < fieldCount; fieldIndex++ {
			field, fieldOK := allocation.FieldAt(fieldIndex)
			if !fieldOK || !field.Available() {
				return fail("field", fieldIndex)
			}
			fieldTerm, fieldTermOK := field.SourceTerm()
			if !fieldTermOK {
				return fail("field-source", fieldIndex)
			}
			_, selector, values, fieldKind, fieldOK := view.Authored().Fields().Get(fieldTerm)
			if !fieldOK || selector == 0 || values == 0 {
				return fail("field-row", fieldIndex)
			}
			width, widthOK := view.Authored().Values().Len(values)
			_, finalOpen, finalOpenOK := view.Authored().Fields().Values(fieldTerm)
			normalized, normalizedOK := view.AccessGeometry().TableFields().Get(fieldTerm)
			if !widthOK || !finalOpenOK {
				return fail("field-values", fieldIndex)
			}
			row.fields[fieldIndex] = allocationFieldReceipt{field: field, term: fieldTerm, kind: fieldKind, selector: selector, normalized: normalized, normalizedOK: normalizedOK, values: values, width: width, finalOpen: finalOpen, valuesOK: true}
		}
		row.template, templateOK = makeAllocationTemplate(input, allocation, occurrence, row.role, row.form, row.fields)
		if !templateOK || row.template.ID() == (keyspace.ContentID{}) {
			return fail("template", -1)
		}
		if _, exists := seenTemplates[row.template.ID()]; exists {
			return fail("duplicate-template", -1)
		}
		seenTemplates[row.template.ID()] = struct{}{}
		receipt.rows = append(receipt.rows, row)
		return true
	}) {
		if failure != nil {
			return nil, failure
		}
		return nil, errors.New("denominator-unavailable")
	}
	return receipt, nil
}

func makeAllocationTemplate(input TransformerInput, allocation flow.Allocation, occurrence flow.AllocationOccurrence, role AllocationRole, form AllocationForm, fields []allocationFieldReceipt) (AllocationTemplate, bool) {
	if !input.Available() || !allocation.Available() || !role.Valid() || !form.Valid() || (role == AllocationTable && fields == nil) || (role == AllocationClosure && len(fields) != 0) {
		return AllocationTemplate{}, false
	}
	if !occurrence.Available() || occurrence.ID() == (keyspace.ContentID{}) {
		return AllocationTemplate{}, false
	}
	hash := sha256.New()
	var writer canonical.Writer
	occurrenceID := occurrence.ID()
	if writer.Reset(hash, "program/allocation-template", 2) != nil || writer.Record(1) != nil || writer.Bytes(occurrenceID[:]) != nil || writer.Uint(uint64(role)) != nil || writer.Uint(uint64(form)) != nil {
		return AllocationTemplate{}, false
	}
	if role == AllocationTable {
		if writer.Count(uint64(len(fields))) != nil {
			return AllocationTemplate{}, false
		}
		for _, field := range fields {
			if writer.Record(1) != nil || writer.Uint(uint64(field.kind)) != nil || writer.Bool(field.normalizedOK) != nil {
				return AllocationTemplate{}, false
			}
			if field.normalizedOK && writer.Uint(uint64(field.normalized)) != nil {
				return AllocationTemplate{}, false
			}
			if writer.Uint(uint64(field.width)) != nil || writer.Bool(field.finalOpen) != nil {
				return AllocationTemplate{}, false
			}
		}
	}
	if writer.Finish() != nil {
		return AllocationTemplate{}, false
	}
	var id keyspace.ContentID
	copy(id[:], hash.Sum(nil))
	return AllocationTemplate{id: id, role: role, form: form, occurrence: SemanticOccurrence{input: input, id: occurrenceID, role: role}}, id.Available()
}

// semanticBodyPath is retained for call-formal compatibility. Body.ContextID
// is an opaque Flow-issued boundary identity; no Program-side Body denominator
// or global BodyAt scan is reconstructed here.
func semanticBodyPath(input TransformerInput, body Body) (keyspace.ContentID, bool) {
	if !input.Available() || !body.Available() || !input.OwnsBody(body) {
		return keyspace.ContentID{}, false
	}
	bodyTerm, bodyOK := body.boundary.Body()
	if !bodyOK {
		return keyspace.ContentID{}, false
	}
	return input.owner.Flow().BodyPath(bodyTerm)
}

func (receipt *allocationReceipt) valid(owner *Program) bool {
	return receipt != nil && receipt.owner == owner && owner != nil && owner.id.Available()
}
