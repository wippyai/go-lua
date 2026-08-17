package astcodec

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestOccurrenceModelRoundTripsTypedFieldState(t *testing.T) {
	want := Occurrence{
		Type:      "NumberExpr",
		StartLine: 2,
		StartCol:  4,
		EndLine:   2,
		EndCol:    6,
		Fields:    []Field{{Name: "value", State: FieldStateNonZero, Value: 7}},
	}
	encoded, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	var got Occurrence
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("round trip = %#v, want %#v", got, want)
	}
}
