package mutator

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/abstract/core"
	"github.com/wippyai/go-lua/compiler/check/abstract/predicate"
	"github.com/wippyai/go-lua/compiler/check/callsite"
	"github.com/wippyai/go-lua/compiler/check/domain/calleffect"
	flowpath "github.com/wippyai/go-lua/compiler/check/domain/path"
	"github.com/wippyai/go-lua/compiler/check/domain/resolve"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
)

// ExtractContainerMutatorAssignments extracts container mutator assignments (channel.send-like)
// from call sites in the graph and appends them to inputs.ContainerMutatorAssignments.
func ExtractContainerMutatorAssignments(fc *core.FlowContext, inputs *flow.Inputs) {
	if fc.Graph == nil || inputs == nil {
		return
	}

	bindings := fc.Graph.Bindings()

	// Build a resolver that can look up types from the just-extracted assignments
	assignmentTypes := resolve.BuildAssignmentTypeResolver(inputs)

	for _, call := range fc.Evidence.Calls {
		p := call.Point
		info := call.Info
		if info == nil {
			continue
		}

		cm := calleffect.ContainerMutatorFromCall(info, p, fc.Derived.Synth, fc.Derived.SymResolver, assignmentTypes, fc.Graph, bindings, fc.ModuleBindings)
		if cm == nil {
			continue
		}

		targetExpr := callsite.RuntimeArgAt(info, cm.Container.Index)
		valueExpr := callsite.RuntimeArgAt(info, cm.Value.Index)

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
		if path := flowpath.FromExprWithBindingsAt(targetExpr, constResolver, bindings, fc.Graph, p); !path.IsEmpty() && path.Symbol != 0 {
			inputs.ContainerMutatorAssignments = append(inputs.ContainerMutatorAssignments, flow.ContainerMutatorAssignment{
				Point: p,
				Target: constraint.Path{
					Root:     resolve.RootNameFromBindings(bindings, path.Symbol, path.Root),
					Symbol:   path.Symbol,
					Segments: path.Segments,
				},
				ValuePath: valuePath,
				ValueType: valueType,
			})
		}
	}
}
