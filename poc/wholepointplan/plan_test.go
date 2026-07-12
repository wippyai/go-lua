package wholepointplan

import (
	"math/rand"
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/factapply"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestEveryPointKeyedFactsInputFieldIsClassified(t *testing.T) {
	classified := make(map[string]Kind)
	for _, spec := range operationSpecs {
		if spec.Field == "" {
			continue
		}
		if previous, exists := classified[spec.Field]; exists {
			t.Fatalf("field %s classified by both %s and %s", spec.Field, previous, spec.Kind)
		}
		classified[spec.Field] = spec.Kind
	}
	typeOfPoint := reflect.TypeOf(cfg.Point(0))
	typ := reflect.TypeOf(factflow.FactsInput{})
	seen := make(map[string]bool)
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if field.Type.Kind() != reflect.Map || field.Type.Key() != typeOfPoint {
			continue
		}
		if _, ok := classified[field.Name]; !ok {
			t.Fatalf("point-keyed FactsInput field %s has no whole-point operation classification", field.Name)
		}
		seen[field.Name] = true
	}
	for field := range classified {
		if !seen[field] {
			t.Fatalf("stale classification for non-point field %s", field)
		}
	}
}

func TestOperationRegistryMatchesConcreteApplicatorBarriers(t *testing.T) {
	want := map[Kind]struct {
		phase   Phase
		barrier Barrier
		stages  BarrierSet
		role    OperationRole
		owner   Kind
	}{
		OpCallSite:                    {Node, N0Materialize, barriers(N0Materialize, E5CallEffects), Semantic, ""},
		OpCallResultValue:             {Node, N0Materialize, barriers(N0Materialize), Sidecar, OpCallSite},
		OpChannelSelect:               {Node, N0Materialize, barriers(N0Materialize, N3Postconditions), Semantic, ""},
		OpNoNormalReturn:              {Node, N1NoReturn, barriers(N1NoReturn), Semantic, ""},
		OpPathPresenceImplication:     {Node, N2ImplicationClosure, barriers(N2ImplicationClosure), Semantic, ""},
		OpPathDescendantInvalidation:  {Node, N3Postconditions, barriers(N3Postconditions), Semantic, ""},
		OpPostconditionRefinement:     {Node, N3Postconditions, barriers(N3Postconditions), Semantic, ""},
		OpPostconditionPathRelation:   {Node, N3Postconditions, barriers(N3Postconditions), Semantic, ""},
		OpDynamicIndexWrite:           {Node, N4Writes, barriers(N4Writes), Semantic, ""},
		OpRootAssignment:              {Node, N4Writes, barriers(N4Writes), Semantic, ""},
		OpPathAssignment:              {Node, N4Writes, barriers(N4Writes), Semantic, ""},
		OpPathStaticMemberWrite:       {Node, N4Writes, barriers(N4Writes), Semantic, ""},
		OpReturn:                      {Node, N5Return, barriers(N5Return), Semantic, ""},
		OpReturnPresenceRelation:      {Node, N5Return, barriers(N5Return), Sidecar, OpReturn},
		OpCovariantExposure:           {Node, N6CovariantFinalizer, barriers(N6CovariantFinalizer), Semantic, ""},
		OpBranchReachability:          {Edge, E0Reachability, barriers(E0Reachability), Semantic, ""},
		OpBranchConditionSource:       {Edge, E0Reachability, barriers(E0Reachability), Sidecar, OpBranchReachability},
		OpBranchRefinement:            {Edge, E1Refinements, barriers(E1Refinements), Semantic, ""},
		OpBranchSufficientLiteralCase: {Edge, E1Refinements, barriers(E1Refinements), Sidecar, OpBranchRefinement},
		OpBranchLengthRefinement:      {Edge, E3Relations, barriers(E3Relations), Semantic, ""},
		OpBranchNumberFloor:           {Edge, E3Relations, barriers(E3Relations), Semantic, ""},
		OpBranchNumberCeil:            {Edge, E3Relations, barriers(E3Relations), Semantic, ""},
		OpBranchDifferenceConstraint:  {Edge, E3Relations, barriers(E3Relations), Semantic, ""},
		OpBranchPresenceRelation:      {Edge, E3Relations, barriers(E3Relations), Semantic, ""},
		OpBranchPathRelation:          {Edge, E3Relations, barriers(E3Relations), Semantic, ""},
		OpBranchPathEvidence:          {Edge, E4Evidence, barriers(E4Evidence), Semantic, ""},
	}
	if len(want) != len(operationSpecs) {
		t.Fatalf("barrier fixture has %d kinds, registry has %d", len(want), len(operationSpecs))
	}
	for _, spec := range operationSpecs {
		expect, ok := want[spec.Kind]
		if !ok {
			t.Fatalf("operation %s has no concrete-applicator barrier fixture", spec.Kind)
		}
		stages := spec.Stages
		if stages == 0 {
			stages = barriers(spec.Barrier)
		}
		if spec.Phase != expect.phase || spec.Barrier != expect.barrier || stages != expect.stages || spec.Role != expect.role || spec.Owner != expect.owner {
			t.Errorf("%s metadata=(%d,%d,%d,%d,%s), want (%d,%d,%d,%d,%s)", spec.Kind,
				spec.Phase, spec.Barrier, stages, spec.Role, spec.Owner,
				expect.phase, expect.barrier, expect.stages, expect.role, expect.owner)
		}
	}
	if got := canonicalBarriers; got != [13]Barrier{
		N0Materialize, N1NoReturn, N2ImplicationClosure, N3Postconditions, N4Writes, N5Return, N6CovariantFinalizer,
		E0Reachability, E1Refinements, E2ImplicationClosure, E3Relations, E4Evidence, E5CallEffects,
	} {
		t.Fatalf("canonical barrier sequence changed: %v", got)
	}
}

