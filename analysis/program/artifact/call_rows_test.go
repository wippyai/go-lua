package artifact_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
)

func TestCallRowsExposeOnlyAvailableChildRanges(t *testing.T) {
	published, err := lower.Lower(lower.Source{
		Name: "call-rows.lua",
		Text: []byte(`
local function identity(value) return value end
return identity(1)
`),
	})
	if err != nil {
		t.Fatal(err)
	}
	grammar, ok := programartifact.NewGrammarIdentity(identity.ContentID{3}, programartifact.GrammarABIVersion)
	if !ok {
		t.Fatal("valid grammar identity was rejected")
	}
	artifact, failure := programartifact.CompileDetailed(published, grammar, programartifact.IssuanceDirectory{})
	if failure.Available() || artifact == nil || !artifact.Available() || artifact.CallCount() == 0 {
		t.Fatalf("call fixture did not compile: %s", failure.Error())
	}
	publishedDirect := false
	for callIndex := 0; callIndex < artifact.CallCount(); callIndex++ {
		row, rowOK := artifact.CallAt(callIndex)
		if !rowOK {
			t.Fatalf("CallAt(%d) unavailable", callIndex)
		}
		if target, targetOK := row.DirectTargetBody(); targetOK {
			found := false
			for bodyIndex := 0; bodyIndex < artifact.BodyCount(); bodyIndex++ {
				body, bodyOK := artifact.BodyAt(bodyIndex)
				if bodyOK && body.Callable() && body.ID() == target {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("CallAt(%d) target %x is not a callable body", callIndex, target[:4])
			}
			publishedDirect = true
		}
	}
	if !publishedDirect {
		t.Fatal("direct identity(1) call published no target body")
	}
	for callIndex := 0; callIndex < artifact.CallCount(); callIndex++ {
		row, rowOK := artifact.CallAt(callIndex)
		if !rowOK {
			t.Fatalf("CallAt(%d) unavailable", callIndex)
		}
		for childIndex := 0; childIndex < row.OperandCount(); childIndex++ {
			child, childOK := artifact.CallOperandFor(callIndex, childIndex)
			if !childOK || !child.Available() || child.CallID() != row.ID() {
				t.Fatalf("CallOperandFor(%d,%d) escaped its parent range", callIndex, childIndex)
			}
		}
		for childIndex := 0; childIndex < row.ArgumentCount(); childIndex++ {
			child, childOK := artifact.CallArgumentFor(callIndex, childIndex)
			if !childOK || !child.Available() || child.CallID() != row.ID() {
				t.Fatalf("CallArgumentFor(%d,%d) escaped its parent range", callIndex, childIndex)
			}
		}
		for childIndex := 0; childIndex < row.TypeArgumentCount(); childIndex++ {
			child, childOK := artifact.CallTypeArgumentFor(callIndex, childIndex)
			if !childOK || !child.Available() || child.CallID() != row.ID() {
				t.Fatalf("CallTypeArgumentFor(%d,%d) escaped its parent range", callIndex, childIndex)
			}
		}
		if _, childOK := artifact.CallOperandFor(callIndex, row.OperandCount()); childOK {
			t.Fatal("CallOperandFor exposed a child beyond the sealed range")
		}
		if _, childOK := artifact.CallArgumentFor(callIndex, row.ArgumentCount()); childOK {
			t.Fatal("CallArgumentFor exposed a child beyond the sealed range")
		}
		if _, childOK := artifact.CallTypeArgumentFor(callIndex, row.TypeArgumentCount()); childOK {
			t.Fatal("CallTypeArgumentFor exposed a child beyond the sealed range")
		}
	}
}
