package transfer

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	placementdomain "github.com/wippyai/go-lua/domain/placement"
)

// transferLawOperations is deliberately only the Target surface consumed by
// the pure transfer laws. It keeps these tests independent of Link fixtures,
// and makes it difficult for the reducer to acquire an accidental authority
// through a test helper.
type transferLawOperations struct {
	target      vocabulary.Operation
	formalCount int
	input       vocabulary.Values
	tail        vocabulary.ValuesTail
	variable    vocabulary.ValuesVar
	transfer    vocabulary.TransferID
	owner       vocabulary.Operation
	outcomes    []transferLawOutcome
}

type transferLawOutcome struct {
	canonical   uint32
	possibility vocabulary.TransferPossibility
}

func (operations transferLawOperations) ValueFormalCount(target vocabulary.Operation) int {
	if target != operations.target {
		return 0
	}
	return operations.formalCount
}

func (operations transferLawOperations) Input(target vocabulary.Operation) (vocabulary.Values, bool) {
	return operations.input, target == operations.target && operations.input != 0
}

func (operations transferLawOperations) ValuesTail(input vocabulary.Values) (vocabulary.ValuesTail, vocabulary.ValuesVar, bool) {
	return operations.tail, operations.variable, input != 0 && input == operations.input
}

func (operations transferLawOperations) TransferOwner(transfer vocabulary.TransferID) (vocabulary.Operation, bool) {
	return operations.owner, transfer != 0 && transfer == operations.transfer && operations.owner != 0
}

func (operations transferLawOperations) TransferOutcomeCount(target vocabulary.Operation, transferIndex int) int {
	if target != operations.target || transferIndex != 0 || operations.transfer == 0 {
		return 0
	}
	return len(operations.outcomes)
}

func (operations transferLawOperations) TransferOutcomeAt(target vocabulary.Operation, transferIndex, outcome int) (uint32, vocabulary.TransferPossibility, bool) {
	if target != operations.target || transferIndex != 0 || outcome < 0 || outcome >= len(operations.outcomes) {
		return 0, 0, false
	}
	row := operations.outcomes[outcome]
	return row.canonical, row.possibility, true
}

func TestTransferSourceAndEndpointAdmitOnlyExactTargetCoordinates(t *testing.T) {
	operations := transferLawOperations{
		target: vocabulary.Operation(1), formalCount: 2, input: vocabulary.Values(1),
		tail: vocabulary.ValuesVariable, variable: vocabulary.ValuesVar(3),
	}
	tests := []struct {
		name   string
		source vocabulary.InputSource
		want   bool
	}{
		{name: "fixed-formal", source: vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: 1}, want: true},
		{name: "fixed-formal-out-of-range", source: vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal, Ordinal: 2}},
		{name: "open-tail-exact", source: vocabulary.InputSource{Kind: vocabulary.InputSourceValuesVar, Ordinal: 3}, want: true},
		{name: "open-tail-foreign-variable", source: vocabulary.InputSource{Kind: vocabulary.InputSourceValuesVar, Ordinal: 4}},
		{name: "all-inputs-not-a-payload", source: vocabulary.InputSource{Kind: vocabulary.InputSourceAllInputs}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := validTransferSource(operations.target, test.source, operations); got != test.want {
				t.Fatalf("source admission = %t, want %t", got, test.want)
			}
		})
	}
	if validTransferSource(vocabulary.Operation(2), tests[0].source, operations) {
		t.Fatal("foreign operation source was admitted")
	}

	endpoints := []struct {
		name     string
		endpoint vocabulary.TransferEndpoint
		want     bool
	}{
		{name: "input-formal", endpoint: vocabulary.TransferEndpoint{Kind: vocabulary.TransferEndpointInput, Input: 1}, want: true},
		{name: "input-out-of-range", endpoint: vocabulary.TransferEndpoint{Kind: vocabulary.TransferEndpointInput, Input: 2}},
		{name: "external-boundary", endpoint: vocabulary.TransferEndpoint{Kind: vocabulary.TransferEndpointExternal}, want: true},
		{name: "external-with-input-authority", endpoint: vocabulary.TransferEndpoint{Kind: vocabulary.TransferEndpointExternal, Input: 1}},
		{name: "invalid", endpoint: vocabulary.TransferEndpoint{Kind: vocabulary.TransferEndpointInvalid}},
	}
	for _, test := range endpoints {
		t.Run(test.name, func(t *testing.T) {
			if got := validTransferEndpoint(operations.target, test.endpoint, operations); got != test.want {
				t.Fatalf("endpoint admission = %t, want %t", got, test.want)
			}
		})
	}
}