func TestRandomWholePointRowsContainEveryFactExactlyOnce(t *testing.T) {
	rng := rand.New(rand.NewSource(0x17a11))
	const point cfg.Point = 7
	for trial := 0; trial < 500; trial++ {
		input, want := randomWholePoint(rng, point)
		plan, err := Compile(input, Config{})
		if err != nil {
			t.Fatalf("trial %d: Compile: %v", trial, err)
		}
		row := plan.Row(point)
		got := make(map[Kind]int)
		lastRank := -1
		lastOrdinal := make(map[Kind]int)
		for i, op := range row {
			rank := operationRank(op.Kind)
			if rank < lastRank {
				t.Fatalf("trial %d row[%d]=%s is out of production order", trial, i, op.Kind)
			}
			if got[op.Kind] != 0 && op.Ordinal != lastOrdinal[op.Kind]+1 {
				t.Fatalf("trial %d %s ordinal=%d after %d", trial, op.Kind, op.Ordinal, lastOrdinal[op.Kind])
			}
			if got[op.Kind] == 0 && op.Ordinal != 0 {
				t.Fatalf("trial %d %s starts at ordinal %d", trial, op.Kind, op.Ordinal)
			}
			lastRank, lastOrdinal[op.Kind] = rank, op.Ordinal
			spec := specForKind(op.Kind)
			stages := spec.Stages
			if stages == 0 {
				stages = barriers(spec.Barrier)
			}
			if op.Phase != spec.Phase || op.Barrier != spec.Barrier || op.Stages != stages || op.Role != spec.Role || op.Owner != spec.Owner {
				t.Fatalf("trial %d %s cursor metadata differs from registry", trial, op.Kind)
			}
			got[op.Kind]++
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("trial %d counts=%v, want %v", trial, got, want)
		}
	}
}

