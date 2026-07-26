package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/factkey"
)

func TestDeclaredRevokerInvalidatesLengthFloorFamilyRead(t *testing.T) {
	identity := []byte("sealed-table/license")
	subject := factkey.TaggedIdentityPart(identity)
	floor := func(point string) equation.Fact {
		return equation.Fact{
			Key: factkey.BuildKey(
				factkey.HeapLengthFloor, []factkey.Part{subject}, point,
			).String(),
			Value: []byte("1"),
		}
	}
	revoker := func(point string) equation.Fact {
		return equation.Fact{
			Key: factkey.BuildKey(
				factkey.HeapIndexRevoke, []factkey.Part{subject}, point,
			).String(),
			Value: []byte("revoked"),
		}
	}

	if got := subjectLengthFloorProven(subject, joinTestPartition(t, nil, floor("op-00000002"))); got != 1 {
		t.Fatalf("unrevoked length floor = %d, want 1", got)
	}
	if got := subjectLengthFloorProven(subject, joinTestPartition(t, nil, revoker("op-00000001"), floor("op-00000002"))); got != 1 {
		t.Fatal("a revoker before the proof invalidated a later publication")
	}
	for _, point := range []string{"op-00000002", "op-00000003"} {
		if got := subjectLengthFloorProven(subject, joinTestPartition(t, nil, floor("op-00000002"), revoker(point))); got != 0 {
			t.Fatalf("declared index revoker at %s left length floor %d, want 0", point, got)
		}
	}
}
