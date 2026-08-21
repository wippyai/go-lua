package modulecomposition

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/snapshot"
)

func ImportDenominatorID(linkID identity.ContentID) (identity.ContentID, bool) {
	return denominatorID("imports", linkID)
}
func CacheDenominatorID(linkID identity.ContentID) (identity.ContentID, bool) {
	return denominatorID("cache-ingresses", linkID)
}
func GenerationDenominatorID(linkID identity.ContentID) (identity.ContentID, bool) {
	return denominatorID("init-generations", linkID)
}
func OutcomeDenominatorID(linkID identity.ContentID) (identity.ContentID, bool) {
	return denominatorID("init-outcomes", linkID)
}
func TerminalDenominatorID(linkID identity.ContentID) (identity.ContentID, bool) {
	return denominatorID("init-terminals", linkID)
}

func denominatorID(family string, linkID identity.ContentID) (identity.ContentID, bool) {
	if family == "" || !linkID.Available() {
		return identity.ContentID{}, false
	}
	return identity.DeriveContentID("analysis/schema/module-composition/denominator/v1/"+family, idPart(linkID))
}

func ImportContent(rows []ResolvedImport, denominator identity.ContentID) (snapshot.Content[identity.ContentID, ResolvedImport], bool) {
	return content(rows, denominator, func(row ResolvedImport) identity.ContentID { return row.ID() }, func(row ResolvedImport) bool { return row.Available() })
}
func CacheContent(rows []CacheIngress, denominator identity.ContentID) (snapshot.Content[identity.ContentID, CacheIngress], bool) {
	return content(rows, denominator, func(row CacheIngress) identity.ContentID { return row.ID() }, func(row CacheIngress) bool { return row.Available() })
}
func GenerationContent(rows []InitGeneration, denominator identity.ContentID) (snapshot.Content[identity.ContentID, InitGeneration], bool) {
	return content(rows, denominator, func(row InitGeneration) identity.ContentID { return row.ID() }, func(row InitGeneration) bool { return row.Available() })
}
func OutcomeContent(rows []InitOutcome, denominator identity.ContentID) (snapshot.Content[identity.ContentID, InitOutcome], bool) {
	return content(rows, denominator, func(row InitOutcome) identity.ContentID { return row.ID() }, func(row InitOutcome) bool { return row.Available() })
}
func TerminalContent(rows []InitTerminal, denominator identity.ContentID) (snapshot.Content[identity.ContentID, InitTerminal], bool) {
	return content(rows, denominator, func(row InitTerminal) identity.ContentID { return row.ID() }, func(row InitTerminal) bool { return row.Available() })
}

func content[V any](rows []V, denominator identity.ContentID, key func(V) identity.ContentID, available func(V) bool) (snapshot.Content[identity.ContentID, V], bool) {
	if !denominator.Available() {
		return snapshot.Content[identity.ContentID, V]{}, false
	}
	byID := make(map[identity.ContentID]V, len(rows))
	members := make([]identity.ContentID, 0, len(rows))
	for _, row := range rows {
		if !available(row) {
			return snapshot.Content[identity.ContentID, V]{}, false
		}
		id := key(row)
		if _, duplicate := byID[id]; duplicate {
			return snapshot.Content[identity.ContentID, V]{}, false
		}
		byID[id] = row
		members = append(members, id)
	}
	identity.SortContentIDs(members)
	return snapshot.Content[identity.ContentID, V]{Rows: byID, Denominator: denominator, Members: members}, true
}

func ImportAxis(runtimeSchema identity.ContentID, slot uint32) snapshot.Axis[identity.ContentID, ResolvedImport] {
	return snapshot.Axis[identity.ContentID, ResolvedImport]{SchemaID: runtimeSchema, Slot: slot}
}
func CacheAxis(runtimeSchema identity.ContentID, slot uint32) snapshot.Axis[identity.ContentID, CacheIngress] {
	return snapshot.Axis[identity.ContentID, CacheIngress]{SchemaID: runtimeSchema, Slot: slot}
}
func GenerationAxis(runtimeSchema identity.ContentID, slot uint32) snapshot.Axis[identity.ContentID, InitGeneration] {
	return snapshot.Axis[identity.ContentID, InitGeneration]{SchemaID: runtimeSchema, Slot: slot}
}
func OutcomeAxis(runtimeSchema identity.ContentID, slot uint32) snapshot.Axis[identity.ContentID, InitOutcome] {
	return snapshot.Axis[identity.ContentID, InitOutcome]{SchemaID: runtimeSchema, Slot: slot}
}
func TerminalAxis(runtimeSchema identity.ContentID, slot uint32) snapshot.Axis[identity.ContentID, InitTerminal] {
	return snapshot.Axis[identity.ContentID, InitTerminal]{SchemaID: runtimeSchema, Slot: slot}
}

func ImportAt(published *snapshot.Snapshot, address snapshot.Axis[identity.ContentID, ResolvedImport], key identity.ContentID) (ResolvedImport, bool) {
	row, status := snapshot.Read(published, address, key)
	return row, status == snapshot.ReadHit && row.Available()
}
func CacheAt(published *snapshot.Snapshot, address snapshot.Axis[identity.ContentID, CacheIngress], key identity.ContentID) (CacheIngress, bool) {
	row, status := snapshot.Read(published, address, key)
	return row, status == snapshot.ReadHit && row.Available()
}
func GenerationAt(published *snapshot.Snapshot, address snapshot.Axis[identity.ContentID, InitGeneration], key identity.ContentID) (InitGeneration, bool) {
	row, status := snapshot.Read(published, address, key)
	return row, status == snapshot.ReadHit && row.Available()
}
func OutcomeAt(published *snapshot.Snapshot, address snapshot.Axis[identity.ContentID, InitOutcome], key identity.ContentID) (InitOutcome, bool) {
	row, status := snapshot.Read(published, address, key)
	return row, status == snapshot.ReadHit && row.Available()
}
func TerminalAt(published *snapshot.Snapshot, address snapshot.Axis[identity.ContentID, InitTerminal], key identity.ContentID) (InitTerminal, bool) {
	row, status := snapshot.Read(published, address, key)
	return row, status == snapshot.ReadHit && row.Available()
}
