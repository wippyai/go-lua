package factapply

import (
	"context"
	"errors"
	"testing"

	statekey "github.com/wippyai/go-lua/__legacy/analysis/domain/state/key"
	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/formal"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/cancellation"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestValueRefinementFactorProgramMatchesConcreteRootInvalidation(t *testing.T) {
	reg := standard.Registry()
	domain := state.RegisteredProductDomain(reg)
	point := cfg.Point(1)
	sym := symbol.ID(901)
	builder := visibility.NewBuilder()
	builder.Define(point, sym, "value-refinement-factor")
	resolver := visibility.NewResolver(builder.Build())
	rootPath := pathdom.Path{Symbol: sym}
	childPath := rootPath.Field("child")
	rootKey, rootOK := factKeyspaceKeyAt(resolver, point, rootPath)
	childKey, childOK := factKeyspaceKeyAt(resolver, point, childPath)
	if !rootOK || !childOK {
		t.Fatal("fixture paths are unresolved")
	}
	input := state.Reachable(state.State{}).
		WriteValue(reg, statekey.SymbolValue(sym), product.Top()).
		WriteLocalPathKey(reg, childKey, presentValue(reg)).
		WriteLocalPathStaticMember(childKey, presentValue(reg))
	refinement := factflow.NewValueConstraint(absentValue(reg))
	program, frame := prepareConcreteValueRefinementFactorTest(t, domain, resolver, rootKey, refinement, input)
	gotFrame, err := program.Apply(context.Background(), nil, frame)
	if err != nil {
		t.Fatal(err)
	}
	got := composeConcreteValueRefinementFactorTest(t, domain, input, program, gotFrame)
	if actual := product.PresenceOf(got.ReadValue(reg, statekey.SymbolValue(sym))); !presence.Equal(actual, presence.Absent()) {
		t.Fatalf("root presence = %v, want absent", actual)
	}
	if child := got.ReadLocalPathKey(reg, childKey); !product.Equal(reg, child, product.Bottom(reg)) {
		t.Fatalf("invalidated descendant = %s, want bottom", formatValue(reg, child))
	}
	if _, present := got.ReadLocalPathStaticMember(childKey); present {
		t.Fatal("root invalidation retained descendant static-member evidence")
	}
}

func TestValueRefinementFactorProgramCancellationIsAtomic(t *testing.T) {
	reg := standard.Registry()
	domain := state.RegisteredProductDomain(reg)
	point := cfg.Point(1)
	sym := symbol.ID(902)
	builder := visibility.NewBuilder()
	builder.Define(point, sym, "value-refinement-cancel")
	resolver := visibility.NewResolver(builder.Build())
	rootPath := pathdom.Path{Symbol: sym}
	rootKey, ok := factKeyspaceKeyAt(resolver, point, rootPath)
	if !ok {
		t.Fatal("fixture root is unresolved")
	}
	input := state.Reachable(state.State{}).WriteValue(reg, statekey.SymbolValue(sym), product.Top())
	program, frame := prepareConcreteValueRefinementFactorTest(
		t, domain, resolver, rootKey, factflow.NewValueConstraint(absentValue(reg)), input,
	)
	_, session := cancellation.Attach(context.Background())
	session.Token().Cancel(context.Canceled)
	got, err := program.Apply(context.Background(), session.Token(), frame)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled refinement error=%v", err)
	}
	if got.Carrier != frame.Carrier || &got.Factors[0] != &frame.Factors[0] {
		t.Fatal("canceled refinement published a detached partial frame")
	}
}

