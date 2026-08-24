package value_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/domain/value"
)

// exactFoldReducerFor resolves the one generated reducer whose owner-issued
// candidate directory redeems this endpoint ordinal. The reducer identity is
// derived from the generated dispatch itself rather than restated by hand, so
// a catalog that renumbers its members moves the law with it.
func exactFoldReducerFor(t testing.TB, schema *value.Schema, candidate uint32) (uint32, value.ExactFoldMapping) {
	t.Helper()
	var reducer uint32
	var mapping value.ExactFoldMapping
	found := false
	for probe := uint32(0); probe < 64; probe++ {
		if !value.SupportsExactFoldReducer(probe) {
			continue
		}
		candidateMapping, mappingOK := schema.ExactFoldMappingAt(probe)
		if !mappingOK || candidateMapping.ReducerOrdinal != probe {
			t.Fatalf("supported reducer %d publishes no mapping of its own", probe)
		}
		if _, payloadOK := schema.ExactFoldPayloadAt(probe, candidateMapping.CandidateRelationMember, candidate); !payloadOK {
			continue
		}
		if found {
			t.Fatalf("candidate %d redeems through more than one generated reducer", candidate)
		}
		reducer, mapping, found = probe, candidateMapping, true
	}
	if !found {
		t.Fatalf("candidate %d redeems through no generated reducer", candidate)
	}
	return reducer, mapping
}

// TestExactFoldPayloadRejectsForeignOwner ensures the generated receiver
// fence is checked before a concrete candidate reaches its semantic fold.
// The payload is minted by one sealed Schema and must not be redeemable by a
// second Schema, even when both schemas were built from equivalent source.
func TestExactFoldPayloadRejectsForeignOwner(t *testing.T) {
	local := newEndpointFixture(t, "exact_fold_payload_local")
	foreign := newEndpointFixture(t, "exact_fold_payload_foreign")
	for _, occurrence := range local.occurrences {
		row, rowOK := local.values.BinaryArithmetic(local.module, occurrence)
		if !rowOK || !local.values.OwnsBinaryArithmetic(row) {
			continue
		}
		ordinal, ordinalOK := local.values.BinaryArithmeticOrdinal(row)
		if !ordinalOK {
			t.Fatal("local arithmetic ordinal")
		}
		reducer, mapping := exactFoldReducerFor(t, local.values, ordinal)
		payload, payloadOK := local.values.ExactFoldPayloadAt(reducer, mapping.CandidateRelationMember, ordinal)
		if !payloadOK {
			t.Fatal("local exact fold payload")
		}
		var reads [value.ExactFoldArity]value.Value
		for position := range reads {
			reads[position] = foreign.values.Bottom()
		}
		if _, _, dispatched := foreign.values.ReduceExactFoldPayload(payload, reads); dispatched {
			t.Fatal("foreign Value Schema accepted a local exact fold payload")
		}
		return
	}
	t.Fatal("fixture admitted no arithmetic exact fold candidate")
}

// TestExactFoldPayloadRefusesMalformedOwnedInputs proves that a payload
// minted by the receiving owner does not turn malformed input evidence into a
// semantic reduction. Refuse from the owner fold is a structural dispatch
// failure at this boundary; execution must abort instead of publishing it.
func TestExactFoldPayloadRefusesMalformedOwnedInputs(t *testing.T) {
	fixture := newEndpointFixture(t, "exact_fold_payload_malformed_input")
	for _, occurrence := range fixture.occurrences {
		row, rowOK := fixture.values.BinaryArithmetic(fixture.module, occurrence)
		if !rowOK || !fixture.values.OwnsBinaryArithmetic(row) {
			continue
		}
		ordinal, ordinalOK := fixture.values.BinaryArithmeticOrdinal(row)
		if !ordinalOK {
			t.Fatal("arithmetic ordinal")
		}
		reducer, mapping := exactFoldReducerFor(t, fixture.values, ordinal)
		payload, payloadOK := fixture.values.ExactFoldPayloadAt(reducer, mapping.CandidateRelationMember, ordinal)
		if !payloadOK {
			t.Fatal("local exact fold payload")
		}
		var reads [value.ExactFoldArity]value.Value
		if _, outcome, dispatched := fixture.values.ReduceExactFoldPayload(payload, reads); dispatched || outcome != structure.Refuse {
			t.Fatalf("malformed owned inputs were dispatched: outcome=%v dispatched=%v", outcome, dispatched)
		}
		return
	}
	t.Fatal("fixture admitted no arithmetic exact fold candidate")
}

