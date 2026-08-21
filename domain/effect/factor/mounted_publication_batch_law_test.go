package factor_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	effectfactor "github.com/wippyai/go-lua/domain/effect/factor"
)

func TestMountedPublicationBatchAllowsZeroRows(t *testing.T) {
	fixture := newEffectFactorFixture(t, effectFactorSpec(vocabulary.RowClosed, false), "local function sink(value) return value end\nsink(1)")
	batch, ok := fixture.factor.PublicationBatchForMountedCall(fixture.mountedCall)
	if !ok || !batch.Valid() || batch.RowCount() != 0 {
		t.Fatalf("zero-row publication batch = %d/%v/%v", batch.RowCount(), ok, batch.Valid())
	}
	rows := batch.Rows()
	if len(rows) != 0 {
		t.Fatalf("zero-row batch Rows() = %d", len(rows))
	}
	if id, idOK := batch.ContentID(); !idOK || !id.Available() {
		t.Fatal("zero-row batch has no stable identity")
	}
}

func TestMountedPublicationBatchPreservesOperationAndReceiptOrder(t *testing.T) {
	fixture := newEffectFactorFixture(t, publicationEffectFactorSpec(vocabulary.PublicationEffectSendTransfer, true), "local function sink(left, right) return left end\nsink(1, 2)")
	first, firstOK := fixture.factor.PublicationBatchForMountedCall(fixture.mountedCall)
	second, secondOK := fixture.factor.PublicationBatchForMountedCall(fixture.mountedCall)
	if !firstOK || !secondOK || !first.Valid() || !second.Valid() || first.RowCount() != 2 || second.RowCount() != 2 {
		t.Fatalf("publication batches = %d/%d (%v/%v)", first.RowCount(), second.RowCount(), firstOK, secondOK)
	}
	firstID, firstIDOK := first.ContentID()
	secondID, secondIDOK := second.ContentID()
	if !firstIDOK || !secondIDOK || firstID != secondID {
		t.Fatal("repeated batch issuance changed ContentID")
	}
	rows := first.Rows()
	if len(rows) != 2 || rows[0].Role() != effectfactor.MountedPublicationOrdinary || rows[1].Role() != effectfactor.MountedPublicationCallback {
		t.Fatal("batch did not preserve ordinary-then-callback receipt order")
	}
	rows[0] = effectfactor.MountedPublication{}
	if !first.Valid() || first.RowCount() != 2 {
		t.Fatal("mutating defensive Rows copy changed sealed batch")
	}
	firstRow, firstRowOK := first.RowAt(0)
	secondRow, secondRowOK := second.RowAt(0)
	firstRowID, firstRowIDOK := firstRow.ContentID()
	secondRowID, secondRowIDOK := secondRow.ContentID()
	if !firstRowOK || !secondRowOK || !firstRowIDOK || !secondRowIDOK || firstRowID != secondRowID {
		t.Fatal("repeated batch issuance changed receipt order")
	}
}

func TestMountedPublicationBatchRejectsForeignMountedCall(t *testing.T) {
	fixture := newEffectFactorFixture(t, publicationEffectFactorSpec(vocabulary.PublicationEffectSendTransfer, true), "local function sink(left, right) return left end\nsink(1, 2)")
	foreign, ok := effectfactor.NewWithMountedArtifacts(fixture.linked, fixture.packs, fixture.contract, fixture.mounts)
	if !ok || foreign == nil {
		t.Fatal("foreign same-content Effect algebra")
	}
	foreignMounted, ok := foreign.MountedCallAt(0)
	if !ok {
		t.Fatal("foreign mounted call")
	}
	if _, ok := fixture.factor.PublicationBatchForMountedCall(foreignMounted); ok {
		t.Fatal("batch admitted a foreign Effect mounted call")
	}
}
