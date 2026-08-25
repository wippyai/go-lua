package program

import (
	"testing"

	programissuance "github.com/wippyai/go-lua/analysis/schema/program/issuance"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
)

func TestClosureCaptureDeclarationHasParentSourcesAndOneRoute(t *testing.T) {
	declaration := ClosureCapture()
	if problem, ok := declaration.Check(); !ok {
		t.Fatalf("closure-capture declaration rejected: %+v", problem)
	}
	if !declaration.Candidate.Issued() || declaration.Candidate.IssuedRow != programissuance.RelationOccurrenceClosureProof || declaration.Candidate.AxisRelation.Declared() {
		t.Fatalf("candidate=%+v, want canonical issued closure-proof row", declaration.Candidate)
	}
	if got, want := declaration.JoinCount(), 3; got != want {
		t.Fatalf("join count=%d, want %d", got, want)
	}
	parent, parentOK := declaration.JoinAt(0)
	sources, sourcesOK := declaration.JoinAt(1)
	route, routeOK := declaration.JoinAt(2)
	if !parentOK || parent.Read.Form != ruleprogram.Exact || parent.Relation.Member != captureParents || parent.Key.Member != captureParentKey {
		t.Fatalf("parent join=%+v/%t, want exact Placement parent", parent, parentOK)
	}
	if parent.Read.Input != 0 || sources.Read.Input != 0 || route.Read.Input != 0 {
		t.Fatalf("read inputs=%d/%d/%d, want the single authored operand input", parent.Read.Input, sources.Read.Input, route.Read.Input)
	}
	if parent.Read.PointBound != ruleprogram.PointBound || sources.Read.PointBound != ruleprogram.PointBound || route.Read.PointBound != ruleprogram.PointBound {
		t.Fatalf("point bounds=%v/%v/%v, want the transported operand point", parent.Read.PointBound, sources.Read.PointBound, route.Read.PointBound)
	}
	if !sourcesOK || sources.Read.Form != ruleprogram.Selected || sources.Predicate.Member != captureSourceTag || sources.Relation.Member != captureSources {
		t.Fatalf("source join=%+v/%t, want tagged selected Value sources", sources, sourcesOK)
	}
	if !routeOK || route.Read.Form != ruleprogram.Selected || route.Predicate.Declared() || route.Relation.Member != captureRoutes {
		t.Fatalf("route join=%+v/%t, want owner-issued RouteMember selection", route, routeOK)
	}
	if len(declaration.Fold.Inputs) != 2 || declaration.Fold.Inputs[0] != 0 || declaration.Fold.Inputs[1] != 2 {
		t.Fatalf("fold inputs=%v, want parent and routed Placement joins", declaration.Fold.Inputs)
	}
	output := declaration.Fold.Outputs[0]
	if output.Mode != ruleprogram.ModeRoute || !output.RouteJoinPresent || output.RouteJoin != 2 || output.Destination.Member != captureDestination {
		t.Fatalf("route output=%+v, want explicit route join 2", output)
	}
	if declaration.Carry == nil || declaration.Carry.Input != 0 || declaration.Carry.Mode != ruleprogram.CarryIdentity {
		t.Fatalf("carry=%+v, want the identity of the authored operand", declaration.Carry)
	}
	entry := RuleEntry()
	if entry.Key != RuleKey || entry.Program.OperandRole != declaration.OperandRole || len(entry.Issues) != 1 || len(StructureSpecs()) != 2 {
		t.Fatalf("rule entry=%+v specs=%d", entry, len(StructureSpecs()))
	}
	entry.Issues[0].Occurrence = "mutated"
	if RuleEntry().Issues[0].Occurrence == "mutated" {
		t.Fatal("RuleEntry shares mutable issuance storage")
	}
}
