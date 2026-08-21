package factor_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/target/vocabulary"
	effectfactor "github.com/wippyai/go-lua/domain/effect/factor"
)

var (
	mountedPublicationBatchIndexBatchSink effectfactor.MountedPublicationBatch
	mountedPublicationBatchIndexOKSink    bool
)

func TestMountedPublicationBatchIndexRetainsZeroRowBatch(t *testing.T) {
	fixture := newEffectFactorFixture(t, effectFactorSpec(vocabulary.RowClosed, false), "local function sink(value) return value end\nsink(1)")
	index, indexOK := effectfactor.NewMountedPublicationBatchIndex(fixture.factor)
	if !indexOK || index == nil || !index.Valid() || index.Count() != fixture.factor.MountedCallCount() {
		t.Fatalf("publication batch index = %#v/%v/%d", index, indexOK, index.Count())
	}
	batch, batchOK := fixture.factor.PublicationBatchForMountedCall(fixture.mountedCall)
	if !batchOK || !batch.Valid() || batch.RowCount() != 0 {
		t.Fatalf("zero-row source batch = %d/%v/%v", batch.RowCount(), batchOK, batch.Valid())
	}
	id, idOK := batch.SealedContentID()
	module, occurrence, provenanceOK := batch.CallProvenance()
	if !idOK || !provenanceOK {
		t.Fatal("zero-row batch provenance")
	}
	byID, byIDOK := index.BatchForContentID(id)
	byCall, byCallOK := index.BatchForCall(module, occurrence)
	byMounted, byMountedOK := index.BatchForMountedCall(fixture.mountedCall)
	byOrdinal, byOrdinalOK := index.BatchAt(0)
	byIDIdentity, byIDIdentityOK := byID.SealedContentID()
	byCallIdentity, byCallIdentityOK := byCall.SealedContentID()
	byMountedIdentity, byMountedIdentityOK := byMounted.SealedContentID()
	byOrdinalIdentity, byOrdinalIdentityOK := byOrdinal.SealedContentID()
	if !byIDOK || !byCallOK || !byMountedOK || !byOrdinalOK || !byIDIdentityOK || !byCallIdentityOK || !byMountedIdentityOK || !byOrdinalIdentityOK || !index.Owns(batch) || byIDIdentity != byCallIdentity || byCallIdentity != byMountedIdentity || byMountedIdentity != byOrdinalIdentity {
		t.Fatal("index did not retain the exact zero-row batch across all sealed indexes")
	}
}

func TestMountedPublicationBatchIndexRejectsForeignAndUnknownLookups(t *testing.T) {
	fixture := newEffectFactorFixture(t, publicationEffectFactorSpec(vocabulary.PublicationEffectSendTransfer, true), "local function sink(left, right) return left end\nsink(1, 2)")
	index, indexOK := effectfactor.NewMountedPublicationBatchIndex(fixture.factor)
	if !indexOK || index == nil {
		t.Fatal("publication batch index")
	}
	batch, batchOK := index.BatchAt(0)
	if !batchOK {
		t.Fatal("indexed publication batch")
	}
	id, idOK := batch.SealedContentID()
	if !idOK {
		t.Fatal("indexed batch ID")
	}
	id[0] ^= 0xff
	if _, ok := index.BatchForContentID(id); ok {
		t.Fatal("index admitted unknown sealed ID")
	}
	if _, ok := index.BatchForCall(identity.ContentID{}, identity.ContentID{}); ok {
		t.Fatal("index admitted unavailable call coordinate")
	}
	foreign, foreignOK := effectfactor.NewWithMountedArtifacts(fixture.linked, fixture.packs, fixture.contract, fixture.mounts)
	if !foreignOK || foreign == nil {
		t.Fatal("foreign same-content Effect algebra")
	}
	foreignMounted, foreignMountedOK := foreign.MountedCallAt(0)
	if !foreignMountedOK {
		t.Fatal("foreign mounted call")
	}
	foreignBatch, foreignBatchOK := foreign.PublicationBatchForMountedCall(foreignMounted)
	if !foreignBatchOK || index.Owns(foreignBatch) {
		t.Fatal("index admitted foreign Effect batch")
	}
	if _, ok := index.BatchForMountedCall(foreignMounted); ok {
		t.Fatal("index admitted foreign mounted call")
	}
}

func BenchmarkMountedPublicationBatchIndexLookup(b *testing.B) {
	fixture := newEffectFactorFixture(b, publicationEffectFactorSpec(vocabulary.PublicationEffectSendTransfer, true), "local function sink(left, right) return left end\nsink(1, 2)")
	index, indexOK := effectfactor.NewMountedPublicationBatchIndex(fixture.factor)
	if !indexOK || index == nil {
		b.Fatal("publication batch index")
	}
	batch, batchOK := index.BatchAt(0)
	if !batchOK {
		b.Fatal("indexed publication batch")
	}
	id, idOK := batch.SealedContentID()
	module, occurrence, provenanceOK := batch.CallProvenance()
	if !idOK || !provenanceOK {
		b.Fatal("indexed batch identity")
	}
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		byID, byIDOK := index.BatchForContentID(id)
		byCall, byCallOK := index.BatchForCall(module, occurrence)
		mountedPublicationBatchIndexBatchSink = byID
		byIDIdentity, byIDIdentityOK := byID.SealedContentID()
		byCallIdentity, byCallIdentityOK := byCall.SealedContentID()
		mountedPublicationBatchIndexOKSink = byIDOK && byCallOK && byIDIdentityOK && byCallIdentityOK && byIDIdentity == byCallIdentity
	}
}
