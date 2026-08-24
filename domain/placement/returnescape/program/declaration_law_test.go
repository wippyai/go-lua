package program

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema"
	ruleprogram "github.com/wippyai/go-lua/analysis/schema/rule/program"
)

func TestReturnEscapeProgramDeclaresTheCurrentThreeReadRoute(t *testing.T) {
	declaration := ReturnEscape()
	if problem, valid := declaration.Check(); !valid {
		t.Fatalf("return-escape declaration rejected: %+v", problem)
	}
	if declaration.OperandRole != "semantic/operand/placement/return-escape" {
		t.Fatalf("operand role=%q", declaration.OperandRole)
	}
	if declaration.Candidate.Axis != axisReference(valueAxisKey) || declaration.Candidate.Member != returnBoundaryCandidates {
		t.Fatalf("candidate=%+v, want Value return-boundary candidates", declaration.Candidate)
	}
	if got := declaration.JoinCount(); got != 3 {
		t.Fatalf("join count=%d, want anchor, member selection, and route selection", got)
	}

	anchor, ok := declaration.JoinAt(0)
	if !ok || anchor.Read.Form != ruleprogram.Exact || anchor.Read.Axis.EntryReference().Key != valueAxisKey ||
		anchor.Relation.Member != returnBoundaryRoots || anchor.Key.Member != returnBoundaryRootKey ||
		len(anchor.Sources) != 1 || !anchor.Sources[0].Candidate || anchor.Predicate.Declared() {
		t.Fatalf("anchor=%+v, want candidate-only exact Value root read", anchor)
	}
	if anchor.Read.Contract.DenominatorRef.EntryReference().Key != valueCoordinateDenominator ||
		anchor.Read.Contract.Sparse != ruleprogram.SparseDefault ||
		anchor.Read.Contract.OnOpaque != ruleprogram.OnOpaquePropagateAuthenticated {
		t.Fatalf("anchor contract=%+v, want authenticated Value default delivery", anchor.Read.Contract)
	}

	members, ok := declaration.JoinAt(1)
	if !ok || members.Read.Form != ruleprogram.Selected || members.Read.Axis.EntryReference().Key != valueAxisKey ||
		members.Relation.Member != returnBoundaryMembers || members.Key.Member != returnBoundaryMemberKey ||
		members.Predicate.Declared() || len(members.Sources) != 1 || !members.Sources[0].Candidate {
		t.Fatalf("members=%+v, want a candidate-sourced self-provided nested Value member read", members)
	}
	if members.Read.Contract.DenominatorRef.EntryReference().Key != valueCoordinateDenominator {
		t.Fatalf("members denominator=%+v, want Value coordinate denominator", members.Read.Contract.DenominatorRef)
	}

	routes, ok := declaration.JoinAt(2)
	if !ok || routes.Read.Form != ruleprogram.Selected || routes.Read.Axis.EntryReference().Key != placementAxisKey ||
		routes.Relation.Member != returnEscapeRoutes || routes.Key.Member != returnEscapeRouteKey ||
		routes.Predicate.Declared() || len(routes.Sources) != 3 ||
		!routes.Sources[0].Candidate || routes.Sources[1] != ruleprogram.PriorSource(0) || routes.Sources[2] != ruleprogram.PriorSource(1) {
		t.Fatalf("routes=%+v, want candidate + both Value reads dependent selected Placement RouteMember read", routes)
	}
	if routes.Read.Contract.DenominatorRef.EntryReference().Key != placementDenominator ||
		routes.Read.Contract.OnOpaque != ruleprogram.OnOpaqueRefuse {
		t.Fatalf("routes contract=%+v, want bounded Placement route delivery", routes.Read.Contract)
	}

	if declaration.Carry != nil {
		t.Fatalf("carry=%+v, want routed predecessor transport without a duplicate carry", declaration.Carry)
	}
	if len(declaration.Fold.Inputs) != 1 || declaration.Fold.Inputs[0] != 2 {
		t.Fatalf("fold inputs=%v, want only the authenticated routed Placement read", declaration.Fold.Inputs)
	}
	if len(declaration.Fold.Outputs) != 1 {
		t.Fatalf("fold outputs=%d, want one routed publication", len(declaration.Fold.Outputs))
	}
	output := declaration.Fold.Outputs[0]
	if output.Mode != ruleprogram.ModeRoute || !output.RouteJoinPresent || output.RouteJoin != 2 ||
		output.Column.Key != placementFactsColumn || output.Destination.Member != returnEscapeRouteDestination {
		t.Fatalf("output=%+v, want explicit RouteMember route output", output)
	}
	if declaration.TransportCount() != 0 {
		t.Fatal("return-escape is a mounted local-successor rule and declares no activation transport vector")
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

func TestReturnEscapeProgramRequiresTheExplicitRouteJoin(t *testing.T) {
	missing := ReturnEscape()
	missing.Fold.Outputs[0].RouteJoinPresent = false
	if problem, valid := missing.Check(); valid || problem.Kind != ruleprogram.ProblemOutput {
		t.Fatalf("missing route join valid=%v problem=%+v", valid, problem)
	}

	exact := ReturnEscape()
	exact.Fold.Outputs[0].RouteJoin = 0
	if problem, valid := exact.Check(); valid || problem.Kind != ruleprogram.ProblemJoin {
		t.Fatalf("exact anchor used as route join valid=%v problem=%+v", valid, problem)
	}

	foreign := ReturnEscape()
	foreign.Candidate.Axis = schema.EntryReference{Surface: schema.SurfaceKindAxis, Key: placementAxisKey}
	if problem, valid := foreign.Check(); !valid {
		// Check is intentionally owner-blind: the upward seal resolves the
		// local key against the complete catalog. Keep this assertion explicit
		// so the law does not pretend a provisional key can prove that seam.
		t.Fatalf("foreign-axis provisional declaration unexpectedly became structurally malformed: %+v", problem)
	}
}
