package schedule

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

// digests derives the original logical/physical schedule identities from the
// immutable table. The physical digest differs only by the mounted node
// digest; all recurrence and wake data is shared between both identities.
func digests(data *data) (identity.ContentID, identity.ContentID, bool) {
	logicalParts := make([][]byte, 0, len(data.entries)*8+len(data.components)*4)
	physicalParts := make([][]byte, 0, len(data.entries)*9+len(data.components)*4)
	for _, entry := range data.entries {
		parts := [][]byte{nominalBytes(entry.id.Owner().Content(), entry.id.Content()), nominalBytes(entry.root.Owner().Content(), entry.root.Content())}
		for _, relation := range entry.reads {
			parts = append(parts, nominalBytes(relation.Owner().Content(), relation.Content()))
		}
		parts = append(parts, nil)
		for _, column := range entry.columns {
			parts = append(parts, nominalBytes(column.Relation().Owner().Content(), column.Content()))
		}
		parts = append(parts, nil)
		for _, relation := range entry.writes {
			parts = append(parts, nominalBytes(relation.Owner().Content(), relation.Content()))
		}
		parts = append(parts, nil)
		for _, relation := range entry.heads {
			parts = append(parts, nominalBytes(relation.Owner().Content(), relation.Content()))
		}
		appendUint32(&parts, entry.component)
		logicalParts = append(logicalParts, parts...)
		physicalParts = append(physicalParts, parts...)
		physicalParts = append(physicalParts, contentBytes(entry.node))
	}
	for _, component := range data.components {
		parts := [][]byte{}
		appendUint32(&parts, component.order)
		appendUint32(&parts, uint32(component.recurrence))
		for _, member := range component.members {
			parts = append(parts, nominalBytes(member.Owner().Content(), member.Content()))
		}
		for _, edge := range component.edges {
			parts = append(parts, nominalBytes(edge.from.Owner().Content(), edge.from.Content()), nominalBytes(edge.to.Owner().Content(), edge.to.Content()))
		}
		for _, head := range component.heads {
			parts = append(parts, nominalBytes(head.dependency.Owner().Content(), head.dependency.Content()), nominalBytes(head.relation.Owner().Content(), head.relation.Content()))
		}
		logicalParts = append(logicalParts, parts...)
		physicalParts = append(physicalParts, parts...)
	}
	appendRelationWakes(&logicalParts, data.byRelation, data.entries)
	appendRelationWakes(&physicalParts, data.byRelation, data.entries)
	appendColumnWakes(&logicalParts, data.byColumn, data.entries)
	appendColumnWakes(&physicalParts, data.byColumn, data.entries)
	logical, logicalOK := identity.DeriveContentID("analysis/relation/mount/arrangement/dependencies/v1/logical", logicalParts...)
	physical, physicalOK := identity.DeriveContentID("analysis/relation/mount/arrangement/dependencies/v1", physicalParts...)
	return logical, physical, logicalOK && physicalOK
}

func appendRelationWakes(parts *[][]byte, wakes map[model.RelationID][]int, entries []Entry) {
	relations := make([]model.RelationID, 0, len(wakes))
	for relation := range wakes {
		relations = append(relations, relation)
	}
	sortRelations(relations)
	for _, relation := range relations {
		*parts = append(*parts, nominalBytes(relation.Owner().Content(), relation.Content()))
		for _, index := range wakes[relation] {
			if index >= 0 && index < len(entries) {
				entry := entries[index]
				*parts = append(*parts, nominalBytes(entry.id.Owner().Content(), entry.id.Content()))
			}
		}
	}
}

func appendColumnWakes(parts *[][]byte, wakes map[model.ColumnID][]int, entries []Entry) {
	columns := make([]model.ColumnID, 0, len(wakes))
	for column := range wakes {
		columns = append(columns, column)
	}
	sortColumns(columns)
	for _, column := range columns {
		*parts = append(*parts, nominalBytes(column.Relation().Owner().Content(), column.Content()))
		for _, index := range wakes[column] {
			if index >= 0 && index < len(entries) {
				entry := entries[index]
				*parts = append(*parts, nominalBytes(entry.id.Owner().Content(), entry.id.Content()))
			}
		}
	}
}
