package assign

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/check/abstract/numconst"
	fbpath "github.com/wippyai/go-lua/compiler/check/domain/path"
	"github.com/wippyai/go-lua/compiler/check/domain/resolve"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
)

func lengthIndexSourceFromAttr(attr *ast.AttrGetExpr, constResolver func(string) *flow.ConstValue, bindings *bind.BindingTable) (flow.AssignmentSource, bool) {
	if attr == nil {
		return flow.AssignmentSource{}, false
	}
	container := fbpath.FromExprWithBindings(attr.Object, constResolver, bindings)
	if container.IsEmpty() || container.Symbol == 0 {
		return flow.AssignmentSource{}, false
	}
	indexedPath, offset, ok := lengthIndexPathFromExpr(attr.Key, constResolver, bindings)
	if !ok || !indexedPath.Equal(container) {
		return flow.AssignmentSource{}, false
	}
	container.Root = resolve.RootNameFromBindings(bindings, container.Symbol, container.Root)
	return flow.AssignmentSource{
		Kind:          flow.AssignmentSourceLengthIndex,
		ContainerPath: container,
		Offset:        offset,
	}, true
}

func lengthIndexPathFromExpr(expr ast.Expr, constResolver func(string) *flow.ConstValue, bindings *bind.BindingTable) (constraint.Path, int64, bool) {
	switch e := expr.(type) {
	case *ast.UnaryLenOpExpr:
		path := fbpath.FromExprWithBindings(e.Expr, constResolver, bindings)
		return path, 0, !path.IsEmpty()
	case *ast.ArithmeticOpExpr:
		if e.Operator != "+" && e.Operator != "-" {
			return constraint.Path{}, 0, false
		}
		path, offset, ok := lengthIndexPathFromExpr(e.Lhs, constResolver, bindings)
		if !ok {
			return constraint.Path{}, 0, false
		}
		k, ok := numconst.IntConstFromExpr(e.Rhs)
		if !ok {
			return constraint.Path{}, 0, false
		}
		if e.Operator == "-" {
			k = -k
		}
		return path, offset + k, true
	default:
		return constraint.Path{}, 0, false
	}
}
