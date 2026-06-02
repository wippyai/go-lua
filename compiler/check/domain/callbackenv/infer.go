// Package callbackenv owns inference of callback-scoped global environment
// overlays.
//
// Raw name-keyed maps are accepted only at contract boundaries because
// contract.CallbackSpec.EnvOverlay is an external signature vocabulary.
// Canonical facts immediately lower GlobalName values to graph-local symbols
// before storing analysis facts.
package callbackenv

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/cfg/analysis"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

type globalSetup struct {
	point cfg.Point
	name  GlobalName
	expr  ast.Expr
}

type globalClear struct {
	point cfg.Point
	name  GlobalName
}

type paramCall struct {
	point      cfg.Point
	paramIndex int
}

// Source is one graph scope that may bracket a callback parameter invocation
// with temporary global assignments.
type Source struct {
	Graph     *cfg.Graph
	Evidence  api.FlowEvidence
	SynthExpr func(ast.Expr, cfg.Point) typ.Type
}

// Infer detects the "setup -> param call -> cleanup" pattern using dominance
// and post-dominance. It returns a deterministic domain carrier; callers convert
// to contract maps only when rebuilding external signatures.
func Infer(
	graph *cfg.Graph,
	evidence api.FlowEvidence,
	paramSlots []cfg.ParamSlot,
	synthExpr func(ast.Expr, cfg.Point) typ.Type,
	moduleBindings *bind.BindingTable,
) Overlays {
	return InferFromSources([]Source{{
		Graph:     graph,
		Evidence:  evidence,
		SynthExpr: synthExpr,
	}}, paramSlots, moduleBindings)
}

// InferFromSources reduces callback environment evidence across one product
// family. The param slots belong to the public function contract; each source
// graph may be the function body itself or a closure it returns.
func InferFromSources(
	sources []Source,
	paramSlots []cfg.ParamSlot,
	moduleBindings *bind.BindingTable,
) Overlays {
	if len(sources) == 0 {
		return nil
	}
	var out Overlays
	for _, source := range sources {
		overlays := newInference(
			source.Graph,
			source.Evidence,
			paramSlots,
			source.SynthExpr,
			moduleBindings,
		).infer()
		out = JoinProducts(out, overlays)
	}
	return out
}

type inference struct {
	graph          *cfg.Graph
	evidence       api.FlowEvidence
	paramSlots     []cfg.ParamSlot
	synthExpr      func(ast.Expr, cfg.Point) typ.Type
	moduleBindings *bind.BindingTable
	paramSet       map[cfg.SymbolID]int
	setups         []globalSetup
	clears         []globalClear
	calls          []paramCall
}

func newInference(
	graph *cfg.Graph,
	evidence api.FlowEvidence,
	paramSlots []cfg.ParamSlot,
	synthExpr func(ast.Expr, cfg.Point) typ.Type,
	moduleBindings *bind.BindingTable,
) *inference {
	return &inference{
		graph:          graph,
		evidence:       evidence,
		paramSlots:     paramSlots,
		synthExpr:      synthExpr,
		moduleBindings: moduleBindings,
	}
}

func (i *inference) infer() Overlays {
	if !i.prepare() {
		return nil
	}
	i.collectGlobalAssignments()
	i.collectParameterCalls()
	if len(i.setups) == 0 || len(i.clears) == 0 || len(i.calls) == 0 {
		return nil
	}
	return i.matchDominatedOverlays()
}

func (i *inference) prepare() bool {
	if i.graph == nil || len(i.paramSlots) == 0 || i.synthExpr == nil {
		return false
	}
	i.paramSet = make(map[cfg.SymbolID]int, len(i.paramSlots))
	for _, slot := range i.paramSlots {
		if srcIdx, ok := slot.SourceParamIndex(); ok && slot.Symbol != 0 {
			i.paramSet[slot.Symbol] = srcIdx
		}
	}
	return len(i.paramSet) > 0
}

func (i *inference) collectGlobalAssignments() {
	for _, assign := range i.evidence.Assignments {
		p := assign.Point
		info := assign.Info
		if info == nil || info.IsLocal {
			continue
		}
		info.EachTargetSource(func(_ int, target cfg.AssignTarget, src ast.Expr) {
			if target.Kind != cfg.TargetField {
				return
			}
			if target.BaseName != "_G" || len(target.FieldPath) != 1 {
				return
			}
			name := target.FieldPath[0]
			if src == nil {
				return
			}
			if _, isNil := src.(*ast.NilExpr); isNil {
				i.clears = append(i.clears, globalClear{point: p, name: GlobalName(name)})
			} else {
				i.setups = append(i.setups, globalSetup{point: p, name: GlobalName(name), expr: src})
			}
		})
	}
}

func (i *inference) collectParameterCalls() {
	for _, call := range i.evidence.Calls {
		p := call.Point
		info := call.Info
		if info == nil {
			continue
		}
		sym := callsite.SelectPreferredSymbol(
			callsite.CallableCalleeSymbolCandidates(info, i.graph, i.graph.Bindings(), i.moduleBindings),
			func(candidate cfg.SymbolID) bool {
				_, ok := i.paramSet[candidate]
				return ok
			},
		)
		if idx, ok := i.paramSet[sym]; ok {
			i.calls = append(i.calls, paramCall{point: p, paramIndex: idx})
		}
	}
}

func (i *inference) matchDominatedOverlays() Overlays {
	baseCFG := i.graph.CFG()
	if baseCFG == nil {
		return nil
	}
	idom := analysis.ComputeImmediateDominators(baseCFG)
	postIdom := analysis.ComputeImmediatePostDominators(baseCFG)
	result := make(map[int]map[GlobalName]typ.Type)
	for _, call := range i.calls {
		for _, setup := range i.setups {
			if !analysis.Dominates(idom, setup.point, call.point) {
				continue
			}
			if !i.hasPostDominatingClear(postIdom, setup.name, call.point) {
				continue
			}
			t := i.setupType(setup)
			if t == nil {
				continue
			}
			if result[call.paramIndex] == nil {
				result[call.paramIndex] = make(map[GlobalName]typ.Type)
			}
			result[call.paramIndex][setup.name] = t
		}
	}
	if len(result) == 0 {
		return nil
	}
	return overlaysFromMutableMap(result)
}

func (i *inference) setupType(setup globalSetup) typ.Type {
	if callExpr, ok := setup.expr.(*ast.FuncCallExpr); ok {
		for _, call := range i.evidence.Calls {
			if call.Point != setup.point || call.Info == nil || call.Info.Call != callExpr {
				continue
			}
			if fn := unwrap.Function(call.CalleeType); fn != nil && len(fn.Returns) > 0 && !typ.IsAbsentOrUnknown(fn.Returns[0]) {
				return fn.Returns[0]
			}
		}
	}
	return i.synthExpr(setup.expr, setup.point)
}

func (i *inference) hasPostDominatingClear(
	postIdom map[cfg.Point]cfg.Point,
	name GlobalName,
	callPoint cfg.Point,
) bool {
	for _, clr := range i.clears {
		if clr.name != name {
			continue
		}
		if analysis.PostDominates(postIdom, clr.point, callPoint) {
			return true
		}
	}
	return false
}
