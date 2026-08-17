package engine

import (
	"encoding/hex"
	"testing"
)

// This file is the engine-side identity fence of the construction cut. The cut
// moves who builds a program and when; it must move nothing a published address
// or a persisted identity is spelled with. Each law below pins one such
// vocabulary so a construction-path edit that reaches it fails here rather than
// in a consumer holding a stale address.
//
// Values already pinned relationally elsewhere are referenced, not duplicated:
//   - snapshot_materialize_law_test.go asserts published.Columns() ==
//     solvedStoreColumns and Queries().Len() == solvedAxisCount, and
//     runtime_point_state_snapshot_law_test.go repeats it for the point store.
//     Those laws fence the relations; this file fences the absolute ordinals
//     they are relations between.
//   - state_borrow_law_test.go TestResultAddressIsNotPersistable fences the
//     lane's fail-closed semantics; this file fences its ordinals.
//   - internal/composition identity_fence_law_test.go pins codecVersion and the
//     cold digest of a hand-built Candidate; the law here pins the digest the
//     SchemaBuilder declaration surface itself produces, so the two together
//     fence both the codec and the declarations fed into it.
//   - internal/equation identity_fence_law_test.go pins identityVersion and the
//     deriveQueryKey preimage.

// schemaDeclarationFenceHex is the cold CompositionID of fenceSchema. The
// builder.candidate declaration set in schema_slots.go is its sole preimage, so
// this literal fences every candidate declaration the builder emits: a changed
// field, a dropped part, or a reordered ordered sub-slice moves it.
const schemaDeclarationFenceHex = "3c5c0b7267f63d0e7a285b5b82bc0f0316d05ecb14649fef2b2b93da7dae0a59"

// fenceSchema declares a fixed schema through the public SchemaBuilder surface:
// a Factor with both intrinsic forms, a Factor-output Rule carrying a trusted
// admission and an exact write, and a query family with one exact projection.
// Each of those is a distinct builder.candidate append, so the sealed cold
// identity below is the digest of that declaration sequence.
func fenceSchema(t testing.TB) *Schema {
	t.Helper()
	builder := NewSchema()
	factor, factorOK := DeclareFactorSlot[uint64](builder, coldKey(970_001))
	writeForm, writeFormOK := factor.ExactWrite()
	readForm, readFormOK := factor.ExactRead()
	rule, ruleOK := DeclareRuleSlot[uint64, ruleUnit](builder, SchemaRuleSpec[uint64]{
		Semantic: coldKey(970_011), OperandFamily: unitOperandFamily, Inputs: 0,
		Admission: SchemaAdmission{Basis: RuleAdmissionBasisTrustedTheorem, Identity: coldKey(970_012)}, Output: factor.Ref(),
	})
	_, writeOK := SchemaWrite(rule, writeForm)
	query, queryOK := DeclareQuerySlot[uint64](builder, SchemaQuerySpec{Semantic: coldKey(970_021), Freezer: coldKey(970_022)})
	queryReadOK := SchemaQueryRead(query, readForm)
	schema, schemaOK := builder.Seal()
	if !factorOK || !writeFormOK || !readFormOK || !ruleOK || !writeOK || !queryOK || !queryReadOK || !schemaOK || schema == nil {
		t.Fatal("the fenced schema declaration set no longer seals")
	}
	return schema
}

// TestSchemaDeclarationSetHasFencedColdIdentity is the fence on the builder's
// candidate declarations: the cold identity a fixed declaration sequence seals
// to is the identity every persisted artifact and every mounted graph is keyed
// under, so the construction cut must not move it.
func TestSchemaDeclarationSetHasFencedColdIdentity(t *testing.T) {
	schema := fenceSchema(t)
	id := schema.coldID()
	if !id.Available() {
		t.Fatal("the fenced schema sealed to an unavailable cold identity")
	}
	if got := hex.EncodeToString(id[:]); got != schemaDeclarationFenceHex {
		t.Fatalf("fenced schema cold identity is %s, the fence pins %s; a construction-path edit must not reach the candidate declaration set", got, schemaDeclarationFenceHex)
	}
}

// TestSchemaDeclarationSetIsDeterministic records that the pinned digest is a
// property of the declarations rather than of one build: two independent builds
// of the same declaration sequence seal to the same cold identity.
func TestSchemaDeclarationSetIsDeterministic(t *testing.T) {
	if first, second := fenceSchema(t).coldID(), fenceSchema(t).coldID(); first != second {
		t.Fatal("two builds of one declaration set sealed to different cold identities")
	}
}

