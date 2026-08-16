package analysis

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestArtifactResultRootRowsPreserveOrderAndSeparateDuplicateMounts(t *testing.T) {
	artifact := artifactResultLawID(1)
	localRoot := artifactResultLawID(2)
	firstMount, secondMount := artifactResultLawID(3), artifactResultLawID(4)
	firstID, firstOK := mountedResultID("root", firstMount, artifact, localRoot)
	secondID, secondOK := mountedResultID("root", secondMount, artifact, localRoot)
	singleID, singleOK := mountedResultID("root", firstMount, artifact, artifactResultLawID(10))
	if !firstOK || !secondOK || !singleOK || firstID == secondID {
		t.Fatal("duplicate artifact mounts did not receive distinct Result root identities")
	}
	result := &Result{
		source:  artifactResultLawID(5),
		content: artifactResultLawID(6),
		bodies: []resultBody{
			{id: artifactResultLawID(7)},
			{id: artifactResultLawID(8), roots: []resultRoot{{id: singleID, family: keyspace.FamilyBind}}},
			{id: artifactResultLawID(9), roots: []resultRoot{{id: firstID, family: keyspace.FamilyBind}, {id: secondID, family: keyspace.FamilyReturn}}},
		},
		sealed: true,
	}
	zero, zeroOK := result.BodyAt(0)
	if !zeroOK || zero.RootCount() != 0 {
		t.Fatalf("zero-root body = %d/%t, want 0/true", zero.RootCount(), zeroOK)
	}
	single, singleOK := result.BodyAt(1)
	if !singleOK || single.RootCount() != 1 {
		t.Fatalf("single-root body = %d/%t, want 1/true", single.RootCount(), singleOK)
	}
	body, bodyOK := result.BodyAt(2)
	if !bodyOK || body.RootCount() != 2 {
		t.Fatalf("root count = %d/%t, want 2/true", body.RootCount(), bodyOK)
	}
	for index, want := range []struct {
		id     identity.ContentID
		family keyspace.Family
	}{{firstID, keyspace.FamilyBind}, {secondID, keyspace.FamilyReturn}} {
		root, rootOK := body.RootAt(index)
		rootID, rootIDOK := root.ID()
		if !rootOK || !rootIDOK || rootID != want.id || root.Family() != want.family {
			t.Fatalf("RootAt(%d) lost exact ordered row", index)
		}
	}
	if _, ok := body.RootAt(2); ok {
		t.Fatal("out-of-range root was accepted")
	}
}
