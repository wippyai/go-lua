package factapply

import (
	"context"
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/cancellation"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/engine/state/userlattice"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestConcretePresenceImplicationClosesChainsAndCycles(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(811)
	const (
		first  = symbol.ID(811)
		second = symbol.ID(812)
		third  = symbol.ID(813)
	)
	resolver := presenceTransactionResolver(point, first, second, third)
	ks := resolver.KeySpace()
	firstKey := ks.FromPath(pathdom.NewPath(first, "first"))
	secondKey := ks.FromPath(pathdom.NewPath(second, "second"))
	thirdKey := ks.FromPath(pathdom.NewPath(third, "third"))
	present := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	maybe := product.NewWithPresence(reg, product.ShapeTop, presence.Maybe())

	// Publish the dependent edge first so reaching the result requires another
	// closure round rather than benefiting from slice order.
	publications := []pathevidence.PathPresenceImplication{
		pathevidence.NewPathPresenceImplication(secondKey, presence.Present(), thirdKey, presence.Present()),
		pathevidence.NewPathPresenceImplication(firstKey, presence.Present(), secondKey, presence.Present()),
		pathevidence.NewPathPresenceImplication(thirdKey, presence.Present(), secondKey, presence.Present()),
	}
	in := state.State{}.
		WriteValue(reg, key.SymbolValue(first), present).
		WriteValue(reg, key.SymbolValue(second), maybe).
		WriteValue(reg, key.SymbolValue(third), maybe)
	result := ApplyConcretePresenceImplications(ConcretePresenceImplicationRequest{
		Registry: reg, Resolver: resolver, Point: point, Input: in, Output: in, Publications: publications,
	})
	if result.Canceled {
		t.Fatal("closure unexpectedly canceled")
	}
	for _, sym := range []symbol.ID{second, third} {
		got := product.PresenceOf(result.Output.ReadValue(reg, key.SymbolValue(sym)))
		if !presence.Equal(got, presence.Present()) {
			t.Fatalf("symbol %d presence = %s, want present", sym, got)
		}
	}
	stable := ApplyConcretePresenceImplications(ConcretePresenceImplicationRequest{
		Registry: reg, Resolver: resolver, Point: point, Output: result.Output,
	})
	if stable.Canceled || !state.Domain(reg).Equal(stable.Output, result.Output) {
		t.Fatal("cyclic implication closure is not idempotent")
	}
}

func TestConcretePresenceImplicationClosesBeforeDescendantInvalidation(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(8111)
	const (
		trigger = symbol.ID(814)
		root    = symbol.ID(815)
		witness = symbol.ID(816)
	)
	resolver := presenceTransactionResolver(point, trigger, root, witness)
	ks := resolver.KeySpace()
	triggerKey := ks.FromPath(pathdom.NewPath(trigger, "trigger"))
	rootPath := pathdom.NewPath(root, "root")
	rootKey := ks.FromPath(rootPath)
	childKey, childOK := visibility.AddressAt(resolver, point, rootPath.Field("child")).RootOrVisibleKeyspaceKey()
	if !childOK {
		t.Fatal("child has no visible key")
	}
	witnessKey := ks.FromPath(pathdom.NewPath(witness, "witness"))
	present := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	maybe := product.NewWithPresence(reg, product.ShapeTop, presence.Maybe())
	in := state.State{}.
		WriteValue(reg, key.SymbolValue(trigger), present).
		WriteValue(reg, key.SymbolValue(root), maybe).
		WriteValue(reg, key.SymbolValue(witness), maybe).
		WriteLocalPathKey(reg, childKey, maybe).
		AddPathPresenceImplication(pathevidence.NewPathPresenceImplication(childKey, presence.Present(), witnessKey, presence.Present()))

	result := ApplyConcretePresenceImplications(ConcretePresenceImplicationRequest{
		Registry: reg, Resolver: resolver, Point: point, Input: in, Output: in,
		Barriers: ConcretePresenceImplicationDescendantInvalidationBarriers,
		Publications: []pathevidence.PathPresenceImplication{
			// This consequence must close through the pre-existing child ->
			// witness implication before the following publication removes the
			// root's descendants.
			pathevidence.NewPathPresenceImplication(triggerKey, presence.Present(), childKey, presence.Present()),
			pathevidence.NewPathPresenceImplication(triggerKey, presence.Present(), rootKey, presence.Absent()),
		},
	})
	if result.Canceled {
		t.Fatal("closure unexpectedly canceled")
	}
	if got := product.PresenceOf(result.Output.ReadValue(reg, key.SymbolValue(witness))); !presence.Equal(got, presence.Present()) {
		t.Fatalf("witness presence = %s, want present from pre-invalidation barrier", got)
	}
	if got := product.PresenceOf(result.Output.ReadValue(reg, key.SymbolValue(root))); !presence.Equal(got, presence.Absent()) {
		t.Fatalf("root presence = %s, want absent", got)
	}
}

func TestConcretePresenceImplicationCancellationUsesNodeAndEdgeRollback(t *testing.T) {
	reg := standard.Registry()
	point := cfg.Point(812)
	resolver := presenceTransactionResolver(point, 821, 822)
	input := state.State{}.WriteValue(reg, key.SymbolValue(900), presentValue(reg))
	evolving := input.WriteValue(reg, key.SymbolValue(901), presentValue(reg))
	ctx, cancel := context.WithCancel(context.Background())
	_, session := cancellation.Attach(ctx)
	cancel()

	node := ApplyConcretePresenceImplications(ConcretePresenceImplicationRequest{
		Registry: reg, Resolver: resolver, Point: point, Input: input, Output: evolving,
		Token: session.Token(), Cancellation: ConcretePresenceImplicationRollbackNode,
	})
	if !node.Canceled || !state.Domain(reg).Equal(node.Output, input) {
		t.Fatal("canceled node barrier did not roll back to immutable Input")
	}
	edge := ApplyConcretePresenceImplications(ConcretePresenceImplicationRequest{
		Registry: reg, Resolver: resolver, Point: point, Input: input, Output: evolving,
		Token: session.Token(), Cancellation: ConcretePresenceImplicationKeepEvolving,
	})
	if !edge.Canceled || !state.Domain(reg).Equal(edge.Output, evolving) {
		t.Fatal("canceled edge barrier did not retain its evolving Output")
	}
}

func TestConcretePresenceImplicationPreservesAllStateLanes(t *testing.T) {
	const userAxis userlattice.AxisID = "test.presence-transaction"
	reg := concreteRootTransactionRegistry(t, userAxis)
	point := cfg.Point(813)
	const (
		trigger = symbol.ID(831)
		target  = symbol.ID(832)
	)
	resolver := presenceTransactionResolver(point, trigger, target)
	ks := resolver.KeySpace()
	seeds := concreteRootTransactionLaneSeeds(t, reg, ks, userAxis)
	full := state.Reachable(state.State{})
	for _, lane := range state.DefaultLanes() {
		full = state.Domain(reg).Join(full, seeds[lane])
	}
	if got := len(state.DefaultLanes()); got != 17 {
		t.Fatalf("default state lane count = %d, want 17", got)
	}
	present := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	maybe := product.NewWithPresence(reg, product.ShapeTop, presence.Maybe())
	triggerKey := ks.FromPath(pathdom.NewPath(trigger, "trigger"))
	targetKey := ks.FromPath(pathdom.NewPath(target, "target"))
	before := full.
		WriteValue(reg, key.SymbolValue(trigger), present).
		WriteValue(reg, key.SymbolValue(target), maybe).
		AddPathPresenceImplication(pathevidence.NewPathPresenceImplication(triggerKey, presence.Present(), targetKey, presence.Present()))
	result := ApplyConcretePresenceImplications(ConcretePresenceImplicationRequest{
		Registry: reg, Resolver: resolver, Point: point, Output: before,
	})
	if result.Canceled {
		t.Fatal("closure unexpectedly canceled")
	}
	if got := product.PresenceOf(result.Output.ReadValue(reg, key.SymbolValue(target))); !presence.Equal(got, presence.Present()) {
		t.Fatalf("target presence = %s, want present", got)
	}
	for _, lane := range state.DefaultLanes() {
		if lane == state.LaneValues {
			continue
		}
		domain := state.DomainWithLanes(reg, []state.LaneID{lane})
		if !domain.Equal(state.NormalizeForDomain(domain, before), state.NormalizeForDomain(domain, result.Output)) {
			t.Fatalf("presence implication closure changed unrelated lane %q", lane)
		}
	}
}

func presenceTransactionResolver(point cfg.Point, symbols ...symbol.ID) *visibility.Resolver {
	builder := visibility.NewBuilder()
	for _, sym := range symbols {
		builder.Define(point, sym, "sym")
	}
	return visibility.NewResolver(builder.Build())
}
