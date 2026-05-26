package assign

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/domain/path"
	"github.com/wippyai/go-lua/compiler/check/domain/resolve"
	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

// EmitTableLiteralFieldAssignments emits per-field assignments for a table literal.
// This enables flow narrowing to propagate through table construction.
// For `local t = {from = obj.from}`, emits `t.from` with source path obj.from.
func EmitTableLiteralFieldAssignments(
	table *ast.TableExpr,
	targetSym cfg.SymbolID,
	targetRoot string,
	p cfg.Point,
	bindings *bind.BindingTable,
	constResolver func(string) *flow.ConstValue,
	synth func(ast.Expr, cfg.Point) typ.Type,
	sc *scope.State,
	inputs *flow.Inputs,
) {
	if table == nil || targetSym == 0 {
		return
	}

	for _, field := range table.Fields {
		if field == nil || field.Value == nil {
			continue
		}
		// Skip function fields
		if _, ok := field.Value.(*ast.FunctionExpr); ok {
			continue
		}

		seg, ok := path.StaticKeySegment(field.Key)
		if !ok {
			// Skip non-static keys (array elements / computed keys).
			continue
		}

		// Build source path from field value expression when one exists. Literal
		// and constructed values are still structural table evidence and are
		// emitted with their synthesized type below.
		var sourcePath constraint.Path
		if sp := path.FromExprWithBindings(field.Value, constResolver, bindings); !sp.IsEmpty() && sp.Symbol != 0 {
			sourcePath = constraint.Path{
				Root:     resolve.RootNameFromBindings(bindings, sp.Symbol, sp.Root),
				Symbol:   sp.Symbol,
				Segments: sp.Segments,
			}
		}

		var fieldType typ.Type
		if synth != nil {
			fieldType = synth(field.Value, p)
		}
		if sourcePath.IsEmpty() && typ.IsAbsentOrUnknown(fieldType) {
			continue
		}
		if fieldType == nil {
			fieldType = typ.Unknown
		}

		source := flow.AssignmentSource{}
		if !sourcePath.IsEmpty() {
			source = flow.AssignmentSource{
				Kind: flow.AssignmentSourcePath,
				Path: sourcePath,
			}
		}

		// Emit field assignment: t.fieldName = <source or structural value>
		inputs.Assignments = append(inputs.Assignments, flow.UnifiedAssignment{
			Point: p,
			TargetPath: constraint.Path{
				Root:     targetRoot,
				Symbol:   targetSym,
				Segments: []constraint.Segment{seg},
			},
			Source: source,
			Type:   resolve.Ref(fieldType, sc),
		})
	}
}
