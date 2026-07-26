package engine

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
)

func TestLuaFloorModuloCarriesTheModulusSign(t *testing.T) {
	tests := []struct {
		dividend, modulus, want float64
	}{
		{5, 3, 2},
		{-5, 3, 1},
		{5, -3, -1},
		{-5, -3, -2},
		{6, 3, 0},
		{5.5, 3, 2.5},
	}
	for _, test := range tests {
		if got := luaFloorModulo(test.dividend, test.modulus); got != test.want {
			t.Fatalf("%v %% %v = %v, want %v", test.dividend, test.modulus, got, test.want)
		}
	}
}

func TestConstantModulusWindowMatchesTheOperator(t *testing.T) {
	positive, ok := constantModulusWindow(3)
	if !ok || positive.Low != 0 || positive.High != 2 {
		t.Fatalf("window for 3 = %+v (ok=%v), want [0, 2]", positive, ok)
	}
	negative, ok := constantModulusWindow(-3)
	if !ok || negative.Low != -2 || negative.High != 0 {
		t.Fatalf("window for -3 = %+v (ok=%v), want [-2, 0]", negative, ok)
	}
	if _, ok := constantModulusWindow(0); ok {
		t.Fatalf("a zero modulus produced a window")
	}
}

func TestResidueWindowShiftMovesBothBounds(t *testing.T) {
	window, ok := constantModulusWindow(3)
	if !ok {
		t.Fatalf("no window for modulus 3")
	}
	shifted, ok := window.shift(1)
	if !ok || shifted.Low != 1 || shifted.High != 3 {
		t.Fatalf("shifted = %+v (ok=%v), want [1, 3]", shifted, ok)
	}
	self := selfLengthWindow("path/xs")
	shifted, ok = self.shift(1)
	if !ok || shifted.Low != 1 || shifted.High != 0 || shifted.Container != "path/xs" {
		t.Fatalf("self-length shift = %+v (ok=%v), want [1, #xs]", shifted, ok)
	}
}

func TestResidueClassCeilingTakesTheLargestMemberInRange(t *testing.T) {
	tests := []struct {
		ceiling, modulus, residue, want int64
	}{
		{2, 2, 1, 1},
		{4, 2, 1, 3},
		{3, 2, 1, 3},
		{10, 5, 0, 10},
		{9, 5, 0, 5},
		{0, 2, 1, -1},
		{-3, 4, 1, -3},
		{-2, 4, 1, -3},
	}
	for _, test := range tests {
		got, ok := residueClassCeiling(test.ceiling, test.modulus, test.residue)
		if !ok || got != test.want {
			t.Fatalf("largest v <= %d with v = %d (mod %d) = %d (ok=%v), want %d",
				test.ceiling, test.residue, test.modulus, got, ok, test.want)
		}
	}
	if _, ok := residueClassCeiling(4, 0, 0); ok {
		t.Fatalf("a zero modulus produced a ceiling")
	}
	if _, ok := residueClassCeiling(4, 2, 2); ok {
		t.Fatalf("an unreduced residue produced a ceiling")
	}
}

// branchPartitions returns the partition verdict published for every branch of
// the checked source, keyed by its occurrence.
func branchPartitions(t *testing.T, source string) map[string]string {
	t.Helper()
	result, err := Check(source)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	out := make(map[string]string)
	for _, fact := range result.Native.Facts() {
		if fact.Family == "branch_partition" {
			out[fact.Key] = fact.Value
		}
	}
	if len(out) == 0 {
		t.Fatalf("no branch partition published for:\n%s", source)
	}
	return out
}

func assertNoDeadArm(t *testing.T, source string) {
	t.Helper()
	for key, value := range branchPartitions(t, source) {
		if strings.Contains(value, "always_not_taken") || strings.Contains(value, "always_taken") {
			t.Fatalf("branch %s decided as %q, want a runtime test:\n%s", key, value, source)
		}
	}
}

func assertDeadThenArm(t *testing.T, source string) {
	t.Helper()
	for _, value := range branchPartitions(t, source) {
		if strings.Contains(value, "always_not_taken") {
			return
		}
	}
	t.Fatalf("no branch was decided false:\n%s", source)
}

