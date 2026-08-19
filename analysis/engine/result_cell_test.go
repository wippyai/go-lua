package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

func resultCellID(seed byte) identity.ContentID {
	var id identity.ContentID
	for index := range id {
		id[index] = seed + byte(index)
	}
	return id
}

func resultCellCodec(seed byte, version uint64) identity.SemanticKey {
	key, ok := identity.NewSemanticKey([32]byte(resultCellID(seed)), version)
	if !ok {
		panic("test codec")
	}
	return key
}

func TestCanonicalResultCellOwnsCanonicalPayloadAndIdentity(t *testing.T) {
	family, codec := resultCellID(1), resultCellCodec(67, 1)
	payload := []byte{0, 1, 2, 3, 255}
	cell, ok := NewCanonicalResultCell(family, codec, true, 2, payload)
	if !ok || !cell.Available() || !cell.Present() || cell.RowCount() != 2 {
		t.Fatal("canonical cell refused")
	}
	original := cell.Payload()
	payload[0] = 99
	if cell.Payload() != original || cell.Payload()[0] != 0 {
		t.Fatal("cell retained the encoder's mutable payload")
	}
	if cell.FamilyID() != family || cell.Codec() != codec || cell.CodecID() != identity.ContentID(codec.Digest()) || cell.CodecVersion() != codec.Version() || !cell.ContentID().Available() {
		t.Fatal("cell lost sealed registration identity")
	}
}

func TestCanonicalResultCellIdentityCoversEnvelopeAndPayload(t *testing.T) {
	family, otherFamily := resultCellID(1), resultCellID(2)
	codec, otherCodec := resultCellCodec(67, 1), resultCellCodec(68, 1)
	base, baseOK := NewCanonicalResultCell(family, codec, true, 1, []byte("payload"))
	variants := []struct {
		name    string
		family  identity.ContentID
		codec   identity.SemanticKey
		present bool
		rows    uint64
		payload []byte
	}{
		{"family", otherFamily, codec, true, 1, []byte("payload")},
		{"codec", family, otherCodec, true, 1, []byte("payload")},
		{"codec version", family, resultCellCodec(67, 2), true, 1, []byte("payload")},
		{"presence", family, codec, false, 1, []byte("payload")},
		{"rows", family, codec, true, 2, []byte("payload")},
		{"payload", family, codec, true, 1, []byte("changed")},
	}
	if !baseOK {
		t.Fatal("base canonical cell")
	}
	for _, variant := range variants {
		t.Run(variant.name, func(t *testing.T) {
			cell, ok := NewCanonicalResultCell(variant.family, variant.codec, variant.present, variant.rows, variant.payload)
			if !ok || cell.ContentID() == base.ContentID() {
				t.Fatal("canonical envelope change did not change identity")
			}
		})
	}
}

func TestCanonicalResultCellRejectsUnsealedShape(t *testing.T) {
	family, codec := resultCellID(1), resultCellCodec(67, 1)
	for name, construct := range map[string]func() (CanonicalResultCell, bool){
		"missing family": func() (CanonicalResultCell, bool) {
			return NewCanonicalResultCell(identity.ContentID{}, codec, false, 0, nil)
		},
		"missing codec": func() (CanonicalResultCell, bool) {
			return NewCanonicalResultCell(family, identity.SemanticKey{}, false, 0, nil)
		},
	} {
		t.Run(name, func(t *testing.T) {
			if cell, ok := construct(); ok || cell.Available() {
				t.Fatal("invalid canonical cell admitted")
			}
		})
	}
}

func TestCanonicalResultCellReadsAllocateNothing(t *testing.T) {
	cell, ok := NewCanonicalResultCell(resultCellID(1), resultCellCodec(67, 1), true, 1, []byte("payload"))
	if !ok {
		t.Fatal("canonical cell")
	}
	allocations := testing.AllocsPerRun(1000, func() {
		_ = cell.Available()
		_ = cell.FamilyID()
		_ = cell.Codec()
		_ = cell.CodecID()
		_ = cell.CodecVersion()
		_ = cell.ContentID()
		_ = cell.Present()
		_ = cell.RowCount()
		_ = cell.Payload()
	})
	if allocations != 0 {
		t.Fatalf("canonical cell reads allocate: %v", allocations)
	}
}

func BenchmarkCanonicalResultCellConstruct(b *testing.B) {
	family, codec := resultCellID(1), resultCellCodec(67, 1)
	payload := make([]byte, 512)
	b.ReportAllocs()
	b.SetBytes(int64(len(payload)))
	for index := 0; index < b.N; index++ {
		cell, ok := NewCanonicalResultCell(family, codec, true, 8, payload)
		if !ok {
			b.Fatal("canonical cell")
		}
		_ = cell
	}
}

func BenchmarkCanonicalResultCellRead(b *testing.B) {
	cell, ok := NewCanonicalResultCell(resultCellID(1), resultCellCodec(67, 1), true, 8, make([]byte, 512))
	if !ok {
		b.Fatal("canonical cell")
	}
	b.ReportAllocs()
	for index := 0; index < b.N; index++ {
		_ = cell.ContentID()
		_ = cell.Payload()
		_ = cell.RowCount()
	}
}
