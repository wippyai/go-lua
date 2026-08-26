package typing

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

func columnIDString(value model.ColumnID) string {
	return value.Relation().Owner().Content().String() + "/" + value.Relation().Content().String() + "/" + value.Content().String()
}

// checkExpandKeySchema proves the logical key shape required by Expand. The
// operator carries one semantic key column, so accepting a composite key (or
// two different singleton keys with the same column) would leave the mount
// resolver to guess which coordinate space that ContentID addresses. Keep
// this obligation at schema check time; runtime layout code must not discover
// it after a program has been admitted.
func (checker *checker) checkExpandKeySchema(reader model.RelationSchema, column model.ColumnID, path string) {
	if checker == nil || !reader.Available() || !column.Available() {
		return
	}
	matches := 0
	for _, key := range checker.registry.Keys() {
		if !key.Available() || key.Relation() != reader.ID() {
			continue
		}
		columns := key.Columns()
		if len(columns) == 1 && columns[0] == column {
			matches++
		}
	}
	if matches != 1 {
		checker.report.add(CodeKeyMismatch, path, fmt.Sprintf("Expand requires exactly one reader key schema with the declared key column, found %d", matches))
	}
}
