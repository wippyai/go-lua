package engine

import (
	"context"
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"
	"github.com/wippyai/go-lua/analysis/engine/internal/contextfiber"
	"github.com/wippyai/go-lua/analysis/engine/internal/demand"
	"github.com/wippyai/go-lua/analysis/engine/rows"
	"github.com/wippyai/go-lua/analysis/identity"
	programissuance "github.com/wippyai/go-lua/analysis/schema/program/issuance"
	queryschema "github.com/wippyai/go-lua/analysis/schema/query"
)

// newArtifactSummaryReadLane builds the smallest sealed artifact whose one
// producer records a declared summary predecessor. The ordinary artifact law
// already covers exact reads; this fixture isolates the generic predecessor
// class that previously fell through the inverse filter.
func newArtifactSummaryReadLane(t testing.TB) readLaneFixture {
	t.Helper()
	builder := NewSchema()
	factor, factorOK := DeclareFactorSlot[uint64](builder, coldKey(962_000))
	exactForm, exactFormOK := factor.ExactRead()
	summaryForm, summaryFormOK := factor.SummaryRead(coldKey(962_001))
	writeForm, writeFormOK := factor.ExactWrite()
	outputReadForm, outputReadFormOK := exactForm, exactFormOK
	rule, ruleOK := DeclareRuleSlot[uint64, summaryLawOperand](builder, SchemaRuleSpec[uint64]{
		Semantic: coldKey(962_002), OperandFamily: coldKey(962_008), Inputs: 1,
		Output: factor.Ref(),
	})
	input, inputOK := rule.Input(0)
	summarySlot, summarySlotOK := SchemaRead(rule, summaryForm, input)
	writeSlot, writeSlotOK := SchemaWrite(rule, writeForm)
	query, queryOK := DeclareQuerySlot[uint64](builder, SchemaQuerySpec{Semantic: coldKey(962_003), Freezer: coldKey(962_004), Population: queryschema.PopulationKindSelectedPoint})
	if queryOK {
		queryOK = SchemaQueryRead(query, outputReadForm)
	}
	schema, schemaOK := builder.Seal()
	if !factorOK || !exactFormOK || !summaryFormOK || !writeFormOK || !outputReadFormOK || !ruleOK || !inputOK || !summarySlotOK || !writeSlotOK || !queryOK || !schemaOK || schema == nil {
		t.Fatal("artifact-summary-read-index schema")
	}
	binding := NewSchemaBinding(schema)
	spec := HotRuleSpec[uint64, summaryLawOperand]{
		OperandContent: func(summaryLawOperand) (summaryLawOperand, [32]byte, bool) {
			return summaryLawOperand{}, [32]byte{0x5a}, true
		},
		OperandResolver: func(OperandCoords) (summaryLawOperand, bool) { return summaryLawOperand{}, true },
		Fold: func(frame Frame[uint64, summaryLawOperand]) RuleResult[uint64] {
			return Staged(frame, uint64(1))
		},
	}
	querySpec := hotExactQuerySpec()
	querySpec.Result.Semantic = coldKey(962_004)
	if !BindFactor(binding, factor, hotUintFactorSpec()) ||
		!BindIdentitySummaryReadForFactor[uint64](binding, factor, summaryForm) ||
		!BindExactQuery(binding, query, factor, querySpec) {
		t.Fatal("artifact-summary-read-index factor binding")
	}
	if !BindSelectedExactRuleDirect[uint64, uint64, summaryLawOperand](binding, rule, writeSlot, factor.Ref(), spec, func(summaryLawOperand) (uint64, bool) { return 0, true }) {
		t.Fatal("artifact-summary-read-index direct rule")
	}
	if _, bound := BindSelectedRuleDirectSummaryRead[uint64, uint64, summaryLawOperand, uint64, OrderedCells[uint64]](binding, rule, summarySlot, factor.Ref(), summaryForm); !bound {
		t.Fatal("artifact-summary-read-index summary read")
	}
	capability, capabilityOK := IssueMountedRuleCapability(binding, rule)
	if !capabilityOK || !RegisterRuleSlot(binding, rule, capability) || !binding.Seal() {
		t.Fatal("artifact-summary-read-index capability")
	}
	implementation, implementationOK := RuleImplementationAt[uint64, uint64, summaryLawOperand](binding, rule)
	queryImplementation, queryImplementationOK := ExactQueryImplementationAt[uint64, uint64](binding, query)
	if !implementationOK || implementation == nil || !queryImplementationOK || queryImplementation == nil {
		t.Fatal("artifact-summary-read-index implementation")
	}
	program := constructArtifactSummaryReadLaneProgram(t, binding, schema, capability, implementation, queryImplementation)
	plane, planeOK := bindProgramPlane(program.state, program.graph)
	if !planeOK || plane == nil {
		t.Fatal("artifact-summary-read-index plane")
	}
	lane := readLaneFixture{binding: binding, program: program, plane: plane, readKey: schema.factorSemanticAt(0), factors: plane.byKey}
	lane.member = readLaneMember(t, program, schema.ruleSemanticAt(0))
	return lane
}

