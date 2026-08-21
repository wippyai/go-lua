package boundary

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/lower"
	linkproject "github.com/wippyai/go-lua/analysis/program/link/project"
)

// A computation occurrence is named downstream by the context identity of the
// span its owner sealed. The mounted semantic directory is the sole inverse of
// that identity, so every executable computation family publishes a row there:
// a consumer holding one occurrence identity either reaches its Value or the
// identity means nothing at all. The law below states that for the binary
// operator family, whose results Placement's suspension catalog joins by
// occurrence identity exactly as it joins unary, select, and claim results.
func TestBoundaryPublishesEveryExecutableBinarySemantic(t *testing.T) {
	contract := boundaryEndpointTarget(t)
	source, err := lower.Lower(lower.Source{Name: "boundary-binary-semantic-law", Text: []byte(`
local function fib(n: number): number
    if n < 2 then return n end
    return fib(n - 1) + fib(n - 2)
end
return fib(10)
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
	if project.Mounts().Count() != 1 {
		t.Fatalf("the sealed project mounts %d shards; the law reads one", project.Mounts().Count())
	}
	shard, shardOK := project.Mounts().At(0)
	module, moduleOK := project.ModuleKey(shard)
	if !shardOK || !moduleOK || !module.Available() {
		t.Fatal("the mounted program publishes no module key")
	}
	flow := source.Flow()
	binaries := flow.Authored().Operators().Binaries()
	executable := flow.Executable()
	measured := 0
	for index := 0; index < binaries.Count(); index++ {
		term, termOK := binaries.At(index)
		if !termOK {
			t.Fatalf("binary row %d publishes no term", index)
		}
		if !executable.Contains(term) {
			continue
		}
		span, spanOK := source.Span(term)
		if !spanOK || !source.OwnsSpan(span) || !span.ContextID().Available() {
			t.Fatalf("executable binary term %d owns no span identity", term)
		}
		measured++
		if _, ok := component.Values().ForMountedSemantic(module, span.ContextID()); !ok {
			t.Fatalf("binary term %d publishes occurrence %v with no mounted semantic Value", term, span.ContextID())
		}
	}
	if measured == 0 {
		t.Fatal("the program declares no executable binary; the law measures nothing")
	}
}
