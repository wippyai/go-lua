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
		// The fresh-result transition is issued by the ROUTE row, not by the
		// call: a call that publishes at several destinations has one
		// transition per destination, and asking which of them is the call's
		// has no answer. So the row is keyed by the route and its receiver is
		// the route, which is what the freshresult Program's carry names.
		{"transform/value/fresh-result-route", "Route", false},
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
	relationKeys := make(map[string]string, len(source.Relations))
	for _, relation := range source.Relations {
		if strings.Contains(string(relation.Key), "formal-freeze") {
			t.Fatalf("Value relation %q is named for a consumer", relation.Key)
		}
		if _, duplicate := relations[relation.Name]; duplicate {
			t.Fatalf("duplicate Value relation name %q", relation.Name)
		}
		if previous, duplicate := relationKeys[string(relation.Key)]; duplicate {
			t.Fatalf("Value relation key %q names both %s and %s", relation.Key, previous, relation.Name)
		}
		relations[relation.Name] = relation
		relationKeys[string(relation.Key)] = relation.Name
	}
	parents, parentsOK := relations["MountedCallParents"]
	members, membersOK := relations["MountedCallActualMembers"]
	callCandidates := member.RelationRef{
		Axis:   schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: "call"},
		Member: "call/mounted-call/candidates",
	}
	if !parentsOK || parents.Key != "value/mounted-call/parents" || parents.Subject != "MountedCallActualsCarrier" ||
		len(parents.Inputs) != 1 || parents.Inputs[0].Carrier != "CallCoordinateCarrier" ||
		parents.CandidateResolver.Name != "MountedCallActualsForMountedOccurrence" ||
		parents.CandidateOrdinal.Name != "MountedCallActualsOrdinal" || parents.CandidateAt.Name != "MountedCallActualsAt" ||
		len(parents.Correspondences) != 1 || parents.Correspondences[0] != callCandidates {
		t.Fatalf("mounted-call parent declaration = %#v", parents)
	}
	if !membersOK || members.Key != "value/mounted-call/actual-members" || members.Subject != "MountedCallArgumentCarrier" ||
		len(members.Inputs) != 1 || members.Inputs[0].Carrier != "CallCoordinateCarrier" ||
		members.CandidateResolver.Name != "MountedCallArgumentForMountedOccurrence" ||
		members.CandidateOrdinal.Name != "MountedCallArgumentOrdinal" || members.CandidateAt.Name != "MountedCallArgumentAt" ||
		members.MemberParent.Member != parents.Key || members.MemberOrdinal != "MountedCallActualTagCarrier" ||
		members.MemberCount.Name != "MemberCount" || members.MemberAt.Name != "MemberAt" {
		t.Fatalf("mounted-call member declaration = %#v", members)
	}

	wantProjections := map[string]string{
		"MountedCallCalleeKey": "value/mounted-call/callee-key",
		"MountedCallActualKey": "value/mounted-call/actual-key",
		"MountedCallActualTag": "value/mounted-call/actual-tag",
	}
	projectionNames := make(map[string]struct{}, len(source.Projections))
	projectionKeys := make(map[string]string, len(source.Projections))
	for _, projection := range source.Projections {
		if strings.Contains(string(projection.Key), "formal-freeze") {
			t.Fatalf("Value projection %q is named for a consumer", projection.Key)
		}
		if _, duplicate := projectionNames[projection.Name]; duplicate {
			t.Fatalf("duplicate Value projection name %q", projection.Name)
		}
		if previous, duplicate := projectionKeys[string(projection.Key)]; duplicate {
			t.Fatalf("Value projection key %q names both %s and %s", projection.Key, previous, projection.Name)
		}
		projectionNames[projection.Name] = struct{}{}
		projectionKeys[string(projection.Key)] = projection.Name
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
	// The carrier is stated on the OWNER source, which is what makes the
	// directory shared rather than a reading rule's own. Presence is the whole
	// claim: carriers are resolved by name everywhere, so an absolute position
	// among them encodes nothing and pinning one would refuse any unrelated
	// carrier declared ahead of this one.
	arithmeticCarrier := false
	for _, carrier := range source.Carriers {
		if carrier.Name == "BinaryArithmeticCarrier" {
			arithmeticCarrier = true
			break
		}
	}
	if !arithmeticCarrier {
		t.Fatalf("arithmetic carrier was not added to the owner source: %#v", source.Carriers)
	}
}
