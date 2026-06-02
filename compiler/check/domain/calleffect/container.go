package calleffect

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/bind"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/domain/resolve"
	"github.com/wippyai/go-lua/types/contract"
	"github.com/wippyai/go-lua/types/effect"
	"github.com/wippyai/go-lua/types/typ"
)

// ContainerElementReturnInfo holds info about a method that returns container element types.
type ContainerElementReturnInfo struct {
	ReturnIndex int
	SourceRef   effect.ParamRef
}

// ContainerElementReturnFromCall detects if a call returns a container's element type.
func ContainerElementReturnFromCall(
	info *cfg.CallInfo,
	p cfg.Point,
	synth func(ast.Expr, cfg.Point) typ.Type,
	symResolver func(cfg.Point, cfg.SymbolID) (typ.Type, bool),
	assignmentTypes func(cfg.SymbolID) typ.Type,
	graph *cfg.Graph,
	bindings *bind.BindingTable,
	moduleBindings *bind.BindingTable,
) *ContainerElementReturnInfo {
	if info == nil {
		return nil
	}

	fnType := resolve.CalleeType(info, p, synth, symResolver, assignmentTypes, graph, bindings, moduleBindings)
	if fnType == nil {
		return nil
	}

	spec := contract.ExtractSpec(fnType)
	if spec == nil {
		return nil
	}

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
