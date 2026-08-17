package continuation

import (
	"sort"
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/causal"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

func TestContinuationSealBranchDecisionSupportAndBothArms(t *testing.T) {
	fixture := openContinuationFixture(t, continuationBranchGuardSpec())
	cell := continuationTerm(keyspace.FamilyCell, 1)
	for _, term := range []keyspace.Term{continuationTerm(keyspace.FamilyUnary, 2), continuationTerm(keyspace.FamilyUnary, 3)} {
		if count, ok := fixture.result.CellCount(term); !ok || count != 1 {
			t.Fatalf("branch-arm CellCount(%08x) = %d/%v, want inherited Bind Cell", uint32(term), count, ok)
		}
		if got, ok := fixture.result.CellAt(term, 0); !ok || got != cell {
			t.Fatalf("branch-arm CellAt(%08x) = %08x/%v, want Cell 1", uint32(term), uint32(got), ok)
		}
		count, ok := fixture.result.GuardCount(term)
		if !ok || count != 1 {
			t.Fatalf("branch-arm GuardCount(%08x) = %d/%v, want 1/true", uint32(term), count, ok)
		}
		guard, guardOK := fixture.result.GuardAt(term, 0)
		if !guardOK || guard != continuationTerm(keyspace.FamilyBranch, 1) {
			t.Fatalf("branch-arm GuardAt(%08x) = %08x/%v, want Branch 1/true", uint32(term), uint32(guard), guardOK)
		}
	}
}

func continuationBranchGuardSpec() continuationSpec {
	body, trueBody, falseBody := continuationTerm(keyspace.FamilyBody, 1), continuationTerm(keyspace.FamilyBody, 2), continuationTerm(keyspace.FamilyBody, 3)
	branch, unary := continuationTerm(keyspace.FamilyBranch, 1), []keyspace.Term{continuationTerm(keyspace.FamilyUnary, 1), continuationTerm(keyspace.FamilyUnary, 2), continuationTerm(keyspace.FamilyUnary, 3)}
	nils := []keyspace.Term{continuationTerm(keyspace.FamilyNil, 1), continuationTerm(keyspace.FamilyNil, 2), continuationTerm(keyspace.FamilyNil, 3), continuationTerm(keyspace.FamilyNil, 4)}
	values := []keyspace.Term{continuationTerm(keyspace.FamilyValues, 1), continuationTerm(keyspace.FamilyValues, 2), continuationTerm(keyspace.FamilyValues, 3)}
	returns := []keyspace.Term{continuationTerm(keyspace.FamilyReturn, 1), continuationTerm(keyspace.FamilyReturn, 2)}
	return continuationSpec{
		name:   "continuation-branch-guards.lua",
		counts: testContinuationCounts(familyCount(keyspace.FamilyBody, 3), familyCount(keyspace.FamilyBranch, 1), familyCount(keyspace.FamilyUnary, 3), familyCount(keyspace.FamilyNil, 4), familyCount(keyspace.FamilyCell, 1), familyCount(keyspace.FamilyBind, 1), familyCount(keyspace.FamilyValues, 3), familyCount(keyspace.FamilyReturn, 2)),
		rows:   [][]keyspace.Term{{continuationTerm(keyspace.FamilyBind, 1), branch}, {returns[0]}, {returns[1]}},
		binds:  []source.BindCells{{Bind: continuationTerm(keyspace.FamilyBind, 1), Cells: []keyspace.Term{continuationTerm(keyspace.FamilyCell, 1)}}}, nilOwners: []keyspace.Term{body, body, trueBody, falseBody},
		flow: authored.Input{
			Storage:   authored.StorageInput{Cells: []authored.Cell{{Kind: authored.CellLocal, Body: body}}, Binds: []authored.Bind{{Owner: body, Values: values[0]}}},
			Values:    authored.ValuesInput{Rows: []authored.Value{{Owner: body, Fixed: authored.Range{End: 1}}, {Owner: trueBody, Fixed: authored.Range{Start: 1, End: 2}}, {Owner: falseBody, Fixed: authored.Range{Start: 2, End: 3}}}, Terms: []keyspace.Term{unary[0], unary[1], unary[2]}},
			Operators: authored.OperatorsInput{Unaries: []authored.Unary{{Owner: body, Op: kind.UnaryNeg, Operand: nils[0]}, {Owner: trueBody, Op: kind.UnaryNeg, Operand: nils[2]}, {Owner: falseBody, Op: kind.UnaryNeg, Operand: nils[3]}}},
			Control:   authored.ControlInput{Branches: []authored.Branch{{Owner: body, Condition: nils[1], WhenTrue: trueBody, WhenFalse: falseBody}}, Returns: []authored.Return{{Owner: trueBody, Values: values[1]}, {Owner: falseBody, Values: values[2]}}},
		},
	}
}

func TestContinuationSealGenericLoopDecisionAndChildBoundary(t *testing.T) {
	fixture := openContinuationFixture(t, continuationLoopCellSpec())
	loop := continuationTerm(keyspace.FamilyLoop, 1)
	child := continuationTerm(keyspace.FamilyUnary, 1)
	if count, ok := fixture.result.GuardCount(loop); !ok || count != 0 {
		t.Fatalf("loop-header GuardCount = %d/%v, want bottom 0/true", count, ok)
	}
	count, ok := fixture.result.GuardCount(child)
	if !ok || count != 1 {
		t.Fatalf("loop-child GuardCount = %d/%v, want 1/true", count, ok)
	}
	if guard, guardOK := fixture.result.GuardAt(child, 0); !guardOK || guard != loop {
		t.Fatalf("loop-child GuardAt = %08x/%v, want Loop 1/true", uint32(guard), guardOK)
	}
}

func TestContinuationSealGuardMatchesReferenceLateDeltaAndResetEquation(t *testing.T) {
	fixture := openContinuationFixture(t, continuationLoopCellSpec())
	want := referenceContinuationGuards(t, fixture)
	counts := fixture.sourceView.Identity()
	for _, family := range continuationSubjects {
		for ordinal := uint32(1); ordinal <= uint32(counts.FamilyCount(family)); ordinal++ {
			term := continuationTerm(family, ordinal)
			if !subjectFrom(fixture.executable, fixture.candidates, term) {
				continue
			}
			expected := make([]keyspace.Term, 0, len(want[term]))
			for guard := range want[term] {
				expected = append(expected, guard)
			}
			sort.Slice(expected, func(i, j int) bool { return expected[i] < expected[j] })
			gotCount, gotOK := fixture.result.GuardCount(term)
			if !gotOK || gotCount != len(expected) {
				t.Fatalf("reference GuardCount(%08x) = %d/%v, want %d/true", uint32(term), gotCount, gotOK, len(expected))
			}
			for index, wantGuard := range expected {
				gotGuard, guardOK := fixture.result.GuardAt(term, index)
				if !guardOK || gotGuard != wantGuard {
					t.Fatalf("reference GuardAt(%08x,%d) = %08x/%v, want %08x/true", uint32(term), index, uint32(gotGuard), guardOK, uint32(wantGuard))
				}
			}
		}
	}
}

func TestContinuationSealRealLoopResetPrecedesOwnDecision(t *testing.T) {
	fixture := openContinuationFixture(t, continuationLoopResetSpec())
	want := referenceContinuationGuards(t, fixture)
	edges := fixture.causal.Edges()
	loop := continuationTerm(keyspace.FamilyLoop, 1)
	resetFound, decisionFound, removalObserved := false, false, false
	for index := 0; index < edges.Count(); index++ {
		edge, ok := edges.At(index)
		if !ok {
			t.Fatalf("loop Edge %d is unavailable", index)
		}
		// Final causal recurrence puts the reset range on the Mu transfer and
		// the Loop Decision on guarded alternatives; both must participate in
		// the same least-fixed-point equation.
		if edge.Decision == loop {
			if _, supportOK := want[edge.To][loop]; supportOK {
				decisionFound = true
			}
		}
		if edge.Mu == 0 {
			continue
		}
		resetCount, resetOK := edges.ResetCount(index)
		if !resetOK {
			t.Fatalf("loop decision Edge %d has no reset range", index)
		}
		includesLoop := false
		for resetIndex := 0; resetIndex < resetCount; resetIndex++ {
			reset, resetTermOK := edges.ResetAt(index, resetIndex)
			if !resetTermOK {
				t.Fatalf("loop decision Edge %d ResetAt(%d) failed", index, resetIndex)
			}
			includesLoop = includesLoop || reset == loop
		}
		if !includesLoop {
			continue
		}
		resetFound = true
		if _, before := want[edge.From][loop]; before {
			if _, after := want[edge.To][loop]; !after {
				removalObserved = true
			}
		}
	}
	if !resetFound || !decisionFound || !removalObserved {
		t.Fatalf("real loop reset/decision law = reset %v, decision %v, removal %v", resetFound, decisionFound, removalObserved)
	}
}

func TestContinuationSealOwnDecisionSurvivesSameEdgeReset(t *testing.T) {
	// The final causal fixture publishes the Mu/reset carrier and its guarded
	// Loop alternatives as separate typed edges.  Reapply the exact edge-local
	// reset and install that same existing Decision in one transfer to exercise
	// the non-commutative ordering required when both annotations share an Edge.
	fixture := openContinuationFixture(t, continuationLoopResetSpec())
	loop := continuationTerm(keyspace.FamilyLoop, 1)
	counts, countsErr := continuationCounts(fixture.sourceView.Identity())
	if countsErr != nil {
		t.Fatal(countsErr)
	}
	rank, rankOK := guardRank(loop, counts)
	if !rankOK {
		t.Fatal("Loop decision rank is unavailable")
	}
	found := false
	for index := 0; index < fixture.causal.Edges().Count(); index++ {
		edge, ok := fixture.causal.Edges().At(index)
		if !ok || edge.Mu == 0 {
			continue
		}
		resetCount, resetOK := fixture.causal.Edges().ResetCount(index)
		if !resetOK {
			t.Fatalf("Mu Edge %d has no reset range", index)
		}
		includesLoop := false
		for offset := 0; offset < resetCount; offset++ {
			reset, resetTermOK := fixture.causal.Edges().ResetAt(index, offset)
			if !resetTermOK {
				t.Fatalf("Mu Edge %d ResetAt(%d) failed", index, offset)
			}
			includesLoop = includesLoop || reset == loop
		}
		if !includesLoop {
			continue
		}
		var routeSuccessor causal.Successor
		routeFound := false
		successors := fixture.causal.Successors()
		for successorIndex := 0; successorIndex < successors.Count(edge.From); successorIndex++ {
			successor, successorOK := successors.At(edge.From, successorIndex)
			if successorOK && successor.IsLocal() && successor.To == edge.To && successor.Decision == edge.Decision &&
				successor.Truth == edge.Truth && successor.Mu == edge.Mu {
				routeSuccessor = successor
				routeFound = true
				break
			}
		}
		if !routeFound {
			t.Fatalf("Mu Edge %d has no semantic successor route", index)
		}
		route := guardRoute{successor: routeSuccessor, decision: rank}
		if !guardRouteAdmits(route, rank, loop) {
			t.Fatal("same-edge own Decision was lost after reset")
		}
		found = true
		break
	}
	if !found {
		t.Fatal("real loop fixture did not publish a reset edge containing its own Loop Decision")
	}
}

func continuationLoopResetSpec() continuationSpec {
	parent, child, functionBody := continuationTerm(keyspace.FamilyBody, 1), continuationTerm(keyspace.FamilyBody, 2), continuationTerm(keyspace.FamilyBody, 3)
	loop := continuationTerm(keyspace.FamilyLoop, 1)
	function := continuationTerm(keyspace.FamilyFunction, 1)
	returned := continuationTerm(keyspace.FamilyReturn, 1)
	values := continuationTerm(keyspace.FamilyValues, 1)
	unary := continuationTerm(keyspace.FamilyUnary, 1)
	nilValue := continuationTerm(keyspace.FamilyNil, 1)
	return continuationSpec{
		name: "continuation-loop-reset.lua",
		counts: testContinuationCounts(
			familyCount(keyspace.FamilyBody, 3), familyCount(keyspace.FamilyLoop, 1), familyCount(keyspace.FamilyFunction, 1),
			familyCount(keyspace.FamilyReturn, 1), familyCount(keyspace.FamilyValues, 1), familyCount(keyspace.FamilyUnary, 1), familyCount(keyspace.FamilyNil, 1),
		),
		rows:      [][]keyspace.Term{{loop}, nil, {returned}},
		nilOwners: []keyspace.Term{functionBody},
		flow: authored.Input{
			Values:    authored.ValuesInput{Rows: []authored.Value{{Owner: functionBody, Fixed: authored.Range{End: 1}}}, Terms: []keyspace.Term{unary}},
			Functions: authored.FunctionsInput{Rows: []authored.Function{{Owner: parent, Body: functionBody}}},
			Operators: authored.OperatorsInput{Unaries: []authored.Unary{{Owner: functionBody, Op: kind.UnaryNeg, Operand: nilValue}}},
			Control: authored.ControlInput{
				Loops:   []authored.Loop{{Owner: parent, Body: child, Kind: kind.LoopWhile, Control: function}},
				Returns: []authored.Return{{Owner: functionBody, Values: values}},
			},
		},
	}
}

func TestContinuationSealRepeatMultipleIncomingPaths(t *testing.T) {
	fixture := openContinuationFixture(t, continuationRepeatFrontierSpec())
	want := referenceContinuationGuards(t, fixture)
	identity := fixture.sourceView.Identity()
	successors := fixture.causal.Successors()
	condition := continuationTerm(keyspace.FamilyUnary, 1)
	child := continuationTerm(keyspace.FamilyBody, 2)
	incoming := make([]causal.Successor, 0, 2)
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		for ordinal := uint32(1); ordinal <= uint32(identity.FamilyCount(family)); ordinal++ {
			from := continuationTerm(family, ordinal)
			for index := 0; index < successors.Count(from); index++ {
				successor, ok := successors.At(from, index)
				if ok && successor.To == child {
					incoming = append(incoming, successor)
				}
			}
		}
	}
	if len(incoming) != 1 {
		t.Fatalf("Repeat Return-only child Body incoming path count = %d, want exactly one initial path", len(incoming))
	}
	initial := incoming[0]
	if initial.From != continuationTerm(keyspace.FamilyLoop, 1) || initial.Decision != 0 || initial.Truth {
		t.Fatalf("Repeat Return-only child Body incoming path = %#v, want unguarded local Loop initial route", initial)
	}
	for index := 0; index < successors.TotalCount(); index++ {
		successor, ok := successors.TotalAt(index)
		if !ok {
			t.Fatalf("Repeat successor %d is unavailable", index)
		}
		if successor.From == condition && successor.Decision == continuationTerm(keyspace.FamilyLoop, 1) {
			t.Fatalf("Return-only Repeat child published unreachable condition route: %#v", successor)
		}
	}
	for _, subject := range []keyspace.Term{condition, continuationTerm(keyspace.FamilyUnary, 2)} {
		expected := make([]keyspace.Term, 0, len(want[subject]))
		for term := range want[subject] {
			expected = append(expected, term)
		}
		sort.Slice(expected, func(i, j int) bool { return expected[i] < expected[j] })
		count, ok := fixture.result.GuardCount(subject)
		if !ok || count != len(expected) {
			t.Fatalf("Repeat subject %08x GuardCount = %d/%v, want %d/true", uint32(subject), count, ok, len(expected))
		}
		for index, wantTerm := range expected {
			got, gotOK := fixture.result.GuardAt(subject, index)
			if !gotOK || got != wantTerm {
				t.Fatalf("Repeat subject %08x GuardAt(%d) = %08x/%v, want %08x/true", uint32(subject), index, uint32(got), gotOK, uint32(wantTerm))
			}
		}
	}
}