// resultLaneFence is the published result-column vocabulary. A lane ordinal is
// part of a resultAddress and of solvedAxisIdentity's preimage, so it is a
// persisted coordinate: renumbering a lane silently repoints every address
// minted under the old numbering at the other column.
var resultLaneFence = []struct {
	lane     resultLane
	ordinal  uint8
	spelling string
}{
	{resultLaneNone, 0, "resultLaneNone"},
	{resultLaneQuery, 1, "resultLaneQuery"},
	{resultLaneObservation, 2, "resultLaneObservation"},
}

func TestResultLaneVocabularyIsTheSealedTable(t *testing.T) {
	for _, row := range resultLaneFence {
		if uint8(row.lane) != row.ordinal {
			t.Errorf("%s is ordinal %d, the fence pins %d; a lane ordinal is part of every result address minted under it", row.spelling, uint8(row.lane), row.ordinal)
		}
	}
	// The table is total over the lane width: a lane added without extending
	// this fence would publish a column no pinned address can name.
	if int(resultLaneObservation)+1 != len(resultLaneFence) {
		t.Errorf("the lane vocabulary spans %d ordinals, the fence table holds %d rows", int(resultLaneObservation)+1, len(resultLaneFence))
	}
}

// TestSolvedLaneSlotsAreFenced pins the absolute slot arithmetic the lane
// vocabulary induces. snapshot_materialize_law_test.go and
// runtime_point_state_snapshot_law_test.go already fence the relations a
// publication must satisfy against these names; this law fences the numbers, so
// a lane change cannot satisfy both sides by moving them together.
func TestSolvedLaneSlotsAreFenced(t *testing.T) {
	if solvedLaneWidth != 3 {
		t.Errorf("solvedLaneWidth is %d, the fence pins 3", solvedLaneWidth)
	}
	if solvedAxisCount != 2 {
		t.Errorf("solvedAxisCount is %d, the fence pins 2", solvedAxisCount)
	}
	if solvedPointSlot != 2 {
		t.Errorf("solvedPointSlot is %d, the fence pins 2", solvedPointSlot)
	}
	if solvedStoreColumns != 3 {
		t.Errorf("solvedStoreColumns is %d, the fence pins 3", solvedStoreColumns)
	}
	if solvedIdentityVersion != 1 {
		t.Errorf("solvedIdentityVersion is %d, the fence pins 1; raising it invalidates every published solved-snapshot identity", solvedIdentityVersion)
	}
}

// solvedVocabularyFence is the published snapshot vocabulary: the column
// outputs, the writer capability, and the four domains that frame a published
// identity's preimage. Every spelling here is either a schema key a consumer
// opens a column by or a domain tag a published ContentID is derived under, so
// respelling one is a silent republication under a different address.
var solvedVocabularyFence = []struct {
	spelling string
	value    string
}{
	{"solvedQueryOutput", "engine/solved-query"},
	{"solvedObservationOutput", "engine/solved-observation"},
	{"solvedPointOutput", "engine/solved-point-state"},
	{"solvedColumnWriter", "engine/solved-publisher"},
	{"solvedSchemaDomain", "engine/solved-snapshot-schema"},
	{"solvedAxisDomain", "engine/solved-snapshot-axis"},
	{"solvedRowKeyDomain", "engine/solved-snapshot-row-key"},
	{"solvedContentDomain", "engine/solved-snapshot-content"},
}

func TestSolvedPublicationVocabularyIsTheSealedTable(t *testing.T) {
	observed := []string{
		string(solvedQueryOutput), string(solvedObservationOutput), string(solvedPointOutput), string(solvedColumnWriter),
		solvedSchemaDomain, solvedAxisDomain, solvedRowKeyDomain, solvedContentDomain,
	}
	if len(observed) != len(solvedVocabularyFence) {
		t.Fatalf("the fence reads %d spellings, the table holds %d rows", len(observed), len(solvedVocabularyFence))
	}
	for index, row := range solvedVocabularyFence {
		if observed[index] != row.value {
			t.Errorf("%s is %q, the fence pins %q", row.spelling, observed[index], row.value)
		}
	}
}
