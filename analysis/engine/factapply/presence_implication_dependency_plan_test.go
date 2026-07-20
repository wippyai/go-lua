package factapply

import (
	"context"
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	statekey "github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func preparePresenceDependencyPlanTest(
	t *testing.T,
	reg *axis.Registry,
	resolver *visibility.Resolver,
	point cfg.Point,
	publications []pathevidence.PathPresenceImplication,
	barriers ConcretePresenceImplicationBarriers,
) PresenceImplicationPlan {
	t.Helper()
	plan, err := NewPathSemanticAuthority(resolver, nil, nil).PreparePresenceImplicationPlan(reg, point, publications, barriers)
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func TestPresenceDependencyBlocksKeepSharedRootReadFactorizedAndAugmentOutputs(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(901)
	trigger, left, right := symbol.ID(901), symbol.ID(902), symbol.ID(903)
	builder := visibility.NewBuilder()
	builder.Define(point, trigger, "trigger")
	builder.Define(point, left, "left")
	builder.Define(point, right, "right")
	resolver := visibility.NewResolver(builder.Build())
	ks := resolver.KeySpace()
	keyOf := func(sym symbol.ID, name string) keyspace.Key { return ks.FromPath(path.NewPath(sym, name)) }
	publications := []pathevidence.PathPresenceImplication{
		pathevidence.NewPathPresenceImplication(keyOf(trigger, "trigger"), presence.Present(), keyOf(left, "left"), presence.Absent()),
		pathevidence.NewPathPresenceImplication(keyOf(trigger, "trigger"), presence.Present(), keyOf(right, "right"), presence.Absent()),
	}
	plan := preparePresenceDependencyPlanTest(t, reg, resolver, point, publications, ConcretePresenceImplicationTrailingBarrier)
	domain := state.RegisteredProductDomain(reg)
	dependency, err := plan.DependencyBlocks(domain, mustPresenceCoordinateFactorInventory(t, plan, domain, nil))
	if err != nil {
		t.Fatal(err)
	}
	stages := dependency.Stages()
	if len(stages) != 1 {
		t.Fatalf("stages = %d, want 1", len(stages))
	}
	blocks := stages[0].Blocks()
	if len(blocks) != 2 {
		t.Fatalf("shared read joined independent writers into %d blocks, want 2", len(blocks))
	}
	triggerSlot := statekey.SymbolValue(trigger)
	writes := map[statekey.Value]bool{}
	for _, block := range blocks {
		reads := block.ValueReads()
		if len(reads) == 0 || reads[0] != triggerSlot {
			t.Fatalf("block root reads = %v, want shared trigger %v", reads, triggerSlot)
		}
		for _, slot := range block.ValueWrites() {
			writes[slot] = true
		}
	}
	if !writes[statekey.SymbolValue(left)] || !writes[statekey.SymbolValue(right)] {
		t.Fatalf("block writes = %v, want both independent targets", writes)
	}
	if got := len(dependency.Slots()); got != len(publications) {
		t.Fatalf("augmented publication slots = %d, want %d", got, len(publications))
	}
}

func TestCoordinateFactorInventoryClosureBuildsOneUnionDependencyUniverse(t *testing.T) {
	type fixture struct {
		authority *PathSemanticAuthority
		domain    state.ProductDomain
		seed      state.CoordinateFactorInventory
		width     int
	}
	build := func(width int) fixture {
		t.Helper()
		reg := standard.Registry()
		point := cfg.Point(897)
		builder := visibility.NewBuilder()
		for index := 0; index < width; index++ {
			builder.Define(point, symbol.ID(1200+index*2), fmt.Sprintf("trigger%d", index))
			builder.Define(point, symbol.ID(1201+index*2), fmt.Sprintf("target%d", index))
		}
		resolver := visibility.NewResolver(builder.Build())
		authority := NewPathSemanticAuthority(resolver, nil, nil)
		domain := state.RegisteredProductDomain(reg)
		slots := make([]state.CoordinateSlot, width)
		for index := range slots {
			row := pathevidence.NewPathPresenceImplication(
				resolver.KeySpace().FromPath(path.NewPath(symbol.ID(1200+index*2), fmt.Sprintf("trigger%d", index))), presence.Present(),
				resolver.KeySpace().FromPath(path.NewPath(symbol.ID(1201+index*2), fmt.Sprintf("target%d", index))), presence.Present(),
			)
			var err error
			slots[index], err = domain.PresenceImplicationCoordinateSlot(resolver.KeySpace(), row)
			if err != nil {
				t.Fatal(err)
			}
		}
		seed, err := domain.SealCoordinateFactorInventory(resolver.KeySpace(), slots)
		if err != nil {
			t.Fatal(err)
		}
		return fixture{authority: authority, domain: domain, seed: seed, width: width}
	}
	close := func(f fixture) (state.CoordinateFactorInventory, int) {
		t.Helper()
		closed, constructions, err := f.authority.closeCoordinateFactorInventory(f.domain, f.seed)
		if err != nil {
			t.Fatal(err)
		}
		if closed.Len() < f.width {
			t.Fatalf("closed inventory width = %d, want at least %d publication rows", closed.Len(), f.width)
		}
		return closed, constructions
	}

	one, many := build(1), build(32)
	_, oneConstructions := close(one)
	_, manyConstructions := close(many)
	if oneConstructions != manyConstructions || oneConstructions != 1 {
		t.Fatalf("union dependency constructions width 1=%d width 32=%d, want one exact-universe construction independent of producer width", oneConstructions, manyConstructions)
	}
}

func TestPresencePublicationDeltaSchedulesOnlyAffectedIndependentBlock(t *testing.T) {
	reg := standard.Registry()
	domain := state.RegisteredProductDomain(reg)
	point := cfg.Point(900)
	const width = 16
	builder := visibility.NewBuilder()
	rows := make([]pathevidence.PathPresenceImplication, width)
	for index := 0; index < width; index++ {
		trigger := symbol.ID(1000 + index*2)
		target := symbol.ID(1001 + index*2)
		builder.Define(point, trigger, "trigger")
		builder.Define(point, target, "target")
	}
	resolver := visibility.NewResolver(builder.Build())
	ks := resolver.KeySpace()
	slots := make([]state.CoordinateSlot, 0, width)
	for index := 0; index < width; index++ {
		trigger := symbol.ID(1000 + index*2)
		target := symbol.ID(1001 + index*2)
		row := pathevidence.NewPathPresenceImplication(
			ks.FromPath(path.NewPath(trigger, "trigger")), presence.Present(),
			ks.FromPath(path.NewPath(target, "target")), presence.Present(),
		)
		rows[index] = row
		slot, err := domain.PresenceImplicationCoordinateSlot(ks, row)
		if err != nil {
			t.Fatal(err)
		}
		slots = append(slots, slot)
	}
	inventory, err := domain.SealCoordinateFactorInventory(ks, slots)
	if err != nil {
		t.Fatal(err)
	}
	totalBlocks := 0
	for publishedIndex, publication := range rows {
		plan := preparePresenceDependencyPlanTest(t, reg, resolver, point, []pathevidence.PathPresenceImplication{publication}, ConcretePresenceImplicationTrailingBarrier)
		dependency, dependencyErr := plan.DependencyBlocks(domain, inventory)
		if dependencyErr != nil {
			t.Fatal(dependencyErr)
		}
		stages := dependency.Stages()
		if len(stages) != 1 || len(stages[0].Blocks()) != 1 {
			t.Fatalf("publication %d scheduled %d stages/%d blocks, want one reducer barrier and one affected block", publishedIndex, len(stages), len(stages[0].Blocks()))
		}
		block := stages[0].Blocks()[0]
		if len(block.rows) != 1 || block.rows[0] != publication {
			t.Fatalf("publication %d selected rows=%v, want only its own row", publishedIndex, block.rows)
		}
		totalBlocks += len(stages[0].Blocks())
	}
	if totalBlocks != width {
		t.Fatalf("%d independent publications scheduled %d total blocks, want O(N)=%d", width, totalBlocks, width)
	}
}

func TestPresencePublicationDeltaKeepsDownstreamConeAndExcludesIndependentBlock(t *testing.T) {
	reg := standard.Registry()
	domain := state.RegisteredProductDomain(reg)
	point := cfg.Point(899)
	symbols := []symbol.ID{1101, 1102, 1103, 1104, 1105}
	builder := visibility.NewBuilder()
	for index, id := range symbols {
		builder.Define(point, id, fmt.Sprintf("v%d", index))
	}
	resolver := visibility.NewResolver(builder.Build())
	ks := resolver.KeySpace()
	key := func(index int) keyspace.Key {
		return ks.FromPath(path.NewPath(symbols[index], fmt.Sprintf("v%d", index)))
	}
	rows := []pathevidence.PathPresenceImplication{
		pathevidence.NewPathPresenceImplication(key(0), presence.Present(), key(1), presence.Present()),
		pathevidence.NewPathPresenceImplication(key(1), presence.Present(), key(2), presence.Present()),
		pathevidence.NewPathPresenceImplication(key(3), presence.Present(), key(4), presence.Present()),
	}
	slots := make([]state.CoordinateSlot, len(rows))
	for index, row := range rows {
		var err error
		slots[index], err = domain.PresenceImplicationCoordinateSlot(ks, row)
		if err != nil {
			t.Fatal(err)
		}
	}
	inventory, err := domain.SealCoordinateFactorInventory(ks, slots)
	if err != nil {
		t.Fatal(err)
	}
	plan := preparePresenceDependencyPlanTest(t, reg, resolver, point, rows[:1], ConcretePresenceImplicationTrailingBarrier)
	dependency, err := plan.DependencyBlocks(domain, inventory)
	if err != nil {
		t.Fatal(err)
	}
	stages := dependency.Stages()
	if len(stages) != 1 || len(stages[0].Blocks()) != 2 {
		t.Fatalf("publication cone=%d stages/%d blocks, want A and downstream B only", len(stages), len(stages[0].Blocks()))
	}
	selected := make(map[pathevidence.PathPresenceImplication]bool)
	for _, block := range stages[0].Blocks() {
		for _, row := range block.rows {
			selected[row] = true
		}
	}
	if !selected[rows[0]] || !selected[rows[1]] || selected[rows[2]] {
		t.Fatalf("publication cone selected=%v, want A+B and not independent C", selected)
	}
}

func TestPresenceDependencyPlanPreservesDescendantBarrierStagesAndExactSlotIdentity(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(902)
	trigger, root, first, later := symbol.ID(911), symbol.ID(912), symbol.ID(913), symbol.ID(914)
	builder := visibility.NewBuilder()
	for sym, name := range map[symbol.ID]string{trigger: "trigger", root: "root", first: "first", later: "later"} {
		builder.Define(point, sym, name)
	}
	resolver := visibility.NewResolver(builder.Build())
	ks := resolver.KeySpace()
	keyOf := func(sym symbol.ID, name string) keyspace.Key { return ks.FromPath(path.NewPath(sym, name)) }
	publications := []pathevidence.PathPresenceImplication{
		pathevidence.NewPathPresenceImplication(keyOf(trigger, "trigger"), presence.Present(), keyOf(first, "first"), presence.Present()),
		pathevidence.NewPathPresenceImplication(keyOf(trigger, "trigger"), presence.Present(), keyOf(root, "root"), presence.Absent()),
		pathevidence.NewPathPresenceImplication(keyOf(trigger, "trigger"), presence.Present(), keyOf(later, "later"), presence.Present()),
	}
	plan := preparePresenceDependencyPlanTest(t, reg, resolver, point, publications, ConcretePresenceImplicationDescendantInvalidationBarriers)
	domain := state.RegisteredProductDomain(reg)
	dependency, err := plan.DependencyBlocks(domain, mustPresenceCoordinateFactorInventory(t, plan, domain, nil))
	if err != nil {
		t.Fatal(err)
	}
	stages := dependency.Stages()
	if len(stages) != 3 {
		t.Fatalf("barrier stages = %d, want 3", len(stages))
	}
	for index, stage := range stages {
		got := stage.Publications()
		if len(got) != 1 || got[0] != publications[index] {
			t.Fatalf("stage %d publications = %#v, want original publication %d", index, got, index)
		}
		writes := stage.ReducerWrites()
		if len(writes) != 1 {
			t.Fatalf("stage %d reducer writes = %d, want one exact slot", index, len(writes))
		}
		want, slotErr := domain.PresenceImplicationCoordinateSlot(ks, publications[index])
		if slotErr != nil {
			t.Fatal(slotErr)
		}
		equal, equalErr := domain.CoordinateSlotEqual(writes[0], want)
		if equalErr != nil || !equal {
			t.Fatalf("stage %d reducer slot drifted: equal=%t err=%v", index, equal, equalErr)
		}
	}
}

func TestPresenceDependencyCoordinateRoundDoesNotSelfCloseTransitively(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(903)
	first, second, third := symbol.ID(921), symbol.ID(922), symbol.ID(923)
	builder := visibility.NewBuilder()
	for sym, name := range map[symbol.ID]string{first: "first", second: "second", third: "third"} {
		builder.Define(point, sym, name)
	}
	resolver := visibility.NewResolver(builder.Build())
	ks := resolver.KeySpace()
	keyOf := func(sym symbol.ID, name string) keyspace.Key { return ks.FromPath(path.NewPath(sym, name)) }
	rows := []pathevidence.PathPresenceImplication{
		pathevidence.NewPathPresenceImplication(keyOf(first, "first"), presence.Present(), keyOf(second, "second"), presence.Present()),
		pathevidence.NewPathPresenceImplication(keyOf(second, "second"), presence.Present(), keyOf(third, "third"), presence.Present()),
		pathevidence.NewPathPresenceImplication(keyOf(third, "third"), presence.Present(), keyOf(first, "first"), presence.Present()),
	}
	domain := state.RegisteredProductDomain(reg)
	plan := preparePresenceDependencyPlanTest(t, reg, resolver, point, rows, ConcretePresenceImplicationTrailingBarrier)
	dependency, err := plan.DependencyBlocks(domain, mustPresenceCoordinateFactorInventory(t, plan, domain, nil))
	if err != nil {
		t.Fatal(err)
	}
	stages := dependency.Stages()
	if len(stages) != 1 || len(stages[0].Blocks()) != 1 {
		t.Fatalf("cycle schedule = %d stages/%d blocks, want one sealed block", len(stages), len(stages[0].Blocks()))
	}
	block := stages[0].Blocks()[0]
	if !block.RequiresFeedback() {
		t.Fatal("three-row trigger cycle omitted its registered feed feedback")
	}
	present := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	input := state.Reachable(domain.Lattice().Bottom()).
		WriteValue(reg, statekey.SymbolValue(first), present).
		WriteValue(reg, statekey.SymbolValue(second), product.Top()).
		WriteValue(reg, statekey.SymbolValue(third), product.Top())
	for _, row := range rows {
		input = input.AddPathPresenceImplication(row)
	}
	family, ok := domain.PathEvidenceCoordinateFamily()
	if !ok {
		t.Fatal("registered product has no presence-implication family")
	}
	binding, err := SealPresenceImplicationRootBinding(dependency, func(dependency statekey.ValueDependency) (statekey.Value, bool) {
		return dependency.Concrete()
	}, func(root statekey.Value) bool { return root != 0 })
	if err != nil {
		t.Fatal(err)
	}
	blockAuthority, authorityOK := binding.BlockAuthority(block)
	if !authorityOK {
		t.Fatal("presence block authority")
	}
	openCarrier := func() *state.CoordinatePathEvidenceCarrier[statekey.Value] {
		factors, factorErr := domain.DecomposeLanes(input, []state.ProductLane{family.Lane()})
		if factorErr != nil || len(factors) != 1 {
			t.Fatalf("path family decomposition = %d/%v", len(factors), factorErr)
		}
		skeleton, scalars, decomposeErr := domain.DecomposeCoordinateFamily(factors[0], family, ks)
		if decomposeErr != nil {
			t.Fatal(decomposeErr)
		}
		_, values := state.DecomposeValueLane(domain.Lattice(), input)
		mutation := state.PathDescendantMutationFactors{}
		if block.PathMutation() {
			mutation, decomposeErr = domain.DecomposePathDescendantMutationFactors(input, ks)
			if decomposeErr != nil {
				t.Fatal(decomposeErr)
			}
		}
		carrier, openErr := domain.OpenCoordinatePathEvidenceCarrier(
			skeleton, scalars, values, true,
			blockAuthority, mutation,
		)
		if openErr != nil {
			t.Fatal(openErr)
		}
		return carrier
	}
	readPresence := func(carrier *state.CoordinatePathEvidenceCarrier[statekey.Value], sym symbol.ID) presence.Value {
		value, valid := carrier.ReadValue(statekey.SymbolValue(sym))
		if !valid {
			t.Fatalf("value %d is outside block authority", sym)
		}
		return product.PresenceOf(value)
	}
	initial := openCarrier()
	roundOne, feasible, changed, err := ApplyPresenceImplicationCoordinateRound(dependency, context.Background(), initial, block, binding)
	if err != nil || !feasible || !changed {
		t.Fatalf("round one = feasible:%t changed:%t err:%v", feasible, changed, err)
	}
	if !presence.Equal(readPresence(roundOne, second), presence.Present()) {
		t.Fatal("round one did not apply the direct first => second consequence")
	}
	if !presence.Equal(readPresence(roundOne, third), presence.Top()) {
		t.Fatal("one round self-closed the transitive second => third consequence")
	}
	if !presence.Equal(readPresence(initial, second), presence.Top()) {
		t.Fatal("one-round evaluation mutated its trigger snapshot")
	}

	roundTwo, feasible, changed, err := ApplyPresenceImplicationCoordinateRound(dependency, context.Background(), roundOne, block, binding)
	if err != nil || !feasible || !changed {
		t.Fatalf("round two = feasible:%t changed:%t err:%v", feasible, changed, err)
	}
	if !presence.Equal(readPresence(roundTwo, third), presence.Present()) {
		t.Fatal("external second round did not apply the transitive consequence")
	}
	roundThree, feasible, changed, err := ApplyPresenceImplicationCoordinateRound(dependency, context.Background(), roundTwo, block, binding)
	if err != nil || !feasible || changed {
		t.Fatalf("round three = feasible:%t changed:%t err:%v, want stable", feasible, changed, err)
	}

	legacyClosure := openCarrier()
	legacyFeasible, err := ApplyPresenceImplicationCoordinateBlock(dependency, context.Background(), legacyClosure, block, binding)
	if err != nil || !legacyFeasible {
		t.Fatalf("legacy closure = feasible:%t err:%v", legacyFeasible, err)
	}
	for _, sym := range []symbol.ID{first, second, third} {
		got, gotOK := roundThree.ReadValue(statekey.SymbolValue(sym))
		want, wantOK := legacyClosure.ReadValue(statekey.SymbolValue(sym))
		if !gotOK || !wantOK || !product.Equal(reg, got, want) {
			t.Fatalf("externally scheduled rounds disagree with legacy closure at symbol %d", sym)
		}
	}
}

func TestPresenceDependencyBlocksExposeExactCondensationPredecessors(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(904)
	first, second, third := symbol.ID(931), symbol.ID(932), symbol.ID(933)
	builder := visibility.NewBuilder()
	for sym, name := range map[symbol.ID]string{first: "first", second: "second", third: "third"} {
		builder.Define(point, sym, name)
	}
	resolver := visibility.NewResolver(builder.Build())
	ks := resolver.KeySpace()
	keyOf := func(sym symbol.ID, name string) keyspace.Key { return ks.FromPath(path.NewPath(sym, name)) }
	rows := []pathevidence.PathPresenceImplication{
		pathevidence.NewPathPresenceImplication(keyOf(first, "first"), presence.Present(), keyOf(second, "second"), presence.Present()),
		pathevidence.NewPathPresenceImplication(keyOf(second, "second"), presence.Present(), keyOf(third, "third"), presence.Present()),
	}
	plan := preparePresenceDependencyPlanTest(t, reg, resolver, point, rows, ConcretePresenceImplicationTrailingBarrier)
	domain := state.RegisteredProductDomain(reg)
	dependency, err := plan.DependencyBlocks(domain, mustPresenceCoordinateFactorInventory(t, plan, domain, nil))
	if err != nil {
		t.Fatal(err)
	}
	stages := dependency.Stages()
	if len(stages) != 1 {
		t.Fatalf("stages = %d, want 1", len(stages))
	}
	blocks := stages[0].Blocks()
	if len(blocks) != 2 {
		t.Fatalf("chain blocks = %d, want 2 exact SCCs", len(blocks))
	}
	if got := blocks[0].Predecessors(); len(got) != 0 {
		t.Fatalf("first block predecessors = %v, want none", got)
	}
	if got := blocks[1].Predecessors(); len(got) != 1 || got[0] != 0 {
		t.Fatalf("second block predecessors = %v, want [0]", got)
	}
	for index, block := range blocks {
		if block.RequiresFeedback() {
			t.Fatalf("block %d treated target accumulation or WAW as cyclic feedback", index)
		}
	}
}
