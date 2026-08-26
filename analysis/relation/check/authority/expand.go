package authority

import "github.com/wippyai/go-lua/analysis/relation/schema/model"

// checkExpandKeySchema closes the authority side of the Expand contract. One
// semantic key column must name one and only one singleton reader key schema;
// otherwise mount would have to choose between a composite coordinate and
// multiple same-column layouts after the schema was already admitted.
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
		checker.add(CodeInvalidMembership, path+".keySchema")
	}
}
