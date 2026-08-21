package allocation

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/lower"
)

func TestBuildReportsCanonicalFieldFault(t *testing.T) {
	program, err := lower.Lower(lower.Source{Name: "allocation-fault.lua", Text: []byte("return {1}\n")})
	if err != nil {
		t.Fatal(err)
	}
	bundle, fault := Build(Input{Program: program})
	if bundle != nil || !fault.Failed() || fault.Row() != 0 || fault.Field() != 0 {
		t.Fatalf("Build missing value = bundle=%v fault=%#v, want nil row=0 field=0", bundle, fault)
	}
}

func TestTakeCanonicalPlanesTransfersExactlyOnce(t *testing.T) {
	program, err := lower.Lower(lower.Source{Name: "allocation-transfer.lua", Text: []byte("return {}\n")})
	if err != nil {
		t.Fatal(err)
	}
	bundle, fault := Build(Input{Program: program})
	if fault.Failed() || bundle == nil {
		t.Fatalf("Build empty allocation = bundle=%v fault=%#v", bundle, fault)
	}
	row, rowOK := bundle.RowAt(0)
	template, templateOK := row.Template()
	if !rowOK || !templateOK {
		t.Fatal("source allocation row")
	}
	allocations, fields, taken := bundle.TakeCanonicalPlanes()
	if !taken || len(allocations) != 1 || len(fields) != 0 {
		t.Fatalf("transferred planes = %d/%d/%t, want 1/0/true", len(allocations), len(fields), taken)
	}
	if _, ok := bundle.AllocationForID(template); ok {
		t.Fatal("allocation lookup survived canonical-plane transfer")
	}
	if _, ok := row.Template(); ok {
		t.Fatal("source row reopened transferred canonical plane")
	}
	if _, _, ok := bundle.TakeCanonicalPlanes(); ok {
		t.Fatal("canonical planes transferred twice")
	}
}