func constructArtifactSummaryReadLaneProgram(t testing.TB, binding *SchemaBinding, schema *Schema, capability RuleSlotCapability, implementation *RuleImplementation[uint64, uint64, summaryLawOperand], queryImplementation *ExactQueryImplementation[uint64, uint64]) *CommittedProgram {
	t.Helper()
	spec, specOK := rows.NewArtifactScalarSpec(readLaneID(962_010), readLaneID(962_011), identity.ContentID(schema.ID().Digest()), rows.ArtifactScalarCapacity{Roles: 1, Points: 2, Regions: 1, Events: 4, Rules: 1, Bodies: 1})
	role, roleOK := spec.DeclareRole(readLaneID(962_012))
	entry, member := readLaneID(962_013), readLaneID(962_014)
	_, entryOK := spec.AddPoint(rows.ArtifactScalarPoint{ID: entry, Initial: true})
	_, memberOK := spec.AddPoint(rows.ArtifactScalarPoint{ID: member})
	region, regionOK := spec.AddRegion(rows.ArtifactScalarRegion{ID: readLaneID(962_015), Head: entry})
	regionOK = regionOK && spec.AddRegionMember(region, entry) && spec.AddRegionMember(region, member)
	events := spec.AddEvent(rows.ArtifactScalarEvent{Kind: rows.ArtifactEventEnter, Region: readLaneID(962_015)}) &&
		spec.AddEvent(rows.ArtifactScalarEvent{Kind: rows.ArtifactEventPoint, Point: entry}) &&
		spec.AddEvent(rows.ArtifactScalarEvent{Kind: rows.ArtifactEventPoint, Point: member}) &&
		spec.AddEvent(rows.ArtifactScalarEvent{Kind: rows.ArtifactEventExit, Region: readLaneID(962_015)})
	body, bodyOK := spec.AddBody(rows.ArtifactScalarBody{ID: readLaneID(962_016)})
	if !specOK || !roleOK || !entryOK || !memberOK || !regionOK || !events || !bodyOK || !spec.AddBodyEntry(body, entry) || !spec.AddBodyExit(body, member) || !spec.AddRule(rows.ArtifactScalarRule{Role: role, Stage: programissuance.StageCallDispatch, Point: member, Inputs: [6]identity.ContentID{entry}, InputCount: 1, ID: readLaneID(962_017), Native: true}) {
		t.Fatal("artifact-summary-read-index artifact rows")
	}
	installArtifactStageTable(t, spec)
	template, templateOK := rows.NewArtifactScalarTemplate(spec)
	bootstrap, bootstrapOK := NewProgramBootstrap(readLaneID(962_018), readLaneID(962_019))
	contexts := explicitTestContextDirectory(t, readLaneID(962_018), []identity.ContentID{readLaneID(962_020)}, readLaneID(962_021), readLaneID(962_022))
	cell, cellOK := implementation.sealedRuleCell()
	queryAdmission, queryAdmissionOK := NewExactQueryAdmission(queryImplementation, readLaneID(962_023), readLaneID(962_020), member, explicitTestContext(t, contexts, readLaneID(962_020)))
	if !templateOK || !bootstrapOK || !cellOK || cell == nil || !queryAdmissionOK {
		t.Fatal("artifact-summary-read-index artifact seal")
	}
	mount := MountedProgramArtifact{Template: template, Roles: []MountedProgramRole{{Scalar: role, Capability: capability}}, Module: readLaneID(962_020)}
	admission := MountedProgramAdmission{Mounted: []MountedRuleAdmission{{Capability: capability, Mount: readLaneID(962_020), Point: member, Occurrence: readLaneID(962_017)}}, Queries: []ProgramQueryAdmission{queryAdmission}}
	program, refusal, constructed := ConstructProgram(ProgramDeclaration{Binding: binding, Mounts: []MountedProgramArtifact{mount}, Bootstrap: bootstrap, Contexts: contexts, Admission: admission})
	if !constructed || program == nil {
		t.Fatalf("artifact-summary-read-index ConstructProgram stage=%v seal=%v commit=%v", refusal.Stage(), refusal.Seal(), refusal.Commit())
	}
	return program
}

