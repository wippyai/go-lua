package memberdefinition

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/axis/member/definition"
	"github.com/wippyai/go-lua/domain/value"
)

// These assignments are intentional compile-time laws: Value's two carry
// symbols must be directly callable with their declared candidate and prior
// fact types. This rules out owner-schema key-taking methods masquerading as
// candidate transforms.
var (
	_ func(*value.AllocationResult, value.Value) (value.Value, bool) = (*value.AllocationResult).Age
	_ func(value.FreshResultCall, value.Value) (value.Value, bool)   = value.FreshResultCall.Age
)

func TestCarryTransformRowsNameDirectCandidateMethods(t *testing.T) {
	rows := StorageTransfer().CarryTransforms
	if len(rows) != 2 {
		t.Fatalf("value carry transforms = %d, want 2", len(rows))
	}
	want := []struct {
		key, receiver string
		pointer       bool
	}{
		{"transform/value/allocation", "AllocationResult", true},
		{"transform/value/callresult-freshresult", "FreshResultCall", false},
	}
	for index, row := range rows {
		if string(row.Key) != want[index].key {
			t.Fatalf("transform[%d] key = %q, want %q", index, row.Key, want[index].key)
		}
		if row.Implementation.Name != "Age" || row.Implementation.Receiver.Name != want[index].receiver || row.Implementation.ReceiverPointer != want[index].pointer {
			t.Fatalf("transform[%d] implementation = %#v, want %s.Age", index, row.Implementation, want[index].receiver)
		}
	}
}

func TestStorageTransferPublishesTheSharedArithmeticCandidateDirectory(t *testing.T) {
	source := StorageTransfer()
	var candidate *definition.Relation
	for index := range source.Relations {
		if source.Relations[index].Name == "BinaryArithmeticCandidates" {
			candidate = &source.Relations[index]
			break
		}
	}
	if candidate == nil {
		t.Fatal("value source does not publish BinaryArithmeticCandidates")
	}
	if candidate.Key != "value/binary-arithmetic/candidates" || candidate.Subject != "BinaryArithmeticCarrier" {
		t.Fatalf("arithmetic candidate = %#v", *candidate)
	}
	if candidate.CandidateResolver.Name != "BinaryArithmeticForArtifactOccurrence" ||
		candidate.CandidateOrdinal.Name != "BinaryArithmeticOrdinal" ||
		candidate.CandidateAt.Name != "BinaryArithmeticAt" {
		t.Fatalf("arithmetic directory methods = %#v", *candidate)
	}
	if len(source.Carriers) < 4 || source.Carriers[3].Name != "BinaryArithmeticCarrier" {
		t.Fatalf("arithmetic carrier was not added to the owner source: %#v", source.Carriers)
	}
}