func referenceContinuationGuards(t *testing.T, fixture *continuationFixture) map[keyspace.Term]map[keyspace.Term]struct{} {
	t.Helper()
	support := make(map[keyspace.Term]map[keyspace.Term]struct{})
	queued := make(map[keyspace.Term]bool)
	queue := make([]keyspace.Term, 0)
	// Mirror the production lexical Body-entry projection: a guarded route to
	// a child Body reaches each candidate subject fronted by that Body.
	bodySubjects := make(map[keyspace.Term][]keyspace.Term)
	identity := fixture.sourceView.Identity()
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		for ordinal := uint32(1); ordinal <= uint32(identity.FamilyCount(family)); ordinal++ {
			term := continuationTerm(family, ordinal)
			support[term] = make(map[keyspace.Term]struct{})
			queued[term] = true
			queue = append(queue, term)
			if !subjectFrom(fixture.executable, fixture.candidates, term) {
				continue
			}
			body, _, frontierOK := fixture.sourceView.Index().Frontier(term)
			if !frontierOK {
				t.Fatalf("reference subject %08x lacks Source Frontier", uint32(term))
			}
			bodySubjects[body] = append(bodySubjects[body], term)
		}
	}
	successors := fixture.causal.Successors()
	for cursor := 0; cursor < len(queue); cursor++ {
		from := queue[cursor]
		queued[from] = false
		incoming := support[from]
		if keyspace.TermFamily(from) == keyspace.FamilyBody {
			for _, subject := range bodySubjects[from] {
				to := support[subject]
				changed := false
				for term := range incoming {
					if _, exists := to[term]; !exists {
						to[term] = struct{}{}
						changed = true
					}
				}
				if changed && !queued[subject] {
					queued[subject] = true
					queue = append(queue, subject)
				}
			}
		}
		for index := 0; index < successors.Count(from); index++ {
			successor, ok := successors.At(from, index)
			if !ok {
				t.Fatalf("reference Successors.At(%08x,%d) failed", uint32(from), index)
			}
			transfer := make(map[keyspace.Term]struct{}, len(incoming)+1)
			for term := range incoming {
				transfer[term] = struct{}{}
			}
			if successor.IsLocal() {
				resetCount, resetOK := successor.ResetCount()
				if successor.Mu != 0 && !resetOK {
					t.Fatalf("reference reset range for route %08x -> %08x unavailable", uint32(successor.From), uint32(successor.To))
				}
				if resetOK {
					for resetIndex := 0; resetIndex < resetCount; resetIndex++ {
						resetTerm, termOK := successor.ResetAt(resetIndex)
						if !termOK {
							t.Fatalf("reference route ResetAt(%08x -> %08x,%d) failed", uint32(successor.From), uint32(successor.To), resetIndex)
						}
						delete(transfer, resetTerm)
					}
				}
			}
			if successor.Decision != 0 {
				transfer[successor.Decision] = struct{}{}
			}
			to := support[successor.To]
			changed := false
			for term := range transfer {
				if _, exists := to[term]; !exists {
					to[term] = struct{}{}
					changed = true
				}
			}
			if changed && !queued[successor.To] {
				queued[successor.To] = true
				queue = append(queue, successor.To)
			}
		}
	}
	return support
}

