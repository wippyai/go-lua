package protocol

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
)

// demandInput declares one operation that carries every state obligation kind
// the protocol surface can state about a callable: it acquires the resource at
// a fixed result slot, requires an input state without moving it, moves that
// same input on its normal arm, and hands it to a callback holder. The opaque
// operation carries the derived escape every protocol publishes.
func demandInput() Input {
	input := requirementInput()
	input.Protocols[0].Escapes = []vocabulary.EscapeSpec{{
		Operation: 1,
		Input:     vocabulary.InputSource{Kind: vocabulary.InputSourceValueFormal},
	}}
	return input
}

// The callable-requirement authority answers, for one mounted callable
// operation, every typestate obligation it declares - across every protocol
// that states one - by dense ordinal. A consumer holding an operation handle
// reads that relation directly; it never scans the protocol table, and the
// answer costs it no allocation.
func TestCallableDemandAnswersTheOperationsWholeObligation(t *testing.T) {
	table, err := Compile(demandInput())
	if err != nil {
		t.Fatal(err)
	}
	protocol, ok := table.ProtocolAt(0)
	if !ok {
		t.Fatal("sealed table has no protocol handle")
	}
	const declaring vocabulary.Operation = 1
	count := table.DemandCount(declaring)
	if count != 4 {
		t.Fatalf("demand count = %d, want the acquisition, requirement, transition, and escape", count)
	}
	kinds := make(map[DemandKind]int, count)
	for index := 0; index < count; index++ {
		demand, found := table.DemandAt(declaring, index)
		if !found {
			t.Fatalf("demand %d is unavailable", index)
		}
		if demand.Protocol != protocol {
			t.Fatalf("demand %d protocol = %d, want %d", index, demand.Protocol, protocol)
		}
		kinds[demand.Kind]++
	}
	for _, kind := range []DemandKind{DemandAcquisition, DemandRequirement, DemandTransition, DemandEscape} {
		if kinds[kind] != 1 {
			t.Fatalf("demand kind %d appears %d times, want exactly one", kind, kinds[kind])
		}
	}
}

// A demand addresses the protocol-local row that states it, so the payload is
// read from the one relation that owns it rather than copied into a second
// table that could drift from it.
func TestCallableDemandAddressesTheOwningProtocolRow(t *testing.T) {
	table, err := Compile(demandInput())
	if err != nil {
		t.Fatal(err)
	}
	const declaring vocabulary.Operation = 1
	var seen int
	for index := 0; index < table.DemandCount(declaring); index++ {
		demand, found := table.DemandAt(declaring, index)
		if !found {
			t.Fatalf("demand %d is unavailable", index)
		}
		switch demand.Kind {
		case DemandRequirement:
			operation, input, state, rowOK := table.ProtocolRequirementAt(demand.Protocol, demand.Row)
			if !rowOK || operation != declaring || input.Kind != vocabulary.InputSourceValueFormal {
				t.Fatalf("requirement row %d = %d/%+v/%v", demand.Row, operation, input, rowOK)
			}
			if name, nameOK := table.StateName(demand.Protocol, state); !nameOK || name != "open" {
				t.Fatalf("requirement state = %q/%v, want open", name, nameOK)
			}
			seen++
		case DemandTransition:
			operation, kind, ordinal, from, rowOK := table.TransitionAt(demand.Protocol, demand.Row)
			if !rowOK || operation != declaring || kind != vocabulary.InputSourceValueFormal || ordinal != 0 {
				t.Fatalf("transition row %d = %d/%d/%d/%v", demand.Row, operation, kind, ordinal, rowOK)
			}
			if name, nameOK := table.StateName(demand.Protocol, from); !nameOK || name != "open" {
				t.Fatalf("transition source state = %q/%v, want open", name, nameOK)
			}
			seen++
		case DemandAcquisition:
			operation, _, result, state, rowOK := table.ProtocolAcquisitionAt(demand.Protocol, demand.Row)
			if !rowOK || operation != declaring || result != 0 {
				t.Fatalf("acquisition row %d = %d/%d/%v", demand.Row, operation, result, rowOK)
			}
			if name, nameOK := table.StateName(demand.Protocol, state); !nameOK || name != "open" {
				t.Fatalf("acquired state = %q/%v, want open", name, nameOK)
			}
			seen++
		case DemandEscape:
			operation, kind, _, rowOK := table.EscapeAt(demand.Protocol, demand.Row)
			if !rowOK || operation != declaring || kind != vocabulary.InputSourceValueFormal {
				t.Fatalf("escape row %d = %d/%d/%v", demand.Row, operation, kind, rowOK)
			}
			seen++
		default:
			t.Fatalf("demand %d has no declared kind", index)
		}
	}
	if seen != 4 {
		t.Fatalf("addressed %d owning rows, want 4", seen)
	}
}

