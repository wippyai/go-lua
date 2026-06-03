package equation

import (
	"math"
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/canonical/state"
	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/flow/numeric"
	"github.com/wippyai/go-lua/types/flow/propagate"
	"github.com/wippyai/go-lua/types/typ"
)

// The tests use mock NodeTransfers to prove the STRUCTURE of the combined
// equation graph (topology, predecessor join, successor emission, widening at
// the combined FVS, backward demand into contract cells) independently of the
// real value/condition/numeric transfer, which is injected later.

// straightLineGraph builds a CFG with no cycles: two sequential local
// assignments. Its FVS is empty, so the solver runs to the exact least fixed
// point with no widening.
func straightLineGraph() *cfg.Graph {
	fn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{
			&ast.LocalAssignStmt{Names: []string{"x"}, Exprs: []ast.Expr{&ast.NumberExpr{Value: "1"}}},
			&ast.LocalAssignStmt{Names: []string{"y"}, Exprs: []ast.Expr{&ast.NumberExpr{Value: "2"}}},
		},
	}
	return cfg.Build(fn)
}

// whileLoopGraph builds a CFG with a loop, whose header is a feedback-vertex
// point. A mock transfer that grows a value at the back-edge will deadlock under
// pure Join and must be terminated by widening at that header.
func whileLoopGraph() *cfg.Graph {
	fn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{
			&ast.LocalAssignStmt{Names: []string{"x"}, Exprs: []ast.Expr{&ast.NumberExpr{Value: "0"}}},
			&ast.WhileStmt{
				Condition: &ast.TrueExpr{},
				Stmts: []ast.Stmt{
					&ast.AssignStmt{
						Lhs: []ast.Expr{&ast.IdentExpr{Value: "x"}},
						Rhs: []ast.Expr{&ast.NumberExpr{Value: "1"}},
					},
				},
			},
		},
	}
	return cfg.Build(fn)
}

const growKey = constraint.PathKey("loopvar")

// pointStateWithUpper is a PointState whose numeric component bounds growKey from
// above by u. The element identity is carried entirely by the numeric axis, so a
// strictly increasing u yields a strictly ascending chain.
func pointStateWithUpper(u int64) flow.PointState {
	st := numeric.NewState()
	st.ApplyLeConst(growKey, u)
	return flow.PointState{
		Env:  map[flow.ValueKey]product.AbstractValue{},
		Cond: constraint.Domain.Bottom(),
		Num:  st,
	}
}

// upperOf reads the upper bound of growKey from a PointState, defaulting to 0
// when the bound is absent (the Bottom numeric state).
func upperOf(ps flow.PointState) int64 {
	if ps.Num == nil {
		return 0
	}
	if _, upper, ok := ps.Num.BoundsFor(growKey); ok {
		return upper
	}
	return 0
}

// TestStraightLine proves gate (a): on an acyclic graph the solver produces the
// expected per-point states by exact join, with no widening (the FVS is empty).
func TestStraightLine(t *testing.T) {
	g := straightLineGraph()

	if fvs := propagate.FeedbackVertexSet(g); len(fvs) != 0 {
		t.Fatalf("acyclic graph must have empty FVS, got %v", fvs)
	}

	// Mock: each point writes its own point id into Env under key "p" as a
	// string-typed value, and carries forward whatever incoming holds. The
	// result at a point is therefore incoming joined with the point's own mark.
	mock := NodeTransferFunc(func(
		g *cfg.Graph, p cfg.Point, incoming flow.PointState,
		_ paramevidence.Contracts, _ func(int, paramevidence.ParamContract),
	) flow.PointState {
		out := flow.PointStateDomain.Join(incoming, flow.PointStateDomain.Bottom())
		mark := map[flow.ValueKey]product.AbstractValue{markKey(p): product.FromType(typ.String)}
		out.Env = envJoin(out.Env, mark)
		return out
	})

	fs := NewBuilder(g, 0, mock).Solve()

	// Every reachable point must appear with its own mark, and a point reached
	// only after others must also carry their marks (forward accumulation).
	entry := g.Entry()
	if _, ok := fs.Points[entry]; !ok {
		t.Fatalf("entry point %v missing from result", entry)
	}
	// The exit point accumulates the marks of all points on the path to it.
	exit := g.Exit()
	if ps, ok := fs.Points[exit]; ok {
		if _, has := ps.Env[markKey(entry)]; !has {
			t.Errorf("exit state must carry the entry mark by forward join; env=%v", ps.Env)
		}
	}
	// No contracts were demanded.
	if len(fs.Contracts) != 0 {
		t.Errorf("no demand emitted, Contracts must be empty, got %v", fs.Contracts)
	}
}

