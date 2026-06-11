package transfer

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func TestRun_LinearGraphPropagatesEntryStateThroughIdentityTransfers(t *testing.T) {
	reg := product.DefaultRegistry()
	graph := cfg.New()
	mid := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(graph.Entry(), mid, false)
	graph.AddEdge(mid, graph.Exit(), false)

	slot := key.ReturnSlot(0)
	entryState := state.State{}.WriteValue(reg, slot, presentValue(reg))

	got := Run(Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: entryState,
	})

	for _, point := range []cfg.Point{graph.Entry(), mid, graph.Exit()} {
		assertValue(t, reg, got[point], slot, presentValue(reg))
	}
}

func TestRun_NodeTransferWritesAssignmentOutputForSuccessor(t *testing.T) {
	reg := product.DefaultRegistry()
	graph := cfg.New()
	assign := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), assign, false)
	graph.AddEdge(assign, graph.Exit(), false)

	slot := key.ReturnSlot(1)
	written := presentValue(reg)
	got := Run(Config{
		Graph:    graph,
		Registry: reg,
		NodeTransfer: func(ctx NodeContext, in state.State) state.State {
			if ctx.Point != assign {
				return in
			}
			if ctx.Node == nil || ctx.Node.Kind != cfg.NodeAssign {
				t.Fatalf("node context = %#v, want assignment node", ctx.Node)
			}
			return in.WriteValue(reg, slot, written)
		},
	})

	assertValue(t, reg, got[assign], slot, product.Bottom(reg))
	assertValue(t, reg, got[graph.Exit()], slot, written)
}

func TestRun_EdgeTransferDistinguishesBranchEdges(t *testing.T) {
	reg := product.DefaultRegistry()
	graph := cfg.New()
	branch := graph.AddNode(cfg.NodeBranch)
	thenPoint := graph.AddNode(cfg.NodeNoop)
	elsePoint := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(graph.Entry(), branch, false)
	graph.AddEdge(branch, thenPoint, true)
	graph.AddEdge(branch, elsePoint, false)
	graph.AddEdge(thenPoint, graph.Exit(), false)
	graph.AddEdge(elsePoint, graph.Exit(), false)

	trueSlot := key.ReturnSlot(2)
	falseSlot := key.ReturnSlot(3)
	got := Run(Config{
		Graph:    graph,
		Registry: reg,
		EdgeTransfer: func(ctx EdgeContext, out state.State) state.State {
			if !ctx.Graph.IsBranch(ctx.Edge.From) {
				return out
			}
			if !ctx.HasCond {
				t.Fatalf("branch edge %d -> %d did not expose condition", ctx.Edge.From, ctx.Edge.To)
			}
			if ctx.Edge.Cond {
				return out.WriteValue(reg, trueSlot, presentValue(reg))
			}
			return out.WriteValue(reg, falseSlot, absentValue(reg))
		},
	})

	assertValue(t, reg, got[thenPoint], trueSlot, presentValue(reg))
	assertValue(t, reg, got[thenPoint], falseSlot, product.Bottom(reg))
	assertValue(t, reg, got[elsePoint], trueSlot, product.Bottom(reg))
	assertValue(t, reg, got[elsePoint], falseSlot, absentValue(reg))
}

func TestRun_EdgeTransferDoesNotExposeConditionForOrdinaryEdges(t *testing.T) {
	reg := product.DefaultRegistry()
	graph := cfg.New()
	mid := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(graph.Entry(), mid, false)
	graph.AddEdge(mid, graph.Exit(), false)

	gotOrdinaryEdge := false
	Run(Config{
		Graph:    graph,
		Registry: reg,
		EdgeTransfer: func(ctx EdgeContext, out state.State) state.State {
			if ctx.Edge.From == graph.Entry() && ctx.Edge.To == mid {
				gotOrdinaryEdge = true
				if ctx.HasCond {
					t.Fatal("ordinary edge exposed a conditional branch flag")
				}
			}
			return out
		},
	})

	if !gotOrdinaryEdge {
		t.Fatal("ordinary edge transfer was not called")
	}
}

func TestRun_JoinPointJoinsPredecessorStatesViaStateDomain(t *testing.T) {
	reg := product.DefaultRegistry()
	graph := cfg.New()
	branch := graph.AddNode(cfg.NodeBranch)
	left := graph.AddNode(cfg.NodeNoop)
	right := graph.AddNode(cfg.NodeNoop)
	join := graph.AddNode(cfg.NodeJoin)
	graph.AddEdge(graph.Entry(), branch, false)
	graph.AddEdge(branch, left, true)
	graph.AddEdge(branch, right, false)
	graph.AddEdge(left, join, false)
	graph.AddEdge(right, join, false)
	graph.AddEdge(join, graph.Exit(), false)

	slot := key.ReturnSlot(4)
	got := Run(Config{
		Graph:    graph,
		Registry: reg,
		NodeTransfer: func(ctx NodeContext, in state.State) state.State {
			switch ctx.Point {
			case left:
				return in.WriteValue(reg, slot, presentValue(reg))
			case right:
				return in.WriteValue(reg, slot, absentValue(reg))
			default:
				return in
			}
		},
	})

	assertValue(t, reg, got[join], slot, product.Top())
	assertValue(t, reg, got[graph.Exit()], slot, product.Top())
}

