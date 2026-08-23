package engine

import (
	"context"
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"
	"github.com/wippyai/go-lua/analysis/engine/internal/contextfiber"
	"github.com/wippyai/go-lua/analysis/engine/internal/demand"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/linkexecutionplan"
	"github.com/wippyai/go-lua/analysis/engine/internal/schedule"
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/executioncontext"
)

// artifactReadIndexLawFixture keeps a real sealed Rule read and extends its
// authenticated compact plane with a second Context. The production runtime
// deliberately retains one producer row per graph Group, so this fixture can
// exercise the sparse inverse's two independent contextual consumers without
// manufacturing a graph-point fallback.
type artifactReadIndexLawFixture struct {
	lane         readLaneFixture
	source       *executorEpoch
	epoch        *executorEpoch
	runtime      *solverRuntime
	group        int
	inputPoint   int
	outputPoint  int
	outputStates [2]int
	inputStates  [2]int
	primary      carrier.Unit
	alternate    carrier.Unit
}

func artifactReadIndexLawID(t testing.TB, label string) identity.ContentID {
	t.Helper()
	id, ok := identity.DeriveContentID("analysis/engine/artifact-read-index-law/"+label, nil)
	if !ok {
		t.Fatalf("derive artifact-read-index law id %q", label)
	}
	return id
}

func artifactReadIndexLawDirectory(t testing.TB, runtime *solverRuntime, module identity.ContentID) executioncontext.Directory {
	t.Helper()
	if runtime == nil || !runtime.contexts.Available() || !module.Available() {
		t.Fatal("artifact-read-index context inputs")
	}
	link := runtime.contexts.LinkID()
	contexts := make([]executioncontext.Context, 0, 2)
	roots := make([]executioncontext.RootContext, 0, 2)
	for _, label := range []string{"left", "right"} {
		row, rowOK := executioncontext.NewContext(link, module, artifactReadIndexLawID(t, label+"-actor"), artifactReadIndexLawID(t, label+"-representative"))
		root, rootOK := executioncontext.NewRootContext(link, artifactReadIndexLawID(t, label+"-root"), row.ID())
		if !rowOK || !rootOK {
			t.Fatal("artifact-read-index context row")
		}
		contexts = append(contexts, row)
		roots = append(roots, root)
	}
	directory, sealed := executioncontext.Seal(link, contexts, roots, nil)
	if !sealed || !directory.Available() || directory.ContextCount() != 2 {
		t.Fatal("artifact-read-index context directory")
	}
	return directory
}

func artifactReadIndexLawUnits(t testing.TB, runtime *solverRuntime, primary carrier.Unit) carrier.Unit {
	t.Helper()
	primarySlot, primaryOK := primary.Slot()
	if !primaryOK || primary.Kind() != carrier.ExactUnit {
		t.Fatal("artifact-read-index primary unit")
	}
	if runtime != nil && runtime.program != nil {
		for _, row := range runtime.program.queryTable {
			if row.unit == (carrier.Unit{}) || row.unit.Kind() != carrier.ExactUnit {
				continue
			}
			slot, slotOK := row.unit.Slot()
			if slotOK && slot != primarySlot && runtime.carrier.OwnsUnit(slot, row.unit) {
				return row.unit
			}
		}
	}
	t.Fatal("artifact-read-index alternate factor slot unit")
	return carrier.Unit{}
}

