package compiler

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	programpublication "github.com/wippyai/go-lua/analysis/schema/program/publication"
)

func occurrenceCatalogPlainCall(t testing.TB, seed byte, callID, span, member identity.ContentID, argumentOffset uint32) (programschema.Call, programschema.CallArgument) {
	t.Helper()
	values := valuesLawID(seed + 1)
	call, callOK := programschema.NewCall(
		callID, valuesLawID(seed+2), span, valuesLawID(seed+3), values, valuesLawID(seed+4), valuesLawID(seed+5),
		valuesLawID(seed+6), valuesLawID(seed+7), identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, programschema.CallFormPlain,
		0, 0, argumentOffset, argumentOffset+1, 0, 0, false, false,
	)
	argument, argumentOK := programschema.NewCallArgument(valuesLawID(seed+8), callID, values, member, span, 0)
	if !callOK || !argumentOK {
		t.Fatal("call-span validation fixture was not available")
	}
	return call, argument
}

// TestOccurrenceCatalogCallSpanValidationPinsLaterConflict keeps the catalog
// pass order: repeated equal (CallID, MemberID) pairs are admissible, and the
// first distinct pair for that SpanID fails at its later Call row.
func TestOccurrenceCatalogCallSpanValidationPinsLaterConflict(t *testing.T) {
	span, callID, member := valuesLawID(101), valuesLawID(102), valuesLawID(103)
	first, firstArgument := occurrenceCatalogPlainCall(t, 110, callID, span, member, 0)
	repeated, repeatedArgument := occurrenceCatalogPlainCall(t, 120, callID, span, member, 1)
	accepted := compiler{publication: programpublication.Publication{Calls: []programschema.Call{first, repeated}, CallArguments: []programschema.CallArgument{firstArgument, repeatedArgument}}}
	if failure := accepted.validateOccurrenceCausalInputsFailure(); failure.Available() {
		t.Fatalf("equal call-span pair failed: %+v", failure)
	}

	conflicting, conflictingArgument := occurrenceCatalogPlainCall(t, 130, valuesLawID(104), span, member, 2)
	rejected := compiler{publication: programpublication.Publication{Calls: []programschema.Call{first, repeated, conflicting}, CallArguments: []programschema.CallArgument{firstArgument, repeatedArgument, conflictingArgument}}}
	failure := rejected.validateOccurrenceCausalInputsFailure()
	row, rowKnown := failure.Row()
	if !failure.Available() || failure.Reason() != CompileReasonOccurrenceCall || !rowKnown || row != 2 {
		t.Fatalf("later conflicting call-span pair failure=%+v row=%d/%t, want occurrence call row 2", failure, row, rowKnown)
	}
}