// The derived opaque escape is part of the authority. Opaque dispatch escapes
// every input of every protocol, and a consumer that reads obligations by
// operation must see that escape at the opaque operation rather than having to
// know the derivation.
func TestOpaqueOperationCarriesTheDerivedEscapeDemand(t *testing.T) {
	table, err := Compile(demandInput())
	if err != nil {
		t.Fatal(err)
	}
	if table.opaque == 0 {
		t.Fatal("sealed table has no opaque operation")
	}
	count := table.DemandCount(table.opaque)
	if count != 1 {
		t.Fatalf("opaque demand count = %d, want the one derived escape", count)
	}
	demand, ok := table.DemandAt(table.opaque, 0)
	if !ok || demand.Kind != DemandEscape {
		t.Fatalf("opaque demand = %+v/%v, want an escape", demand, ok)
	}
	operation, kind, _, rowOK := table.EscapeAt(demand.Protocol, demand.Row)
	if !rowOK || operation != table.opaque || kind != vocabulary.InputSourceAllInputs {
		t.Fatalf("opaque escape row = %d/%d/%v, want the all-inputs escape", operation, kind, rowOK)
	}
}

// An operation that declares nothing carries no obligation, and an operation
// outside the sealed geometry is not an operation. Neither answers a row.
func TestCallableDemandRefusesOperationsOutsideTheAuthority(t *testing.T) {
	table, err := Compile(demandInput())
	if err != nil {
		t.Fatal(err)
	}
	if count := table.DemandCount(0); count != 0 {
		t.Fatalf("invalid operation carries %d demands", count)
	}
	if _, ok := table.DemandAt(0, 0); ok {
		t.Fatal("invalid operation answered a demand row")
	}
	beyond := vocabulary.Operation(table.opaque + 1)
	if count := table.DemandCount(beyond); count != 0 {
		t.Fatalf("operation outside the geometry carries %d demands", count)
	}
	if _, ok := table.DemandAt(1, table.DemandCount(1)); ok {
		t.Fatal("a demand index past the operation's own relation answered a row")
	}
}

// A program that declares no protocol declares no obligation, so the authority
// is an empty sealed relation rather than an absent one.
func TestCallableDemandIsSealedEvenWithoutProtocols(t *testing.T) {
	input := demandInput()
	input.Protocols = nil
	table, err := Compile(input)
	if err != nil {
		t.Fatal(err)
	}
	if count := table.DemandCount(1); count != 0 {
		t.Fatalf("protocol-free program carries %d demands", count)
	}
	if table.demands.Len() != 0 {
		t.Fatalf("protocol-free program sealed %d demand rows", table.demands.Len())
	}
}

// The authority is a projection of rows the identity already covers. Reading it
// therefore cannot change what a Contract is addressed by: the encoded
// contribution is byte-identical with and without a consumer that reads it.
func TestCallableDemandAddsNoIdentityBearingRow(t *testing.T) {
	table, err := Compile(demandInput())
	if err != nil {
		t.Fatal(err)
	}
	before := table.CountRows()
	for index := 0; index < table.DemandCount(1); index++ {
		if _, ok := table.DemandAt(1, index); !ok {
			t.Fatalf("demand %d is unavailable", index)
		}
	}
	after := table.CountRows()
	if before.Count() != after.Count() {
		t.Fatalf("row census changed from %d to %d entries", before.Count(), after.Count())
	}
	for index := 0; index < before.Count(); index++ {
		row, rowOK := before.At(index)
		if !rowOK {
			t.Fatalf("census row %d is unavailable", index)
		}
		other, ok := after.Value(row.ID())
		if !ok || other != row.Count() {
			t.Fatalf("relation %v = %d/%v, want the unchanged %d", row.ID(), other, ok, row.Count())
		}
	}
}

// Zero allocations: the authority exists so a per-call-occurrence consumer can
// ask an operation for its obligations inside a solve without allocating.
func TestCallableDemandReadsWithoutAllocating(t *testing.T) {
	table, err := Compile(demandInput())
	if err != nil {
		t.Fatal(err)
	}
	allocations := testing.AllocsPerRun(64, func() {
		for index := 0; index < table.DemandCount(1); index++ {
			if _, ok := table.DemandAt(1, index); !ok {
				panic("demand row unavailable")
			}
		}
	})
	if allocations != 0 {
		t.Fatalf("reading one operation's obligations allocated %.0f times", allocations)
	}
}
