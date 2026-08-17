// publication_column_law_test.go states the write-capability laws: a published
// column is filled through the one capability the engine minted for it and
// through nothing else, and the published value carries none.

package engine

import (
	"errors"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
	"github.com/wippyai/go-lua/analysis/snapshot"
)

const (
	lawColumnOutput schema.Key = "law/column"
	lawPeerOutput   schema.Key = "law/peer"
	lawColumnWriter schema.Key = "law/writer"
	lawOtherWriter  schema.Key = "law/other-writer"
)

var (
	lawColumnTable      = identity.ContentID{0x21, 0x01}
	lawColumnStore      = identity.StoreID(11)
	lawColumnGeneration = identity.Generation(1)
	lawColumnAxis       = snapshot.Axis[uint64, uint64]{SchemaID: lawColumnTable, Slot: 0}
)

// lawColumnAdmissions is the sealed table's issuance request for the two law
// columns, in slot order.
func lawColumnAdmissions() []ColumnAdmission {
	return []ColumnAdmission{
		{Schema: lawColumnTable, Output: lawColumnOutput, Writer: lawColumnWriter, Slot: 0},
		{Schema: lawColumnTable, Output: lawPeerOutput, Writer: lawColumnWriter, Slot: 1},
	}
}

// lawOpenColumnBinding returns an open binding whose factor and query cells are
// complete, so the only thing left to state about it is its column admissions.
func lawOpenColumnBinding(t testing.TB) *SchemaBinding {
	t.Helper()
	_, factor, query := exactQuerySchemaFixture(t)
	binding := NewSchemaBinding(factor.Schema())
	if binding == nil || !BindFactor(binding, factor, hotUintFactorSpec()) || !BindExactQuery(binding, query, factor, hotExactQuerySpec()) {
		t.Fatal("law column binding")
	}
	return binding
}

// lawSealedColumnBinding seals one binding that admits the two law columns.
func lawSealedColumnBinding(t testing.TB) *SchemaBinding {
	t.Helper()
	binding := lawOpenColumnBinding(t)
	if !AdmitColumns(binding, lawColumnAdmissions()) || !binding.Seal() {
		t.Fatal("law column admission")
	}
	return binding
}

// TestColumnWriteWithoutAMintedCapabilityIsRefused states the forgery law. A
// ColumnWrite holds one unexported grant, so the only value a package outside
// the engine can produce is the zero one, and the zero one unlocks no column.
// The same builder the forged capability is refused on is filled by the minted
// one, so what the refusal states is the capability and not the builder.
func TestColumnWriteWithoutAMintedCapabilityIsRefused(t *testing.T) {
	binding := lawSealedColumnBinding(t)
	builder := snapshot.NewBuilder(lawColumnTable, lawColumnStore, lawColumnGeneration)
	var forged ColumnWrite[uint64, uint64]
	if forged.Available() {
		t.Fatal("the zero write capability reports itself available")
	}
	if err := PublishColumn(forged, &builder, snapshot.Content[uint64, uint64]{Rows: map[uint64]uint64{1: 10}}); !errors.Is(err, ErrUnauthorizedColumnWrite) {
		t.Fatalf("a forged capability sealed a column: %v", err)
	}
	if err := PublishRow(forged, &builder, 1, 10); !errors.Is(err, ErrUnauthorizedColumnWrite) {
		t.Fatalf("a forged capability published a row: %v", err)
	}
	if err := WithdrawRow(forged, &builder, 1); !errors.Is(err, ErrUnauthorizedColumnWrite) {
		t.Fatalf("a forged capability withdrew a row: %v", err)
	}
	write, minted := MintColumnWrite[uint64, uint64](binding, lawColumnOutput, lawColumnWriter)
	if !minted || !write.Available() {
		t.Fatal("the sealed admission mints no write capability")
	}
	if err := PublishColumn(write, &builder, snapshot.Content[uint64, uint64]{Rows: map[uint64]uint64{1: 10}}); err != nil {
		t.Fatalf("the minted capability sealed no column: %v", err)
	}
	if err := PublishRow(write, &builder, 2, 20); err != nil {
		t.Fatalf("the minted capability published no row: %v", err)
	}
	if err := WithdrawRow(write, &builder, 2); err != nil {
		t.Fatalf("the minted capability withdrew no row: %v", err)
	}
}

