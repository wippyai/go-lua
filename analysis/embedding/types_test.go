package embedding

import "testing"

func TestDocumentIDIdentityExcludesSnapshotRevision(t *testing.T) {
	id := RegistryDocument("component:worker/source:lua")
	first, err := (SourceSnapshot{Document: id, ProviderRevision: "registry-41", Content: []byte("return 1")}).Verify()
	if err != nil {
		t.Fatal(err)
	}
	second, err := (SourceSnapshot{Document: id, ProviderRevision: "registry-42", Content: []byte("return 2")}).Verify()
	if err != nil {
		t.Fatal(err)
	}
	if first.Document != second.Document {
		t.Fatalf("document identity changed across revisions: %#v != %#v", first.Document, second.Document)
	}
	if first.ContentDigest == second.ContentDigest {
		t.Fatal("different content has the same digest")
	}
}

func TestSourceSnapshotRejectsClaimedMismatchedDigest(t *testing.T) {
	_, err := (SourceSnapshot{
		Document:      MemDocument("test"),
		ContentDigest: DigestBytes([]byte("old")),
		Content:       []byte("new"),
	}).Verify()
	if err == nil {
		t.Fatal("Verify accepted a mismatched provider digest")
	}
}
