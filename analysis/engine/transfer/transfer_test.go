package transfer

import (
	"context"
	"errors"
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/solve"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestTryRunCancellationAfterNodeCallbackPublishesNothing(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	point := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(graph.Entry(), point, false)
	graph.AddEdge(point, graph.Exit(), false)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result, err := TryRun(Config{
		Context:  ctx,
		Graph:    graph,
		Registry: reg,
		NodeTransfer: func(ctx NodeContext, in state.State) state.State {
			if ctx.Point == point {
				cancel()
			}
			return in.WriteValue(reg, key.ReturnSlot(0), presentValue(reg))
		},
	})
	if result != nil {
		t.Fatalf("canceled transfer result = %#v, want nil", result)
	}
	if !errors.Is(err, solve.ErrCanceled) || !errors.Is(err, context.Canceled) {
		t.Fatalf("TryRun error = %v, want cancellation", err)
	}
}

func TestRun_LinearGraphPropagatesEntryStateThroughIdentityTransfers(t *testing.T) {
	reg := standard.Registry()
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

func TestRun_WTOFallsBackToFreshFIFOOnBackwardDynamicRead(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	mid := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(graph.Entry(), mid, false)
	graph.AddEdge(mid, graph.Exit(), false)
	plan := solve.NewWTOPlan(graph.RPO(), func(point cfg.Point) []cfg.Point {
		return cfg.SuccessorsReadOnly(graph, point)
	})
	var comparison WTOComparison
	resets := 0
	records := 0
	result, err := TryRun(Config{
		Graph:                    graph,
		Registry:                 reg,
		Schedule:                 ScheduleWTO,
		WTOPlan:                  plan,
		CompareWTO:               func(got WTOComparison) { comparison = got },
		ObserveNode:              func(cfg.Point) bool { return true },
		RecordNodeObservation:    func(NodeObservation) { records++ },
		FinalizeNodeObservations: func(func(cfg.Point) uint64) {},
		ResetNodeObservations:    func() { resets++; records = 0 },
		NodeTransfer: func(ctx NodeContext, in state.State) state.State {
			if ctx.Point == graph.Entry() {
				_ = ctx.Read(graph.Exit())
			}
			return in
		},
	})
	if err != nil {
		t.Fatalf("TryRun: %v", err)
	}
	if result == nil {
		t.Fatal("fallback returned nil result")
	}
	if !comparison.Fallback {
		t.Fatalf("comparison = %#v, want fallback", comparison)
	}
	if resets != 1 {
		t.Fatalf("observation resets = %d, want 1", resets)
	}
	if records == 0 {
		t.Fatal("fresh FIFO fallback did not rebuild observations")
	}
}

func TestRun_NilContextFIFOParity(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	mid := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(graph.Entry(), mid, false)
	graph.AddEdge(mid, graph.Exit(), false)
	slot := key.ReturnSlot(32)
	entry := state.State{}.WriteValue(reg, slot, presentValue(reg))
	run := func(ctx context.Context) (Result, int) {
		stats := &Stats{}
		result := Run(Config{Context: ctx, Graph: graph, Registry: reg, EntryState: entry, Stats: stats})
		return result, stats.Solver.TransferCalls
	}
	fast, fastTransfers := run(nil)
	cancelable, cancelableTransfers := run(context.Background())
	if fastTransfers != cancelableTransfers {
		t.Fatalf("transfer calls nil=%d context=%d", fastTransfers, cancelableTransfers)
	}
	domain := state.Domain(reg)
	for _, point := range graph.RPO() {
		if !domain.Equal(fast[point], cancelable[point]) {
			t.Fatalf("point %d differs between nil-context and context FIFO", point)
		}
	}
}

func TestRun_WTODualPublishesFIFOAndIsDeterministic(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	mid := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(graph.Entry(), mid, false)
	graph.AddEdge(mid, graph.Exit(), false)
	plan := solve.NewWTOPlan(graph.RPO(), func(point cfg.Point) []cfg.Point {
		return cfg.SuccessorsReadOnly(graph, point)
	})
	slot := key.ReturnSlot(31)
	entryState := state.State{}.WriteValue(reg, slot, presentValue(reg))
	run := func() (Result, WTOComparison) {
		var comparison WTOComparison
		result := Run(Config{
			Graph:      graph,
			Registry:   reg,
			Schedule:   ScheduleWTODual,
			WTOPlan:    plan,
			EntryState: entryState,
			CompareWTO: func(got WTOComparison) { comparison = got },
		})
		return result, comparison
	}
	first, firstComparison := run()
	second, secondComparison := run()
	if firstComparison.Fallback || firstComparison.StateDifferences != 0 {
		t.Fatalf("first comparison = %#v", firstComparison)
	}
	if !reflect.DeepEqual(firstComparison, secondComparison) {
		t.Fatalf("comparison changed: first=%#v second=%#v", firstComparison, secondComparison)
	}
	domain := state.Domain(reg)
	for _, point := range graph.RPO() {
		if !domain.Equal(first[point], second[point]) {
			t.Fatalf("point %d differs across repeated dual runs", point)
		}
	}
}

func TestRun_CustomEntryStateSeedsReachableCustomEntry(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	mid := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(graph.Entry(), mid, false)
	graph.AddEdge(mid, graph.Exit(), false)

	slot := key.ReturnSlot(6)
	entryState := state.State{}.WriteValue(reg, slot, presentValue(reg))
	got := Run(Config{
		Graph:      graph,
		Registry:   reg,
		Entry:      &mid,
		EntryState: entryState,
	})

	assertValue(t, reg, got[graph.Entry()], slot, product.Bottom(reg))
	assertValue(t, reg, got[mid], slot, presentValue(reg))
	assertValue(t, reg, got[graph.Exit()], slot, presentValue(reg))
}

func TestRun_RejectsCustomEntryNotInRPO(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	dead := graph.AddNode(cfg.NodeNoop)

	mustPanic(t, "transfer: Config.Entry is not in graph.RPO()", func() {
		Run(Config{
			Graph:    graph,
			Registry: reg,
			Entry:    &dead,
		})
	})
}

func TestTryRun_ReturnsConfigErrors(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	dead := graph.AddNode(cfg.NodeNoop)

	cases := []struct {
		name string
		cfg  Config
		want string
	}{
		{
			name: "nil graph",
			cfg:  Config{Registry: reg},
			want: "transfer: Config.Graph is nil",
		},
		{
			name: "nil registry",
			cfg:  Config{Graph: graph},
			want: "transfer: Config.Registry is nil",
		},
		{
			name: "entry not in RPO",
			cfg:  Config{Graph: graph, Registry: reg, Entry: &dead},
			want: "transfer: Config.Entry is not in graph.RPO()",
		},
		{
			name: "unknown lane",
			cfg:  Config{Graph: graph, Registry: reg, StateLanes: []state.LaneID{state.LaneID("missing")}},
			want: `state: unknown lane "missing"`,
		},
		{
			name: "invalid schedule",
			cfg:  Config{Graph: graph, Registry: reg, Schedule: Schedule(255)},
			want: "transfer: Config.Schedule is invalid",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := TryRun(tc.cfg); err == nil || err.Error() != tc.want {
				t.Fatalf("TryRun error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestRun_NodeTransferWritesAssignmentOutputForSuccessor(t *testing.T) {
	reg := standard.Registry()
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
	reg := standard.Registry()
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
	reg := standard.Registry()
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

func TestRun_UnreachableBranchInputDoesNotRunNodeTransfer(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	branch := graph.AddNode(cfg.NodeBranch)
	thenPoint := graph.AddNode(cfg.NodeNoop)
	elsePoint := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(graph.Entry(), branch, false)
	graph.AddEdge(branch, thenPoint, true)
	graph.AddEdge(branch, elsePoint, false)
	graph.AddEdge(thenPoint, graph.Exit(), false)
	graph.AddEdge(elsePoint, graph.Exit(), false)

	slot := key.ReturnSlot(10)
	got := Run(Config{
		Graph:    graph,
		Registry: reg,
		NodeTransfer: func(ctx NodeContext, in state.State) state.State {
			if ctx.Point == elsePoint {
				return in.WriteValue(reg, slot, absentValue(reg))
			}
			return in
		},
		EdgeTransfer: func(ctx EdgeContext, out state.State) state.State {
			if ctx.Edge.From == branch && !ctx.Edge.Cond {
				return state.Domain(reg).Bottom()
			}
			return out
		},
	})

	assertValue(t, reg, got[elsePoint], slot, product.Bottom(reg))
	assertValue(t, reg, got[graph.Exit()], slot, product.Bottom(reg))
}

func TestRun_JoinPointJoinsPredecessorStatesViaStateDomain(t *testing.T) {
	reg := standard.Registry()
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

func TestRun_StateLanesSelectExactStateAxes(t *testing.T) {
	reg := standard.Registry()
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

	slot := key.ReturnSlot(7)
	tableID := identity.ID{Kind: "table", Site: "transfer-lanes", Index: 1}
	run := func(lanes []state.LaneID) Result {
		return Run(Config{
			Graph:      graph,
			Registry:   reg,
			StateLanes: lanes,
			NodeTransfer: func(ctx NodeContext, in state.State) state.State {
				switch ctx.Point {
				case left:
					return in.WriteValue(reg, slot, presentValue(reg)).FreezeTable(tableID)
				case right:
					return in.FreezeTable(tableID)
				default:
					return in
				}
			},
		})
	}

	defaultFlow := run(nil)
	assertValue(t, reg, defaultFlow[join], slot, presentValue(reg))
	if !defaultFlow[join].IsTableFrozen(tableID) {
		t.Fatal("default state lanes dropped frozen-table fact")
	}

	valueOnly := run([]state.LaneID{state.LaneValues})
	assertValue(t, reg, valueOnly[join], slot, presentValue(reg))
	if valueOnly[join].IsTableFrozen(tableID) {
		t.Fatal("values-only state lane selection preserved disabled frozen-table fact")
	}

	reversed := run([]state.LaneID{state.LaneFrozenTables, state.LaneValues})
	assertValue(t, reg, reversed[join], slot, presentValue(reg))
	if !reversed[join].IsTableFrozen(tableID) {
		t.Fatal("reversed state lane selection dropped enabled frozen-table fact")
	}
}

func TestRun_StateLanesNormalizeStraightLineOutputs(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	mid := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(graph.Entry(), mid, false)
	graph.AddEdge(mid, graph.Exit(), false)

	slot := key.ReturnSlot(8)
	tableID := identity.ID{Kind: "table", Site: "transfer-lanes-linear", Index: 1}
	got := Run(Config{
		Graph:      graph,
		Registry:   reg,
		StateLanes: []state.LaneID{state.LaneValues},
		NodeTransfer: func(ctx NodeContext, in state.State) state.State {
			if ctx.Point == graph.Entry() {
				return in.WriteValue(reg, slot, presentValue(reg)).FreezeTable(tableID)
			}
			return in
		},
		EdgeTransfer: func(_ EdgeContext, out state.State) state.State {
			if out.IsTableFrozen(tableID) {
				t.Fatal("edge transfer observed disabled frozen-table lane")
			}
			return out
		},
	})

	assertValue(t, reg, got[mid], slot, presentValue(reg))
	if got[mid].IsTableFrozen(tableID) {
		t.Fatal("straight-line transfer preserved disabled frozen-table fact")
	}
	assertValue(t, reg, got[graph.Exit()], slot, presentValue(reg))
	if got[graph.Exit()].IsTableFrozen(tableID) {
		t.Fatal("straight-line exit state preserved disabled frozen-table fact")
	}
}

func TestRun_StateLanesEmptySelectionDropsEveryAxis(t *testing.T) {
	reg := standard.Registry()
	graph := cfg.New()
	mid := graph.AddNode(cfg.NodeNoop)
	graph.AddEdge(graph.Entry(), mid, false)
	graph.AddEdge(mid, graph.Exit(), false)

	slot := key.ReturnSlot(9)
	tableID := identity.ID{Kind: "table", Site: "transfer-lanes-empty", Index: 1}
	entryState := state.State{}.WriteValue(reg, slot, presentValue(reg)).FreezeTable(tableID)
	got := Run(Config{
		Graph:      graph,
		Registry:   reg,
		StateLanes: []state.LaneID{},
		EntryState: entryState,
		NodeTransfer: func(_ NodeContext, in state.State) state.State {
			return in.WriteValue(reg, slot, absentValue(reg)).FreezeTable(tableID)
		},
	})

	for _, point := range []cfg.Point{graph.Entry(), mid, graph.Exit()} {
		assertValue(t, reg, got[point], slot, product.Bottom(reg))
		if got[point].IsTableFrozen(tableID) {
			t.Fatalf("empty lane selection preserved frozen-table fact at point %d", point)
		}
	}
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
		t.Fatalf("slot %v = %s, want %s", slot, formatValue(reg, got), formatValue(reg, want))
	}
}

func mustPanic(t *testing.T, want any, f func()) {
	t.Helper()
	defer func() {
		got := recover()
		if got == nil {
			t.Fatalf("expected panic %v", want)
		}
		if got != want {
			t.Fatalf("panic = %v, want %v", got, want)
		}
	}()
	f()
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
