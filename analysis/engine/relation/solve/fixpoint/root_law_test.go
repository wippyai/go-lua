package fixpoint

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/relation/state/database"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
)

func TestRootRefusesUnsealedInputsAndPreservesExplicitMode(t *testing.T) {
	if root, ok := Full(database.Version{}); ok || root.Available() || root.Mode() != RootInvalid {
		t.Fatal("unsealed full root was admitted")
	}
	if root, ok := Later(database.Delta{}); ok || root.Available() || root.Mode() != RootInvalid {
		t.Fatal("unsealed later root was admitted")
	}
	var zero Root
	if zero.Available() || zero.Revision() != 0 || zero.BaseRevision() != 0 || zero.Same(zero) {
		t.Fatal("zero root exposed state or identity")
	}
}

func TestQueueRedeemsOnlyASealedExecutionAndExactMountFence(t *testing.T) {
	if queue, ok := New(arrangement.Execution{}, witness.Mounted{}); ok || !queue.Empty() || queue.Len() != 0 {
		t.Fatal("queue redeemed unsealed execution or mount")
	}
}

func TestWorkIdentityIsDependencyAndRootOnly(t *testing.T) {
	var work Work
	if work.Available() || work.Dependency().Available() || work.Root().Available() {
		t.Fatal("work exposed an incomplete dependency/root pair")
	}
	typeOfWork := reflect.TypeOf(work)
	for index := 0; index < typeOfWork.NumField(); index++ {
		field := typeOfWork.Field(index)
		if field.Name != "dependency" && field.Name != "root" {
			t.Fatalf("work carries legacy field %q", field.Name)
		}
	}
}

func TestQueueCarriesNoLegacyGraphOrWideningPermit(t *testing.T) {
	typeOfQueue := reflect.TypeOf(Queue{})
	for index := 0; index < typeOfQueue.NumField(); index++ {
		field := typeOfQueue.Field(index)
		if field.Name == "graph" || field.Name == "permit" || field.Name == "widening" || field.Name == "order" {
			t.Fatalf("queue carries legacy authority field %q", field.Name)
		}
	}
}
