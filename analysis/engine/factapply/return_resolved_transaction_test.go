package factapply

import (
	"context"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/sourceprojection"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestResolvedReturnTransactionScalarMatchesConcreteN5(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(7101)
	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: 7101, HasExpr: true}
	value := typevalue.String(reg)
	facts := factflow.NewFacts(factflow.FactsInput{Returns: map[cfg.Point]factflow.Return{
		point: factflow.NewReturn([]factflow.ValueSource{source}),
	}})
	resolver := visibility.NewResolver(visibility.NewTable(nil))
	input := state.Reachable(state.State{})
	output := input.WriteValue(reg, key.SymbolValue(9991), presentValue(reg))

	want := referenceApplyReturn(transfer.NodeContext{Registry: reg, Point: point}, facts,
		&recordingSourceValues{values: map[factflow.ValueSource]product.Value{source: value}},
		func(cfg.Point) state.State { return input }, input, output, mustReturnFact(t, facts, point), resolver, nil, typevalue.NewCache())
	got := applyResolvedReturnForTest(t, reg, facts, resolver, point, input, output, map[factflow.ValueSource]product.Value{source: value})
	assertStateEqual(t, reg, got, want)
	assertValue(t, reg, got, key.ReturnSlot(0), value)

	plan, ok := PlanReturnTransaction(facts, point)
	if !ok {
		t.Fatal("return plan missing")
	}
	values := []product.Value{value}
	if allocations := testing.AllocsPerRun(1000, func() {
		if _, bound := plan.Bind(reg, values); !bound {
			t.Fatal("bind failed")
		}
	}); allocations != 0 {
		t.Fatalf("Bind allocations = %v, want 0", allocations)
	}
}

func TestResolvedReturnTransactionPathMatchesConcreteN5(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(7102)
	root := symbol.ID(7102)
	ref := factflow.ExprRef(7102)
	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: ref, HasExpr: true}
	path := pathdom.NewPath(root, "result")
	pathValue := presentValue(reg)
	facts := factflow.NewFacts(factflow.FactsInput{
		Returns:         map[cfg.Point]factflow.Return{point: factflow.NewReturn([]factflow.ValueSource{source})},
		ExpressionPaths: map[factflow.ExprRef]pathdom.Path{ref: path},
	})
	builder := visibility.NewBuilder()
	builder.Define(point, root, "result")
	resolver := visibility.NewResolver(builder.Build())
	input := state.Reachable(state.State{}).WriteValue(reg, key.SymbolValue(root), pathValue)
	providerValue := product.Top()
	want := referenceApplyReturn(transfer.NodeContext{Registry: reg, Point: point}, facts,
		&recordingSourceValues{values: map[factflow.ValueSource]product.Value{source: providerValue}},
		func(cfg.Point) state.State { return input }, input, input, mustReturnFact(t, facts, point), resolver, nil, typevalue.NewCache())
	got := applyResolvedReturnForTest(t, reg, facts, resolver, point, input, input, map[factflow.ValueSource]product.Value{source: providerValue})
	assertStateEqual(t, reg, got, want)
	assertValue(t, reg, got, key.ReturnSlot(0), pathValue)
}

func TestReturnedFreshObjectGraphNeverContainsStack(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(7103)
	rootSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: 7103, HasExpr: true}
	nestedSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: 7104, HasExpr: true}
	leafSource := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: 7105, HasExpr: true}
	rootID := testTableLiteralID(rootSource.ExprRef)
	nestedID := testTableLiteralID(nestedSource.ExprRef)
	rootValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(rootID))
	nestedValue := product.Set(reg, presentValue(reg), identity.Key, identity.Singleton(nestedID))
	leafValue := presentValue(reg)
	facts := factflow.NewFacts(factflow.FactsInput{
		Returns: map[cfg.Point]factflow.Return{point: factflow.NewReturn([]factflow.ValueSource{rootSource})},
		ObjectLiterals: map[factflow.ExprRef]factflow.ObjectLiteral{
			rootSource.ExprRef: factflow.NewObjectLiteral([]factflow.ObjectEntry{
				factflow.NewObjectEntryWithMetadata(fieldSuffix("child"), nestedSource, factflow.SourceSpan{}, ""),
			}).WithIdentity(rootID),
			nestedSource.ExprRef: factflow.NewObjectLiteral([]factflow.ObjectEntry{
				factflow.NewObjectEntryWithMetadata(fieldSuffix("leaf"), leafSource, factflow.SourceSpan{}, ""),
			}).WithIdentity(nestedID),
		},
	})
	resolver := visibility.NewResolver(visibility.NewTable(nil))
	values := map[factflow.ValueSource]product.Value{rootSource: rootValue, nestedSource: nestedValue, leafSource: leafValue}
	input := state.Reachable(state.State{})
	typeValues := typevalue.NewCache()
	want := referenceApplyReturn(transfer.NodeContext{Registry: reg, Point: point}, facts,
		&recordingSourceValues{values: values}, func(cfg.Point) state.State { return input }, input, input,
		mustReturnFact(t, facts, point), resolver, nil, typeValues)
	got := applyResolvedReturnForTest(t, reg, facts, resolver, point, input, input, values)
	assertStateEqual(t, reg, got, want)
	assertValue(t, reg, got, key.ReturnSlot(0), want.ReadReturnSlot(reg, 0))
	assertHeapStaticMember(t, reg, resolver.KeySpace(), got, rootSource.ExprRef, ".child", nestedValue)
	assertHeapStaticMember(t, reg, resolver.KeySpace(), got, nestedSource.ExprRef, ".leaf", leafValue)
	for _, id := range []identity.ID{rootID, nestedID} {
		if gotPlacement := got.ReadPlacement(id); gotPlacement == placement.Stack {
			t.Fatalf("returned fresh graph placement[%v] = stack, want heap", id)
		}
		assertPlacement(t, got, id, placement.OwnedHeap)
	}

	plan, ok := PlanReturnTransaction(facts, point)
	if !ok || plan.SourceCount() != 3 {
		t.Fatalf("recursive source inventory = %d/%v, want 3/true", plan.SourceCount(), ok)
	}
}