// TestExactFoldPayloadRefusesAProviderMemberWithTheSameOrdinal proves the
// owner-issued relation/reducer correspondence is part of cold redemption.
// A hostile installer must not be able to swap the candidate relation while
// retaining an otherwise valid endpoint ordinal.
func TestExactFoldPayloadRefusesAProviderMemberWithTheSameOrdinal(t *testing.T) {
	fixture := newEndpointFixture(t, "exact_fold_payload_provider_member")
	for _, occurrence := range fixture.occurrences {
		row, rowOK := fixture.values.BinaryArithmetic(fixture.module, occurrence)
		if !rowOK || !fixture.values.OwnsBinaryArithmetic(row) {
			continue
		}
		ordinal, ordinalOK := fixture.values.BinaryArithmeticOrdinal(row)
		if !ordinalOK {
			t.Fatal("arithmetic ordinal")
		}
		reducer, mapping := exactFoldReducerFor(t, fixture.values, ordinal)
		if _, accepted := fixture.values.ExactFoldPayloadAt(reducer, mapping.CandidateRelationMember+1, ordinal); accepted {
			t.Fatal("owner accepted a foreign candidate relation member")
		}
		return
	}
	t.Fatal("fixture admitted no arithmetic exact fold candidate")
}

func TestExactFoldPayloadDispatchMatchesOwnedFolds(t *testing.T) {
	fixture := newEndpointFixture(t, "exact_fold_payload_fold_parity")
	left, right := fixture.values.Bottom(), fixture.values.Top()
	seen := [3]bool{}
	for ordinal := 0; ordinal < fixture.values.EndpointCount(); ordinal++ {
		if candidate, ok := fixture.values.BinaryArithmeticAt(ordinal); ok {
			want, wantOutcome := value.ArithmeticValue(candidate, left, right)
			assertExactFoldPayloadFold(t, fixture.values, uint32(ordinal), 2, [value.ExactFoldArity]value.Value{left, right}, want, wantOutcome)
			seen[0] = true
		}
		if candidate, ok := fixture.values.BinaryEqualityAt(ordinal); ok {
			want, wantOutcome := value.EqualityValue(candidate, left, right)
			assertExactFoldPayloadFold(t, fixture.values, uint32(ordinal), 2, [value.ExactFoldArity]value.Value{left, right}, want, wantOutcome)
			seen[1] = true
		}
		if candidate, ok := fixture.values.BinaryOrderAt(ordinal); ok {
			want, wantOutcome := value.OrderValue(candidate, left, right)
			assertExactFoldPayloadFold(t, fixture.values, uint32(ordinal), 2, [value.ExactFoldArity]value.Value{left, right}, want, wantOutcome)
			seen[2] = true
		}
	}
	if !seen[0] || !seen[1] || !seen[2] {
		t.Fatalf("exact fold fixture coverage arithmetic/equality/order = %v", seen)
	}
}

// assertExactFoldPayloadFold checks one candidate's generated dispatch against
// the Value-owned fold it names, including the sealed read width the payload
// publishes.
func assertExactFoldPayloadFold(t testing.TB, schema *value.Schema, candidate uint32, reads int, vector [value.ExactFoldArity]value.Value, want value.Value, wantOutcome structure.ReductionOutcome) {
	t.Helper()
	reducer, mapping := exactFoldReducerFor(t, schema, candidate)
	payload, payloadOK := schema.ExactFoldPayloadAt(reducer, mapping.CandidateRelationMember, candidate)
	payloadReads, payloadReadsOK := payload.ReadCount()
	if !payloadOK || !payloadReadsOK || payloadReads != reads || int(mapping.ReadCount) != reads {
		t.Fatalf("payload reducer=%d candidate=%d publishes %d reads; the fold consumes %d", reducer, candidate, payloadReads, reads)
	}
	got, gotOutcome, dispatched := schema.ReduceExactFoldPayload(payload, vector)
	wantDispatched := wantOutcome.Available() && wantOutcome != structure.Refuse
	if dispatched != wantDispatched || gotOutcome != wantOutcome || (gotOutcome == structure.Concrete && !schema.Equal(got, want)) {
		t.Fatalf("payload reducer=%d candidate=%d = outcome:%v dispatched:%v; want outcome:%v dispatched:%v", reducer, candidate, gotOutcome, dispatched, wantOutcome, wantDispatched)
	}
}
