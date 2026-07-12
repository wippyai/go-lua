package program

import (
	"fmt"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/module/signaturelookup"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/compiler/parse"
)

// TestContextFanoutCostShape preserves the dominant shape reduced from the
// kickside.core.threads contract: many distinct call entries specialize one
// guarded loop body. The production source had 51 contexts, 92 summary solves,
// and 3,978 summary point transfers in a dependency-free probe. This compact
// fixture retains the multiplicative context x CFG cost without importing any
// product source or relying on wall-clock assertions.
func TestContextFanoutCostShape(t *testing.T) {
	var source strings.Builder
	source.WriteString(`local function worker(value)
  local total = 0
  for i = 1, 8 do
    if value then total = total + i end
  end
  return total
end
local total = 0
`)
	for i := 0; i < 48; i++ {
		fmt.Fprintf(&source, "total = total + worker(\"value-%d\")\n", i)
	}
	source.WriteString("return total\n")

	stmts, err := parse.ParseString(source.String(), "context-fanout-cost.lua")
	if err != nil {
		t.Fatal(err)
	}
	stats := &Stats{}
	_, err = RunChunk(stmts, Config{Check: body.Config{
		Registry: standard.Registry(), TypeValues: typevalue.NewCache(),
		Schedule: transfer.ScheduleWTO, Signatures: signaturelookup.Source{IncludeStdlib: true},
	}, Stats: stats})
	if err != nil {
		t.Fatal(err)
	}
	if stats.MaxContextCount != 48 || stats.SummaryBodySolves != 50 || stats.SummaryPointTransfers != 934 {
		t.Fatalf("context fanout cost shape = contexts:%d solves:%d transfers:%d, want 48/50/934",
			stats.MaxContextCount, stats.SummaryBodySolves, stats.SummaryPointTransfers)
	}
	census := stats.CompositionCostCensus()
	if len(census) != 2 || census[0].Reason != "shape:chunk" || census[1].Reason != "shape:loop" || census[1].BodySolves != 98 || census[1].PointTransfers != 1666 {
		t.Fatalf("context fanout composition census = %#v", census)
	}
}