func TestContinuationSealCallBoundaryArmsDirectSelectAndTail(t *testing.T) {
	direct := openContinuationFixture(t, directContinuationSpec("continuation-boundary-direct.lua"))
	assertCallBoundaryArms(t, direct, continuationTerm(keyspace.FamilyCall, 1), 4)
	selectFixture := openContinuationFixture(t, continuationSelectCallSpec())
	assertCallBoundaryArms(t, selectFixture, continuationTerm(keyspace.FamilyCall, 1), 5)
	tail := openContinuationFixture(t, continuationTailCallSpec())
	assertCallBoundaryArms(t, tail, continuationTerm(keyspace.FamilyCall, 1), 4)
}

func TestContinuationSealBoundaryPropagatesSupportToNormalResume(t *testing.T) {
	fixture := openContinuationFixture(t, continuationBoundaryPropagationSpec())
	branch := continuationTerm(keyspace.FamilyBranch, 1)
	first := continuationTerm(keyspace.FamilyCall, 1)
	second := continuationTerm(keyspace.FamilyCall, 2)
	assertCallBoundaryArmsWithGuard(t, fixture, first, 4, 1)
	assertCallBoundaryArmsWithGuard(t, fixture, second, 4, 1)
	for _, call := range []keyspace.Term{first, second} {
		count, ok := fixture.result.GuardCount(call)
		if !ok || count != 1 {
			t.Fatalf("boundary Call %08x GuardCount = %d/%v, want one incoming Branch", uint32(call), count, ok)
		}
		guard, ok := fixture.result.GuardAt(call, 0)
		if !ok || guard != branch {
			t.Fatalf("boundary Call %08x GuardAt = %08x/%v, want Branch 1", uint32(call), uint32(guard), ok)
		}
	}
	successors := fixture.causal.Successors()
	want := referenceContinuationGuards(t, fixture)

	boundary, boundaryOK := fixture.causal.Boundaries().For(first)
	if !boundaryOK {
		t.Fatal("first Call boundary is unavailable")
	}
	foundResume := false
	for index := 0; index < successors.Count(first); index++ {
		successor, ok := successors.At(first, index)
		if ok && successor.Arm == causal.BoundaryResume {
			if successor.To != boundary.Normal {
				t.Fatalf("first Call normal resume target = %08x, want sealed Boundary.Normal", uint32(successor.To))
			}
			if _, supportOK := want[successor.To][branch]; !supportOK {
				t.Fatalf("first Call normal target %08x dropped incoming Branch support", uint32(successor.To))
			}
			foundResume = true
		}
	}
	if !foundResume {
		t.Fatal("first Call omitted its direct normal resume arm")
	}
}

