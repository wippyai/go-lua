package front

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
)

func TestFrontPublishesBranchChainPathsStructurally(t *testing.T) {
	compilation, err := Compile(`
local selected = { channel = {} }
local first, second = {}, {}
if selected.channel == first then
    selected = selected
elseif selected.channel == second then
    selected = selected
end
return selected`)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	var chains []BranchChainWire
	for _, operation := range compilation.Artifact.Equations {
		for _, operand := range operation.Operands {
			if operand.Role != equation.RoleBranchChain {
				continue
			}
			chain, present, decodeErr := DecodeBranchChainWire(operand.Term.Encoding)
			if decodeErr != nil || !present {
				t.Fatalf("branch-chain decode: present=%v err=%v", present, decodeErr)
			}
			chains = append(chains, chain)
		}
	}
	if len(chains) != 2 {
		t.Fatalf("branch-chain publications = %#v, want two authored arms", chains)
	}
	for _, chain := range chains {
		if chain.Count != 2 || len(chain.Checks) != 1 {
			t.Fatalf("branch-chain topology = %#v", chain)
		}
		check := chain.Checks[0]
		var selected BranchChainPathWire
		switch {
		case check.Path.FinalField == "channel":
			selected = check.Path
		case check.OtherPath.FinalField == "channel":
			selected = check.OtherPath
		default:
			t.Fatalf("branch chain omitted structural channel segment: %#v", check)
		}
		if selected.ParentKey == "" || selected.ParentDisplay != "selected" {
			t.Fatalf("branch chain omitted structural parent: %#v", selected)
		}
	}
}
