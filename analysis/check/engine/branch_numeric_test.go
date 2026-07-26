package engine

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/front"
)

// residueWindowPartition holds one published residue window for a path, in the
// exact closed encoding the arithmetic lane writes.
func residueWindowPartition(t *testing.T, path string, window string) equation.Partition {
	t.Helper()
	partition, err := equation.PartitionFromClosuresWithGuards(nil, equation.OutputClosure{Values: []equation.Fact{
		{Key: residueWindowPrefix + "path/" + path + "/op-00000001", Value: []byte(window)},
	}})
	if err != nil {
		t.Fatalf("partition: %v", err)
	}
	return partition
}

// encodeEvidence renders one implied check in the front's closed branch
// evidence encoding, on the requested edge and polarity.
func encodeEvidence(t *testing.T, edge, polarity string, predicate branchPredicateWire) []byte {
	t.Helper()
	encoded, err := front.EncodeBranchEvidenceWire(predicate, edge == "true", polarity == "true")
	if err != nil {
		t.Fatalf("encoding predicate: %v", err)
	}
	return encoded
}

func TestNumericEdgeSatisfiableRefutesAnInfeasibleConjunction(t *testing.T) {
	// `x >= 5 and x <= 2`: the front normalizes the ceiling as a negated floor,
	// so the true edge asserts x >= 5 together with x < 3.
	predicates := []branchPredicateWire{
		{Kind: "num-ge", Path: "x", NumFloor: 5},
		{Kind: "num-ge", Path: "x", NumFloor: 3, Negated: true},
	}
	if numericEdgeSatisfiable(predicates, nil, equation.Partition{}) {
		t.Fatal("no value is both at least five and below three, so the edge has no model")
	}
}

func TestNumericEdgeSatisfiableAdmitsASatisfiableRange(t *testing.T) {
	predicates := []branchPredicateWire{
		{Kind: "num-ge", Path: "x", NumFloor: 2},
		{Kind: "num-ge", Path: "x", NumFloor: 6, Negated: true},
	}
	if !numericEdgeSatisfiable(predicates, nil, equation.Partition{}) {
		t.Fatal("2 <= x < 6 has a model and must leave both arms reachable")
	}
}

func TestNumericEdgeSatisfiableRelaxesAStrictBoundRatherThanTighteningIt(t *testing.T) {
	// The negated floor states x < 3, which is carried as x <= 3. The relaxed
	// form still refutes x >= 5 and must not refute x >= 3: a tightened bound
	// would report the second conjunction as dead on integrality this predicate
	// does not carry.
	relaxed := []branchPredicateWire{
		{Kind: "num-ge", Path: "x", NumFloor: 3},
		{Kind: "num-ge", Path: "x", NumFloor: 3, Negated: true},
	}
	if !numericEdgeSatisfiable(relaxed, nil, equation.Partition{}) {
		t.Fatal("a strict bound must be relaxed, never tightened, so x = 3 stays a model of the relaxed edge")
	}
}

func TestNumericEdgeSatisfiableRefutesALiteralOutsideTheResidueWindow(t *testing.T) {
	partition := residueWindowPartition(t, "y", `{"low":0,"high":3}`)
	predicates := []branchPredicateWire{{Kind: "literal-equal", Path: "y", Literal: "scalar/number/5"}}
	if numericEdgeSatisfiable(predicates, nil, partition) {
		t.Fatal("5 lies outside [0, 3], so the equality has no model")
	}
}

func TestNumericEdgeSatisfiableAdmitsALiteralInsideTheResidueWindow(t *testing.T) {
	partition := residueWindowPartition(t, "y", `{"low":0,"high":3}`)
	predicates := []branchPredicateWire{{Kind: "literal-equal", Path: "y", Literal: "scalar/number/1"}}
	if !numericEdgeSatisfiable(predicates, nil, partition) {
		t.Fatal("1 lies inside [0, 3], so the window decides nothing")
	}
}

func TestNumericEdgeSatisfiableRefutesACeilingBelowAShiftedWindow(t *testing.T) {
	partition := residueWindowPartition(t, "y", `{"low":10,"high":12}`)
	predicates := []branchPredicateWire{{Kind: "num-le", Path: "y", NumCeil: 5, HasNumCeil: true}}
	if numericEdgeSatisfiable(predicates, nil, partition) {
		t.Fatal("a shifted window at [10, 12] admits no value at or below five")
	}
}

func TestNumericEdgeSatisfiableRefutesAResidueClassOutsideTheWindow(t *testing.T) {
	partition := residueWindowPartition(t, "y", `{"low":0,"high":1}`)
	predicates := []branchPredicateWire{{Kind: "mod-residue", Path: "y", Modulus: 4, Residue: 3}}
	if numericEdgeSatisfiable(predicates, nil, partition) {
		t.Fatal("the class 3 (mod 4) holds no member of [0, 1]")
	}
}