func TestContinuationSealBoundaryPreservesIncomingSupportAcrossAllDirectArms(t *testing.T) {
	fixture := openContinuationFixture(t, continuationBoundaryPropagationSpec())
	branch := continuationTerm(keyspace.FamilyBranch, 1)
	want := referenceContinuationGuards(t, fixture)
	for _, call := range []keyspace.Term{continuationTerm(keyspace.FamilyCall, 1), continuationTerm(keyspace.FamilyCall, 2)} {
		for index := 0; index < fixture.causal.Successors().Count(call); index++ {
			successor, ok := fixture.causal.Successors().At(call, index)
			if !ok || !successor.IsBoundary() {
				t.Fatalf("Call %08x arm %d unavailable", uint32(call), index)
			}
			if _, supportOK := want[successor.To][branch]; !supportOK {
				t.Fatalf("Call %08x %v target %08x dropped incoming Branch", uint32(call), successor.Arm, uint32(successor.To))
			}
		}
	}
}

func TestContinuationSealBoundaryPreservesIncomingSupportAcrossSelectModes(t *testing.T) {
	for _, testCase := range []struct {
		name string
		op   kind.SelectOp
	}{
		{name: "and", op: kind.SelectAnd},
		{name: "or", op: kind.SelectOr},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := openContinuationFixture(t, continuationBoundarySelectPropagationSpec(testCase.op))
			call := continuationTerm(keyspace.FamilyCall, 1)
			branch := continuationTerm(keyspace.FamilyBranch, 1)
			assertCallBoundaryArmsWithGuard(t, fixture, call, 5, 1)
			boundary, boundaryOK := fixture.causal.Boundaries().For(call)
			if !boundaryOK || boundary.Other == 0 {
				t.Fatalf("Select-%s Call boundary = %#v/%v, want alternate target", testCase.name, boundary, boundaryOK)
			}
			want := referenceContinuationGuards(t, fixture)
			for index := 0; index < fixture.causal.Successors().Count(call); index++ {
				successor, ok := fixture.causal.Successors().At(call, index)
				if !ok || !successor.IsBoundary() {
					t.Fatalf("Select-%s Call arm %d unavailable", testCase.name, index)
				}
				if _, supportOK := want[successor.To][branch]; !supportOK {
					t.Fatalf("Select-%s Call %v target %08x dropped incoming Branch", testCase.name, successor.Arm, uint32(successor.To))
				}
				switch successor.Arm {
				case causal.BoundarySelectTrue:
					wantTarget := boundary.Other
					if testCase.op == kind.SelectOr {
						wantTarget = boundary.Normal
					}
					if successor.To != wantTarget {
						t.Fatalf("Select-%s true arm target = %08x, want %08x", testCase.name, uint32(successor.To), uint32(wantTarget))
					}
				case causal.BoundarySelectFalse:
					wantTarget := boundary.Normal
					if testCase.op == kind.SelectOr {
						wantTarget = boundary.Other
					}
					if successor.To != wantTarget {
						t.Fatalf("Select-%s false arm target = %08x, want %08x", testCase.name, uint32(successor.To), uint32(wantTarget))
					}
				}
			}
		})
	}
}

