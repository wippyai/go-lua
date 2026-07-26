package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
)

func memberCellPartition(t *testing.T, facts ...equation.Fact) equation.Partition {
	t.Helper()
	partition, err := equation.PartitionFromClosuresWithGuards(nil, equation.OutputClosure{Values: facts})
	if err != nil {
		t.Fatalf("building partition: %v", err)
	}
	return partition
}

// TestApplicationRecursesReportsOnlyBodiesOnTheStack pins the coordinate the
// re-entry guard is keyed on: the body being applied, counted for the exact
// span of that private application.
func TestApplicationRecursesReportsOnlyBodiesOnTheStack(t *testing.T) {
	lexical := &lexicalEvaluator{}
	if lexical.applicationRecurses("proto/a") {
		t.Fatalf("no body is on the stack yet")
	}
	outer := lexical.enterApplication("proto/a")
	if !lexical.applicationRecurses("proto/a") {
		t.Fatalf("the applied body is on the stack")
	}
	if lexical.applicationRecurses("proto/b") {
		t.Fatalf("an unrelated body is not on the stack")
	}
	inner := lexical.enterApplication("proto/a")
	inner()
	if !lexical.applicationRecurses("proto/a") {
		t.Fatalf("the outer application still holds the body")
	}
	outer()
	if lexical.applicationRecurses("proto/a") {
		t.Fatalf("the body left the stack with its last application")
	}
	if len(lexical.applying) != 0 {
		t.Fatalf("applying retained %d entries after the stack drained", len(lexical.applying))
	}
}

// TestOpaqueMemberWriteRowCarriesTheStoredCallable pins the unresolved-key
// inventory: the row records both the incomplete inventory and the callables
// the store placed in the container, because no member coordinate names them.
func TestOpaqueMemberWriteRowCarriesTheStoredCallable(t *testing.T) {
	identity := []byte("sealed-table/abc/op-00000001")
	handle := closureHandle{Prototype: "proto/stored", Captures: []string{"path/sym1"}}
	fact, err := heapOpaqueMemberWriteFact(identity, "op-00000002", []closureHandle{handle, handle})
	if err != nil {
		t.Fatalf("publishing the unresolved-key row: %v", err)
	}
	partition := memberCellPartition(t, fact)
	if !heapOpaqueMemberWrite(identity, partition) {
		t.Fatalf("the row must still mark the inventory incomplete")
	}
	callables := heapOpaqueMemberCallables(identity, partition)
	if len(callables) != 1 || callables[0].Prototype != "proto/stored" {
		t.Fatalf("callables = %#v, want the single stored handle", callables)
	}
	rows := heapOpaqueMemberWriteRows(identity, partition)
	if len(rows) != 1 || len(rows[0].Handles) != 1 {
		t.Fatalf("rows = %#v, want one row carrying the stored handle", rows)
	}
}

// TestOpaqueMemberWriteRowWithoutACallableStaysAMarker pins that a store of a
// non-callable value records only the incomplete inventory. Nothing callable
// entered the container, so nothing callable leaves it.
func TestOpaqueMemberWriteRowWithoutACallableStaysAMarker(t *testing.T) {
	identity := []byte("sealed-table/def/op-00000001")
	fact, err := heapOpaqueMemberWriteFact(identity, "op-00000002", nil)
	if err != nil {
		t.Fatalf("publishing the unresolved-key row: %v", err)
	}
	partition := memberCellPartition(t, fact)
	if !heapOpaqueMemberWrite(identity, partition) {
		t.Fatalf("the row must mark the inventory incomplete")
	}
	if callables := heapOpaqueMemberCallables(identity, partition); len(callables) != 0 {
		t.Fatalf("callables = %#v, want none", callables)
	}
}

