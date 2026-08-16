package program

import (
	"crypto/sha256"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// AllocationRole and AllocationForm are the neutral Program vocabulary for
// authored allocation templates. Freshening and escape remain runtime/bind
// semantics and are intentionally absent.
type AllocationRole = flow.AllocationRole
type AllocationForm = flow.AllocationForm

const (
	AllocationInvalid = flow.AllocationInvalid
	AllocationTable   = flow.AllocationTable
	AllocationClosure = flow.AllocationClosure

	AllocationFormInvalid   = flow.AllocationFormInvalid
	AllocationFormEmpty     = flow.AllocationFormEmpty
	AllocationFormClosed    = flow.AllocationFormClosed
	AllocationFormFinalOpen = flow.AllocationFormFinalOpen
)

// Allocations is an O(1) Program-owned view over the immutable publication
// receipt. It retains no hot copy or reconstruction cursor.
type Allocations struct{ input TransformerInput }

// Allocation is an opaque exact-Program-owned allocation proof. Its stable
// ID commits to the composite Program owner, not merely Flow.ContentID.
type Allocation struct {
	input      TransformerInput
	allocation flow.Allocation
	receipt    *allocationReceipt
	index      int
}

// AllocationTarget is the sealed Program projection of one closure
// allocation. It carries the exact Body, owning Function, and formal Call
// target proofs issued during Program publication; consumers never recover a
// root Term or scan Flow to reconstruct the correspondence.
type AllocationTarget struct {
	allocation Allocation
	body       Body
	function   Function
	callTarget CallBodyTarget
}

// SemanticOccurrence is the owner-issued, non-Term occurrence anchor for one
// authored allocation. Its identity is the canonical path of the owning Body,
// the allocation family, and the allocation's position within that Body. It
// contains no raw coordinate or global Program identity.
type SemanticOccurrence struct {
	input TransformerInput
	id    identity.ContentID
	role  AllocationRole
}

func (occurrence SemanticOccurrence) Available() bool {
	return occurrence.input.Available() && occurrence.id.Available() && occurrence.role.Valid()
}

func (occurrence SemanticOccurrence) ID() identity.ContentID {
	if !occurrence.Available() {
		return identity.ContentID{}
	}
	return occurrence.id
}

func (occurrence SemanticOccurrence) Owns(input TransformerInput) bool {
	return occurrence.Available() && input.Available() && occurrence.input == input
}

func (occurrence SemanticOccurrence) Equal(other SemanticOccurrence) bool {
	return occurrence.Available() && other.Available() && occurrence.id == other.id && occurrence.role == other.role
}

// AllocationTemplate is the cold, content-addressed constructor identity of
// one authored Program allocation. It is deliberately independent of the
// enclosing Link and of the Project mount/shard that will freshen it into a
// Heap key. The ID is the existing Program-owned allocation identity; role and
// form remain typed witnesses rather than a caller-minted coordinate.
type AllocationTemplate struct {
	id         identity.ContentID
	role       AllocationRole
	form       AllocationForm
	occurrence SemanticOccurrence
}

// Available reports whether this is a complete owner-neutral allocation
// template. A zero template cannot be used as a Heap freshening operand.
func (template AllocationTemplate) Available() bool {
	return template.id.Available() && template.role.Valid() && template.form.Valid() && template.occurrence.Available() && template.occurrence.role == template.role
}

// ID returns the stable content address of the authored allocation template.
// It excludes Link identity, mount identity, and any dense/shard ordinal.
func (template AllocationTemplate) ID() identity.ContentID {
	if !template.Available() {
		return identity.ContentID{}
	}
	return template.id
}

func (template AllocationTemplate) Role() AllocationRole {
	if !template.Available() {
		return AllocationInvalid
	}
	return template.role
}

func (template AllocationTemplate) Form() AllocationForm {
	if !template.Available() {
		return AllocationFormInvalid
	}
	return template.form
}

// Equal compares the complete typed template receipt. It does not compare a
// Heap owner or a mounted occurrence, since those are supplied only by
// FreshenAllocation.
func (template AllocationTemplate) Equal(other AllocationTemplate) bool {
	return template.Available() && other.Available() && template.id == other.id && template.role == other.role && template.form == other.form && template.occurrence.Equal(other.occurrence)
}

// AllocationField is an opaque ordered field proof under one Allocation.
type AllocationField struct {
	input      TransformerInput
	allocation Allocation
	field      flow.AllocationField
	receipt    *allocationReceipt
	index      int
}

// AllocationGeometry is the immutable authored geometry of one exact
// allocation. It retains no copied rows; FieldAt reissues the exact ordered
// child proof from the sealed Flow relation.
type AllocationGeometry struct{ allocation Allocation }

// AllocationFieldGeometry is the typed authored geometry of one exact field.
// The selector and Values terms are cold source coordinates only; they are
// never part of Allocation or AllocationField identity.
type AllocationFieldGeometry struct {
	field      AllocationField
	receipt    *allocationReceipt
	allocation Allocation
	index      int
}

// AllocationSpan is the sealed entry/finish proof for one allocation root.
// It carries no authored Term and cannot be rebound to another Program.
type AllocationSpan struct {
	input  TransformerInput
	entry  flow.Site
	finish flow.Site
}

// Allocations returns the allocation proof view for this exact published
// Program. An unavailable or zero TransformerInput exposes an empty view.
func (input TransformerInput) Allocations() Allocations {
	if !input.Available() {
		return Allocations{}
	}
	return Allocations{input: input}
}

// Count and At preserve the canonical authored table-then-closure order with
// no Flow query or topology reconstruction on the hot path.
func (allocations Allocations) Count() int {
	if !allocations.input.Available() || allocations.input.allocationReceipt == nil {
		return 0
	}
	return len(allocations.input.allocationReceipt.rows)
}

func (allocations Allocations) At(index int) (Allocation, bool) {
	if !allocations.input.Available() || index < 0 || index >= allocations.Count() {
		return Allocation{}, false
	}
	row := allocations.input.allocationReceipt.rows[index]
	result := Allocation{input: allocations.input, allocation: row.allocation, receipt: allocations.input.allocationReceipt, index: index}
	return result, result.Available()
}

// Owns is the exact hot owner fence. Equivalent Program replay retains the
// same stable ID but is not owner-identical.
func (allocations Allocations) Owns(allocation Allocation) bool {
	return allocations.input.Available() && allocation.Available() && allocation.input.owner == allocations.input.owner && allocation.receipt == allocations.input.allocationReceipt
}

func (allocations Allocations) OwnsField(field AllocationField) bool {
	return allocations.input.Available() && field.Available() && field.input.owner == allocations.input.owner && field.receipt == allocations.input.allocationReceipt && allocations.Owns(field.allocation)
}

func (allocation Allocation) Available() bool {
	return allocation.input.Available() && allocation.receipt == allocation.input.allocationReceipt && allocation.index >= 0 && allocation.index < len(allocation.receipt.rows) && allocation.receipt.rows[allocation.index].allocation == allocation.allocation
}

func (allocation Allocation) Role() AllocationRole {
	if !allocation.Available() {
		return AllocationInvalid
	}
	return allocation.receipt.rows[allocation.index].role
}

func (allocation Allocation) Form() AllocationForm {
	if !allocation.Available() {
		return AllocationFormInvalid
	}
	return allocation.receipt.rows[allocation.index].form
}

func (allocation Allocation) ID() identity.ContentID {
	if !allocation.Available() {
		return identity.ContentID{}
	}
	return allocation.receipt.rows[allocation.index].template.ID()
}

// Template returns the owner-neutral constructor identity for this exact
// authored allocation. The returned value carries no source term, Link, or
// mount/shard coordinate.
func (allocation Allocation) Template() AllocationTemplate {
	if !allocation.Available() {
		return AllocationTemplate{}
	}
	return allocation.receipt.rows[allocation.index].template
}

// SemanticOccurrence returns the exact owner-issued non-Term occurrence
// anchor carried by this allocation's template.
func (allocation Allocation) SemanticOccurrence() SemanticOccurrence {
	if !allocation.Available() {
		return SemanticOccurrence{}
	}
	return allocation.receipt.rows[allocation.index].template.occurrence
}

func (allocation Allocation) Owns(input TransformerInput) bool {
	return input.Available() && allocation.Available() && allocation.input.owner == input.owner && allocation.receipt == input.allocationReceipt
}

// ClosureTarget returns the exact Program-issued target projection for a
// closure allocation. Tables and foreign/replayed allocations fail closed.
func (allocation Allocation) ClosureTarget() (AllocationTarget, bool) {
	if !allocation.Available() || allocation.Role() != AllocationClosure {
		return AllocationTarget{}, false
	}
	row := allocation.receipt.rows[allocation.index]
	target := AllocationTarget{allocation: allocation, body: row.target.body, function: row.target.function, callTarget: row.target.callTarget}
	return target, target.Available()
}

func (target AllocationTarget) Available() bool {
	input := target.allocation.input
	body, bodyOK := target.function.Body()
	issuedCallTarget, callTargetOK := target.body.CallTarget()
	if !input.Available() || !target.allocation.Available() || target.allocation.Role() != AllocationClosure || !target.allocation.Owns(input) || target.body.input != input || !input.OwnsBody(target.body) || target.function.body.input != input || !input.OwnsFunction(target.function) || !target.body.Available() || !target.function.Available() || !bodyOK || !body.Equal(target.body) || !target.callTarget.Valid() || !callTargetOK || issuedCallTarget.ContextID() != target.callTarget.ContextID() {
		return false
	}
	row := target.allocation.receipt.rows[target.allocation.index].target
	return row.body == target.body && row.function == target.function && row.callTarget == target.callTarget
}

func (target AllocationTarget) Allocation() (Allocation, bool) {
	if !target.Available() {
		return Allocation{}, false
	}
	return target.allocation, true
}

func (target AllocationTarget) Body() (Body, bool) {
	if !target.Available() {
		return Body{}, false
	}
	return target.body, true
}

func (target AllocationTarget) Function() (Function, bool) {
	if !target.Available() {
		return Function{}, false
	}
	return target.function, true
}

func (target AllocationTarget) CallTarget() (CallBodyTarget, bool) {
	if !target.Available() {
		return CallBodyTarget{}, false
	}
	return target.callTarget, true
}

func (allocation Allocation) FieldCount() int {
	if !allocation.Available() {
		return 0
	}
	return len(allocation.receipt.rows[allocation.index].fields)
}

func (allocation Allocation) FieldAt(index int) (AllocationField, bool) {
	if !allocation.Available() {
		return AllocationField{}, false
	}
	if index < 0 || index >= allocation.FieldCount() {
		return AllocationField{}, false
	}
	row := allocation.receipt.rows[allocation.index]
	result := AllocationField{input: allocation.input, allocation: allocation, field: row.fields[index].field, receipt: allocation.receipt, index: index}
	return result, result.Available()
}

// Geometry returns the exact authored root/span projection for this
// allocation. Invalid allocations expose an unavailable projection.
func (allocation Allocation) Geometry() AllocationGeometry {
	if !allocation.Available() {
		return AllocationGeometry{}
	}
	return AllocationGeometry{allocation: allocation}
}

func (geometry AllocationGeometry) Available() bool {
	return geometry.allocation.Available()
}

func (geometry AllocationGeometry) Allocation() Allocation {
	if !geometry.Available() {
		return Allocation{}
	}
	return geometry.allocation
}

func (geometry AllocationGeometry) Role() AllocationRole {
	if !geometry.Available() {
		return AllocationInvalid
	}
	return geometry.allocation.Role()
}

func (geometry AllocationGeometry) Form() AllocationForm {
	if !geometry.Available() {
		return AllocationFormInvalid
	}
	return geometry.allocation.Form()
}

// RootTerm is the sealed authored root coordinate consumed by cold Link
// adapters. It is not an Allocation identity or a caller-mintable handle.
func (geometry AllocationGeometry) RootTerm() (keyspace.Term, bool) {
	if !geometry.Available() {
		return 0, false
	}
	return geometry.allocation.receipt.rows[geometry.allocation.index].term, true
}

// RootSpan is the exact typed Program value occurrence for this allocation
// root. It is the artifact compiler's portable binding projection; consumers
// never need to recover the authored root Term.
func (geometry AllocationGeometry) RootSpan() (Span, bool) {
	if !geometry.Available() {
		return Span{}, false
	}
	term, ok := geometry.RootTerm()
	if !ok {
		return Span{}, false
	}
	span, ok := geometry.allocation.input.Span(term)
	return span, ok && geometry.allocation.input.OwnsSpan(span)
}

func (geometry AllocationGeometry) FieldCount() int {
	if !geometry.Available() {
		return 0
	}
	return geometry.allocation.FieldCount()
}

func (geometry AllocationGeometry) Span() AllocationSpan {
	if !geometry.Available() {
		return AllocationSpan{}
	}
	row := geometry.allocation.receipt.rows[geometry.allocation.index]
	return AllocationSpan{input: geometry.allocation.input, entry: row.entry, finish: row.finish}
}

func (span AllocationSpan) Available() bool {
	return span.input.Available() && span.entry.Available() && span.finish.Available()
}

func (span AllocationSpan) Entry() (flow.Site, bool) {
	if !span.Available() {
		return flow.Site{}, false
	}
	return span.entry, true
}

func (span AllocationSpan) Finish() (flow.Site, bool) {
	if !span.Available() {
		return flow.Site{}, false
	}
	return span.finish, true
}

func (geometry AllocationGeometry) FieldAt(index int) (AllocationFieldGeometry, bool) {
	field, ok := geometry.allocation.FieldAt(index)
	if !ok || !geometry.Available() {
		return AllocationFieldGeometry{}, false
	}
	return AllocationFieldGeometry{field: field, receipt: geometry.allocation.receipt, allocation: geometry.allocation, index: index}, true
}

func (field AllocationFieldGeometry) Available() bool {
	return field.field.Available() && field.receipt == field.field.input.allocationReceipt && field.index >= 0 && field.index < len(field.receipt.rows[field.allocation.index].fields)
}

func (field AllocationFieldGeometry) BelongsTo(geometry AllocationGeometry) bool {
	return field.Available() && geometry.Available() && field.field.Allocation() == geometry.allocation
}

func (field AllocationFieldGeometry) Allocation() Allocation {
	if !field.Available() {
		return Allocation{}
	}
	return field.field.Allocation()
}

// Field returns the exact opaque allocation-field proof backing this geometry.
// It exposes no source coordinate and is suitable for semantic catalog rows.
func (field AllocationFieldGeometry) Field() (AllocationField, bool) {
	if !field.Available() {
		return AllocationField{}, false
	}
	return field.field, true
}

func (field AllocationFieldGeometry) FieldTerm() (keyspace.Term, bool) {
	if !field.Available() {
		return 0, false
	}
	return field.receipt.rows[field.allocation.index].fields[field.index].term, true
}

// FieldSpan is the exact typed Program occurrence for the field expression.
func (field AllocationFieldGeometry) FieldSpan() (Span, bool) {
	if !field.Available() {
		return Span{}, false
	}
	term, ok := field.FieldTerm()
	if !ok {
		return Span{}, false
	}
	span, ok := field.field.input.Span(term)
	return span, ok && field.field.input.OwnsSpan(span)
}

func (field AllocationFieldGeometry) Kind() (flowkind.FieldKind, bool) {
	if !field.Available() {
		return 0, false
	}
	return field.receipt.rows[field.allocation.index].fields[field.index].kind, true
}

func (field AllocationFieldGeometry) SelectorTerm() (keyspace.Term, bool) {
	if !field.Available() {
		return 0, false
	}
	selector := field.receipt.rows[field.allocation.index].fields[field.index].selector
	return selector, selector != 0
}

// SelectorSpan is the exact typed Program occurrence for a dynamic field
// selector. Exact/list/name fields have no selector evaluation span.
func (field AllocationFieldGeometry) SelectorSpan() (Span, bool) {
	if !field.Available() {
		return Span{}, false
	}
	if kind, ok := field.Kind(); !ok || kind != flowkind.FieldKey {
		return Span{}, false
	}
	term, ok := field.SelectorTerm()
	if !ok {
		return Span{}, false
	}
	span, ok := field.field.input.Span(term)
	return span, ok && field.field.input.OwnsSpan(span)
}

func (field AllocationFieldGeometry) NormalizedKey() (keyspace.Key, bool) {
	if !field.Available() {
		return 0, false
	}
	row := field.receipt.rows[field.allocation.index].fields[field.index]
	return row.normalized, row.normalizedOK
}

func (field AllocationFieldGeometry) Values() (keyspace.Term, int, bool, bool) {
	if !field.Available() {
		return 0, 0, false, false
	}
	row := field.receipt.rows[field.allocation.index].fields[field.index]
	return row.values, row.width, row.finalOpen, row.valuesOK
}

// ValueOccurrence is the exact sealed Values proof for this field. It is the
// proof-native replacement for consumers that previously reopened Values()
// and joined its raw coordinate themselves.
func (field AllocationFieldGeometry) ValueOccurrence() (ValuesOccurrence, bool) {
	if !field.Available() {
		return ValuesOccurrence{}, false
	}
	row := field.receipt.rows[field.allocation.index].fields[field.index]
	if !row.valuesOK {
		return ValuesOccurrence{}, false
	}
	values, ok := field.field.input.valuesForTerm(row.values)
	return values, ok && field.field.input.OwnsValuesOccurrence(values) && values.Count() == row.width
}

// ValueAt returns one ordered existing Values member under this field.
func (field AllocationFieldGeometry) ValueAt(index int) (ValuesMember, bool) {
	values, ok := field.ValueOccurrence()
	if !ok {
		return ValuesMember{}, false
	}
	member, memberOK := values.At(index)
	return member, memberOK && field.field.input.OwnsValuesMember(member)
}

// SharesFirstValueCell reports whether a dynamic field selector and its first
// value member are distinct Reads of the same table-owner Cell.  The raw
// authored relation is consumed here while TransformerInput still owns it;
// callers receive only this sealed semantic fact.
func (field AllocationFieldGeometry) SharesFirstValueCell() bool {
	if !field.Available() {
		return false
	}
	kind, kindOK := field.Kind()
	fieldTerm, fieldOK := field.FieldTerm()
	selector, selectorOK := field.SelectorTerm()
	values, width, _, valuesOK := field.Values()
	if !kindOK || kind != flowkind.FieldKey || !fieldOK || !selectorOK || !valuesOK || width <= 0 {
		return false
	}
	flowView := field.field.input.owner.Flow()
	table, source, _, _, fieldGeometryOK := flowView.Authored().Fields().Get(fieldTerm)
	member, memberOK := flowView.Authored().Values().Member(values, 0)
	if !fieldGeometryOK || source != selector || !memberOK {
		return false
	}
	reads := flowView.Authored().Storage().Reads()
	leftOwner, leftCell, _, leftOK := reads.Get(selector)
	rightOwner, rightCell, _, rightOK := reads.Get(member)
	if !leftOK || !rightOK || leftOwner != table || rightOwner != table ||
		keyspace.TermFamily(leftCell) != keyspace.FamilyCell || keyspace.TermOrdinal(leftCell) == 0 ||
		keyspace.TermFamily(rightCell) != keyspace.FamilyCell || keyspace.TermOrdinal(rightCell) == 0 {
		return false
	}
	return leftCell == rightCell
}

func (field AllocationField) Available() bool {
	return field.input.Available() && field.receipt == field.input.allocationReceipt && field.allocation.Available() && field.index >= 0 && field.index < len(field.receipt.rows[field.allocation.index].fields) && field.receipt.rows[field.allocation.index].fields[field.index].field == field.field
}

func (field AllocationField) Owns(input TransformerInput) bool {
	return input.Available() && field.Available() && field.input.owner == input.owner && field.receipt == input.allocationReceipt
}

func (field AllocationField) Allocation() Allocation {
	if !field.Available() {
		return Allocation{}
	}
	return field.allocation
}

func (field AllocationField) ID() identity.ContentID {
	if !field.Available() || !field.field.BelongsTo(field.allocation.allocation) {
		return identity.ContentID{}
	}
	child := field.receipt.rows[field.allocation.index].fields[field.index].field.ID()
	programID := field.input.ContentID()
	var payload [fieldProgramIDPrefixLen + sha256.Size + sha256.Size]byte
	copy(payload[:fieldProgramIDPrefixLen], fieldProgramIDPrefix)
	copy(payload[fieldProgramIDPrefixLen:fieldProgramIDPrefixLen+sha256.Size], programID[:])
	copy(payload[fieldProgramIDPrefixLen+sha256.Size:], child[:])
	return sha256.Sum256(payload[:])
}

func (field AllocationField) SourceTerm() (keyspace.Term, bool) {
	if !field.Available() {
		return 0, false
	}
	return field.receipt.rows[field.allocation.index].fields[field.index].term, true
}

const (
	allocationProgramIDPrefix    = "program-allocation-v1"
	allocationProgramIDPrefixLen = len(allocationProgramIDPrefix)
	fieldProgramIDPrefix         = "program-allocation-field-v1"
	fieldProgramIDPrefixLen      = len(fieldProgramIDPrefix)
)
