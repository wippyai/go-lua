package body

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestAllocationSiteFactDecomposableUseCases(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want int
	}{
		{
			name: "options table static reads",
			src: `
local opts = { a = 1, b = 2 }
local total = opts.a + opts.b
return total
`,
			want: 1,
		},
		{
			name: "passed to function",
			src: `
local opts = { a = 1, b = 2 }
sink(opts)
`,
			want: 0,
		},
		{
			name: "identity compare",
			src: `
local opts = { a = 1 }
local other = { a = 1 }
local same = opts == other
return same
`,
			want: 0,
		},
		{
			name: "dynamic index",
			src: `
local opts = { a = 1, b = 2 }
local key = "a"
local value = opts[key]
return value
`,
			want: 0,
		},
		{
			name: "pairs iteration",
			src: `
local opts = { a = 1, b = 2 }
local total = 0
for _, value in pairs(opts) do
	total = total + value
end
return total
`,
			want: 0,
		},
		{
			name: "setmetatable",
			src: `
local opts = { a = 1 }
setmetatable(opts, {})
return opts.a
`,
			want: 0,
		},
		{
			name: "captured by closure",
			src: `
local opts = { a = 1, b = 2 }
local function read()
	return opts.a
end
return read()
`,
			want: 0,
		},
		{
			name: "returned",
			src: `
local opts = { a = 1, b = 2 }
return opts
`,
			want: 0,
		},
		{
			name: "stored into another table",
			src: `
local opts = { a = 1, b = 2 }
local holder = { ref = opts }
return holder
`,
			want: 0,
		},
		{
			name: "corpus shaped local config static reads",
			src: `
local config = {
	retry_count = 3,
	timeout_ms = 2500,
	enabled = true,
}
local budget = config.retry_count * config.timeout_ms
if config.enabled then
	return budget
end
return 0
`,
			want: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			facts := allocationSiteFactsForSource(t, tc.src)
			got := 0
			for _, fact := range facts {
				if fact.Decomposable {
					got++
				}
			}
			if got != tc.want {
				t.Fatalf("decomposable allocation count = %d, want %d; facts=%#v", got, tc.want, facts)
			}
		})
	}
}

func TestAllocationSiteFactFrameLocalUseProofCases(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want int
	}{
		{
			name: "body-local static reads",
			src: `
local opts = { a = 1, b = 2 }
local total = opts.a + opts.b
return total
`,
			want: 1,
		},
		{
			name: "returned allocation",
			src: `
local opts = { a = 1, b = 2 }
return opts
`,
			want: 0,
		},
		{
			name: "captured allocation",
			src: `
local opts = { a = 1, b = 2 }
local function read()
	return opts.a
end
return read()
`,
			want: 0,
		},
		{
			name: "stored into another table",
			src: `
local opts = { a = 1, b = 2 }
local holder = { ref = opts }
return holder
`,
			want: 0,
		},
		{
			name: "passed to function",
			src: `
local opts = { a = 1, b = 2 }
sink(opts)
`,
			want: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			facts := allocationSiteFactsForSource(t, tc.src)
			got := 0
			for _, fact := range facts {
				if fact.FrameLocalUseProof {
					got++
				}
			}
			if got != tc.want {
				t.Fatalf("frame-local use proof count = %d, want %d; facts=%#v", got, tc.want, facts)
			}
		})
	}
}

func TestAllocationSiteFactShapeAndPlacementAreExported(t *testing.T) {
	facts := allocationSiteFactsForSource(t, `
local opts = { a = 1, b = 2 }
local total = opts.a + opts.b
return total
`)
	if len(facts) != 1 {
		t.Fatalf("allocation facts = %d, want 1: %#v", len(facts), facts)
	}
	fact := facts[0]
	if fact.SchemaVersion != AllocationSiteFactSchemaVersion {
		t.Fatalf("schema version = %d, want %d", fact.SchemaVersion, AllocationSiteFactSchemaVersion)
	}
	if !fact.StableShape || len(fact.Fields) != 2 {
		t.Fatalf("stable shape/fields = %v/%#v, want 2-field stable shape", fact.StableShape, fact.Fields)
	}
	if !fact.HasPlacement {
		t.Fatalf("allocation fact has no placement: %#v", fact)
	}
	if !fact.Decomposable {
		t.Fatalf("allocation fact not decomposable: %#v", fact)
	}
	if !fact.FrameLocalUseProof {
		t.Fatalf("allocation fact lacks frame-local use proof: %#v", fact)
	}
}

func allocationSiteFactsForSource(t testing.TB, src string) []AllocationSiteFact {
	t.Helper()
	stmts, err := parse.ParseString(src, "decomposable_allocation_test.lua")
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	result, err := CheckChunk(stmts, Config{
		Registry: standard.Registry(),
		Globals:  []string{"sink", "pairs", "setmetatable"},
	})
	if err != nil {
		t.Fatalf("CheckChunk: %v", err)
	}
	var facts []AllocationSiteFact
	result.ForEachAllocationSiteFact(func(fact AllocationSiteFact) bool {
		facts = append(facts, fact)
		return true
	})
	return facts
}
