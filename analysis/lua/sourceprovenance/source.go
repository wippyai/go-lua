package sourceprovenance

import (
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/compiler/ast"
)

// ASTSource describes Lua AST provenance for one value-list slot.
type ASTSource struct {
	Kind factflow.ValueSourceKind
	Expr ast.Expr

	ExprIndex    int
	TargetIndex  int
	ResultIndex  int
	CallPoint    cfg.Point
	HasCallPoint bool

	Final    bool
	Expanded bool
	Adjusted bool
	OpenTail bool
}