func newArtifactReadIndexLawFixture(t testing.TB) artifactReadIndexLawFixture {
	t.Helper()
	lane := newReadLaneFixture(t)
	solver, failure, sealed := lane.program.Seal(nil)
	if !sealed || solver == nil {
		t.Fatalf("artifact-read-index solver seal=%t failure=%v", sealed, failure)
	}
	source, opened := newRuntimeEpoch(solver.runtime, solver.relation, context.Background())
	if !opened || source == nil {
		t.Fatal("artifact-read-index source epoch")
	}
	t.Cleanup(func() { source.discard() })

	runtime := solver.runtime
	group := -1
	primary := carrier.Unit{}
	for index, producer := range runtime.producers {
		if len(producer.reads) == 0 || producer.group.InputCount() == 0 {
			continue
		}
		group = index
		primary = producer.reads[0].Unit
		break
	}
	if group < 0 || primary == (carrier.Unit{}) || primary.Kind() != carrier.ExactUnit {
		t.Fatal("artifact-read-index producer exact read")
	}
	input, inputOK := runtime.producers[group].group.InputAt(0)
	output := runtime.producers[group].group.Output()
	inputPoint, inputPointOK := runtime.graph.PointIndex(input.Point())
	outputPoint, outputPointOK := runtime.graph.PointIndex(output)
	if !inputOK || !inputPointOK || !outputPointOK || inputPoint < 0 || outputPoint < 0 {
		t.Fatal("artifact-read-index producer point addresses")
	}
	module := runtime.contexts.LinkID()
	for index := 0; index < runtime.contexts.ContextCount(); index++ {
		row, rowOK := runtime.contexts.ContextAt(index)
		if rowOK && row.Available() && row.ModuleKey().Available() {
			module = row.ModuleKey()
			break
		}
	}
	directory := artifactReadIndexLawDirectory(t, runtime, module)
	owners := append([]contextfiber.PointOwner(nil), runtime.pointOwners...)
	index, indexOK := contextfiber.New(directory, runtime.graph.PointCount(), runtime.contextLayout.Generation())
	layout, layoutOK := contextfiber.NewLayoutForGraph(index, directory, owners, runtime.contextLayout.Generation(), runtime.graph)
	plan, planOK := linkexecutionplan.New(runtime.graph, layout, directory, nil)
	if !indexOK || !layoutOK || !planOK || plan == nil || plan.StateCount() == 0 {
		t.Fatal("artifact-read-index contextual execution plan")
	}
	activePoints := make([]bool, runtime.graph.PointCount())
	for point := range activePoints {
		activePoints[point] = true
	}
	producerRows, activeStates, rowsOK := buildStateGroupIndex(runtime.graph, plan, true, activePoints)
	statePointRows, statePointRowsOK := buildStatePointRows(runtime.graph, plan, true)
	if !rowsOK || !statePointRowsOK || len(activeStates) != int(plan.StateCount()) {
		t.Fatal("artifact-read-index contextual producer rows")
	}
	outputStates := [2]int{}
	inputStates := [2]int{}
	if directory.ContextCount() != 2 {
		t.Fatal("artifact-read-index contextual width")
	}
	for contextIndex := 0; contextIndex < 2; contextIndex++ {
		contextRow, contextOK := directory.ContextAt(contextIndex)
		contextOrdinal, ordinalOK := index.ContextOrdinal(contextRow.ID())
		outputState, outputStateOK := plan.Lookup(contextOrdinal, contextfiber.PointOrdinal(outputPoint))
		inputState, inputStateOK := plan.Lookup(contextOrdinal, contextfiber.PointOrdinal(inputPoint))
		if !contextOK || !contextRow.Available() || !ordinalOK || !outputStateOK || !inputStateOK {
			t.Fatal("artifact-read-index contextual state address")
		}
		outputStates[contextIndex], inputStates[contextIndex] = int(outputState), int(inputState)
	}

	// The expanded runtime shares only the sealed Graph and carrier layout; all
	// contextual indexes and active rows are owned by this law fixture.
	expanded := *runtime
	expanded.contexts = directory
	expanded.contextIndex = index
	expanded.contextLayout = layout
	expanded.executionPlan = plan
	expanded.producerRows = producerRows
	expanded.activeStates = activeStates
	expanded.activePoints = activePoints
	expanded.statePointRows = statePointRows
	expanded.pointRegion = make([]int, int(plan.StateCount()))
	for state := range expanded.pointRegion {
		expanded.pointRegion[state] = schedule.NoRegion
	}
	expanded.regions = nil
	expanded.activeRegions = nil
	operands, operandsOK := buildStateOperandPlane(&expanded, stateFactorSources(expanded.stateFactorRows), nil)
	if !operandsOK || operands == nil {
		t.Fatal("artifact-read-index operand plane")
	}
	expanded.operands = operands
	work, workOK := expanded.carrier.NewWork()
	if !workOK {
		t.Fatal("artifact-read-index carrier work")
	}
	epoch := &executorEpoch{
		runtime:        &expanded,
		ctx:            context.Background(),
		work:           work,
		points:         make([]carrier.PointState, int(plan.StateCount())),
		producers:      make([]producerEpoch, len(producerRows.rows)),
		queue:          newPointQueue(int(plan.StateCount())),
		postfixDirty:   make([]bool, int(plan.StateCount())),
		postfixPending: make([]int, 0),
		currentState:   -1,
	}
	empty, emptyOK := support.FromGuard(expanded.carrier.Guards(), expanded.carrier.Guards().False())
	if !emptyOK {
		work.Close()
		t.Fatal("artifact-read-index empty support")
	}
	for stateIndex := range epoch.points {
		_, pointIndex, _, pointOK := expanded.graphPointAtState(stateIndex)
		if !pointOK || pointIndex < 0 || pointIndex >= len(expanded.pointScopes) {
			work.Close()
			t.Fatal("artifact-read-index point state address")
		}
		feasible := empty
		point, pointOK := expanded.graph.PointAt(schedule.Node(pointIndex))
		if !pointOK {
			work.Close()
			t.Fatal("artifact-read-index point")
		}
		if point.HasInit() {
			feasible = expanded.pointInitials[pointIndex]
		}
		state, stateOK := carrier.NewState(expanded.carrier, expanded.pointScopes[pointIndex], feasible)
		pointState, pointStateOK := work.EmptyPointState(state)
		if !stateOK || !pointStateOK {
			work.Close()
			t.Fatal("artifact-read-index point state")
		}
		epoch.points[stateIndex] = pointState
	}
	epoch.terminal.Store(epochRunning)
	if !epoch.operands.open(expanded.operands) {
		work.Close()
		t.Fatal("artifact-read-index operand epoch")
	}
	epoch.artifactProducerReads.byKey = make(map[artifactProducerReadKey][]stateGroupKey)
	epoch.artifactProducerReads.byFactor = make(map[artifactProducerReadFactorKey][]stateGroupKey)
	for rowIndex, row := range producerRows.rows {
		inputCount := expanded.producers[row.group].group.InputCount()
		epoch.producers[rowIndex] = producerEpoch{state: row.state, group: row.group, generation: 1, applied: 1, inputs: make([]carrier.PointState, inputCount), inputStates: make([]carrier.State, inputCount)}
	}
	for rowIndex, row := range producerRows.rows {
		producer := &expanded.producers[row.group]
		cache := &epoch.producers[rowIndex]
		epoch.currentState = int(row.state)
		values, valuesOK := epoch.inputs(producer, cache)
		if !valuesOK {
			work.Close()
			t.Fatal("artifact-read-index evaluated inputs")
		}
		for inputIndex := range values {
			cache.inputStates[inputIndex] = values[inputIndex].State()
		}
	}
	epoch.currentState = -1
	t.Cleanup(func() { work.Close() })
	return artifactReadIndexLawFixture{
		lane: lane, source: source, epoch: epoch, runtime: &expanded, group: group,
		inputPoint: inputPoint, outputPoint: outputPoint, outputStates: outputStates,
		inputStates: inputStates, primary: primary, alternate: artifactReadIndexLawUnits(t, runtime, primary),
	}
}

