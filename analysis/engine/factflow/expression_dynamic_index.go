package factflow

import "github.com/wippyai/go-lua/analysis/domain/path"

// DynamicIndexExpression describes an expression `table[key]`. A static table
// path is carried when one exists for equality/membership facts; the table
// value source is carried so expression evaluation also works for unnameable
// table expressions such as call results.
type DynamicIndexExpression struct {
	tablePath      path.Path
	tableSource    ValueSource
	hasTableSource bool
	keySource      ValueSource
}

func NewDynamicIndexExpression(tablePath path.Path, keySource ValueSource) (DynamicIndexExpression, bool) {
	if tablePath.IsEmpty() || !keySource.Valid() {
		return DynamicIndexExpression{}, false
	}
	return DynamicIndexExpression{
		tablePath: tablePath.Clone(),
		keySource: keySource,
	}, true
}

func NewDynamicIndexExpressionFromSource(tableSource ValueSource, keySource ValueSource) (DynamicIndexExpression, bool) {
	if !tableSource.Valid() || !keySource.Valid() {
		return DynamicIndexExpression{}, false
	}
	return DynamicIndexExpression{
		tableSource:    tableSource,
		hasTableSource: true,
		keySource:      keySource,
	}, true
}

func (e DynamicIndexExpression) WithTableSource(tableSource ValueSource) DynamicIndexExpression {
	if !tableSource.Valid() {
		e.tableSource = ValueSource{}
		e.hasTableSource = false
		return e
	}
	e.tableSource = tableSource
	e.hasTableSource = true
	return e
}

func (e DynamicIndexExpression) TablePath() path.Path { return e.tablePath.Clone() }

// TablePathRef returns the dynamic-index table path for immediate read-only use.
// Callers must not mutate or retain the returned path.
func (e DynamicIndexExpression) TablePathRef() path.Path { return e.tablePath }

func (e DynamicIndexExpression) TableSource() (ValueSource, bool) {
	return e.tableSource, e.hasTableSource
}
func (e DynamicIndexExpression) KeySource() ValueSource { return e.keySource }

func (e DynamicIndexExpression) copy() DynamicIndexExpression {
	e.tablePath = e.tablePath.Clone()
	return e
}

func copyDynamicIndexExpressionMap(in map[ExprRef]DynamicIndexExpression) map[ExprRef]DynamicIndexExpression {
	if len(in) == 0 {
		return nil
	}
	out := make(map[ExprRef]DynamicIndexExpression, len(in))
	for ref, expr := range in {
		if ref == 0 {
			continue
		}
		out[ref] = expr.copy()
	}
	return out
}
