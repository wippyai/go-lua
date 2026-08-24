package composition

import (
	"encoding/hex"
	"testing"

	queryschema "github.com/wippyai/go-lua/analysis/schema/query"
)

// The cold codec version and the digest it frames are the identity fence of the
// declaration plane. A construction-path change may reorganize who builds a
// Candidate and when, but it must not move the CompositionID a fixed
// declaration set seals to: a persisted CompositionID outlives the code that
// minted it, so a silent digest move aliases two different cold meanings under
// one identity.
//
// codecVersionFence is the version compositionID frames its preimage under.
// Raising it is a deliberate declaration that every persisted CompositionID is
// now unreadable, so it may only change together with coldCompositionFenceHex.
const codecVersionFence = 19

// coldCompositionFenceHex is the sealed CompositionID of fenceCandidate. It is
// stable by design: the preimage is the sorted declaration set, the codec
// version, and the domain tag, none of which a construction-path edit is
// allowed to reach.
const coldCompositionFenceHex = "37c05a57248b14657925862409e84fffef5e5bd970883cb37b24cb24b280c5dc"

func TestColdCodecVersionIsFenced(t *testing.T) {
	if codecVersion != codecVersionFence {
		t.Fatalf("codecVersion is %d, the fence pins %d; raising it invalidates every persisted CompositionID and must move coldCompositionFenceHex in the same change", codecVersion, codecVersionFence)
	}
}

// fenceCandidate is the fixed declaration set the digest is pinned over. It
// exercises every field of Candidate that enters compositionID: both factor
// output kinds, a form, all three rule part kinds in declaration order, the
// completion pair, an activation family, and a query family with projections.
func fenceCandidate() Candidate {
	factor, summary, structural := fenceKey(11), fenceKey(12), fenceKey(13)
	completion, prune, family := fenceKey(14), fenceKey(15), fenceKey(16)
	return Candidate{
		Factors: []Factor{
			{Key: factor},
			{Key: summary, Forms: []FactorForm{{Kind: FactorSummaryRead, Semantic: fenceKey(21)}}},
			{Key: structural},
		},
		Completion:         Completion{Semantic: completion, Prune: prune},
		ActivationFamilies: []ActivationFamily{{Semantic: family}},
		Rules: []Rule{
			{
				Key: fenceKey(31), OperandFamily: fenceKey(32),
				OutputKind: FactorOutput, Output: factor, Inputs: 2,
				Reads: []Read{
					{Kind: ReadExact, Input: 0, Factor: structural, PointBound: true},
					{Kind: ReadSummary, Input: 1, Factor: summary, Semantic: fenceKey(21), Normalizer: fenceKey(21), PointBound: false},
				},
				Carries: []Carry{{Input: 0, Factor: factor, Transform: fenceKey(33)}},
				Writes:  []Write{{Kind: WriteExact, Factor: factor}},
			},
			{
				Key: fenceKey(41), OperandFamily: fenceKey(42),
				OutputKind: StructuralOutput, Inputs: 1,
				Reads:    []Read{{Kind: ReadExact, Input: 0, Factor: factor}},
				Supports: []Support{{Semantic: completion}},
				Prunes:   []Prune{{Semantic: prune}},
			},
			{
				Key: fenceKey(51), OperandFamily: fenceKey(52),
				OutputKind: StructuralOutput, Inputs: 0,
				Activations: []ActivationRange{{Family: family}},
			},
		},
		Queries: []QueryFamily{{
			Key: fenceKey(61), Freezer: fenceKey(62), Population: queryschema.PopulationKindSelectedPoint,
			Projections: []QueryProjection{
				{Kind: QueryFactorExact, Factor: factor},
				{Kind: QueryFactorSummary, Factor: summary, Normalizer: fenceKey(21)},
			},
		}},
	}
}

func fenceKey(value byte) Key {
	var id ID
	id[0] = value
	return Key{ID: id, Version: 1}
}

// TestColdCompositionDigestIsFenced pins the whole preimage in one literal: the
// domain tag, the codec version, the field order, and the canonicalization
// Seal applies before hashing. It is the fence the construction cut runs
// against.
func TestColdCompositionDigestIsFenced(t *testing.T) {
	sealed, ok := Seal(fenceCandidate())
	if !ok || sealed == nil {
		t.Fatal("the fenced declaration set no longer seals")
	}
	id := sealed.ID()
	if !id.Available() {
		t.Fatal("the fenced declaration set sealed to an unavailable CompositionID")
	}
	if got := hex.EncodeToString(id[:]); got != coldCompositionFenceHex {
		t.Fatalf("fenced cold CompositionID is %s, the fence pins %s; a construction-path edit must not move it", got, coldCompositionFenceHex)
	}
}

// TestColdCompositionDigestIsDeclarationOrderIndependent records why the digest
// above is a legitimate literal rather than a snapshot of one build order: Seal
// canonicalizes the four keyed sets, so re-ordering the declarations a builder
// emits cannot reach identity. Only the ordered sub-slices do.
func TestColdCompositionDigestIsDeclarationOrderIndependent(t *testing.T) {
	forward, forwardOK := Seal(fenceCandidate())
	shuffled := fenceCandidate()
	shuffled.Factors[0], shuffled.Factors[1] = shuffled.Factors[1], shuffled.Factors[0]
	shuffled.Rules[0], shuffled.Rules[2] = shuffled.Rules[2], shuffled.Rules[0]
	reverse, reverseOK := Seal(shuffled)
	if !forwardOK || !reverseOK || forward == nil || reverse == nil {
		t.Fatal("the fenced declaration set no longer seals in both orders")
	}
	if forward.ID() != reverse.ID() {
		t.Fatal("declaration order reached cold identity; the pinned digest would not be stable")
	}
	ordered := fenceCandidate()
	ordered.Rules[0].Reads[0], ordered.Rules[0].Reads[1] = ordered.Rules[0].Reads[1], ordered.Rules[0].Reads[0]
	permuted, permutedOK := Seal(ordered)
	if !permutedOK || permuted == nil {
		t.Fatal("the fenced declaration set no longer seals with permuted reads")
	}
	if permuted.ID() == forward.ID() {
		t.Fatal("read declaration order did not reach cold identity; the ordered sub-slices are no longer part of the preimage")
	}
}