// TestOneColumnMintsOneWriteCapability states the runtime end of the seal's
// one-writer law. The table admits one writer per column, so the engine mints
// one capability per column: a second mint of the same column is refused
// whatever key and value types it claims, and a mint naming a writer the table
// did not admit is refused as well.
func TestOneColumnMintsOneWriteCapability(t *testing.T) {
	binding := lawSealedColumnBinding(t)
	if _, minted := MintColumnWrite[uint64, uint64](binding, lawColumnOutput, lawColumnWriter); !minted {
		t.Fatal("the admitted column mints no write capability")
	}
	if _, minted := MintColumnWrite[uint64, uint64](binding, lawColumnOutput, lawColumnWriter); minted {
		t.Fatal("one column minted a second write capability")
	}
	if _, minted := MintColumnWrite[string, uint64](binding, lawColumnOutput, lawColumnWriter); minted {
		t.Fatal("a second claim over one column minted another write capability")
	}
	if _, minted := MintColumnWrite[uint64, uint64](binding, lawPeerOutput, lawOtherWriter); minted {
		t.Fatal("a writer the table never admitted minted a write capability")
	}
	if _, minted := MintColumnWrite[uint64, uint64](binding, "law/undeclared", lawColumnWriter); minted {
		t.Fatal("a column the table never declared minted a write capability")
	}
	if _, minted := MintColumnWrite[uint64, uint64](binding, lawPeerOutput, lawColumnWriter); !minted {
		t.Fatal("the second admitted column mints no write capability of its own")
	}
}

// TestWriteCapabilityRequiresASealedAdmission states that the admitted set and
// the minted capability are one law at two ends. A binding that has not sealed
// its admissions mints nothing, and an admission stated after the seal reaches
// no column.
func TestWriteCapabilityRequiresASealedAdmission(t *testing.T) {
	binding := lawOpenColumnBinding(t)
	if !AdmitColumns(binding, lawColumnAdmissions()) {
		t.Fatal("the open binding admits no columns")
	}
	if _, minted := MintColumnWrite[uint64, uint64](binding, lawColumnOutput, lawColumnWriter); minted {
		t.Fatal("an unsealed binding minted a write capability")
	}
	if !binding.Seal() {
		t.Fatal("the admitted binding does not seal")
	}
	if AdmitColumns(binding, lawColumnAdmissions()) {
		t.Fatal("a sealed binding admitted a column")
	}
	if _, minted := MintColumnWrite[uint64, uint64](binding, lawColumnOutput, lawColumnWriter); !minted {
		t.Fatal("the sealed admission mints no write capability")
	}
}

