package factapply

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/cancellation"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
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
	if !state.IsBottom(reg, got) {
		t.Fatalf("conflicting simultaneous target refinements must make the route unreachable")
	}
}

func TestPathTruthinessImplicationUsesLuaPartitionAndWaitsAtJoin(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(81)
	trigger, target := symbol.ID(721), symbol.ID(722)
	builder := visibility.NewBuilder()
	builder.Define(point, trigger, "trigger")
	builder.Define(point, target, "target")
	resolver := visibility.NewResolver(builder.Build())
	ks := resolver.KeySpace()
	triggerKey := ks.FromPath(path.NewPath(trigger, "trigger"))
	targetKey := ks.FromPath(path.NewPath(target, "target"))
	stringType := typ.LiteralString("non-boolean-truth")
	truthy := typevalue.WithWitness(reg, typevalue.FromType(reg, stringType), stringType)
	falsy := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.False), typ.False)
	targetType := typ.LiteralString("selected")
	selected := typevalue.WithWitness(reg, typevalue.FromType(reg, targetType), targetType)
	row := pathevidence.NewPathTruthinessValueRefinementImplication(triggerKey, true, targetKey, selected)
	base := func(value product.Value) state.State {
		return state.State{}.
			WriteValue(reg, key.SymbolValue(trigger), value).
			WriteValue(reg, key.SymbolValue(target), product.Top()).
			AddPathPresenceImplication(row)
	}

	gotTruthy := activatePathPresenceImplications(reg, resolver, point, base(truthy))
	if got := gotTruthy.ReadValue(reg, key.SymbolValue(target)); !product.Equal(reg, got, selected) {
		t.Fatalf("non-boolean truthy trigger target = %#v", got)
	}
	gotFalsy := activatePathPresenceImplications(reg, resolver, point, base(falsy))
	if got := gotFalsy.ReadValue(reg, key.SymbolValue(target)); !product.Equal(reg, got, product.Top()) {
		t.Fatalf("falsy complement fired truthy implication: %#v", got)
	}
	joinedTrigger := product.Join(reg, truthy, falsy)
	gotJoined := activatePathPresenceImplications(reg, resolver, point, base(joinedTrigger))
	if got := gotJoined.ReadValue(reg, key.SymbolValue(target)); !product.Equal(reg, got, product.Top()) {
		t.Fatalf("mixed truthiness join fired must implication: %#v", got)
	}
}

func TestPathPresenceImplicationConflictCannotTransientlyFireDownstreamRow(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(80)
	firstTrigger, secondTrigger := symbol.ID(711), symbol.ID(712)
	target, downstream := symbol.ID(713), symbol.ID(714)
	builder := visibility.NewBuilder()
	for sym, name := range map[symbol.ID]string{
		firstTrigger: "first", secondTrigger: "second", target: "target", downstream: "downstream",
	} {
		builder.Define(point, sym, name)
	}
	resolver := visibility.NewResolver(builder.Build())
	ks := resolver.KeySpace()
	pathKey := func(sym symbol.ID, name string) keyspace.Key { return ks.FromPath(path.NewPath(sym, name)) }
	falseValue := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.False), typ.False)
	literal := func(value string) product.Value {
		typValue := typ.LiteralString(value)
		return typevalue.WithWitness(reg, typevalue.FromType(reg, typValue), typValue)
	}
	left, right, leaked := literal("left"), literal("right"), literal("leaked")
	in := state.State{}.
		WriteValue(reg, key.SymbolValue(firstTrigger), falseValue).
		WriteValue(reg, key.SymbolValue(secondTrigger), falseValue).
		WriteValue(reg, key.SymbolValue(target), product.Top()).
		WriteValue(reg, key.SymbolValue(downstream), product.Top()).
		AddPathPresenceImplication(pathevidence.NewPathValueRefinementImplication(pathKey(firstTrigger, "first"), falseValue, pathKey(target, "target"), left)).
		AddPathPresenceImplication(pathevidence.NewPathValueRefinementImplication(pathKey(secondTrigger, "second"), falseValue, pathKey(target, "target"), right)).
		AddPathPresenceImplication(pathevidence.NewPathValueRefinementImplication(pathKey(target, "target"), left, pathKey(downstream, "downstream"), leaked))

	got := activatePathPresenceImplications(reg, resolver, point, in)
	if !state.IsBottom(reg, got) {
		t.Fatalf("conflicting target was exposed transiently to its downstream row")
	}
}

