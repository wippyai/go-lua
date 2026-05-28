package abstract

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	abstractcore "github.com/wippyai/go-lua/compiler/check/abstract/core"
	"github.com/wippyai/go-lua/compiler/check/abstract/predicate"
	"github.com/wippyai/go-lua/compiler/check/domain/path"
	basecfg "github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
)

// ExtractConditionReads records every field/index access path read by the
// function body into flow.Inputs.ConditionExtraReads, keyed by the CFG point
// that consumes it.
//
// Path-condition projection (types/flow/propagate) forgets a condition fact
// once all of its referenced access paths are dead — no read at or after the
// point. Soundness therefore depends on every real read being recorded as
// demand: a guard whose narrowing flows into any downstream consumer (a return
// value, a call argument, a field/index read, an operand, a table constructor,
// an index key) must stay live there, or projection would drop a fact a
// downstream query needs.
//
// The flow inputs already encode some reads structurally (edge-guard subjects,
// whole-expression assignment sources, container mutator paths, phi operands).
// What they do NOT encode is the field-precise read embedded inside an
// arbitrary expression: `x` or `x.f` read inside an arithmetic operand, an
// index key, a nested call argument, a relational comparison, or a table
// constructor. This walks every expression of every graph event and records the
// access path of each variable/field/index access at its consuming point. Both
// root reads (a guard narrowing a whole local, e.g. a type-test or
// local-predicate guard, must stay live wherever that local is used) and field
// reads (x.f) are recorded; ancestor-prefix demand (x for a read of x.f) is
// derived in the liveness solve, so recording the maximal access path at each
// node suffices.
func ExtractConditionReads(fc *abstractcore.FlowContext, inputs *flow.Inputs) {
	if fc == nil || fc.Graph == nil || inputs == nil {
		return
	}
	bindings := fc.Graph.Bindings()
	if bindings == nil {
		return
	}

	rec := &readRecorder{
		graph:    fc.Graph,
		bindings: bindings,
		inputs:   inputs,
		reads:    make(map[basecfg.Point][]constraint.Path),
	}

	fc.Graph.EachAssign(func(p cfg.Point, info *cfg.AssignInfo) {
		if info == nil {
			return
		}
		for _, expr := range info.Sources {
			rec.walk(p, expr)
		}
		for _, expr := range info.IterExprs {
			rec.walk(p, expr)
		}
		if info.NumericFor != nil {
			rec.walk(p, info.NumericFor.Init)
			rec.walk(p, info.NumericFor.Limit)
			rec.walk(p, info.NumericFor.Step)
		}
		// A structured assignment target (x.f = …, t[k] = …) reads its base and
		// key to locate the slot.
		for _, target := range info.Targets {
			if target.Kind == cfg.TargetField || target.Kind == cfg.TargetIndex {
				rec.walk(p, target.Base)
				rec.walk(p, target.Key)
			}
		}
	})

	fc.Graph.EachReturn(func(p cfg.Point, info *cfg.ReturnInfo) {
		if info == nil {
			return
		}
		for _, expr := range info.Exprs {
			rec.walk(p, expr)
		}
	})

	fc.Graph.EachBranch(func(p cfg.Point, info *cfg.BranchInfo) {
		if info != nil {
			rec.walk(p, info.Condition)
		}
	})

	fc.Graph.EachCall(func(p cfg.Point, info *cfg.CallInfo) {
		rec.walkCall(p, info)
	})

	if len(rec.reads) > 0 {
		inputs.ConditionExtraReads = rec.reads
	}
}

type readRecorder struct {
	graph    *cfg.Graph
	bindings *bind.BindingTable
	inputs   *flow.Inputs
	reads    map[basecfg.Point][]constraint.Path
}

func (r *readRecorder) record(p cfg.Point, expr ast.Expr) {
	constResolver := predicate.BuildConstResolver(r.inputs, p)
	pth := path.FromExprWithBindingsAt(expr, constResolver, r.bindings, r.graph, p)
	if pth.Symbol == 0 {
		return
	}
	r.reads[p] = append(r.reads[p], pth)
}

// walk records every variable/field/index access path read in expr and recurses
// into every sub-expression. A bare identifier records its root path; each
// AttrGetExpr node records the maximal access path ending at it (x.a.b at the
// outer node, x.a at the inner node); the index key of an x[k] access is itself
// recursed so a path read inside the key stays live. Root reads must be recorded
// so a guard that narrows a whole value (a type-test or local-predicate guard on
// a plain local) stays live wherever that local is consumed.
func (r *readRecorder) walk(p cfg.Point, expr ast.Expr) {
	if expr == nil {
		return
	}
	switch e := expr.(type) {
	case *ast.IdentExpr:
		r.record(p, e)
	case *ast.AttrGetExpr:
		r.record(p, e)
		r.walk(p, e.Object)
		r.walk(p, e.Key)
	case *ast.FuncCallExpr:
		r.walkCallExpr(p, e)
	case *ast.TableExpr:
		for _, field := range e.Fields {
			if field == nil {
				continue
			}
			r.walk(p, field.Key)
			r.walk(p, field.Value)
		}
	case *ast.LogicalOpExpr:
		r.walk(p, e.Lhs)
		r.walk(p, e.Rhs)
	case *ast.RelationalOpExpr:
		r.walk(p, e.Lhs)
		r.walk(p, e.Rhs)
	case *ast.StringConcatOpExpr:
		r.walk(p, e.Lhs)
		r.walk(p, e.Rhs)
	case *ast.ArithmeticOpExpr:
		r.walk(p, e.Lhs)
		r.walk(p, e.Rhs)
	case *ast.UnaryMinusOpExpr:
		r.walk(p, e.Expr)
	case *ast.UnaryNotOpExpr:
		r.walk(p, e.Expr)
	case *ast.UnaryLenOpExpr:
		r.walk(p, e.Expr)
	case *ast.UnaryBNotOpExpr:
		r.walk(p, e.Expr)
	case *ast.CastExpr:
		r.walk(p, e.Expr)
	case *ast.NonNilAssertExpr:
		r.walk(p, e.Expr)
	}
}

func (r *readRecorder) walkCall(p cfg.Point, info *cfg.CallInfo) {
	if info == nil {
		return
	}
	r.walkCallExpr(p, info.Call)
}

func (r *readRecorder) walkCallExpr(p cfg.Point, call *ast.FuncCallExpr) {
	if call == nil {
		return
	}
	r.walk(p, call.Func)
	r.walk(p, call.Receiver)
	for _, arg := range call.Args {
		r.walk(p, arg)
	}
}
