package canonical

import (
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/canonical/summary"
	"github.com/wippyai/go-lua/compiler/check/canonical/transfer"
	"github.com/wippyai/go-lua/compiler/check/scope"
	phasecore "github.com/wippyai/go-lua/compiler/check/synth/core"
	"github.com/wippyai/go-lua/types/typ"
)

// transferConfigProjection assembles the immutable semantic services and facts
// consumed by one function's per-node transfer. It keeps transfer wiring out of
// driver orchestration while preserving transfer as the owner of node semantics.
type transferConfigProjection struct {
	driver  *Driver
	program *program
	ref     summary.FuncRef
	graph   *cfg.Graph
	base    *scope.State
}

func (d *Driver) transferConfigProjection(p *program, ref summary.FuncRef) transferConfigProjection {
	return transferConfigProjection{
		driver:  d,
		program: p,
		ref:     ref,
		graph:   p.Graph(ref),
		base:    d.baseScope(),
	}
}

func (p transferConfigProjection) config() transfer.Config {
	return transfer.Config{
		Ops:               opsResolver{p.driver},
		FuncTyper:         funcTyper{d: p.driver, prog: p.program},
		CallTyper:         callTyper{d: p.driver, g: p.graph, ref: p.ref},
		TypeChecks:        p.program.facts.TypeChecks(p.ref),
		SelfType:          p.methodSelfSeed(),
		MethodReceivers:   p.program.facts.MethodReceivers(p.ref),
		SetMetatableSites: p.program.facts.SetMetatableSites(p.ref),
		MetatableIndexes:  p.program.facts.MetatableIndexes(),
		PrototypeMethods:  p.program.facts.PrototypeMethods(),
		PredicateFacts:    p.program.facts.PredicateFacts(),
		PredicateGuards:   p.program.facts.PredicateGuards(p.ref),
		CastType:          p.castType,
		TypeNameValue:     p.typeNameValue,
	}
}

func (p transferConfigProjection) castType(expr ast.TypeExpr) typ.Type {
	return p.driver.resolveType(expr, p.base)
}

// typeNameValue resolves a bare identifier naming a source `type` used as a
// value (`M.AppError = AppError`) to that type's reified Meta, the same
// MetaForName rule the synth flow applies.
func (p transferConfigProjection) typeNameValue(name string) typ.Type {
	if p.base == nil {
		return nil
	}
	if meta := p.base.MetaForName(name); meta != nil {
		return meta
	}
	return nil
}

// methodSelfSeed resolves a source-declared implicit `self` type for a method
// body's entry state. It applies only to a method/field definition whose receiver
// resolves in the type namespace. Value receivers are runtime facts and flow
// through the PrototypeSelf product axis, not through declared entry seeding.
func (p transferConfigProjection) methodSelfSeed() typ.Type {
	if p.graph == nil {
		return nil
	}
	fn := p.graph.Func()
	if fn == nil {
		return nil
	}
	ref, ok := p.program.refByFunc(fn)
	if !ok {
		return nil
	}
	info := p.program.methodDef(ref)
	if info == nil || info.Receiver == nil {
		return nil
	}
	bindings := p.graph.Bindings()
	if bindings == nil || !phasecore.HasUnannotatedSelfParam(fn, bindings) {
		return nil
	}
	return p.driver.namedReceiverType(info, p.driver.baseScope())
}
