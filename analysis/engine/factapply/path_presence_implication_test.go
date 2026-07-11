package factapply

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/cancellation"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestPathPresenceImplicationActivationMeetsSimultaneousTargetRefinements(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(79)
	firstTrigger := symbol.ID(701)
	secondTrigger := symbol.ID(702)
	target := symbol.ID(703)
	resolverBuilder := visibility.NewBuilder()
	resolverBuilder.Define(point, firstTrigger, "first")
	resolverBuilder.Define(point, secondTrigger, "second")
	resolverBuilder.Define(point, target, "target")
	resolver := visibility.NewResolver(resolverBuilder.Build())
	ks := resolver.KeySpace()

	firstKey := ks.FromPath(path.NewPath(firstTrigger, "first"))
	secondKey := ks.FromPath(path.NewPath(secondTrigger, "second"))
	targetKey := ks.FromPath(path.NewPath(target, "target"))
	falseValue := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.False), typ.False)
	literal := func(value string) product.Value {
		t := typ.LiteralString(value)
		return typevalue.WithWitness(reg, typevalue.FromType(reg, t), t)
	}

	in := state.State{}.
		WriteValue(reg, key.SymbolValue(firstTrigger), falseValue).
		WriteValue(reg, key.SymbolValue(secondTrigger), falseValue).
		WriteValue(reg, key.SymbolValue(target), product.Top()).
		AddPathPresenceImplication(pathevidence.NewPathValueRefinementImplication(firstKey, falseValue, targetKey, literal("left"))).
		AddPathPresenceImplication(pathevidence.NewPathValueRefinementImplication(secondKey, falseValue, targetKey, literal("right")))

	got := activatePathPresenceImplications(reg, resolver, point, in)
	if value := got.ReadValue(reg, key.SymbolValue(target)); !product.Equal(reg, value, product.Bottom(reg)) {
		t.Fatalf("conflicting simultaneous target refinements = %#v, want conservative bottom", value)
	}
}

func TestPathPresenceImplicationActivationStopsAtCanceledTransferSession(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(80)
	trigger := symbol.ID(704)
	target := symbol.ID(705)
	builder := visibility.NewBuilder()
	builder.Define(point, trigger, "trigger")
	builder.Define(point, target, "target")
	resolver := visibility.NewResolver(builder.Build())
	ks := resolver.KeySpace()
	triggerKey := ks.FromPath(path.NewPath(trigger, "trigger"))
	targetKey := ks.FromPath(path.NewPath(target, "target"))
	falseValue := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.False), typ.False)
	in := state.State{}.
		WriteValue(reg, key.SymbolValue(trigger), falseValue).
		WriteValue(reg, key.SymbolValue(target), product.Top()).
		AddPathPresenceImplication(pathevidence.NewPathValuePresenceImplication(triggerKey, falseValue, targetKey, presence.Present()))

	ctx, cancel := context.WithCancel(context.Background())
	_, session := cancellation.Attach(ctx)
	cancel()
	got := activatePathPresenceImplicationsWithToken(reg, resolver, point, in, session.Token())
	if !product.Equal(reg, got.ReadValue(reg, key.SymbolValue(target)), product.Top()) {
		t.Fatalf("canceled activation refined target: %#v", got.ReadValue(reg, key.SymbolValue(target)))
	}
}
