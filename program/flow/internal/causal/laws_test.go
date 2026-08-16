package causal

import (
	"fmt"
	"sort"
	"testing"

	"github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
)

func TestOutcomePhaseClassifierExcludesOnlyNormal(t *testing.T) {
	if outcomePhaseOutcome(kind.OutcomeNormal) {
		t.Fatal("Normal selected an Outcome phase")
	}
	for _, outcomeKind := range []kind.OutcomeKind{kind.OutcomeReturn, kind.OutcomeThrow, kind.OutcomeYield, kind.OutcomeCancel, kind.OutcomeBreak, kind.OutcomeGoto} {
		if !outcomePhaseOutcome(outcomeKind) {
			t.Fatalf("%v did not select an Outcome phase", outcomeKind)
		}
	}
}

// syntheticResult is deliberately assembled from the published typed rows.
// It keeps these laws independent of the upstream fixture builders while
// exercising the final authority's closed storage and query contracts.
func syntheticResult() *Result {
	var counts [keyspace.FamilyCount]uint32
	counts[keyspace.FamilyBody] = 1
	counts[keyspace.FamilyCall] = 3
	counts[keyspace.FamilyReturn] = 1
	counts[keyspace.FamilyBreak] = 1
	counts[keyspace.FamilyGoto] = 1
	counts[keyspace.FamilyLabel] = 1
	counts[keyspace.FamilySelect] = 1
	counts[keyspace.FamilyBranch] = 1
	counts[keyspace.FamilyLoop] = 1
	counts[keyspace.FamilyTableField] = 2
	counts[keyspace.FamilyOutcome] = 4
	r := &Result{
		index:    successorIndex{familyCounts: counts},
		sourceID: keyspace.ContentID{1},
		flowID:   keyspace.ContentID{2},
		staticID: keyspace.ContentID{3},
		moduleID: keyspace.ContentID{4},
	}
	return r
}

func rebuildSyntheticSuccessors(r *Result, edges []edgeRow, boundaries []boundaryRow) error {
	r.edges.rows = edges
	r.boundaries.rows = boundaries
	// Synthetic rows are a closed test fixture, not a production certificate
	// path. Give their fixture authority the exact heads it deliberately uses.
	componentSet := make(map[keyspace.Term]struct{})
	for _, row := range edges {
		if row.component != 0 {
			componentSet[row.component] = struct{}{}
		}
	}
	for _, row := range boundaries {
		for _, arm := range [...]BoundaryArmKind{BoundaryResume, BoundarySelectTrue, BoundarySelectFalse, BoundaryTail, BoundaryThrow, BoundaryYield, BoundaryCancel} {
			if boundaryArmPresent(row, arm) && row.components[arm] != 0 {
				componentSet[row.components[arm]] = struct{}{}
			}
		}
	}
	r.components = make([]keyspace.Term, 0, len(componentSet))
	for component := range componentSet {
		r.components = append(r.components, component)
	}
	sort.Slice(r.components, func(left, right int) bool { return r.components[left] < r.components[right] })
	r.componentIndex = make(map[keyspace.Term]uint32, len(r.components))
	for index, component := range r.components {
		if !canonicalComponent(component, true) || uint64(index) > uint64(^uint32(0)) {
			return fmt.Errorf("synthetic component %v is unavailable", component)
		}
		if _, duplicate := r.componentIndex[component]; duplicate {
			return fmt.Errorf("synthetic component %v is duplicated", component)
		}
		r.componentIndex[component] = uint32(index)
	}
	r.boundaries.callSlots = make([]uint32, r.index.familyCounts[keyspace.FamilyCall]+1)
	edgeOwners := make([]keyspace.Term, len(edges))
	for index := range edges {
		edgeOwners[index] = keyspace.MakeTerm(keyspace.FamilyBody, 1)
	}
	boundaryOwners := make([]keyspace.Term, len(boundaries))
	for index := range boundaries {
		boundaryOwners[index] = keyspace.MakeTerm(keyspace.FamilyBody, 1)
		ordinal := keyspace.TermOrdinal(boundaries[index].Call)
		r.boundaries.callSlots[ordinal] = uint32(index + 1)
	}
	proof := &proofState{counts: r.index.familyCounts}
	rows := &resultScratch{result: r}
	index := &indexState{
		proofState:          proof,
		edgeRowsScratch:     &edgeRowsScratch{edgeRows: edges, edgeOwners: edgeOwners},
		boundaryRowsScratch: &boundaryRowsScratch{boundaryRows: boundaries, boundaryOwners: boundaryOwners},
		resultScratch:       rows,
	}
	return index.buildSuccessorIndex()
}

func TestSemanticWTOSitesArePathOrderedPerVertexWithoutCrossVertexCollapse(t *testing.T) {
	r := syntheticResult()
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	call := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	pathBody := keyspace.ContentID{1}
	pathCall := keyspace.ContentID{2}
	r.sites.rows = []siteRow{{term: body, path: pathBody}, {term: call, path: pathCall}}
	// The same issued Site is a legitimate projection of two graph vertices;
	// only the duplicate inside vertex zero is removed.
	r.pendingNodeSites = [][]uint32{{1, 0, 1}, {1}}

	first, firstID, ok := r.semanticWTOSites(0)
	if !ok || len(first) != 2 || first[0] != 0 || first[1] != 1 || !firstID.Available() {
		t.Fatalf("first semantic WTO site set = (%v, %v, %t)", first, firstID, ok)
	}
	second, secondID, ok := r.semanticWTOSites(1)
	if !ok || len(second) != 1 || second[0] != 1 || !secondID.Available() {
		t.Fatalf("second semantic WTO site set = (%v, %v, %t)", second, secondID, ok)
	}
}

