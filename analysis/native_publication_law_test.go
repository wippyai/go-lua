package analysis

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

func TestCorpusNativePublicationUsesTypedBranchValueIssuerLaw(t *testing.T) {
	_, result, _, _ := testCorpusDiagnosticLaw(t, "advice/always-true-guard")
	if !result.NativePublicationAvailable() || result.NativePublicationCount() == 0 {
		t.Fatal("completed branch solve did not expose its typed native receipt")
	}
	seen := make(map[string]bool)
	for index := 0; index < result.NativePublicationCount(); index++ {
		row, rowOK := result.NativePublicationAt(index)
		id, idOK := row.ID()
		value, valueOK := row.Value()
		provenance, provenanceOK := row.Provenance()
		validity, validityOK := row.Validity()
		byID, byIDOK := result.NativePublicationByID(id)
		byToken, byTokenOK := result.NativePublicationByToken(row.Token())
		byIDID, _ := byID.ID()
		byTokenID, _ := byToken.ID()
		if !rowOK || !idOK || !id.Available() || !row.Lane().Valid() || !row.Kind().Valid() || !row.Trust().Valid() ||
			!row.SemanticID().Available() || row.Family() == "" || row.Key() == "" || row.Module() == "" || !valueOK || value == "" ||
			!provenanceOK || !provenance.MountID().Available() || !provenance.ArtifactID().Available() || !provenance.LocalID().Available() || !provenance.BodyID().Available() || !provenance.PointID().Available() || !provenance.SourceSpanID().Available() ||
			!validityOK || !validity.valid() || !byIDOK || !byTokenOK || byIDID != id || byTokenID != id {
			t.Fatalf("native row[%d] is not a complete Result-owned publication", index)
		}
		seen[row.Family()] = true
	}
	if !seen["constant_value"] || !seen["representation"] || !seen["truthiness_class"] || !seen["branch_partition"] {
		t.Fatalf("native branch families=%v, want constant/representation/truthiness/partition", seen)
	}

	_, foreign, _, _ := testCorpusDiagnosticLaw(t, "advice/always-true-guard")
	row, _ := result.NativePublicationAt(0)
	if _, ok := foreign.NativePublicationByToken(row.Token()); ok {
		t.Fatal("foreign equal-content Result accepted native row token")
	}
	if _, ok := result.NativePublicationAt(-1); ok {
		t.Fatal("negative native ordinal accepted")
	}
}

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

func TestNativePublicationRowSealRejectsPrivateSplicesLaw(t *testing.T) {
	_, result, _, _ := testCorpusDiagnosticLaw(t, "advice/always-true-guard")
	if result == nil || !result.NativePublicationAvailable() || result.NativePublicationCount() == 0 {
		t.Fatal("native splice fixture unavailable")
	}
	original := result.native
	clone := func() *nativePublicationReceipt {
		copyReceipt := *original
		copyReceipt.rows = append([]nativePublicationRow(nil), original.rows...)
		copyReceipt.byID = make(map[identity.ContentID]uint32, len(original.byID))
		for id, ordinal := range original.byID {
			copyReceipt.byID[id] = ordinal
		}
		return &copyReceipt
	}
	tests := []struct {
		name   string
		mutate func(*nativePublicationReceipt)
	}{
		{name: "stored id", mutate: func(receipt *nativePublicationReceipt) { receipt.rows[0].id[0] ^= 0xff }},
		{name: "semantic", mutate: func(receipt *nativePublicationReceipt) { receipt.rows[0].semantic[0] ^= 0xff }},
		{name: "family", mutate: func(receipt *nativePublicationReceipt) { receipt.rows[0].family = nativePublicationFamilyInvalid }},
		{name: "value", mutate: func(receipt *nativePublicationReceipt) { receipt.rows[0].value += "-spliced" }},
		{name: "provenance", mutate: func(receipt *nativePublicationReceipt) { receipt.rows[0].provenance.point[0] ^= 0xff }},
		{name: "validity", mutate: func(receipt *nativePublicationReceipt) { receipt.rows[0].validity.establishedOrdinal = 1 }},
		{name: "id index", mutate: func(receipt *nativePublicationReceipt) {
			receipt.byID[receipt.rows[0].id] = uint32(len(receipt.rows) + 1)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			spliced := clone()
			test.mutate(spliced)
			result.native = spliced
			t.Cleanup(func() { result.native = original })
			if _, ok := result.NativePublicationAt(0); ok {
				t.Fatal("private native row splice remained readable")
			}
			result.native = original
		})
	}
}
