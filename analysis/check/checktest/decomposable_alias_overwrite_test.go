package checktest

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestDecomposableAliasOverwriteCompletesAndIsPointSensitive(t *testing.T) {
	if os.Getenv("GO_WANT_DECOMPOSABLE_ALIAS_OVERWRITE_HELPER") == "1" {
		assertDecomposableAliasOverwriteCases(t)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestDecomposableAliasOverwriteCompletesAndIsPointSensitive$")
	cmd.Env = append(os.Environ(), "GO_WANT_DECOMPOSABLE_ALIAS_OVERWRITE_HELPER=1")
	output, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("decomposable alias-overwrite analysis did not complete before timeout\n%s", output)
	}
	if err != nil {
		t.Fatalf("decomposable alias-overwrite regression helper failed: %v\n%s", err, output)
	}
}

func assertDecomposableAliasOverwriteCases(t *testing.T) {
	t.Helper()
	tests := []struct {
		name string
		src  string
		want int
	}{
		{
			name: "review repro completes",
			src: `
local function probe(flag: boolean): number
  local t = { x = 1 }
  local alias = t
  if flag then
    alias = nil
  else
    local same = alias == { x = 2 }
  end
  return t.x
end
return probe
`,
			want: -1,
		},
		{
			name: "later use after overwrite does not disqualify",
			src: `
local t = { x = 1 }
local alias = t
alias = nil
local same = alias == { x = 2 }
return t.x
`,
			want: 1,
		},
		{
			name: "earlier use before overwrite disqualifies",
			src: `
local t = { x = 1 }
local alias = t
local same = alias == { x = 2 }
alias = nil
return t.x
`,
			want: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			plan := Check(tc.src).PlacementPlan()
			if len(plan.Entries) == 0 {
				t.Fatalf("placement plan has no allocation entries: %#v", plan)
			}
			if tc.want < 0 {
				return
			}
			_, got := plan.AllocationStats()
			if got != tc.want {
				t.Fatalf("decomposable allocation count = %d, want %d; entries=%#v", got, tc.want, plan.Entries)
			}
		})
	}
}
