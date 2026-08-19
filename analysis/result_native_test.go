package analysis

import (
	"sort"
	"testing"
)

func TestCorpusNativePublicationUsesTypedBranchValueIssuerLaw(t *testing.T) {
	_, result, _, _ := testCorpusDiagnosticLaw(t, "advice/always-true-guard")
	if !result.NativePublicationAvailable() || result.NativePublicationCount() == 0 {
		t.Fatal("completed branch solve did not expose its typed native publication")
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
			!validityOK || !validity.Valid() || !byIDOK || !byTokenOK || byIDID != id || byTokenID != id {
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

func TestCorpusNativePublicationRenderingIsPinnedLaw(t *testing.T) {
	for _, fixture := range []struct {
		name   string
		values []string
	}{
		{
			name: "advice/always-true-guard",
			values: []string{
				"branch_partition | partition=always_taken dead_arm=else dead_arm_reachable=false",
				"branch_partition | partition=dynamic",
				"constant_value | representation=boolean value=true",
				"representation | exact=true representation=boolean",
				"truthiness_class | class=always_truthy",
				"truthiness_class | class=dynamic_nil_or_false",
			},
		},
		{
			name: "native/const-folded-through-local",
			values: []string{
				"constant_value | representation=integer value=10",
				"constant_value | representation=integer value=15",
				"constant_value | representation=integer value=5",
				"representation | exact=true representation=integer",
				"representation | exact=true representation=integer",
				"representation | exact=true representation=integer",
				"representation | exact=true representation=integer left=integer operator=add overflow=promote_integer_to_number result_representation=integer right=integer",
				"scalar_operator | class=number dispatch=primitive left=integer operator=add overflow=promote_integer_to_number result=integer right=integer",
			},
		},
		{
			name: "native/const-float-literal-representation",
			values: []string{
				"constant_value | representation=float value=42.0",
				"representation | exact=true representation=float",
				"representation | representation=number left=number operator=add overflow=ieee754 result_representation=number right=float",
				"scalar_operator | class=number dispatch=primitive left=number operator=add overflow=ieee754 result=number right=float",
			},
		},
		{
			name: "native/arith-divisor-nonzero-proved",
			values: []string{
				"divisor_property | divisor=nonzero_not_minus_one operator=idiv",
				"representation | exact=true operator=unm overflow=closed_integer representation=integer result_representation=integer operand_representation=integer",
				"representation | exact=true representation=integer left=integer operator=idiv overflow=closed_integer result_representation=integer right=integer",
				"scalar_operator | class=number dispatch=primitive left=integer operator=idiv overflow=closed_integer result=integer right=integer divisor=nonzero_not_minus_one",
			},
		},
		{
			name: "native/repr-pow-int-operands-float-result",
			values: []string{
				"representation | exact=true representation=float left=integer operator=pow overflow=ieee754 result_representation=float right=integer",
				"scalar_operator | class=number dispatch=primitive left=integer operator=pow overflow=ieee754 result=float right=integer",
			},
		},
		{
			name: "native/truthy-empty-string-is-truthy",
			values: []string{
				"branch_partition | partition=always_taken dead_arm=else dead_arm_reachable=false",
				"constant_value | representation=string value=\"\"",
				"representation | exact=true representation=string",
				"truthiness_class | class=always_truthy",
			},
		},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			_, result, _, _ := testCorpusDiagnosticLaw(t, fixture.name)
			if result == nil || !result.NativePublicationAvailable() {
				t.Fatal("solved fixture exposes no native publication")
			}
			published := make([]string, 0, result.NativePublicationCount())
			for index := 0; index < result.NativePublicationCount(); index++ {
				row, rowOK := result.NativePublicationAt(index)
				value, valueOK := row.Value()
				if !rowOK || !valueOK {
					t.Fatalf("native row[%d] is unreadable", index)
				}
				published = append(published, row.Family()+" | "+value)
			}
			sort.Strings(published)
			if len(published) != len(fixture.values) {
				t.Fatalf("published %d rows, want %d: %v", len(published), len(fixture.values), published)
			}
			for index, value := range published {
				if value != fixture.values[index] {
					t.Fatalf("row[%d] published %q, want %q", index, value, fixture.values[index])
				}
			}
		})
	}
}