func TestClassifyWTORoutesUsesLowestCommonRegionAndExplicitCrossDisposition(t *testing.T) {
	r := syntheticResult()
	rootID, childID, otherID := keyspace.ContentID{11}, keyspace.ContentID{12}, keyspace.ContentID{13}
	firstDigest, secondDigest := keyspace.ContentID{21}, keyspace.ContentID{22}
	firstPath, secondPath := keyspace.ContentID{31}, keyspace.ContentID{32}
	first := successorRef{local: true, arm: BoundaryLocal, routeDigest: firstDigest, semanticPath: firstPath}
	second := successorRef{local: true, arm: BoundaryLocal, routeDigest: secondDigest, semanticPath: secondPath}
	r.index.refs = []successorRef{first, second}
	r.routeIndex = []routeLookup{{digest: firstDigest, ref: first}, {digest: secondDigest, ref: second}}
	r.pendingNodeSites = make([][]uint32, 3)
	r.pendingWTORoutes = []pendingWTORoute{{from: 0, to: 1}, {from: 1, to: 2}}
	store := wtoStore{
		regions: []wtoRegion{{id: rootID}, {id: childID, parent: rootID}, {id: otherID}},
		byID:    map[keyspace.ContentID]uint32{rootID: 0, childID: 1, otherID: 2},
	}
	if err := r.classifyWTORoutes(&store, []uint32{1, 0, 2}); err != nil {
		t.Fatalf("classifyWTORoutes rejected valid nested/cross membership: %v", err)
	}
	if r.index.refs[0].wtoRegion != rootID || r.routeIndex[0].ref.wtoRegion != rootID || len(store.regions[0].routes) != 1 {
		t.Fatal("nested route was not assigned to its lowest common parent region")
	}
	if r.index.refs[1].wtoRegion.Available() || r.routeIndex[1].ref.wtoRegion.Available() || len(store.regions[2].routes) != 0 {
		t.Fatal("cross-root route was not retained as an explicit zero membership")
	}
}

func TestClosedBoundaryArmsAndTwoPlanePartition(t *testing.T) {
	r := syntheticResult()
	r.reset.headRanges[keyspace.FamilyLoop] = make([]range32, 2)
	r.reset.headRanges[keyspace.FamilyLoop][1] = range32{}
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	call1 := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	call2 := keyspace.MakeTerm(keyspace.FamilyCall, 2)
	call3 := keyspace.MakeTerm(keyspace.FamilyCall, 3)
	selectTerm := keyspace.MakeTerm(keyspace.FamilySelect, 1)
	branch := keyspace.MakeTerm(keyspace.FamilyBranch, 1)
	loop := keyspace.MakeTerm(keyspace.FamilyLoop, 1)
	outcome1 := keyspace.MakeTerm(keyspace.FamilyOutcome, 1)
	outcome2 := keyspace.MakeTerm(keyspace.FamilyOutcome, 2)
	outcome3 := keyspace.MakeTerm(keyspace.FamilyOutcome, 3)
	outcome4 := keyspace.MakeTerm(keyspace.FamilyOutcome, 4)
	edges := []edgeRow{
		{Edge: Edge{From: body, To: selectTerm, Decision: branch, Truth: true, Mu: loop}, component: loop},
	}
	boundaries := []boundaryRow{
		{CallBoundary: CallBoundary{Call: call1, Normal: body, Throw: outcome1, Yield: outcome2, Cancel: outcome3, mode: boundaryDirect}},
		{CallBoundary: CallBoundary{Call: call2, Normal: selectTerm, Other: body, Throw: outcome1, Yield: outcome2, Cancel: outcome3, mode: boundarySelectAnd}},
		{CallBoundary: CallBoundary{Call: call3, TailReturn: outcome4, Throw: outcome1, Yield: outcome2, Cancel: outcome3, mode: boundaryTail}},
	}
	if err := rebuildSyntheticSuccessors(r, edges, boundaries); err != nil {
		t.Fatal(err)
	}

	if got := r.Edges().Count(); got != 1 {
		t.Fatalf("local Edge count = %d, want 1", got)
	}
	if got := r.Boundaries().Count(); got != 3 {
		t.Fatalf("CallBoundary count = %d, want 3", got)
	}
	if got := r.Successors().Count(call1); got != 4 {
		t.Fatalf("direct boundary arm count = %d, want 4", got)
	}
	if got := r.Successors().Count(call2); got != 5 {
		t.Fatalf("Select-left boundary arm count = %d, want 5", got)
	}
	if got := r.Successors().Count(call3); got != 4 {
		t.Fatalf("tail boundary arm count = %d, want 4", got)
	}
	if got := r.Successors().Count(body); got != 1 {
		t.Fatalf("local arm count = %d, want 1", got)
	}

	selectTrue, ok := r.Successors().At(call2, 0)
	if !ok || selectTrue.Arm != BoundarySelectTrue || selectTrue.To != body ||
		selectTrue.Decision != selectTerm || !selectTrue.Truth {
		t.Fatalf("Select true arm = %+v, want guarded Other route", selectTrue)
	}
	selectFalse, ok := r.Successors().At(call2, 1)
	if !ok || selectFalse.Arm != BoundarySelectFalse || selectFalse.To != selectTerm ||
		selectFalse.Decision != selectTerm || selectFalse.Truth {
		t.Fatalf("Select false arm = %+v, want guarded Select route", selectFalse)
	}
	tail, ok := r.Successors().At(call3, 0)
	if !ok || tail.Arm != BoundaryTail || tail.To != outcome4 {
		t.Fatalf("tail arm = %+v, want terminal Return Outcome", tail)
	}
	if _, ok := r.Successors().At(call1, 0); !ok {
		t.Fatal("direct boundary lost its normal arm")
	}
	selectOr := CallBoundary{Normal: selectTerm, Other: body, mode: boundarySelectOr}
	if to, decision, truth, ok := boundarySuccessor(selectOr, BoundarySelectTrue); !ok || to != selectTerm || decision != selectTerm || !truth {
		t.Fatalf("Select-or true arm = %v,%v,%v,%v, want Select/Select/true", to, decision, truth, ok)
	}
	if to, decision, truth, ok := boundarySuccessor(selectOr, BoundarySelectFalse); !ok || to != body || decision != selectTerm || truth {
		t.Fatalf("Select-or false arm = %v,%v,%v,%v, want Other/Select/false", to, decision, truth, ok)
	}
	for index := 0; index < r.Edges().Count(); index++ {
		edge, ok := r.Edges().At(index)
		if !ok || keyspace.TermFamily(edge.From) == keyspace.FamilyCall {
			t.Fatalf("local plane contains Call-origin row: %+v", edge)
		}
	}
}

