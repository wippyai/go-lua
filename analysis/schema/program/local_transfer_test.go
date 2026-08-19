package programschema

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
)

func localTransferLawID(t *testing.T, label string) identity.ContentID {
	t.Helper()
	id, ok := identity.DeriveContentID("analysis/schema/program/local-transfer-law", []byte(label))
	if !ok {
		t.Fatalf("derive %s", label)
	}
	return id
}

func TestLocalTransferPublishesOwnedOrderedWriteSpan(t *testing.T) {
	keys := []schema.Key{"call-dispatch", "value-source"}
	writes := make([]LocalTransferWrite, len(keys))
	for index, key := range keys {
		var ok bool
		writes[index], ok = NewLocalTransferWrite(key)
		if !ok {
			t.Fatalf("write %d", index)
		}
	}
	transfer, ok := NewLocalTransfer(
		localTransferLawID(t, "transfer"), localTransferLawID(t, "from"), localTransferLawID(t, "to"),
		false, 0, uint32(len(writes)),
	)
	if !ok {
		t.Fatal("partial transfer")
	}
	catalog, _ := CatalogID(localTransferLawID(t, "schema"))
	frozen, sealed := (Publication{LocalTransfers: []LocalTransfer{transfer}, LocalTransferWrites: writes}).Seal(catalog, identity.StoreID(1))
	program := Program{
		Frozen: frozen, ArtifactID: localTransferLawID(t, "artifact"),
		ProgramID: localTransferLawID(t, "program"), SchemaID: localTransferLawID(t, "schema"),
	}
	if !sealed || !program.Available() {
		t.Fatal("publication")
	}
	for index, want := range keys {
		row, held := program.LocalTransferWriteFor(0, index)
		got, available := row.Key()
		if !held || !available || got != want {
			t.Fatalf("write %d = %q/%v/%v, want %q", index, got, held, available, want)
		}
	}
	if _, held := program.LocalTransferWriteFor(0, len(keys)); held {
		t.Fatal("read past owned write span")
	}
}

func TestLocalTransferRejectsMixedFullAndFactorShape(t *testing.T) {
	id, from, to := localTransferLawID(t, "transfer"), localTransferLawID(t, "from"), localTransferLawID(t, "to")
	if _, ok := NewLocalTransfer(id, from, to, true, 0, 1); ok {
		t.Fatal("full transfer with factor write was accepted")
	}
	if _, ok := NewLocalTransfer(id, from, to, false, 0, 0); ok {
		t.Fatal("partial transfer without factor write was accepted")
	}
}