func TestCursorUsesCanonicalBarriersNotFactsDeclarationOrder(t *testing.T) {
	const point cfg.Point = 13
	input, _ := randomWholePoint(rand.New(rand.NewSource(99)), point)
	// Ensure the test covers both sides of every otherwise-empty barrier.
	input.CallSites = map[cfg.Point]factflow.CallSite{point: {}}
	input.NoNormalReturns = map[cfg.Point]struct{}{point: {}}
	input.PathValuePresenceImplications = map[cfg.Point]factflow.PathValuePresenceImplicationSet{point: factflow.NewPathValuePresenceImplicationSet(factflow.PathValuePresenceImplication{})}
	input.RootAssignments = map[cfg.Point]factflow.RootAssignment{point: {}}
	input.Returns = map[cfg.Point]factflow.Return{point: factflow.NewReturn(nil)}
	input.CovariantExposures = map[cfg.Point][]factflow.CovariantExposure{point: {{}}}
	input.BranchEdgeReachability = map[cfg.Point]factflow.BranchEdgeReachability{point: factflow.NewBranchEdgeReachability(false, false)}
	input.BranchRefinements = map[cfg.Point]factflow.BranchRefinementSet{point: factflow.NewBranchRefinementSet(factflow.BranchRefinement{})}
	input.BranchPresenceRelations = map[cfg.Point]factflow.BranchPresenceRelationSet{point: factflow.NewBranchPresenceRelationSet(factflow.BranchPresenceRelation{})}
	input.BranchPathEvidence = map[cfg.Point]factflow.BranchPathEvidenceSet{point: factflow.NewBranchPathEvidenceSet(factflow.BranchPathEvidence{})}
	plan, err := Compile(input, Config{})
	if err != nil {
		t.Fatal(err)
	}
	cursor := plan.Cursor(point)
	last := -1
	seen := 0
	for {
		op, ok := cursor.Next()
		if !ok {
			break
		}
		rank := barrierRank(op.Barrier)
		if rank < last {
			t.Fatalf("cursor moved backward from barrier rank %d to %d at %s", last, rank, op.Kind)
		}
		last = rank
		seen++
	}
	if seen != len(plan.Row(point)) {
		t.Fatalf("cursor emitted %d operations, row contains %d", seen, len(plan.Row(point)))
	}
}

func TestRandomWholePointExecutionMatchesCurrentTransfers(t *testing.T) {
	rng := rand.New(rand.NewSource(0xc0ffee))
	reg := standard.Registry()
	domain := state.Domain(reg)
	const point cfg.Point = 3
	for trial := 0; trial < 300; trial++ {
		unreachableTrue := rng.Intn(2) == 0
		unreachableFalse := rng.Intn(2) == 0
		input := factflow.FactsInput{
			BranchEdgeReachability: map[cfg.Point]factflow.BranchEdgeReachability{
				point: factflow.NewBranchEdgeReachability(unreachableTrue, unreachableFalse),
			},
		}
		if rng.Intn(3) == 0 {
			input.NoNormalReturns = map[cfg.Point]struct{}{point: {}}
		}
		facts := factflow.NewFacts(input)
		nodeConfig := factapply.FactsNodeTransferConfig{Facts: facts}
		edgeConfig := factapply.FactsEdgeTransferConfig{Facts: facts}
		plan, err := Compile(input, Config{Node: nodeConfig, Edge: edgeConfig})
		if err != nil {
			t.Fatal(err)
		}
		in := domain.Top()
		nodeCtx := transfer.NodeContext{Registry: reg, Point: point, Read: func(cfg.Point) state.State { return in }}
		wantNode := factapply.NewFactsNodeTransfer(nodeConfig)(nodeCtx, in)
		gotNode := plan.ExecuteNode(nodeCtx, in)
		if !domain.Equal(gotNode, wantNode) {
			t.Fatalf("trial %d node state differs", trial)
		}

		cond := rng.Intn(2) == 0
		edgeCtx := transfer.EdgeContext{
			Registry: reg, HasCond: true,
			Edge: cfg.Edge{From: point, To: point + 1, Cond: cond},
			Read: func(cfg.Point) state.State { return gotNode },
		}
		wantEdge := factapply.NewFactsEdgeTransfer(edgeConfig)(edgeCtx, gotNode)
		gotEdge := plan.ExecuteEdge(edgeCtx, gotNode)
		if !domain.Equal(gotEdge, wantEdge) {
			t.Fatalf("trial %d edge state differs", trial)
		}
	}
}