func artifactReadIndexLawConsumer(fixture artifactReadIndexLawFixture, contextIndex int) stateGroupKey {
	return stateGroupKey{state: contextfiber.StateOrdinal(fixture.outputStates[contextIndex]), group: fixture.group}
}

func artifactReadIndexLawPrimaryKey(fixture artifactReadIndexLawFixture, contextIndex int) artifactProducerReadKey {
	return artifactProducerReadKey{state: contextfiber.StateOrdinal(fixture.inputStates[contextIndex]), unit: fixture.primary}
}

func artifactReadIndexLawAlternateKey(fixture artifactReadIndexLawFixture, contextIndex int) artifactProducerReadKey {
	return artifactProducerReadKey{state: contextfiber.StateOrdinal(fixture.inputStates[contextIndex]), unit: fixture.alternate}
}

func artifactReadIndexLawPrimaryFactorKey(fixture artifactReadIndexLawFixture, contextIndex int) artifactProducerReadFactorKey {
	slot, _ := fixture.primary.Slot()
	return artifactProducerReadFactorKey{state: contextfiber.StateOrdinal(fixture.inputStates[contextIndex]), slot: slot}
}

func artifactReadIndexLawAlternateFactorKey(fixture artifactReadIndexLawFixture, contextIndex int) artifactProducerReadFactorKey {
	slot, _ := fixture.alternate.Slot()
	return artifactProducerReadFactorKey{state: contextfiber.StateOrdinal(fixture.inputStates[contextIndex]), slot: slot}
}

