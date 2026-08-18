package analysis

import (
	"math"
	"sort"
	"testing"

	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	valuedomain "github.com/wippyai/go-lua/domain/value"
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

func TestNativeRenderCatalogsAreTotalOverTheirSealedVocabulariesLaw(t *testing.T) {
	representations := map[programartifact.NumericRepresentation]string{
		programartifact.NumericRepresentationInteger: "integer",
		programartifact.NumericRepresentationFloat:   "float",
		programartifact.NumericRepresentationNumber:  "number",
	}
	for representation, spelling := range representations {
		rendered, ok := nativeNumericRepresentation(representation)
		if !ok || rendered != spelling {
			t.Fatalf("representation %d renders as %q, want %q", representation, rendered, spelling)
		}
	}
	if _, ok := nativeNumericRepresentation(programartifact.NumericRepresentationInvalid); ok {
		t.Fatal("the invalid representation rendered")
	}
	if _, ok := nativeNumericRepresentation(programartifact.NumericRepresentationNumber + 1); ok {
		t.Fatal("a representation above the sealed vocabulary rendered")
	}

	operators := map[flowkind.BinaryOp]string{
		flowkind.BinaryAdd:  "add",
		flowkind.BinarySub:  "sub",
		flowkind.BinaryMul:  "mul",
		flowkind.BinaryDiv:  "div",
		flowkind.BinaryIDiv: "idiv",
		flowkind.BinaryMod:  "mod",
		flowkind.BinaryPow:  "pow",
	}
	for op := flowkind.BinaryAdd; op <= flowkind.BinaryPow; op++ {
		rendered, ok := nativeArithmeticOperator(op)
		if !ok || rendered != operators[op] {
			t.Fatalf("arithmetic operator %d renders as %q, want %q", op, rendered, operators[op])
		}
	}
	for op := flowkind.BinaryConcat; op <= flowkind.BinaryGreaterEqual; op++ {
		if _, ok := nativeArithmeticOperator(op); ok {
			t.Fatalf("non-arithmetic operator %d rendered as an arithmetic operator", op)
		}
	}

	divisors := map[programartifact.ArithmeticDivisorProperty]string{
		programartifact.ArithmeticDivisorNone:               "",
		programartifact.ArithmeticDivisorNonzero:            "nonzero",
		programartifact.ArithmeticDivisorNonzeroNotMinusOne: "nonzero_not_minus_one",
	}
	for property, spelling := range divisors {
		rendered, ok := nativeArithmeticDivisor(property)
		if !ok || rendered != spelling {
			t.Fatalf("divisor property %d renders as %q, want %q", property, rendered, spelling)
		}
	}
	if _, ok := nativeArithmeticDivisor(programartifact.ArithmeticDivisorNonzeroNotMinusOne + 1); ok {
		t.Fatal("a divisor property above the sealed vocabulary rendered")
	}
}

func TestNativeLiteralRenderingIsOneTableLaw(t *testing.T) {
	for _, rendering := range []struct {
		literal        keyspace.LiteralValue
		representation string
		value          string
	}{
		{literal: keyspace.LiteralValue{Kind: keyspace.LiteralBool}, representation: "boolean", value: "false"},
		{literal: keyspace.LiteralValue{Kind: keyspace.LiteralBool, Bool: true}, representation: "boolean", value: "true"},
		{literal: keyspace.LiteralValue{Kind: keyspace.LiteralInteger, Integer: -7}, representation: "integer", value: "-7"},
		{literal: keyspace.LiteralValue{Kind: keyspace.LiteralFloat, FloatBits: math.Float64bits(42)}, representation: "float", value: "42.0"},
		{literal: keyspace.LiteralValue{Kind: keyspace.LiteralFloat, FloatBits: math.Float64bits(0.5)}, representation: "float", value: "0.5"},
		{literal: keyspace.LiteralValue{Kind: keyspace.LiteralFloat, FloatBits: math.Float64bits(1e21)}, representation: "float", value: "1e+21"},
		{literal: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: ""}, representation: "string", value: `""`},
		{literal: keyspace.LiteralValue{Kind: keyspace.LiteralString, String: "a\"b"}, representation: "string", value: `"a\"b"`},
	} {
		representation, value, ok := renderNativeLiteral(rendering.literal)
		if !ok || representation != rendering.representation || value != rendering.value {
			t.Fatalf("literal kind %d renders as (%q,%q), want (%q,%q)", rendering.literal.Kind, representation, value, rendering.representation, rendering.value)
		}
	}
	for _, unrenderable := range []keyspace.LiteralValue{
		{},
		{Kind: keyspace.LiteralString + 1},
		{Kind: keyspace.LiteralFloat, FloatBits: math.Float64bits(math.NaN())},
		{Kind: keyspace.LiteralFloat, FloatBits: math.Float64bits(math.Inf(1))},
		{Kind: keyspace.LiteralFloat, FloatBits: math.Float64bits(math.Inf(-1))},
	} {
		if _, _, ok := renderNativeLiteral(unrenderable); ok {
			t.Fatalf("literal kind %d rendered a value with no exact spelling", unrenderable.Kind)
		}
	}
	if representation, value, ok := renderNativeNil(); !ok || representation != "nil" || value != "nil" {
		t.Fatalf("nil renders as (%q,%q), want (nil,nil)", representation, value)
	}
	if _, _, ok := renderNativeExactScalar(valuedomain.ExactScalar{}); ok {
		t.Fatal("an unclassified exact scalar rendered")
	}
	if _, _, ok := renderNativeScalarSummary(compiledNativeScalarSource{}); ok {
		t.Fatal("an invalid scalar summary rendered")
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