func TestClosedBoundarySelectOrGeneratedArms(t *testing.T) {
	r := syntheticResult()
	call := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	selectTerm := keyspace.MakeTerm(keyspace.FamilySelect, 1)
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	outcome1 := keyspace.MakeTerm(keyspace.FamilyOutcome, 1)
	outcome2 := keyspace.MakeTerm(keyspace.FamilyOutcome, 2)
	outcome3 := keyspace.MakeTerm(keyspace.FamilyOutcome, 3)
	boundary := boundaryRow{CallBoundary: CallBoundary{
		Call: call, Normal: selectTerm, Other: body,
		Throw: outcome1, Yield: outcome2, Cancel: outcome3, mode: boundarySelectOr,
	}}
	if err := rebuildSyntheticSuccessors(r, nil, []boundaryRow{boundary}); err != nil {
		t.Fatal(err)
	}
	if got := r.Successors().Count(call); got != 5 {
		t.Fatalf("Select-or boundary denominator = %d, want 5", got)
	}
	trueArm, ok := r.Successors().At(call, 0)
	if !ok || trueArm.Arm != BoundarySelectTrue || trueArm.To != selectTerm || trueArm.Decision != selectTerm || !trueArm.Truth {
		t.Fatalf("Select-or true arm = %#v/%v", trueArm, ok)
	}
	falseArm, ok := r.Successors().At(call, 1)
	if !ok || falseArm.Arm != BoundarySelectFalse || falseArm.To != body || falseArm.Decision != selectTerm || falseArm.Truth {
		t.Fatalf("Select-or false arm = %#v/%v", falseArm, ok)
	}
}

func TestStructuralGuardsAndOutcomeRoutesStayTyped(t *testing.T) {
	r := syntheticResult()
	r.reset.headRanges[keyspace.FamilyLoop] = make([]range32, 2)
	r.reset.headRanges[keyspace.FamilyLoop][1] = range32{}
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	branch := keyspace.MakeTerm(keyspace.FamilyBranch, 1)
	loop := keyspace.MakeTerm(keyspace.FamilyLoop, 1)
	breakTerm := keyspace.MakeTerm(keyspace.FamilyBreak, 1)
	gotoTerm := keyspace.MakeTerm(keyspace.FamilyGoto, 1)
	label := keyspace.MakeTerm(keyspace.FamilyLabel, 1)
	returnTerm := keyspace.MakeTerm(keyspace.FamilyReturn, 1)
	fieldKey := keyspace.MakeTerm(keyspace.FamilyTableField, 1)
	fieldExact := keyspace.MakeTerm(keyspace.FamilyTableField, 2)
	outcome1 := keyspace.MakeTerm(keyspace.FamilyOutcome, 1)
	outcome2 := keyspace.MakeTerm(keyspace.FamilyOutcome, 2)
	outcome3 := keyspace.MakeTerm(keyspace.FamilyOutcome, 3)
	outcome4 := keyspace.MakeTerm(keyspace.FamilyOutcome, 4)
	edges := []edgeRow{
		{Edge: Edge{From: branch, To: body, Decision: branch, Truth: true}},
		{Edge: Edge{From: branch, To: outcome1, Decision: branch, Truth: false}},
		{Edge: Edge{From: loop, To: body, Decision: loop, Truth: true, Mu: loop}, component: loop},
		{Edge: Edge{From: loop, To: outcome2, Decision: loop, Truth: false}},
		{Edge: Edge{From: breakTerm, To: outcome3, Mu: loop}, component: loop},
		{Edge: Edge{From: gotoTerm, To: label}},
		{Edge: Edge{From: returnTerm, To: outcome4}},
		{Edge: Edge{From: fieldKey, To: outcome1}},
		{Edge: Edge{From: fieldExact, To: outcome2}},
	}
	if err := rebuildSyntheticSuccessors(r, edges, nil); err != nil {
		t.Fatal(err)
	}
	for _, term := range []keyspace.Term{branch, loop} {
		if got := r.Successors().Count(term); got != 2 {
			t.Fatalf("guarded %v successor count = %d, want 2", term, got)
		}
	}
	for _, term := range []keyspace.Term{breakTerm, gotoTerm, returnTerm, fieldKey, fieldExact} {
		if got := r.Successors().Count(term); got != 1 {
			t.Fatalf("typed route %v successor count = %d, want 1", term, got)
		}
	}
	for index, want := range []struct {
		decision keyspace.Term
		truth    bool
	}{
		{branch, true},
		{branch, false},
		{loop, true},
		{loop, false},
	} {
		got, truth, ok := r.Edges().Decision(index)
		edge, edgeOK := r.Edges().At(index)
		if !ok || !edgeOK || got != want.decision || truth != want.truth || edge.Truth != want.truth {
			t.Fatalf("guarded Edge[%d] = %v,%v,%v, want %v,%v", index, got, truth, ok, want.decision, want.truth)
		}
	}
	if mu, ok := r.Edges().Mu(4); !ok || mu != loop {
		t.Fatalf("Break route Mu = %v,%v, want Loop,true", mu, ok)
	}
}

func TestWideFamilyBaseLookupIsIterativeAndAllocationFree(t *testing.T) {
	var counts [keyspace.FamilyCount]uint32
	counts[keyspace.FamilyBody] = 4096
	counts[keyspace.FamilyOutcome] = 1
	r := &Result{
		index:    successorIndex{familyCounts: counts},
		sourceID: keyspace.ContentID{1},
		flowID:   keyspace.ContentID{2},
		staticID: keyspace.ContentID{3},
		moduleID: keyspace.ContentID{4},
	}
	high := keyspace.MakeTerm(keyspace.FamilyBody, 4096)
	outcome := keyspace.MakeTerm(keyspace.FamilyOutcome, 1)
	if err := rebuildSyntheticSuccessors(r, []edgeRow{{Edge: Edge{From: high, To: outcome}}}, nil); err != nil {
		t.Fatal(err)
	}
	if got := r.Successors().Count(high); got != 1 {
		t.Fatalf("wide family successor count = %d, want 1", got)
	}
	view := r.Successors()
	if allocs := testing.AllocsPerRun(1000, func() { _, _ = view.At(high, 0) }); allocs != 0 {
		t.Fatalf("wide successor query allocates %v times", allocs)
	}
}

