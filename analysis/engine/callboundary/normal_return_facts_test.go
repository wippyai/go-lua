package callboundary

import (
	"reflect"
	"testing"
)

func TestNormalReturnFactsEmptyAndAppendCoverEveryLane(t *testing.T) {
	typ := reflect.TypeOf(NormalReturnFacts{})
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if field.Type.Kind() != reflect.Slice {
			continue
		}
		fact := normalReturnFactsWithOneElement(t, field.Name)
		if fact.Empty() {
			t.Fatalf("NormalReturnFacts.Empty ignored lane %s", field.Name)
		}
		appended := NormalReturnFacts{}.Append(fact)
		got := reflect.ValueOf(appended).FieldByName(field.Name)
		if got.Len() != 1 {
			t.Fatalf("NormalReturnFacts.Append ignored lane %s: len=%d", field.Name, got.Len())
		}
	}
}

func normalReturnFactsWithOneElement(t *testing.T, fieldName string) NormalReturnFacts {
	t.Helper()
	out := NormalReturnFacts{}
	value := reflect.ValueOf(&out).Elem().FieldByName(fieldName)
	if !value.IsValid() || value.Kind() != reflect.Slice {
		t.Fatalf("NormalReturnFacts.%s is not a slice lane", fieldName)
	}
	elem := reflect.New(value.Type().Elem()).Elem()
	value.Set(reflect.Append(value, elem))
	return out
}