func artifactReadIndexLawInstall(t testing.TB, fixture artifactReadIndexLawFixture, contextIndex int, unit carrier.Unit) {
	t.Helper()
	if !fixture.epoch.replaceArtifactProducerReads(fixture.outputStates[contextIndex], fixture.group, []demand.Observation{{Input: 0, Unit: unit}}) {
		t.Fatal("artifact-read-index valid replacement")
	}
}

type artifactReadIndexLawSnapshot struct {
	byKey    map[artifactProducerReadKey][]stateGroupKey
	byFactor map[artifactProducerReadFactorKey][]stateGroupKey
	keys     []artifactProducerReadKey
	factors  []artifactProducerReadFactorKey
}

func artifactReadIndexLawSnapshotOf(fixture artifactReadIndexLawFixture, contextIndex int) artifactReadIndexLawSnapshot {
	cache, ok := fixture.epoch.producerCache(contextfiber.StateOrdinal(fixture.outputStates[contextIndex]), fixture.group)
	if !ok || cache == nil {
		return artifactReadIndexLawSnapshot{}
	}
	byKey := make(map[artifactProducerReadKey][]stateGroupKey, len(fixture.epoch.artifactProducerReads.byKey))
	for key, rows := range fixture.epoch.artifactProducerReads.byKey {
		byKey[key] = append([]stateGroupKey(nil), rows...)
	}
	byFactor := make(map[artifactProducerReadFactorKey][]stateGroupKey, len(fixture.epoch.artifactProducerReads.byFactor))
	for key, rows := range fixture.epoch.artifactProducerReads.byFactor {
		byFactor[key] = append([]stateGroupKey(nil), rows...)
	}
	return artifactReadIndexLawSnapshot{
		byKey: byKey, byFactor: byFactor,
		keys:    append([]artifactProducerReadKey(nil), cache.artifactReadKeys...),
		factors: append([]artifactProducerReadFactorKey(nil), cache.artifactReadFactorKeys...),
	}
}

func artifactReadIndexLawAssertSnapshot(t testing.TB, fixture artifactReadIndexLawFixture, contextIndex int, want artifactReadIndexLawSnapshot) {
	t.Helper()
	got := artifactReadIndexLawSnapshotOf(fixture, contextIndex)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("artifact-read-index state mutated on refusal: got=%#v want=%#v", got, want)
	}
}