func TestContinuationSealTailBoundaryPreservesIncomingSupport(t *testing.T) {
	fixture := openContinuationFixture(t, continuationBoundaryTailPropagationSpec())
	call := continuationTerm(keyspace.FamilyCall, 1)
	branch := continuationTerm(keyspace.FamilyBranch, 1)
	assertCallBoundaryArmsWithGuard(t, fixture, call, 4, 1)
	want := referenceContinuationGuards(t, fixture)
	for index := 0; index < fixture.causal.Successors().Count(call); index++ {
		successor, ok := fixture.causal.Successors().At(call, index)
		if !ok || !successor.IsBoundary() {
			t.Fatalf("tail Call arm %d unavailable", index)
		}
		if _, supportOK := want[successor.To][branch]; !supportOK {
			t.Fatalf("tail Call %v target %08x dropped incoming Branch", successor.Arm, uint32(successor.To))
		}
	}
}

func continuationBoundaryPropagationSpec() continuationSpec {
	body, trueBody, falseBody := continuationTerm(keyspace.FamilyBody, 1), continuationTerm(keyspace.FamilyBody, 2), continuationTerm(keyspace.FamilyBody, 3)
	branch := continuationTerm(keyspace.FamilyBranch, 1)
	calls := []keyspace.Term{continuationTerm(keyspace.FamilyCall, 1), continuationTerm(keyspace.FamilyCall, 2)}
	values := []keyspace.Term{continuationTerm(keyspace.FamilyValues, 1), continuationTerm(keyspace.FamilyValues, 2)}
	nils := []keyspace.Term{continuationTerm(keyspace.FamilyNil, 1), continuationTerm(keyspace.FamilyNil, 2), continuationTerm(keyspace.FamilyNil, 3)}
	return continuationSpec{
		name: "continuation-boundary-propagation.lua",
		counts: testContinuationCounts(
			familyCount(keyspace.FamilyBody, 3), familyCount(keyspace.FamilyBranch, 1), familyCount(keyspace.FamilyCall, 2),
			familyCount(keyspace.FamilyValues, 2), familyCount(keyspace.FamilyNil, 3),
		),
		rows:      [][]keyspace.Term{{branch}, calls, nil},
		nilOwners: []keyspace.Term{body, trueBody, trueBody},
		flow: authored.Input{
			Values: authored.ValuesInput{Rows: []authored.Value{{Owner: trueBody}, {Owner: trueBody}}},
			Calls: []authored.Call{
				{Owner: trueBody, Callee: nils[1], Actuals: values[0]},
				{Owner: trueBody, Callee: nils[2], Actuals: values[1]},
			},
			Control: authored.ControlInput{Branches: []authored.Branch{{Owner: body, Condition: nils[0], WhenTrue: trueBody, WhenFalse: falseBody}}},
		},
	}
}

