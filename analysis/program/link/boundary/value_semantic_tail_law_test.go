package boundary

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
)

// A call's actuals are one authored Values row, so the row's open tail carries
// a Values-owned identity: ValuesTailID is derived from the row and its tail
// term alone, with no call in the equation. The authored Values pass therefore
// publishes that inverse once for the row, and the Calls pass reads the same
// mapping rather than submitting the identity a second time; the mounted
// semantic directory admits one publication per identity, so a second
// submission of a Values-owned fact refuses the whole seal.
//
// The law below states the resulting relation for a call in tail-argument
// position: the seal completes, and the row's tail identity names the exact
// Boundary Value of the tail term - the same Value the tail call's own
// occurrence identity names.
func TestBoundaryPublishesCallActualsTailOnce(t *testing.T) {
	contract := boundaryEndpointTarget(t)
	source, err := lower.Lower(lower.Source{Name: "boundary-call-tail-semantic-law", Text: []byte(`
local function inner(): number
    return 1
end

local function outer(a: number, b: number): number
    return a + b
end

print(outer(1, inner()))
`)})
	if err != nil {
		t.Fatal(err)
	}
	projectDraft, err := linkproject.Build(linkproject.Input{Modules: []linkproject.Module{{Name: "main", Program: source}}, Target: contract})
	if err != nil {
		t.Fatal(err)
	}
	project, err := projectDraft.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	draft, err := Build(Input{Project: project, Target: contract})
	if err != nil {
		t.Fatal(err)
	}
	component, err := draft.Finalize()
	if err != nil {
		t.Fatal(err)
	}
	shard, shardOK := project.Mounts().At(0)
	module, moduleOK := project.ModuleKey(shard)
	if !shardOK || !moduleOK || !module.Available() {
		t.Fatal("the mounted program publishes no module key")
	}
	flow := source.Flow()
	calls := flow.Authored().Calls()
	executable := flow.Executable()
	values := flow.Authored().Values()
	measured := 0
	for index := 0; index < calls.Count(); index++ {
		callTerm, callTermOK := calls.At(index)
		if !callTermOK {
			t.Fatalf("call row %d publishes no term", index)
		}
		_, _, _, actuals, rowOK := calls.Get(callTerm)
		if !rowOK {
			t.Fatalf("call term %d publishes no relation row", callTerm)
		}
		if !executable.Contains(callTerm) {
			continue
		}
		_, tailTerm, valuesRowOK := values.Get(actuals)
		if !valuesRowOK {
			t.Fatalf("call term %d actuals %d publish no Values row", callTerm, actuals)
		}
		if tailTerm == 0 {
			continue
		}
		measured++
		tailID, tailIDOK := flow.ValuesTailID(actuals)
		if !tailIDOK || !tailID.Available() {
			t.Fatalf("open actuals %d publish no tail identity", actuals)
		}
		published, publishedOK := component.Values().ForMountedSemantic(module, tailID)
		if !publishedOK {
			t.Fatalf("tail identity %v of actuals %d names no mounted semantic Value", tailID, actuals)
		}
		expected, expectedOK := component.Values().Of(shard, tailTerm)
		if !expectedOK {
			t.Fatalf("tail term %d owns no Boundary Value", tailTerm)
		}
		if order, orderOK := component.Values().Compare(published, expected); !orderOK || order != 0 {
			t.Fatalf("tail identity %v names a Value other than the one tail term %d owns", tailID, tailTerm)
		}
		if keyspace.TermFamily(tailTerm) != keyspace.FamilyCall {
			continue
		}
		identities, identitiesOK := source.CallIdentityAt(tailIndex(t, calls, tailTerm))
		if !identitiesOK || !identities.Call.Available() {
			t.Fatalf("tail call term %d publishes no call identity", tailTerm)
		}
		tailCall, tailCallOK := component.Values().ForMountedSemantic(module, identities.Call)
		if !tailCallOK {
			t.Fatalf("tail call identity %v names no mounted semantic Value", identities.Call)
		}
		if order, orderOK := component.Values().Compare(published, tailCall); !orderOK || order != 0 {
			t.Fatalf("the row tail identity and the tail call occurrence name different Values for term %d", tailTerm)
		}
	}
	if measured == 0 {
		t.Fatal("the program declares no executable call with an open actuals tail; the law measures nothing")
	}
}

// tailIndex names the authored Calls enumeration position of one call term.
func tailIndex(t *testing.T, calls interface {
	Count() int
	At(int) (keyspace.Term, bool)
}, term keyspace.Term) int {
	t.Helper()
	for index := 0; index < calls.Count(); index++ {
		got, ok := calls.At(index)
		if ok && got == term {
			return index
		}
	}
	t.Fatalf("call term %d has no authored enumeration position", term)
	return -1
}