func artifactReadIndexLawChange(t testing.TB, fixture artifactReadIndexLawFixture) (carrier.ChangeSet, carrier.Unit, shape.Slot) {
	t.Helper()
	pointIndex := fixture.outputPoint
	stateIndex, stateOK := fixture.source.runtime.executionPlan.Lookup(0, contextfiber.PointOrdinal(pointIndex))
	if !stateOK || int(stateIndex) < 0 || int(stateIndex) >= len(fixture.source.points) {
		t.Fatal("artifact-read-index source publication point")
	}
	whole, wholeOK := support.FromGuard(fixture.source.runtime.carrier.Guards(), fixture.source.runtime.carrier.Guards().True())
	state, stateOK := carrier.NewState(fixture.source.runtime.carrier, fixture.source.runtime.pointScopes[pointIndex], whole)
	if !wholeOK || !stateOK {
		t.Fatal("artifact-read-index source publication state")
	}
	producer := fixture.source.runtime.producers[fixture.group]
	if producer.span.start < 0 || producer.span.end <= producer.span.start || int(producer.span.end) > len(fixture.source.runtime.program.memberTable) {
		t.Fatal("artifact-read-index source publication member span")
	}
	geometry, geometryOK := fixture.source.runtime.program.memberTable[producer.span.start].geometry()
	if !geometryOK {
		t.Fatal("artifact-read-index source publication member geometry")
	}
	targets := geometry.targets()
	outputSlot, outputSlotOK := geometry.outputSlot()
	if len(targets) != 1 || !outputSlotOK || int(outputSlot) < 0 || int(outputSlot) >= len(fixture.source.runtime.program.factorOwners) {
		t.Fatal("artifact-read-index source publication target")
	}
	outputFactor, outputFactorOK := fixture.source.runtime.program.factorOwners[outputSlot].(*boundFactor[uint64, uint64])
	if !outputFactorOK || outputFactor == nil || outputFactor.binding == nil {
		t.Fatal("artifact-read-index source publication factor owner")
	}
	patch := outputFactor.binding.Begin(fixture.source.work, state)
	if patch == nil || !patch.Write(targets[0], state.Support(), uint64(2)) {
		t.Fatal("artifact-read-index source publication patch")
	}
	publication, publicationOK := patch.Accept(fixture.source.work)
	if !publicationOK {
		t.Fatal("artifact-read-index source publication patch acceptance")
	}
	_, changes, publishable := fixture.source.work.Commit(state, []carrier.Patch{publication})
	if !publishable || !fixture.source.runtime.carrier.OwnsChangeSet(changes) || changes.Count() == 0 || changes.FactorCount() == 0 {
		t.Fatalf("artifact-read-index source publication rows=%d factors=%d publishable=%t", changes.Count(), changes.FactorCount(), publishable)
	}
	for rowIndex := 0; rowIndex < changes.Count(); rowIndex++ {
		row, rowOK := changes.At(rowIndex)
		if rowOK && row.Unit().Kind() == carrier.ExactUnit {
			slot, slotOK := row.Unit().Slot()
			if slotOK {
				return changes, row.Unit(), slot
			}
		}
	}
	t.Fatal("artifact-read-index source publication exact row")
	return carrier.ChangeSet{}, carrier.Unit{}, 0
}

