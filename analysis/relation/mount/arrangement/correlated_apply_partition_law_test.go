package arrangement

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/signature"
)

// TestCorrelatedApplyRefusesWithoutPartitionDirectory pins the soundness
// boundary for the cold mount cut. A population driver and child layouts are
// not sufficient: without the binding-owned per-population-row partition
// witness, a global child Complete denominator could fabricate rows from a
// different site. ApplyBinding must therefore remain unavailable until the
// owner directory is bound; no driver-only replay is an admitted fallback.
func TestCorrelatedApplyRefusesWithoutPartitionDirectory(t *testing.T) {
	ownerToken, ok := identity.DeriveContentID("mount/arrangement/partition-law/v1", []byte("owner"))
	if !ok {
		t.Fatal("owner token")
	}
	owner, ok := model.IssueOwnerID(ownerToken)
	if !ok {
		t.Fatal("owner")
	}
	operation, ok := model.IssueOperationID(owner, contentIDForPartitionLaw(t, "operation"))
	if !ok {
		t.Fatal("operation")
	}
	relation, ok := model.IssueRelationID(owner, contentIDForPartitionLaw(t, "relation"))
	if !ok {
		t.Fatal("relation")
	}
	column, ok := model.IssueColumnID(relation, contentIDForPartitionLaw(t, "column"))
	if !ok {
		t.Fatal("column")
	}
	key, ok := model.IssueKeyID(relation, contentIDForPartitionLaw(t, "key"))
	if !ok {
		t.Fatal("key")
	}
	population, ok := model.NewDenominatorRef(relation, key)
	if !ok {
		t.Fatal("population")
	}
	typeID, ok := model.IssueTypeID(owner, contentIDForPartitionLaw(t, "type"))
	if !ok {
		t.Fatal("type")
	}
	correlation := algebra.NewApplyCorrelation(population, column, typeID, [][]model.ColumnID{{column}, {column}})
	if !correlation.Available() {
		t.Fatal("correlation")
	}

	value := ApplyBinding{
		operation:   signature.Identity{Operation: operation, Version: 1},
		deliveries:  []DeliveryBinding{},
		slotSource:  []algebra.SlotSource{},
		output:      algebra.OwnerNamed(),
		outputSlot:  -1,
		childCount:  2,
		correlation: correlation,
	}
	if value.Available() {
		t.Fatal("correlated Apply became available without a partition directory")
	}
	if _, ok := value.Replay(); ok {
		t.Fatal("correlated Apply exposed a driver-only replay")
	}
}

func contentIDForPartitionLaw(t *testing.T, label string) identity.ContentID {
	t.Helper()
	value, ok := identity.DeriveContentID("mount/arrangement/partition-law/v1", []byte(label))
	if !ok {
		t.Fatal("content id")
	}
	return value
}