func TestGuardRootValueRefinementRejectsWithoutSkeletonWrite(t *testing.T) {
	reg := standard.Registry()
	domain := state.RegisteredProductDomain(reg)
	point := cfg.Point(1)
	sym := symbol.ID(904)
	builder := visibility.NewBuilder()
	builder.Define(point, sym, "guard-root-skeleton-capability")
	resolver := visibility.NewResolver(builder.Build())
	root, ok := factKeyspaceKeyAt(resolver, point, pathdom.Path{Symbol: sym})
	if !ok {
		t.Fatal("fixture root is unresolved")
	}
	inventory, err := domain.SealCoordinateFactorInventory(resolver.KeySpace(), nil)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := domain.SealValueRefinementPlan(resolver.KeySpace(), root, inventory)
	if err != nil {
		t.Fatal(err)
	}
	program, err := PrepareGuardValueRefinementFactorProgram(
		domain, plan, factflow.NewValueConstraint(presentValue(reg)),
		func(dependency statekey.ValueDependency) (statekey.Value, bool) { return dependency.Concrete() },
		nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if program.WritesCoordinateSkeleton() {
		t.Fatal("guard rejection manufactured a component-skeleton write below product Bottom")
	}
}

func TestValueRefinementFactorProgramMatchesConcreteDescendantDynamicRead(t *testing.T) {
	reg := standard.Registry()
	domain := state.RegisteredProductDomain(reg)
	point := cfg.Point(1)
	sym := symbol.ID(903)
	builder := visibility.NewBuilder()
	builder.Define(point, sym, "value-refinement-descendant")
	resolver := visibility.NewResolver(builder.Build())
	rootPath := pathdom.Path{Symbol: sym}
	targetPath := rootPath.Field("child")
	targetKey, ok := factKeyspaceKeyAt(resolver, point, targetPath)
	if !ok {
		t.Fatal("fixture target is unresolved")
	}
	unrelatedLenKey := resolver.KeySpace().FromPath(pathdom.Path{Root: "value-refinement-unrelated-length"})
	unrelatedLenState, ok := pathaddr.StateKeyFromPathKey(resolver.KeySpace().FormatReadOnly(unrelatedLenKey))
	if !ok {
		t.Fatal("unrelated length coordinate")
	}
	input := state.Reachable(state.State{}).
		WriteValue(reg, statekey.SymbolValue(sym), product.Top()).
		WriteLocalPathKey(reg, targetKey, product.Top()).
		WriteLenFloor(resolver.KeySpace(), unrelatedLenState, 7)
	refinement := factflow.NewValueConstraint(presentValue(reg))
	program, frame := prepareConcreteValueRefinementFactorTest(t, domain, resolver, targetKey, refinement, input)
	lenSlot, err := domain.LenFloorCoordinateSlot(resolver.KeySpace(), unrelatedLenKey)
	if err != nil {
		t.Fatal(err)
	}
	if !coordinateSlotsContain(t, domain, program.plan.FactorCoordinateReads(), lenSlot) {
		t.Fatal("value-refinement factor footprint omitted non-default LenFloor evidence")
	}
	gotFrame, err := program.Apply(context.Background(), nil, frame)
	if err != nil {
		t.Fatal(err)
	}
	got := composeConcreteValueRefinementFactorTest(t, domain, input, program, gotFrame)
	if actual := product.PresenceOf(got.ReadLocalPathKey(reg, targetKey)); !presence.Equal(actual, presence.Present()) {
		t.Fatalf("descendant presence = %v, want present", actual)
	}
	if floor, present := got.ReadLenFloor(resolver.KeySpace(), unrelatedLenState); !present || floor != 7 {
		t.Fatalf("unrelated length floor = %d/%t, want 7/true", floor, present)
	}
}

func TestValueRefinementHeapOwnershipDoesNotInventAbsentRuntimeObject(t *testing.T) {
	reg := standard.Registry()
	domain := state.RegisteredProductDomain(reg)
	point := cfg.Point(1)
	sym := symbol.ID(905)
	builder := visibility.NewBuilder()
	builder.Define(point, sym, "value-refinement-sparse-heap")
	resolver := visibility.NewResolver(builder.Build())
	root, ok := factKeyspaceKeyAt(resolver, point, pathdom.Path{Symbol: sym})
	if !ok {
		t.Fatal("fixture root is unresolved")
	}
	id := identity.ID{Kind: "value-refinement", Site: t.Name(), Index: 1}
	idValue := identityvalue.Present(reg, id)
	declared := state.Reachable(state.State{}).WriteHeapTableObject(
		reg, id, heapidentity.NewTableObject(heapidentity.TableObjectConfig{Root: idValue}),
	)
	inventory, err := domain.CoordinateFactorInventoryFromPreparedState(resolver.KeySpace(), declared)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := domain.SealValueRefinementPlan(resolver.KeySpace(), root, inventory)
	if err != nil {
		t.Fatal(err)
	}
	program, err := PrepareValueRefinementFactorProgram(
		domain, plan, factflow.NewValueConstraint(presentValue(reg)),
		func(dependency statekey.ValueDependency) (statekey.Value, bool) { return dependency.Concrete() },
		nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	factors, err := domain.DecomposeLanes(domain.Lattice().Bottom(), program.Lanes())
	if err != nil {
		t.Fatal(err)
	}
	frame := ValueRefinementFactorFrame[statekey.Value]{Factors: factors}
	if err := program.rewriteHeapRoot(&frame, idValue, idValue); err != nil {
		t.Fatalf("absent runtime heap root rejected: %v", err)
	}
	heapIndex := program.factorIndex(state.LaneHeapTableIdentity)
	if heapIndex < 0 {
		t.Fatal("value-refinement heap factor is absent")
	}
	_, roots, _, err := domain.DecomposeHeapTableIdentity(frame.Factors[heapIndex], resolver.KeySpace())
	if err != nil {
		t.Fatal(err)
	}
	if len(roots) != 0 {
		t.Fatalf("value refinement invented %d runtime heap roots", len(roots))
	}
}

func coordinateSlotsContain(t *testing.T, domain state.ProductDomain, slots []state.CoordinateSlot, want state.CoordinateSlot) bool {
	t.Helper()
	for _, slot := range slots {
		equal, err := domain.CoordinateSlotEqual(slot, want)
		if err != nil {
			t.Fatal(err)
		}
		if equal {
			return true
		}
	}
	return false
}

func TestValueRefinementFactorProgramAppliesFormalRootDescendant(t *testing.T) {
	reg := standard.Registry()
	domain := state.RegisteredProductDomain(reg)
	keys := keyspace.New()
	owner := lexicalidentity.RootBody(lexicalidentity.UnitNamespaceFromContent([]byte(t.Name())))
	formalRoot := formal.NewRoot(owner, 1, formal.Input)
	root, ok := keys.InternFormalRoot(formalRoot)
	if !ok {
		t.Fatal("formal refinement root")
	}
	target, ok := keys.AppendSegment(root, segment.Segment{Kind: segment.SegmentField, Name: "child"})
	if !ok {
		t.Fatal("formal refinement descendant")
	}
	inventory, err := domain.SealCoordinateFactorInventory(keys, nil)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := domain.SealValueRefinementPlan(keys, target, inventory)
	if err != nil {
		t.Fatal(err)
	}
	program, err := PrepareValueRefinementFactorProgram(
		domain,
		plan,
		factflow.NewValueConstraint(presentValue(reg)),
		func(dependency statekey.ValueDependency) (int, bool) {
			got, exact := dependency.Formal()
			return 1, exact && got == formalRoot
		},
		nil,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	family, present := domain.PathEvidenceCoordinateFamily()
	if !present {
		t.Fatal("path evidence coordinate family")
	}
	pathFactors, err := domain.DecomposeLanes(domain.Lattice().Bottom(), []state.ProductLane{family.Lane()})
	if err != nil {
		t.Fatal(err)
	}
	skeleton, scalars, err := domain.DecomposeCoordinateFamily(pathFactors[0], family, keys)
	if err != nil {
		t.Fatal(err)
	}
	factors, err := domain.DecomposeLanes(domain.Lattice().Bottom(), program.Lanes())
	if err != nil {
		t.Fatal(err)
	}
	mutation, err := domain.DecomposePathDescendantMutationFactors(domain.Lattice().Bottom(), keys)
	if err != nil {
		t.Fatal(err)
	}
	carrier, err := state.OpenCoordinatePathEvidenceCarrier(
		domain, skeleton, scalars, state.ValueFactor[int]{Values: map[int]product.Value{}}, true,
		program.PathEvidenceAuthority(), mutation,
	)
	if err != nil {
		t.Fatal(err)
	}
	got, err := program.Apply(context.Background(), nil, ValueRefinementFactorFrame[int]{
		Values:  state.ValueFactor[int]{Values: map[int]product.Value{1: product.Top()}},
		Factors: factors, Carrier: carrier, Reachable: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	refined, readable := got.Carrier.ReadPath(target)
	if !readable {
		t.Fatal("formal descendant refinement was not published")
	}
	if actual := product.PresenceOf(refined); !presence.Equal(actual, presence.Present()) {
		t.Fatalf("formal descendant presence = %v, want present", actual)
	}
}

func prepareConcreteValueRefinementFactorTest(
	t *testing.T,
	domain state.ProductDomain,
	resolver *visibility.Resolver,
	target keyspace.Key,
	refinement factflow.ValueRefinement,
	input state.State,
) (ValueRefinementFactorProgram[statekey.Value], ValueRefinementFactorFrame[statekey.Value]) {
	t.Helper()
	residual, values := state.DecomposeValueLane(domain.Lattice(), input)
	family, present := domain.PathEvidenceCoordinateFamily()
	if !present {
		t.Fatal("path evidence coordinate family is absent")
	}
	pathFactors, err := domain.DecomposeLanes(residual, []state.ProductLane{family.Lane()})
	if err != nil {
		t.Fatal(err)
	}
	skeleton, scalars, err := domain.DecomposeCoordinateFamily(pathFactors[0], family, resolver.KeySpace())
	if err != nil {
		t.Fatal(err)
	}
	fullInventory, err := domain.CoordinateFactorInventoryFromPreparedState(resolver.KeySpace(), input)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := domain.SealValueRefinementPlan(resolver.KeySpace(), target, fullInventory)
	if err != nil {
		t.Fatal(err)
	}
	program, err := PrepareValueRefinementFactorProgram(
		domain, plan, refinement,
		func(dependency statekey.ValueDependency) (statekey.Value, bool) { return dependency.Concrete() },
		nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	factors, err := domain.DecomposeLanes(residual, program.Lanes())
	if err != nil {
		t.Fatal(err)
	}
	mutation, err := domain.DecomposePathDescendantMutationFactors(residual, resolver.KeySpace())
	if err != nil {
		t.Fatal(err)
	}
	carrier, err := domain.OpenCoordinatePathEvidenceCarrier(
		skeleton, scalars, state.ValueLaneFactor{}, !domain.Lattice().Equal(input, domain.Lattice().Bottom()),
		program.PathEvidenceAuthority(), mutation,
	)
	if err != nil {
		t.Fatal(err)
	}
	return program, ValueRefinementFactorFrame[statekey.Value]{
		Values: values, Factors: factors, Carrier: carrier,
		Reachable: !domain.Lattice().Equal(input, domain.Lattice().Bottom()),
	}
}

func composeConcreteValueRefinementFactorTest(
	t *testing.T,
	domain state.ProductDomain,
	input state.State,
	program ValueRefinementFactorProgram[statekey.Value],
	frame ValueRefinementFactorFrame[statekey.Value],
) state.State {
	return composeFactorFrameTest(t, domain, input, program.Lanes(), frame)
}

func composeFactorFrameTest(
	t *testing.T,
	domain state.ProductDomain,
	input state.State,
	lanes []state.ProductLane,
	frame ValueRefinementFactorFrame[statekey.Value],
) state.State {
	t.Helper()
	if !frame.Reachable {
		return domain.Lattice().Bottom()
	}
	residual, _ := state.DecomposeValueLane(domain.Lattice(), input)
	delta, err := domain.ComposeSparse(frame.Factors)
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]state.LaneID, len(lanes))
	for index, lane := range lanes {
		ids[index] = lane.ID()
	}
	residual, err = domain.PatchFactors(residual, delta, state.NewLaneSet(ids...))
	if err != nil {
		t.Fatal(err)
	}
	if frame.Carrier == nil {
		return state.RecomposeValueLane(domain.Registry(), domain.Lattice(), residual, frame.Values)
	}
	skeleton, scalars, _, mutationLanes, mutationCoordinates, _, err := frame.Carrier.Freeze()
	if err != nil {
		t.Fatal(err)
	}
	pathFactors, err := domain.DecomposeLanes(residual, []state.ProductLane{skeleton.Family().Lane()})
	if err != nil || len(pathFactors) != 1 {
		t.Fatal("path factor missing")
	}
	pathFactors[0], err = domain.ReplaceCoordinateFamily(pathFactors[0], skeleton, scalars)
	if err != nil {
		t.Fatal(err)
	}
	updates := append([]state.LaneFactor{pathFactors[0]}, mutationLanes...)
	for _, coordinate := range mutationCoordinates {
		base, baseErr := domain.DecomposeLanes(residual, []state.ProductLane{coordinate.Family().Lane()})
		if baseErr != nil || len(base) != 1 {
			t.Fatal("coordinate factor missing")
		}
		base[0], err = domain.ReplaceCoordinateFamily(base[0], coordinate.Skeleton(), coordinate.Scalars())
		if err != nil {
			t.Fatal(err)
		}
		updates = append(updates, base[0])
	}
	delta, err = domain.ComposeSparse(updates)
	if err != nil {
		t.Fatal(err)
	}
	updateIDs := make([]state.LaneID, len(updates))
	for index, factor := range updates {
		updateIDs[index] = factor.Lane().ID()
	}
	residual, err = domain.PatchFactors(residual, delta, state.NewLaneSet(updateIDs...))
	if err != nil {
		t.Fatal(err)
	}
	return state.RecomposeValueLane(domain.Registry(), domain.Lattice(), residual, frame.Values)
}