func TestArtifactProducerReadIndexIsolatesStateOrdinalAndFactorSlot(t *testing.T) {
	fixture := newArtifactReadIndexLawFixture(t)
	artifactReadIndexLawInstall(t, fixture, 0, fixture.primary)
	artifactReadIndexLawInstall(t, fixture, 1, fixture.primary)
	consumer0, consumer1 := artifactReadIndexLawConsumer(fixture, 0), artifactReadIndexLawConsumer(fixture, 1)
	primary0, primary1 := artifactReadIndexLawPrimaryKey(fixture, 0), artifactReadIndexLawPrimaryKey(fixture, 1)
	factor0, factor1 := artifactReadIndexLawPrimaryFactorKey(fixture, 0), artifactReadIndexLawPrimaryFactorKey(fixture, 1)
	if !reflect.DeepEqual(fixture.epoch.artifactProducerReads.byKey[primary0], []stateGroupKey{consumer0}) || !reflect.DeepEqual(fixture.epoch.artifactProducerReads.byKey[primary1], []stateGroupKey{consumer1}) {
		t.Fatalf("same Unit crossed StateOrdinal rows: byKey=%#v", fixture.epoch.artifactProducerReads.byKey)
	}
	if !reflect.DeepEqual(fixture.epoch.artifactProducerReads.byFactor[factor0], []stateGroupKey{consumer0}) || !reflect.DeepEqual(fixture.epoch.artifactProducerReads.byFactor[factor1], []stateGroupKey{consumer1}) {
		t.Fatalf("same factor slot crossed StateOrdinal rows: byFactor=%#v", fixture.epoch.artifactProducerReads.byFactor)
	}

	artifactReadIndexLawInstall(t, fixture, 0, fixture.alternate)
	alternate0, alternateFactor0 := artifactReadIndexLawAlternateKey(fixture, 0), artifactReadIndexLawAlternateFactorKey(fixture, 0)
	if _, present := fixture.epoch.artifactProducerReads.byKey[primary0]; present || !reflect.DeepEqual(fixture.epoch.artifactProducerReads.byKey[primary1], []stateGroupKey{consumer1}) {
		t.Fatal("factor-slot replacement retained the old contextual exact key")
	}
	if !reflect.DeepEqual(fixture.epoch.artifactProducerReads.byKey[alternate0], []stateGroupKey{consumer0}) || !reflect.DeepEqual(fixture.epoch.artifactProducerReads.byFactor[alternateFactor0], []stateGroupKey{consumer0}) {
		t.Fatal("factor-slot replacement did not isolate the new slot")
	}
	if !reflect.DeepEqual(fixture.epoch.artifactProducerReads.byFactor[factor1], []stateGroupKey{consumer1}) {
		t.Fatal("factor-slot replacement crossed the sibling StateOrdinal")
	}
}

func TestArtifactProducerReadReplacementRefusesMalformedDestinationTransactionally(t *testing.T) {
	for _, test := range []struct {
		name    string
		corrupt func(artifactReadIndexLawFixture, stateGroupKey)
	}{
		{name: "empty exact destination", corrupt: func(f artifactReadIndexLawFixture, _ stateGroupKey) {
			f.epoch.artifactProducerReads.byKey[artifactReadIndexLawAlternateKey(f, 0)] = []stateGroupKey{}
		}},
		{name: "unsorted exact destination", corrupt: func(f artifactReadIndexLawFixture, consumer stateGroupKey) {
			f.epoch.artifactProducerReads.byKey[artifactReadIndexLawAlternateKey(f, 0)] = []stateGroupKey{{state: consumer.state + 1, group: consumer.group}, consumer}
		}},
		{name: "empty factor destination", corrupt: func(f artifactReadIndexLawFixture, _ stateGroupKey) {
			f.epoch.artifactProducerReads.byFactor[artifactReadIndexLawAlternateFactorKey(f, 0)] = []stateGroupKey{}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newArtifactReadIndexLawFixture(t)
			artifactReadIndexLawInstall(t, fixture, 0, fixture.primary)
			consumer := artifactReadIndexLawConsumer(fixture, 0)
			test.corrupt(fixture, consumer)
			before := artifactReadIndexLawSnapshotOf(fixture, 0)
			if fixture.epoch.replaceArtifactProducerReads(fixture.outputStates[0], fixture.group, []demand.Observation{{Input: 0, Unit: fixture.alternate}}) {
				t.Fatal("malformed destination admitted")
			}
			artifactReadIndexLawAssertSnapshot(t, fixture, 0, before)
		})
	}
}

