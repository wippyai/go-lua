package lsp

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/embedding"
)

func TestFileDocumentCodecRoundTrip(t *testing.T) {
	codec := FileDocumentCodec{}
	for _, document := range []embedding.DocumentID{
		embedding.FileDocument("/workspace/main.lua"),
		embedding.FileDocument("/workspace/with space/猫.lua"),
	} {
		uri, err := codec.URIForDocument(document)
		if err != nil {
			t.Fatalf("URIForDocument(%q): %v", document, err)
		}
		got, err := codec.DocumentForURI(uri)
		if err != nil {
			t.Fatalf("DocumentForURI(%q): %v", uri, err)
		}
		if got != document {
			t.Fatalf("%q -> %q -> %q", document, uri, got)
		}
	}
	if _, err := codec.DocumentForURI("file:///workspace/with%20space/%e7%8c%ab.lua"); err == nil {
		t.Fatal("non-canonical percent encoding was accepted")
	}
	if _, err := codec.DocumentForURI("file://localhost/workspace/main.lua"); err == nil {
		t.Fatal("hosted file URI was accepted")
	}
}
