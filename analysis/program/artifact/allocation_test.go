package artifact

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow"
)

func TestAllocationTemplateIdentityCommitsConstructorShape(t *testing.T) {
	occurrence := identity.ContentID{1}
	if id := allocationTemplateID(occurrence, flow.AllocationClosure, flow.AllocationFormEmpty, nil); !id.Available() {
		t.Fatal("empty closure allocation did not receive an identity")
	}
	field := allocationFieldCompileRow{kind: 1, width: 1}
	if id := allocationTemplateID(occurrence, flow.AllocationClosure, flow.AllocationFormEmpty, []allocationFieldCompileRow{field}); id.Available() {
		t.Fatal("closure allocation admitted table fields")
	}
	if left, right := allocationTemplateID(occurrence, flow.AllocationTable, flow.AllocationFormEmpty, nil), allocationTemplateID(occurrence, flow.AllocationTable, flow.AllocationFormClosed, nil); left == right {
		t.Fatal("allocation form mutation did not change identity")
	}
}
