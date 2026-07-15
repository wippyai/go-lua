package program

import (
	"context"
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/query"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestEvaluatedBoundChunkSpecializesCaptureAndGlobalThroughDirectChild(t *testing.T) {
	stmts := parseChunk(t, `
local registry_entry = registry
local captured = true
local function child()
	if registry then return captured end
	return captured
end
local result = child()
if registry_entry then return result end
return result
`)
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"registry"}})
	stats := &Stats{}
	reg := standard.Registry()
	artifact, err := runEvaluatedBoundChunk(context.Background(), stmts, bindings, Config{
		Check: body.Config{
			Registry: reg, Globals: []string{"registry"},
			UnitNamespace: lexicalidentity.UnitNamespaceFromContent([]byte("evaluated-capture-global-boundary")),
		},
		Stats: stats,
	})
	if err != nil {
		t.Fatal(err)
	}
	entry, ok, err := artifact.Entry(context.Background(), reg)
	if err != nil || !ok || !entry.Coverage().Complete() {
		t.Fatalf("evaluated entry = ok:%v coverage:%#v err:%v", ok, entry.Coverage(), err)
	}
	if stats.EvaluatedObserverCallTemplates != 1 || stats.EvaluatedObserverTermApplications == 0 ||
		stats.EvaluatedObserverProgramPublications != 1 {
		t.Fatalf("call templates/term applications/publications = %d/%d/%d, want 1/>0/1",
			stats.EvaluatedObserverCallTemplates, stats.EvaluatedObserverTermApplications,
			stats.EvaluatedObserverProgramPublications)
	}
	if stats.PrepassBodySolves != 0 || stats.SummaryBodySolves != 0 || stats.MaterializeBodySolves != 0 ||
		stats.Body.BodySolves != 0 || !reflect.DeepEqual(stats.Query, query.Stats{}) {
		t.Fatalf("capture/global evaluated path entered legacy work: %#v", stats)
	}
}
