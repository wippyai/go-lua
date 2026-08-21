package staticnode

import (
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

type staticIdentityReplay struct{ fields []string }

func (replay *staticIdentityReplay) WriteContentID(value identity.ContentID) bool {
	replay.fields = append(replay.fields, fmt.Sprintf("id:%x", value))
	return true
}
func (replay *staticIdentityReplay) WriteUint(value uint64) bool {
	replay.fields = append(replay.fields, fmt.Sprintf("uint:%d", value))
	return true
}
func (replay *staticIdentityReplay) WriteBool(value bool) bool {
	replay.fields = append(replay.fields, fmt.Sprintf("bool:%t", value))
	return true
}
func (replay *staticIdentityReplay) WriteString(value string) bool {
	replay.fields = append(replay.fields, "string:"+value)
	return true
}

func TestWriteArtifactIdentityFieldsReplaysRecordMetadataAndChildren(t *testing.T) {
	parentID := staticnodeLawID(t, "identity-parent")
	childID := staticnodeLawID(t, "identity-child")
	parent, parentOK := NewStaticTypeNode(StaticTypeNodeSpec{
		ID: parentID, Owner: staticnodeLawID(t, "identity-owner"), Kind: StaticNodeRecord,
		Name: "Record", RecordFieldOffset: 0, RecordFieldCount: 1,
	})
	field, fieldOK := NewStaticTypeNodeRecordField(parentID, childID, keyspace.Key(7), "member", true, true, 0)
	if !parentOK || !fieldOK {
		t.Fatal("static identity fixture construction failed")
	}
	view := staticnodeLawView(t, Publication{
		StaticTypeNodes:            []StaticTypeNode{parent},
		StaticTypeNodeRecordFields: []StaticTypeNodeRecordField{field},
	}, staticnodeLawID(t, "identity-catalog"))
	var first, second staticIdentityReplay
	if !view.WriteArtifactIdentityFields(&first) || !view.WriteArtifactIdentityFields(&second) {
		t.Fatal("static identity replay failed")
	}
	if fmt.Sprint(first.fields) != fmt.Sprint(second.fields) {
		t.Fatal("static identity replay was not deterministic")
	}
	for _, required := range []string{"uint:1", "string:Record", "string:member", "bool:true", fmt.Sprintf("id:%x", childID)} {
		found := false
		for _, field := range first.fields {
			if field == required {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("static identity replay omitted %q: %v", required, first.fields)
		}
	}
}
