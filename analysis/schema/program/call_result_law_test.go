package programschema

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

func callResultLawID(seed byte) identity.ContentID {
	var id identity.ContentID
	for index := range id {
		id[index] = seed + byte(index)
	}
	return id
}

func TestCallResultKeepsValueAndValuesFormsDisjoint(t *testing.T) {
	call, values, value, tail := callResultLawID(1), callResultLawID(33), callResultLawID(65), callResultLawID(97)
	fixed, fixedOK := NewCallResult(call, values, value, identity.ContentID{}, 3, CallResultValue)
	open, openOK := NewCallResult(callResultLawID(2), values, identity.ContentID{}, tail, 0, CallResultValues)
	if !fixedOK || !fixed.Available() || !openOK || !open.Available() {
		t.Fatal("valid call-result forms were not admitted")
	}
	if fixed.ID() != call || fixed.CallID() != call || fixed.ValuesID() != values || fixed.Form() != CallResultValue {
		t.Fatal("fixed call-result lost its parent-issued coordinates")
	}
	if got, ok := fixed.ValueID(); !ok || got != value {
		t.Fatal("fixed call-result did not expose its exact Value member")
	}
	if _, ok := fixed.ValuesTailID(); ok {
		t.Fatal("fixed call-result exposed an open Values tail")
	}
	if position, ok := fixed.Position(); !ok || position != 3 {
		t.Fatal("fixed call-result lost its authored position")
	}
	if open.Form() != CallResultValues {
		t.Fatal("open call-result form changed")
	}
	if got, ok := open.ValuesTailID(); !ok || got != tail {
		t.Fatal("open call-result did not expose its exact Values tail")
	}
	if _, ok := open.ValueID(); ok {
		t.Fatal("open call-result exposed a fixed Value member")
	}
	if _, ok := open.Position(); ok {
		t.Fatal("open call-result fabricated a fixed position")
	}
}

func TestCallResultRejectsTupleOrMissingCoordinates(t *testing.T) {
	call, values, value, tail := callResultLawID(3), callResultLawID(35), callResultLawID(67), callResultLawID(99)
	for name, row := range map[string]CallResult{
		"both":          {call: call, values: values, value: value, tail: tail, form: CallResultValue},
		"none":          {call: call, values: values, form: CallResultValues},
		"wrong":         {call: call, values: values, tail: tail, form: CallResultValue},
		"open-position": {call: call, values: values, tail: tail, position: 1, form: CallResultValues},
	} {
		if row.Available() {
			t.Fatalf("malformed %s call-result row was available", name)
		}
	}
	if row, ok := NewCallResult(call, values, value, identity.ContentID{}, 0, CallResultInvalid); ok || row.Available() {
		t.Fatal("invalid call-result form was admitted")
	}
}

func TestCallResultExactAndOpenMultiplicityAdmitOnlyConsumerOrdinals(t *testing.T) {
	call, values, tail := callResultLawID(9), callResultLawID(41), callResultLawID(73)
	bounded, boundedOK := NewCallResultWithMultiplicity(call, values, identity.ContentID{}, tail, 0, CallResultValues, CallResultMultiplicityExact, 2)
	open, openOK := NewCallResultWithMultiplicity(callResultLawID(10), values, identity.ContentID{}, tail, 0, CallResultValues, CallResultMultiplicityOpen, 0)
	if !boundedOK || !openOK || bounded.Multiplicity() != CallResultMultiplicityExact || open.Multiplicity() != CallResultMultiplicityOpen {
		t.Fatal("exact/open call-result multiplicities were not sealed")
	}
	if !bounded.AdmitsResult(0) || !bounded.AdmitsResult(1) || bounded.AdmitsResult(2) {
		t.Fatal("bounded Values tail admitted an ordinal outside its consumer width")
	}
	if !open.AdmitsResult(17) {
		t.Fatal("open Values tail rejected an arbitrary result ordinal")
	}
	if count, ok := bounded.ResultCount(); !ok || count != 2 {
		t.Fatalf("bounded result count = %d/%v, want 2/true", count, ok)
	}
	if _, ok := open.ResultCount(); ok {
		t.Fatal("open Values tail exposed a finite result count")
	}
	if row, ok := NewCallResultWithMultiplicity(callResultLawID(11), values, identity.ContentID{}, tail, 0, CallResultValues, CallResultMultiplicityOpen, 1); ok || row.Available() {
		t.Fatal("open Values tail accepted a fabricated finite count")
	}
}

func TestCallResultFamilyRejectsUnavailableRows(t *testing.T) {
	catalog, catalogOK := CatalogID(callResultLawID(131))
	if !catalogOK {
		t.Fatal("call-result law catalog")
	}
	row, rowOK := NewCallResult(callResultLawID(5), callResultLawID(37), callResultLawID(69), identity.ContentID{}, 0, CallResultValue)
	if !rowOK {
		t.Fatal("call-result law row")
	}
	if _, sealed := CallResultFamily().Content([]CallResult{row, CallResult{}}, catalog); sealed {
		t.Fatal("call-result family admitted an unavailable row")
	}
	if _, sealed := CallResultFamily().Content([]CallResult{row}, identity.ContentID{}); sealed {
		t.Fatal("call-result family sealed under an unavailable catalog")
	}
}