func TestCompressedMuQueriesAndEmptyReset(t *testing.T) {
	r := syntheticResult()
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	loop := keyspace.MakeTerm(keyspace.FamilyLoop, 1)
	selectTerm := keyspace.MakeTerm(keyspace.FamilySelect, 1)
	branch := keyspace.MakeTerm(keyspace.FamilyBranch, 1)
	r.reset.streams = []keyspace.Term{selectTerm, branch}
	r.reset.headRanges[keyspace.FamilyLoop] = make([]range32, 2)
	r.reset.headRanges[keyspace.FamilyLoop][1] = range32{start: 0, end: 2}
	r.reset.decisionHead[keyspace.FamilySelect] = make([]keyspace.Term, 2)
	r.reset.decisionRank[keyspace.FamilySelect] = make([]uint32, 2)
	r.reset.decisionHead[keyspace.FamilySelect][1] = loop
	r.reset.decisionRank[keyspace.FamilySelect][1] = 0
	r.reset.decisionHead[keyspace.FamilyBranch] = make([]keyspace.Term, 2)
	r.reset.decisionRank[keyspace.FamilyBranch] = make([]uint32, 2)
	r.reset.decisionHead[keyspace.FamilyBranch][1] = loop
	r.reset.decisionRank[keyspace.FamilyBranch][1] = 1
	r.edges.rows = []edgeRow{
		{Edge: Edge{From: body, To: selectTerm, Mu: loop}, component: loop, resetStart: 0, resetPast: 2},
		{Edge: Edge{From: body, To: branch, Mu: loop}, component: loop, resetStart: 2, resetPast: 2},
	}
	r.components = []keyspace.Term{loop}

	view := r.Edges()
	if count, ok := view.ResetCount(0); !ok || count != 2 {
		t.Fatalf("reset count = %d,%v, want 2,true", count, ok)
	}
	if got, ok := view.ResetAt(0, 1); !ok || got != branch {
		t.Fatalf("reset At = %v,%v, want Branch", got, ok)
	}
	if !view.ResetContains(0, selectTerm) || !view.ResetContains(0, branch) {
		t.Fatal("compressed reset membership lost a stream member")
	}
	r.reset.decisionRank[keyspace.FamilySelect] = nil
	if view.ResetContains(0, selectTerm) {
		t.Fatal("truncated decision-rank plane exposed reset membership")
	}
	if count, ok := view.ResetCount(1); !ok || count != 0 {
		t.Fatalf("empty reset count = %d,%v, want 0,true", count, ok)
	}
	if view.ResetContains(1, selectTerm) {
		t.Fatal("empty reset falsely contains a decision")
	}
}

func TestComponentShapedButUnissuedRowFailsClosed(t *testing.T) {
	r := syntheticResult()
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	loop := keyspace.MakeTerm(keyspace.FamilyLoop, 1)
	selectTerm := keyspace.MakeTerm(keyspace.FamilySelect, 1)
	if err := rebuildSyntheticSuccessors(r, []edgeRow{{Edge: Edge{From: body, To: selectTerm}, component: loop}}, nil); err != nil {
		t.Fatal(err)
	}
	if _, ok := r.Edges().At(0); !ok {
		t.Fatal("fixture-issued component row disappeared")
	}
	r.components = nil
	if _, ok := r.Edges().At(0); ok {
		t.Fatal("component-shaped but unissued local row remained observable")
	}
}

func TestLocalProjectionUsesOnlyIssuedCausalRows(t *testing.T) {
	build := func(t *testing.T) (*Result, Successor, Site, Region) {
		t.Helper()
		r := syntheticResult()
		body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
		loop := keyspace.MakeTerm(keyspace.FamilyLoop, 1)
		selectTerm := keyspace.MakeTerm(keyspace.FamilySelect, 1)
		if err := rebuildSyntheticSuccessors(r, []edgeRow{{Edge: Edge{From: body, To: selectTerm}, component: loop}}, nil); err != nil {
			t.Fatal(err)
		}
		r.sites.rows = []siteRow{
			{term: body, context: hashSiteContext(r.sourceID, r.flowID, r.staticID, r.moduleID, body)},
			{term: selectTerm, context: hashSiteContext(r.sourceID, r.flowID, r.staticID, r.moduleID, selectTerm)},
		}
		if !r.buildLocal() {
			t.Fatal("failed to project issued local component")
		}
		successor, ok := r.Successors().At(body, 0)
		if !ok {
			t.Fatal("issued local successor disappeared")
		}
		site, ok := r.SiteForTerm(body)
		if !ok {
			t.Fatal("issued local site disappeared")
		}
		region, ok := r.Local().ForSuccessor(successor)
		if !ok || !region.Available() || !region.ContainsSuccessor(successor) || !region.ContainsSite(site) {
			t.Fatal("region lost its existing causal route/site references")
		}
		if head, ok := region.Head(); !ok || head != loop {
			t.Fatalf("region head = %v/%v, want %v/true", head, ok, loop)
		}
		if region.SuccessorCount() != 1 || region.SiteCount() != 2 {
			t.Fatalf("region traversal counts = routes %d sites %d, want 1/2", region.SuccessorCount(), region.SiteCount())
		}
		if route, ok := region.SuccessorAt(0); !ok || !route.IsLocal() || route.From != body || route.To != selectTerm {
			t.Fatalf("region route traversal lost existing successor: %#v/%v", route, ok)
		}
		if endpoint, ok := region.SiteAt(0); !ok || !endpoint.Available() {
			t.Fatal("region site traversal lost existing site")
		}
		return r, successor, site, region
	}
	_, successor, site, first := build(t)
	_, foreignSuccessor, foreignSite, second := build(t)
	if first.ID() != second.ID() {
		t.Fatal("equivalent issued component changed stable local identity")
	}
	if _, ok := first.result.Local().ForSuccessor(foreignSuccessor); ok {
		t.Fatal("foreign successor entered local inverse")
	}
	if regions := first.result.Local().RegionCountForSite(foreignSite); regions != 0 {
		t.Fatal("foreign site entered local inverse")
	}
	if !first.ContainsSuccessor(successor) || !first.ContainsSite(site) {
		t.Fatal("local membership was not stable")
	}
}