// TestAdmittedColumnSetIsStatedOnceAndWithoutCollision states the admission's
// own law. The set names each column once and each slot once, and it is stated
// once: a table that named one column twice, or two columns one slot, would
// leave the engine holding two writers for one column.
func TestAdmittedColumnSetIsStatedOnceAndWithoutCollision(t *testing.T) {
	restated := lawOpenColumnBinding(t)
	if !AdmitColumns(restated, lawColumnAdmissions()) {
		t.Fatal("the open binding admits no columns")
	}
	if AdmitColumns(restated, lawColumnAdmissions()) {
		t.Fatal("the admitted column set was stated twice")
	}
	if restated.Seal() || !restated.Poisoned() {
		t.Fatal("a binding whose admissions were restated sealed anyway")
	}

	duplicateOutput := lawOpenColumnBinding(t)
	if AdmitColumns(duplicateOutput, []ColumnAdmission{
		{Schema: lawColumnTable, Output: lawColumnOutput, Writer: lawColumnWriter, Slot: 0},
		{Schema: lawColumnTable, Output: lawColumnOutput, Writer: lawOtherWriter, Slot: 1},
	}) {
		t.Fatal("one column was admitted for two writers")
	}

	duplicateSlot := lawOpenColumnBinding(t)
	if AdmitColumns(duplicateSlot, []ColumnAdmission{
		{Schema: lawColumnTable, Output: lawColumnOutput, Writer: lawColumnWriter, Slot: 0},
		{Schema: lawColumnTable, Output: lawPeerOutput, Writer: lawColumnWriter, Slot: 0},
	}) {
		t.Fatal("two columns were admitted into one slot")
	}

	foreignTable := lawOpenColumnBinding(t)
	if AdmitColumns(foreignTable, []ColumnAdmission{
		{Schema: lawColumnTable, Output: lawColumnOutput, Writer: lawColumnWriter, Slot: 0},
		{Schema: identity.ContentID{0x99}, Output: lawPeerOutput, Writer: lawColumnWriter, Slot: 1},
	}) {
		t.Fatal("columns of two tables were admitted into one publication")
	}

	incomplete := lawOpenColumnBinding(t)
	if AdmitColumns(incomplete, []ColumnAdmission{{Schema: lawColumnTable, Output: lawColumnOutput, Slot: 0}}) {
		t.Fatal("a column with no admitted writer was admitted")
	}
}

// TestPublishedSnapshotCarriesNoWriteCapability states that the capability
// lives with the engine and never with the value. A reader recovers a
// published row from an address alone, and the capability that wrote a sealed
// publication cannot reach back into it: the next generation it writes is a new
// value and the sealed one still answers exactly what it was sealed with.
func TestPublishedSnapshotCarriesNoWriteCapability(t *testing.T) {
	binding := lawSealedColumnBinding(t)
	write, minted := MintColumnWrite[uint64, uint64](binding, lawColumnOutput, lawColumnWriter)
	peer, peerMinted := MintColumnWrite[uint64, uint64](binding, lawPeerOutput, lawColumnWriter)
	if !minted || !peerMinted {
		t.Fatal("the sealed admission mints no write capability")
	}
	builder := snapshot.NewBuilder(lawColumnTable, lawColumnStore, lawColumnGeneration)
	if err := PublishColumn(write, &builder, snapshot.Content[uint64, uint64]{Rows: map[uint64]uint64{1: 10}}); err != nil {
		t.Fatalf("seal law column: %v", err)
	}
	if err := PublishColumn(peer, &builder, snapshot.Content[uint64, uint64]{}); err != nil {
		t.Fatalf("seal law peer column: %v", err)
	}
	sealed, err := builder.Seal()
	if err != nil {
		t.Fatalf("seal law publication: %v", err)
	}
	if value, status := snapshot.Read(&sealed, lawColumnAxis, 1); status != snapshot.ReadHit || value != 10 {
		t.Fatalf("the published row reads %d as %s from its address alone", value, status)
	}

	next := snapshot.NewDelta(sealed, lawColumnGeneration+1)
	if err := PublishRow(write, &next, 1, 99); err != nil {
		t.Fatalf("publish the next generation: %v", err)
	}
	following, err := next.Seal()
	if err != nil {
		t.Fatalf("seal the next generation: %v", err)
	}
	if value, status := snapshot.Read(&sealed, lawColumnAxis, 1); status != snapshot.ReadHit || value != 10 {
		t.Fatalf("the sealed publication moved to %d (%s) under a held capability", value, status)
	}
	if value, status := snapshot.Read(&following, lawColumnAxis, 1); status != snapshot.ReadHit || value != 99 {
		t.Fatalf("the following publication reads %d as %s", value, status)
	}
}
