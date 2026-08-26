package issuance

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
)

// routeIdentityParameter is the parameter a stage carries when it stands on a
// route rather than in its point's chain.
func routeIdentityParameter() DataType { return IdentityType(TypeRouteIdentity) }

// TestRoutedEdgeRequiresAStageThatCarriesItsRoute states the closed-vocabulary
// half of routed placement: the routed edge source is only meaningful on a
// stage whose identity names the route.
//
// A stage without the route in its identity is one stage per point and axis, so
// two routes reaching that point would share it and compose what each proves
// separately. Admitting the routed edge there would silently produce exactly
// the collapse the route parameter exists to prevent, so the seal refuses it
// rather than leaving it to the host to notice.
func TestRoutedEdgeRequiresAStageThatCarriesItsRoute(t *testing.T) {
	entries := canonicalEntries(t)
	for _, entry := range entries {
		if entry.key != "stage/local" {
			continue
		}
		entry.edges = []StageEdge{{Source: StageEdgeSourceRoute, Transport: StageTransportAll, Framing: "issuance/routed-transfer/v1"}}
	}
	if _, failure := sealTable(t, entries...); !failure.Available() {
		t.Fatal("a routed edge on a stage that carries no route was admitted")
	}
}

// TestRoutedEdgeIsAdmittedOnAStageThatCarriesItsRoute is the positive half: the
// same edge on a stage that does name its route seals. Without this the law
// above would be satisfied by a vocabulary that refuses routed edges outright.
func TestRoutedEdgeIsAdmittedOnAStageThatCarriesItsRoute(t *testing.T) {
	entries := canonicalEntries(t)
	for _, entry := range entries {
		if entry.key != "stage/local" {
			continue
		}
		entry.parameters = []DataType{
			{Value: ValuePointRange, Name: TypePoint, Cardinality: CardinalityMany},
			routeIdentityParameter(),
		}
		entry.identity = []uint16{1, 2}
		entry.edges = []StageEdge{{Source: StageEdgeSourceRoute, Transport: StageTransportAll, Framing: "issuance/routed-transfer/v1"}}
	}
	entries = removeFormEntries(entries)
	if _, failure := sealTable(t, entries...); failure.Available() {
		t.Fatalf("a routed edge on a stage that carries its route was refused: %+v", failure)
	}
}

// TestRoutedInputIsRefusedOnAStageThatCarriesNoRoute states the same law from
// the input side. A routed input reads the state its stage's route delivers, so
// a stage with no route has nothing for it to name.
func TestRoutedInputIsRefusedOnAStageThatCarriesNoRoute(t *testing.T) {
	entries := canonicalEntries(t)
	entries = append(entries, mustEntry(t, Spec{
		Key: "input/route", Kind: KindInput, Ordinal: 2,
		Input: InputPredecessor, InputSource: InputSourceRoute, Selection: InputSelectionRoute,
	}))
	for _, entry := range entries {
		if entry.key != "form/local" {
			continue
		}
		entry.program[3] = Instruction{Op: OpInput, Out: 4, Ref: "input/route"}
	}
	if _, failure := sealTable(t, entries...); !failure.Available() {
		t.Fatal("a routed input paired with a stage that carries no route was admitted")
	}
}

// removeFormEntries drops the form that requests the mutated stage. The stage
// laws above state a property of the stage declaration itself; the form's own
// argument contract is stated by its own laws and would otherwise refuse first
// for an unrelated reason.
func removeFormEntries(entries []*Entry) []*Entry {
	kept := make([]*Entry, 0, len(entries))
	for _, entry := range entries {
		if entry.kind == KindForm {
			continue
		}
		kept = append(kept, entry)
	}
	return kept
}

var _ = schema.Key("")
