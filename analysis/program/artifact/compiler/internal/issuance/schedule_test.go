package issuance

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	schemaissuance "github.com/wippyai/go-lua/analysis/schema/issuance"
	programissuance "github.com/wippyai/go-lua/analysis/schema/program/issuance"
	"github.com/wippyai/go-lua/analysis/schema/seal"
)

type emptySurface struct{ kind schema.SurfaceKind }

func (surface emptySurface) Kind() schema.SurfaceKind { return surface.kind }
func (emptySurface) Entries() []schema.Entry          { return nil }
func (emptySurface) Seal(seal.View, seal.Sealed) schema.SealFailure {
	return schema.SealFailure{}
}

func TestScheduleResolvesPreviousFromDeclaredStageOrder(t *testing.T) {
	table := scheduleTable(t)
	plan, ok := schemaissuance.NewPlan(table, []schemaissuance.SubscriptionSpec{{
		Family:      "occurrence/storage-write",
		Requirement: programissuance.RequirementUnrestricted,
		Form:        programissuance.FormLocalPredecessor,
		Rule:        "rule/write",
		Writes:      "axis/write",
	}})
	if !ok {
		t.Fatal("execution plan refused sealed subscription")
	}
	subscription, subscriptionOK := plan.At(0)
	if !subscriptionOK {
		t.Fatal("sealed subscription unavailable")
	}
	base := testID(1)
	predecessorStage := scheduleEntry(t, table, programissuance.StagePredecessor, schemaissuance.KindStage)
	localStage := scheduleEntry(t, table, programissuance.StageLocal, schemaissuance.KindStage)
	previousInput := scheduleEntry(t, table, programissuance.InputPreviousStage, schemaissuance.KindInput)
	pointOne := schemaissuance.DataType{Value: schemaissuance.ValuePointRange, Name: schemaissuance.TypePoint, Cardinality: schemaissuance.CardinalityOne}
	pointMany := pointOne
	pointMany.Cardinality = schemaissuance.CardinalityMany
	requests := []Request{
		{subscription: subscription, stage: localStage, base: base, parameters: []value{{typ: pointMany, present: true, points: []identity.ContentID{base}}}, inputs: []Input{{declaration: previousInput}}},
		{subscription: subscription, stage: predecessorStage, base: base, parameters: []value{
			{typ: pointOne, present: true, points: []identity.ContentID{base}},
			{typ: schemaissuance.IdentityType(schemaissuance.TypeAxisKey), present: true, key: "axis/write"},
		}, inputs: []Input{{declaration: previousInput}}},
	}
	schedule, scheduled := BuildSchedule(41, plan, requests)
	if !scheduled || schedule.NodeCount() != 3 || schedule.EmissionCount() != 2 {
		t.Fatalf("schedule refused: nodes=%d emissions=%d", schedule.NodeCount(), schedule.EmissionCount())
	}
	predecessor, _ := schedule.EmissionAt(1)
	local, _ := schedule.EmissionAt(0)
	input, inputOK := predecessor.InputPointAt(0)
	if !inputOK || input != local.Point() {
		t.Fatal("predecessor data input was not selected from the preceding Local stage")
	}
	for _, emission := range []Emission{local, predecessor} {
		writers, found := schedule.PointWriters(emission.Point())
		if !found || len(writers) != 1 || writers[0] != "axis/write" {
			t.Fatalf("point writers = %v found=%v, want exact owner-issued axis", writers, found)
		}
	}
	for _, stage := range []schema.Key{programissuance.StageLocal, programissuance.StagePredecessor} {
		writers, available := schedule.StageWriters(base, stage)
		if !available || len(writers) != 1 || writers[0] != "axis/write" {
			t.Fatalf("stage %s writers = %v available=%v, want exact owner-issued axis", stage, writers, available)
		}
	}
	writers, available := schedule.StageWriters(base, programissuance.StageCallEffect)
	if !available || len(writers) != 0 {
		t.Fatalf("unissued stage writers = %v available=%v, want canonical empty set", writers, available)
	}
	if _, found := schedule.PointWriters(testID(99)); found {
		t.Fatal("unissued point acquired fabricated writer authority")
	}
}

