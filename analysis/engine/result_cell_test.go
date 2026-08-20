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

func TestCanonicalResultContractSealsRegistrationIdentity(t *testing.T) {
	family, codec := resultCellID(1), resultCellCodec(67, 1)
	contract, ok := NewCanonicalResultContract(family, codec)
	if !ok || !contract.Available() {
		t.Fatal("canonical contract refused")
	}
	if contract.FamilyID() != family || contract.Codec() != codec || !contract.ContentID().Available() {
		t.Fatal("contract lost sealed registration identity")
	}
}

func TestCanonicalResultCellOwnsCanonicalPayloadAndContract(t *testing.T) {
	contract, ok := NewCanonicalResultContract(resultCellID(1), resultCellCodec(67, 1))
	if !ok {
		t.Fatal("canonical contract")
	}
	payload := []byte{0, 1, 2, 3, 255}
	cell, ok := NewCanonicalResultCell(contract, true, 2, payload)
	if !ok || !cell.Available() || !cell.Present() || cell.RowCount() != 2 {
		t.Fatal("canonical cell refused")
	}
	original := cell.Payload()
	payload[0] = 99
	if cell.Payload() != original || cell.Payload()[0] != 0 {
		t.Fatal("cell retained the encoder's mutable payload")
	}
	if cell.ContractID() != contract.ContentID() || !cell.ContentID().Available() {
		t.Fatal("cell lost sealed contract identity")
	}
}

func TestCanonicalResultContractAndCellIdentityCompleteness(t *testing.T) {
	family, otherFamily := resultCellID(1), resultCellID(2)
	codec, otherCodec := resultCellCodec(67, 1), resultCellCodec(68, 1)
	baseContract, baseContractOK := NewCanonicalResultContract(family, codec)
	otherFamilyContract, otherFamilyOK := NewCanonicalResultContract(otherFamily, codec)
	otherCodecContract, otherCodecOK := NewCanonicalResultContract(family, otherCodec)
	otherVersionContract, otherVersionOK := NewCanonicalResultContract(family, resultCellCodec(67, 2))
	if !baseContractOK || !otherFamilyOK || !otherCodecOK || !otherVersionOK || !baseContract.Available() || !otherFamilyContract.Available() || !otherCodecContract.Available() || !otherVersionContract.Available() {
		t.Fatal("canonical contract")
	}
	if baseContract.ContentID() == otherFamilyContract.ContentID() ||
		baseContract.ContentID() == otherCodecContract.ContentID() ||
		baseContract.ContentID() == otherVersionContract.ContentID() {
		t.Fatal("contract identity omitted registration identity")
	}

	base, baseOK := NewCanonicalResultCell(baseContract, true, 1, []byte("payload"))
	variants := []struct {
		name     string
		contract CanonicalResultContract
		present  bool
		rows     uint64
		payload  []byte
	}{
		{"family", otherFamilyContract, true, 1, []byte("payload")},
		{"codec", otherCodecContract, true, 1, []byte("payload")},
		{"codec version", otherVersionContract, true, 1, []byte("payload")},
		{"presence", baseContract, false, 1, []byte("payload")},
		{"rows", baseContract, true, 2, []byte("payload")},
		{"payload", baseContract, true, 1, []byte("changed")},
	}
	if !baseOK {
		t.Fatal("base canonical cell")
	}
	for _, variant := range variants {
		t.Run(variant.name, func(t *testing.T) {
			cell, ok := NewCanonicalResultCell(variant.contract, variant.present, variant.rows, variant.payload)
			if !ok || cell.ContentID() == base.ContentID() {
				t.Fatal("canonical envelope change did not change identity")
			}
		})
	}
}

func TestCanonicalResultContractRejectsUnsealedShape(t *testing.T) {
	family, codec := resultCellID(1), resultCellCodec(67, 1)
	for name, construct := range map[string]func() (CanonicalResultContract, bool){
		"missing family": func() (CanonicalResultContract, bool) {
			return NewCanonicalResultContract(identity.ContentID{}, codec)
		},
		"missing codec": func() (CanonicalResultContract, bool) {
			return NewCanonicalResultContract(family, identity.SemanticKey{})
		},
	} {
		t.Run(name, func(t *testing.T) {
			if contract, ok := construct(); ok || contract.Available() {
				t.Fatal("invalid canonical contract admitted")
			}
		})
	}
	if cell, ok := NewCanonicalResultCell(CanonicalResultContract{}, false, 0, nil); ok || cell.Available() {
		t.Fatal("cell admitted an unavailable contract")
	}
}

func TestCanonicalResultCellReadsAllocateNothing(t *testing.T) {
	contract, ok := NewCanonicalResultContract(resultCellID(1), resultCellCodec(67, 1))
	if !ok {
		t.Fatal("canonical contract")
	}
	cell, ok := NewCanonicalResultCell(contract, true, 1, []byte("payload"))
	if !ok {
		t.Fatal("canonical cell")
	}
	allocations := testing.AllocsPerRun(1000, func() {
		_ = contract.Available()
		_ = contract.FamilyID()
		_ = contract.Codec()
		_ = contract.ContentID()
		_ = cell.Available()
		_ = cell.ContractID()
		_ = cell.ContentID()
		_ = cell.Present()
		_ = cell.RowCount()
		_ = cell.Payload()
	})
	if allocations != 0 {
		t.Fatalf("canonical result reads allocate: %v", allocations)
	}
}

func BenchmarkCanonicalResultContractConstruct(b *testing.B) {
	family, codec := resultCellID(1), resultCellCodec(67, 1)
	b.ReportAllocs()
	for index := 0; index < b.N; index++ {
		contract, ok := NewCanonicalResultContract(family, codec)
		if !ok {
			b.Fatal("canonical contract")
		}
		_ = contract
	}
}

func BenchmarkCanonicalResultCellConstruct(b *testing.B) {
	contract, ok := NewCanonicalResultContract(resultCellID(1), resultCellCodec(67, 1))
	if !ok {
		b.Fatal("canonical contract")
	}
	payload := make([]byte, 512)
	b.ReportAllocs()
	b.SetBytes(int64(len(payload)))
	for index := 0; index < b.N; index++ {
		cell, ok := NewCanonicalResultCell(contract, true, 8, payload)
		if !ok {
			b.Fatal("canonical cell")
		}
		_ = cell
	}
}

func BenchmarkCanonicalResultCellRead(b *testing.B) {
	contract, ok := NewCanonicalResultContract(resultCellID(1), resultCellCodec(67, 1))
	if !ok {
		b.Fatal("canonical contract")
	}
	cell, ok := NewCanonicalResultCell(contract, true, 8, make([]byte, 512))
	if !ok {
		b.Fatal("canonical cell")
	}
	b.ReportAllocs()
	for index := 0; index < b.N; index++ {
		_ = cell.ContractID()
		_ = cell.ContentID()
		_ = cell.Present()
		_ = cell.RowCount()
		_ = cell.Payload()
	}
}
