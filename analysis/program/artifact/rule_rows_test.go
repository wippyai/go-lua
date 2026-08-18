package artifact_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/lua/lower"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	"github.com/wippyai/go-lua/domain/composite"
)

func TestRulePlacementSurfaceEqualsIssuedSubscriptions(t *testing.T) {
	published, err := lower.Lower(lower.Source{Name: "issuance.lua", Text: []byte(`
local function run(limit: number)
    local total = 0
    local index = 1
    while index < limit do
        total = total + 1
        index = index + 1
    end
    return total
end
`)})
	if err != nil {
		t.Fatal(err)
	}
	compilation, compilationOK := composite.Global()
	if !compilationOK {
		t.Fatal("Program artifact grammar unavailable")
	}
	artifact, failure := composite.CompileArtifactDetailed(published, compilation)
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("compile: %s", failure.Error())
	}
	if artifact.RulePlacementCount() == 0 {
		t.Fatal("compiled program issued no rule placements")
	}
	seen := make(map[identity.ContentID]int)
	for index := 0; index < artifact.RulePlacementCount(); index++ {
		row, ok := artifact.RulePlacementAt(index)
		if !ok || !row.Available() {
			t.Fatalf("placement %d unavailable", index)
		}
		if !row.Key().Available() {
			t.Fatalf("placement %d has no declaration key", index)
		}
		seen[row.ID()]++
	}
	if len(seen) == 0 {
		t.Fatal("compiled program issued no rule placements")
	}
}

func TestRulePlacementForKeySelectsTheIssuedDeclaration(t *testing.T) {
	published, err := lower.Lower(lower.Source{Name: "issuance-key.lua", Text: []byte(`
local function run(limit: number)
    local total = 0
    local index = 1
    while index < limit do
        total = total + 1
        index = index + 1
    end
    return total
end
`)})
	if err != nil {
		t.Fatal(err)
	}
	compilation, compilationOK := composite.Global()
	if !compilationOK {
		t.Fatal("Program artifact grammar unavailable")
	}
	artifact, failure := composite.CompileArtifactDetailed(published, compilation)
	if failure.Available() || artifact == nil || !artifact.Available() {
		t.Fatalf("compile: %s", failure.Error())
	}
	if artifact.RulePlacementCountForKey("") != 0 {
		t.Fatal("empty key selected placements")
	}
	if artifact.RulePlacementCountForKey("value-bootstrap") != 0 {
		t.Fatal("link-lane key selected mounted placements")
	}
	counted := 0
	for index := 0; index < artifact.RulePlacementCount(); index++ {
		row, ok := artifact.RulePlacementAt(index)
		if !ok {
			t.Fatalf("placement %d unavailable", index)
		}
		counted++
		matched := 0
		for keyIndex := 0; keyIndex < artifact.RulePlacementCountForKey(row.Key()); keyIndex++ {
			got, gotOK := artifact.RulePlacementForKeyAt(row.Key(), keyIndex)
			if !gotOK || got.Key() != row.Key() {
				t.Fatalf("key %q placement %d is not that declaration", row.Key(), keyIndex)
			}
			matched++
		}
		if matched == 0 {
			t.Fatalf("key %q issued a placement the key index cannot see", row.Key())
		}
	}
	if counted == 0 {
		t.Fatal("compiled program issued no rule placements")
	}
	if _, ok := artifact.RulePlacementForKeyAt("value-binary-arithmetic", artifact.RulePlacementCountForKey("value-binary-arithmetic")); ok {
		t.Fatal("key index accepted an out-of-range ordinal")
	}
}
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
			compilation, compilationOK := composite.Global()
			if !compilationOK {
				t.Fatal("Program artifact grammar unavailable")
			}
			artifact, failure := composite.CompileArtifactDetailed(published, compilation)
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
			for ruleIndex := 0; ruleIndex < artifact.RulePlacementCount(); ruleIndex++ {
				rule, ruleOK := artifact.RulePlacementAt(ruleIndex)
				if !ruleOK {
					t.Fatalf("placement %d unavailable", ruleIndex)
				}
				if rule.Stage() == programartifact.RuleStageBase {
					continue
				}
				point, pointOK := rule.PointAt(0)
				input, inputOK := rule.InputPoint()
				if !pointOK || !inputOK {
					t.Fatalf("placement %d staged placement has no point pair", ruleIndex)
				}
				outputRank, outputRanked := rank[point]
				inputRank, inputRanked := rank[input]
				if !outputRanked || !inputRanked {
					t.Fatalf("placement %d point pair is not scheduled: input=%t output=%t", ruleIndex, inputRanked, outputRanked)
				}
				if input == point || inputRank >= outputRank {
					t.Fatalf("placement %d reads rank %d and writes rank %d", ruleIndex, inputRank, outputRank)
				}
				staged++
			}
			if staged == 0 {
				t.Fatalf("%s issued no staged rule placement", fixture.name)
			}
		})
	}
}
