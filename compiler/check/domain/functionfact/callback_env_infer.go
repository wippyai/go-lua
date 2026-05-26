package functionfact

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/cfg/analysis"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// globalSetup records a _G.name = expr assignment (non-nil value).
type globalSetup struct {
	point cfg.Point
	name  string
	expr  ast.Expr
}

// globalClear records a _G.name = nil assignment.
type globalClear struct {
	point cfg.Point
	name  string
}

// paramCall records a call to a function parameter.
type paramCall struct {
	point      cfg.Point
	paramIndex int
}

// InferCallbackEnvOverlays detects the "setup -> param call -> cleanup" pattern
// using dominance and post-dominance to bracket the callback call.
// Returns map[paramIndex]map[globalName]typ.Type, or nil if no pattern detected.
func InferCallbackEnvOverlays(
	graph *cfg.Graph,
	evidence api.FlowEvidence,
	paramSlots []cfg.ParamSlot,
	synthExpr func(ast.Expr, cfg.Point) typ.Type,
	moduleBindings *bind.BindingTable,
) map[int]map[string]typ.Type {
	return InferCallbackEnvOverlaysFromSources([]CallbackEnvOverlaySource{{
		Graph:     graph,
		Evidence:  evidence,
		SynthExpr: synthExpr,
	}}, paramSlots, moduleBindings)
}

// CallbackEnvOverlaySource is one abstract-interpreter graph scope that may
// bracket a callback parameter invocation with temporary global assignments.
type CallbackEnvOverlaySource struct {
	Graph     *cfg.Graph
	Evidence  api.FlowEvidence
	SynthExpr func(ast.Expr, cfg.Point) typ.Type
}

// InferCallbackEnvOverlaysFromSources reduces callback environment evidence
// across one product family. The param slots belong to the public function
// contract; each source graph may be the function body itself or a closure that
// it returns and that invokes one of those captured parameters.
func InferCallbackEnvOverlaysFromSources(
	sources []CallbackEnvOverlaySource,
	paramSlots []cfg.ParamSlot,
	moduleBindings *bind.BindingTable,
) map[int]map[string]typ.Type {
	if len(sources) == 0 {
		return nil
	}
	var out map[int]map[string]typ.Type
	for _, source := range sources {
		overlays := newCallbackEnvOverlayInference(
			source.Graph,
			source.Evidence,
			paramSlots,
			source.SynthExpr,
			moduleBindings,
		).infer()
		out = joinCallbackEnvOverlayProducts(out, overlays)
	}
	return out
}

// AttachCallbackEnvOverlays attaches inferred callback environment overlays to
// the canonical function contract.
func AttachCallbackEnvOverlays(fnType *typ.Function, overlays map[int]map[string]typ.Type) *typ.Function {
	if fnType == nil || len(overlays) == 0 {
		return fnType
	}
	spec := cloneContractSpec(fnType)
	for paramIdx, overlay := range overlays {
		if len(overlay) == 0 {
			continue
		}
		cb := spec.GetCallback(paramIdx).Clone()
		if cb == nil {
			cb = &contract.CallbackSpec{Cardinality: contract.CardExactlyOnce}
		}
		cb.EnvOverlay = mergeCallbackEnvOverlay(cb.EnvOverlay, overlay)
		spec.WithCallback(paramIdx, cb)
	}
	return rebuildFunctionWithSpec(fnType, spec)
}

func mergeCallbackEnvOverlay(base, overlay map[string]typ.Type) map[string]typ.Type {
	if len(base) == 0 && len(overlay) == 0 {
		return nil
	}
	out := make(map[string]typ.Type, len(base)+len(overlay))
	for name, t := range base {
		if name != "" && t != nil {
			out[name] = t
		}
	}
	for name, candidate := range overlay {
		if name == "" || candidate == nil {
			continue
		}
		if existing := out[name]; existing != nil {
			out[name] = value.JoinPrecise(existing, candidate)
		} else {
			out[name] = candidate
		}
	}
	return out
}

func joinCallbackEnvOverlayProducts(base, overlay map[int]map[string]typ.Type) map[int]map[string]typ.Type {
	if len(base) == 0 && len(overlay) == 0 {
		return nil
	}
	out := make(map[int]map[string]typ.Type, len(base)+len(overlay))
	for idx, env := range base {
		out[idx] = mergeCallbackEnvOverlay(nil, env)
	}
	for idx, env := range overlay {
		out[idx] = mergeCallbackEnvOverlay(out[idx], env)
	}
	return out
}

type callbackEnvOverlayInference struct {
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

func newCallbackEnvOverlayInference(
	graph *cfg.Graph,
	evidence api.FlowEvidence,
	paramSlots []cfg.ParamSlot,
	synthExpr func(ast.Expr, cfg.Point) typ.Type,
	moduleBindings *bind.BindingTable,
) *callbackEnvOverlayInference {
	return &callbackEnvOverlayInference{
		graph:          graph,
		evidence:       evidence,
		paramSlots:     paramSlots,
		synthExpr:      synthExpr,
		moduleBindings: moduleBindings,
	}
}

func (i *callbackEnvOverlayInference) infer() map[int]map[string]typ.Type {
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

func (i *callbackEnvOverlayInference) prepare() bool {
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

func (i *callbackEnvOverlayInference) collectGlobalAssignments() {
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
				i.clears = append(i.clears, globalClear{point: p, name: name})
			} else {
				i.setups = append(i.setups, globalSetup{point: p, name: name, expr: src})
			}
		})
	}
}

func (i *callbackEnvOverlayInference) collectParameterCalls() {
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

func (i *callbackEnvOverlayInference) matchDominatedOverlays() map[int]map[string]typ.Type {
	baseCFG := i.graph.CFG()
	if baseCFG == nil {
		return nil
	}

	idom := analysis.ComputeImmediateDominators(baseCFG)
	postIdom := analysis.ComputeImmediatePostDominators(baseCFG)

	result := make(map[int]map[string]typ.Type)

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
				result[call.paramIndex] = make(map[string]typ.Type)
			}
			result[call.paramIndex][setup.name] = t
		}
	}

	if len(result) == 0 {
		return nil
	}
	return result
}

func (i *callbackEnvOverlayInference) setupType(setup globalSetup) typ.Type {
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

func (i *callbackEnvOverlayInference) hasPostDominatingClear(
	postIdom map[cfg.Point]cfg.Point,
	name string,
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
