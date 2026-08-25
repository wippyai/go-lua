package program

import (
	"testing"

	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
)

func TestContainmentProgramDeclaresTwoCompleteVectorsAndOneRoutedJoin(t *testing.T) {
	declaration := PlacementContainment()
	if problem, valid := declaration.Check(); !valid {
		t.Fatalf("containment declaration rejected: %+v", problem)
	}
	if declaration.Candidate.AxisRelation.Member != ContainmentCandidates || declaration.JoinCount() != 3 {
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
	output := declaration.Fold.Outputs[0]
	if output.Mode != ruleprogram.ModeRoute || !output.RouteJoinPresent || output.RouteJoin != 2 || output.Destination.Member != ContainmentRouteDestination {
		t.Fatalf("output=%+v", output)
	}
	entry := RuleEntry()
	if entry.Key != RuleKey || entry.Program.OperandRole != declaration.OperandRole || len(entry.Issues) != 0 || len(StructureSpecs()) != 2 {
		t.Fatalf("rule entry=%+v specs=%d", entry, len(StructureSpecs()))
	}
}
