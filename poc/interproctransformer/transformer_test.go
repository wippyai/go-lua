package interproctransformer

import (
	"math/rand"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

type fixture struct {
	callee, caller                        *cfg.CFG
	calleeBranch, calleeTrue, calleeFalse cfg.Point
	call, callerBranch                    cfg.Point
	left, right                           pathdom.Path
	facts                                 factflow.Facts
	resolver                              *visibility.Resolver
}

func newFixture() fixture {
	callee := cfg.New()
	cb := callee.AddNode(cfg.NodeBranch)
	ct := callee.AddNode(cfg.NodeNoop)
	cf := callee.AddNode(cfg.NodeNoop)
	previous := callee.Entry()
	// Identity points model a modest real function's syntax/CFG scaffolding.
	// They are semantically absent from the compiled boundary but are paid on
	// every exact-context body solve, which is the multiplicative defect under
	// test rather than an artificially expensive transfer callback.
	for i := 0; i < 80; i++ {
		point := callee.AddNode(cfg.NodeNoop)
		callee.AddEdge(previous, point, false)
		previous = point
	}
	callee.AddEdge(previous, cb, false)
	callee.AddEdge(cb, ct, true)
	callee.AddEdge(cb, cf, false)
	callee.AddEdge(ct, callee.Exit(), false)
	callee.AddEdge(cf, callee.Exit(), false)

	caller := cfg.New()
	call := caller.AddNode(cfg.NodeCall)
	branch := caller.AddNode(cfg.NodeBranch)
	t := caller.AddNode(cfg.NodeNoop)
	f := caller.AddNode(cfg.NodeNoop)
	caller.AddEdge(caller.Entry(), call, false)
	caller.AddEdge(call, branch, false)
	caller.AddEdge(branch, t, true)
	caller.AddEdge(branch, f, false)
	caller.AddEdge(t, caller.Exit(), false)
	caller.AddEdge(f, caller.Exit(), false)

	left := pathdom.NewPath(symbol.ID(8101), "left")
	right := pathdom.NewPath(symbol.ID(8102), "right")
	const leftExpr, rightExpr = factflow.ExprRef(8101), factflow.ExprRef(8102)
	facts := factflow.NewFacts(factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{call: factflow.NewCallSite(factflow.CallSiteConfig{
			Context: factflow.CallSiteContextCondition,
			ArgumentSources: []factflow.ValueSource{
				{Kind: factflow.ValueSourceExpression, ExprRef: leftExpr, HasExpr: true},
				{Kind: factflow.ValueSourceExpression, ExprRef: rightExpr, HasExpr: true},
			},
		})},
		ExpressionPaths: map[factflow.ExprRef]pathdom.Path{leftExpr: left, rightExpr: right},
	})
	b := visibility.NewBuilder()
	for _, point := range append(caller.RPO(), callee.RPO()...) {
		b.Define(point, left.Symbol, left.Root)
		b.Define(point, right.Symbol, right.Root)
	}
	return fixture{callee: callee, caller: caller, calleeBranch: cb, calleeTrue: ct, calleeFalse: cf,
		call: call, callerBranch: branch, left: left, right: right, facts: facts,
		resolver: visibility.NewResolver(b.Build())}
}

type exactEngine struct{ bodySolves int }

func (e *exactEngine) solve(f fixture, left, right product.Value) (transfer.Result, summary.Summary) {
	e.bodySolves++
	reg := standard.Registry()
	entry := state.Reachable(state.State{}).
		WritePathKey(reg, f.resolver.KeySpace(), f.resolver.KeyAt(f.callee.Entry(), f.left), left).
		WritePathKey(reg, f.resolver.KeySpace(), f.resolver.KeyAt(f.callee.Entry(), f.right), right)
	result := transfer.Run(transfer.Config{
		Graph: f.callee, Registry: reg, EntryState: entry,
		EdgeTransfer: func(ctx transfer.EdgeContext, out state.State) state.State {
			if ctx.Edge.From != f.calleeBranch {
				return out
			}
			// This is the concrete exact-context guard oracle. It deliberately
			// executes inside every callee body solve; the transformer compiles the
			// same guard once and specializes it without graph traversal.
			if ctx.Edge.Cond != product.Equal(reg, left, right) {
				return state.Domain(reg).Bottom()
			}
			return out
		},
	})
	equal := !state.IsBottom(reg, result[f.calleeTrue])
	sum := summary.Summary{
		Returns: []product.Value{typevalue.LiteralBool(reg, equal)}, NormalReturnParams: []product.Value{left, right},
		ReturnConditionParamRefinements: []summary.ReturnConditionParamRefinement{{
			ReturnIndex: 0, ReturnValue: equal, Target: pathdom.NewPlaceholder(0), Value: left,
		}},
	}
	if equal {
		sum.NormalReturnParamEqualities = []summary.ParamEquality{{Left: 0, Right: 1}}
	}
	return result, summary.Normalize(reg, sum)
}

