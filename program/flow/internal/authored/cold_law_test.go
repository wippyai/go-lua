package authored

import (
	"testing"
	"unsafe"

	"github.com/wippyai/go-lua/program/keyspace"
)

// Cold is an identity snapshot, not a view that keeps the authored Flow
// graph alive. Its representation is therefore exactly the fixed-width
// ContentID value and its query is allocation-free.
func TestColdIsIdentitySnapshot(t *testing.T) {
	if got, want := unsafe.Sizeof(Cold{}), unsafe.Sizeof(keyspace.ContentID{}); got != want {
		t.Fatalf("Cold size = %d, want ContentID size %d", got, want)
	}

	input, _ := flowFixture()
	component := buildFlowForTest(t, input)
	cold := component.Cold()
	if !cold.ContentID().Available() {
		t.Fatalf("Cold ContentID unavailable: %x", cold.ContentID())
	}
	if allocations := testing.AllocsPerRun(1000, func() { _ = cold.ContentID() }); allocations != 0 {
		t.Fatalf("Cold.ContentID allocations = %f, want 0", allocations)
	}
	if got := (Cold{}).ContentID(); got.Available() {
		t.Fatalf("zero Cold exposed identity: %x", got)
	}
}