// TestLoopTerminatesByWidening proves gate (b), the key property the whole
// rebuild exists for: a mock transfer that GROWS a value every loop iteration
// (the shape that deadlocks a widen-free SCC solve)
// converges under the flow engine because WidenAt fires at the loop-header
// FVS cell, jumping the unbounded numeric axis to Top.
func TestLoopTerminatesByWidening(t *testing.T) {
	g := whileLoopGraph()

	fvs := propagate.FeedbackVertexSet(g)
	if len(fvs) == 0 {
		t.Fatalf("while loop must have a non-empty FVS")
	}

	// Mock: at every point, emit a numeric upper bound one greater than the
	// largest bound on any incoming edge. On the loop back-edge this makes the
	// header's incoming strictly ascend (0,1,2,...): under pure Join (with no
	// data-axis widening) the worklist NEVER converges.
	// flow.PointStateDomain's real Cousot widen at the header FVS widens the
	// growing-above interval to unconstrained, so the chain is stationary after
	// one widening step and Solve returns. We record the maximum incoming bound
	// observed to confirm the chain genuinely ascended past 1 (not a fixed value)
	// before widening cut it.
	var maxIncoming int64
	mock := NodeTransferFunc(func(
		g *cfg.Graph, p cfg.Point, incoming flow.PointState,
		_ paramevidence.Contracts, _ func(int, paramevidence.ParamContract),
	) flow.PointState {
		if u := upperOf(incoming); u > maxIncoming {
			maxIncoming = u
		}
		return pointStateWithUpper(upperOf(incoming) + 1)
	})

	// Solve must terminate. The -timeout on the test process is the backstop;
	// the design property is that WidenAt makes it converge, not a cap.
	fs := NewBuilder(g, 0, mock).Solve()

	// The chain genuinely ascended at the loop: an incoming bound above the
	// initial step proves the value was growing each iteration (the deadlock
	// shape), not converging on its own.
	if maxIncoming < 2 {
		t.Fatalf("loop value did not ascend (max incoming upper=%d); the growth precondition is not exercised", maxIncoming)
	}

	// Widening fired at the loop-header FVS cell: its converged state carries NO
	// finite growKey upper bound. The Cousot widen of an interval growing without
	// bound above is the unconstrained interval, which the numeric domain drops.
	// A finite bound here would mean the chain was capped by an iteration count
	// rather than terminated by widening — the exact unsoundness this rebuild
	// removes.
	for p := range fvs {
		ps, ok := fs.Points[p]
		if !ok {
			t.Fatalf("loop-header FVS point %v missing from result", p)
		}
		if _, upper, bounded := ps.Num.BoundsFor(growKey); bounded {
			t.Fatalf("loop-header FVS cell %v retained a finite bound (upper=%d); widening did not fire; points=%v",
				p, upper, fvsBounds(fs, fvs))
		}
	}
}

// TestBidirectionalDemand proves gate (c): a mock body use emits an obligation
// into ContractCell(i); the entry point reads the accumulated contract (the
// assumed entry value) and the assembled FunctionState.Contracts[i] holds the
// joined demand.
func TestBidirectionalDemand(t *testing.T) {
	g := straightLineGraph()
	const param = 0
	demandType := typ.String
	want := paramevidence.DemandFromType(demandType)

	var entrySawContract bool
	entry := g.Entry()

	// Mock: the LAST point on the path (a body use) emits a demand obligation on
	// parameter 0. The entry point asserts it can read the accumulated contract
	// back — the backward demand flow closing onto entry.
	exit := g.Exit()
	mock := NodeTransferFunc(func(
		g *cfg.Graph, p cfg.Point, incoming flow.PointState,
		entryContracts paramevidence.Contracts, demand func(int, paramevidence.ParamContract),
	) flow.PointState {
		if p == entry {
			if c, ok := entryContracts[param]; ok &&
				paramevidence.ParamContractDomain.Equal(c, want) {
				entrySawContract = true
			}
		}
		if p == exit {
			demand(param, want)
		}
		return incoming
	})

	fs := NewBuilder(g, 1, mock).Solve()

	got, ok := fs.Contracts[param]
	if !ok {
		t.Fatalf("Contracts[%d] missing; backward demand did not accumulate", param)
	}
	if !paramevidence.ParamContractDomain.Equal(got, want) {
		t.Fatalf("Contracts[%d] = %v, want joined demand %v", param, got, want)
	}
	if !entrySawContract {
		t.Fatalf("entry point never read the accumulated contract; backward demand did not reach entry")
	}
}

// TestContractsAreEntryOnlyContext pins the dependency shape of the combined
// graph: parameter contracts feed the entry equation, then entry's out-state
// carries any changed assumption forward through ordinary CFG edges. Non-entry
// points must not receive contracts as a side channel, or every body point
// becomes directly contract-dependent and the graph over-widens.
func TestContractsAreEntryOnlyContext(t *testing.T) {
	g := straightLineGraph()
	const param = 0
	want := paramevidence.DemandFromType(typ.String)
	entry := g.Entry()
	exit := g.Exit()

	var entrySawContract bool
	var nonEntrySawContract bool
	mock := NodeTransferFunc(func(
		g *cfg.Graph, p cfg.Point, incoming flow.PointState,
		entryContracts paramevidence.Contracts, demand func(int, paramevidence.ParamContract),
	) flow.PointState {
		if c, ok := entryContracts[param]; ok &&
			paramevidence.ParamContractDomain.Equal(c, want) {
			if p == entry {
				entrySawContract = true
			} else {
				nonEntrySawContract = true
			}
		}
		if p == exit {
			demand(param, want)
		}
		return incoming
	})

	fs := NewBuilder(g, 1, mock).Solve()

	if got, ok := fs.Contracts[param]; !ok || !paramevidence.ParamContractDomain.Equal(got, want) {
		t.Fatalf("Contracts[%d] = %v, want %v", param, got, want)
	}
	if !entrySawContract {
		t.Fatalf("entry point never read the accumulated contract")
	}
	if nonEntrySawContract {
		t.Fatalf("non-entry point received entryContracts directly; contracts must flow through entry state")
	}
}

