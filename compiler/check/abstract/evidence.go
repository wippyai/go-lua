package abstract

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/abstract/core"
	"github.com/wippyai/go-lua/compiler/check/abstract/trace"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
)

// ExtractEvidence records abstract-interpreter events that are consumed after
// the flow solution is available. It owns AST/CFG event discovery; later phases
// only reduce this evidence with narrowed expression types.
func ExtractEvidence(fc *core.FlowContext, inputs *flow.Inputs) api.FlowEvidence {
	if fc == nil || fc.Graph == nil {
		return api.FlowEvidence{}
	}
	bindings := graphBindings(fc.Graph, fc.ModuleBindings)
	out := fc.Evidence
	fc.Evidence = out
	captured := capturedSymbolSet(bindings, fc.Fn)
	out.CapturedFields = ExtractCapturedFieldEvidence(inputs, captured)
	return out
}

// MaterializeGraphEvidence returns the canonical interpreter-owned graph event
// trace for this flow context and stores it on the context for later reducers.
func MaterializeGraphEvidence(fc *core.FlowContext) api.FlowEvidence {
	if fc == nil || fc.Graph == nil {
		return api.FlowEvidence{}
	}
	if !fc.Evidence.IsZero() {
		return fc.Evidence
	}
	fc.Evidence = trace.GraphEvidence(fc.Graph, graphBindings(fc.Graph, fc.ModuleBindings))
	return fc.Evidence
}

// ExtractCapturedFieldEvidence records lowered field writes to captured symbols.
func ExtractCapturedFieldEvidence(inputs *flow.Inputs, capturedSyms map[cfg.SymbolID]bool) []api.CapturedFieldEvidence {
	if inputs == nil || len(inputs.Assignments) == 0 || len(capturedSyms) == 0 {
		return nil
	}
	var out []api.CapturedFieldEvidence
	for _, assign := range inputs.Assignments {
		sym, field, ok := capturedFieldPathRoot(assign.TargetPath)
		if !ok || !capturedSyms[sym] {
			continue
		}
		out = append(out, api.CapturedFieldEvidence{
			Point:       assign.Point,
			Target:      sym,
			Field:       field,
			TargetPath:  assign.TargetPath,
			ValueType:   assign.Type,
			ValueSource: assign.Source,
		})
	}
	return out
}

func capturedFieldPathRoot(path constraint.Path) (cfg.SymbolID, string, bool) {
	if path.Symbol == 0 || len(path.Segments) == 0 {
		return 0, "", false
	}
	seg := path.Segments[0]
	switch seg.Kind {
	case constraint.SegmentField, constraint.SegmentIndexString:
		if seg.Name != "" {
			return path.Symbol, seg.Name, true
		}
	default:
	}
	return 0, "", false
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
