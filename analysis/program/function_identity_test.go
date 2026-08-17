package program

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow"
)

func TestFunctionIdentityRequiresAnOwnedFlowBoundary(t *testing.T) {
	published, err := Publish(rootAssembly(t, "program-function-identity-law.lua"))
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	var boundary flow.FunctionBoundary
	if got, ok := published.FunctionID(boundary); ok || got.Available() {
		t.Fatalf("FunctionID(zero) = %x/%v; want unavailable", got, ok)
	}
	if formal, cell, storage, declared, ok := published.FunctionFormalAt(boundary, -1); ok || formal.Available() || cell.Available() || storage.Available() || declared.Available() {
		t.Fatalf("FunctionFormalAt(zero,-1) returned identities: %x/%x/%x/%x/%v", formal, cell, storage, declared, ok)
	}
	if id, ok := published.StorageCellID(0); ok || id.Available() {
		t.Fatalf("StorageCellID(0) = %x/%v for a missing cell row", id, ok)
	}
	if id, cell, ok := published.FunctionVararg(boundary); ok || id.Available() || cell.Available() {
		t.Fatalf("FunctionVararg(zero) = %x/%x/%v; want unavailable", id, cell, ok)
	}
	if id, inner, outer, innerBody, outerBody, ok := published.FunctionCaptureAt(boundary, 0); ok || id.Available() || inner.Available() || outer.Available() || innerBody.Available() || outerBody.Available() {
		t.Fatalf("FunctionCaptureAt(zero,0) returned identities: %x/%x/%x/%x/%x/%v", id, inner, outer, innerBody, outerBody, ok)
	}
}
