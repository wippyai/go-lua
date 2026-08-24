package memberdefinition

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/schema/axis/member"
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

// TestMountedCallGeometryIsDeclaredOnceByValue holds the ownership boundary
// that both call dispatch and formal freeze consume. A consumer-specific copy
// would recreate the foreign dense-ordinal coupling this relation exists to
// remove.
func TestMountedCallGeometryIsDeclaredOnceByValue(t *testing.T) {
	source := StorageTransfer()
	relations := make(map[string]definition.Relation, len(source.Relations))
	for _, relation := range source.Relations {
		if strings.Contains(string(relation.Key), "formal-freeze") {
			t.Fatalf("Value relation %q is named for a consumer", relation.Key)
		}
		relations[relation.Name] = relation
	}
	parents, parentsOK := relations["MountedCallParents"]
	members, membersOK := relations["MountedCallActualMembers"]
	callCandidates := member.RelationRef{
		Axis:   schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "call"},
		Member: "call/mounted-call/candidates",
	}
	if !parentsOK || parents.Key != "value/mounted-call/parents" || parents.Subject != "MountedCallActualsCarrier" ||
		len(parents.Correspondences) != 1 || parents.Correspondences[0] != callCandidates {
		t.Fatalf("mounted-call parent declaration = %#v", parents)
	}
	if !membersOK || members.Key != "value/mounted-call/actual-members" || members.Subject != "MountedCallArgumentCarrier" ||
		members.MemberParent.Member != parents.Key || members.MemberOrdinal != "MountedCallActualTagCarrier" {
		t.Fatalf("mounted-call member declaration = %#v", members)
	}

	wantProjections := map[string]string{
		"MountedCallCalleeKey": "value/mounted-call/callee-key",
		"MountedCallActualKey": "value/mounted-call/actual-key",
		"MountedCallActualTag": "value/mounted-call/actual-tag",
	}
	for _, projection := range source.Projections {
		if strings.Contains(string(projection.Key), "formal-freeze") {
			t.Fatalf("Value projection %q is named for a consumer", projection.Key)
		}
		if key, wanted := wantProjections[projection.Name]; wanted {
			if string(projection.Key) != key {
				t.Fatalf("projection %s key = %q, want %q", projection.Name, projection.Key, key)
			}
			delete(wantProjections, projection.Name)
		}
	}
	if len(wantProjections) != 0 {
		t.Fatalf("missing neutral mounted-call projections: %#v", wantProjections)
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
