package analysis

import (
	"crypto/sha256"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/program/keyspace"
)

func canonicalLawID(seed byte) keyspace.ContentID {
	return keyspace.ContentID(sha256.Sum256([]byte{seed}))
}

func TestCanonicalPortableSchedulePermutationLaw(t *testing.T) {
	linkID := canonicalLawID(1)
	leftID, rightID := canonicalLawID(2), canonicalLawID(3)

	firstOrder := []keyspace.ContentID{leftID, rightID}
	secondOrder := []keyspace.ContentID{rightID, leftID}
	firstKeys := make([]engine.SemanticKey, len(firstOrder))
	secondKeys := make([]engine.SemanticKey, len(secondOrder))
	for index, id := range firstOrder {
		key, ok := canonicalScheduleKey(linkID, "value-source", id)
		if !ok {
			t.Fatal("portable schedule identity rejected an available owner ID")
		}
		firstKeys[index] = key
	}
	for index, id := range secondOrder {
		key, ok := canonicalScheduleKey(linkID, "value-source", id)
		if !ok {
			t.Fatal("portable schedule identity rejected an available owner ID")
		}
		secondKeys[index] = key
	}
	if firstKeys[0] != secondKeys[1] || firstKeys[1] != secondKeys[0] {
		t.Fatal("row permutation changed the identity attached to an owner-issued row")
	}
	if firstKeys[0] == firstKeys[1] {
		t.Fatal("distinct owner-issued rows collided within one schedule role")
	}
	if firstKeys[0] == (engine.SemanticKey{}) {
		t.Fatal("portable schedule identity rejected an available owner ID")
	}
}

func TestCanonicalPortableScheduleResealLaw(t *testing.T) {
	linkID := canonicalLawID(4)
	rowID := canonicalLawID(5)
	first, firstOK := canonicalScheduleKey(linkID, "pack-source", rowID)
	second, secondOK := canonicalScheduleKey(linkID, "pack-source", rowID)
	changedLink, changedLinkOK := canonicalScheduleKey(canonicalLawID(6), "pack-source", rowID)
	if !firstOK || !secondOK || !changedLinkOK {
		t.Fatal("portable schedule identity rejected an available reseal input")
	}
	if first != second {
		t.Fatal("equivalent reseal changed the portable schedule identity")
	}
	if first == changedLink {
		t.Fatal("changed Link owner identity did not change the schedule identity")
	}
}

func TestCanonicalBoundaryPortableCollisionLaw(t *testing.T) {
	linkID := canonicalLawID(7)
	instanceID, otherInstanceID := canonicalLawID(8), canonicalLawID(9)
	instance, instanceOK := canonicalScheduleKey(linkID, "raw-get", instanceID)
	otherInstance, otherInstanceOK := canonicalScheduleKey(linkID, "raw-get", otherInstanceID)
	portSemantic, portOK := analysisSemanticKey(linkID, "factor/value")
	otherPortSemantic, otherPortOK := analysisSemanticKey(linkID, "factor/call")
	if !instanceOK || !otherInstanceOK || !portOK || !otherPortOK {
		t.Fatal("boundary law setup failed")
	}
	first, firstOK := canonicalBoundaryKey(linkID, instance, portSemantic)
	repeated, repeatedOK := canonicalBoundaryKey(linkID, instance, portSemantic)
	otherPort, otherPortOK := canonicalBoundaryKey(linkID, instance, otherPortSemantic)
	otherRow, otherRowOK := canonicalBoundaryKey(linkID, otherInstance, portSemantic)
	if !firstOK || !repeatedOK || !otherPortOK || !otherRowOK {
		t.Fatal("boundary identity rejected available instance/port semantics")
	}
	if first != repeated {
		t.Fatal("same instance/port pair did not reproduce its boundary identity")
	}
	if first == otherPort || first == otherRow {
		t.Fatal("distinct boundary port or instance identities collided")
	}
	if valueRole, ok := canonicalScheduleKey(linkID, "value-source", instanceID); !ok || valueRole == instance {
		t.Fatal("role namespace failed to separate equal operand IDs")
	}
}
