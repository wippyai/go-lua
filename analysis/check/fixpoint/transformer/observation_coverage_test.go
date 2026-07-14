package transformer

import (
	"context"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/observation"
	"github.com/wippyai/go-lua/analysis/engine/operationplan"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lexicalidentity"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestObservationCoverageRequiresEveryFeasibleSiblingWorld(t *testing.T) {
	arena, plan, owner, anchor := observationCoverageFixture(t)
	atom := arena.Root(Root{Kind: RootParam})
	truthy, falsy := arena.Truthy(atom), arena.Falsy(atom)
	owed := func(guard Guard) observationObligation {
		return observationObligation{BodyOwner: owner, Anchor: anchor, Guard: guard}
	}
	evidence := func(guard Guard) ObservationTerm {
		return ObservationTerm{BodyOwner: owner, Kind: ObservationAssignment, Anchor: anchor, Guard: guard}
	}
	rows := []SymbolicCFGRow{
		{Guard: truthy, observationObligations: []observationObligation{owed(truthy)}, Observations: []ObservationTerm{evidence(truthy)}},
		{Guard: falsy, observationObligations: []observationObligation{owed(falsy)}},
	}
	scratch := newObservationCoverageScratch()
	if complete, err := relationRowsCoverObservations(context.Background(), arena, plan, rows, scratch); err != nil || complete {
		t.Fatalf("sibling omission complete/error=%v/%v, want false/nil", complete, err)
	}
	rows[1].Observations = []ObservationTerm{evidence(falsy)}
	if complete, err := relationRowsCoverObservations(context.Background(), arena, plan, rows, scratch); err != nil || !complete {
		t.Fatalf("complementary sibling union complete/error=%v/%v, want true/nil", complete, err)
	}
}

func TestObservationCoverageHandlesUnreachableAndKeepsKeysIsolated(t *testing.T) {
	arena, plan, owner, anchor := observationCoverageFixture(t)
	owed := observationObligation{BodyOwner: owner, Anchor: anchor, Guard: arena.False()}
	rows := []SymbolicCFGRow{{Guard: arena.True(), observationObligations: []observationObligation{owed}}}
	scratch := newObservationCoverageScratch()
	if complete, err := relationRowsCoverObservations(context.Background(), arena, plan, rows, scratch); err != nil || !complete {
		t.Fatalf("unreachable obligation complete/error=%v/%v, want true/nil", complete, err)
	}
	owed.Guard = arena.True()
	foreign := owner
	foreign[0]++
	rows[0] = SymbolicCFGRow{
		Guard: arena.True(), observationObligations: []observationObligation{owed},
		Observations: []ObservationTerm{{BodyOwner: foreign, Kind: ObservationAssignment, Anchor: anchor, Guard: arena.True()}},
	}
	if complete, err := relationRowsCoverObservations(context.Background(), arena, plan, rows, scratch); err != nil || complete {
		t.Fatalf("foreign-owner evidence complete/error=%v/%v, want false/nil", complete, err)
	}
}

