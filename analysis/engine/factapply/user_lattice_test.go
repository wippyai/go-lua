package factapply

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/userlattice"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestUserLatticeTaintPropagatesThroughAssignmentAndJoins(t *testing.T) {
	const taintAxis userlattice.AxisID = "test.taint"
	reg := axis.NewRegistry()
	if _, err := userlattice.Register(reg, testTaintSpec(taintAxis)); err != nil {
		t.Fatalf("register taint axis: %v", err)
	}
	reg.Freeze()

	graph := cfg.New()
	branch := graph.AddNode(cfg.NodeBranch)
	left := graph.AddNode(cfg.NodeAssign)
	right := graph.AddNode(cfg.NodeAssign)
	join := graph.AddNode(cfg.NodeJoin)
	graph.AddEdge(graph.Entry(), branch, false)
	graph.AddEdge(branch, left, true)
	graph.AddEdge(branch, right, false)
	graph.AddEdge(left, join, false)
	graph.AddEdge(right, join, false)
	graph.AddEdge(join, graph.Exit(), false)

	raw := symbol.ID(701)
	clean := symbol.ID(702)
	target := symbol.ID(703)
	rawPath := pathdom.NewPath(raw, "raw")
	cleanPath := pathdom.NewPath(clean, "clean")
	targetPath := pathdom.NewPath(target, "sink")
	rawSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(701), HasExpr: true}
	cleanSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(702), HasExpr: true}

	builder := visibility.NewBuilder()
	rawVersion := builder.Define(left, raw, "raw")
	builder.SetVisible(right, raw, rawVersion)
	cleanVersion := builder.Define(right, clean, "clean")
	builder.SetVisible(left, clean, cleanVersion)
	targetVersion := builder.Define(left, target, "sink")
	builder.SetVisible(right, target, targetVersion)
	builder.SetVisible(join, target, targetVersion)
	resolver := visibility.NewResolver(builder.Build())

	rawKey := mustStateKeyForPath(t, resolver, left, rawPath)
	cleanKey := mustStateKeyForPath(t, resolver, right, cleanPath)
	targetKey := mustStateKeyForPath(t, resolver, join, targetPath)
	entry := state.State{}.
		WriteUserElement(reg, resolver.KeySpace(), taintAxis, rawKey, "Tainted").
		WriteUserElement(reg, resolver.KeySpace(), taintAxis, cleanKey, "Sanitized")

	got := transfer.Run(transfer.Config{
		Graph:      graph,
		Registry:   reg,
		StateLanes: []state.LaneID{state.LaneUserLattices},
		EntryState: entry,
		NodeTransfer: NewFactsNodeTransfer(FactsNodeTransferConfig{
			Facts: factflow.NewFacts(factflow.FactsInput{
				RootAssignments: map[cfg.Point]factflow.RootAssignment{
					left:  factflow.NewRootAssignment(factflow.RootAssignmentOrdinaryRootWrite, target, targetPath, rawSource),
					right: factflow.NewRootAssignment(factflow.RootAssignmentOrdinaryRootWrite, target, targetPath, cleanSource),
				},
				ExpressionPaths: map[factflow.ExprRef]pathdom.Path{
					rawSource.ExprRef:   rawPath,
					cleanSource.ExprRef: cleanPath,
				},
			}),
			Sources: &recordingSourceValues{values: map[factflow.ValueSource]product.Value{
				rawSource:   product.Top(),
				cleanSource: product.Top(),
			}},
			Visibility: resolver,
		}),
	})

	if gotElem, ok := got[join].ReadUserElement(reg, resolver.KeySpace(), taintAxis, targetKey); !ok || gotElem != "Unknown" {
		t.Fatalf("joined taint = %q/%v, want Unknown/true", gotElem, ok)
	}
	if gotElem, ok := got[graph.Exit()].ReadUserElement(reg, resolver.KeySpace(), taintAxis, targetKey); !ok || gotElem != "Unknown" {
		t.Fatalf("exit taint = %q/%v, want Unknown/true", gotElem, ok)
	}
}

func mustStateKeyForPath(t *testing.T, resolver *visibility.Resolver, point cfg.Point, p pathdom.Path) pathaddr.StateKey {
	t.Helper()
	key, ok := visibility.AddressAt(resolver, point, p).RootOrVisibleStateKey()
	if !ok {
		t.Fatalf("missing state key for %s at point %d", p.String(), point)
	}
	return key
}

func testTaintSpec(id userlattice.AxisID) userlattice.Spec {
	return userlattice.Spec{
		ID:       id,
		Elements: []userlattice.ElementID{"Untainted", "Sanitized", "Tainted", "Unknown"},
		Bottom:   "Untainted",
		Top:      "Unknown",
		Order: []userlattice.OrderPair{
			{Lower: "Untainted", Upper: "Sanitized"},
			{Lower: "Untainted", Upper: "Tainted"},
			{Lower: "Sanitized", Upper: "Unknown"},
			{Lower: "Tainted", Upper: "Unknown"},
		},
		Hooks: userlattice.Hooks{
			OnAssign:       userlattice.AssignHook{Mode: userlattice.AssignPropagate},
			OnCallBoundary: userlattice.CallBoundaryHook{Mode: userlattice.CallBoundaryKeep},
			OnClaim: []userlattice.ClaimHook{
				{Claim: "tainted", Element: "Tainted"},
				{Claim: "sanitized", Element: "Sanitized"},
			},
		},
	}
}
