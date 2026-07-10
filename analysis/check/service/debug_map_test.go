package service

import (
	"context"
	"reflect"
	"testing"
)

func TestCompletedResultExportsDeterministicDebugMapsAndStaticArtifacts(t *testing.T) {
	completed := solveDebugFixture(t, `
local function run(value: number): number
	local copy = value
	return copy
end
return run(1)
`)
	maps := completed.DebugMaps()
	if len(maps) == 0 {
		t.Fatal("completed result has no debug maps")
	}
	for _, debugMap := range maps {
		if debugMap.SchemaVersion != DebugMapSchemaVersion || debugMap.BodyDigest == 0 || debugMap.Digest.IsZero() {
			t.Fatalf("debug map metadata = %#v", debugMap)
		}
		if got := digestBytes(debugMap.CanonicalBytes()); got != debugMap.Digest {
			t.Fatalf("debug map %s canonical digest = %s, want %s", debugMap.BodyID, got, debugMap.Digest)
		}
		for _, entry := range debugMap.Entries {
			if entry.ID.String() == "" || entry.SourceSpan.StartLine == 0 || entry.Anchor.StartLine == 0 {
				t.Fatalf("debug map %s has incomplete source mapping: %#v", debugMap.BodyID, entry)
			}
		}
		for index := 1; index < len(debugMap.Entries); index++ {
			previous, current := debugMap.Entries[index-1].ID, debugMap.Entries[index].ID
			if current.Ordinal < previous.Ordinal || (current.Ordinal == previous.Ordinal && current.Phase < previous.Phase) {
				t.Fatalf("debug map %s is not in canonical point/phase order: %#v", debugMap.BodyID, debugMap.Entries)
			}
		}
	}

	artifacts := completed.StaticArtifacts()
	if len(artifacts) != len(maps) {
		t.Fatalf("static artifacts = %d, debug maps = %d", len(artifacts), len(maps))
	}
	byBody := make(map[BodyID]BodyDebugMap, len(maps))
	for _, debugMap := range maps {
		byBody[debugMap.BodyID] = debugMap
	}
	tag := completed.Tag()
	for _, artifact := range artifacts {
		debugMap, ok := byBody[artifact.BodyID]
		if !ok {
			t.Fatalf("artifact body %s has no debug map", artifact.BodyID)
		}
		if !artifact.ID.Valid() || artifact.ID.UnitDigest != tag.UnitDigest || artifact.ID.BodyDigest != debugMap.BodyDigest || artifact.ID.Profile != tag.Profile || artifact.ID.EngineBuildTag != EngineBuildTag || artifact.ID.DebugMapDigest != debugMap.Digest {
			t.Fatalf("artifact %s = %#v, debug map = %#v, tag = %#v", artifact.BodyID, artifact.ID, debugMap, tag)
		}
		if got := artifact.ID.String(); got == "" || got != artifact.ID.String() {
			t.Fatalf("artifact canonical ID = %q", got)
		}
	}

	// CompletedResult accessors must not expose snapshot-owned debug-map slices.
	maps[0].Entries[0].Visible = append(maps[0].Entries[0].Visible, DbgLocal{Name: "mutated"})
	if hasVisibleName(completed.DebugMaps()[0].Entries[0].Visible, "mutated") {
		t.Fatal("mutating returned debug map changed completed snapshot")
	}
}

func TestStaticArtifactIDCanonicalString(t *testing.T) {
	unit := digestBytes([]byte("unit"))
	debugMap := digestBytes([]byte("debug-map"))
	id := StaticArtifactID{
		UnitDigest:     unit,
		BodyDigest:     0x2a,
		Profile:        "debug",
		EngineBuildTag: "engine-test",
		DebugMapDigest: debugMap,
	}
	want := "static-artifact-v1|unit=64:" + unit.String() + "|body=16:000000000000002a|profile=5:debug|engine=11:engine-test|debug-map=64:" + debugMap.String()
	if got := id.String(); got != want {
		t.Fatalf("StaticArtifactID.String() = %q, want %q", got, want)
	}
}