func TestLocalEnumeratesEmptyIssuedHeadInCanonicalOrder(t *testing.T) {
	r := syntheticResult()
	loop := keyspace.MakeTerm(keyspace.FamilyLoop, 1)
	r.components = []keyspace.Term{loop}
	if !r.buildLocal() {
		t.Fatal("failed to build empty issued region")
	}
	local := r.Local()
	if local.Count() != 1 {
		t.Fatalf("empty issued region count = %d, want 1", local.Count())
	}
	region, ok := local.At(0)
	if !ok || region.SuccessorCount() != 0 || region.SiteCount() != 0 {
		t.Fatal("empty issued region was not enumerable")
	}
	if replay, ok := local.Resolve(region.ID()); !ok || replay.ID() != region.ID() {
		t.Fatal("canonical empty region identity failed to resolve")
	}
	if _, ok := local.At(1); ok {
		t.Fatal("region enumeration exposed a physical overflow row")
	}
	if allocs := testing.AllocsPerRun(1000, func() { _, _ = local.At(0); _, _ = local.Resolve(region.ID()) }); allocs != 0 {
		t.Fatalf("local enumeration allocates %v times", allocs)
	}
}

func TestLocalSiteMembershipIsMultiValued(t *testing.T) {
	r := syntheticResult()
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	loop := keyspace.MakeTerm(keyspace.FamilyLoop, 1)
	label := keyspace.MakeTerm(keyspace.FamilyLabel, 1)
	selectTerm := keyspace.MakeTerm(keyspace.FamilySelect, 1)
	branch := keyspace.MakeTerm(keyspace.FamilyBranch, 1)
	if err := rebuildSyntheticSuccessors(r, []edgeRow{
		{Edge: Edge{From: body, To: selectTerm}, component: loop},
		{Edge: Edge{From: body, To: branch}, component: label},
	}, nil); err != nil {
		t.Fatal(err)
	}
	r.sites.rows = []siteRow{
		{term: body, context: hashSiteContext(r.sourceID, r.flowID, r.staticID, r.moduleID, body)},
		{term: selectTerm, context: hashSiteContext(r.sourceID, r.flowID, r.staticID, r.moduleID, selectTerm)},
		{term: branch, context: hashSiteContext(r.sourceID, r.flowID, r.staticID, r.moduleID, branch)},
	}
	if !r.buildLocal() {
		t.Fatal("failed to build multi-region site inverse")
	}
	first, firstOK := r.Local().At(0)
	second, secondOK := r.Local().At(1)
	firstHead, firstHeadOK := first.Head()
	secondHead, secondHeadOK := second.Head()
	if !firstOK || !secondOK || !firstHeadOK || !secondHeadOK || firstHead != label || secondHead != loop {
		t.Fatalf("canonical Region order = %v/%v, want Label/Loop", firstHead, secondHead)
	}
	site, ok := r.SiteForTerm(body)
	if !ok {
		t.Fatal("shared site disappeared")
	}
	if regions := r.Local().RegionCountForSite(site); regions != 2 {
		t.Fatalf("shared Site region count = %d, want 2", regions)
	}
	for index := 0; index < r.Local().RegionCountForSite(site); index++ {
		if region, ok := r.Local().RegionAtSite(site, index); !ok || !region.ContainsSite(site) {
			t.Fatalf("site inverse %d lost exact region", index)
		}
	}
	if allocs := testing.AllocsPerRun(1000, func() { _, _ = r.Local().RegionAtSite(site, 0) }); allocs != 0 {
		t.Fatalf("site region inverse allocates %v times", allocs)
	}
}

func TestLocalSiteInverseDeduplicatesInterleavedRegionRoutes(t *testing.T) {
	r := syntheticResult()
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	loop := keyspace.MakeTerm(keyspace.FamilyLoop, 1)
	label := keyspace.MakeTerm(keyspace.FamilyLabel, 1)
	selectTerm := keyspace.MakeTerm(keyspace.FamilySelect, 1)
	branch := keyspace.MakeTerm(keyspace.FamilyBranch, 1)
	if err := rebuildSyntheticSuccessors(r, []edgeRow{
		{Edge: Edge{From: body, To: selectTerm}, component: loop},
		{Edge: Edge{From: body, To: branch}, component: label},
		{Edge: Edge{From: body, To: selectTerm}, component: loop},
	}, nil); err != nil {
		t.Fatal(err)
	}
	r.sites.rows = []siteRow{
		{term: body, context: hashSiteContext(r.sourceID, r.flowID, r.staticID, r.moduleID, body)},
		{term: selectTerm, context: hashSiteContext(r.sourceID, r.flowID, r.staticID, r.moduleID, selectTerm)},
		{term: branch, context: hashSiteContext(r.sourceID, r.flowID, r.staticID, r.moduleID, branch)},
	}
	if !r.buildLocal() {
		t.Fatal("failed to build interleaved region inverse")
	}
	site, ok := r.SiteForTerm(body)
	if !ok || r.Local().RegionCountForSite(site) != 2 {
		t.Fatal("interleaved A/B/A routes duplicated shared site membership")
	}
}

func TestLocalRejectsDuplicateSiteTerms(t *testing.T) {
	r := syntheticResult()
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	loop := keyspace.MakeTerm(keyspace.FamilyLoop, 1)
	selectTerm := keyspace.MakeTerm(keyspace.FamilySelect, 1)
	if err := rebuildSyntheticSuccessors(r, []edgeRow{{Edge: Edge{From: body, To: selectTerm}, component: loop}}, nil); err != nil {
		t.Fatal(err)
	}
	context := hashSiteContext(r.sourceID, r.flowID, r.staticID, r.moduleID, body)
	r.sites.rows = []siteRow{
		{term: body, context: context},
		{term: body, context: context},
		{term: selectTerm, context: hashSiteContext(r.sourceID, r.flowID, r.staticID, r.moduleID, selectTerm)},
	}
	if r.buildLocal() {
		t.Fatal("Local accepted duplicate canonical Site terms")
	}
}

