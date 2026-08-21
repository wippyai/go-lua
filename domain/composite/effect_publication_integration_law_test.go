package composite

import (
	"testing"
)

// This is the hostile end-to-end boundary currently available from the
// canonical materializer fixture: its Target contract contains no authored
// PublicationEffectSpec. The composition must therefore admit the selected
// Effect rows but issue no inferred publication observation. A positive
// publication fixture remains blocked by the predecessor engine topology
// refusal recorded in the placement journal; this law keeps that missing
// upstream fact from being papered over with a synthetic row.
func TestEffectPublicationIntegrationDoesNotInferMissingDescriptor(t *testing.T) {
	record := mountedRecord(t, "effect-publication-integration", `
local root = { value = 1 }
local function identity(value) return value end
return identity(root)
`)
	bound := materializerBinding(t, record)
	committed, _ := queryCanonicalProgram(t, record, bound)

	observations, observationsOK := bound.EffectPublicationObservations(committed, record.Artifacts)
	if !observationsOK || len(observations) != 0 {
		t.Fatalf("missing authored publication descriptor inferred observations: %d/%t", len(observations), observationsOK)
	}
	solver, failure, sealed := committed.Seal(observations)
	if !sealed || solver == nil {
		t.Fatalf("seal descriptor-free effect inventory: %v", failure)
	}
	if _, observationsOK := bound.EffectPublicationObservations(committed, nil); observationsOK {
		t.Fatal("publication observation enumeration accepted a missing mounted denominator")
	}
}
