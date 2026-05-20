package mutator

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/abstract/core"
	"github.com/wippyai/go-lua/compiler/check/abstract/literal"
	"github.com/wippyai/go-lua/compiler/check/abstract/predicate"
	"github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/compiler/check/domain/calleffect"
	flowpath "github.com/wippyai/go-lua/compiler/check/domain/path"
	"github.com/wippyai/go-lua/compiler/check/domain/resolve"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

// ExtractTableMutatorAssignments extracts table mutator assignments (table.insert-like)
// from call sites in the graph and appends them to inputs.TableMutatorAssignments.
func ExtractTableMutatorAssignments(fc *core.FlowContext, inputs *flow.Inputs) {
	if fc.Graph == nil || inputs == nil {
		return
	}

	bindings := fc.Graph.Bindings()

	for _, call := range fc.Evidence.Calls {
		p := call.Point
		info := call.Info
		if info == nil {
			continue
		}
		tm := calleffect.TableMutatorFromCall(info, p, fc.Derived.Synth, fc.Derived.SymResolver, fc.Graph, bindings, fc.ModuleBindings)
		if tm == nil {
			continue
		}

		targetExpr := callsite.RuntimeArgAt(info, tm.Target.Index)
		valueExpr := callsite.RuntimeArgAt(info, tm.Value.Index)
		if targetExpr == nil || valueExpr == nil {
			continue
		}

		sc := fc.Scopes[p]
		valueType := typ.Unknown
		if fc.Derived != nil && fc.Derived.Synth != nil {
			if t := fc.Derived.Synth(valueExpr, p); t != nil {
				valueType = t
			}
		}
		valueType = resolve.Ref(valueType, sc)

		// Build value path for flow-resolved lookup at solve time
		var valuePath constraint.Path
		if ident, ok := valueExpr.(*ast.IdentExpr); ok && bindings != nil {
			if sym, found := bindings.SymbolOf(ident); found && sym != 0 {
				valuePath = constraint.Path{
					Root:   resolve.RootNameFromBindings(bindings, sym, ident.Value),
					Symbol: sym,
				}
			}
		}

		constResolver := predicate.BuildConstResolver(inputs, p)

		// Handle direct paths using bindings
		if path := flowpath.FromExprWithBindingsAt(targetExpr, constResolver, bindings, fc.Graph, p); !path.IsEmpty() && path.Symbol != 0 {
			inputs.TableMutatorAssignments = append(inputs.TableMutatorAssignments, flow.TableMutatorAssignment{
				Point: p,
				Target: constraint.Path{
					Root:     resolve.RootNameFromBindings(bindings, path.Symbol, path.Root),
					Symbol:   path.Symbol,
					Segments: path.Segments,
				},
				ValuePath: valuePath,
				ValueType: valueType,
			})
			continue
		}

		// Handle dynamic index targets like suites[suite]
		attr, ok := targetExpr.(*ast.AttrGetExpr)
		if !ok {
			continue
		}
		basePath := flowpath.FromExprWithBindingsAt(attr.Object, constResolver, bindings, fc.Graph, p)
		if basePath.IsEmpty() || basePath.Symbol == 0 {
			continue
		}
		assign := flow.TableMutatorAssignment{
			Point: p,
			Target: constraint.Path{
				Root:     resolve.RootNameFromBindings(bindings, basePath.Symbol, basePath.Root),
				Symbol:   basePath.Symbol,
				Segments: basePath.Segments,
			},
			ValuePath: valuePath,
			ValueType: valueType,
		}

		if ident, ok := attr.Key.(*ast.IdentExpr); ok && ident.Value != "" {
			if bindings != nil {
				if keySym, found := bindings.SymbolOf(ident); found && keySym != 0 {
					assign.KeySymbol = keySym
					assign.KeyVar = resolve.RootNameFromBindings(bindings, keySym, ident.Value)
				}
			}
		} else if keyType := literal.KeyTypeFromExpr(attr.Key, constResolver); keyType != nil {
			assign.KeyType = keyType
		}

		inputs.TableMutatorAssignments = append(inputs.TableMutatorAssignments, assign)
	}
}
