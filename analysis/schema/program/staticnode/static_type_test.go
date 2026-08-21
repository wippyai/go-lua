package staticnode

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

func TestStaticTypeNodeRejectsInvalidKind(t *testing.T) {
	row, ok := NewStaticTypeNode(StaticTypeNodeSpec{
		ID: identity.ContentID{1}, Owner: identity.ContentID{2}, Kind: StaticNodeInvalid,
	})
	if ok || row.Available() {
		t.Fatal("invalid static node kind was admitted")
	}
}
