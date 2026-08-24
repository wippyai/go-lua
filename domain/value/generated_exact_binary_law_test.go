package value_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/domain/value"
)

// TestExactBinaryPayloadRejectsForeignOwner ensures the generated receiver
// fence is checked before a concrete candidate reaches its semantic fold.
// The payload is minted by one sealed Schema and must not be redeemable by a
// second Schema, even when both schemas were built from equivalent source.
func TestExactBinaryPayloadRejectsForeignOwner(t *testing.T) {
	local := newEndpointFixture(t, "exact_binary_payload_local")
	foreign := newEndpointFixture(t, "exact_binary_payload_foreign")
	for _, occurrence := range local.occurrences {
		row, rowOK := local.values.BinaryArithmetic(local.module, occurrence)
		if !rowOK || !local.values.OwnsBinaryArithmetic(row) {
			continue
		}
		ordinal, ordinalOK := local.values.BinaryArithmeticOrdinal(row)
		mapping, mappingOK := local.values.ExactBinaryMappingAt(3)
		payload, payloadOK := local.values.ExactBinaryPayloadAt(3, mapping.CandidateRelationMember, ordinal)
		if !ordinalOK || !mappingOK || !payloadOK {
			t.Fatal("local exact-binary payload")
		}
		if _, _, dispatched := foreign.values.ReduceExactBinaryPayload(payload, foreign.values.Bottom(), foreign.values.Bottom()); dispatched {
			t.Fatal("foreign Value Schema accepted a local exact-binary payload")
		}
		return
	}
	t.Fatal("fixture admitted no arithmetic exact-binary candidate")
}

// TestExactBinaryPayloadRefusesMalformedOwnedInputs proves that a payload
// minted by the receiving owner does not turn malformed input evidence into a
// semantic reduction. Refuse from the owner fold is a structural dispatch
// failure at this boundary; execution must abort instead of publishing it.
func TestExactBinaryPayloadRefusesMalformedOwnedInputs(t *testing.T) {
	fixture := newEndpointFixture(t, "exact_binary_payload_malformed_input")
	for _, occurrence := range fixture.occurrences {
		row, rowOK := fixture.values.BinaryArithmetic(fixture.module, occurrence)
		if !rowOK || !fixture.values.OwnsBinaryArithmetic(row) {
			continue
		}
		ordinal, ordinalOK := fixture.values.BinaryArithmeticOrdinal(row)
		mapping, mappingOK := fixture.values.ExactBinaryMappingAt(3)
		payload, payloadOK := fixture.values.ExactBinaryPayloadAt(3, mapping.CandidateRelationMember, ordinal)
		if !ordinalOK || !mappingOK || !payloadOK {
			t.Fatal("local exact-binary payload")
		}
		if _, outcome, dispatched := fixture.values.ReduceExactBinaryPayload(payload, value.Value{}, value.Value{}); dispatched || outcome != structure.Refuse {
			t.Fatalf("malformed owned inputs were dispatched: outcome=%v dispatched=%v", outcome, dispatched)
		}
		return
	}
	t.Fatal("fixture admitted no arithmetic exact-binary candidate")
}

// TestExactBinaryPayloadRefusesAProviderMemberWithTheSameOrdinal proves the
// owner-issued relation/reducer correspondence is part of cold redemption.
// A hostile installer must not be able to swap the candidate relation while
// retaining an otherwise valid endpoint ordinal.
func TestExactBinaryPayloadRefusesAProviderMemberWithTheSameOrdinal(t *testing.T) {
	fixture := newEndpointFixture(t, "exact_binary_payload_provider_member")
	for _, occurrence := range fixture.occurrences {
		row, rowOK := fixture.values.BinaryArithmetic(fixture.module, occurrence)
		if !rowOK || !fixture.values.OwnsBinaryArithmetic(row) {
			continue
		}
		ordinal, ordinalOK := fixture.values.BinaryArithmeticOrdinal(row)
		mapping, mappingOK := fixture.values.ExactBinaryMappingAt(3)
		if !ordinalOK || !mappingOK {
			t.Fatal("arithmetic mapping")
		}
		if _, accepted := fixture.values.ExactBinaryPayloadAt(3, mapping.CandidateRelationMember+1, ordinal); accepted {
			t.Fatal("owner accepted a foreign candidate relation member")
		}
		return
	}
	t.Fatal("fixture admitted no arithmetic exact-binary candidate")
}

func TestExactBinaryPayloadDispatchMatchesOwnedFolds(t *testing.T) {
	fixture := newEndpointFixture(t, "exact_binary_payload_fold_parity")
	left, right := fixture.values.Bottom(), fixture.values.Top()
	seen := [3]bool{}
	for ordinal := 0; ordinal < fixture.values.EndpointCount(); ordinal++ {
		if candidate, ok := fixture.values.BinaryArithmeticAt(ordinal); ok {
			want, wantOutcome := value.ArithmeticValue(candidate, left, right)
			assertExactBinaryPayloadFold(t, fixture.values, 3, uint32(ordinal), left, right, want, wantOutcome)
			seen[0] = true
		}
		if candidate, ok := fixture.values.BinaryEqualityAt(ordinal); ok {
			want, wantOutcome := value.EqualityValue(candidate, left, right)
			assertExactBinaryPayloadFold(t, fixture.values, 4, uint32(ordinal), left, right, want, wantOutcome)
			seen[1] = true
		}
		if candidate, ok := fixture.values.BinaryOrderAt(ordinal); ok {
			want, wantOutcome := value.OrderValue(candidate, left, right)
			assertExactBinaryPayloadFold(t, fixture.values, 5, uint32(ordinal), left, right, want, wantOutcome)
			seen[2] = true
		}
	}
	if !seen[0] || !seen[1] || !seen[2] {
		t.Fatalf("exact-binary fixture coverage arithmetic/equality/order = %v", seen)
	}
}

func assertExactBinaryPayloadFold(t testing.TB, schema *value.Schema, reducer, candidate uint32, left, right, want value.Value, wantOutcome structure.ReductionOutcome) {
	t.Helper()
	mapping, mappingOK := schema.ExactBinaryMappingAt(reducer)
	payload, payloadOK := schema.ExactBinaryPayloadAt(reducer, mapping.CandidateRelationMember, candidate)
	got, gotOutcome, dispatched := schema.ReduceExactBinaryPayload(payload, left, right)
	wantDispatched := wantOutcome.Available() && wantOutcome != structure.Refuse
	if !mappingOK || !payloadOK || dispatched != wantDispatched || gotOutcome != wantOutcome || (gotOutcome == structure.Concrete && !schema.Equal(got, want)) {
		t.Fatalf("payload reducer=%d candidate=%d = outcome:%v dispatched:%v; want outcome:%v dispatched:%v", reducer, candidate, gotOutcome, dispatched, wantOutcome, wantDispatched)
	}
}
