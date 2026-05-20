package transfer

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/abstract/trace"
	"github.com/wippyai/go-lua/compiler/check/abstract/transfer/core"
	"github.com/wippyai/go-lua/compiler/check/abstract/transfer/mutator"
	flowpath "github.com/wippyai/go-lua/compiler/check/abstract/transfer/path"
	"github.com/wippyai/go-lua/compiler/check/abstract/transfer/predicate"
	"github.com/wippyai/go-lua/compiler/check/abstract/transfer/resolve"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

// ExtractEvidence records abstract-transfer events that are consumed after the
// flow solution is available. It owns AST/CFG event discovery; later phases only
// reduce this evidence with narrowed expression types.
func ExtractEvidence(fc *core.FlowContext, inputs *flow.Inputs) api.FlowEvidence {
	if fc == nil || fc.Graph == nil {
		return api.FlowEvidence{}
	}
	bindings := graphBindings(fc.Graph, fc.ModuleBindings)
	out := fc.Evidence
	if flowEvidenceEmpty(out) {
		out = trace.GraphEvidence(fc.Graph, bindings)
	}
	fc.Evidence = out
	captured := capturedSymbolSet(bindings, fc.Fn)
	out.CapturedFields = ExtractCapturedFieldEvidence(out.Assignments, captured)
	out.CapturedContainers = ExtractCapturedContainerEvidence(fc, inputs, bindings, captured)
	return out
}

// EnsureGraphEvidence returns the canonical transfer-owned graph event trace
// for this flow context and stores it on the context for later reducers.
func EnsureGraphEvidence(fc *core.FlowContext) api.FlowEvidence {
	if fc == nil || fc.Graph == nil {
		return api.FlowEvidence{}
	}
	if !flowEvidenceEmpty(fc.Evidence) {
		return fc.Evidence
	}
	fc.Evidence = trace.GraphEvidence(fc.Graph, graphBindings(fc.Graph, fc.ModuleBindings))
	return fc.Evidence
}

func flowEvidenceEmpty(e api.FlowEvidence) bool {
	return len(e.Calls) == 0 &&
		len(e.Returns) == 0 &&
		len(e.Assignments) == 0 &&
		len(e.Branches) == 0 &&
		!e.NormalExit.Valid &&
		len(e.IdentifierUses) == 0 &&
		len(e.FieldDefaults) == 0 &&
		len(e.FreshTableLiterals) == 0 &&
		len(e.FunctionDefinitions) == 0 &&
		len(e.EscapedFunctions) == 0 &&
		len(e.LocalTypePredicates) == 0 &&
		len(e.CapturedFields) == 0 &&
		len(e.CapturedContainers) == 0
}

// ExtractCapturedFieldEvidence records direct field writes to captured symbols.
func ExtractCapturedFieldEvidence(
	assignments []api.AssignmentEvidence,
	capturedSyms map[cfg.SymbolID]bool,
) []api.CapturedFieldEvidence {
	if len(assignments) == 0 || len(capturedSyms) == 0 {
		return nil
	}
	var out []api.CapturedFieldEvidence
	for _, assign := range assignments {
		p := assign.Point
		info := assign.Info
		if info == nil {
			continue
		}
		for i, target := range info.Targets {
			sym, field := capturedFieldTarget(target)
			if sym == 0 || field == "" || !capturedSyms[sym] {
				continue
			}
			var value ast.Expr
			if i < len(info.Sources) {
				value = info.Sources[i]
			}
			out = append(out, api.CapturedFieldEvidence{
				Point:  p,
				Target: sym,
				Field:  field,
				Value:  value,
			})
		}
	}
	return out
}

