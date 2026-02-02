package mutator

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/core"
	flowpath "github.com/wippyai/go-lua/compiler/check/flowbuild/path"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/predicate"
	"github.com/wippyai/go-lua/compiler/check/flowbuild/resolve"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/effect"
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

	fc.Graph.EachCall(func(p cfg.Point, info *cfg.CallInfo) {
		if info == nil {
			return
		}

		cm := containerMutatorFromCall(info, p, fc.Derived.Synth, fc.Derived.SymResolver, assignmentTypes)
		if cm == nil {
			return
		}

		// For method calls, index 0 is self (receiver)
		var targetExpr, valueExpr ast.Expr
		if info.Method != "" && info.Receiver != nil {
			targetExpr = containerArgAtMethod(info.Receiver, info.Args, cm.Target.Index)
			valueExpr = containerArgAtMethod(info.Receiver, info.Args, cm.Value.Index)
		} else {
			targetExpr = ArgAtCall(info.Args, cm.Target.Index)
			valueExpr = ArgAtCall(info.Args, cm.Value.Index)
		}

		if targetExpr == nil || valueExpr == nil {
			return
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
	})
}

// ContainerElementReturnInfo holds info about a method that returns container element types.
type ContainerElementReturnInfo struct {
	ReturnIndex int             // Which return value (0-based)
	SourceRef   effect.ParamRef // Which parameter is the container
}

// ContainerElementReturnFromCall detects if a call returns a container's element type.
// Returns info about the Return effect if found, nil otherwise.
func ContainerElementReturnFromCall(info *cfg.CallInfo, p cfg.Point, synth func(ast.Expr, cfg.Point) typ.Type, symResolver func(cfg.Point, cfg.SymbolID) (typ.Type, bool), assignmentTypes func(cfg.SymbolID) typ.Type) *ContainerElementReturnInfo {
	if info == nil {
		return nil
	}

	fnType := resolve.CalleeType(info, p, synth, symResolver, assignmentTypes)
	if fnType == nil {
		return nil
	}

	spec := contract.ExtractSpec(fnType)
	if spec == nil {
		return nil
	}

	// Look for Return effects with ElementOf transform
	for _, label := range spec.Effects.Labels {
		ret, ok := label.(effect.Return)
		if !ok {
			continue
		}
		elemOf, ok := ret.Transform.(effect.ElementOf)
		if !ok {
			continue
		}
		return &ContainerElementReturnInfo{
			ReturnIndex: ret.ReturnIndex,
			SourceRef:   elemOf.Source,
		}
	}

	return nil
}

// containerMutatorInfo holds the extracted mutation info.
type containerMutatorInfo struct {
	Target effect.ParamRef
	Value  effect.ParamRef
}

func containerMutatorFromCall(info *cfg.CallInfo, p cfg.Point, synth func(ast.Expr, cfg.Point) typ.Type, symResolver func(cfg.Point, cfg.SymbolID) (typ.Type, bool), assignmentTypes func(cfg.SymbolID) typ.Type) *containerMutatorInfo {
	if info == nil {
		return nil
	}

	fnType := resolve.CalleeType(info, p, synth, symResolver, assignmentTypes)
	if fnType == nil {
		return nil
	}

	spec := contract.ExtractSpec(fnType)
	if spec == nil {
		return nil
	}

	for _, label := range spec.Effects.Labels {
		mut, ok := label.(effect.Mutate)
		if !ok {
			continue
		}
		ceu, ok := mut.Transform.(effect.ContainerElementUnion)
		if !ok {
			continue
		}
		return &containerMutatorInfo{
			Target: ceu.Container,
			Value:  ceu.Value,
		}
	}

	return nil
}

// containerArgAtMethod returns the argument at the given index for method calls.
// For method calls, index 0 is the receiver (self).
func containerArgAtMethod(receiver ast.Expr, args []ast.Expr, idx int) ast.Expr {
	if idx == 0 {
		return receiver
	}
	return ArgAtCall(args, idx-1)
}