func TestDebugMapsDeterministicAcrossIndependentSolves(t *testing.T) {
	src := `
local function run(value: number): number
	local copy = value
	return copy
end
return run(1)
`
	first := solveDebugFixture(t, src)
	second := solveDebugFixture(t, src)
	if !reflect.DeepEqual(first.DebugMaps(), second.DebugMaps()) {
		t.Fatalf("independent solves produced different debug maps\nfirst:  %#v\nsecond: %#v", first.DebugMaps(), second.DebugMaps())
	}
	if !reflect.DeepEqual(first.StaticArtifacts(), second.StaticArtifacts()) {
		t.Fatalf("independent solves produced different static artifacts\nfirst:  %#v\nsecond: %#v", first.StaticArtifacts(), second.StaticArtifacts())
	}
}

func TestDebugMapsIgnoreUnrelatedBodyEdit(t *testing.T) {
	before := solveDebugFixture(t, `
local function leaf(): number
	return 10
end

local function unrelated(): number
	local fixed = 7
	return fixed
end

return leaf() + unrelated()
`)
	after := solveDebugFixture(t, `
local function leaf(): number
	return 42
end

local function unrelated(): number
	local fixed = 7
	return fixed
end

return leaf() + unrelated()
`)
	left := debugMapContainingVisibleName(t, before.DebugMaps(), "fixed")
	right := debugMapContainingVisibleName(t, after.DebugMaps(), "fixed")
	if !reflect.DeepEqual(left, right) {
		t.Fatalf("unrelated body debug map changed\nbefore: %#v\nafter:  %#v", left, right)
	}
}

func TestDebugMapCoversSuspensionPhaseAndNestedLocalVisibility(t *testing.T) {
	completed := solveDebugFixture(t, `
local function run()
	local outer = 1
	do
		local outer = 2
		local inner = outer
		coroutine.yield()
	end
	return outer
end
return run()
`)
	var selected BodyDebugMap
	var suspend DebugMapEntry
	found := false
	for _, debugMap := range completed.DebugMaps() {
		for _, entry := range debugMap.Entries {
			if entry.ID.Phase == DebugPhaseSuspend && entry.MaySuspend && hasVisibleName(entry.Visible, "inner") {
				selected, suspend, found = debugMap, entry, true
				break
			}
		}
		if found {
			break
		}
	}
	if !found || !suspend.ID.Valid() || !hasVisibleName(suspend.Visible, "outer") || countVisibleName(suspend.Visible, "outer") != 1 {
		t.Fatalf("suspension phase/lexical visibility = map:%#v entry:%#v", selected, suspend)
	}

	for _, entry := range selected.Entries {
		if entry.ID.Phase != DebugPhaseReturn || !hasVisibleName(entry.Visible, "outer") {
			continue
		}
		if hasVisibleName(entry.Visible, "inner") || countVisibleName(entry.Visible, "outer") != 1 {
			t.Fatalf("nested scope leaked after end: %#v", entry)
		}
		return
	}
	t.Fatalf("missing return-phase visibility for outer local: %#v", selected.Entries)
}

func solveDebugFixture(t *testing.T, source string) CompletedResult {
	t.Helper()
	ctx := context.Background()
	session := NewBatchSession()
	input := UnitInput{
		ID:              "debug-fixture",
		ModulePath:      "example/debug-fixture",
		EntryFile:       "main.lua",
		SourceFiles:     map[string][]byte{"main.lua": []byte(source)},
		Profile:         "debug",
		IncludeStdlib:   true,
		DocumentVersion: 1,
	}
	if _, err := session.UpsertUnit(ctx, input); err != nil {
		t.Fatalf("UpsertUnit: %v", err)
	}
	tag := mustSolve(t, session, SolveRequest{UnitID: input.ID, Trigger: TriggerBatch})
	completed, ok := session.LastComplete(ctx, ResultRequest{Selector: selectorFor(tag)})
	if !ok {
		t.Fatal("missing completed debug fixture")
	}
	return completed
}

func debugMapContainingVisibleName(t *testing.T, maps []BodyDebugMap, name string) BodyDebugMap {
	t.Helper()
	for _, debugMap := range maps {
		for _, entry := range debugMap.Entries {
			if hasVisibleName(entry.Visible, name) {
				return debugMap
			}
		}
	}
	t.Fatalf("no debug map exposes local %q: %#v", name, maps)
	return BodyDebugMap{}
}

func hasVisibleName(locals []DbgLocal, name string) bool {
	return countVisibleName(locals, name) != 0
}

func countVisibleName(locals []DbgLocal, name string) int {
	count := 0
	for _, local := range locals {
		if local.Name == name {
			count++
		}
	}
	return count
}
