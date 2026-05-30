package canonical

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/parse"
)

// TestZZCfg2Probe dumps the process body CFG for the NoReturn fixture. Debug probe.
func TestZZCfg2Probe(t *testing.T) {
	src := `
local function fail(msg)
    error(msg)
end
function process(x)
    if x == nil then
        fail("x is nil")
    end
    return x
end
`
	chunk, err := parse.Parse(strings.NewReader(src), "probe")
	if err != nil {
		t.Fatal(err)
	}
	root := &ast.FunctionExpr{ParList: &ast.ParList{HasVargs: true}, Stmts: chunk}
	bind.Bind(root, []string{"error", "assert"})
	g := cfg.Build(root, "error", "assert")
	for _, nf := range g.NestedFunctions() {
		if nf.Func == nil {
			continue
		}
		ng := cfg.Build(nf.Func, "error", "assert")
		t.Logf("=== nested fn (params=%v) entry=%v exit=%v ===", ng.ParamSymbols(), ng.Entry(), ng.Exit())
		for pi := 0; pi < 16; pi++ {
			p := cfg.Point(pi)
			node := ng.Node(p)
			if node == nil {
				continue
			}
			t.Logf("p=%v kind=%v succ=%v preds=%v", p, node.Kind, ng.Successors(p), ng.Predecessors(p))
		}
	}
}
