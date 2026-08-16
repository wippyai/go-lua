package flow

import (
	"crypto/sha256"
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// AllocationRole is the authored Program constructor family. Target fresh
// results and runtime escape state are deliberately outside this vocabulary.
type AllocationRole uint8

const (
	AllocationInvalid AllocationRole = iota
	AllocationTable
	AllocationClosure
)

func (role AllocationRole) Valid() bool {
	return role == AllocationTable || role == AllocationClosure
}

// AllocationForm is the sealed constructor geometry of one executable
// allocation occurrence. Empty covers closures and zero-field tables.
type AllocationForm uint8

const (
	AllocationFormInvalid AllocationForm = iota
	AllocationFormEmpty
	AllocationFormClosed
	AllocationFormFinalOpen
)

func (form AllocationForm) Valid() bool {
	return form >= AllocationFormEmpty && form <= AllocationFormFinalOpen
}

// Allocations is a view over the existing authored Table and Function
// denominators. It retains no allocation table, slice, or map.
type Allocations struct{ view View }

// Allocation is an opaque executable Program allocation proof. Its private
// source coordinate is never exposed as a Term or ordinal.
type Allocation struct {
	owner   *Component
	program identity.ContentID
	role    AllocationRole
	term    keyspace.Term
}

// AllocationField is an opaque ordered field proof belonging to one table
// Allocation. It carries no public key, Term, ordinal, or domain coordinate.
type AllocationField struct {
	owner      *Component
	program    identity.ContentID
	allocation Allocation
	term       keyspace.Term
	position   int
}

// Allocations returns the canonical executable allocation view for this Flow.
func (view View) Allocations() Allocations {
	if !view.available() {
		return Allocations{}
	}
	return Allocations{view: view}
}

// Count is the executable allocation denominator: authored tables precede
// authored closures, matching the existing Flow/Heap source order.
func (allocations Allocations) Count() int {
	if !allocations.view.available() {
		return 0
	}
	authored := allocations.view.Authored()
	executable := allocations.view.Executable()
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
	return count
}

// Visit emits the canonical executable allocation denominator in one pass.
// It is a cold sealing primitive; callers must not retain or mutate the
// supplied proof outside their owner-fenced receipt.
func (allocations Allocations) Visit(visit func(Allocation) bool) bool {
	if !allocations.view.available() || visit == nil {
		return false
	}
	authored := allocations.view.Authored()
	executable := allocations.view.Executable()
	for position := 0; position < authored.Tables().Count(); position++ {
		term, ok := authored.Tables().At(position)
		if !ok || !executable.Contains(term) {
			continue
		}
		if !visit(Allocation{owner: allocations.view.component, program: allocations.view.ContentID(), role: AllocationTable, term: term}) {
			return false
		}
	}
	for position := 0; position < authored.Functions().Count(); position++ {
		term, ok := authored.Functions().At(position)
		if !ok || !executable.Contains(term) {
			continue
		}
		if !visit(Allocation{owner: allocations.view.component, program: allocations.view.ContentID(), role: AllocationClosure, term: term}) {
			return false
		}
	}
	return true
}

// Owns accepts only a proof issued by this exact hot Flow owner. Equivalent
// replay has the same ID but is intentionally not owner-identical.
func (allocations Allocations) Owns(allocation Allocation) bool {
	return allocations.ownsAllocation(allocation)
}

// OwnsField is the exact hot owner fence for an ordered field proof.
func (allocations Allocations) OwnsField(field AllocationField) bool {
	return allocations.view.available() && field.availableIn(allocations.view) && allocations.Owns(field.allocation)
}

func (allocations Allocations) ownsAllocation(allocation Allocation) bool {
	if !allocations.view.available() || !allocation.Valid() || allocation.owner != allocations.view.component || allocation.program != allocations.view.ContentID() {
		return false
	}
	authored := allocations.view.Authored()
	executable := allocations.view.Executable()
	if allocation.role == AllocationTable {
		_, ok := authored.Tables().Get(allocation.term)
		return ok && executable.Contains(allocation.term)
	}
	if allocation.role == AllocationClosure {
		_, _, _, ok := authored.Functions().Get(allocation.term)
		return ok && executable.Contains(allocation.term)
	}
	return false
}

// At returns one executable allocation in canonical authored order. It scans
// the existing dense denominators and creates no secondary index.
func (allocations Allocations) At(index int) (Allocation, bool) {
	if !allocations.view.available() || index < 0 {
		return Allocation{}, false
	}
	authored := allocations.view.Authored()
	executable := allocations.view.Executable()
	seen := 0
	for position := 0; position < authored.Tables().Count(); position++ {
		term, ok := authored.Tables().At(position)
		if !ok || !executable.Contains(term) {
			continue
		}
		if seen == index {
			return Allocation{owner: allocations.view.component, program: allocations.view.ContentID(), role: AllocationTable, term: term}, true
		}
		seen++
	}
	for position := 0; position < authored.Functions().Count(); position++ {
		term, ok := authored.Functions().At(position)
		if !ok || !executable.Contains(term) {
			continue
		}
		if seen == index {
			return Allocation{owner: allocations.view.component, program: allocations.view.ContentID(), role: AllocationClosure, term: term}, true
		}
		seen++
	}
	return Allocation{}, false
}

// Valid reports whether this is a nonzero owner-issued proof handle.
func (allocation Allocation) Valid() bool {
	return allocation.owner != nil && allocation.program.Available() && allocation.role.Valid() && allocation.term != 0
}

// Available reports whether this is a nonzero owner-issued proof handle.
func (allocation Allocation) Available() bool {
	return allocation.Valid() && allocation.Owns(View{component: allocation.owner})
}

// Owns is the exact hot owner fence. It performs no authored-topology
// reconstruction and rejects an equivalent replay owned by another Flow.
func (allocation Allocation) Owns(view View) bool {
	return Allocations{view: view}.ownsAllocation(allocation)
}

func (allocation Allocation) Role() AllocationRole {
	if !allocation.Owns(View{component: allocation.owner}) {
		return AllocationInvalid
	}
	return allocation.role
}

// Occurrence returns the exact Source/Flow-issued structural path for this
// allocation. The receipt is opaque and contains no source coordinate.
func (allocation Allocation) Occurrence() AllocationOccurrence {
	if !allocation.Available() {
		return AllocationOccurrence{}
	}
	return allocation.owner.allocationOccurrence(allocation.term, allocation.role)
}

// ID is stable across equivalent Flow replay and fenced by the Flow
// ContentID. It is not a serialization of the private source coordinate.
func (allocation Allocation) ID() identity.ContentID {
	if !allocation.Available() {
		return identity.ContentID{}
	}
	var payload [allocationIDPrefixLen + sha256.Size + 1 + 4]byte
	copy(payload[:allocationIDPrefixLen], allocationIDPrefix)
	copy(payload[allocationIDPrefixLen:allocationIDPrefixLen+sha256.Size], allocation.program[:])
	payload[allocationIDPrefixLen+sha256.Size] = byte(allocation.role)
	binary.BigEndian.PutUint32(payload[allocationIDPrefixLen+sha256.Size+1:], uint32(allocation.term))
	return sha256.Sum256(payload[:])
}

// SourceTerm is a cold-only adapter coordinate for domain projections. It is
// not used by Allocation identity or ownership checks.
func (allocation Allocation) SourceTerm() (keyspace.Term, bool) {
	if !allocation.Available() {
		return 0, false
	}
	return allocation.term, true
}

// Form returns the cold constructor geometry from the existing authored
// Tables/Fields/Values relations. Invalid or foreign handles fail closed.
func (allocation Allocation) Form() AllocationForm {
	if !allocation.Available() {
		return AllocationFormInvalid
	}
	view := View{component: allocation.owner}
	if !view.available() || allocation.program != view.ContentID() {
		return AllocationFormInvalid
	}
	if allocation.role == AllocationClosure {
		return AllocationFormEmpty
	}
	authored := view.Authored()
	if _, ok := authored.Tables().Get(allocation.term); !ok {
		return AllocationFormInvalid
	}
	count, ok := authored.Tables().FieldCount(allocation.term)
	if !ok {
		return AllocationFormInvalid
	}
	if count == 0 {
		return AllocationFormEmpty
	}
	open := false
	for index := 0; index < count; index++ {
		field, fieldOK := authored.Tables().FieldAt(allocation.term, index)
		if !fieldOK {
			return AllocationFormInvalid
		}
		table, _, values, _, fieldOK := authored.Fields().Get(field)
		resolved, fieldOpen, valuesOK := authored.Fields().Values(field)
		if !fieldOK || table != allocation.term || !valuesOK || resolved != values {
			return AllocationFormInvalid
		}
		if fieldOpen {
			open = true
			continue
		}
		width, widthOK := authored.Values().Len(values)
		if !widthOK || width != 1 {
			return AllocationFormInvalid
		}
	}
	if open {
		return AllocationFormFinalOpen
	}
	return AllocationFormClosed
}

// FieldCount returns the authored field denominator for a table allocation.
func (allocation Allocation) FieldCount() int {
	if !allocation.Owns(View{component: allocation.owner}) || allocation.role != AllocationTable {
		return 0
	}
	view := View{component: allocation.owner}
	count, ok := view.Authored().Tables().FieldCount(allocation.term)
	if !ok {
		return 0
	}
	return count
}

// FieldAt returns one ordered opaque field proof. Closure and invalid
// allocations have no fields.
func (allocation Allocation) FieldAt(index int) (AllocationField, bool) {
	if !allocation.Owns(View{component: allocation.owner}) || allocation.role != AllocationTable || index < 0 {
		return AllocationField{}, false
	}
	view := View{component: allocation.owner}
	field, ok := view.Authored().Tables().FieldAt(allocation.term, index)
	if !ok {
		return AllocationField{}, false
	}
	return AllocationField{owner: allocation.owner, program: allocation.program, allocation: allocation, term: field, position: index}, true
}

func (field AllocationField) Valid() bool {
	return field.owner != nil && field.program.Available() && field.allocation.Valid() && field.term != 0
}

// Available reports whether this is a nonzero owner-issued field proof.
func (field AllocationField) Available() bool {
	return field.Valid() && field.availableIn(View{component: field.owner})
}

func (field AllocationField) availableIn(view View) bool {
	if !field.Valid() || !view.available() || field.owner != view.component || field.program != view.ContentID() || !field.allocation.Owns(view) || field.position < 0 {
		return false
	}
	term, ok := view.Authored().Tables().FieldAt(field.allocation.term, field.position)
	return ok && term == field.term
}

func (field AllocationField) Owns(view View) bool {
	return field.availableIn(view)
}

// BelongsTo proves the field was issued by the exact parent allocation and
// remains at that parent's authored position. It exposes no field coordinate.
func (field AllocationField) BelongsTo(allocation Allocation) bool {
	if !field.Valid() || !allocation.Valid() || field.allocation != allocation || field.owner != allocation.owner || field.program != allocation.program {
		return false
	}
	return field.availableIn(View{component: allocation.owner})
}

func (field AllocationField) Allocation() Allocation {
	if !field.Available() {
		return Allocation{}
	}
	return field.allocation
}

func (field AllocationField) ID() identity.ContentID {
	if !field.Available() {
		return identity.ContentID{}
	}
	parent := field.allocation.ID()
	var payload [fieldIDPrefixLen + sha256.Size + 4]byte
	copy(payload[:fieldIDPrefixLen], fieldIDPrefix)
	copy(payload[fieldIDPrefixLen:fieldIDPrefixLen+sha256.Size], parent[:])
	binary.BigEndian.PutUint32(payload[fieldIDPrefixLen+sha256.Size:], uint32(field.term))
	return sha256.Sum256(payload[:])
}

// SourceTerm is a cold-only adapter coordinate for domain projections. It is
// not used by AllocationField identity or ownership checks.
func (field AllocationField) SourceTerm() (keyspace.Term, bool) {
	if !field.Available() {
		return 0, false
	}
	return field.term, true
}

const (
	allocationIDPrefix    = "program-flow-allocation-v1"
	allocationIDPrefixLen = len(allocationIDPrefix)
	fieldIDPrefix         = "program-flow-allocation-field-v1"
	fieldIDPrefixLen      = len(fieldIDPrefix)
)