func continuationBoundarySelectPropagationSpec(op kind.SelectOp) continuationSpec {
	body, trueBody, falseBody := continuationTerm(keyspace.FamilyBody, 1), continuationTerm(keyspace.FamilyBody, 2), continuationTerm(keyspace.FamilyBody, 3)
	branch := continuationTerm(keyspace.FamilyBranch, 1)
	selectTerm := continuationTerm(keyspace.FamilySelect, 1)
	call := continuationTerm(keyspace.FamilyCall, 1)
	values := []keyspace.Term{continuationTerm(keyspace.FamilyValues, 1), continuationTerm(keyspace.FamilyValues, 2)}
	nils := []keyspace.Term{continuationTerm(keyspace.FamilyNil, 1), continuationTerm(keyspace.FamilyNil, 2), continuationTerm(keyspace.FamilyNil, 3)}
	returned := continuationTerm(keyspace.FamilyReturn, 1)
	return continuationSpec{
		name: "continuation-boundary-select-propagation.lua",
		counts: testContinuationCounts(
			familyCount(keyspace.FamilyBody, 3), familyCount(keyspace.FamilyBranch, 1), familyCount(keyspace.FamilySelect, 1),
			familyCount(keyspace.FamilyCall, 1), familyCount(keyspace.FamilyValues, 2), familyCount(keyspace.FamilyNil, 3), familyCount(keyspace.FamilyReturn, 1),
		),
		rows:      [][]keyspace.Term{{branch}, {returned}, nil},
		nilOwners: []keyspace.Term{body, trueBody, trueBody},
		flow: authored.Input{
			Values:    authored.ValuesInput{Rows: []authored.Value{{Owner: trueBody, Fixed: authored.Range{End: 1}}, {Owner: trueBody, Fixed: authored.Range{Start: 1, End: 1}}}, Terms: []keyspace.Term{selectTerm}},
			Calls:     []authored.Call{{Owner: trueBody, Callee: nils[2], Actuals: values[1]}},
			Operators: authored.OperatorsInput{Selects: []authored.Select{{Owner: trueBody, Op: op, Left: call, Right: nils[1]}}},
			Control:   authored.ControlInput{Branches: []authored.Branch{{Owner: body, Condition: nils[0], WhenTrue: trueBody, WhenFalse: falseBody}}, Returns: []authored.Return{{Owner: trueBody, Values: values[0]}}},
		},
	}
}

