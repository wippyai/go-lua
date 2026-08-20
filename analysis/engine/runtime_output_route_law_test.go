package engine

import (
	"reflect"
	"testing"
)

// TestRouteOutputBatchRetainsNoDomainCallback prevents route publication from
// regaining a hidden second semantic phase after Routed returns.
func TestRouteOutputBatchRetainsNoDomainCallback(t *testing.T) {
	typeOfBatch := reflect.TypeOf(routeOutputBatch[uint64]{})
	want := []string{"read", "selectionID", "refs", "values"}
	if typeOfBatch.NumField() != len(want) {
		t.Fatalf("route output batch fields = %d, want the one token plus paired vectors (%d)", typeOfBatch.NumField(), len(want))
	}
	for index := 0; index < typeOfBatch.NumField(); index++ {
		field := typeOfBatch.Field(index)
		if field.Name != want[index] {
			t.Fatalf("route output batch field %d = %q, want %q", index, field.Name, want[index])
		}
		if field.Type.Kind() == reflect.Func {
			t.Fatalf("route output batch retained callback field %q", field.Name)
		}
	}
}
