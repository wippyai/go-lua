package callpayload

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/callboundary"
)

func TestCallOutcomeEmptyAndPostReturnEvidenceCoverEveryLane(t *testing.T) {
	postReturnEvidenceFields := map[string]struct{}{
		"NormalReturnFacts":          {},
		"HeapTableObjects":           {},
		"Placements":                 {},
		"ParamPathRefinements":       {},
		"ParamLengthFloors":          {},
		"ParamPathInvalidations":     {},
		"ParamConditions":            {},
		"ParamPathRelations":         {},
		"ReturnConditionRefinements": {},
		"ReturnPresenceRelations":    {},
	}
	nonPostReturnFields := map[string]struct{}{
		"Results":             {},
		"PostReturnAuthority": {},
		"ParamObligations":    {},
		"ParamExposures":      {},
	}

	typ := reflect.TypeOf(CallOutcome{})
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		outcome := callOutcomeWithOneField(t, field.Name)
		if outcome.Empty() {
			t.Fatalf("CallOutcome.Empty ignored field %s", field.Name)
		}

		_, wantPostReturn := postReturnEvidenceFields[field.Name]
		_, wantNonPostReturn := nonPostReturnFields[field.Name]
		if !wantPostReturn && !wantNonPostReturn {
			t.Fatalf("CallOutcome field %s is not categorized by the boundary evidence tests", field.Name)
		}
		if got := outcome.HasPostReturnEvidence(); got != wantPostReturn {
			t.Fatalf("CallOutcome.HasPostReturnEvidence(%s) = %v, want %v", field.Name, got, wantPostReturn)
		}
	}
}

func callOutcomeWithOneField(t *testing.T, fieldName string) CallOutcome {
	t.Helper()
	out := CallOutcome{}
	if fieldName == "NormalReturnFacts" {
		out.NormalReturnFacts = callboundary.NormalReturnFacts{
			PathRefinements: []callboundary.PathValueFact{{}},
		}
		return out
	}
	value := reflect.ValueOf(&out).Elem().FieldByName(fieldName)
	if !value.IsValid() {
		t.Fatalf("CallOutcome.%s does not exist", fieldName)
	}
	switch value.Kind() {
	case reflect.Bool:
		value.SetBool(true)
	case reflect.Map:
		m := reflect.MakeMapWithSize(value.Type(), 1)
		m.SetMapIndex(reflect.Zero(value.Type().Key()), reflect.Zero(value.Type().Elem()))
		value.Set(m)
	case reflect.Slice:
		value.Set(reflect.Append(value, reflect.Zero(value.Type().Elem())))
	default:
		t.Fatalf("CallOutcome.%s has unsupported test kind %s", fieldName, value.Kind())
	}
	return out
}
