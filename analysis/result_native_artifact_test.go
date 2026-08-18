package analysis

import "testing"

func TestCorpusNativePublicationPublishesReusableExactScalarSummaryLaw(t *testing.T) {
	_, result, _, _ := testCorpusDiagnosticLaw(t, "native/const-folded-through-local")
	if result == nil || !result.NativePublicationAvailable() {
		t.Fatal("folded-local result has no typed native publication")
	}
	constants := make(map[string]int)
	representations := make(map[string]int)
	operators := make(map[string]int)
	for index := 0; index < result.NativePublicationCount(); index++ {
		row, rowOK := result.NativePublicationAt(index)
		value, valueOK := row.Value()
		_, provenanceOK := row.Provenance()
		if !rowOK || !valueOK || !provenanceOK {
			t.Fatalf("native scalar row[%d] unavailable", index)
		}
		switch row.Family() {
		case "constant_value":
			constants[value]++
		case "representation":
			representations[value]++
		case "scalar_operator":
			operators[value]++
		}
	}
	if len(constants) != 3 || constants["representation=integer value=5"] != 1 ||
		constants["representation=integer value=10"] != 1 ||
		constants["representation=integer value=15"] != 1 ||
		representations["exact=true representation=integer"] != 3 || len(operators) != 1 {
		t.Fatalf("folded-local constants=%v representations=%v operators=%v", constants, representations, operators)
	}
	for value := range constants {
		if value == "representation=float value=10.0" || value == "representation=float value=15.0" {
			t.Fatalf("integer Program summary widened to float: %q", value)
		}
	}
}
