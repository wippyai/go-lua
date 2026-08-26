package differential_test

import (
	"reflect"
	"testing"
	"unsafe"

	physicalapply "github.com/wippyai/go-lua/analysis/engine/relation/apply"
	differential "github.com/wippyai/go-lua/analysis/engine/relation/apply/differential"
	physicalcomplete "github.com/wippyai/go-lua/analysis/engine/relation/operator/complete"
	physicalinput "github.com/wippyai/go-lua/analysis/engine/relation/operator/input"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/read"
	"github.com/wippyai/go-lua/analysis/engine/relation/tuple"
	relationfixture "github.com/wippyai/go-lua/analysis/engine/testdata/relationfixture"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	"github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

// applyValues redeems the fixture's already-mounted two-scalar Apply.  The
// inert worker returns NoSelection, which is still a live Application with an
// authenticated (empty) proposal lease and exact invocation address.
func applyExtent(t *testing.T, mount byte) (physicalapply.Results, relationfixture.Fixture) {
	t.Helper()
	fixture := relationfixture.New(t, mount)
	mounted := fixture.Mounted()
	left := scalarBatch(t, fixture, true)
	right := scalarBatch(t, fixture, false)
	node, ok := fixture.TwoScalarApplyNode()
	if !ok {
		t.Fatal("two-scalar Apply node")
	}
	plan, ok := node.Apply()
	if !ok || !plan.Available() {
		t.Fatal("two-scalar Apply binding")
	}
	witnesses := make([]binding.DenominatorWitness, len(plan.Deliveries()))
	for index, delivery := range plan.Deliveries() {
		ref := delivery.Requirement().Input().Denominator
		value, witnessOK := mounted.Denominator(ref)
		if !witnessOK || !value.Available() || !value.ValidFor(mounted.RuntimeFence()) || !value.Matches(ref) {
			t.Fatalf("delivery %d denominator witness", index)
		}
		witnesses[index] = value
	}
	results, ok := physicalapply.Execute(plan, mounted, [][]tuple.Batch{{left}, {right}}, fixture.Geometry(), witness.Scope{}, witnesses)
	if !ok || !results.Available() || results.Len() < 2 {
		t.Fatalf("Apply results ok=%t available=%t len=%d", ok, results.Available(), results.Len())
	}
	return results, fixture
}

func applyValues(t *testing.T, mount byte) ([]physicalapply.Application, relationfixture.Fixture) {
	t.Helper()
	results, fixture := applyExtent(t, mount)
	values := make([]physicalapply.Application, results.Len())
	for index := range values {
		var ok bool
		values[index], ok = results.At(index)
		if !ok || !values[index].Available() {
			t.Fatalf("application %d", index)
		}
	}
	return values, fixture
}

func scalarBatch(t *testing.T, fixture relationfixture.Fixture, left bool) tuple.Batch {
	t.Helper()
	var node arrangement.Node
	var ok bool
	if left {
		node, ok = fixture.LeftInputNode()
	} else {
		node, ok = fixture.RightInputNode()
	}
	if !ok || !node.Available() {
		t.Fatal("input node")
	}
	input, ok := node.Input()
	if !ok || !input.Available() {
		t.Fatal("input binding")
	}
	reader, ok := read.Bind(fixture.BothRoot(), input.Values(), fixture.Geometry(), fixture.Scratch())
	if !ok || !reader.Available() {
		t.Fatal("input reader")
	}
	batches, ok := physicalinput.Execute(input, fixture.Mounted(), reader)
	if !ok || len(batches) != 1 || !batches[0].ValidFor(fixture.Mounted()) || batches[0].Len() == 0 {
		t.Fatalf("input batch ok=%t count=%d", ok, len(batches))
	}
	return batches[0]
}

func TestNewRejectsMissingSides(t *testing.T) {
	if value, ok := differential.New(physicalapply.Application{}, physicalapply.Application{}); ok || value.Available() {
		t.Fatal("empty differential was accepted")
	}
}

func TestNewAcceptsBeforeOnlyAfterOnlyAndBoth(t *testing.T) {
	values, _ := applyValues(t, 0xd1)
	afterValues, _ := applyValues(t, 0xd1)
	before, after := values[0], afterValues[0]
	for name, sides := range map[string][2]physicalapply.Application{
		"before-only": {before, physicalapply.Application{}},
		"after-only":  {physicalapply.Application{}, after},
		"both":        {before, after},
	} {
		left, right := sides[0], sides[1]
		t.Run(name, func(t *testing.T) {
			value, ok := differential.New(left, right)
			if !ok || !value.Available() {
				t.Fatalf("differential ok=%t available=%t", ok, value.Available())
			}
			gotBefore, beforeOK := value.Before()
			gotAfter, afterOK := value.After()
			wantBefore := left.Available()
			wantAfter := right.Available()
			if beforeOK != wantBefore || afterOK != wantAfter {
				t.Fatalf("sides before=%t/%t after=%t/%t", beforeOK, wantBefore, afterOK, wantAfter)
			}
			if beforeOK && (!gotBefore.Available() || gotBefore.Lineage() != left.Lineage() || !gotBefore.Invocation().Same(left.Invocation())) {
				t.Fatal("Before was not retained as the authenticated Application")
			}
			if afterOK && (!gotAfter.Available() || gotAfter.Lineage() != right.Lineage() || !gotAfter.Invocation().Same(right.Invocation())) {
				t.Fatal("After was not retained as the authenticated Application")
			}
		})
	}
}

func TestNewRejectsForeignFenceOperationAndInvocation(t *testing.T) {
	values, fixture := applyValues(t, 0xd2)
	foreignFence, _ := applyValues(t, 0xd3)
	if value, ok := differential.New(values[0], foreignFence[0]); ok || value.Available() {
		t.Fatal("foreign runtime fence was accepted")
	}

	// The mixed scalar/Complete specimen is mounted with a different operation
	// while retaining the same runtime fence. Its setup is intentionally
	// separate from the transport; the transport compares only signed fields.
	otherNode, ok := fixture.ScalarCompleteApplyNode()
	if !ok {
		t.Fatal("scalar/Complete Apply node")
	}
	otherPlan, ok := otherNode.Apply()
	if !ok || !otherPlan.Available() {
		t.Fatal("scalar/Complete Apply binding")
	}
	complete, ok := fixture.CompleteBinding()
	if !ok || !complete.Available() {
		t.Fatal("Complete binding")
	}
	left := scalarBatch(t, fixture, true)
	completeBatch, ok := physicalcomplete.Execute(complete, fixture.Mounted(), left, denominatorWitness(t, fixture, complete.Denominator()))
	if !ok || !completeBatch.Available() {
		t.Fatal("Complete batch")
	}
	otherWitnesses := make([]binding.DenominatorWitness, len(otherPlan.Deliveries()))
	for index, delivery := range otherPlan.Deliveries() {
		ref := delivery.Requirement().Input().Denominator
		otherWitnesses[index], ok = fixture.Mounted().Denominator(ref)
		if !ok {
			t.Fatal("other denominator witness")
		}
	}
	otherResults, ok := physicalapply.Execute(otherPlan, fixture.Mounted(), [][]tuple.Batch{{left}, {completeBatch}}, fixture.Geometry(), witness.Scope{}, otherWitnesses)
	if !ok || !otherResults.Available() || otherResults.Len() == 0 {
		t.Fatal("other Apply results")
	}
	other, ok := otherResults.At(0)
	if !ok || !other.Available() {
		t.Fatal("other application")
	}
	if value, ok := differential.New(values[0], other); ok || value.Available() {
		t.Fatal("foreign operation was accepted")
	}

	// Two cartesian members share the exact operation and runtime but carry
	// distinct source rows in their structural invocation addresses.
	if value, ok := differential.New(values[0], values[1]); ok || value.Available() {
		t.Fatal("foreign invocation address was accepted")
	}
}

func denominatorWitness(t *testing.T, fixture relationfixture.Fixture, ref model.DenominatorRef) binding.DenominatorWitness {
	t.Helper()
	value, ok := fixture.Mounted().Denominator(ref)
	if !ok || !value.Available() || !value.ValidFor(fixture.Mounted().RuntimeFence()) || !value.Matches(ref) {
		t.Fatal("denominator witness")
	}
	return value
}

func TestProposalLeaseInvalidationPropagates(t *testing.T) {
	values, _ := applyValues(t, 0xd4)
	afterValues, _ := applyValues(t, 0xd4)
	value, ok := differential.New(values[0], afterValues[0])
	if !ok || !value.Available() {
		t.Fatal("differential")
	}
	batch, ok := values[0].Proposals()
	if !ok || !batch.Available() {
		t.Fatal("proposal lease")
	}
	// ProposalBatch deliberately exposes no reset operation: reset belongs to
	// the producer buffer. This test reaches that producer only to model the
	// hostile lifetime event and verify that Differential keeps the original
	// lease rather than a copied/rebuilt batch.
	bufferPointer := reflect.ValueOf(batch).FieldByName("buffer").Pointer()
	if bufferPointer == 0 || !(*binding.ProposalBuffer)(unsafe.Pointer(bufferPointer)).Reset() {
		t.Fatal("reset proposal lease")
	}
	if value.Available() {
		t.Fatal("differential retained an invalidated proposal lease")
	}
	if _, beforeOK := value.Before(); beforeOK {
		t.Fatal("Before accessor exposed an invalidated transport")
	}
}

func TestSameIdentityDifferentProposalDestinationIsTransportValid(t *testing.T) {
	// The transport intentionally has no destination comparison. The mounted
	// NoSelection applications carry empty proposal batches, so this law uses
	// the exact same signed identity and proves the acceptance boundary; a
	// later output classifier owns destination move semantics.
	values, _ := applyValues(t, 0xd5)
	value, ok := differential.New(values[0], values[0])
	if !ok || !value.Available() || !value.Operation().Available() || !value.Invocation().Same(values[0].Invocation()) {
		t.Fatal("same signed identity was rejected")
	}
}

// rewriteResults is test-only hostile plumbing. apply.Results intentionally
// has no public reorder/subset constructor: production callers must receive
// authored Apply extents from Execute. These laws need malformed/permuted
// extents to prove the zipper does not use ordinal pairing.
func rewriteResults(t *testing.T, source physicalapply.Results, indexes ...int) physicalapply.Results {
	t.Helper()
	values := make([]physicalapply.Application, len(indexes))
	for index, sourceIndex := range indexes {
		value, ok := source.At(sourceIndex)
		if !ok {
			t.Fatalf("source application %d", sourceIndex)
		}
		values[index] = value
	}
	result := source
	field := reflect.ValueOf(&result).Elem().FieldByName("values")
	if !field.IsValid() {
		t.Fatal("apply.Results values field")
	}
	reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().Set(reflect.ValueOf(values))
	if !result.Available() {
		t.Fatal("rewritten Apply results became unavailable")
	}
	return result
}

func TestPairUsesInvocationAddressNotOrdinalAndCanonicalizesPermutation(t *testing.T) {
	before, _ := applyExtent(t, 0xe1)
	after, _ := applyExtent(t, 0xe1)
	canonical, ok := differential.Pair(before, after)
	if !ok || !canonical.Available() || canonical.Len() != before.Len() {
		t.Fatalf("canonical pair ok=%t available=%t len=%d", ok, canonical.Available(), canonical.Len())
	}
	permuted := rewriteResults(t, after, 3, 1, 0, 2)
	zippered, ok := differential.Pair(before, permuted)
	if !ok || !zippered.Available() || zippered.Len() != canonical.Len() {
		t.Fatalf("permuted pair ok=%t available=%t len=%d", ok, zippered.Available(), zippered.Len())
	}
	for index := 0; index < canonical.Len(); index++ {
		left, leftOK := canonical.At(index)
		right, rightOK := zippered.At(index)
		if !leftOK || !rightOK || !left.Invocation().Same(right.Invocation()) {
			t.Fatalf("permutation changed address pairing at %d", index)
		}
		if _, beforeOK := right.Before(); !beforeOK {
			t.Fatalf("permutation lost Before at %d", index)
		}
		if _, afterOK := right.After(); !afterOK {
			t.Fatalf("permutation lost After at %d", index)
		}
	}
}

func TestPairEmitsDeletionAndInsertionSides(t *testing.T) {
	fullBefore, _ := applyExtent(t, 0xe2)
	fullAfter, _ := applyExtent(t, 0xe2)
	subset := rewriteResults(t, fullBefore, 0, 1, 2)
	deletion, ok := differential.Pair(fullBefore, subset)
	if !ok || !deletion.Available() || deletion.Len() != fullBefore.Len() {
		t.Fatalf("deletion pair ok=%t available=%t len=%d", ok, deletion.Available(), deletion.Len())
	}
	deletions := 0
	for index := 0; index < deletion.Len(); index++ {
		entry, entryOK := deletion.At(index)
		if !entryOK {
			t.Fatal("deletion entry")
		}
		if _, afterOK := entry.After(); !afterOK {
			deletions++
		}
	}
	if deletions != 1 {
		t.Fatalf("deletion-only entries=%d, want 1", deletions)
	}

	insertion, ok := differential.Pair(subset, fullAfter)
	if !ok || !insertion.Available() || insertion.Len() != fullAfter.Len() {
		t.Fatalf("insertion pair ok=%t available=%t len=%d", ok, insertion.Available(), insertion.Len())
	}
	insertions := 0
	for index := 0; index < insertion.Len(); index++ {
		entry, entryOK := insertion.At(index)
		if !entryOK {
			t.Fatal("insertion entry")
		}
		if _, beforeOK := entry.Before(); !beforeOK {
			insertions++
		}
	}
	if insertions != 1 {
		t.Fatalf("insertion-only entries=%d, want 1", insertions)
	}
}

func TestPairRejectsDuplicateAddressAndAllowsEmptyExtent(t *testing.T) {
	extent, _ := applyExtent(t, 0xe3)
	duplicate := rewriteResults(t, extent, 0, 0)
	if value, ok := differential.Pair(duplicate, extent); ok || value.Available() {
		t.Fatal("duplicate invocation address was accepted")
	}
	empty := rewriteResults(t, extent)
	value, ok := differential.Pair(empty, physicalapply.Results{})
	if !ok || !value.Available() || value.Len() != 0 || value.Operation() != extent.Operation() {
		t.Fatalf("empty pair ok=%t available=%t len=%d operation-retained=%t", ok, value.Available(), value.Len(), value.Operation() == extent.Operation())
	}
	if value, ok := differential.Pair(physicalapply.Results{}, physicalapply.Results{}); ok || value.Available() {
		t.Fatal("two omitted result extents were accepted")
	}
}