// TestBuilderDoesNotProbeTransferOutsideSolver locks the Kildall shape: the
// builder must not execute NodeTransfer in a fake discovery/pre-pass with Bottom
// inputs. Non-entry points may run only as solver cells, after their reachable
// predecessors have emitted the marker below.
func TestBuilderDoesNotProbeTransferOutsideSolver(t *testing.T) {
	g := straightLineGraph()
	entry := g.Entry()
	entryMark := markKey(entry)

	mock := NodeTransferFunc(func(
		g *cfg.Graph, p cfg.Point, incoming flow.PointState,
		_ paramevidence.Contracts, _ func(int, paramevidence.ParamContract),
	) flow.PointState {
		if p != entry {
			if _, ok := incoming.Env[entryMark]; !ok {
				t.Fatalf("transfer for non-entry point %v saw fake/pre-solver Bottom input; incoming=%v", p, incoming.Env)
			}
		}
		out := flow.PointStateDomain.Join(incoming, flow.PointStateDomain.Bottom())
		if p == entry {
			out.Env = envJoin(out.Env, map[flow.ValueKey]product.AbstractValue{
				entryMark: product.FromType(typ.Boolean),
			})
		}
		return out
	})

	NewBuilder(g, 0, mock).Solve()
}

// TestConditionProjectionRunsAsCellAbstraction pins the canonical acyclic
// condition bound: relevance projection is applied as a solver cell abstraction,
// after the Kildall accumulator joins into the point cell. If it were only
// applied to transfer output, stale DNF vocabulary already stored in the cell
// would survive future emits and a straight-line guard chain could still grow
// without bound.
func TestConditionProjectionRunsAsCellAbstraction(t *testing.T) {
	g := straightLineGraph()
	xa := constraint.NewPath(cfg.SymbolID(101), "x").Field("a")
	xb := constraint.NewPath(cfg.SymbolID(101), "x").Field("b")
	deadFact := constraint.Or(
		constraint.FromConstraints(constraint.Truthy{Path: xa}),
		constraint.FromConstraints(constraint.Truthy{Path: xb}),
	)

	mock := projectingTransfer{
		projector: propagate.NewConditionProjector(&propagate.Inputs{
			Graph:  g,
			Demand: &propagate.Demand{},
		}),
		fact: deadFact,
	}

	fs := NewBuilder(g, 0, mock).Solve()
	for p, ps := range fs.Points {
		if !ps.Cond.IsTrue() {
			t.Fatalf("point %v retained dead condition vocabulary: %v", p, ps.Cond)
		}
	}
	for p, ps := range fs.InPoints {
		if !ps.Cond.IsTrue() {
			t.Fatalf("in-state %v retained dead condition vocabulary: %v", p, ps.Cond)
		}
	}
}

type projectingTransfer struct {
	projector *propagate.ConditionProjector
	fact      constraint.Condition
}

func (m projectingTransfer) Transfer(
	_ *cfg.Graph,
	_ cfg.Point,
	incoming flow.PointState,
	_ paramevidence.Contracts,
	_ func(int, paramevidence.ParamContract),
) flow.PointState {
	out := flow.PointStateDomain.Join(incoming, flow.PointStateDomain.Bottom())
	out.Cond = m.fact
	return out
}

func (m projectingTransfer) ConditionProjector() *propagate.ConditionProjector {
	return m.projector
}

// markKey names the Env slot a straight-line mock writes for point p.
func markKey(p cfg.Point) flow.ValueKey {
	return flow.ValueKey("mark/" + itoa(uint64(p)))
}

func itoa(v uint64) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}

// envJoin merges b into a via the value domain, returning a fresh map.
func envJoin(a, b map[flow.ValueKey]product.AbstractValue) map[flow.ValueKey]product.AbstractValue {
	out := make(map[flow.ValueKey]product.AbstractValue, len(a)+len(b))
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		if cur, ok := out[k]; ok {
			out[k] = product.Domain.Join(cur, v)
		} else {
			out[k] = v
		}
	}
	return out
}

// fvsBounds renders the converged growKey upper bound at each FVS point for
// diagnostics on failure.
func fvsBounds(fs state.FunctionState, fvs map[cfg.Point]bool) map[cfg.Point]int64 {
	out := map[cfg.Point]int64{}
	for p := range fvs {
		if ps, ok := fs.Points[p]; ok {
			out[p] = upperOf(ps)
		} else {
			out[p] = math.MinInt64
		}
	}
	return out
}
