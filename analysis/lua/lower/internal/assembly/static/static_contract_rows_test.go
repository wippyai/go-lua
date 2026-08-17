package static

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestStaticContractRowsSeparateFunctionAndCallSidecars(t *testing.T) {
	rows := &staticRows{}
	function := staticTestTerm(keyspace.FamilyFunction, 1)
	call := staticTestTerm(keyspace.FamilyCall, 1)
	if err := rows.FunctionContractDeclare(function); err != nil {
		t.Fatal(err)
	}
	if err := rows.FunctionContractGenerics(function, nil); err != nil {
		t.Fatal(err)
	}
	if err := rows.FunctionContractReturns(function, true, nil); err != nil {
		t.Fatal(err)
	}
	if err := rows.CallContractPlaceholder(call); err != nil || rows.CallContractArguments(call, nil) != nil {
		t.Fatalf("call contract fill failed: %v", err)
	}
	if err := rows.CallContractArguments(call, nil); err == nil {
		t.Fatal("CallContractArguments accepted a duplicate fill")
	}
}