func TestBoundaryArmRetainsExactMuResetWitness(t *testing.T) {
	r := syntheticResult()
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	call := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	loop := keyspace.MakeTerm(keyspace.FamilyLoop, 1)
	selectTerm := keyspace.MakeTerm(keyspace.FamilySelect, 1)
	throw := keyspace.MakeTerm(keyspace.FamilyOutcome, 1)
	yield := keyspace.MakeTerm(keyspace.FamilyOutcome, 2)
	cancel := keyspace.MakeTerm(keyspace.FamilyOutcome, 3)
	r.reset.streams = []keyspace.Term{selectTerm}
	r.reset.headRanges[keyspace.FamilyLoop] = make([]range32, 2)
	r.reset.headRanges[keyspace.FamilyLoop][1] = range32{start: 0, end: 1}
	r.reset.decisionHead[keyspace.FamilySelect] = make([]keyspace.Term, 2)
	r.reset.decisionRank[keyspace.FamilySelect] = make([]uint32, 2)
	r.reset.decisionHead[keyspace.FamilySelect][1] = loop
	boundary := boundaryRow{CallBoundary: CallBoundary{
		Call: call, Normal: body, Throw: throw, Yield: yield, Cancel: cancel, mode: boundaryDirect,
	}}
	boundary.components[BoundaryResume] = loop
	boundary.proofs[BoundaryResume] = boundaryRecurrenceProof{mu: loop, resetStart: 0, resetPast: 1}
	if err := rebuildSyntheticSuccessors(r, nil, []boundaryRow{boundary}); err != nil {
		t.Fatal(err)
	}
	row := r.boundaries.rows[0]
	if !r.validBoundaryProof(row, BoundaryResume) || row.components[BoundaryResume] != loop || row.proofs[BoundaryResume].mu != loop {
		t.Fatalf("pre-seal boundary recurrence proof = %#v", row)
	}
	if count, ok := r.boundaryResetCount(0, BoundaryResume); !ok || count != 1 {
		t.Fatalf("boundary reset count = %d/%v, want 1/true", count, ok)
	}
	if decision, ok := r.boundaryResetAt(0, BoundaryResume, 0); !ok || decision != selectTerm || !r.boundaryResetContains(0, BoundaryResume, selectTerm) {
		t.Fatalf("boundary reset witness = %v/%v", decision, ok)
	}
	r.reset.decisionRank[keyspace.FamilySelect] = nil
	if r.boundaryResetContains(0, BoundaryResume, selectTerm) {
		t.Fatal("truncated boundary decision-rank plane exposed reset membership")
	}
}

func TestProvenanceAndQueryPathsAreAllocationFree(t *testing.T) {
	r := syntheticResult()
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	if !Matches(r, keyspace.ContentID{1}, keyspace.ContentID{2}, keyspace.ContentID{3}, keyspace.ContentID{4}) {
		t.Fatal("matching Source/Flow/Static/Module identities rejected")
	}
	if Matches(r, keyspace.ContentID{5}, keyspace.ContentID{2}, keyspace.ContentID{3}, keyspace.ContentID{4}) ||
		Matches(r, keyspace.ContentID{1}, keyspace.ContentID{5}, keyspace.ContentID{3}, keyspace.ContentID{4}) ||
		Matches(r, keyspace.ContentID{1}, keyspace.ContentID{2}, keyspace.ContentID{5}, keyspace.ContentID{4}) ||
		Matches(r, keyspace.ContentID{1}, keyspace.ContentID{2}, keyspace.ContentID{3}, keyspace.ContentID{5}) {
		t.Fatal("foreign equal-shape identity accepted")
	}
	view := r.Successors()
	allocs := testing.AllocsPerRun(1000, func() {
		_ = view.Count(body)
		_, _ = view.At(body, 0)
	})
	if allocs != 0 {
		t.Fatalf("successor queries allocate %v times", allocs)
	}
}

func TestSemanticRouteIdentityResolvesThroughExistingRef(t *testing.T) {
	r := syntheticResult()
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	outcome := keyspace.MakeTerm(keyspace.FamilyOutcome, 1)
	if err := rebuildSyntheticSuccessors(r, []edgeRow{{Edge: Edge{From: body, To: outcome}}}, nil); err != nil {
		t.Fatal(err)
	}
	if err := r.buildRouteIndex(); err != nil {
		t.Fatal(err)
	}
	successor, ok := r.Successors().At(body, 0)
	if !ok {
		t.Fatal("sealed successor is unavailable")
	}
	identity, ok := successor.Identity()
	if !ok || !identity.available() {
		t.Fatal("sealed successor did not publish an owner-fenced identity")
	}
	resolved, ok := r.Successors().Resolve(identity)
	if !ok || resolved.From != successor.From || resolved.To != successor.To || resolved.Arm != successor.Arm {
		t.Fatalf("Resolve(identity) = %#v/%v, want %#v/true", resolved, ok, successor)
	}
	if allocs := testing.AllocsPerRun(1000, func() {
		_, _ = r.Successors().Resolve(identity)
	}); allocs != 0 {
		t.Fatalf("Resolve(identity) allocates %v times", allocs)
	}
	foreign := identity
	foreign.SourceID = keyspace.ContentID{9}
	if _, ok := r.Successors().Resolve(foreign); ok {
		t.Fatal("foreign owner identity resolved in this causal authority")
	}
}

func TestSemanticRouteIdentityRejectsDuplicatePreimage(t *testing.T) {
	r := syntheticResult()
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	outcome := keyspace.MakeTerm(keyspace.FamilyOutcome, 1)
	if err := rebuildSyntheticSuccessors(r, []edgeRow{
		{Edge: Edge{From: body, To: outcome}},
		{Edge: Edge{From: body, To: outcome}},
	}, nil); err != nil {
		t.Fatal(err)
	}
	if err := r.buildRouteIndex(); err == nil {
		t.Fatal("duplicate semantic route preimage was published")
	}
}

