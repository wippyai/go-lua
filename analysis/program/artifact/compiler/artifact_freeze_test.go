package compiler_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/lower"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/domain/composite"
)

func TestArtifactFreezePublishesOneAvailableIdentity(t *testing.T) {
	published, err := lower.Lower(lower.Source{Name: "artifact-freeze.lua", Text: []byte("return 1")})
	if err != nil {
		t.Fatal(err)
	}
	compilation, ok := composite.Build()
	if !ok {
		t.Fatal("artifact grammar unavailable")
	}
	artifact, failure := compileArtifactForTest(t, published, compilation)
	if failure.Available() || artifact == nil || !artifact.Available() || !artifact.ID().Available() {
		t.Fatalf("freeze result = artifact:%v failure:%s", artifact != nil, failure.Error())
	}
	program := artifact.Program()
	entryCount, entriesPublished := program.ModuleEntryCount()
	entry, entryHeld := program.ModuleEntryAt(0)
	if !entriesPublished || entryCount != 1 || !entryHeld || entry.ID() == entry.ReturnID() {
		t.Fatalf("module entry identity separation = published:%v count:%d held:%v distinct:%v", entriesPublished, entryCount, entryHeld, entry.ID() != entry.ReturnID())
	}
	outcomeCount, outcomesPublished := program.OutcomeCount()
	returnAuthenticated := false
	for index := 0; index < outcomeCount; index++ {
		outcome, held := program.OutcomeAt(index)
		if held && outcome.ID() == entry.ReturnID() && outcome.Kind() == programschema.OutcomeReturn {
			returnAuthenticated = true
			break
		}
	}
	if !outcomesPublished || !returnAuthenticated {
		t.Fatal("ModuleEntry.ReturnID did not join its canonical Return Outcome")
	}
}

func TestModuleEntriesJoinTheRootActivationReturnOutcome(t *testing.T) {
	published, err := lower.Lower(lower.Source{Name: "module-entry-return.lua", Text: []byte(`
local condition = ...
if condition then
  return 1
end
return 2
`)})
	if err != nil {
		t.Fatal(err)
	}
	compilation, ok := composite.Build()
	if !ok {
		t.Fatal("artifact grammar unavailable")
	}
	artifact, failure := compileArtifactForTest(t, published, compilation)
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("freeze result = artifact:%v failure:%s", artifact != nil, failure.Error())
	}
	program := artifact.Program()
	entryBody, entryBodyOK := program.EntryBody()
	bodyCount, bodiesPublished := program.BodyCount()
	nonCallableBodies := 0
	for index := 0; index < bodyCount; index++ {
		body, bodyHeld := program.BodyAt(index)
		if !bodyHeld || !body.Available() {
			t.Fatalf("body %d unavailable", index)
		}
		if !body.Callable() {
			nonCallableBodies++
		}
	}
	if !entryBodyOK || !entryBody.Available() || entryBody.Callable() || !bodiesPublished || nonCallableBodies < 2 {
		t.Fatalf("root activation body = available:%v held:%v callable:%v bodies:%d published:%v; want exact root with multiple non-callable bodies", entryBody.Available(), entryBodyOK, entryBody.Callable(), nonCallableBodies, bodiesPublished)
	}
	entryCount, publishedEntries := program.ModuleEntryCount()
	first, firstHeld := program.ModuleEntryAt(0)
	second, secondHeld := program.ModuleEntryAt(1)
	if !publishedEntries || entryCount != 2 || !firstHeld || !secondHeld {
		t.Fatalf("module entries = published:%v count:%d held:%v/%v", publishedEntries, entryCount, firstHeld, secondHeld)
	}
	if first.ID() == second.ID() || first.ReturnID() != second.ReturnID() {
		t.Fatal("authored Return identities collapsed or failed to share the root Return Outcome")
	}
	matched := false
	outcomeCount, outcomesPublished := program.OutcomeCount()
	for index := 0; index < outcomeCount; index++ {
		outcome, held := program.OutcomeAt(index)
		if held && outcome.ID() == first.ReturnID() && outcome.Kind() == programschema.OutcomeReturn {
			matched = true
			break
		}
	}
	if !outcomesPublished || !matched {
		t.Fatal("ModuleEntry.ReturnID did not join the entry Body Return Outcome")
	}
}
