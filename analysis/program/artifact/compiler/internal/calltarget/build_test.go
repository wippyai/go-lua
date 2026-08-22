package calltarget

import (
	"testing"

	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
	programconstruction "github.com/wippyai/go-lua/analysis/schema/program/construction"
)

func TestBuildRequiresCanonicalBodyBundle(t *testing.T) {
	rows, fault := Build(Input{})
	row, rowOK := fault.Row()
	subrow, subrowOK := fault.Subrow()
	if rows != nil || !fault.Available() || fault.Family() != programcatalog.CallTarget() || fault.Issue() != programconstruction.IssueCallTargetUnavailable || rowOK || row != -1 || subrowOK || subrow != -1 {
		t.Fatalf("unexpected empty-input result: rows=%v fault=%+v", rows, fault)
	}
}