func TestScheduleEmissionIsTheIssuedRequest(t *testing.T) {
	table := scheduleTable(t)
	plan, ok := schemaissuance.NewPlan(table, []schemaissuance.SubscriptionSpec{{
		Family:      "occurrence/storage-write",
		Requirement: programissuance.RequirementUnrestricted,
		Form:        programissuance.FormLocalPredecessor,
		Rule:        "rule/write",
		Writes:      "axis/write",
	}})
	if !ok {
		t.Fatal("execution plan refused sealed subscription")
	}
	subscription, subscriptionOK := plan.At(0)
	if !subscriptionOK {
		t.Fatal("sealed subscription unavailable")
	}
	base := testID(7)
	localStage := scheduleEntry(t, table, programissuance.StageLocal, schemaissuance.KindStage)
	previousInput := scheduleEntry(t, table, programissuance.InputPreviousStage, schemaissuance.KindInput)
	pointMany := schemaissuance.DataType{Value: schemaissuance.ValuePointRange, Name: schemaissuance.TypePoint, Cardinality: schemaissuance.CardinalityMany}
	schedule, scheduled := BuildSchedule(41, plan, []Request{{
		subscription: subscription,
		stage:        localStage,
		base:         base,
		parameters:   []value{{typ: pointMany, present: true, points: []identity.ContentID{base}}},
		input:        Input{declaration: previousInput},
	}})
	if !scheduled {
		t.Fatal("schedule refused a single issued request")
	}
	if schedule.EmissionCount() != 1 {
		t.Fatalf("EmissionCount=%d, want 1 issued request", schedule.EmissionCount())
	}
	if _, ok := schedule.EmissionAt(-1); ok {
		t.Fatal("EmissionAt(-1) issued a row")
	}
	if _, ok := schedule.EmissionAt(1); ok {
		t.Fatal("EmissionAt past count issued a row")
	}
	var emission Emission
	var emissionOK bool
	emission, emissionOK = schedule.EmissionAt(0)
	if !emissionOK {
		t.Fatal("EmissionAt(0) unavailable")
	}
	request := emission.Request()
	point := emission.Point()
	if request.Stage() != localStage || !point.Available() {
		t.Fatalf("emission request/point unavailable: stage=%v point=%s", request.Stage() != nil, point)
	}
	if _, nativeOK := emission.Native(); !nativeOK {
		t.Fatal("Emission.Native unavailable on a KindStage request")
	}
}

func TestScheduleEmissionCarriesSealedStageNativeBit(t *testing.T) {
	table := scheduleTable(t)
	plan, ok := schemaissuance.NewPlan(table, []schemaissuance.SubscriptionSpec{{
		Family:      "occurrence/call",
		Requirement: programissuance.RequirementUnrestricted,
		Form:        programissuance.FormCallDispatch,
		Rule:        "rule/call",
		Writes:      "axis/call",
	}})
	if !ok {
		t.Fatal("call execution plan refused sealed subscription")
	}
	subscription, subscriptionOK := plan.At(0)
	stage := scheduleEntry(t, table, programissuance.StageCallDispatch, schemaissuance.KindStage)
	input := scheduleEntry(t, table, programissuance.InputPreviousStage, schemaissuance.KindInput)
	base := testID(1)
	pointMany := schemaissuance.DataType{Value: schemaissuance.ValuePointRange, Name: schemaissuance.TypePoint, Cardinality: schemaissuance.CardinalityMany}
	request := Request{
		subscription: subscription,
		stage:        stage,
		base:         base,
		parameters:   []value{{typ: pointMany, present: true, points: []identity.ContentID{base}}},
		input:        Input{declaration: input},
	}
	schedule, scheduled := BuildSchedule(41, plan, []Request{request})
	emission, emissionOK := schedule.EmissionAt(0)
	native, nativeOK := emission.Native()
	inputPoint, inputOK := emission.InputPointAt(0)
	if !subscriptionOK || !scheduled || schedule.NodeCount() != 2 || schedule.EmissionCount() != 1 ||
		!emissionOK || !nativeOK || native != stage.Native() || !native || !inputOK || inputPoint != base {
		t.Fatalf("native emission = (subscription=%v scheduled=%v nodes=%d emissions=%d row=%v native=%v/%v input=%x/%v), want sealed native stage over base",
			subscriptionOK, scheduled, schedule.NodeCount(), schedule.EmissionCount(), emissionOK, native, nativeOK, inputPoint, inputOK)
	}
}

