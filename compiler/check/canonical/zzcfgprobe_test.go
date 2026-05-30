package canonical

import (
	"fmt"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/parse"
)

// TestZZCfgProbe dumps the CFG of the if-error wrapper body so the effect
// extraction can see its node/edge structure. Debug probe.
func TestZZCfgProbe(t *testing.T) {
	src := `
local function assertEq(a, b)
    if a ~= b then error("not equal") end
end
`
	_ = src
	src = `
local function bump(carrier)
    local data = carrier and carrier:data() or nil
    if type(data) ~= "table" or type(data.amount) ~= "number" then
        return nil
    end
    local next_amount = data.amount + 1
    return next_amount
end
`
	chunk, err := parse.Parse(strings.NewReader(src), "probe")
	if err != nil {
		t.Fatal(err)
	}
	root := &ast.FunctionExpr{ParList: &ast.ParList{HasVargs: true}, Stmts: chunk}
	bind.Bind(root, []string{"error", "assert"})
	g := cfg.Build(root, "error", "assert")
	dumpGraph(t, g, 0)
	for _, nf := range g.NestedFunctions() {
		if nf.Func == nil {
			continue
		}
		ng := cfg.Build(nf.Func, "error", "assert")
		t.Logf("=== nested fn (params=%v) ===", ng.ParamSymbols())
		dumpGraph(t, ng, 0)
	}
}

func dumpGraph(t *testing.T, g *cfg.Graph, _ int) {
	g.EachNode(func(p cfg.Point, info cfg.NodeInfo) {
		node := g.Node(p)
		kind := "?"
		if node != nil {
			kind = fmt.Sprintf("%v", node.Kind)
		}
		extra := ""
		switch i := info.(type) {
		case *cfg.BranchInfo:
			extra = fmt.Sprintf(" BRANCH condVar=%q check=%v sym=%d cond=%T", i.CondVar, i.CondCheck.Kind, i.CondSymbol, i.Condition)
		case *cfg.CallInfo:
			extra = fmt.Sprintf(" CALL callee=%q args=%d", i.CalleeName, len(i.Args))
		}
		succ := g.Successors(p)
		conds := ""
		for _, s := range succ {
			if v, ok := g.EdgeCond(p, s); ok {
				conds += fmt.Sprintf(" %v=%v", s, v)
			} else {
				conds += fmt.Sprintf(" %v=?", s)
			}
		}
		t.Logf("p=%v kind=%s%s succ=%v edgeconds=[%s]", p, kind, extra, succ, conds)
	})
	t.Logf("entry=%v exit=%v", g.Entry(), g.Exit())
}
