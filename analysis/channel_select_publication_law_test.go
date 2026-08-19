package analysis

import (
	"testing"

	anadiag "github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/lua/selectapply"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/link"
	"github.com/wippyai/go-lua/internal/testfixture"
)

func TestCompilePublishesChannelSelectCaseFacts(t *testing.T) {
	contract, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatal(err)
	}
	linked := mustLink(t, `
type Event = {kind: string}
type Stop = {reason: string}

local function handle(events_ch: Channel<Event>, stop_ch: Channel<Stop>)
    channel.select {
        events_ch:case_receive(),
        { channel = events_ch, value = 1, ok = true, default = nil },
        stop_ch:case_receive(),
    }
end
`, contract)
	plan, status := Compile(linked)
	if status != CompileComplete || plan == nil {
		t.Fatalf("compile = %v plan=%t", status, plan != nil)
	}
	t.Cleanup(func() { plan.Close() })
	published := plan.state.composition
	if !published.Published() {
		t.Fatal("compile published no composition snapshot")
	}
	loaded, loadedOK := anadiag.LoadCaseSet(&published, plan.state.selectSites)
	if !loadedOK {
		t.Fatal("diagnostic reader refused the composition snapshot")
	}
	apps := selectapply.Apply(mustCompileProgram(t, linked))
	if len(apps) != 1 {
		t.Fatalf("Apply = %d, want 1", len(apps))
	}
	for _, fact := range apps[0].Facts.All() {
		got, ok := loaded.Lookup(apps[0].Site, fact.Ordinal)
		if !ok || got.Site != fact.Site || got.Ordinal != fact.Ordinal {
			t.Fatalf("accepted fact %d missing from diagnostic reader", fact.Ordinal)
		}
	}
	if _, ok := loaded.Lookup(apps[0].Site, 1); ok {
		t.Fatal("lookalike ordinal loaded as an accepted fact")
	}
}

func TestCompilePublishesEmptyChannelSelectColumnWithoutSelect(t *testing.T) {
	contract, err := testfixture.StandardLibraryTarget()
	if err != nil {
		t.Fatal(err)
	}
	linked := mustLink(t, "return 1\n", contract)
	plan, status := Compile(linked)
	if status != CompileComplete || plan == nil {
		t.Fatalf("compile = %v plan=%t", status, plan != nil)
	}
	t.Cleanup(func() { plan.Close() })
	if !plan.state.composition.Published() {
		t.Fatal("compile published no composition snapshot")
	}
	loaded, loadedOK := anadiag.LoadCaseSet(&plan.state.composition, plan.state.selectSites)
	if !loadedOK {
		t.Fatal("diagnostic reader refused the empty composition snapshot")
	}
	if len(loaded.All()) != 0 {
		t.Fatalf("empty select column loaded %d facts", len(loaded.All()))
	}
}

func mustCompileProgram(t *testing.T, linked *link.Link) *program.Program {
	t.Helper()
	mounts := linked.Project().Mounts()
	if mounts.Count() != 1 {
		t.Fatalf("fixture mounts = %d", mounts.Count())
	}
	shard, shardOK := mounts.At(0)
	prog, progOK := mounts.Program(shard)
	if !shardOK || !progOK || prog == nil {
		t.Fatal("fixture program")
	}
	return prog
}