func TestResolvedReturnTransactionZeroSourceMatchesConcreteN5(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(7104)
	facts := factflow.NewFacts(factflow.FactsInput{Returns: map[cfg.Point]factflow.Return{point: factflow.NewReturn(nil)}})
	resolver := visibility.NewResolver(visibility.NewTable(nil))
	input := state.Reachable(state.State{}).WriteValue(reg, key.SymbolValue(9994), presentValue(reg))
	want := referenceApplyReturn(transfer.NodeContext{Registry: reg, Point: point}, facts, &recordingSourceValues{},
		func(cfg.Point) state.State { return input }, input, input, mustReturnFact(t, facts, point), resolver, nil, typevalue.NewCache())
	got := applyResolvedReturnForTest(t, reg, facts, resolver, point, input, input, nil)
	assertStateEqual(t, reg, got, want)
	assertStateEqual(t, reg, got, input)
}

func TestResolvedReturnTransactionDuplicateSourceMatchesConcreteN5(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(7105)
	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: 7110, HasExpr: true, TargetIndex: 0}
	value := typevalue.String(reg)
	facts := factflow.NewFacts(factflow.FactsInput{Returns: map[cfg.Point]factflow.Return{
		point: factflow.NewReturn([]factflow.ValueSource{source, source}),
	}})
	resolver := visibility.NewResolver(visibility.NewTable(nil))
	input := state.Reachable(state.State{})
	want := referenceApplyReturn(transfer.NodeContext{Registry: reg, Point: point}, facts,
		&recordingSourceValues{values: map[factflow.ValueSource]product.Value{source: value}},
		func(cfg.Point) state.State { return input }, input, input, mustReturnFact(t, facts, point), resolver, nil, typevalue.NewCache())
	got := applyResolvedReturnForTest(t, reg, facts, resolver, point, input, input, map[factflow.ValueSource]product.Value{source: value})
	assertStateEqual(t, reg, got, want)
	plan, ok := PlanReturnTransaction(facts, point)
	if !ok || !plan.Valid() || plan.SourceCount() != 1 {
		t.Fatalf("duplicate-source plan = %#v/%v, want one indexed source", plan, ok)
	}
}

func TestResolvedReturnTransactionCancellationPublishesNoPrefix(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(7106)
	source := factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: 7111, HasExpr: true}
	facts := factflow.NewFacts(factflow.FactsInput{Returns: map[cfg.Point]factflow.Return{
		point: factflow.NewReturn([]factflow.ValueSource{source}),
	}})
	plan, ok := PlanReturnTransaction(facts, point)
	if !ok {
		t.Fatal("return plan missing")
	}
	transaction, ok := plan.Bind(reg, []product.Value{typevalue.String(reg)})
	if !ok {
		t.Fatal("return bind failed")
	}
	resolver := visibility.NewResolver(visibility.NewTable(nil))
	authority := NewReturnAuthority(NewPathSemanticAuthority(resolver, nil, typevalue.NewCache()), facts)
	input := state.Reachable(state.State{})
	output := input.WriteValue(reg, key.SymbolValue(9996), presentValue(reg))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, err := authority.Apply(ctx, reg, transaction, input, output)
	if err == nil {
		t.Fatal("canceled return apply succeeded")
	}
	assertStateEqual(t, reg, got, output)
}

