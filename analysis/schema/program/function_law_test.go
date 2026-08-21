package programschema

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

func functionLawID(seed byte) identity.ContentID {
	var id identity.ContentID
	for index := range id {
		id[index] = seed + byte(index)
	}
	return id
}

func TestFunctionCaptureCarriesCallableAndStorageBridgesTogether(t *testing.T) {
	id, inner, outer := functionLawID(1), functionLawID(2), functionLawID(3)
	innerStorage, outerStorage := functionLawID(4), functionLawID(5)
	innerBody, outerBody := functionLawID(6), functionLawID(7)
	row, ok := NewFunctionCapture(id, inner, outer, innerStorage, outerStorage, innerBody, outerBody, 0)
	if !ok || !row.Available() {
		t.Fatal("complete FunctionCapture bridge was not admitted")
	}
	if row.InnerCellID() != inner || row.OuterCellID() != outer ||
		row.InnerStorageCellID() != innerStorage || row.OuterStorageCellID() != outerStorage ||
		row.InnerBodyID() != innerBody || row.OuterBodyID() != outerBody {
		t.Fatal("FunctionCapture lost one callable/storage/body bridge column")
	}
}

func TestFunctionCaptureRejectsIncompleteOrAliasedStorageBridge(t *testing.T) {
	id, inner, outer := functionLawID(1), functionLawID(2), functionLawID(3)
	innerBody, outerBody := functionLawID(6), functionLawID(7)
	validStorage := functionLawID(4)
	missingInner, ok := NewFunctionCapture(id, inner, outer, identity.ContentID{}, validStorage, innerBody, outerBody, 0)
	if ok || missingInner.Available() {
		t.Fatal("FunctionCapture admitted a missing inner storage identity")
	}
	missingOuter, ok := NewFunctionCapture(id, inner, outer, validStorage, identity.ContentID{}, innerBody, outerBody, 0)
	if ok || missingOuter.Available() {
		t.Fatal("FunctionCapture admitted a missing outer storage identity")
	}
	aliased, ok := NewFunctionCapture(id, inner, outer, validStorage, validStorage, innerBody, outerBody, 0)
	if ok || aliased.Available() {
		t.Fatal("FunctionCapture admitted aliased inner/outer storage identities")
	}
}