func TestTransferDescriptionsAndOutcomesHaveClosedAlgebras(t *testing.T) {
	for value := vocabulary.TransferIdentityInvalid; value <= vocabulary.TransferIdentityDistinct; value++ {
		want := value != vocabulary.TransferIdentityInvalid
		if got := validTransferDescription(value, vocabulary.TransferCapabilitiesUnspecified); got != want {
			t.Fatalf("identity %d admission = %t, want %t", value, got, want)
		}
	}
	for value := vocabulary.TransferCapabilitiesInvalid; value <= vocabulary.TransferCapabilitiesLoseAll; value++ {
		want := value != vocabulary.TransferCapabilitiesInvalid
		if got := validTransferDescription(vocabulary.TransferIdentityUnspecified, value); got != want {
			t.Fatalf("capabilities %d admission = %t, want %t", value, got, want)
		}
	}
	if validTransferDescription(vocabulary.TransferIdentityDistinct+1, vocabulary.TransferCapabilitiesUnspecified) {
		t.Fatal("unknown identity label was admitted")
	}
	if validTransferDescription(vocabulary.TransferIdentityUnspecified, vocabulary.TransferCapabilitiesLoseAll+1) {
		t.Fatal("unknown capabilities label was admitted")
	}

	operations := transferLawOperations{target: 1, transfer: 9, owner: 1}
	for _, test := range []struct {
		name        string
		outcomes    []transferLawOutcome
		wantDeliver bool
		wantOK      bool
	}{
		{name: "reject-only", outcomes: []transferLawOutcome{{canonical: 0, possibility: vocabulary.TransferMayReject}}, wantOK: true},
		{name: "deliver", outcomes: []transferLawOutcome{{canonical: 0, possibility: vocabulary.TransferMayDeliver}}, wantDeliver: true, wantOK: true},
		{name: "deliver-or-reject", outcomes: []transferLawOutcome{{canonical: 0, possibility: vocabulary.TransferMayDeliver | vocabulary.TransferMayReject}}, wantDeliver: true, wantOK: true},
		{name: "malformed-possibility", outcomes: []transferLawOutcome{{canonical: 0, possibility: vocabulary.TransferPossibility(4)}}},
		{name: "malformed-canonical", outcomes: []transferLawOutcome{{canonical: 1, possibility: vocabulary.TransferMayDeliver}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			operations.outcomes = test.outcomes
			gotDeliver, gotOK := transferMayDeliver(operations, operations.target, operations.transfer, 0)
			if gotDeliver != test.wantDeliver || gotOK != test.wantOK {
				t.Fatalf("outcome reduction = %t/%t, want %t/%t", gotDeliver, gotOK, test.wantDeliver, test.wantOK)
			}
		})
	}
}

func TestTransferMayDeliverRefusesAbsentForeignAndMalformedEvidence(t *testing.T) {
	base := transferLawOperations{
		target:   vocabulary.Operation(1),
		transfer: vocabulary.TransferID(9),
		owner:    vocabulary.Operation(1),
		outcomes: []transferLawOutcome{{canonical: 0, possibility: vocabulary.TransferMayDeliver}},
	}
	tests := []struct {
		name         string
		operations   *transferLawOperations
		target       vocabulary.Operation
		transfer     vocabulary.TransferID
		transferIdx  int
		wantDeliver  bool
		wantAccepted bool
	}{
		{name: "nil-operations", wantAccepted: false},
		{name: "absent-outcomes", operations: func() *transferLawOperations {
			copy := base
			copy.outcomes = nil
			return &copy
		}(), target: base.target, transfer: base.transfer, transferIdx: 0, wantAccepted: false},
		{name: "foreign-target", operations: &base, target: vocabulary.Operation(2), transfer: base.transfer, transferIdx: 0, wantAccepted: false},
		{name: "foreign-transfer", operations: &base, target: base.target, transfer: vocabulary.TransferID(10), transferIdx: 0, wantAccepted: false},
		{name: "negative-transfer-index", operations: &base, target: base.target, transfer: base.transfer, transferIdx: -1, wantAccepted: false},
		{name: "foreign-owner", operations: func() *transferLawOperations {
			copy := base
			copy.owner = vocabulary.Operation(2)
			return &copy
		}(), target: base.target, transfer: base.transfer, transferIdx: 0, wantAccepted: false},
		{name: "malformed-possibility", operations: func() *transferLawOperations {
			copy := base
			copy.outcomes = []transferLawOutcome{{canonical: 0, possibility: vocabulary.TransferPossibility(4)}}
			return &copy
		}(), target: base.target, transfer: base.transfer, transferIdx: 0, wantAccepted: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var operations interface {
				TransferOwner(vocabulary.TransferID) (vocabulary.Operation, bool)
				TransferOutcomeCount(vocabulary.Operation, int) int
				TransferOutcomeAt(vocabulary.Operation, int, int) (uint32, vocabulary.TransferPossibility, bool)
			}
			if test.operations != nil {
				operations = *test.operations
			}
			gotDeliver, gotAccepted := transferMayDeliver(operations, test.target, test.transfer, test.transferIdx)
			if gotDeliver != test.wantDeliver || gotAccepted != test.wantAccepted {
				t.Fatalf("transfer evidence = %t/%t, want %t/%t", gotDeliver, gotAccepted, test.wantDeliver, test.wantAccepted)
			}
		})
	}
}

