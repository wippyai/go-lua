package checktest

import "testing"

// Repeated use of the same branch predicate must carry the predicate's exact
// edge knowledge into presence consequences. The first branch publishes the
// correlation ok => x is present; the second branch consumes that correlation
// and therefore passes x to a non-optional parameter without a nilability
// diagnostic.
func TestCheckBranchPredicateCarriesPresenceConsequence(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{name: "true edge", source: `
local function consume(value: string)
end

local function run(ok: boolean)
    local x: string?
    if ok then
        x = "v"
    end
    if ok then
        consume(x)
    end
end

return run
`},
		{name: "false edge", source: `
local function consume(value: string)
end

local function run(ok: boolean)
    local x: string?
    if not ok then
        x = "v"
    end
    if not ok then
        consume(x)
    end
end

return run
		`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := Check(test.source)
			defer result.ReleaseTransient()
			if len(result.Diagnostics) != 0 {
				t.Fatalf("diagnostics = %#v, want repeated branch predicate to prove x present", result.Diagnostics)
			}
		})
	}
}

// A Boolean observation is correlated only with the value epoch that it read.
// Rebinding the predicate to an independent Boolean must not let the later
// branch borrow the earlier branch's presence consequence.
func TestCheckBranchPredicateMutationStartsNewDecisionEpoch(t *testing.T) {
	result := Check(`
local function consume(value: string)
end

local function run(ok: boolean, replacement: boolean)
    local x: string?
    if ok then
        x = "v"
    end
    ok = replacement
    if ok then
        consume(x)
    end
end

return run
`)
	defer result.ReleaseTransient()
	if len(result.Diagnostics) == 0 {
		t.Fatal("diagnostics = none, want independent predicate epoch not to prove x present")
	}
}

func TestCheckLoopPredicateMutationDoesNotReusePriorIterationDecision(t *testing.T) {
	result := Check(`
local function consume(value: string)
end

local function run(again: boolean, ok: boolean, replacement: boolean)
    while again do
        local x: string?
        if ok then
            x = "v"
        end
        ok = replacement
        if ok then
            consume(x)
        end
        again = false
    end
end

return run
`)
	defer result.ReleaseTransient()
	if len(result.Diagnostics) == 0 {
		t.Fatal("diagnostics = none, want loop mutation to start an iteration-local predicate epoch")
	}
}
