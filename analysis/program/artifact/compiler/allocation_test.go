package compiler

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

func TestAllocationTemplateIdentityCommitsConstructorShape(t *testing.T) {
	occurrence := identity.ContentID{1}
	if id := allocationTemplateID(occurrence, allocationClosure, allocationFormEmpty, nil); !id.Available() {
		t.Fatal("empty closure allocation did not receive an identity")
	}
	field := allocationFieldCompileRow{kind: 1, width: 1}
	if id := allocationTemplateID(occurrence, allocationClosure, allocationFormEmpty, []allocationFieldCompileRow{field}); id.Available() {
		t.Fatal("closure allocation admitted table fields")
	}
	if left, right := allocationTemplateID(occurrence, allocationTable, allocationFormEmpty, nil), allocationTemplateID(occurrence, allocationTable, allocationFormClosed, nil); left == right {
		t.Fatal("allocation form mutation did not change identity")
	}
}