func TestTransferSendDisplacementIsMonotoneAndNeverDemotes(t *testing.T) {
	for _, current := range []placementdomain.Placement{
		placementdomain.Stack,
		placementdomain.OwnedHeap,
		placementdomain.SharedHeap,
		placementdomain.Unknown,
	} {
		got, ok := placementdomain.DisplaceChecked(current, placementdomain.Send)
		want := placementdomain.SharedHeap
		if current == placementdomain.SharedHeap || current == placementdomain.Unknown {
			want = current
		}
		if !ok || got != want || !got.Covers(current) {
			t.Fatalf("Send displacement from %s = %s/%t, want %s and monotone", current, got, ok, want)
		}
	}
	if _, ok := placementdomain.DisplaceChecked(placementdomain.Bottom, placementdomain.Send); ok {
		t.Fatal("Bottom was converted into a placement instead of refusing")
	}
}

func TestTransferDemandScratchIsSortedDeduplicatedAndBounded(t *testing.T) {
	var scratch demandScratch
	for dense := 23; dense >= 0; dense-- {
		if !scratch.add(demand{dense: dense}) || !scratch.add(demand{dense: dense}) {
			t.Fatalf("demand %d admission", dense)
		}
	}
	if scratch.count != 24 || len(scratch.extra) != 8 {
		t.Fatalf("demand storage count/overflow = %d/%d, want 24/8", scratch.count, len(scratch.extra))
	}
	for index := 0; index < scratch.count; index++ {
		current, currentOK := scratch.at(index)
		if !currentOK || current.dense != index {
			t.Fatalf("demand %d = %#v/%t, want dense %d", index, current, currentOK, index)
		}
	}

	var inline demandScratch
	allocs := testing.AllocsPerRun(100, func() {
		inline = demandScratch{}
		_ = inline.add(demand{dense: 2})
		_ = inline.add(demand{dense: 1})
		_ = inline.add(demand{dense: 2})
	})
	if inline.count != 2 || allocs != 0 {
		t.Fatalf("inline demand reduction = count %d/allocation %f, want 2/0", inline.count, allocs)
	}
}

func TestTransferObservationBufferKeepsInvocationLocalBounds(t *testing.T) {
	var inline [8]actualObservation
	observations, ok := observationBuffer(len(inline), inline[:])
	if !ok || len(observations) != len(inline) || cap(observations) != len(inline) {
		t.Fatalf("inline observation buffer = len %d cap %d/%t", len(observations), cap(observations), ok)
	}
	observations[0].present = true
	if !inline[0].present {
		t.Fatal("inline observation buffer did not use invocation storage")
	}
	wide, wideOK := observationBuffer(len(inline)+1, inline[:])
	if !wideOK || len(wide) != len(inline)+1 || cap(wide) < len(wide) {
		t.Fatalf("wide observation buffer = len %d cap %d/%t", len(wide), cap(wide), wideOK)
	}
	if _, negativeOK := observationBuffer(-1, inline[:]); negativeOK {
		t.Fatal("negative observation count was admitted")
	}
}
