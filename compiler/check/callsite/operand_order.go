package callsite

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
)

// CallsBeforeOperandReads returns the calls of the statement at p that
// complete before the statement reads another operand. Their effects can
// change values the statement reads afterwards. Every other call's effects
// follow all of the statement's reads: a call runs only after its callee and
// arguments are evaluated.
//
// Operands are visited in Lua evaluation order: assignment targets' tables
// and keys, then sources left to right; within a call, the callee or
// receiver, then arguments, then the call itself.
func CallsBeforeOperandReads(graph *cfg.Graph, p cfg.Point) map[*ast.FuncCallExpr]bool {
	if graph == nil {
		return nil
	}
	var order operandOrder
	switch info := graph.Info(p).(type) {
	case *cfg.CallInfo:
		if info != nil {
			order.expr(info.Call)
		}
	case *cfg.AssignInfo:
		if info == nil {
			break
		}
		for _, target := range info.Targets {
			order.expr(target.Base)
			order.expr(target.Key)
		}
		order.exprs(info.Sources)
		order.exprs(info.IterExprs)
		if nf := info.NumericFor; nf != nil {
			order.expr(nf.Init)
			order.expr(nf.Limit)
			order.expr(nf.Step)
		}
	case *cfg.ReturnInfo:
		if info != nil {
			order.exprs(info.Exprs)
		}
	case *cfg.BranchInfo:
		if info != nil {
			order.expr(info.Condition)
		}
	}
	return order.before
}

// operandOrder walks expressions in evaluation order and records each call
// that completes before a later read.
type operandOrder struct {
	completed []*ast.FuncCallExpr
	before    map[*ast.FuncCallExpr]bool
}

func (o *operandOrder) read() {
	if len(o.completed) == 0 {
		return
	}
	if o.before == nil {
		o.before = make(map[*ast.FuncCallExpr]bool, len(o.completed))
	}
	for _, call := range o.completed {
		o.before[call] = true
	}
	o.completed = o.completed[:0]
}

func (o *operandOrder) exprs(exprs []ast.Expr) {
	for _, expr := range exprs {
		o.expr(expr)
	}
}

func (o *operandOrder) expr(expr ast.Expr) {
	switch e := expr.(type) {
	case *ast.IdentExpr, *ast.Comma3Expr:
		o.read()
	case *ast.FuncCallExpr:
		if e == nil {
			return
		}
		o.expr(e.Func)
		o.expr(e.Receiver)
		o.exprs(e.Args)
		o.completed = append(o.completed, e)
	case *ast.AttrGetExpr:
		o.expr(e.Object)
		o.expr(e.Key)
	case *ast.TableExpr:
		for _, field := range e.Fields {
			if field != nil {
				o.expr(field.Key)
				o.expr(field.Value)
			}
		}
	case *ast.LogicalOpExpr:
		o.expr(e.Lhs)
		o.expr(e.Rhs)
	case *ast.RelationalOpExpr:
		o.expr(e.Lhs)
		o.expr(e.Rhs)
	case *ast.ArithmeticOpExpr:
		o.expr(e.Lhs)
		o.expr(e.Rhs)
	case *ast.StringConcatOpExpr:
		o.expr(e.Lhs)
		o.expr(e.Rhs)
	case *ast.UnaryNotOpExpr:
		o.expr(e.Expr)
	case *ast.UnaryMinusOpExpr:
		o.expr(e.Expr)
	case *ast.UnaryLenOpExpr:
		o.expr(e.Expr)
	case *ast.UnaryBNotOpExpr:
		o.expr(e.Expr)
	case *ast.CastExpr:
		o.expr(e.Expr)
	case *ast.NonNilAssertExpr:
		o.expr(e.Expr)
	}
}
