package equation

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/factkey"
	"github.com/wippyai/go-lua/analysis/domain/typestate"
)

func TestFamilyValuesFencesTypestateKeyAndPayload(t *testing.T) {
	identity := []byte("resource-id")
	publication, ok := typestate.AcquirePublication(
		typestate.Resource{ID: typestate.ResourceID(string(identity)), Protocol: typestate.ProtocolConnection},
		typestate.StateOpen,
		typestate.Obligation{Final: typestate.StateClosed},
	)
	if !ok {
		t.Fatal("construct publication")
	}
	encoded, ok := typestate.EncodePublication(publication)
	if !ok {
		t.Fatal("encode publication")
	}
	key := factkey.BuildKey(
		factkey.LifecycleResourceState,
		[]factkey.Part{factkey.IdentityPart(identity)},
		"op-1",
	)
	wrongKey := factkey.BuildKey(
		factkey.LifecycleResourceState,
		[]factkey.Part{factkey.IdentityPart([]byte("other-resource"))},
		"op-1",
	)
	partition, err := PartitionFromClosuresWithGuards(nil, OutputClosure{Values: []Fact{
		{Key: key.String(), Value: encoded},
		{Key: wrongKey.String(), Value: encoded},
	}})
	if err != nil {
		t.Fatal(err)
	}

	values := partition.FamilyValues(factkey.BuildKey(
		factkey.LifecycleResourceState,
		[]factkey.Part{factkey.IdentityPart(identity)},
		"",
	))
	value, found := values.Next()
	if !found {
		t.Fatal("missing typestate family value")
	}
	got, decoded := value.DecodedTypestatePublication()
	if !decoded || got != publication {
		t.Fatalf("decoded publication = %#v/%v, want %#v", got, decoded, publication)
	}

	values = partition.FamilyValues(factkey.BuildKey(
		factkey.LifecycleResourceState,
		[]factkey.Part{factkey.IdentityPart([]byte("other-resource"))},
		"",
	))
	value, found = values.Next()
	if !found {
		t.Fatal("missing mismatched typestate family value")
	}
	if _, decoded := value.DecodedTypestatePublication(); decoded {
		t.Fatal("payload whose resource disagrees with its key was accepted")
	}

	bytesPartition, err := PartitionFromClosuresWithGuards(nil, OutputClosure{Values: []Fact{{
		Key:   factkey.BuildKey(factkey.LifecycleChannelDisplay, []factkey.Part{factkey.EncodedTermPart(identity)}, "op-1").String(),
		Value: encoded,
	}}})
	if err != nil {
		t.Fatal(err)
	}
	bytesValues := bytesPartition.FamilyValues(factkey.BuildKey(
		factkey.LifecycleChannelDisplay,
		[]factkey.Part{factkey.EncodedTermPart(identity)},
		"",
	))
	value, found = bytesValues.Next()
	if !found {
		t.Fatal("missing byte family value")
	}
	if _, decoded := value.DecodedTypestatePublication(); decoded {
		t.Fatal("non-typestate family payload was decoded as typestate")
	}
}
