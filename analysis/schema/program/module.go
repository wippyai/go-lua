package programschema

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// ModuleImport is one compiled authored import occurrence. The request is a
// child row because its exact Source key is derived rather than authored.
// Optional aliases are authenticated explicitly; a zero identity never means
// both "absent" and "unavailable".
type ModuleImport struct {
	id, call, alias identity.ContentID
	requestOffset   uint32
	requestCount    uint32
	hasAlias        bool
}

func NewModuleImport(id, call, alias identity.ContentID, requestOffset, requestCount uint32, hasAlias bool) (ModuleImport, bool) {
	row := ModuleImport{id: id, call: call, alias: alias, requestOffset: requestOffset, requestCount: requestCount, hasAlias: hasAlias}
	return row, row.Available()
}

func (row ModuleImport) Available() bool {
	return row.id.Available() && row.call.Available() && row.requestCount == 1 &&
		moduleSpanFits(row.requestOffset, row.requestCount) && row.hasAlias == row.alias.Available()
}
func (row ModuleImport) ID() identity.ContentID {
	if row.Available() {
		return row.id
	}
	return identity.ContentID{}
}
func (row ModuleImport) CallID() identity.ContentID {
	if row.Available() {
		return row.call
	}
	return identity.ContentID{}
}
func (row ModuleImport) AliasID() (identity.ContentID, bool) {
	if !row.Available() || !row.hasAlias {
		return identity.ContentID{}, false
	}
	return row.alias, true
}
func (row ModuleImport) RequestSpan() (uint32, uint32, bool) {
	return row.requestOffset, row.requestCount, row.Available()
}

// ModuleRequest is the sole compiled request witness. It joins one Import to
// the authored String value and its Source-owned exact key.
type ModuleRequest struct {
	id, imported, value identity.ContentID
	key                 keyspace.Key
}

func NewModuleRequest(id, imported, value identity.ContentID, key keyspace.Key) (ModuleRequest, bool) {
	row := ModuleRequest{id: id, imported: imported, value: value, key: key}
	return row, row.Available()
}
func (row ModuleRequest) Available() bool {
	return row.id.Available() && row.imported.Available() && row.value.Available() && row.key != 0
}
func (row ModuleRequest) ID() identity.ContentID {
	if row.Available() {
		return row.id
	}
	return identity.ContentID{}
}
func (row ModuleRequest) ImportID() identity.ContentID {
	if row.Available() {
		return row.imported
	}
	return identity.ContentID{}
}
func (row ModuleRequest) ValueID() identity.ContentID {
	if row.Available() {
		return row.value
	}
	return identity.ContentID{}
}
func (row ModuleRequest) Key() keyspace.Key {
	if row.Available() {
		return row.key
	}
	return 0
}

// ModuleEntry is one retained chunk Return. Its sparse children occupy three
// independent dense families; original value positions live on child rows.
type ModuleEntry struct {
	id, returned                          identity.ContentID
	returnOrdinal, rootWidth              uint32
	rootCellOffset, rootCellCount         uint32
	rootFunctionOffset, rootFunctionCount uint32
	memberOffset, memberCount             uint32
}

// NewModuleEntry copies one retained Return row. returnOrdinal is the
// authored Return ordinal (one-based), while rootWidth is the fixed number of
// Values slots in that Return. Root children are sparse sidecars: their own
// spans count only present Cell/Function witnesses, and each child retains its
// original slot in Position().
func NewModuleEntry(id, returned identity.ContentID, returnOrdinal, rootWidth, rootCellOffset, rootCellCount, rootFunctionOffset, rootFunctionCount, memberOffset, memberCount uint32) (ModuleEntry, bool) {
	row := ModuleEntry{
		id: id, returned: returned, returnOrdinal: returnOrdinal, rootWidth: rootWidth,
		rootCellOffset: rootCellOffset, rootCellCount: rootCellCount,
		rootFunctionOffset: rootFunctionOffset, rootFunctionCount: rootFunctionCount,
		memberOffset: memberOffset, memberCount: memberCount,
	}
	return row, row.Available()
}
func (row ModuleEntry) Available() bool {
	return row.id.Available() && row.returned.Available() && row.returnOrdinal != 0 &&
		row.rootCellCount <= row.rootWidth && row.rootFunctionCount <= row.rootWidth &&
		moduleSpanFits(row.rootCellOffset, row.rootCellCount) &&
		moduleSpanFits(row.rootFunctionOffset, row.rootFunctionCount) &&
		moduleSpanFits(row.memberOffset, row.memberCount)
}
func (row ModuleEntry) ID() identity.ContentID {
	if row.Available() {
		return row.id
	}
	return identity.ContentID{}
}
func (row ModuleEntry) ReturnID() identity.ContentID {
	if row.Available() {
		return row.returned
	}
	return identity.ContentID{}
}

// ReturnOrdinal is the retained one-based ordinal of the authored Return.
// It is kept independently of ReturnID so a consumer never has to reverse an
// identity to recover the position that selected this entry.
func (row ModuleEntry) ReturnOrdinal() (uint32, bool) {
	return row.returnOrdinal, row.Available()
}

