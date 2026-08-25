package program

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	programissuance "github.com/wippyai/go-lua/analysis/schema/program/issuance"
	"github.com/wippyai/go-lua/analysis/schema/rule"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
	"github.com/wippyai/go-lua/analysis/schema/vocabulary"
)

func TestSuspensionProgramDeclaresCatalogBridgeAndRouteMember(t *testing.T) {
	declaration := Suspension()
	assertSuspensionShape(t, declaration, placementAxisKey, "semantic/operand/placement/suspension", suspensionAnchors, suspensionSources, suspensionSourceKey, suspensionSourceTag, suspensionRoutes, suspensionRouteKey, suspensionRouteTag, suspensionRouteDestination, suspensionReducer, "placement/facts")
}

func TestSuspensionEvidenceUsesItsIndependentOutputAxis(t *testing.T) {
	declaration := SuspensionEvidence()
	assertSuspensionShape(t, declaration, "placement-suspension-evidence", "semantic/operand/placement/suspension-evidence", "value/suspension-evidence/anchors", "value/suspension-evidence/sources", "value/suspension-evidence/source-key", "value/suspension-evidence/source-tag", "placement/suspension-evidence/routes", "placement/suspension-evidence/route-key", "placement/suspension-evidence/route-tag", "placement/suspension-evidence/route-destination", "placement-suspension-evidence/reducer", "placement/suspension-evidence/facts")
	if declaration.Fold.Outputs[0].Destination.Member == suspensionRouteDestination {
		t.Fatal("evidence producer reused the suspension RouteMember destination")
	}
	consumer := RuleEntry()
	evidence := EvidenceRuleEntry()
	if consumer.Key != RuleKey || consumer.Program.OperandRole != vocabulary.RoleKey(OperandRole) ||
		evidence.Key != EvidenceRuleKey || evidence.Program.OperandRole != vocabulary.RoleKey(EvidenceOperandRole) || len(StructureSpecs()) != 4 {
		t.Fatalf("consumer=%+v evidence=%+v specs=%d", consumer, evidence, len(StructureSpecs()))
	}
}

