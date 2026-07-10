package body

import (
	"context"
	"testing"
	"time"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

// TestMatchingBracketLoopWideningConverges is a reduced fixture from
// kickside.workflows:mapping. A decreasing numeric lower bound is carried
// around the character-scanning loop; without dual-order lower-bound widening
// this body re-runs its 39 CFG nodes tens of thousands of times.
func TestMatchingBracketLoopWideningConverges(t *testing.T) {
	fn := parseFunction(t, `
function matching_bracket(source: string, open_at: integer): integer?
    local depth: integer = 1
    local quote: string? = nil
    local i: integer = open_at + 1
    while i <= #source do
        local ch = source:sub(i, i)
        if quote then
            if ch == quote and source:sub(i - 1, i - 1) ~= "\\" then quote = nil end
        elseif ch == "'" or ch == '"' then
            quote = ch
        elseif ch == "[" then
            depth = depth + 1
        elseif ch == "]" then
            depth = depth - 1
            if depth == 0 then return i end
        end
        i = i + 1
    end
    return nil
end`)
	stats := Stats{}
	prepared, err := PrepareBoundFunction(fn, bind.BindFunction(fn, bind.Options{}), Config{
		Registry: standard.Registry(),
		Stats:    &stats,
	})
	if err != nil {
		t.Fatalf("PrepareBoundFunction: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := SolvePrepared(prepared, SolveConfig{Context: ctx, Stats: &stats}); err != nil {
		t.Fatalf("SolvePrepared: %v", err)
	}
	if got := stats.Transfer.Solver.TransferCalls; got > 250 {
		t.Fatalf("transfer calls = %d, want <= 250 (loop widening regressed)", got)
	}
}
