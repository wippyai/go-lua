package engine_test

import (
	"strings"
	"testing"
)

// guardedIndexReadSource is one declaration-closed body whose element read sits
// under the given guard. Every case shares the shape so the guard is the only
// difference between them.
func guardedIndexReadSource(guard string) string {
	return `local function pick(xs: {number}, i: number): number
    if ` + guard + ` then
        local v: number = xs[i]
        return v
    end
    return 0
end
return pick
`
}

// TestUnprovenIndexReadSurfacesUnderEveryGuardFamily pins the reporting
// obligation of a guarded arm. The entry seeds each formal with its declared
// type, so every arm these guards select is reached by some admissible call and
// none of them states a bound on i. The read is therefore optional in each arm
// and the annotation that admits no nil is refuted, whatever predicate family
// the guard belongs to.
func TestUnprovenIndexReadSurfacesUnderEveryGuardFamily(t *testing.T) {
	for _, guard := range []string{"#xs >= 3", "i >= 1", "i ~= 0", "#xs >= 3 and i ~= 0"} {
		t.Run(guard, func(t *testing.T) {
			diagnostics := checkSource(t, guardedIndexReadSource(guard))
			summary := diagnosticSummaries(diagnostics)
			if !strings.Contains(summary, "cannot assign v because it may be nil") {
				t.Fatalf("an unproven element read under %q published no refutation:\n%s", guard, summary)
			}
		})
	}
}

// TestUnguardedIndexReadKeepsItsRefutation is the control the guarded cases are
// measured against: the same read with no branch at all.
func TestUnguardedIndexReadKeepsItsRefutation(t *testing.T) {
	diagnostics := checkSource(t, `local function pick(xs: {number}, i: number): number
    local v: number = xs[i]
    return v
end
return pick
`)
	if summary := diagnosticSummaries(diagnostics); !strings.Contains(summary, "cannot assign v because it may be nil") {
		t.Fatalf("an unguarded unproven element read published no refutation:\n%s", summary)
	}
}

// TestBoundedIndexReadDischargesItsObligation pins the other side: a guard that
// bounds the index below and above puts it inside the sequence, so the read is
// present and the declaration holds.
func TestBoundedIndexReadDischargesItsObligation(t *testing.T) {
	for _, guard := range []string{"i >= 1 and i <= #xs", "#xs >= 3 and i >= 1 and i <= 3"} {
		t.Run(guard, func(t *testing.T) {
			if diagnostics := checkSource(t, guardedIndexReadSource(guard)); len(diagnostics) != 0 {
				t.Fatalf("a proven in-range element read was refuted under %q:\n%s", guard, diagnosticSummaries(diagnostics))
			}
		})
	}
}

// TestConstantCeilingConsumesLengthFloor pins the in-range rule the bounded
// case rests on: a constant ceiling on the index and a length floor on the
// container relate the two with no path between them. A ceiling above the floor
// proves nothing, so the read there stays optional.
func TestConstantCeilingConsumesLengthFloor(t *testing.T) {
	for _, item := range []struct {
		name     string
		guard    string
		refuted  bool
		fixtures string
	}{
		{name: "ceiling within floor", guard: "#xs >= 4 and i >= 1 and i <= 4"},
		{name: "ceiling above floor", guard: "#xs >= 2 and i >= 1 and i <= 4", refuted: true},
		{name: "residue pins ceiling within floor", guard: "#xs >= 1 and i >= 1 and i <= 2 and i % 2 == 1"},
		{name: "residue leaves ceiling above floor", guard: "#xs >= 1 and i >= 1 and i <= 4 and i % 2 == 1", refuted: true},
	} {
		t.Run(item.name, func(t *testing.T) {
			source := `local function pick(xs: {number}, i: integer): number
    if ` + item.guard + ` then
        local v: number = xs[i]
        return v
    end
    return 0
end
return pick
`
			summary := diagnosticSummaries(checkSource(t, source))
			refuted := strings.Contains(summary, "cannot assign v because it may be nil")
			if refuted != item.refuted {
				t.Fatalf("guard %q: refuted=%v, want %v:\n%s", item.guard, refuted, item.refuted, summary)
			}
		})
	}
}

// TestBorderReadConsumesLengthFloor pins the border half of the same relation.
// A proven non-empty length makes the border the operator returns positive, so
// it names a written slot; without that floor the empty border stays available
// and the read is nil.
func TestBorderReadConsumesLengthFloor(t *testing.T) {
	guarded := diagnosticSummaries(checkSource(t, `local function last(xs: {number}): number
    if #xs > 0 then
        local v: number = xs[#xs]
        return v
    end
    return 0
end
return last
`))
	if strings.Contains(guarded, "may be nil") {
		t.Fatalf("a border read under a proven non-empty length was refuted:\n%s", guarded)
	}
	bare := diagnosticSummaries(checkSource(t, `local function last(xs: {number}): number
    local v: number = xs[#xs]
    return v
end
return last
`))
	if !strings.Contains(bare, "cannot assign v because it may be nil") {
		t.Fatalf("a border read with no length floor published no refutation:\n%s", bare)
	}
}

// TestBranchOverUnestablishedRootLeavesBodyDormant pins the limit of the
// admission the guarded cases rely on. A branch whose subject is a captured
// cell rests on an authority the declaration-only entry does not establish, so
// the body waits for a call rather than publishing under a seeded guess.
func TestBranchOverUnestablishedRootLeavesBodyDormant(t *testing.T) {
	diagnostics := checkSource(t, `local mode = false
local function pick(xs: {number}, i: number): number
    if mode then
        local v: number = xs[i]
        return v
    end
    return 0
end
return pick
`)
	if summary := diagnosticSummaries(diagnostics); strings.Contains(summary, "cannot assign v because it may be nil") {
		t.Fatalf("a branch over a capture published a declaration-owned refutation:\n%s", summary)
	}
}