func TestUnsupportedGenericTransferFailsClosedAtomically(t *testing.T) {
	const point cfg.Point = 9
	input := factflow.FactsInput{NoNormalReturns: map[cfg.Point]struct{}{point: {}}}
	plan, err := Compile(input, Config{Extensions: []Extension{{Point: point, Phase: Node, Name: "future-actor-lane"}}})
	if err == nil {
		t.Fatal("Compile unexpectedly accepted an extension without a handler")
	}
	if row := plan.Row(point); row != nil {
		t.Fatalf("partial row escaped failed compile: %v", row)
	}
	reg := standard.Registry()
	in := state.Domain(reg).Top()
	ctx := transfer.NodeContext{Registry: reg, Point: point}
	if got := plan.ExecuteNode(ctx, in); !state.Domain(reg).Equal(got, in) {
		t.Fatal("zero plan mutated state after atomic compile failure")
	}
}

func TestRowIsImmutableToCaller(t *testing.T) {
	const point cfg.Point = 1
	plan, err := Compile(factflow.FactsInput{NoNormalReturns: map[cfg.Point]struct{}{point: {}}}, Config{})
	if err != nil {
		t.Fatal(err)
	}
	row := plan.Row(point)
	row[0].Kind = "corrupt"
	if got := plan.Row(point)[0].Kind; got != OpNoNormalReturn {
		t.Fatalf("plan row mutated to %q", got)
	}
}