func TestObservationCoverageAdmitsCallArgumentOnlyWithCompleteEvidenceAndRoute(t *testing.T) {
	graph := cfg.New()
	call := graph.AddNode(cfg.NodeCall)
	graph.AddEdge(graph.Entry(), call, false)
	graph.AddEdge(call, graph.Exit(), false)
	lowered := wir.NewBody("call-argument-coverage")
	start := lowered.Len()
	lowered.Emit(wir.Instruction{Op: wir.OpCall, Point: call})
	lowered.SetPointRange(call, start, lowered.Len())
	lowered.AssignDebugPointOrdinals(graph)
	var owner lexicalidentity.StableLexicalBodyID
	owner[0] = 9
	site := factflow.NewCallSite(factflow.CallSiteConfig{Point: call, HasPoint: true, ArgumentSources: []factflow.ValueSource{{}}, Final: true})
	plan := operationplan.New(graph, factflow.FactsInput{CallSites: map[cfg.Point]factflow.CallSite{call: site}}).WithObservationIdentity(owner, lowered, graph)
	if !exactObservationCoverage(plan, Shape{}, false) {
		t.Fatal("complete CallArgument vocabulary and same-point route rejected")
	}
	anchor, ok := plan.CallArgumentObservationAnchor(call, 0)
	if !ok {
		t.Fatal("call argument anchor missing")
	}
	arena := NewArena(standard.Registry())
	obligation := observationObligation{BodyOwner: owner, Anchor: anchor, Guard: arena.True()}
	evidence := ObservationTerm{BodyOwner: owner, Kind: ObservationCallArgument, Anchor: anchor, Guard: arena.True()}
	rows := []SymbolicCFGRow{{Guard: arena.True(), observationObligations: []observationObligation{obligation}, Observations: []ObservationTerm{evidence}}}
	scratch := newObservationCoverageScratch()
	if complete, err := relationRowsCoverObservations(context.Background(), arena, plan, rows, scratch); err != nil || !complete {
		t.Fatalf("complete CallArgument evidence=%v/%v", complete, err)
	}
	rows[0].Observations = nil
	if complete, err := relationRowsCoverObservations(context.Background(), arena, plan, rows, scratch); err != nil || complete {
		t.Fatalf("missing CallArgument evidence=%v/%v, want false/nil", complete, err)
	}
}

func TestObservationCoverageRejectsEmptyExitRows(t *testing.T) {
	arena, plan, _, _ := observationCoverageFixture(t)
	if complete, err := relationRowsCoverObservations(context.Background(), arena, plan, nil, newObservationCoverageScratch()); err != nil || complete {
		t.Fatalf("empty exit coverage=%v/%v, want false/nil", complete, err)
	}
}

func TestObservationCoverageCancellation(t *testing.T) {
	arena, plan, _, _ := observationCoverageFixture(t)
	scratch := newObservationCoverageScratch()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if complete, err := relationRowsCoverObservations(ctx, arena, plan, nil, scratch); complete || err != errObservationCoverageCanceled {
		t.Fatalf("canceled coverage complete/error=%v/%v", complete, err)
	}
}

func TestObservationCoverageMoreThan256AtomsCompleteExactlyThroughROBDD(t *testing.T) {
	arena, plan, owner, anchor := observationCoverageFixture(t)
	scratch := newObservationCoverageScratch()
	guards := make([]Guard, 0, 257)
	complement := make([]Guard, 0, 257)
	for index := 0; index < 257; index++ {
		atom := arena.Root(Root{Kind: RootParam, Index: uint32(index)})
		guards = append(guards, arena.Truthy(atom))
		complement = append(complement, arena.Falsy(atom))
	}
	anyTruthy := arena.Or(guards...)
	allFalsy := arena.And(complement...)
	rows := []SymbolicCFGRow{
		{
			Guard:                  arena.True(),
			observationObligations: []observationObligation{{BodyOwner: owner, Anchor: anchor, Guard: arena.True()}},
			Observations:           []ObservationTerm{{BodyOwner: owner, Kind: ObservationAssignment, Anchor: anchor, Guard: anyTruthy}},
		},
		{
			Guard:        arena.True(),
			Observations: []ObservationTerm{{BodyOwner: owner, Kind: ObservationAssignment, Anchor: anchor, Guard: allFalsy}},
		},
	}
	if complete, err := relationRowsCoverObservations(context.Background(), arena, plan, rows, scratch); err != nil || !complete {
		t.Fatalf("257-atom coverage complete/error=%v/%v, want true/nil", complete, err)
	}
	if scratch.applyOps == 0 {
		t.Fatal("257-atom theorem bypassed exact ROBDD proof")
	}
}

func TestObservationCoverageProvesNonPairwiseComplementUnion(t *testing.T) {
	arena, plan, owner, anchor := observationCoverageFixture(t)
	atom := arena.Root(Root{Kind: RootParam})
	rows := []SymbolicCFGRow{
		{Guard: arena.True(), observationObligations: []observationObligation{{BodyOwner: owner, Anchor: anchor, Guard: arena.True()}}, Observations: []ObservationTerm{{BodyOwner: owner, Kind: ObservationAssignment, Anchor: anchor, Guard: arena.Truthy(atom)}}},
		{Guard: arena.True(), Observations: []ObservationTerm{{BodyOwner: owner, Kind: ObservationAssignment, Anchor: anchor, Guard: arena.Falsy(atom)}}},
	}
	complete, err := relationRowsCoverObservations(context.Background(), arena, plan, rows, newObservationCoverageScratch())
	if err != nil || !complete {
		t.Fatalf("complement theorem=%v/%v", complete, err)
	}
}

