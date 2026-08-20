package oracle

import (
	"sort"
	"testing"

	"github.com/wippyai/go-lua/analysis/result"
)

func TestCorpusNativePublicationUsesTypedBranchValueIssuerLaw(t *testing.T) {
	result := nativePublicationCorpusResult(t, "advice/always-true-guard")
	if !result.NativePublicationAvailable() || result.NativePublicationCount() == 0 {
		t.Fatal("completed branch solve did not expose its typed native publication")
	}
	seen := make(map[string]bool)
	for index := 0; index < result.NativePublicationCount(); index++ {
		row, rowOK := result.NativePublicationAt(index)
		id, idOK := row.ID()
		columns, columnsOK := corpusNativeColumns(row)
		provenance, provenanceOK := row.Provenance()
		validity, validityOK := row.Validity()
		byID, byIDOK := result.NativePublicationByID(id)
		byToken, byTokenOK := result.NativePublicationByToken(row.Token())
		byIDID, _ := byID.ID()
		byTokenID, _ := byToken.ID()
		if !rowOK || !idOK || !id.Available() || !row.Lane().Valid() || !row.Kind().Valid() || !row.Trust().Valid() ||
			!row.SemanticID().Available() || row.Family() == "" || row.Key() == "" || row.Module() == "" || !columnsOK || len(columns) == 0 || row.EvidencePointCount() == 0 ||
			!provenanceOK || !provenance.MountID().Available() || !provenance.ArtifactID().Available() || !provenance.LocalID().Available() || !provenance.BodyID().Available() || !provenance.PointID().Available() || !provenance.SourceSpanID().Available() ||
			!validityOK || !validity.Valid() || !byIDOK || !byTokenOK || byIDID != id || byTokenID != id {
			t.Fatalf("native row[%d] is not a complete Result-owned publication", index)
		}
		seen[row.Family()] = true
	}
	if !seen["constant_value"] || !seen["representation"] || !seen["truthiness_class"] || !seen["branch_partition"] {
		t.Fatalf("native branch families=%v, want constant/representation/truthiness/partition", seen)
	}

	foreign := nativePublicationCorpusResult(t, "advice/always-true-guard")
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
				"constant_value | literal=true representation=boolean",
				"representation | exact=true representation=boolean",
				"truthiness_class | truthiness=always_truthy",
				"truthiness_class | truthiness=dynamic_nil_or_false",
			},
		},
		{
			name: "native/const-folded-through-local",
			values: []string{
				"constant_value | literal=10 representation=integer",
				"constant_value | literal=15 representation=integer",
				"constant_value | literal=5 representation=integer",
				"representation | exact=true representation=integer",
				"representation | exact=true representation=integer",
				"representation | exact=true representation=integer",
				"representation | exact=true representation=integer left=integer right=integer operator=add overflow=promote_integer_to_number",
				"scalar_operator | representation=integer left=integer right=integer operator=add overflow=promote_integer_to_number",
			},
		},
		{
			name: "native/const-float-literal-representation",
			values: []string{
				"constant_value | literal=42.0 representation=float",
				"representation | exact=true representation=float",
				"representation | representation=number left=number right=float operator=add overflow=ieee754",
				"scalar_operator | representation=number left=number right=float operator=add overflow=ieee754",
			},
		},
		{
			name: "native/arith-divisor-nonzero-proved",
			values: []string{
				"divisor_property | operator=idiv divisor=nonzero_not_minus_one",
				"representation | exact=true representation=integer left=integer right=integer operator=idiv overflow=closed_integer",
				"representation | exact=true representation=integer operand=integer operator=unm overflow=closed_integer",
				"scalar_operator | representation=integer left=integer right=integer operator=idiv overflow=closed_integer divisor=nonzero_not_minus_one",
			},
		},
		{
			name: "native/repr-pow-int-operands-float-result",
			values: []string{
				"representation | exact=true representation=float left=integer right=integer operator=pow overflow=ieee754",
				"scalar_operator | representation=float left=integer right=integer operator=pow overflow=ieee754",
			},
		},
		{
			name: "native/truthy-empty-string-is-truthy",
			values: []string{
				"branch_partition | partition=always_taken dead_arm=else dead_arm_reachable=false",
				"constant_value | literal=\"\" representation=string",
				"representation | exact=true representation=string",
				"truthiness_class | truthiness=always_truthy",
			},
		},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			result := nativePublicationCorpusResult(t, fixture.name)
			if result == nil || !result.NativePublicationAvailable() {
				t.Fatal("solved fixture exposes no native publication")
			}
			published := make([]string, 0, result.NativePublicationCount())
			for index := 0; index < result.NativePublicationCount(); index++ {
				row, rowOK := result.NativePublicationAt(index)
				columns, columnsOK := corpusNativeColumns(row)
				if !rowOK || !columnsOK {
					t.Fatalf("native row[%d] is unreadable", index)
				}
				published = append(published, row.Family()+" | "+corpusNativeRendering(columns))
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

// nativePublicationCorpusResult keeps these Result-owned assertions on the
// oracle's one canonical corpus spine; it does not rebuild a publication or
// retain an analyzer implementation handle.
func nativePublicationCorpusResult(t *testing.T, name string) *result.Result {
	t.Helper()
	run, _, err := corpusHarnessExecuteDetached(t, corpusHarnessFixture(t, name), corpusHarnessDiagnosticMode())
	if err != nil {
		t.Fatal(err)
	}
	return run.result
}
