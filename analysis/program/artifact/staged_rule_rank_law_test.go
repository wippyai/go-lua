package artifact_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/analysis/program/artifact/schemaadapter"
	"github.com/wippyai/go-lua/analysis/schema/grammar"
)

// TestProgramArtifactStagedRulesReadStrictlyEarlierPoints is the artifact-side
// law behind the staged execution cut: a rule that writes a synthetic stage
// must read a point that the artifact's own WTO schedule visits strictly
// before that stage. A staged rule whose input is scheduled at or after its
// output is a fabricated recurrence, and no consumer of the sealed artifact
// can order it.
//
// Loop membership is not part of that law. An assignment inside a while,
// numeric for, generic for, or nested loop body commits through the same
// parent-issued predecessor route as one inside a branch, so its transfer
// must read the pre-write environment exactly as a straight-line commit does.
func TestProgramArtifactStagedRulesReadStrictlyEarlierPoints(t *testing.T) {
	for _, fixture := range []struct{ name, text string }{
		{name: "while-body-assignment", text: `
local function run(limit: number)
    local total = 0
    local index = 1
    while index < limit do
        total = 1
        index = index + 1
    end
    return total
end
return run
`},
		{name: "numeric-for-loop-carried-assignment", text: `
local function run(limit: number)
    local total = 0
    for step = 1, limit do
        total = total + step
    end
    return total
end
return run
`},
		{name: "numeric-for-non-carried-assignment", text: `
local function run(limit: number)
    local total = 0
    for step = 1, limit do
        total = 9
    end
    return total
end
return run
`},
		{name: "generic-for-body-assignment", text: `
local function run(rows: { string })
    local last = ""
    for _, row in ipairs(rows) do
        last = row
    end
    return last
end
return run
`},
		{name: "nested-loop-assignment", text: `
local function run(outer: number, inner: number)
    local total = 0
    for first = 1, outer do
        for second = 1, inner do
            total = total + second
        end
    end
    return total
end
return run
`},
		{name: "branch-body-assignment", text: `
local function run(flag: boolean)
    local total = 0
    if flag then
        total = 1
    else
        total = 2
    end
    return total
end
return run
`},
		{name: "branch-inside-loop-assignment", text: `
local function run(limit: number, flag: boolean)
    local total = 0
    for step = 1, limit do
        if flag then
            total = total + step
        else
            total = step
        end
    end
    return total
end
return run
`},
		{name: "chunk-numeric-for-assignment", text: `
local last: number = 0
for i = 1, 10 do
    local k: number = i
    last = k
end
return last
`},
		{name: "chunk-while-assignment", text: `
local total: number = 0
local i: number = 1
while i <= 64 do
    total = total + i
    i = i + 1
end
return total
`},
		{name: "index-write-inside-loop", text: `
local function run(limit: number)
    local rows: { number } = {}
    for step = 1, limit do
        rows[step] = step
    end
    return rows
end
return run
`},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			published, err := lower.Lower(lower.Source{Name: fixture.name + ".lua", Text: []byte(fixture.text)})
			if err != nil {
				t.Fatal(err)
			}
			receipt, receiptOK := grammar.Global()
			if !receiptOK {
				t.Fatal("Program artifact grammar capability unavailable")
			}
			artifact, failure := schemaadapter.CompileDetailed(published.TransformerInput(), receipt)
			if failure.Available() || artifact == nil || !artifact.Available() {
				t.Fatalf("compile %s: %s", fixture.name, failure.Error())
			}

			rank := make(map[identity.ContentID]int, artifact.PointCount())
			for eventIndex := 0; eventIndex < artifact.WTOEventCount(); eventIndex++ {
				event, eventOK := artifact.WTOEventAt(eventIndex)
				if !eventOK {
					t.Fatalf("WTO event %d unavailable", eventIndex)
				}
				if event.Kind() == programartifact.WTOEventPoint {
					rank[event.PointID()] = len(rank)
				}
			}

			staged := 0
			for role := programartifact.RuleRoleInvalid; role <= programartifact.RuleRoleValuePresenceRefinement; role++ {
				if !artifact.RuleRoleSupported(role) {
					continue
				}
				for ruleIndex := 0; ruleIndex < artifact.RuleOccurrenceCount(role); ruleIndex++ {
					rule, ruleOK := artifact.RuleOccurrenceAt(role, ruleIndex)
					if !ruleOK {
						t.Fatalf("role=%d rule=%d unavailable", role, ruleIndex)
					}
					if rule.Stage() == programartifact.RuleStageBase {
						continue
					}
					point, pointOK := rule.PointAt(0)
					input, inputOK := rule.InputPoint()
					if !pointOK || !inputOK {
						t.Fatalf("role=%d rule=%d staged placement has no point pair", role, ruleIndex)
					}
					outputRank, outputRanked := rank[point]
					inputRank, inputRanked := rank[input]
					if !outputRanked || !inputRanked {
						t.Fatalf("role=%d rule=%d point pair is not scheduled: input=%t output=%t", role, ruleIndex, inputRanked, outputRanked)
					}
					if input == point || inputRank >= outputRank {
						t.Fatalf("role=%d rule=%d reads rank %d and writes rank %d", role, ruleIndex, inputRank, outputRank)
					}
					staged++
				}
			}
			if staged == 0 {
				t.Fatalf("%s issued no staged rule placement", fixture.name)
			}
		})
	}
}
