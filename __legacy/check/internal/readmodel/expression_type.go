package readmodel

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
)

// ExpressionEvaluationType projects the solved type at a body-owned
// expression-evaluation fact. It deliberately accepts the body DTO instead of
// importing syntax types, keeping readmodel consumers on the architectural
// side of the syntax boundary.
func (r Reader) ExpressionEvaluationType(fact body.ExpressionEvaluationFact) (typ.Type, bool) {
	if r.result == nil || fact.Expr == nil {
		return nil, false
	}
	return r.result.ExpressionTypeBeforeBoundary(fact.Point, fact.Expr)
}