func newArtifactSummaryReadIndexLawFixture(t testing.TB) artifactReadIndexLawFixture {
	t.Helper()
	lane := newArtifactSummaryReadLane(t)
	solver, failure, sealed := lane.program.Seal(nil)
	if !sealed || solver == nil {
		t.Fatalf("artifact-summary-read-index runtime seal=%t failure=%v", sealed, failure)
	}
	runtime := solver.runtime
	var summary carrier.Unit
	for _, producer := range runtime.producers {
		for _, read := range producer.reads {
			if read.Unit.Kind() == carrier.SummaryUnit {
				summary = read.Unit
			}
		}
	}
	if summary == (carrier.Unit{}) || summary.Kind() != carrier.SummaryUnit {
		t.Fatal("artifact-summary-read-index declared predecessor unit")
	}
	source, sourceOK := newRuntimeEpoch(runtime, solver.relation, context.Background())
	if !sourceOK || source == nil {
		t.Fatal("artifact-summary-read-index source epoch")
	}
	epoch, epochOK := newRuntimeEpoch(runtime, solver.relation, context.Background())
	if !epochOK || epoch == nil {
		source.discard()
		t.Fatal("artifact-summary-read-index inverse epoch")
	}
	t.Cleanup(func() {
		source.discard()
		epoch.discard()
	})

	group := -1
	for index, producer := range runtime.producers {
		seenSummary := false
		for _, read := range producer.reads {
			seenSummary = seenSummary || read.Unit.Kind() == carrier.SummaryUnit
		}
		if seenSummary && producer.group.InputCount() != 0 {
			group = index
			break
		}
	}
	if group < 0 {
		t.Fatal("artifact-summary-read-index mixed producer")
	}
	producer := runtime.producers[group]
	input, inputOK := producer.group.InputAt(0)
	output := producer.group.Output()
	inputPoint, inputPointOK := runtime.graph.PointIndex(input.Point())
	outputPoint, outputPointOK := runtime.graph.PointIndex(output)
	if !inputOK || !inputPointOK || !outputPointOK || inputPoint < 0 || outputPoint < 0 {
		t.Fatal("artifact-summary-read-index point addresses")
	}
	contextRow, contextOK := runtime.contexts.ContextAt(0)
	contextOrdinal, ordinalOK := runtime.contextIndex.ContextOrdinal(contextRow.ID())
	outputState, outputStateOK := runtime.executionPlan.Lookup(contextOrdinal, contextfiber.PointOrdinal(outputPoint))
	inputState, inputStateOK := runtime.executionPlan.Lookup(contextOrdinal, contextfiber.PointOrdinal(inputPoint))
	if !contextOK || !ordinalOK || !outputStateOK || !inputStateOK {
		t.Fatal("artifact-summary-read-index state addresses")
	}
	cache, cacheOK := epoch.producerCache(outputState, group)
	if !cacheOK || cache == nil {
		t.Fatal("artifact-summary-read-index producer cache")
	}
	epoch.currentState = int(outputState)
	values, valuesOK := epoch.inputs(&runtime.producers[group], cache)
	epoch.currentState = -1
	if !valuesOK || len(values) != len(cache.inputStates) {
		t.Fatal("artifact-summary-read-index evaluated inputs")
	}
	for index := range values {
		cache.inputStates[index] = values[index].State()
	}
	return artifactReadIndexLawFixture{
		lane: lane, source: source, epoch: epoch, runtime: runtime, group: group,
		inputPoint: inputPoint, outputPoint: outputPoint,
		outputStates: [2]int{int(outputState), int(outputState)},
		inputStates:  [2]int{int(inputState), int(inputState)},
		primary:      summary, alternate: summary,
	}
}