func continuationBoundaryTailPropagationSpec() continuationSpec {
	body, trueBody, falseBody := continuationTerm(keyspace.FamilyBody, 1), continuationTerm(keyspace.FamilyBody, 2), continuationTerm(keyspace.FamilyBody, 3)
	branch := continuationTerm(keyspace.FamilyBranch, 1)
	call := continuationTerm(keyspace.FamilyCall, 1)
	values := []keyspace.Term{continuationTerm(keyspace.FamilyValues, 1), continuationTerm(keyspace.FamilyValues, 2)}
	nils := []keyspace.Term{continuationTerm(keyspace.FamilyNil, 1), continuationTerm(keyspace.FamilyNil, 2)}
	returned := continuationTerm(keyspace.FamilyReturn, 1)
	return continuationSpec{
		name: "continuation-boundary-tail-propagation.lua",
		counts: testContinuationCounts(
			familyCount(keyspace.FamilyBody, 3), familyCount(keyspace.FamilyBranch, 1), familyCount(keyspace.FamilyCall, 1),
			familyCount(keyspace.FamilyReturn, 1), familyCount(keyspace.FamilyValues, 2), familyCount(keyspace.FamilyNil, 2),
		),
		rows:      [][]keyspace.Term{{branch}, {returned}, nil},
		nilOwners: []keyspace.Term{body, trueBody},
		flow: authored.Input{
			Values:  authored.ValuesInput{Rows: []authored.Value{{Owner: trueBody, Tail: call}, {Owner: trueBody}}},
			Calls:   []authored.Call{{Owner: trueBody, Callee: nils[1], Actuals: values[1]}},
			Control: authored.ControlInput{Branches: []authored.Branch{{Owner: body, Condition: nils[0], WhenTrue: trueBody, WhenFalse: falseBody}}, Returns: []authored.Return{{Owner: trueBody, Values: values[0]}}},
		},
	}
}

func assertCallBoundaryArms(t *testing.T, fixture *continuationFixture, call keyspace.Term, want int) {
	assertCallBoundaryArmsWithGuard(t, fixture, call, want, 0)
}

func assertCallBoundaryArmsWithGuard(t *testing.T, fixture *continuationFixture, call keyspace.Term, want, wantGuard int) {
	t.Helper()
	if got := fixture.causal.Successors().Count(call); got != want {
		t.Fatalf("Call %08x successor denominator = %d, want %d", uint32(call), got, want)
	}
	boundary, boundaryOK := fixture.causal.Boundaries().For(call)
	if !boundaryOK {
		t.Fatalf("Call %08x boundary target row is unavailable", uint32(call))
	}
	seen := make(map[causal.BoundaryArmKind]bool, 7)
	for index := 0; index < want; index++ {
		successor, ok := fixture.causal.Successors().At(call, index)
		if !ok || !successor.IsBoundary() || successor.From != call || successor.To == 0 {
			t.Fatalf("Call %08x boundary arm %d = %#v/%v, target must be present", uint32(call), index, successor, ok)
		}
		if seen[successor.Arm] {
			t.Fatalf("Call %08x boundary arm %d repeated", uint32(call), successor.Arm)
		}
		seen[successor.Arm] = true
		switch successor.Arm {
		case causal.BoundaryResume:
			if boundary.Normal == 0 || successor.To != boundary.Normal || successor.Decision != 0 {
				t.Fatalf("Call %08x direct normal target = %#v, want %08x", uint32(call), successor, uint32(boundary.Normal))
			}
		case causal.BoundarySelectTrue, causal.BoundarySelectFalse:
			if boundary.Normal == 0 || boundary.Other == 0 || successor.Decision != boundary.Normal || (successor.To != boundary.Normal && successor.To != boundary.Other) {
				t.Fatalf("Call %08x Select arm target = %#v, want one of %08x/%08x with decision", uint32(call), successor, uint32(boundary.Normal), uint32(boundary.Other))
			}
		case causal.BoundaryTail:
			if boundary.TailReturn == 0 || successor.To != boundary.TailReturn || successor.Decision != 0 {
				t.Fatalf("Call %08x tail target = %#v, want %08x", uint32(call), successor, uint32(boundary.TailReturn))
			}
		case causal.BoundaryThrow:
			if successor.To != boundary.Throw || successor.Decision != 0 {
				t.Fatalf("Call %08x throw target = %#v, want %08x", uint32(call), successor, uint32(boundary.Throw))
			}
		case causal.BoundaryYield:
			if successor.To != boundary.Yield || successor.Decision != 0 {
				t.Fatalf("Call %08x yield target = %#v, want %08x", uint32(call), successor, uint32(boundary.Yield))
			}
		case causal.BoundaryCancel:
			if successor.To != boundary.Cancel || successor.Decision != 0 {
				t.Fatalf("Call %08x cancel target = %#v, want %08x", uint32(call), successor, uint32(boundary.Cancel))
			}
		default:
			t.Fatalf("Call %08x has unknown boundary arm %d", uint32(call), successor.Arm)
		}
		if subjectFrom(fixture.executable, fixture.candidates, successor.To) {
			if _, ok := fixture.result.GuardCount(successor.To); !ok {
				t.Fatalf("Call %08x boundary target %08x lost its Guard root", uint32(call), uint32(successor.To))
			}
		}
	}
	if boundary.Normal != 0 && boundary.Other == 0 && boundary.TailReturn == 0 && !seen[causal.BoundaryResume] {
		t.Fatalf("Call %08x omitted direct normal arm", uint32(call))
	}
	if boundary.Other != 0 && (!seen[causal.BoundarySelectTrue] || !seen[causal.BoundarySelectFalse]) {
		t.Fatalf("Call %08x omitted one Select boundary arm", uint32(call))
	}
	if boundary.TailReturn != 0 && !seen[causal.BoundaryTail] {
		t.Fatalf("Call %08x omitted tail boundary arm", uint32(call))
	}
	for _, arm := range []causal.BoundaryArmKind{causal.BoundaryThrow, causal.BoundaryYield, causal.BoundaryCancel} {
		if !seen[arm] {
			t.Fatalf("Call %08x omitted exceptional boundary arm %d", uint32(call), arm)
		}
	}
	if count, ok := fixture.result.GuardCount(call); !ok || count != wantGuard {
		t.Fatalf("Call %08x boundary Guard root = %d/%v, want %d/true", uint32(call), count, ok, wantGuard)
	}
}