func TestObservationCoverageROBDDMatchesExhaustiveTruthTablesThroughEightAtoms(t *testing.T) {
	arena, plan, owner, anchor := observationCoverageFixture(t)
	scratch := newObservationCoverageScratch()
	for atomCount := 1; atomCount <= 8; atomCount++ {
		atoms := make([]ValueTerm, atomCount)
		for index := range atoms {
			atoms[index] = arena.Root(Root{Kind: RootParam, Index: uint32(index)})
		}
		for seed := 0; seed < 32; seed++ {
			owedParts, evidenceParts := make([]Guard, atomCount), make([]Guard, atomCount)
			for index, atom := range atoms {
				owedParts[index] = arena.Truthy(atom)
				if seed&(1<<(index%5)) != 0 {
					owedParts[index] = arena.Falsy(atom)
				}
				evidenceParts[index] = arena.Truthy(atom)
				if (seed+index)&1 != 0 {
					evidenceParts[index] = arena.Falsy(atom)
				}
			}
			owed := arena.Or(arena.And(owedParts...), arena.And(evidenceParts...))
			evidence := arena.And(evidenceParts...)
			rows := []SymbolicCFGRow{{Guard: arena.True(), observationObligations: []observationObligation{{BodyOwner: owner, Anchor: anchor, Guard: owed}}, Observations: []ObservationTerm{{BodyOwner: owner, Kind: ObservationAssignment, Anchor: anchor, Guard: evidence}}}}
			got, err := relationRowsCoverObservations(context.Background(), arena, plan, rows, scratch)
			if err != nil {
				t.Fatal(err)
			}
			want := true
			for assignment := 0; assignment < 1<<atomCount; assignment++ {
				if evalCoverageGuard(arena, owed, atoms, assignment) && !evalCoverageGuard(arena, evidence, atoms, assignment) {
					want = false
					break
				}
			}
			if got != want {
				t.Fatalf("atoms=%d seed=%d coverage=%v want=%v", atomCount, seed, got, want)
			}
		}
	}
}

func TestObservationCoverageDeterministicUnderRowShuffleAndFingerprintCollisions(t *testing.T) {
	arena, plan, owner, anchor := observationCoverageFixture(t)
	arena.fingerprintMask = 0
	atoms := []ValueTerm{arena.Root(Root{Kind: RootParam, Index: 3}), arena.Root(Root{Kind: RootParam, Index: 1}), arena.Root(Root{Kind: RootParam, Index: 2})}
	rows := make([]SymbolicCFGRow, 0, len(atoms))
	for _, atom := range atoms {
		guard := arena.Truthy(atom)
		rows = append(rows, SymbolicCFGRow{Guard: guard, observationObligations: []observationObligation{{BodyOwner: owner, Anchor: anchor, Guard: guard}}, Observations: []ObservationTerm{{BodyOwner: owner, Kind: ObservationAssignment, Anchor: anchor, Guard: guard}}})
	}
	scratch := newObservationCoverageScratch()
	first, err := relationRowsCoverObservations(context.Background(), arena, plan, rows, scratch)
	if err != nil || !first {
		t.Fatalf("first coverage=%v/%v", first, err)
	}
	for left, right := 0, len(rows)-1; left < right; left, right = left+1, right-1 {
		rows[left], rows[right] = rows[right], rows[left]
	}
	second, err := relationRowsCoverObservations(context.Background(), arena, plan, rows, scratch)
	if err != nil || second != first {
		t.Fatalf("shuffled coverage=%v/%v, want %v", second, err, first)
	}
}