func assertSuspensionShape(t *testing.T, declaration ruleprogram.Program, outputAxisKey, role schema.Key, anchor, sources, sourceKey, sourceTag, routes, routeKey, routeTag, destination, reducer, column schema.Key) {
	t.Helper()
	if problem, valid := declaration.Check(); !valid {
		t.Fatalf("suspension declaration rejected: %+v", problem)
	}
	if declaration.OperandRole != role {
		t.Fatalf("operand role=%q, want %q", declaration.OperandRole, role)
	}
	if !declaration.Candidate.Issued() || declaration.Candidate.IssuedRow != programissuance.RelationOccurrenceSubjectLiveness || declaration.Candidate.AxisRelation.Declared() {
		t.Fatalf("candidate=%+v, want canonical issued subject-liveness row", declaration.Candidate)
	}
	if declaration.JoinCount() != 3 {
		t.Fatalf("join count=%d, want anchor/source/route", declaration.JoinCount())
	}

	first, firstOK := declaration.JoinAt(0)
	if !firstOK || first.Read.Form != ruleprogram.Exact || first.Read.Axis.EntryReference().Key != valueAxisKey ||
		first.Relation.Member != anchor || first.Sources[0] != ruleprogram.CandidateSource() || first.Predicate.Declared() {
		t.Fatalf("anchor join=%+v, want candidate-only exact Value read", first)
	}

	second, secondOK := declaration.JoinAt(1)
	if !secondOK || second.Read.Form != ruleprogram.Selected || second.Relation.Member != sources ||
		second.Key.Member != sourceKey || second.Predicate.Member != sourceTag ||
		len(second.Sources) != 2 || !second.Sources[0].Candidate || second.Sources[1] != ruleprogram.PriorSource(0) {
		t.Fatalf("source join=%+v, want candidate + anchor selected Value read", second)
	}

	third, thirdOK := declaration.JoinAt(2)
	if !thirdOK || third.Read.Form != ruleprogram.Selected || third.Read.Axis.EntryReference().Key != outputAxisKey ||
		third.Relation.Member != routes || third.Key.Member != routeKey || third.Predicate.Declared() ||
		len(third.Sources) != 3 || !third.Sources[0].Candidate || third.Sources[1] != ruleprogram.PriorSource(0) || third.Sources[2] != ruleprogram.PriorSource(1) {
		t.Fatalf("route join=%+v, want candidate + both Value reads selected owner RouteMember read", third)
	}
	if first.Read.PointBound != ruleprogram.PointBound || second.Read.PointBound != ruleprogram.PointBound || third.Read.PointBound != ruleprogram.PointBound {
		t.Fatalf("point bounds=%v/%v/%v, want the transported operand point", first.Read.PointBound, second.Read.PointBound, third.Read.PointBound)
	}
	if third.Read.Contract.DenominatorRef.EntryReference().Key != placementDenominator || third.Read.Contract.OnOpaque != ruleprogram.OnOpaqueRefuse {
		t.Fatalf("route contract=%+v, want bounded Placement route delivery", third.Read.Contract)
	}

	if declaration.Fold.Reducer.Member != reducer || len(declaration.Fold.Inputs) != 3 || declaration.Fold.Inputs[0] != 0 || declaration.Fold.Inputs[1] != 1 || declaration.Fold.Inputs[2] != 2 {
		t.Fatalf("fold=%+v, want reducer and all three reads", declaration.Fold)
	}
	if len(declaration.Fold.Outputs) != 1 {
		t.Fatalf("outputs=%d, want one routed receipt", len(declaration.Fold.Outputs))
	}
	output := declaration.Fold.Outputs[0]
	if output.Mode != ruleprogram.ModeRoute || !output.RouteJoinPresent || output.RouteJoin != 2 || output.Column.Key != column || output.Destination.Member != destination {
		t.Fatalf("output=%+v, want explicit RouteMember output", output)
	}
	if declaration.Fold.Reducer.Axis.Key != outputAxisKey || output.Column.Axis.Key != outputAxisKey || output.Destination.Axis.Key != outputAxisKey {
		t.Fatalf("output axis reducer=%q column=%q destination=%q, want %q", declaration.Fold.Reducer.Axis.Key, output.Column.Axis.Key, output.Destination.Axis.Key, outputAxisKey)
	}
	if declaration.Carry == nil || declaration.Carry.Mode != ruleprogram.CarryIdentity || declaration.Carry.Input != 0 {
		t.Fatalf("carry=%+v, want identity carry", declaration.Carry)
	}
	if declaration.TransportCount() != 0 {
		t.Fatal("suspension Link rule declares no activation transport vector")
	}
}

func TestSuspensionRulesIssueAtTheMountedCallSummary(t *testing.T) {
	for _, entry := range []rule.Spec{RuleEntry(), EvidenceRuleEntry()} {
		if entry.Lane != rule.LaneMounted || len(entry.Issues) != 1 {
			t.Fatalf("rule %q lane/issues=%v/%v, want one mounted issuance", entry.Key, entry.Lane, entry.Issues)
		}
		issue := entry.Issues[0]
		if issue.Occurrence != "occurrence/subject-liveness" || issue.Requirement != programissuance.RequirementUnrestricted || issue.Form != programissuance.FormCallSummary {
			t.Fatalf("rule %q issuance=%+v, want subject-liveness call-summary", entry.Key, issue)
		}
	}
}

func TestSuspensionProgramRequiresTheThirdSelectedJoinAsRoute(t *testing.T) {
	declaration := Suspension()
	declaration.Fold.Outputs[0].RouteJoin = 0
	if problem, valid := declaration.Check(); valid || problem.Kind != ruleprogram.ProblemJoin {
		t.Fatalf("exact anchor used as route valid=%v problem=%+v", valid, problem)
	}
}
