package operation

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	"github.com/wippyai/go-lua/analysis/schema"
	schematype "github.com/wippyai/go-lua/analysis/schema/typecontract"
)

func TestQueryHandoffOwnsOperationValuesAndEffects(t *testing.T) {
	core := compileTestCore(t, Input{
		Operations: []OperationInput{
			{
				Source: 0, Bindings: []vocabulary.BindingSpec{testBinding("query")}, InputFormalCount: 1,
				ValuesVars: 1, OutcomeValueSlots: []OutcomeInput{{ValueSlots: 1, Anchor: []byte("query")}},
			},
		},
	})
	anyType, ok := schematype.NewPrimitive(schematype.PrimitiveAny)
	if !ok {
		t.Fatal("primitive Any unavailable")
	}
	relation := schemaEntryIDForTest()
	query, err := CompileQuery(core, QueryInput{
		Types:   []TypeInput{{Handle: 1, Declaration: anyType}},
		Values:  []ValuesInput{{Handle: 1, Owner: 1, Types: []vocabulary.Type{1}, Tail: vocabulary.ValuesClosed}},
		Effects: []EffectInput{{Target: 1, Values: []vocabulary.ValueFormal{0}}},
		Operations: []QueryOperationInput{{
			Input:       1,
			Outcomes:    []QueryOutcomeInput{{Kind: 1, Values: 1}},
			Behavior:    []BehaviorResultInput{{Outcome: 0, Result: 0, Relation: relation}},
			ValuesTypes: []vocabulary.Type{1}, EffectIndices: []int{0},
			TypeFormals: []vocabulary.Type{1},
		}, {Outcomes: []QueryOutcomeInput{{}, {}, {}, {}}}},
	})
	if err != nil {
		t.Fatalf("CompileQuery: %v", err)
	}
	if got, ok := query.Input(1); !ok || got != 1 || query.ValueFormalCount(1) != 1 {
		t.Fatalf("input/value formal = %d/%v/%d", got, ok, query.ValueFormalCount(1))
	}
	if kind, values, ok := query.OutcomeAt(1, 0); !ok || kind != 1 || values != 1 {
		t.Fatalf("outcome = %d/%d/%v", kind, values, ok)
	}
	if count := query.EffectCount(1); count != 1 {
		t.Fatalf("effect count = %d", count)
	}
	if target, ok := query.EffectTarget(1, 0); !ok || target != 1 {
		t.Fatalf("effect target = %d/%v", target, ok)
	}
	if got, ok := query.TypeDeclaration(1); !ok || !got.Available() {
		t.Fatal("type declaration was not retained by Core")
	}
}

func TestQueryHandoffCopiesInputSlices(t *testing.T) {
	core := compileTestCore(t, Input{Operations: []OperationInput{{
		Source: 0, Bindings: []vocabulary.BindingSpec{testBinding("copy")}, OutcomeValueSlots: []OutcomeInput{{ValueSlots: 1, Anchor: []byte("copy")}},
	}}})
	anyType, ok := schematype.NewPrimitive(schematype.PrimitiveAny)
	if !ok {
		t.Fatal("primitive Any unavailable")
	}
	types := []vocabulary.Type{1}
	values := []ValuesInput{{Handle: 1, Owner: 1, Types: types}}
	input := QueryInput{
		Types: []TypeInput{{Handle: 1, Declaration: anyType}}, Values: values,
		Operations: []QueryOperationInput{{Input: 1, Outcomes: []QueryOutcomeInput{{Values: 1}}}, {Outcomes: []QueryOutcomeInput{{}, {}, {}, {}}}},
	}
	query, err := CompileQuery(core, input)
	if err != nil {
		t.Fatalf("CompileQuery: %v", err)
	}
	types[0] = 0
	values[0].Types[0] = 0
	if got, ok := query.ValuesAt(1, 0); !ok || got != 1 {
		t.Fatalf("query retained caller slice = %d/%v", got, ok)
	}
}

// Keep the test relation opaque while exercising the schema.EntryID handoff.
func schemaEntryIDForTest() (id schema.EntryID) {
	return schema.NewEntryID(schema.SurfaceKindStructure, "operation/query")
}
