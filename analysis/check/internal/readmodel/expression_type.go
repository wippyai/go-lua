package readmodel

import (
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

// ExpressionTypeAt projects the solved type at an expression's owning
// boundary. It is the embedding/readmodel bridge for position queries; it
// deliberately delegates to the existing body projection rather than running
// any new analysis.
func (r Reader) ExpressionTypeAt(point cfg.Point, expr ast.Expr) (typ.Type, bool) {
	if r.result == nil {
		return nil, false
	}
	return r.result.ExpressionTypeBeforeBoundary(point, expr)
}