func TestNumericEdgeSatisfiableAdmitsAResidueClassInsideTheWindow(t *testing.T) {
	partition := residueWindowPartition(t, "y", `{"low":0,"high":1}`)
	predicates := []branchPredicateWire{{Kind: "mod-residue", Path: "y", Modulus: 4, Residue: 1}}
	if !numericEdgeSatisfiable(predicates, nil, partition) {
		t.Fatal("1 lies in both the class and the window, so nothing is decided")
	}
}

func TestNumericEdgeSatisfiableAdmitsAPathWithNoPublishedWindow(t *testing.T) {
	predicates := []branchPredicateWire{{Kind: "mod-residue", Path: "y", Modulus: 4, Residue: 3}}
	if !numericEdgeSatisfiable(predicates, nil, equation.Partition{}) {
		t.Fatal("a subject the authorities carry no fact about decides nothing")
	}
}

func TestResidueClassWindowVerdictIntersectsTheClassWithTheWindow(t *testing.T) {
	tests := []struct {
		name             string
		window           residueWindow
		modulus, residue int64
		holds, decided   bool
	}{
		{"class outside the window", residueWindow{Low: 0, High: 1}, 4, 3, false, true},
		{"class inside the window", residueWindow{Low: 0, High: 1}, 4, 1, false, false},
		{"single point in the class", residueWindow{Low: 2, High: 2}, 2, 0, true, true},
		{"single point outside the class", residueWindow{Low: 3, High: 3}, 2, 0, false, true},
		{"every integer is a residue of one", residueWindow{Low: 0, High: 9}, 1, 0, true, true},
		{"a length-relative window has no constant bounds", residueWindow{Low: 0, High: 0, Container: "path/xs"}, 2, 1, false, false},
		{"a non-positive modulus decides nothing", residueWindow{Low: 0, High: 1}, 0, 0, false, false},
		{"a residue outside the modulus is not this class", residueWindow{Low: 5, High: 5}, 4, 5, false, false},
		{"negative window below the class", residueWindow{Low: -3, High: -2}, 4, 0, false, true},
		{"negative window holding a class member", residueWindow{Low: -3, High: -2}, 4, 1, false, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			holds, decided := residueClassWindowVerdict(test.window, test.modulus, test.residue)
			if holds != test.holds || decided != test.decided {
				t.Fatalf("verdict = (%v, %v), want (%v, %v)", holds, decided, test.holds, test.decided)
			}
		})
	}
}

func TestTrueEdgeNumericPredicatesRejectSufficientChecks(t *testing.T) {
	// A disjunction publishes every arm's sufficient check on the true edge.
	// Their conjunction describes no execution, so admitting them would refute
	// an arm every value of x reaches.
	operation := equation.BoundEquation{Operands: []equation.BoundOperand{
		{Role: "condition", Value: []byte("temp/0")},
		{Role: "sufficient-00000000", Value: encodeEvidence(t, "true", "true", branchPredicateWire{Kind: "num-ge", Path: "x", NumFloor: 5})},
		{Role: "sufficient-00000001", Value: encodeEvidence(t, "true", "true", branchPredicateWire{Kind: "num-ge", Path: "x", NumFloor: 3, Negated: true})},
	}}
	if predicates, err := trueEdgeNumericPredicates(operation); err != nil || len(predicates) != 0 {
		t.Fatalf("a sufficient check states no necessary condition of its edge, got %v", predicates)
	}
	if _, decided, err := branchNumericTruth(operation, equation.Partition{}); err != nil || decided {
		t.Fatal("a disjunction whose arms exclude each other is still reachable")
	}
}

func TestTrueEdgeNumericPredicatesRejectFalseEdgeEvidence(t *testing.T) {
	operation := equation.BoundEquation{Operands: []equation.BoundOperand{
		{Role: "implied-00000000", Value: encodeEvidence(t, "false", "false", branchPredicateWire{Kind: "num-ge", Path: "x", NumFloor: 5})},
	}}
	if predicates, err := trueEdgeNumericPredicates(operation); err != nil || len(predicates) != 0 {
		t.Fatalf("false-edge evidence asserts nothing on the true edge, got %v", predicates)
	}
}

func TestTrueEdgeNumericPredicatesIgnoreAnUnknownRole(t *testing.T) {
	// A role this vocabulary does not name is an orthogonal marker, not an
	// assertion, so it neither enters the set nor blocks the roles that do.
	operation := equation.BoundEquation{Operands: []equation.BoundOperand{
		{Role: "recurrence", Value: encodeEvidence(t, "true", "true", branchPredicateWire{Kind: "num-ge", Path: "x", NumFloor: 5})},
		{Role: "implied-00000000", Value: encodeEvidence(t, "true", "true", branchPredicateWire{Kind: "num-ge", Path: "x", NumFloor: 5})},
	}}
	predicates, err := trueEdgeNumericPredicates(operation)
	if err != nil {
		t.Fatal(err)
	}
	if len(predicates) != 1 || predicates[0].NumFloor != 5 {
		t.Fatalf("expected exactly the implied check, got %v", predicates)
	}
}

