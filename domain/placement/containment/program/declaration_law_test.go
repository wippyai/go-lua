package program

import (
	"testing"

	programissuance "github.com/wippyai/go-lua/analysis/schema/program/issuance"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
)

func TestContainmentProgramDeclaresCompleteRoutePrerequisitesAndOneTwoCellRoutedRead(t *testing.T) {
	declaration := PlacementContainment()
	if problem, valid := declaration.Check(); !valid {
		t.Fatalf("containment declaration rejected: %+v", problem)
	}
	// The candidate is the issued Program row, not an axis relation: a
	// rule-specific candidate directory in the axis owner's schema would be a
	// second authority over rows Program already issues.
	if !declaration.Candidate.Issued() || declaration.Candidate.IssuedRow != programissuance.RelationOccurrenceEntryGeometry ||
		declaration.Candidate.AxisRelation.Declared() || declaration.JoinCount() != 3 {
		t.Fatalf("candidate=%+v joins=%d", declaration.Candidate, declaration.JoinCount())
	}
	first, _ := declaration.JoinAt(0)
	second, _ := declaration.JoinAt(1)
	third, _ := declaration.JoinAt(2)
	if first.Read.Form != ruleprogram.Complete || second.Read.Form != ruleprogram.Complete {
		t.Fatalf("summary forms=%v/%v, want Complete/Complete", first.Read.Form, second.Read.Form)
	}
	if first.Read.PointBound != ruleprogram.PointBound || second.Read.PointBound != ruleprogram.PointBound || third.Read.PointBound != ruleprogram.PointBound {
		t.Fatalf("point bounds=%v/%v/%v, want the transported operand point", first.Read.PointBound, second.Read.PointBound, third.Read.PointBound)
	}
	if third.Read.Form != ruleprogram.Selected || third.Predicate.Declared() || third.Read.Contract.DenominatorRef.EntryReference().Key != "coordinates/placement" {
		t.Fatalf("route join=%+v", third)
	}
	if third.Relation.Member != ContainmentRoutes || third.Key.Member != ContainmentRouteKey || third.Selection.Member != ContainmentRouteSelection ||
		len(third.Sources) != 3 || !third.Sources[0].Candidate || third.Sources[1] != ruleprogram.PriorSource(0) || third.Sources[2] != ruleprogram.PriorSource(1) {
		t.Fatalf("route derivation=%+v, want candidate plus both complete vector results", third)
	}
	if inputs := declaration.Fold.Inputs; len(inputs) != 2 || inputs[0] != 2 || inputs[1] != 2 {
		t.Fatalf("fold inputs=%v, want the selected route row twice for child and retained parent cells", inputs)
	}
	output := declaration.Fold.Outputs[0]
	if output.Mode != ruleprogram.ModeRoute || !output.RouteJoinPresent || output.RouteJoin != 2 || output.Destination.Member != ContainmentRouteDestination {
		t.Fatalf("output=%+v", output)
	}
	entry := RuleEntry()
	if entry.Key != RuleKey || entry.Program.OperandRole != declaration.OperandRole || len(entry.Issues) != 0 || len(StructureSpecs()) != 2 {
		t.Fatalf("rule entry=%+v specs=%d", entry, len(StructureSpecs()))
	}
}
