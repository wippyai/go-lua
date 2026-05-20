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

// TableMutatorFromCall resolves a call's callee contract and returns its
// table-mutator effect, when present.
func TableMutatorFromCall(
	info *cfg.CallInfo,
	p cfg.Point,
	synth func(ast.Expr, cfg.Point) typ.Type,
	symResolver func(cfg.Point, cfg.SymbolID) (typ.Type, bool),
	graph *cfg.Graph,
	bindings *bind.BindingTable,
	moduleBindings *bind.BindingTable,
) *effect.TableMutator {
	if info == nil {
		return nil
	}

	fnType := resolve.CalleeType(info, p, synth, symResolver, nil, graph, bindings, moduleBindings)
	if fnType == nil {
		return nil
	}

	spec := contract.ExtractSpec(fnType)
	if spec == nil {
		return nil
	}
	return spec.GetTableMutator()
}