func TestClosedArmPredicatesRejectZeroAndUnknown(t *testing.T) {
	for _, arm := range []BoundaryArmKind{0, BoundaryCancel + 1, ^BoundaryArmKind(0)} {
		successor := Successor{Arm: arm}
		if successor.IsLocal() || successor.IsBoundary() {
			t.Fatalf("malformed arm %d was classified as a semantic plane", arm)
		}
		if _, _, _, ok := boundarySuccessor(CallBoundary{}, arm); ok {
			t.Fatalf("malformed arm %d produced a boundary route", arm)
		}
	}
}

func TestMalformedEdgeAndResetCombinationsFailClosed(t *testing.T) {
	r := syntheticResult()
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	outcome := keyspace.MakeTerm(keyspace.FamilyOutcome, 1)
	loop := keyspace.MakeTerm(keyspace.FamilyLoop, 1)
	r.reset.headRanges[keyspace.FamilyLoop] = make([]range32, 2)
	r.reset.headRanges[keyspace.FamilyLoop][1] = range32{start: 0, end: 2}
	r.reset.streams = []keyspace.Term{
		keyspace.MakeTerm(keyspace.FamilySelect, 1),
		keyspace.MakeTerm(keyspace.FamilyBranch, 1),
	}
	cases := []edgeRow{
		{Edge: Edge{From: body, To: outcome, Truth: true}},
		{Edge: Edge{From: body, To: outcome, Decision: body}},
		{Edge: Edge{From: body, To: outcome}, resetStart: 1, resetPast: 1},
		{Edge: Edge{From: body, To: outcome, Mu: body}},
		{Edge: Edge{From: body, To: outcome, Mu: loop}, resetStart: 1, resetPast: 0},
		{Edge: Edge{From: body, To: outcome, Mu: loop}, resetStart: 0, resetPast: 3},
		{Edge: Edge{From: body, To: outcome, Mu: loop}, resetDigest: keyspace.ContentID{7}},
	}
	for index, row := range cases {
		r.edges.rows = []edgeRow{row}
		if _, ok := r.Edges().At(0); ok {
			t.Fatalf("malformed Edge combination %d was observable: %#v", index, row)
		}
	}

	// The stream term itself must also be inside the sealed Select/Branch/Loop
	// denominators; family-shaped terms with a foreign ordinal are malformed.
	r.reset.streams = []keyspace.Term{keyspace.MakeTerm(keyspace.FamilySelect, 2)}
	r.edges.rows = []edgeRow{{Edge: Edge{From: body, To: outcome, Mu: loop}, resetStart: 0, resetPast: 1}}
	if err := rebuildSyntheticSuccessors(r, r.edges.rows, nil); err != nil {
		t.Fatal(err)
	}
	if err := r.buildRouteIndex(); err == nil {
		t.Fatal("reset stream term outside the sealed family denominator was accepted")
	}
}

func TestMalformedBoundaryArmAndResetWitnessFailClosed(t *testing.T) {
	r := syntheticResult()
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	call := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	outcome := keyspace.MakeTerm(keyspace.FamilyOutcome, 1)
	boundary := boundaryRow{CallBoundary: CallBoundary{Call: call, Normal: body, Throw: outcome, Yield: outcome, Cancel: outcome, mode: boundaryDirect}}
	if err := rebuildSyntheticSuccessors(r, nil, []boundaryRow{boundary}); err != nil {
		t.Fatal(err)
	}
	for _, malformed := range []successorRef{
		{index: 0, local: false, arm: BoundaryLocal},
		{index: 0, local: false, arm: BoundaryCancel + 1},
		{index: 0, local: true, arm: BoundaryResume},
	} {
		r.index.refs[0] = malformed
		if _, ok := r.Successors().At(call, 0); ok {
			t.Fatalf("malformed successor ref was observable: %#v", malformed)
		}
	}

	r = syntheticResult()
	if err := rebuildSyntheticSuccessors(r, []edgeRow{{Edge: Edge{From: body, To: outcome}}}, nil); err != nil {
		t.Fatal(err)
	}
	if err := r.buildRouteIndex(); err != nil {
		t.Fatal(err)
	}
	successor, ok := r.Successors().At(body, 0)
	if !ok {
		t.Fatal("valid route disappeared before malformed witness checks")
	}
	identity, ok := successor.Identity()
	if !ok {
		t.Fatal("valid route identity unavailable")
	}
	malformedIdentities := []RouteIdentity{
		func() RouteIdentity { copy := identity; copy.Arm = BoundaryCancel + 1; return copy }(),
		func() RouteIdentity { copy := identity; copy.Truth = true; return copy }(),
		func() RouteIdentity { copy := identity; copy.ResetCount = 1; return copy }(),
		func() RouteIdentity { copy := identity; copy.ResetDigest = keyspace.ContentID{7}; return copy }(),
	}
	for index, malformed := range malformedIdentities {
		if malformed.available() {
			t.Fatalf("malformed identity %d passed local preimage validation", index)
		}
		if _, ok := r.Successors().Resolve(malformed); ok {
			t.Fatalf("malformed identity %d resolved", index)
		}
	}
	r.edges.rows[0].resetCount = 1
	if _, ok := r.Successors().At(body, 0); ok {
		t.Fatal("malformed sealed reset witness remained observable")
	}
}