func BenchmarkObservationCoverageTwoWay(b *testing.B) {
	arena, plan, owner, anchor := observationCoverageFixture(b)
	atom := arena.Root(Root{Kind: RootParam})
	truthy, falsy := arena.Truthy(atom), arena.Falsy(atom)
	rows := []SymbolicCFGRow{
		{Guard: truthy, observationObligations: []observationObligation{{BodyOwner: owner, Anchor: anchor, Guard: truthy}}, Observations: []ObservationTerm{{BodyOwner: owner, Kind: ObservationAssignment, Anchor: anchor, Guard: truthy}}},
		{Guard: falsy, observationObligations: []observationObligation{{BodyOwner: owner, Anchor: anchor, Guard: falsy}}, Observations: []ObservationTerm{{BodyOwner: owner, Kind: ObservationAssignment, Anchor: anchor, Guard: falsy}}},
	}
	scratch := newObservationCoverageScratch()
	_, _ = relationRowsCoverObservations(context.Background(), arena, plan, rows, scratch)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if complete, err := relationRowsCoverObservations(context.Background(), arena, plan, rows, scratch); err != nil || !complete {
			b.Fatal(complete, err)
		}
	}
}

func BenchmarkObservationCoverageAdversarial256Rows(b *testing.B) {
	arena, plan, owner, anchor := observationCoverageFixture(b)
	atoms := make([]ValueTerm, 8)
	for index := range atoms {
		atoms[index] = arena.Root(Root{Kind: RootParam, Index: uint32(index)})
	}
	rows := make([]SymbolicCFGRow, 256)
	for assignment := range rows {
		parts := make([]Guard, len(atoms))
		for index, atom := range atoms {
			parts[index] = arena.Truthy(atom)
			if assignment&(1<<index) == 0 {
				parts[index] = arena.Falsy(atom)
			}
		}
		guard := arena.And(parts...)
		rows[assignment] = SymbolicCFGRow{Guard: guard, observationObligations: []observationObligation{{BodyOwner: owner, Anchor: anchor, Guard: arena.True()}}, Observations: []ObservationTerm{{BodyOwner: owner, Kind: ObservationAssignment, Anchor: anchor, Guard: guard}}}
	}
	scratch := newObservationCoverageScratch()
	_, _ = relationRowsCoverObservations(context.Background(), arena, plan, rows, scratch)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if complete, err := relationRowsCoverObservations(context.Background(), arena, plan, rows, scratch); err != nil || !complete {
			b.Fatal(complete, err)
		}
	}
}

func evalCoverageGuard(arena *Arena, guard Guard, atoms []ValueTerm, assignment int) bool {
	node := arena.guards[guard]
	switch node.op {
	case guardTrue:
		return true
	case guardFalse:
		return false
	case guardTruthy, guardFalsy:
		value := false
		for index, atom := range atoms {
			if atom == node.value {
				value = assignment&(1<<index) != 0
				break
			}
		}
		if node.op == guardFalsy {
			return !value
		}
		return value
	case guardAnd:
		for _, child := range node.args {
			if !evalCoverageGuard(arena, child, atoms, assignment) {
				return false
			}
		}
		return true
	case guardOr:
		for _, child := range node.args {
			if evalCoverageGuard(arena, child, atoms, assignment) {
				return true
			}
		}
	}
	return false
}

type coverageTestingT interface {
	Helper()
	Fatalf(string, ...any)
}

func observationCoverageFixture(t coverageTestingT) (*Arena, *operationplan.Plan, lexicalidentity.StableLexicalBodyID, observation.Occurrence) {
	t.Helper()
	graph := cfg.New()
	assign := graph.AddNode(cfg.NodeAssign)
	graph.AddEdge(graph.Entry(), assign, false)
	graph.AddEdge(assign, graph.Exit(), false)
	lowered := wir.NewBody("coverage")
	lowered.AssignDebugPointOrdinals(graph)
	var owner lexicalidentity.StableLexicalBodyID
	owner[0] = 7
	plan := operationplan.New(graph, factflow.FactsInput{RootAssignments: map[cfg.Point]factflow.RootAssignment{assign: {}}}).WithObservationIdentity(owner, lowered, graph)
	anchor, ok := plan.AssignmentObservationAnchor(assign)
	if !ok {
		t.Fatalf("assignment anchor missing")
	}
	return NewArena(standard.Registry()), plan, owner, anchor
}
