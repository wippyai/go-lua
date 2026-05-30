package canonical

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/parse"
)

// TestZZKeyOfCfgProbe dumps the CFG of a keyed-iteration loop body to see the
// node/edge layout (binding node, latch branch, body index read).
func TestZZKeyOfCfgProbe(t *testing.T) {
	src := `
local a = {}
for k in pairs(a) do
    local v = a[k]
end
`
	chunk, err := parse.Parse(strings.NewReader(src), "probe")
	if err != nil {
		t.Fatal(err)
	}
	root := &ast.FunctionExpr{ParList: &ast.ParList{HasVargs: true}, Stmts: chunk}
	bind.Bind(root, []string{"pairs"})
	g := cfg.Build(root, "pairs")
	g.EachNode(func(p cfg.Point, info cfg.NodeInfo) {
		node := g.Node(p)
		extra := ""
		switch i := info.(type) {
		case *cfg.BranchInfo:
			extra = " BRANCH condVar=" + i.CondVar
		case *cfg.AssignInfo:
			if len(i.IterExprs) > 0 {
				extra = " ITER targets="
				for _, tg := range i.Targets {
					extra += tg.Name + "(s" + itoaP(uint64(tg.Symbol)) + ") "
				}
			}
		}
		ll := ""
		if node != nil && len(node.LoopLocals) > 0 {
			ll = " loopLocals="
			for _, s := range node.LoopLocals {
				ll += "s" + itoaP(uint64(s)) + " "
			}
			if node.LoopPreheaderSet {
				ll += " preheader=" + itoaP(uint64(node.LoopPreheader))
			}
		}
		kind := 0
		if node != nil {
			kind = int(node.Kind)
		}
		t.Logf("p=%v kind=%d%s%s succ=%v", p, kind, extra, ll, g.Successors(p))
	})
}

func itoaP(n uint64) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
