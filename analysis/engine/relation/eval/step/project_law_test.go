package step

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/testdata/relationfixture"
	"github.com/wippyai/go-lua/analysis/relation/schema/algebra"
)

// Project is a child of the sealed tree in this law, so it exercises the
// recursive evaluator path rather than manufacturing a dependency/result
// address book for an intermediate node.
func TestExecuteNodeProjectUsesMountedBinding(t *testing.T) {
	fixture := testfixture.New(t, 0xD3)
	session, ok := New(fixture.Mounted(), fixture.BothRoot(), fixture.Geometry())
	if !ok || !session.Available() {
		t.Fatal("evaluator session")
	}
	node, ok := fixture.ProjectNode()
	if !ok || !node.Available() {
		t.Fatal("project node")
	}
	value, ok := session.executeNode(node)
	if !ok || !value.available() || value.kind != algebra.KindProject {
		t.Fatalf("project execution = (%v, %v, %v)", ok, value.available(), value.kind)
	}
	if len(value.batches) == 0 {
		t.Fatal("project lost the source range")
	}
	for _, batch := range value.batches {
		if !batch.ValidFor(fixture.Mounted()) {
			t.Fatal("project returned a foreign batch")
		}
	}
}