func randomWholePoint(rng *rand.Rand, point cfg.Point) (factflow.FactsInput, map[Kind]int) {
	input := factflow.FactsInput{}
	want := make(map[Kind]int)
	maybe := func(kind Kind, install func()) {
		if rng.Intn(2) == 0 {
			install()
			want[kind] = 1
		}
	}
	count := func(kind Kind) int {
		n := rng.Intn(4)
		if n != 0 {
			want[kind] = n
		}
		return n
	}

	maybe(OpCallSite, func() { input.CallSites = map[cfg.Point]factflow.CallSite{point: {}} })
	maybe(OpNoNormalReturn, func() { input.NoNormalReturns = map[cfg.Point]struct{}{point: {}} })
	n := count(OpPathPresenceImplication)
	if n != 0 {
		input.PathValuePresenceImplications = map[cfg.Point]factflow.PathValuePresenceImplicationSet{point: factflow.NewPathValuePresenceImplicationSet(make([]factflow.PathValuePresenceImplication, n)...)}
	}
	maybe(OpPathDescendantInvalidation, func() {
		input.PathDescendantInvalidations = map[cfg.Point]factflow.PathDescendantInvalidation{point: {}}
	})
	n = count(OpPostconditionRefinement)
	if n != 0 {
		input.PostconditionRefinements = map[cfg.Point]factflow.PostconditionRefinementSet{point: factflow.NewPostconditionRefinementSet(make([]factflow.PostconditionRefinement, n)...)}
	}
	n = count(OpPostconditionPathRelation)
	if n != 0 {
		input.PostconditionPathRelations = map[cfg.Point][]factflow.PostconditionPathRelation{point: make([]factflow.PostconditionPathRelation, n)}
	}
	n = count(OpChannelSelect)
	if n != 0 {
		input.ChannelSelects = map[cfg.Point]factflow.ChannelSelectSet{point: factflow.NewChannelSelectSet(make([]factflow.ChannelSelect, n)...)}
	}
	maybe(OpDynamicIndexWrite, func() { input.DynamicIndexWrites = map[cfg.Point]factflow.DynamicIndexWrite{point: {}} })
	maybe(OpRootAssignment, func() { input.RootAssignments = map[cfg.Point]factflow.RootAssignment{point: {}} })
	maybe(OpPathAssignment, func() { input.PathAssignments = map[cfg.Point]factflow.PathAssignment{point: {}} })
	maybe(OpPathStaticMemberWrite, func() { input.PathStaticMemberWrites = map[cfg.Point]factflow.PathStaticMemberWrite{point: {}} })
	maybe(OpReturn, func() { input.Returns = map[cfg.Point]factflow.Return{point: factflow.NewReturn(nil)} })
	n = count(OpCovariantExposure)
	if n != 0 {
		input.CovariantExposures = map[cfg.Point][]factflow.CovariantExposure{point: make([]factflow.CovariantExposure, n)}
	}
	n = count(OpCallResultValue)
	if n != 0 {
		input.CallResultValues = map[cfg.Point]factflow.CallResultValueSet{point: factflow.NewCallResultValueSet(make([]factflow.CallResultValue, n)...)}
	}
	n = count(OpReturnPresenceRelation)
	if n != 0 {
		input.ReturnPresenceRelations = map[cfg.Point]factflow.ReturnPresenceRelationSet{point: factflow.NewReturnPresenceRelationSet(make([]factflow.ReturnPresenceRelation, n)...)}
	}
	maybe(OpBranchReachability, func() {
		input.BranchEdgeReachability = map[cfg.Point]factflow.BranchEdgeReachability{point: factflow.NewBranchEdgeReachability(false, false)}
	})
	maybe(OpBranchConditionSource, func() {
		condition, _ := factflow.NewBranchCondition(factflow.NewNilValueSource(0), true)
		input.BranchConditionSources = map[cfg.Point]factflow.BranchCondition{point: condition}
	})
	refN, lenN, floorN, ceilN, diffN := count(OpBranchRefinement), count(OpBranchLengthRefinement), count(OpBranchNumberFloor), count(OpBranchNumberCeil), count(OpBranchDifferenceConstraint)
	if refN+lenN+floorN+ceilN+diffN != 0 {
		set := factflow.NewBranchRefinementSet(make([]factflow.BranchRefinement, refN)...)
		set = set.WithLenRefinements(make([]factflow.BranchLenRefinement, lenN)...)
		set = set.WithNumFloorRefinements(make([]factflow.BranchNumFloorRefinement, floorN)...)
		set = set.WithNumCeilRefinements(make([]factflow.BranchNumCeilRefinement, ceilN)...)
		set = set.WithDiffConstraints(make([]factflow.BranchDiffConstraint, diffN)...)
		input.BranchRefinements = map[cfg.Point]factflow.BranchRefinementSet{point: set}
	}
	n = count(OpBranchPresenceRelation)
	if n != 0 {
		input.BranchPresenceRelations = map[cfg.Point]factflow.BranchPresenceRelationSet{point: factflow.NewBranchPresenceRelationSet(make([]factflow.BranchPresenceRelation, n)...)}
	}
	n = count(OpBranchPathRelation)
	if n != 0 {
		input.BranchPathRelations = map[cfg.Point]factflow.BranchPathRelationSet{point: factflow.NewBranchPathRelationSet(make([]factflow.BranchPathRelation, n)...)}
	}
	n = count(OpBranchPathEvidence)
	if n != 0 {
		input.BranchPathEvidence = map[cfg.Point]factflow.BranchPathEvidenceSet{point: factflow.NewBranchPathEvidenceSet(make([]factflow.BranchPathEvidence, n)...)}
	}
	n = count(OpBranchSufficientLiteralCase)
	if n != 0 {
		input.BranchSufficientLiteralCases = map[cfg.Point]factflow.BranchSufficientLiteralCaseSet{point: factflow.NewBranchSufficientLiteralCaseSet(make([]factflow.BranchSufficientLiteralCase, n)...)}
	}
	return input, want
}

func operationRank(kind Kind) int {
	spec := specForKind(kind)
	return barrierRank(spec.Barrier)*len(operationSpecs) + kindRank(kind)
}