func continuationSelectCallSpec() continuationSpec {
	body := continuationTerm(keyspace.FamilyBody, 1)
	return continuationSpec{
		name:   "continuation-boundary-select.lua",
		counts: testContinuationCounts(familyCount(keyspace.FamilyBody, 1), familyCount(keyspace.FamilyReturn, 1), familyCount(keyspace.FamilySelect, 1), familyCount(keyspace.FamilyCall, 1), familyCount(keyspace.FamilyValues, 2), familyCount(keyspace.FamilyNil, 2)),
		rows:   [][]keyspace.Term{{continuationTerm(keyspace.FamilyReturn, 1)}}, nilOwners: []keyspace.Term{body, body},
		flow: authored.Input{
			Values:    authored.ValuesInput{Rows: []authored.Value{{Owner: body, Fixed: authored.Range{End: 1}}, {Owner: body, Fixed: authored.Range{Start: 1, End: 1}}}, Terms: []keyspace.Term{continuationTerm(keyspace.FamilySelect, 1)}},
			Calls:     []authored.Call{{Owner: body, Callee: continuationTerm(keyspace.FamilyNil, 2), Actuals: continuationTerm(keyspace.FamilyValues, 2)}},
			Operators: authored.OperatorsInput{Selects: []authored.Select{{Owner: body, Op: kind.SelectAnd, Left: continuationTerm(keyspace.FamilyCall, 1), Right: continuationTerm(keyspace.FamilyNil, 1)}}},
			Control:   authored.ControlInput{Returns: []authored.Return{{Owner: body, Values: continuationTerm(keyspace.FamilyValues, 1)}}},
		},
	}
}

func continuationTailCallSpec() continuationSpec {
	body := continuationTerm(keyspace.FamilyBody, 1)
	return continuationSpec{
		name:   "continuation-boundary-tail.lua",
		counts: testContinuationCounts(familyCount(keyspace.FamilyBody, 1), familyCount(keyspace.FamilyReturn, 1), familyCount(keyspace.FamilyCall, 1), familyCount(keyspace.FamilyValues, 2), familyCount(keyspace.FamilyNil, 1)),
		rows:   [][]keyspace.Term{{continuationTerm(keyspace.FamilyReturn, 1)}}, nilOwners: []keyspace.Term{body},
		flow: authored.Input{
			Values:  authored.ValuesInput{Rows: []authored.Value{{Owner: body, Tail: continuationTerm(keyspace.FamilyCall, 1)}, {Owner: body}}, Terms: nil},
			Calls:   []authored.Call{{Owner: body, Callee: continuationTerm(keyspace.FamilyNil, 1), Actuals: continuationTerm(keyspace.FamilyValues, 2)}},
			Control: authored.ControlInput{Returns: []authored.Return{{Owner: body, Values: continuationTerm(keyspace.FamilyValues, 1)}}},
		},
	}
}