func TestScheduleRefusesDeclaredComputationDependencyCycle(t *testing.T) {
	table := scheduleTable(t)
	plan, ok := schemaissuance.NewPlan(table, nil)
	if !ok {
		t.Fatal("empty execution plan refused sealed table")
	}
	stage := scheduleEntry(t, table, programissuance.StageComputation, schemaissuance.KindStage)
	input := scheduleEntry(t, table, programissuance.InputPreviousStage, schemaissuance.KindInput)
	base, left, right := testID(1), testID(2), testID(3)
	pointMany := schemaissuance.DataType{Value: schemaissuance.ValuePointRange, Name: schemaissuance.TypePoint, Cardinality: schemaissuance.CardinalityMany}
	ruleType := schemaissuance.IdentityType(schemaissuance.TypeRuleKey)
	contentType := schemaissuance.IdentityType(programissuance.TypeContentID)
	request := func(node, dependency identity.ContentID) Request {
		return Request{
			stage: stage,
			base:  base,
			parameters: []value{
				{typ: pointMany, present: true, points: []identity.ContentID{base}},
				{typ: ruleType, present: true, key: "rule/arithmetic"},
				{typ: contentType, present: true, identity: node},
				{typ: contentType, present: true, identity: dependency},
				{typ: contentType, present: true, identity: dependency},
			},
			input: Input{declaration: input},
		}
	}
	if schedule, scheduled := BuildSchedule(41, plan, []Request{request(left, right), request(right, left)}); scheduled || schedule.NodeCount() != 0 || schedule.EmissionCount() != 0 {
		t.Fatal("cyclic computation dependency schedule was admitted")
	}
}

func scheduleTable(t *testing.T) schemaissuance.Table {
	t.Helper()
	entries, ok := programissuance.Entries()
	if !ok {
		t.Fatal("Program issuance declarations refused")
	}
	builder := seal.NewBuilder()
	builder.Register(emptySurface{schema.SurfaceKindStructure})
	builder.Register(emptySurface{schema.SurfaceKindAxis})
	builder.Register(schemaissuance.NewSurface(entries))
	for kind := schema.SurfaceKindRule; kind <= schema.SurfaceKindObservation; kind++ {
		builder.Register(emptySurface{kind})
	}
	sealed, failure := builder.Seal()
	if failure.Available() || sealed == nil {
		t.Fatalf("issuance schema refused: %+v", failure)
	}
	view, ok := sealed.Surface(schema.SurfaceKindIssuance)
	table, tableOK := schemaissuance.NewTable(view)
	if !ok || !tableOK {
		t.Fatal("sealed issuance table unavailable")
	}
	return table
}

func scheduleEntry(t *testing.T, table schemaissuance.Table, key schema.Key, kind schemaissuance.Kind) *schemaissuance.Entry {
	t.Helper()
	entry, ok := table.Entry(key, kind)
	if !ok {
		t.Fatalf("missing %s", key)
	}
	return entry
}

func testID(value byte) identity.ContentID {
	var result identity.ContentID
	result[0] = value
	return result
}
