package lua

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/canonical/input"
	"github.com/wippyai/go-lua/compiler/parse"
)

func dumpFixtureGraph(t *testing.T, s namedSuite) {
	files := resolveFiles(s)
	src := readFixtureFile(s.Dir, files[len(files)-1])
	stmts, err := parse.ParseString(src, "main.lua")
	if err != nil {
		t.Logf("parse err: %v", err)
		return
	}
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}, Stmts: stmts}
	in := input.BuildFromFunction(fn, nil, nil)
	g := in.Graph
	g.EachNode(func(p cfg.Point, _ cfg.NodeInfo) {
		node := g.Node(p)
		var kind interface{} = "?"
		if node != nil {
			kind = node.Kind
		}
		t.Logf("NODE p=%v kind=%v preds=%v", p, kind, g.PredecessorsReadOnly(p))
	})
}

// TestZZProbeNarrow runs a single narrowing fixture through the canonical flow
// for the narrowing-precision investigation. Gated by name so it only fires
// when explicitly run. Keep as a debug probe.
func TestZZProbeNarrow(t *testing.T) {
	targets := map[string]bool{
		"narrowing/equality-discriminant":                      true,
		"narrowing/boolean-discriminant":                       true,
		"narrowing/typeof-guard":                               true,
		"narrowing/typeof-excludes-other":                      true,
		"narrowing/union-page-variant-guard":                   true,
		"narrowing/page-registry-renderer-guard":               true,
		"narrowing/dynamic-registry-renderer-guard":            true,
		"narrowing/channel-select-case-exhaustiveness-warning": true,
	}
	suites, err := discoverFixtures("testdata/fixtures")
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	for _, s := range suites {
		if !targets[s.Name] {
			continue
		}
		diags, entry := fixtureDiagnostics(s)
		t.Logf("=== %s entry=%s ===", s.Name, entry)
		for _, d := range diags {
			t.Logf("DIAG %s", diagSummary(d))
		}
		v := judgeAgainstCuratedExpectations(s, diags, entry)
		t.Logf("passed=%v missing=%v unexpected=%v", v.passed, v.missing, v.unexpected)
	}
}