func callerEntry(f fixture, left, right product.Value) state.State {
	reg := standard.Registry()
	return state.Reachable(state.State{}).
		WritePathKey(reg, f.resolver.KeySpace(), f.resolver.KeyAt(f.caller.Entry(), f.left), left).
		WritePathKey(reg, f.resolver.KeySpace(), f.resolver.KeyAt(f.caller.Entry(), f.right), right)
}

func TestTransformerMatchesExactContextSummaryAndCallerAcrossEveryLane(t *testing.T) {
	f := newFixture()
	compiled, err := Compile(CompileRequest{})
	if err != nil {
		t.Fatal(err)
	}
	reg := standard.Registry()
	if got := len(state.DefaultLanes()); got != 17 {
		t.Fatalf("state lanes = %d, want 17", got)
	}
	values := []product.Value{
		typevalue.LiteralString(reg, "a"), typevalue.LiteralString(reg, "b"),
		typevalue.LiteralInt(reg, 1), typevalue.LiteralInt(reg, 2),
	}
	rng := rand.New(rand.NewSource(8127))
	engine := new(exactEngine)
	for i := 0; i < 512; i++ {
		left, right := values[rng.Intn(len(values))], values[rng.Intn(len(values))]
		calleePoints, wantSummary := engine.solve(f, left, right)
		gotSummary := compiled.Specialize(reg, left, right)
		if !summary.Equal(reg, wantSummary, gotSummary) {
			t.Fatalf("case %d summary differs: left=%#v right=%#v want=%#v got=%#v", i, left, right, wantSummary, gotSummary)
		}
		wantOutcome, err := Lower(wantSummary)
		if err != nil {
			t.Fatal(err)
		}
		gotOutcome, err := Lower(gotSummary)
		if err != nil {
			t.Fatal(err)
		}
		entry := callerEntry(f, left, right)
		want := ApplyCaller(reg, f.caller, f.facts, f.resolver, f.call, f.callerBranch, entry, wantOutcome)
		got := ApplyCaller(reg, f.caller, f.facts, f.resolver, f.call, f.callerBranch, entry, gotOutcome)
		for _, point := range f.callee.RPO() {
			// The boundary does not publish internal callee states. Verify the
			// complete exact-context oracle nonetheless: identity prefix and exit
			// preserve entry, while precisely one guarded arm is reachable.
			wantPoint := entry
			switch point {
			case f.calleeTrue:
				if !product.Equal(reg, left, right) {
					wantPoint = state.Domain(reg).Bottom()
				}
			case f.calleeFalse:
				if product.Equal(reg, left, right) {
					wantPoint = state.Domain(reg).Bottom()
				}
			}
			for _, lane := range state.DefaultLanes() {
				d := state.DomainWithLanes(reg, []state.LaneID{lane})
				if !d.Equal(calleePoints[point], wantPoint) {
					t.Fatalf("case %d callee point %d lane %s differs", i, point, lane)
				}
			}
		}
		for _, point := range f.caller.RPO() {
			for _, lane := range state.DefaultLanes() {
				d := state.DomainWithLanes(reg, []state.LaneID{lane})
				if !d.Equal(want[point], got[point]) {
					t.Fatalf("case %d point %d lane %s differs", i, point, lane)
				}
			}
		}
	}
	if engine.bodySolves != 512 {
		t.Fatalf("oracle body solves = %d", engine.bodySolves)
	}
	before := engine.bodySolves
	for i := 0; i < 4096; i++ {
		_ = compiled.Specialize(reg, values[i%len(values)], values[(i/3)%len(values)])
	}
	if engine.bodySolves != before {
		t.Fatalf("specialization performed %d callee body solves", engine.bodySolves-before)
	}
}

func TestCompileAndLowerFailClosed(t *testing.T) {
	for _, req := range []CompileRequest{{Recursive: true}, {Heap: true}, {Placement: true}, {StateSensitiveSidecars: true}} {
		if _, err := Compile(req); err == nil {
			t.Fatalf("contextual request compiled: %#v", req)
		}
	}
	reg := standard.Registry()
	if _, err := Lower(summary.Summary{ParamObligations: []product.Value{typevalue.LiteralInt(reg, 1)}}); err == nil {
		t.Fatal("unsupported nonempty Summary field lowered")
	}
}
