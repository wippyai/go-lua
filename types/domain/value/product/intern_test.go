package product

import (
	"sync"
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

// TestConcurrentInterningIdentity stresses the interner under concurrency: many
// goroutines constructing the same equal values must all observe one canonical
// node. Run under -race to verify the read-mostly lock is sound.
func TestConcurrentInterningIdentity(t *testing.T) {
	const workers = 64
	results := make([]*node, workers)

	var wg sync.WaitGroup
	wg.Add(workers)
	for i := range workers {
		go func(idx int) {
			defer wg.Done()
			v := withShape(typ.NewUnion(typ.Number, typ.String))
			results[idx] = v.n
		}(i)
	}
	wg.Wait()

	first := results[0]
	for _, n := range results {
		if n != first {
			t.Fatal("concurrent interning of equal values must yield one canonical node")
		}
	}
}

// TestNamedInterfaceNominalInterning is the P7.0 value-domain gate: named
// interfaces must intern by nominal identity (name + method structure). Distinct
// nominal types (different names) must intern to distinct canonical nodes and must
// never be merged structurally; the same nominal type built from two distinct
// instances must intern to one node and project back to a nominally equal
// interface, so a named interface survives FromType -> intern -> Project intact.
func TestNamedInterfaceNominalInterning(t *testing.T) {
	method := []typ.Method{{Name: "recv", Type: typ.Func().Returns(typ.String).Build()}}

	ifaceA := typ.NewInterface("channel.EventChannel", method)
	ifaceB := typ.NewInterface("channel.TimeoutChannel", method)
	// Same declaration as ifaceA (same name + structure) but a distinct instance,
	// modeling a type rebuilt at a different program point.
	ifaceADup := typ.NewInterface("channel.EventChannel", method)

	if ifaceA == ifaceADup {
		t.Fatalf("test precondition: same-decl instances must be distinct pointers")
	}

	va := FromType(ifaceA)
	vb := FromType(ifaceB)
	vaDup := FromType(ifaceADup)

	// Distinct nominal types stay distinct: distinct canonical nodes, not Equal.
	if va.n == vb.n {
		t.Fatal("distinct named interfaces must intern to distinct nodes")
	}
	if Equal(va, vb) {
		t.Fatal("distinct named interfaces must not be Equal")
	}

	// Same nominal type from distinct instances merges to one canonical node.
	if va.n != vaDup.n {
		t.Fatal("same named interface from distinct instances must intern to one node")
	}
	if !Equal(va, vaDup) {
		t.Fatal("same named interface from distinct instances must be Equal")
	}

	// Projecting back recovers a nominally equal interface (name + structure).
	projected := va.Project()
	pi, ok := projected.(*typ.Interface)
	if !ok {
		t.Fatalf("expected projected interface, got %T", projected)
	}
	if !pi.Equals(ifaceA) {
		t.Fatalf("projected interface lost nominal identity: got %v want %v", pi, ifaceA)
	}
}