func TestEquivalentResealAndResetPermutationPreserveIdentity(t *testing.T) {
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	outcome1 := keyspace.MakeTerm(keyspace.FamilyOutcome, 1)
	outcome2 := keyspace.MakeTerm(keyspace.FamilyOutcome, 2)
	left := syntheticResult()
	right := syntheticResult()
	if err := rebuildSyntheticSuccessors(left, []edgeRow{
		{Edge: Edge{From: body, To: outcome1}},
		{Edge: Edge{From: body, To: outcome2}},
	}, nil); err != nil {
		t.Fatal(err)
	}
	if err := rebuildSyntheticSuccessors(right, []edgeRow{
		{Edge: Edge{From: body, To: outcome2}},
		{Edge: Edge{From: body, To: outcome1}},
	}, nil); err != nil {
		t.Fatal(err)
	}
	if err := left.buildRouteIndex(); err != nil {
		t.Fatal(err)
	}
	if err := right.buildRouteIndex(); err != nil {
		t.Fatal(err)
	}
	leftIDs, rightIDs := make([]RouteIdentity, 0, 2), make([]RouteIdentity, 0, 2)
	for index := 0; index < 2; index++ {
		leftRoute, leftOK := left.Successors().At(body, index)
		rightRoute, rightOK := right.Successors().At(body, index)
		if !leftOK || !rightOK {
			t.Fatal("equivalent reseal route disappeared")
		}
		leftID, leftIDOK := leftRoute.Identity()
		rightID, rightIDOK := rightRoute.Identity()
		if !leftIDOK || !rightIDOK {
			t.Fatal("equivalent reseal identity unavailable")
		}
		leftIDs, rightIDs = append(leftIDs, leftID), append(rightIDs, rightID)
	}
	for _, leftID := range leftIDs {
		found := false
		for _, rightID := range rightIDs {
			if leftID == rightID {
				found = true
				if _, ok := right.Successors().Resolve(leftID); !ok {
					t.Fatal("equivalent reseal identity failed to resolve")
				}
				break
			}
		}
		if !found {
			t.Fatalf("equivalent reseal identity %#v was not preserved", leftID)
		}
	}

	permuted := func(order []keyspace.Term) RouteIdentity {
		r := syntheticResult()
		loop := keyspace.MakeTerm(keyspace.FamilyLoop, 1)
		r.reset.streams = append([]keyspace.Term(nil), order...)
		r.reset.headRanges[keyspace.FamilyLoop] = make([]range32, 2)
		r.reset.headRanges[keyspace.FamilyLoop][1] = range32{start: 0, end: uint32(len(order))}
		r.edges.rows = []edgeRow{{Edge: Edge{From: body, To: outcome1, Mu: loop}, component: loop, resetStart: 0, resetPast: uint32(len(order))}}
		if err := rebuildSyntheticSuccessors(r, r.edges.rows, nil); err != nil {
			t.Fatal(err)
		}
		if err := r.buildRouteIndex(); err != nil {
			t.Fatal(err)
		}
		route, ok := r.Successors().At(body, 0)
		if !ok {
			t.Fatal("reset-permuted route disappeared")
		}
		identity, ok := route.Identity()
		if !ok {
			t.Fatal("reset-permuted route identity unavailable")
		}
		return identity
	}
	selectTerm := keyspace.MakeTerm(keyspace.FamilySelect, 1)
	branchTerm := keyspace.MakeTerm(keyspace.FamilyBranch, 1)
	if first, second := permuted([]keyspace.Term{selectTerm, branchTerm}), permuted([]keyspace.Term{branchTerm, selectTerm}); first != second {
		t.Fatal("reset permutation changed canonical route identity")
	}
}

func TestUnavailableResultQueriesFailClosed(t *testing.T) {
	r := syntheticResult()
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	call := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	r.edges.rows = []edgeRow{{Edge: Edge{From: body, To: keyspace.MakeTerm(keyspace.FamilyOutcome, 1)}}}
	r.boundaries.rows = []boundaryRow{{CallBoundary: CallBoundary{Call: call, Normal: body, mode: boundaryDirect}}}
	r.boundaries.callSlots = make([]uint32, 4)
	r.boundaries.callSlots[1] = 1
	var zero keyspace.ContentID
	r.sourceID = zero
	r.flowID = zero

	edges, boundaries, successors := r.Edges(), r.Boundaries(), r.Successors()
	if edges.Count() != 0 || boundaries.Count() != 0 || successors.Count(body) != 0 {
		t.Fatal("unavailable Result exposed retained counts")
	}
	if _, ok := edges.At(0); ok {
		t.Fatal("unavailable Result exposed an Edge row")
	}
	if _, _, ok := edges.Decision(0); ok {
		t.Fatal("unavailable Result exposed an Edge decision")
	}
	if _, ok := edges.Mu(0); ok {
		t.Fatal("unavailable Result exposed an Edge Mu")
	}
	if _, ok := edges.ResetCount(0); ok {
		t.Fatal("unavailable Result exposed reset membership")
	}
	if _, ok := edges.ResetAt(0, 0); ok || edges.ResetContains(0, body) {
		t.Fatal("unavailable Result exposed reset queries")
	}
	if _, ok := edges.BodyCount(body); ok {
		t.Fatal("unavailable Result exposed body projections")
	}
	if _, ok := edges.ActivationCount(body); ok {
		t.Fatal("unavailable Result exposed activation projections")
	}
	if _, ok := boundaries.At(0); ok {
		t.Fatal("unavailable Result exposed a CallBoundary row")
	}
	if _, ok := boundaries.For(call); ok {
		t.Fatal("unavailable Result exposed a CallBoundary slot")
	}
	if _, ok := successors.At(body, 0); ok {
		t.Fatal("unavailable Result exposed a successor")
	}
	if Matches(r, keyspace.ContentID{1}, keyspace.ContentID{2}, keyspace.ContentID{3}, keyspace.ContentID{4}) {
		t.Fatal("unavailable Result matched provenance")
	}

	var nilResult *Result
	if nilResult.Edges().Count() != 0 || nilResult.Boundaries().Count() != 0 || nilResult.Successors().Count(body) != 0 {
		t.Fatal("nil Result query did not fail closed")
	}
}

func TestSealRejectsCallOriginLocalEdge(t *testing.T) {
	call := keyspace.MakeTerm(keyspace.FamilyCall, 1)
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	s := &indexState{proofState: &proofState{}, edgeRowsScratch: &edgeRowsScratch{
		edgeRows:   []edgeRow{{Edge: Edge{From: call, To: body}}},
		edgeOwners: []keyspace.Term{body},
	}}
	if err := s.finish(); err == nil {
		t.Fatal("Call-origin local Edge was accepted")
	}
}

func TestMalformedMuLessSelfEdgeIsRejectedByQueries(t *testing.T) {
	r := syntheticResult()
	body := keyspace.MakeTerm(keyspace.FamilyBody, 1)
	r.edges.rows = []edgeRow{{Edge: Edge{From: body, To: body}}}

	if r.validEdgeRow(r.edges.rows[0]) {
		t.Fatal("Mu-less self Edge passed retained row validation")
	}
	if _, ok := r.Edges().At(0); ok {
		t.Fatal("Mu-less self Edge was published by the public query")
	}
}
