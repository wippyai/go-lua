package mutator

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/abstract/core"
	"github.com/wippyai/go-lua/compiler/check/abstract/tblutil"
	flowpath "github.com/wippyai/go-lua/compiler/check/domain/path"
	"github.com/wippyai/go-lua/compiler/check/domain/resolve"
	"github.com/wippyai/go-lua/compiler/pathseg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

func synthMutationValue(fc *core.FlowContext, expr ast.Expr, p cfg.Point) typ.Type {
	base := func(expr ast.Expr, p cfg.Point) typ.Type {
		if fc != nil && fc.Derived != nil && fc.Derived.Synth != nil {
			if t := fc.Derived.Synth(expr, p); t != nil {
				return t
			}
		}
		return typ.Unknown
	}

	var synth func(ast.Expr, cfg.Point) typ.Type
	synth = func(expr ast.Expr, p cfg.Point) typ.Type {
		if table, ok := expr.(*ast.TableExpr); ok && !tblutil.TableHasFunctionField(table) {
			if t := tblutil.SynthTableLiteralWithWrapper(table, p, synth); t != nil {
				return t
			}
		}
		return base(expr, p)
	}
	return synth(expr, p)
}

func mutationValueTemplate(fc *core.FlowContext, expr ast.Expr) flow.ValueTemplate {
	if fc == nil || fc.Graph == nil || expr == nil {
		return flow.ValueTemplate{}
	}
	bindings := fc.Graph.Bindings()
	if bindings == nil {
		return flow.ValueTemplate{}
	}
	var slots []flow.ValueTemplateSlot
	collectMutationValueSlots(&slots, expr, nil, bindings)
	if len(slots) == 0 {
		return flow.ValueTemplate{}
	}
	return flow.ValueTemplate{Slots: slots}
}

func collectMutationValueSlots(slots *[]flow.ValueTemplateSlot, expr ast.Expr, prefix []constraint.Segment, bindings *bind.BindingTable) {
	if expr == nil {
		return
	}
	if len(prefix) > 0 {
		if path := flowpath.FromExprWithBindings(expr, nil, bindings); !path.IsEmpty() && path.Symbol != 0 {
			path.Root = resolve.RootNameFromBindings(bindings, path.Symbol, path.Root)
			*slots = append(*slots, flow.ValueTemplateSlot{
				Segments: cloneTemplateSegments(prefix),
				Source: flow.AssignmentSource{
					Kind: flow.AssignmentSourcePath,
					Path: path,
				},
			})
			return
		}
	}
	table, ok := expr.(*ast.TableExpr)
	if !ok {
		return
	}
	arrayIndex := 1
	for _, field := range table.Fields {
		if field == nil || field.Value == nil {
			continue
		}
		seg, ok := mutationValueFieldSegment(field, &arrayIndex)
		if !ok {
			continue
		}
		next := append(cloneTemplateSegments(prefix), seg)
		collectMutationValueSlots(slots, field.Value, next, bindings)
	}
}

func mutationValueFieldSegment(field *ast.Field, arrayIndex *int) (constraint.Segment, bool) {
	if field == nil {
		return constraint.Segment{}, false
	}
	if field.Key == nil {
		idx := 1
		if arrayIndex != nil {
			idx = *arrayIndex
			*arrayIndex = *arrayIndex + 1
		}
		return constraint.Segment{Kind: constraint.SegmentIndexInt, Index: idx}, true
	}
	return pathseg.StaticTableFieldSegment(field)
}

func cloneTemplateSegments(in []constraint.Segment) []constraint.Segment {
	if len(in) == 0 {
		return nil
	}
	out := make([]constraint.Segment, len(in))
	copy(out, in)
	return out
}