func TestArtifactProducerReadReplacementRefusesMalformedPreviousBucketTransactionally(t *testing.T) {
	for _, test := range []struct {
		name    string
		corrupt func(artifactReadIndexLawFixture)
	}{
		{name: "missing exact predecessor", corrupt: func(f artifactReadIndexLawFixture) {
			delete(f.epoch.artifactProducerReads.byKey, artifactReadIndexLawPrimaryKey(f, 0))
		}},
		{name: "empty factor predecessor", corrupt: func(f artifactReadIndexLawFixture) {
			f.epoch.artifactProducerReads.byFactor[artifactReadIndexLawPrimaryFactorKey(f, 0)] = []stateGroupKey{}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newArtifactReadIndexLawFixture(t)
			artifactReadIndexLawInstall(t, fixture, 0, fixture.primary)
			test.corrupt(fixture)
			before := artifactReadIndexLawSnapshotOf(fixture, 0)
			if fixture.epoch.replaceArtifactProducerReads(fixture.outputStates[0], fixture.group, []demand.Observation{{Input: 0, Unit: fixture.alternate}}) {
				t.Fatal("malformed predecessor admitted")
			}
			artifactReadIndexLawAssertSnapshot(t, fixture, 0, before)
		})
	}
}

func TestArtifactProducerReadReplacementRefusesDuplicateDestinationConsumer(t *testing.T) {
	for _, test := range []struct {
		name    string
		corrupt func(artifactReadIndexLawFixture, stateGroupKey)
	}{
		{name: "exact duplicate", corrupt: func(f artifactReadIndexLawFixture, consumer stateGroupKey) {
			f.epoch.artifactProducerReads.byKey[artifactReadIndexLawAlternateKey(f, 0)] = []stateGroupKey{consumer}
		}},
		{name: "factor duplicate", corrupt: func(f artifactReadIndexLawFixture, consumer stateGroupKey) {
			f.epoch.artifactProducerReads.byFactor[artifactReadIndexLawAlternateFactorKey(f, 0)] = []stateGroupKey{consumer}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newArtifactReadIndexLawFixture(t)
			artifactReadIndexLawInstall(t, fixture, 0, fixture.primary)
			consumer := artifactReadIndexLawConsumer(fixture, 0)
			test.corrupt(fixture, consumer)
			before := artifactReadIndexLawSnapshotOf(fixture, 0)
			if fixture.epoch.replaceArtifactProducerReads(fixture.outputStates[0], fixture.group, []demand.Observation{{Input: 0, Unit: fixture.alternate}}) {
				t.Fatal("duplicate destination admitted")
			}
			artifactReadIndexLawAssertSnapshot(t, fixture, 0, before)
		})
	}
}

func TestClearArtifactProducerReadsRemovesOnlyTheExactContextMembership(t *testing.T) {
	fixture := newArtifactReadIndexLawFixture(t)
	artifactReadIndexLawInstall(t, fixture, 0, fixture.primary)
	artifactReadIndexLawInstall(t, fixture, 1, fixture.primary)
	if !fixture.epoch.clearArtifactProducerReads(fixture.outputStates[0], fixture.group) {
		t.Fatal("clear refused a valid contextual membership")
	}
	primary0, primary1 := artifactReadIndexLawPrimaryKey(fixture, 0), artifactReadIndexLawPrimaryKey(fixture, 1)
	factor0, factor1 := artifactReadIndexLawPrimaryFactorKey(fixture, 0), artifactReadIndexLawPrimaryFactorKey(fixture, 1)
	if _, present := fixture.epoch.artifactProducerReads.byKey[primary0]; present {
		t.Fatal("clear retained exact StateOrdinal membership")
	}
	if _, present := fixture.epoch.artifactProducerReads.byFactor[factor0]; present {
		t.Fatal("clear retained factor StateOrdinal membership")
	}
	if !reflect.DeepEqual(fixture.epoch.artifactProducerReads.byKey[primary1], []stateGroupKey{artifactReadIndexLawConsumer(fixture, 1)}) || !reflect.DeepEqual(fixture.epoch.artifactProducerReads.byFactor[factor1], []stateGroupKey{artifactReadIndexLawConsumer(fixture, 1)}) {
		t.Fatal("clear crossed into the sibling context")
	}
	cache, cacheOK := fixture.epoch.producerCache(contextfiber.StateOrdinal(fixture.outputStates[0]), fixture.group)
	if !cacheOK || cache == nil || len(cache.artifactReadKeys) != 0 || len(cache.artifactReadFactorKeys) != 0 {
		t.Fatal("clear retained producer cache membership")
	}
}