// TestOpaqueMemberWriteRowRejectsAMalformedHandle pins the fail-closed decode:
// a handle whose captures are not published terms confers no capability, while
// the incomplete-inventory record it travels with survives.
func TestOpaqueMemberWriteRowRejectsAMalformedHandle(t *testing.T) {
	identity := []byte("sealed-table/ghi/op-00000001")
	fact, err := heapOpaqueMemberWriteFact(identity, "op-00000002", []closureHandle{{Prototype: "proto/stored", Captures: []string{"not-a-term"}}})
	if err != nil {
		t.Fatalf("publishing the unresolved-key row: %v", err)
	}
	partition := memberCellPartition(t, fact)
	if !heapOpaqueMemberWrite(identity, partition) {
		t.Fatalf("the row must mark the inventory incomplete")
	}
	if callables := heapOpaqueMemberCallables(identity, partition); len(callables) != 0 {
		t.Fatalf("callables = %#v, want none from an unpublishable handle", callables)
	}
}

// TestContainerCellCallbacksReachesTheUnnamedSlot pins the walk's consumption
// of the unresolved-key inventory beside its published cells: a callable at an
// unnamed slot is reached exactly like one a member coordinate names.
func TestContainerCellCallbacksReachesTheUnnamedSlot(t *testing.T) {
	identity := []byte("sealed-table/jkl/op-00000001")
	named := closureHandle{Prototype: "proto/named", Captures: []string{"path/sym1"}}
	unnamed := closureHandle{Prototype: "proto/unnamed", Captures: []string{"path/sym2"}}
	cell, published, err := memberCellFactWithSource(identity, ".fn", "op-00000002", []byte("scalar/function"), []byte("temp/1"), nil, memberCellPartition(t,
		equation.Fact{Key: "closure/temp/1/op-00000001", Value: []byte(`{"prototype":"proto/named","captures":["path/sym1"]}`)},
	))
	if err != nil || !published {
		t.Fatalf("publishing the named cell: published=%v err=%v", published, err)
	}
	opaque, err := heapOpaqueMemberWriteFact(identity, "op-00000003", []closureHandle{unnamed})
	if err != nil {
		t.Fatalf("publishing the unresolved-key row: %v", err)
	}
	partition := memberCellPartition(t,
		equation.Fact{Key: epochFactPrefix + "path/sym9/op-00000001", Value: []byte("op-00000001")},
		equation.Fact{Key: heapTableIdentityPrefix + "path/sym9/op-00000001", Value: identity},
		cell,
		opaque,
	)
	handles := containerCellCallbacks([]byte("path/sym9"), partition)
	reached := make(map[string]bool, len(handles))
	for _, handle := range handles {
		reached[handle.Prototype] = true
	}
	if !reached[named.Prototype] || !reached[unnamed.Prototype] {
		t.Fatalf("reached = %#v, want both the named and the unnamed slot's callable", reached)
	}
}

// TestContainerCellCallbacksTerminatesOnASelfReachingContainer pins the
// termination coordinate: a cell whose member identity is the container itself
// is a cycle in the heap graph, and the visited identity set closes it after
// one visit.
func TestContainerCellCallbacksTerminatesOnASelfReachingContainer(t *testing.T) {
	identity := []byte("sealed-table/mno/op-00000001")
	source := memberCellPartition(t,
		equation.Fact{Key: "closure/temp/1/op-00000001", Value: []byte(`{"prototype":"proto/self","captures":["path/sym9"]}`)},
	)
	cell, published, err := memberCellFactWithSource(identity, ".self", "op-00000002", []byte("scalar/function"), []byte("temp/1"), identity, source)
	if err != nil || !published {
		t.Fatalf("publishing the self cell: published=%v err=%v", published, err)
	}
	partition := memberCellPartition(t,
		equation.Fact{Key: epochFactPrefix + "path/sym9/op-00000001", Value: []byte("op-00000001")},
		equation.Fact{Key: heapTableIdentityPrefix + "path/sym9/op-00000001", Value: identity},
		cell,
	)
	handles := containerCellCallbacks([]byte("path/sym9"), partition)
	if len(handles) != 1 || handles[0].Prototype != "proto/self" {
		t.Fatalf("walk = %#v, want the single self-referencing callable reported once", handles)
	}
}
