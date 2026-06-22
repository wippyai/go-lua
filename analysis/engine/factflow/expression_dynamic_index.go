package factflow

import "github.com/wippyai/go-lua/analysis/domain/path"

// DynamicIndexExpression describes an expression `table[key]` whose table path
// is static but whose key must be resolved from point-local value evidence.
type DynamicIndexExpression struct {
	tablePath path.Path
	keySource ValueSource
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

func (e DynamicIndexExpression) TablePath() path.Path   { return e.tablePath.Clone() }
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
