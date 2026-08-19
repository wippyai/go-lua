package operation

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/analysis/schema"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
)

func TestQueryBuilderPublishesOperationValuesAndEffects(t *testing.T) {
	core := compileTestCore(t, Input{
		Operations: []OperationInput{
			{
				Source: 0, Bindings: []vocabulary.BindingSpec{testBinding("query")}, InputFormalCount: 1, TypeFormalCount: 1,
				ValuesVars: 1, OutcomeValueSlots: []OutcomeInput{{ValueSlots: 1, Anchor: []byte("query")}},
			},
		},
	})
	anyType, ok := schematype.NewPrimitive(schematype.PrimitiveAny)
	if !ok {
		t.Fatal("primitive Any unavailable")
	}
	relation := schemaEntryIDForTest()
	builder, err := BeginQuery(core)
	if err != nil {
		t.Fatalf("BeginQuery: %v", err)
	}
	types, err := builder.AppendQueryTypes(map[string][]byte{"any": nil}, map[string]schematype.Type{"any": anyType})
	if err != nil {
		t.Fatalf("AppendQueryTypes: %v", err)
	}
	values, err := builder.AppendQueryValues(QueryValuesDeclaration{
		Owner: 1, Types: []string{"any"}, Tail: vocabulary.ValuesClosed,
	}, types)
	if err != nil {
		t.Fatalf("AppendQueryValues: %v", err)
	}
	if _, err := builder.AppendEffect(EffectInput{Target: 1, Values: []vocabulary.ValueFormal{0}, Types: []vocabulary.TypeFormal{0}, ValuesVar: []vocabulary.ValuesVar{0}}); err != nil {
		t.Fatalf("AppendEffect: %v", err)
	}
	if err := builder.AppendQueryOperation(1, QueryOperationInput{
		Input:       values,
		Outcomes:    []QueryOutcomeInput{{Kind: 1, Values: values}},
		Behavior:    []BehaviorResultInput{{Outcome: 0, Result: 0, Relation: relation}},
		ValuesTypes: []vocabulary.Type{types["any"]}, EffectIndices: []int{0},
		TypeFormals: []vocabulary.Type{types["any"]},
	}); err != nil {
		t.Fatalf("AppendQueryOperation: %v", err)
	}
	if err := builder.AppendQueryOperation(2, QueryOperationInput{
		Outcomes: []QueryOutcomeInput{{}, {}, {}, {}},
	}); err != nil {
		t.Fatalf("AppendQueryOperation opaque: %v", err)
	}
	query, err := builder.FinishQuery()
	if err != nil {
		t.Fatalf("FinishQuery: %v", err)
	}
	if got, ok := query.Input(1); !ok || got != values || query.ValueFormalCount(1) != 1 {
		t.Fatalf("input/value formal = %d/%v/%d", got, ok, query.ValueFormalCount(1))
	}
	if kind, got, ok := query.OutcomeAt(1, 0); !ok || kind != 1 || got != values {
		t.Fatalf("outcome = %d/%d/%v", kind, got, ok)
	}
	if count := query.EffectCount(1); count != 1 {
		t.Fatalf("effect count = %d", count)
	}
	if target, ok := query.EffectTarget(1, 0); !ok || target != 1 {
		t.Fatalf("effect target = %d/%v", target, ok)
	}
	if got, ok := query.TypeDeclaration(types["any"]); !ok || !got.Available() {
		t.Fatal("type declaration was not retained by Core")
	}
}

func TestQueryBuilderConsumesDeclarationSlices(t *testing.T) {
	core := compileTestCore(t, Input{Operations: []OperationInput{{
		Source: 0, Bindings: []vocabulary.BindingSpec{testBinding("copy")}, OutcomeValueSlots: []OutcomeInput{{ValueSlots: 1, Anchor: []byte("copy")}},
	}}})
	anyType, ok := schematype.NewPrimitive(schematype.PrimitiveAny)
	if !ok {
		t.Fatal("primitive Any unavailable")
	}
	builder, err := BeginQuery(core)
	if err != nil {
		t.Fatalf("BeginQuery: %v", err)
	}
	types, err := builder.AppendQueryTypes(map[string][]byte{"any": nil}, map[string]schematype.Type{"any": anyType})
	if err != nil {
		t.Fatalf("AppendQueryTypes: %v", err)
	}
	declaration := []string{"any"}
	values, err := builder.AppendQueryValues(QueryValuesDeclaration{Owner: 1, Types: declaration}, types)
	if err != nil {
		t.Fatalf("AppendQueryValues: %v", err)
	}
	declaration[0] = "missing"
	if err := builder.AppendQueryOperation(1, QueryOperationInput{Input: values, Outcomes: []QueryOutcomeInput{{Values: values}}}); err != nil {
		t.Fatalf("AppendQueryOperation: %v", err)
	}
	if err := builder.AppendQueryOperation(2, QueryOperationInput{Outcomes: []QueryOutcomeInput{{}, {}, {}, {}}}); err != nil {
		t.Fatalf("AppendQueryOperation opaque: %v", err)
	}
	query, err := builder.FinishQuery()
	if err != nil {
		t.Fatalf("FinishQuery: %v", err)
	}
	if got, ok := query.ValuesAt(values, 0); !ok || got != types["any"] {
		t.Fatalf("query retained caller slice = %d/%v", got, ok)
	}
}

// Keep the test relation opaque while exercising the schema.EntryID handoff.
func schemaEntryIDForTest() (id schema.EntryID) {
	return schema.NewEntryID(schema.SurfaceKindStructure, "operation/query")
}
