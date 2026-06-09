package canonical

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/scope"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestDriverStatsRecordDiagnosticObservationPhase(t *testing.T) {
	chunk, err := parse.ParseString(`
local function make(v)
    return function()
        return v
    end
end
local get = make("id")
local value = get()
`, "stats-observation.lua")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	driver := NewDriver(Config{Stdlib: scope.NewWithBuiltins()})
	sess := newCanonicalTestSession("stats-observation.lua")
	driver.Run(sess, chunk)

	stats := driver.stats.Snapshot()
	if stats.UniqueSummaryKeyDemands == 0 {
		t.Fatal("expected canonical solve to demand at least one summary key")
	}
	if stats.DiagnosticObservedStates == 0 {
		t.Fatal("expected diagnostic observation phase to record observed states")
	}
	if stats.ObserveIntraWithKeyCalls < stats.DiagnosticObservedStates {
		t.Fatalf("ObserveIntraWithKeyCalls=%d DiagnosticObservedStates=%d, want query observations to cover diagnostics", stats.ObserveIntraWithKeyCalls, stats.DiagnosticObservedStates)
	}
}