func TestArtifactProducerReadIndexRetainsSummaryPredecessorAndWakes(t *testing.T) {
	fixture := newArtifactSummaryReadIndexLawFixture(t)
	if !fixture.epoch.replaceArtifactProducerReads(fixture.outputStates[0], fixture.group, []demand.Observation{{Input: 0, Unit: fixture.primary}}) {
		t.Fatal("artifact-summary-read-index mixed replacement")
	}
	consumer := artifactReadIndexLawConsumer(fixture, 0)
	summaryKey := artifactReadIndexLawPrimaryKey(fixture, 0)
	if summaryKey.unit.Kind() != carrier.SummaryUnit || !reflect.DeepEqual(fixture.epoch.artifactProducerReads.byKey[summaryKey], []stateGroupKey{consumer}) {
		t.Fatalf("declared predecessor inverse omitted a Unit: byKey=%#v", fixture.epoch.artifactProducerReads.byKey)
	}
	if _, present := fixture.epoch.artifactProducerReads.byFactor[artifactProducerReadFactorKey{state: summaryKey.state, slot: mustArtifactSummaryReadIndexSlot(t, fixture.primary)}]; !present {
		t.Fatal("summary predecessor factor inverse missing")
	}

	changes, changedExact, _ := artifactReadIndexLawChange(t, fixture)
	if changedExact.Kind() != carrier.ExactUnit {
		t.Fatal("artifact-summary-read-index exact publication row")
	}
	changedSummary := false
	for index := 0; index < changes.Count(); index++ {
		row, rowOK := changes.At(index)
		if rowOK && row.Unit().Same(fixture.alternate) {
			changedSummary = true
			break
		}
	}
	if !changedSummary {
		t.Fatal("artifact-summary-read-index publication omitted summary row")
	}

	cache, cacheOK := fixture.epoch.producerCache(consumer.state, consumer.group)
	if !cacheOK || cache == nil {
		t.Fatal("artifact-summary-read-index wake cache")
	}
	cache.generation, cache.applied = 1, 1
	fixture.epoch.queue = newPointQueue(len(fixture.epoch.points))
	fixture.epoch.postfixDirty = make([]bool, len(fixture.epoch.points))
	fixture.epoch.postfixPending = fixture.epoch.postfixPending[:0]
	if !fixture.epoch.markArtifactProducerReadConsumers(int(summaryKey.state), changes) {
		t.Fatal("artifact-summary-read-index mixed publication wake")
	}
	if cache.generation != 2 || fixture.epoch.queue.count != 1 || !fixture.epoch.queue.ready[int(consumer.state)] {
		t.Fatalf("mixed publication wake generation=%d queue=%d ready=%v", cache.generation, fixture.epoch.queue.count, fixture.epoch.queue.ready[int(consumer.state)])
	}
}

func mustArtifactSummaryReadIndexSlot(t testing.TB, unit carrier.Unit) shape.Slot {
	t.Helper()
	slot, slotOK := unit.Slot()
	if !slotOK {
		t.Fatal("artifact-summary-read-index Unit slot")
	}
	return slot
}