func TestRun_ForwardsWidenAtAndWidenDelayToSolver(t *testing.T) {
	reg := wideningRegistry()
	graph := cfg.New()
	loop := graph.AddNode(cfg.NodeJoin)
	graph.AddEdge(graph.Entry(), loop, false)
	graph.AddEdge(loop, loop, false)
	graph.AddEdge(loop, graph.Exit(), false)

	slot := key.ReturnSlot(5)
	entryState := state.State{}.WriteValue(reg, slot, wideningValue(reg, wideningOne))
	seenWidenAt := false
	seenWidenDelay := false

	got := Run(Config{
		Graph:      graph,
		Registry:   reg,
		EntryState: entryState,
		NodeTransfer: func(ctx NodeContext, in state.State) state.State {
			if ctx.Point != loop {
				return in
			}
			return in.UpdateValue(reg, slot, func(v product.Value) product.Value {
				current := product.Get(reg, v, wideningKey)
				switch {
				case current == wideningTop:
					return v
				case current < wideningExactMax:
					return wideningValue(reg, current+1)
				default:
					return v
				}
			})
		},
		WidenAt: func(point cfg.Point) bool {
			if point == loop {
				seenWidenAt = true
				return true
			}
			return false
		},
		WidenDelay: func(point cfg.Point) int {
			if point == loop {
				seenWidenDelay = true
			}
			return 10
		},
	})

	if !seenWidenAt {
		t.Fatal("WidenAt was not called for the loop cell")
	}
	if !seenWidenDelay {
		t.Fatal("WidenDelay was not called for the loop cell")
	}
	if gotValue := product.Get(reg, got[loop].ReadValue(reg, slot), wideningKey); gotValue != wideningExactMax {
		t.Fatalf("loop value = %d, want exact delayed value %d", gotValue, wideningExactMax)
	}
	if gotValue := product.Get(reg, got[graph.Exit()].ReadValue(reg, slot), wideningKey); gotValue != wideningExactMax {
		t.Fatalf("exit value = %d, want exact delayed value %d", gotValue, wideningExactMax)
	}
}

func TestTransferPackageDoesNotImportLuaCompilerOldASTOrAssertionPackages(t *testing.T) {
	directCmd := exec.Command("go", "list", "-f", `{{range .Imports}}{{.}}{{"\n"}}{{end}}`, ".")
	directOut, err := directCmd.Output()
	if err != nil {
		t.Fatalf("go list direct imports . failed: %v", err)
	}
	const assertionAxis = "github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	for _, dep := range strings.Fields(string(directOut)) {
		if dep == assertionAxis || strings.HasPrefix(dep, assertionAxis+"/") {
			t.Fatalf("transfer package directly imports forbidden dependency %q", dep)
		}
	}

	cmd := exec.Command("go", "list", "-deps", ".")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list -deps . failed: %v", err)
	}
	banned := []string{
		"github.com/wippyai/go-lua/__old",
		"github.com/wippyai/go-lua/analysis/lua",
		"github.com/wippyai/go-lua/compiler",
		"github.com/wippyai/go-lua/compiler/ast",
		"go/ast",
	}
	for _, dep := range strings.Fields(string(out)) {
		for _, prefix := range banned {
			if dep == prefix || strings.HasPrefix(dep, prefix+"/") {
				t.Fatalf("transfer package imports forbidden dependency %q", dep)
			}
		}
	}
}

func assertValue(t *testing.T, reg *axis.Registry, gotState state.State, slot key.Value, want product.Value) {
	t.Helper()
	if got := gotState.ReadValue(reg, slot); !product.Equal(reg, got, want) {
		t.Fatalf("slot %s = %s, want %s", slot, formatValue(reg, got), formatValue(reg, want))
	}
}

func presentValue(reg *axis.Registry) product.Value {
	return product.NewWithPresence(reg, product.ShapeTop, presence.Present())
}

func absentValue(reg *axis.Registry) product.Value {
	return product.NewWithPresence(reg, product.ShapeTop, presence.Absent())
}

func formatValue(reg *axis.Registry, v product.Value) string {
	switch {
	case product.Equal(reg, v, product.Bottom(reg)):
		return "bottom"
	case product.Equal(reg, v, product.Top()):
		return "top"
	default:
		return product.PresenceOf(v).String()
	}
}

type widening uint8

const (
	wideningBottom   widening = 0
	wideningOne      widening = 1
	wideningExactMax widening = 4
	wideningTop      widening = 100
)

var wideningKey = axis.NewKey[widening]("transfer.test.widening")

func wideningRegistry() *axis.Registry {
	reg := axis.NewRegistry()
	axis.Register(reg, axis.Spec[widening]{
		Key:    wideningKey,
		Bottom: func() widening { return wideningBottom },
		Top:    func() widening { return wideningTop },
		Equal:  func(a, b widening) bool { return a == b },
		LessOrEq: func(a, b widening) bool {
			return a == b || a != wideningTop && b == wideningTop || a < b && b != wideningTop
		},
		Join: func(a, b widening) widening {
			if a == wideningTop || b == wideningTop {
				return wideningTop
			}
			if a > b {
				return a
			}
			return b
		},
		Meet: func(a, b widening) widening {
			if a < b {
				return a
			}
			return b
		},
		Widen: func(prev, next widening) widening {
			if prev == next {
				return prev
			}
			return wideningTop
		},
		Hash: func(v widening) uint64 {
			return uint64(v) + 1
		},
	})
	return reg.Freeze()
}

func wideningValue(reg *axis.Registry, value widening) product.Value {
	return product.Set(reg, product.Top(), wideningKey, value)
}