// ExtractCapturedContainerEvidence records table/container mutator calls that
// target captured symbols.
func ExtractCapturedContainerEvidence(
	fc *core.FlowContext,
	inputs *flow.Inputs,
	bindings *bind.BindingTable,
	capturedSyms map[cfg.SymbolID]bool,
) []api.CapturedContainerEvidence {
	if fc == nil || fc.Graph == nil || len(capturedSyms) == 0 {
		return nil
	}
	EnsureGraphEvidence(fc)

	var out []api.CapturedContainerEvidence
	assignmentTypes := resolve.BuildAssignmentTypeResolver(inputs)
	constResolverAt := func(p cfg.Point) func(string) *flow.ConstValue {
		if inputs == nil {
			return nil
		}
		return predicate.BuildConstResolver(inputs, p)
	}
	for _, call := range fc.Evidence.Calls {
		p := call.Point
		info := call.Info
		if info == nil {
			continue
		}

		if ceu := mutator.ContainerMutatorFromCall(
			info,
			p,
			derivedSynth(fc),
			derivedSymResolver(fc),
			assignmentTypes,
			fc.Graph,
			bindings,
			fc.ModuleBindings,
		); ceu != nil {
			target := callsite.RuntimeArgAt(info, ceu.Container.Index)
			value := callsite.RuntimeArgAt(info, ceu.Value.Index)
			out = appendCapturedContainerEvidence(out, fc.Graph, bindings, constResolverAt(p), capturedSyms, target, value, p, api.ContainerMutationContainerElement)
		}

		if tm := mutator.TableMutatorFromCall(
			info,
			p,
			derivedSynth(fc),
			derivedSymResolver(fc),
			fc.Graph,
			bindings,
			fc.ModuleBindings,
		); tm != nil {
			target := callsite.RuntimeArgAt(info, tm.Target.Index)
			value := callsite.RuntimeArgAt(info, tm.Value.Index)
			out = appendCapturedContainerEvidence(out, fc.Graph, bindings, constResolverAt(p), capturedSyms, target, value, p, api.ContainerMutationTableElement)
		}
	}
	return out
}

func appendCapturedContainerEvidence(
	out []api.CapturedContainerEvidence,
	graph *cfg.Graph,
	bindings *bind.BindingTable,
	constResolver func(string) *flow.ConstValue,
	capturedSyms map[cfg.SymbolID]bool,
	target ast.Expr,
	value ast.Expr,
	p cfg.Point,
	kind api.ContainerMutationKind,
) []api.CapturedContainerEvidence {
	if target == nil || value == nil {
		return out
	}
	path := flowpath.FromExprWithBindingsAt(target, constResolver, bindings, graph, p)
	if path.IsEmpty() || path.Symbol == 0 || !capturedSyms[path.Symbol] {
		return out
	}
	segments := make([]constraint.Segment, len(path.Segments))
	copy(segments, path.Segments)
	return append(out, api.CapturedContainerEvidence{
		Point:    p,
		Target:   path.Symbol,
		Segments: segments,
		Value:    value,
		Kind:     kind,
	})
}

func capturedFieldTarget(target cfg.AssignTarget) (cfg.SymbolID, string) {
	switch target.Kind {
	case cfg.TargetField:
		if target.BaseSymbol != 0 && len(target.FieldPath) == 1 {
			return target.BaseSymbol, target.FieldPath[0]
		}
	case cfg.TargetIndex:
		if target.BaseSymbol != 0 && target.Key != nil {
			if key, ok := target.Key.(*ast.StringExpr); ok && key.Value != "" {
				return target.BaseSymbol, key.Value
			}
		}
	}
	return 0, ""
}

func capturedSymbolSet(bindings *bind.BindingTable, fn *ast.FunctionExpr) map[cfg.SymbolID]bool {
	if bindings == nil || fn == nil {
		return nil
	}
	captured := bindings.CapturedSymbols(fn)
	if len(captured) == 0 {
		return nil
	}
	set := make(map[cfg.SymbolID]bool, len(captured))
	for _, sym := range captured {
		if sym != 0 {
			set[sym] = true
		}
	}
	if len(set) == 0 {
		return nil
	}
	return set
}

func graphBindings(graph *cfg.Graph, module *bind.BindingTable) *bind.BindingTable {
	if graph != nil {
		if bindings := graph.Bindings(); bindings != nil {
			return bindings
		}
	}
	return module
}

func derivedSynth(fc *core.FlowContext) func(ast.Expr, cfg.Point) typ.Type {
	if fc == nil || fc.Derived == nil {
		return nil
	}
	return fc.Derived.Synth
}

func derivedSymResolver(fc *core.FlowContext) func(cfg.Point, cfg.SymbolID) (typ.Type, bool) {
	if fc == nil || fc.Derived == nil {
		return nil
	}
	return fc.Derived.SymResolver
}