func TestResolvedReturnSourceProviderUsesFrozenConstantTimeIndexWithoutAllocation(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(7107)
	const count = 1024
	sources := make([]factflow.ValueSource, count)
	values := make([]product.Value, count)
	for index := range sources {
		sources[index] = factflow.ValueSource{Kind: factflow.ValueSourceExpression, ExprRef: factflow.ExprRef(7200 + index), HasExpr: true}
		values[index] = presentValue(reg)
	}
	facts := factflow.NewFacts(factflow.FactsInput{Returns: map[cfg.Point]factflow.Return{point: factflow.NewReturn(sources)}})
	plan, ok := PlanReturnTransaction(facts, point)
	if !ok || plan.SourceCount() != count {
		t.Fatalf("source inventory = %d/%v, want %d/true", plan.SourceCount(), ok, count)
	}
	transaction, ok := plan.Bind(reg, values)
	if !ok {
		t.Fatal("return bind failed")
	}
	provider := resolvedReturnSources{index: transaction.plan.index, values: transaction.values}
	last := sources[len(sources)-1]
	if allocations := testing.AllocsPerRun(1000, func() {
		if _, found := provider.ValueOfSource(point, last, state.State{}, nil); !found {
			t.Fatal("last frozen source missing")
		}
	}); allocations != 0 {
		t.Fatalf("provider allocations = %v, want 0", allocations)
	}
}

func applyResolvedReturnForTest(
	t *testing.T,
	reg *axis.Registry,
	facts factflow.Facts,
	resolver *visibility.Resolver,
	point cfg.Point,
	input state.State,
	output state.State,
	values map[factflow.ValueSource]product.Value,
) state.State {
	t.Helper()
	plan, ok := PlanReturnTransaction(facts, point)
	if !ok || !plan.Valid() {
		t.Fatalf("invalid return plan: %#v/%v", plan, ok)
	}
	resolvedValues := make([]product.Value, plan.SourceCount())
	for index := range resolvedValues {
		source, _ := plan.Source(index)
		value, present := values[source]
		if !present {
			t.Fatalf("missing source value for %#v", source)
		}
		resolvedValues[index] = value
	}
	transaction, ok := plan.Bind(reg, resolvedValues)
	if !ok {
		t.Fatal("return bind failed")
	}
	authority := NewReturnAuthority(NewPathSemanticAuthority(resolver, nil, typevalue.NewCache()), facts)
	got, err := authority.Apply(context.Background(), reg, transaction, input, output)
	if err != nil {
		t.Fatalf("resolved return apply: %v", err)
	}
	return got
}

func mustReturnFact(t *testing.T, facts factflow.Facts, point cfg.Point) factflow.Return {
	t.Helper()
	fact, ok := facts.Return(point)
	if !ok {
		t.Fatal("return fact missing")
	}
	return fact
}

// referenceApplyReturn is the pre-extraction N5 implementation retained only
// as an independent semantic oracle for the resolved transaction tests.
func referenceApplyReturn(
	ctx transfer.NodeContext,
	facts factflow.Facts,
	sources sourcevalue.SourceValues,
	read func(cfg.Point) state.State,
	in state.State,
	out state.State,
	fact factflow.Return,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	typeValues *typevalue.Cache,
) state.State {
	var edit state.ValueEdit
	editing := false
	var returnSlots []int
	var declaredReturnSources map[int]struct{}
	for i, source := range fact.Sources() {
		targetIndex := source.TargetIndex
		if targetIndex < 0 {
			targetIndex = i
		}
		hasDeclaredContract := returnSourceHasDeclaredContract(facts, source)
		value, ok := returnSourceValue(ctx, facts, sources, read, in, out, source, resolver, projectPath, typeValues)
		if !ok {
			value = product.Top()
		} else {
			cache := newObjectLiteralSourceCache(ctx.Point, sources, read, in, out)
			cache.seed(source, value)
			out = materializeObjectLiteralHeapBatchWithCache(ctx, resolver, facts.ObjectLiteralView, out, []factflow.ValueSource{source}, typeValues, cache)
			out = applyPlacementTransition(ctx.Registry, out, value, placement.EscapeTransitionReturn)
			if !hasDeclaredContract {
				if projected, projectedOK := sourceprojection.HeapObjectContainerType(ctx.Registry, typeValues, out, value); projectedOK {
					value = typevalue.WithWitness(ctx.Registry, value, projected)
				}
			} else {
				if declaredReturnSources == nil {
					declaredReturnSources = make(map[int]struct{}, 1)
				}
				declaredReturnSources[targetIndex] = struct{}{}
			}
		}
		if !editing {
			edit = out.EditValues(ctx.Registry)
			editing = true
		}
		edit.WriteReturnSlot(targetIndex, value)
		returnSlots = append(returnSlots, targetIndex)
	}
	if !editing {
		return out
	}
	out = edit.DoneOn(out)
	for _, index := range returnSlots {
		if _, declared := declaredReturnSources[index]; declared {
			continue
		}
		value := out.ReadReturnSlot(ctx.Registry, index)
		projected, ok := sourceprojection.HeapObjectContainerType(ctx.Registry, typeValues, out, value)
		if !ok {
			continue
		}
		out = out.WriteReturnSlot(ctx.Registry, index, typevalue.WithWitness(ctx.Registry, value, projected))
	}
	return out
}
