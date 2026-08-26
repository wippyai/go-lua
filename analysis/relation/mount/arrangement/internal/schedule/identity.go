package schedule

import (
	"bytes"
	"encoding/binary"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

func dependencyLess(left, right model.DependencyID) bool {
	return compareDependency(left, right) < 0
}

func relationLess(left, right model.RelationID) bool {
	return compareRelation(left, right) < 0
}

func columnLess(left, right model.ColumnID) bool {
	return compareColumn(left, right) < 0
}

func compareDependency(left, right model.DependencyID) int {
	return compareNominal(left.Owner().Content(), left.Content(), right.Owner().Content(), right.Content())
}

func compareRelation(left, right model.RelationID) int {
	return compareNominal(left.Owner().Content(), left.Content(), right.Owner().Content(), right.Content())
}

func compareColumn(left, right model.ColumnID) int {
	return compareNominal(left.Relation().Owner().Content(), left.Content(), right.Relation().Owner().Content(), right.Content())
}

func compareNominal(leftOwner, leftContent, rightOwner, rightContent identity.ContentID) int {
	if compared := bytes.Compare(leftOwner[:], rightOwner[:]); compared != 0 {
		return compared
	}
	return bytes.Compare(leftContent[:], rightContent[:])
}

func nominalBytes(owner, content identity.ContentID) []byte {
	result := make([]byte, 0, len(owner)+len(content))
	result = append(result, owner[:]...)
	return append(result, content[:]...)
}

func contentBytes(value identity.ContentID) []byte {
	result := make([]byte, len(value))
	copy(result, value[:])
	return result
}

func appendUint32(parts *[][]byte, value uint32) {
	encoded := make([]byte, 4)
	binary.BigEndian.PutUint32(encoded, value)
	*parts = append(*parts, encoded)
}

func dependencyKey(id model.DependencyID) string {
	return string(nominalBytes(id.Owner().Content(), id.Content()))
}

func relationKey(id model.RelationID) string {
	return string(nominalBytes(id.Owner().Content(), id.Content()))
}

func componentKey(members []model.DependencyID) string {
	result := make([]byte, 0, len(members)*64)
	for _, member := range members {
		result = append(result, nominalBytes(member.Owner().Content(), member.Content())...)
	}
	return string(result)
}

func canonicalRelations(values []model.RelationID) ([]model.RelationID, bool) {
	result := make([]model.RelationID, 0, len(values))
	seen := make(map[model.RelationID]struct{}, len(values))
	for _, value := range values {
		if !value.Available() {
			return nil, false
		}
		if _, duplicate := seen[value]; duplicate {
			return nil, false
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sortRelations(result)
	return result, true
}

func canonicalColumns(values []model.ColumnID) ([]model.ColumnID, bool) {
	result := make([]model.ColumnID, 0, len(values))
	seen := make(map[model.ColumnID]struct{}, len(values))
	for _, value := range values {
		if !value.Available() {
			return nil, false
		}
		if _, duplicate := seen[value]; duplicate {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sortColumns(result)
	return result, true
}

func canonicalDependencies(values []model.DependencyID) ([]model.DependencyID, bool) {
	result := make([]model.DependencyID, 0, len(values))
	seen := make(map[model.DependencyID]struct{}, len(values))
	for _, value := range values {
		if !value.Available() {
			return nil, false
		}
		if _, duplicate := seen[value]; duplicate {
			return nil, false
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sortDependencies(result)
	return result, true
}

func sortRelations(values []model.RelationID) {
	for index := 1; index < len(values); index++ {
		for cursor := index; cursor > 0 && relationLess(values[cursor], values[cursor-1]); cursor-- {
			values[cursor], values[cursor-1] = values[cursor-1], values[cursor]
		}
	}
}

func sortColumns(values []model.ColumnID) {
	for index := 1; index < len(values); index++ {
		for cursor := index; cursor > 0 && columnLess(values[cursor], values[cursor-1]); cursor-- {
			values[cursor], values[cursor-1] = values[cursor-1], values[cursor]
		}
	}
}

func sortDependencies(values []model.DependencyID) {
	for index := 1; index < len(values); index++ {
		for cursor := index; cursor > 0 && dependencyLess(values[cursor], values[cursor-1]); cursor-- {
			values[cursor], values[cursor-1] = values[cursor-1], values[cursor]
		}
	}
}