// RootWidth is the fixed root Values-slot width. It is intentionally distinct
// from the sparse RootCell/RootFunction spans.
func (row ModuleEntry) RootWidth() (uint32, bool) {
	return row.rootWidth, row.Available()
}
func (row ModuleEntry) RootCellSpan() (uint32, uint32, bool) {
	return row.rootCellOffset, row.rootCellCount, row.Available()
}
func (row ModuleEntry) RootFunctionSpan() (uint32, uint32, bool) {
	return row.rootFunctionOffset, row.rootFunctionCount, row.Available()
}
func (row ModuleEntry) MemberSpan() (uint32, uint32, bool) {
	return row.memberOffset, row.memberCount, row.Available()
}

type ModuleEntryRootCell struct {
	id, entry, cell identity.ContentID
	position        uint32
}

func NewModuleEntryRootCell(id, entry, cell identity.ContentID, position uint32) (ModuleEntryRootCell, bool) {
	row := ModuleEntryRootCell{id: id, entry: entry, cell: cell, position: position}
	return row, row.Available()
}
func (row ModuleEntryRootCell) Available() bool {
	return row.id.Available() && row.entry.Available() && row.cell.Available()
}
func (row ModuleEntryRootCell) ID() identity.ContentID {
	if row.Available() {
		return row.id
	}
	return identity.ContentID{}
}
func (row ModuleEntryRootCell) EntryID() identity.ContentID {
	if row.Available() {
		return row.entry
	}
	return identity.ContentID{}
}
func (row ModuleEntryRootCell) CellID() identity.ContentID {
	if row.Available() {
		return row.cell
	}
	return identity.ContentID{}
}
func (row ModuleEntryRootCell) Position() uint32 {
	if row.Available() {
		return row.position
	}
	return 0
}

type ModuleEntryRootFunction struct {
	id, entry, function identity.ContentID
	position            uint32
}

func NewModuleEntryRootFunction(id, entry, function identity.ContentID, position uint32) (ModuleEntryRootFunction, bool) {
	row := ModuleEntryRootFunction{id: id, entry: entry, function: function, position: position}
	return row, row.Available()
}
func (row ModuleEntryRootFunction) Available() bool {
	return row.id.Available() && row.entry.Available() && row.function.Available()
}
func (row ModuleEntryRootFunction) ID() identity.ContentID {
	if row.Available() {
		return row.id
	}
	return identity.ContentID{}
}
func (row ModuleEntryRootFunction) EntryID() identity.ContentID {
	if row.Available() {
		return row.entry
	}
	return identity.ContentID{}
}
func (row ModuleEntryRootFunction) FunctionID() identity.ContentID {
	if row.Available() {
		return row.function
	}
	return identity.ContentID{}
}
func (row ModuleEntryRootFunction) Position() uint32 {
	if row.Available() {
		return row.position
	}
	return 0
}

// ModuleEntryMember is one exact named table-surface step. Value is present
// only on a final Function leaf; the explicit bit prevents zero laundering.
type ModuleEntryMember struct {
	id, field, parent, value, entry, table identity.ContentID
	suffix                                 keyspace.Key
	position                               uint32
	hasValue                               bool
}

func NewModuleEntryMember(id, field, parent, value, entry, table identity.ContentID, suffix keyspace.Key, position uint32, hasValue bool) (ModuleEntryMember, bool) {
	row := ModuleEntryMember{id: id, field: field, parent: parent, value: value, entry: entry, table: table, suffix: suffix, position: position, hasValue: hasValue}
	return row, row.Available()
}
func (row ModuleEntryMember) Available() bool {
	return row.id.Available() && row.field.Available() && row.parent.Available() && row.entry.Available() && row.table.Available() && row.suffix != 0 && row.hasValue == row.value.Available()
}
func (row ModuleEntryMember) ID() identity.ContentID {
	if row.Available() {
		return row.id
	}
	return identity.ContentID{}
}
func (row ModuleEntryMember) FieldID() identity.ContentID {
	if row.Available() {
		return row.field
	}
	return identity.ContentID{}
}
func (row ModuleEntryMember) ParentID() identity.ContentID {
	if row.Available() {
		return row.parent
	}
	return identity.ContentID{}
}
func (row ModuleEntryMember) ValueID() (identity.ContentID, bool) {
	if !row.Available() || !row.hasValue {
		return identity.ContentID{}, false
	}
	return row.value, true
}
func (row ModuleEntryMember) EntryID() identity.ContentID {
	if row.Available() {
		return row.entry
	}
	return identity.ContentID{}
}
func (row ModuleEntryMember) TableID() identity.ContentID {
	if row.Available() {
		return row.table
	}
	return identity.ContentID{}
}
func (row ModuleEntryMember) Suffix() keyspace.Key {
	if row.Available() {
		return row.suffix
	}
	return 0
}
func (row ModuleEntryMember) Position() uint32 {
	if row.Available() {
		return row.position
	}
	return 0
}

func moduleSpanFits(offset, count uint32) bool {
	return uint64(offset)+uint64(count) <= uint64(^uint32(0))
}