func TestNegatableBranchSelectorRefusesACompoundCondition(t *testing.T) {
	compound := equation.BoundEquation{Operands: []equation.BoundOperand{
		{Role: "condition", Value: []byte("temp/0")},
		{Role: "implied-00000000", Value: encodeEvidence(t, "true", "true", branchPredicateWire{Kind: "num-ge", Path: "x", NumFloor: 5})},
	}}
	if _, single, err := negatableBranchSelector(compound); err != nil || single {
		t.Fatal("a compound condition's false edge refutes no individual conjunct")
	}
}

func TestBranchNumericTruthProvesTheTrueEdgeOfABoundInsideItsWindow(t *testing.T) {
	predicate, err := front.EncodeBranchPredicateWire(branchPredicateWire{Kind: "num-ge", Path: "y", NumFloor: 0})
	if err != nil {
		t.Fatalf("encoding predicate: %v", err)
	}
	operation := equation.BoundEquation{Operands: []equation.BoundOperand{
		{Role: "predicate", Value: predicate},
	}}
	truth, decided, err := branchNumericTruth(operation, residueWindowPartition(t, "y", `{"low":1,"high":3}`))
	if err != nil || !decided || !truth {
		t.Fatalf("a window at [1, 3] proves y >= 0, got truth=%v decided=%v", truth, decided)
	}
}

func TestBranchNumericTruthLeavesAnUndecidedBoundAlone(t *testing.T) {
	predicate, err := front.EncodeBranchPredicateWire(branchPredicateWire{Kind: "num-ge", Path: "y", NumFloor: 2})
	if err != nil {
		t.Fatalf("encoding predicate: %v", err)
	}
	operation := equation.BoundEquation{Operands: []equation.BoundOperand{
		{Role: "predicate", Value: predicate},
	}}
	if _, decided, err := branchNumericTruth(operation, residueWindowPartition(t, "y", `{"low":0,"high":3}`)); err != nil || decided {
		t.Fatal("a window straddling the floor decides neither arm")
	}
}

// branchDiagnostics returns every diagnostic key the checked source publishes.
func branchDiagnostics(t *testing.T, source string) []string {
	t.Helper()
	result, err := Check(source)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	keys := make([]string, 0, len(result.Diagnostics))
	for _, diagnostic := range result.Diagnostics {
		keys = append(keys, diagnostic.Key)
	}
	return keys
}

func TestRefutedArmWithholdsItsDiagnostics(t *testing.T) {
	dead := `
local function f(zs: {number}, i: integer): number
    if i >= 5 and i <= 2 and #zs >= 3 then
        local v: number = zs[(i % 5) + 1]
        return v
    end
    return 0
end
return f
`
	live := `
local function f(zs: {number}, i: integer): number
    if i >= 2 and i <= 5 and #zs >= 3 then
        local v: number = zs[(i % 5) + 1]
        return v
    end
    return 0
end
return f
`
	if keys := branchDiagnostics(t, dead); len(keys) != 0 {
		t.Fatalf("an arm with no model owes no obligation, got %v", keys)
	}
	if keys := branchDiagnostics(t, live); len(keys) != 1 {
		t.Fatalf("a satisfiable arm still owes its read's obligation, got %v", keys)
	}
}

func TestResidueWindowRefutedArmWithholdsItsDiagnostics(t *testing.T) {
	dead := `
local function f(zs: {number}, i: integer): number
    i = i % 4
    if i >= 0 and #zs >= 3 and i == 5 then
        local v: number = zs[i + 1]
        return v
    end
    return 0
end
return f
`
	live := `
local function f(zs: {number}, i: integer): number
    i = i % 4
    if i >= 0 and #zs >= 3 and i == 1 then
        local v: number = zs[i + 1]
        return v
    end
    return 0
end
return f
`
	if keys := branchDiagnostics(t, dead); len(keys) != 0 {
		t.Fatalf("the residue window refutes y == 5, so its arm owes nothing, got %v", keys)
	}
	if keys := branchDiagnostics(t, live); len(keys) != 1 {
		t.Fatalf("the window admits y == 1, so the arm keeps its obligation, got %v", keys)
	}
}

func TestDisjunctionKeepsBothArms(t *testing.T) {
	// Every value of i reaches this arm through one disjunct or the other. The
	// obligation inside it must survive.
	source := `
local function f(zs: {number}, i: integer): number
    if i >= 0 and #zs >= 3 and (i >= 5 or i <= 2) then
        local v: number = zs[(i % 5) + 1]
        return v
    end
    return 0
end
return f
`
	if keys := branchDiagnostics(t, source); len(keys) != 1 {
		t.Fatalf("a reachable disjunctive arm keeps its obligation, got %v", keys)
	}
}