func TestResidueContradictionDecidesOnlyADominatedArm(t *testing.T) {
	assertDeadThenArm(t, `
local function classify(x: integer): string
    if x % 2 == 0 then
        if x % 2 == 1 then
            return "impossible"
        end
    end
    return "other"
end
return classify
`)
}

func TestResidueContradictionRejectsASiblingBranch(t *testing.T) {
	// The second test is reached whichever edge the first one takes, so the
	// first guard proves nothing about it.
	assertNoDeadArm(t, `
local function classify(x: integer): string
    if x % 2 == 0 then
        local seen: integer = x
    end
    if x % 2 == 1 then
        return "odd"
    end
    return "other"
end
return classify
`)
}

func TestResidueContradictionRejectsAnEarlyReturnSibling(t *testing.T) {
	assertNoDeadArm(t, `
local function classify(x: integer): string
    if x % 2 == 0 then
        return "even"
    end
    if x % 2 == 1 then
        return "odd"
    end
    return "other"
end
return classify
`)
}

func TestResidueContradictionRejectsDifferentModuli(t *testing.T) {
	// x = 6 satisfies both, so two moduli constrain jointly rather than
	// exclusively.
	assertNoDeadArm(t, `
local function classify(x: integer): string
    if x % 2 == 0 then
        if x % 3 == 0 then
            return "six"
        end
    end
    return "other"
end
return classify
`)
}

func TestResidueContradictionRejectsAReboundSubject(t *testing.T) {
	assertNoDeadArm(t, `
local function classify(x: integer): string
    if x % 2 == 0 then
        x = x + 1
        if x % 2 == 1 then
            return "shifted"
        end
    end
    return "other"
end
return classify
`)
}

func TestResidueContradictionRejectsTheSameResidue(t *testing.T) {
	assertNoDeadArm(t, `
local function classify(x: integer): string
    if x % 2 == 0 then
        if x % 2 == 0 then
            return "even"
        end
    end
    return "other"
end
return classify
`)
}

func TestResidueContradictionRejectsANegatedEstablishingGuard(t *testing.T) {
	// `x % 2 ~= 0` leaves every other residue class open, so it establishes no
	// class of its own.
	assertNoDeadArm(t, `
local function classify(x: integer): string
    if x % 4 ~= 0 then
        if x % 4 == 1 then
            return "one"
        end
    end
    return "other"
end
return classify
`)
}

func TestResidueIndexPresenceReadsTheWindowAgainstTheLengthFloor(t *testing.T) {
	container := []byte("path/xs")
	key := []byte("temp/3")
	subject := heapIndexSubject(container, equation.Partition{})
	build := func(window string, floor string) equation.Partition {
		partition, err := equation.PartitionFromClosuresWithGuards(nil, equation.OutputClosure{Values: []equation.Fact{
			{Key: residueWindowPrefix + string(key) + "/op-00000001", Value: []byte(window)},
			{Key: heapLengthFloorPrefix + subject + "/op-00000000", Value: []byte(floor)},
		}})
		if err != nil {
			t.Fatalf("partition: %v", err)
		}
		return partition
	}
	tests := []struct {
		name   string
		window string
		floor  string
		want   bool
	}{
		{"window inside the floor", `{"low":1,"high":3}`, "3", true},
		{"window above the floor", `{"low":1,"high":5}`, "3", false},
		{"window below one", `{"low":0,"high":2}`, "3", false},
		{"self length wrap", `{"low":1,"high":0,"container":"path/xs"}`, "1", true},
		{"self length wrap past the border", `{"low":1,"high":1,"container":"path/xs"}`, "1", false},
		{"another container's length", `{"low":1,"high":0,"container":"path/ys"}`, "1", false},
		{"no proven length floor", `{"low":1,"high":3}`, "0", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := residueIndexPresenceProven(container, key, build(test.window, test.floor)); got != test.want {
				t.Fatalf("presence = %v, want %v", got, test.want)
			}
		})
	}
}
