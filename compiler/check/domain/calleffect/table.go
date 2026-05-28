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

// TableMutatorLengthDelta returns the constant length increase the call's
// table-mutator effect guarantees for the mutated parameter, when its Mutate
// effect carries a constant LengthDelta (table.insert is +1). It is read from
// the callee contract structurally, not by name. Returns 0 when there is no
// constant length effect.
func TableMutatorLengthDelta(
	info *cfg.CallInfo,
	tm *effect.TableMutator,
	p cfg.Point,
	synth func(ast.Expr, cfg.Point) typ.Type,
	symResolver func(cfg.Point, cfg.SymbolID) (typ.Type, bool),
	graph *cfg.Graph,
	bindings *bind.BindingTable,
	moduleBindings *bind.BindingTable,
) int64 {
	if info == nil || tm == nil {
		return 0
	}
	fnType := resolve.CalleeType(info, p, synth, symResolver, nil, graph, bindings, moduleBindings)
	if fnType == nil {
		return 0
	}
	spec := contract.ExtractSpec(fnType)
	if spec == nil {
		return 0
	}
	mut := spec.GetMutationAt(tm.Target.Index)
	if mut == nil || mut.LengthDelta == nil {
		return 0
	}
	delta, ok := mut.LengthDelta.Eval(nil)
	if !ok || delta <= 0 {
		return 0
	}
	return delta
}