func TestPathPresenceImplicationActivationUsesReturnSlotRoots(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(78)
	trigger := symbol.ID(700)
	target := symbol.ID(701)
	builder := visibility.NewBuilder()
	builder.Define(point, trigger, "trigger")
	builder.Define(point, target, "target")
	resolver := visibility.NewResolver(builder.Build())
	ks := resolver.KeySpace()
	triggerKey := ks.FromPath(path.NewPath(trigger, "trigger"))
	targetKey := ks.FromPath(path.NewPath(target, "target"))
	returnZero := ks.FromPath(path.Path{Root: "ret[0]"})
	returnOne := ks.FromPath(path.Path{Root: "ret[1]"})

	in := state.State{}.
		WriteValue(reg, key.SymbolValue(trigger), product.Absent(reg)).
		WriteValue(reg, key.SymbolValue(target), product.Top()).
		WriteValue(reg, key.ReturnSlot(0), product.Absent(reg)).
		WriteValue(reg, key.ReturnSlot(1), product.Top()).
		AddPathPresenceImplication(pathevidence.NewPathPresenceImplication(
			returnZero, presence.Absent(), targetKey, presence.Present(),
		)).
		AddPathPresenceImplication(pathevidence.NewPathPresenceImplication(
			triggerKey, presence.Absent(), returnOne, presence.Present(),
		))

	got := activatePathPresenceImplications(reg, resolver, point, in)
	if actual := product.PresenceOf(got.ReadValue(reg, key.SymbolValue(target))); !presence.Equal(actual, presence.Present()) {
		t.Fatalf("return-slot trigger target presence = %s, want present", actual)
	}
	if actual := product.PresenceOf(got.ReadValue(reg, key.ReturnSlot(1))); !presence.Equal(actual, presence.Present()) {
		t.Fatalf("return-slot target presence = %s, want present", actual)
	}
}

func TestPathPresenceImplicationActivationClosesSymbolReturnSlotChain(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(77)
	valueSymbol := symbol.ID(698)
	errorSymbol := symbol.ID(699)
	builder := visibility.NewBuilder()
	builder.Define(point, valueSymbol, "value")
	builder.Define(point, errorSymbol, "err")
	resolver := visibility.NewResolver(builder.Build())
	ks := resolver.KeySpace()
	valueKey := ks.FromPath(path.NewPath(valueSymbol, "value"))
	errorKey := ks.FromPath(path.NewPath(errorSymbol, "err"))
	returnValueKey := ks.FromPath(path.Path{Root: "ret[0]"})
	returnErrorKey := ks.FromPath(path.Path{Root: "ret[1]"})

	in := state.State{}.
		WriteValue(reg, key.SymbolValue(valueSymbol), product.Top()).
		WriteValue(reg, key.SymbolValue(errorSymbol), product.Absent(reg)).
		WriteValue(reg, key.ReturnSlot(0), product.Top()).
		WriteValue(reg, key.ReturnSlot(1), product.Top()).
		AddPathPresenceImplication(pathevidence.NewPathPresenceImplication(
			errorKey, presence.Absent(), returnValueKey, presence.Present(),
		)).
		AddPathPresenceImplication(pathevidence.NewPathPresenceImplication(
			returnValueKey, presence.Present(), returnErrorKey, presence.Absent(),
		)).
		AddPathPresenceImplication(pathevidence.NewPathPresenceImplication(
			returnErrorKey, presence.Absent(), valueKey, presence.Present(),
		))

	got := activatePathPresenceImplications(reg, resolver, point, in)
	if actual := product.PresenceOf(got.ReadValue(reg, key.SymbolValue(valueSymbol))); !presence.Equal(actual, presence.Present()) {
		t.Fatalf("symbol→ret→symbol target presence = %s, want present", actual)
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