func TestMarkArtifactProducerReadConsumersWakesOnlyExactSourceAndFactor(t *testing.T) {
	fixture := newArtifactReadIndexLawFixture(t)
	changes, changedUnit, changedSlot := artifactReadIndexLawChange(t, fixture)
	consumer := artifactReadIndexLawConsumer(fixture, 0)
	changedState := contextfiber.StateOrdinal(fixture.outputStates[0])
	fixture.epoch.artifactProducerReads.byKey[artifactProducerReadKey{state: changedState, unit: changedUnit}] = []stateGroupKey{consumer}
	fixture.epoch.artifactProducerReads.byFactor[artifactProducerReadFactorKey{state: changedState, slot: changedSlot}] = []stateGroupKey{consumer}
	cache, cacheOK := fixture.epoch.producerCache(consumer.state, consumer.group)
	if !cacheOK || cache == nil {
		t.Fatal("artifact-read-index wake consumer cache")
	}
	if !fixture.epoch.markArtifactProducerReadConsumers(int(changedState), changes) {
		t.Fatal("exact artifact publication wake refused")
	}
	if cache.generation != 2 || !fixture.epoch.queue.ready[int(consumer.state)] || fixture.epoch.queue.count != 1 {
		t.Fatalf("exact wake generation=%d ready=%v queue=%d", cache.generation, fixture.epoch.queue.ready[int(consumer.state)], fixture.epoch.queue.count)
	}

	// A sibling contextual source carrying the same Unit is not this inverse
	// row. It must not wake the already-clean consumer or any graph-point alias.
	cache.generation, cache.applied = 1, 1
	fixture.epoch.queue = newPointQueue(len(fixture.epoch.points))
	fixture.epoch.postfixDirty = make([]bool, len(fixture.epoch.points))
	fixture.epoch.postfixPending = fixture.epoch.postfixPending[:0]
	if !fixture.epoch.markArtifactProducerReadConsumers(fixture.inputStates[1], changes) {
		t.Fatal("unrelated contextual publication refused instead of remaining a no-op")
	}
	if cache.generation != 1 || fixture.epoch.queue.count != 0 {
		t.Fatal("same Unit from a sibling StateOrdinal woke a foreign consumer")
	}

	// A publication on another Factor slot is likewise not an exact factor row.
	fixture.epoch.artifactProducerReads.byKey = make(map[artifactProducerReadKey][]stateGroupKey)
	wrongSlot, wrongSlotOK := fixture.primary.Slot()
	if !wrongSlotOK || wrongSlot == changedSlot {
		wrongSlot, wrongSlotOK = fixture.alternate.Slot()
	}
	if !wrongSlotOK || wrongSlot == changedSlot {
		t.Fatal("artifact-read-index foreign factor slot")
	}
	fixture.epoch.artifactProducerReads.byFactor = map[artifactProducerReadFactorKey][]stateGroupKey{
		{state: changedState, slot: wrongSlot}: {consumer},
	}
	if !fixture.epoch.markArtifactProducerReadConsumers(int(changedState), changes) {
		t.Fatal("foreign factor-slot publication refused instead of remaining a no-op")
	}
	if cache.generation != 1 || fixture.epoch.queue.count != 0 {
		t.Fatal("foreign factor slot woke the exact-read consumer")
	}
}
