package static

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

func TestStaticColdCopiesSchemaAndDraftFence(t *testing.T) {
	var content, namespace identity.ContentID
	content[0], namespace[0] = 1, 2
	component := &Component{contentID: content, schema: []identity.ContentID{namespace}}
	cold := component.Cold()
	if cold.ContentID() != content || cold.SchemaContentCount() != 1 {
		t.Fatalf("Static Cold = %v/%d", cold.ContentID(), cold.SchemaContentCount())
	}
	got, ok := cold.SchemaContentAt(0)
	if !ok || got != namespace {
		t.Fatal("Static Cold schema row did not round-trip")
	}
	cold.schema[0][0] = 9
	if got, _ := component.Cold().SchemaContentAt(0); got != namespace {
		t.Fatal("Static Cold leaked component schema storage")
	}
	draft := &Draft{state: &draftState{component: component, fence: &draftFence{}}}
	if _, ok := draft.Cold().SchemaContentAt(0); !ok {
		t.Fatal("live Static Draft did not expose schema")
	}
	draft.state.consumed = true
	if cold := draft.Cold(); cold.ContentID().Available() {
		t.Fatal("consumed Static Draft retained identity")
	}
}
