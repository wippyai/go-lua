package composite

import "testing"

// TestBindingPublishesTheAxisAuthoritiesItsRecordSealed states the publication
// law for the mount record: every per-axis authority the mount phase sealed is
// answered by the binding built from that record, and answered as the same
// authority rather than as a second seal over the same inputs.
//
// Identity is the whole point. A consumer that needs an axis's ascent
// mathematics - the value lattice, the call and effect algebras, the heap and
// pack schemas, the static class set - reads it from the binding that already
// holds it. Comparing the published authority to the record's own by identity
// is what makes a re-seal observable here rather than downstream, where two
// authorities over one Link would disagree about rows no caller can reconcile.
func TestBindingPublishesTheAxisAuthoritiesItsRecordSealed(t *testing.T) {
	record := mountedRecord(t, "axis-publication", "local root = 1\nreturn root\n")
	bound := materializerBinding(t, record)

	heap, heapOK := bound.HeapSchema()
	if !heapOK || heap != record.HeapInput() {
		t.Fatalf("the binding republished the Heap authority: available=%v", heapOK)
	}
	if bound.PackSchema() == nil || bound.PackSchema() != record.PackInput() {
		t.Fatal("the binding republished the Pack authority")
	}
	if bound.EffectAlgebra() == nil || bound.EffectAlgebra() != record.EffectInput() {
		t.Fatal("the binding republished the Effect algebra")
	}
	if bound.CallAlgebra() == nil || bound.CallAlgebra() != record.CallInput() {
		t.Fatal("the binding republished the Call algebra")
	}
	if bound.IndexTopology() == nil || bound.IndexTopology() != record.IndexTopology() {
		t.Fatal("the binding republished the Heap index topology")
	}
	if record.StaticInput() == nil || bound.StaticClasses() == nil || bound.StaticClasses() != record.StaticInput().Classes() {
		t.Fatal("the binding republished the static class set")
	}
	// ValueSchema is the axis this surface already published, so it states the
	// same law and holds the set together.
	if bound.ValueSchema() == nil || bound.ValueSchema() != record.ValueInput() {
		t.Fatal("the binding republished the Value schema")
	}
}

// TestUnmountedRecordPublishesNoAxisAuthority states the refusal half: a
// binding that never received a mounted record answers every axis with its
// unavailable zero rather than a fabricated authority.
func TestUnmountedRecordPublishesNoAxisAuthority(t *testing.T) {
	compilation, ok := Build()
	if !ok {
		t.Fatal("the program schema receipt is unavailable")
	}
	bound, failure := BindProgram(compilation, LinkInputs{})
	if !failure.Available() || bound != nil {
		t.Fatal("the binding transaction admitted an unmounted record")
	}
	var absent *ProgramBinding
	if _, heapOK := absent.HeapSchema(); heapOK {
		t.Fatal("an absent binding answered a Heap authority")
	}
	if absent.PackSchema() != nil || absent.EffectAlgebra() != nil || absent.CallAlgebra() != nil {
		t.Fatal("an absent binding answered an axis authority")
	}
	if absent.StaticClasses() != nil || absent.IndexTopology() != nil {
		t.Fatal("an absent binding answered a derived authority")
	}
}
