package canonical_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/canonical/equation"
	"github.com/wippyai/go-lua/compiler/check/canonical/input"
	"github.com/wippyai/go-lua/compiler/check/canonical/transfer"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	"github.com/wippyai/go-lua/compiler/parse"
	"github.com/wippyai/go-lua/types/flow"
)

func TestDiag2(t *testing.T) {
	body := "local count = 0\nwhile items do\ncount = count + 1\nend\nreturn count\n"
	stmts, _ := parse.ParseString(body, "c.lua")
	fn := &ast.FunctionExpr{ParList: &ast.ParList{Names: []string{"items"}}, Stmts: stmts}
	in := input.BuildFromFunction(fn, nil, nil)
	g := in.Graph
	exitSym, _ := g.SymbolAt(g.Exit(), "count")
	t.Logf("exit count sym = %d", exitSym)
	real := transfer.New(in, nil, nil, nil, nil, nil, nil, nil)
	probe := equation.NodeTransferFunc(func(g *cfg.Graph, p cfg.Point, incoming flow.PointState, ec paramevidence.Contracts, demand func(int, paramevidence.ParamContract)) flow.PointState {
		out := real.Transfer(g, p, incoming, ec, demand)
		t.Logf("point %v in.Num=%v out.Num=%v", p, incoming.Num, out.Num)
		return out
	})
	equation.NewBuilder(g, in.Scope.NumParams(), probe).Solve()
}
