package demand

import (
	"strconv"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"
	"github.com/wippyai/go-lua/analysis/engine/internal/composition"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/internal/factbinding"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/change"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"github.com/wippyai/go-lua/analysis/engine/internal/schedule"
	"github.com/wippyai/go-lua/analysis/lattice"
)

// demandPointOf puts one closed Contribution into the nominal point role that
// the live coverage projection consumes.
func demandPointOf(t testing.TB, work *carrier.Work, value carrier.Contribution) carrier.PointState {
	t.Helper()
	rule, ok := work.AsRuleContribution(value)
	if !ok {
		t.Fatal("closed rule role")
	}
	point, ok := work.PointStateFromRuleContribution(rule)
	if !ok {
		t.Fatal("closed point role")
	}
	return point
}

func demandBinding(keyEnd uint64, join, widen func(uint64, uint64) uint64, declare func(*factbinding.Binding[uint64, uint64]) bool, manager *guard.Manager) (*factbinding.Binding[uint64, uint64], bool) {
	algebra, ok := factbinding.Admit(keyEnd, uint64(0), lattice.Lattice[uint64]{Bottom: func() uint64 { return 0 }, Top: func() uint64 { return ^uint64(0) }, Equal: func(left, right uint64) bool { return left == right }, LessOrEq: func(left, right uint64) bool { return left <= right }, Join: join, Widen: widen}, func(uint64, uint64) bool { return true }, func(value uint64) uint64 { return value }, factbinding.Measure[uint64, uint64]{}, factbinding.Measure[uint64, uint64]{})
	if !ok {
		return nil, false
	}
	return factbinding.Bind(algebra, manager, declare)
}

func routingKey(kind byte, index int) composition.Key {
	var id composition.ID
	id[0], id[1], id[2] = kind, byte(index), byte(index>>8)
	return composition.Key{ID: id, Version: 1}
}

func routingOperandFamily(rule composition.Key) composition.Key {
	family := rule
	family.ID[31] ^= 0xa5
	return family
}

func routingOperand(rule composition.Key) composition.Key {
	operand := rule
	operand.ID[31] ^= 0x5a
	return operand
}

// wideRoutingGraph is a semantic fanout: every destination Group consumes
// the source Point. The demand component is intentionally given the separate
// fact Units below, so this setup tests routing behavior rather than any
// representation detail of the graph or binding.
func wideRoutingGraph(t testing.TB, width int) *equation.Graph {
	t.Helper()
	if width < 1 {
		t.Fatal("routing width")
	}
	factors := make([]composition.Factor, width)
	rules := make([]composition.Rule, width)
	instances := make([]equation.RuleInstance, width)
	points := make([]equation.PointSpec, width+1)
	groups := make([]equation.Group, width)
	scope := equation.EmptyScope()
	batch := equation.NewBatch()
	sites := make([]equation.Site, width+1)
	entry, ok := batch.AdmitSite(routingKey(2, 0), scope, equation.TrueExpr(), equation.InitPresent)
	if !ok {
		t.Fatal("routing entry Site")
	}
	sites[0] = entry
	for index := 0; index < width; index++ {
		factor, rule := routingKey(3, index), routingKey(4, index)
		factors[index] = composition.Factor{Key: factor}
		rules[index] = composition.Rule{Key: rule, OperandFamily: routingOperandFamily(rule), OutputKind: composition.FactorOutput, Output: factor, Inputs: 1, Writes: []composition.Write{{Kind: composition.WriteExact, Factor: factor}}}
		site, admitted := batch.AdmitSite(routingKey(2, index+1), scope, equation.FalseExpr(), equation.InitAbsent)
		occurrence, occurred := batch.At(site)
		operand, attached := batch.AdmitOperand(occurrence, routingOperand(rule))
		if !admitted || !occurred || !attached {
			t.Fatal("routing source row")
		}
		sites[index+1] = site
		instances[index] = equation.RuleInstance{Schema: rule, OperandFamily: routingOperandFamily(rule), Occurrence: occurrence, Operand: operand, Writes: []equation.ResolvedWrite{{Index: 0, Surface: equation.Surface{Factor: factor, Form: equation.SurfaceWriteExact, Local: 1, Mode: equation.TargetModeStrong}}}}
	}
	if !batch.Seal() {
		t.Fatal("routing source batch")
	}
	for index, site := range sites {
		points[index] = equation.PointSpec{Site: site}
	}
	for index := range groups {
		input := equation.BoundaryInput(sites[0], sites[index+1], routingKey(8, index), equation.TrueExpr(), equation.IdentityReindex(scope), equation.TrueExpr())
		if !input.Available() {
			t.Fatal("routing boundary")
		}
		groups[index] = equation.Group{Members: []equation.RuleRef{equation.RuleAt(index)}, Output: equation.PointAt(index + 1), Inputs: []equation.Input{input}}
	}
	queryFamily := routingKey(6, 0)
	cold, ok := composition.Seal(composition.Candidate{Factors: factors, Rules: rules, Queries: []composition.QueryFamily{{Key: queryFamily, Freezer: routingKey(7, 0), Projections: []composition.QueryProjection{{Kind: composition.QueryFactorExact, Factor: factors[0].Key}}}}})
	if !ok || cold == nil {
		t.Fatal("routing composition")
	}
	topology, ok := equation.SealTopology(cold, equation.TopologySpec{Batch: batch, Rules: instances, Points: points, Groups: groups, Queries: []equation.QueryInstance{{Family: queryFamily, Point: equation.PointAt(0), Surfaces: []equation.Surface{{Factor: factors[0].Key, Form: equation.SurfaceReadExact, Local: 1}}}}})
	if !ok || topology == nil {
		t.Fatal("routing topology")
	}
	relation, relationOK := topology.InitialRelation()
	graph, ok := topology.Graph(relation)
	if !relationOK || !ok || graph == nil || graph.GroupCount() != width {
		t.Fatal("routing graph")
	}
	return graph
}

// wideRouteChangeSet publishes one semantic change for every declared Unit.
// It also mints an owned coverage delta with no rows: routing fences coverage
// provenance, so a semantic-only publication still names its issuer.
func wideRouteChangeSet(t testing.TB, width int) (*carrier.Composition, []carrier.Unit, carrier.ChangeSet, carrier.ChangeSet, carrier.CoverageChangeSet, carrier.CoverageChangeSet) {
	t.Helper()
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	whole, ok := support.True(manager)
	if !ok {
		t.Fatal("routing support")
	}
	units, targets := make([]carrier.Unit, width), make([]carrier.Target, width)
	binding, ok := demandBinding(uint64(width), func(left, right uint64) uint64 {
		if left > right {
			return left
		}
		return right
	}, func(left, right uint64) uint64 {
		if left > right {
			return left
		}
		return right
	}, func(binding *factbinding.Binding[uint64, uint64]) bool {
		for index := range units {
			unit, declared := binding.DeclareExact(uint64(index))
			if !declared {
				return false
			}
			units[index] = unit
		}
		for index, unit := range units {
			target, declared := binding.DeclareStrong(unit)
			if !declared {
				return false
			}
			targets[index] = target
		}
		return true
	}, manager)
	if !ok {
		t.Fatal("routing binding")
	}
	prepared, ok := carrier.PrepareComposition([]carrier.FactorOperation{binding})
	if !ok {
		t.Fatal("routing prepare")
	}
	runtime, ok := prepared.Attach()
	if !ok {
		t.Fatal("routing attach")
	}
	coveragePlan, ok := runtime.SealContribution(0, []shape.Slot{0}, nil)
	if !ok {
		t.Fatal("routing coverage plan")
	}
	state, ok := carrier.NewState(runtime, runtime.Scope(), whole)
	if !ok {
		t.Fatal("routing state")
	}
	work, ok := runtime.NewWork()
	if !ok {
		t.Fatal("routing work")
	}
	empty, ok := support.FromGuard(manager, manager.False())
	if !ok {
		t.Fatal("routing empty support")
	}
	emptyState, ok := carrier.NewState(runtime, runtime.Scope(), empty)
	if !ok {
		t.Fatal("routing empty state")
	}
	_, supportChanges, ok := work.Merge3Under(carrier.Join, emptyState, state, runtime.AllMergeScope())
	if !ok || support.Empty(supportChanges.Added()) {
		t.Fatal("routing support change")
	}
	patch := binding.Begin(work, state)
	if patch == nil {
		t.Fatal("routing patch")
	}
	for index, target := range targets {
		if !patch.Write(target, whole, uint64(index+1)) {
			t.Fatal("routing write")
		}
	}
	publication, ok := patch.Accept(work)
	if !ok {
		t.Fatal("routing accept")
	}
	_, changes, ok := work.Commit(state, []carrier.Patch{publication})
	if !ok || changes.Count() != width {
		t.Fatal("routing commit")
	}
	emptyContribution, ok := work.EmptyContribution(state)
	if !ok {
		t.Fatal("routing empty contribution")
	}
	base, ok := work.BeginContribution(coveragePlan, runtime.Scope(), nil, whole)
	if !ok {
		t.Fatal("routing coverage base")
	}
	authoredStage := binding.Begin(work, base.State())
	if authoredStage == nil || !authoredStage.Write(targets[0], whole, 0) {
		t.Fatal("routing authored Default")
	}
	authoredPatch, ok := authoredStage.Accept(work)
	if !ok {
		t.Fatal("routing authored accept")
	}
	authored, ok := work.FinishContribution(base, []carrier.Patch{authoredPatch})
	if !ok {
		t.Fatal("routing authored finish")
	}
	unchanged := demandPointOf(t, work, emptyContribution)
	noCoverage, ok := work.CoverageChangesPointStates(unchanged, unchanged)
	if !ok || noCoverage.Count() != 0 {
		t.Fatal("routing empty coverage changes")
	}
	coverageChanges, ok := work.CoverageChangesPointStates(unchanged, demandPointOf(t, work, authored))
	if !ok || coverageChanges.Count() != 1 {
		t.Fatal("routing coverage changes")
	}
	return runtime, units, changes, supportChanges, coverageChanges, noCoverage
}

func wideRoutingEpoch(t testing.TB, width int) (*Epoch, int, carrier.ChangeSet, carrier.ChangeSet, carrier.CoverageChangeSet, carrier.CoverageChangeSet) {
	t.Helper()
	runtime, units, changes, supportChanges, coverageChanges, noCoverage := wideRouteChangeSet(t, width)
	graph := wideRoutingGraph(t, width)
	source := -1
	for index := 0; index < graph.PointCount(); index++ {
		point, ok := graph.PointAt(schedule.Node(index))
		if !ok {
			t.Fatal("routing source Point")
		}
		if point.HasInit() {
			if source >= 0 {
				t.Fatal("ambiguous routing source Point")
			}
			source = index
		}
	}
	if source < 0 {
		t.Fatal("missing routing source Point")
	}
	plan := &Plan{graph: graph, runtime: runtime, families: make([]Family, graph.GroupCount()), selected: make([]bool, graph.GroupCount())}
	for group := range plan.families {
		plan.families[group] = Family{Group: group, Inputs: []int{source}, InitialReads: make([]Observation, len(units)), Carries: []Carry{{Input: 0, Slot: shape.Slot(0)}}}
		for index, unit := range units {
			plan.families[group].InitialReads[index] = Observation{Input: 0, Unit: unit}
		}
		plan.selected[group] = true
	}
	if !plan.buildCSR() || !plan.buildReadIndex() {
		t.Fatal("routing plan")
	}
	plan.sealed = true
	epoch, ok := Open(plan)
	if !ok {
		t.Fatal("routing epoch")
	}
	return epoch, source, changes, supportChanges, coverageChanges, noCoverage
}

func mixedStaticDynamicRoutingEpoch(t testing.TB) (*Epoch, int, carrier.ChangeSet, carrier.CoverageChangeSet) {
	t.Helper()
	runtime, units, changes, _, _, noCoverage := wideRouteChangeSet(t, 2)
	graph := wideRoutingGraph(t, 1)
	source := -1
	for index := 0; index < graph.PointCount(); index++ {
		point, ok := graph.PointAt(schedule.Node(index))
		if !ok {
			t.Fatal("mixed routing Point")
		}
		if point.HasInit() {
			if source >= 0 {
				t.Fatal("mixed routing ambiguous source")
			}
			source = index
		}
	}
	if source < 0 {
		t.Fatal("mixed routing source")
	}
	slot, ok := units[0].Slot()
	if !ok {
		t.Fatal("mixed routing slot")
	}
	static := Observation{Input: 0, Unit: units[0]}
	plan := &Plan{
		graph:    graph,
		runtime:  runtime,
		families: []Family{{Group: 0, Inputs: []int{source}, InitialReads: []Observation{static}, DynamicReads: []DynamicRead{{Input: 0, Slot: slot}}}},
		selected: []bool{true},
	}
	if !plan.buildCSR() || !plan.buildReadIndex() {
		t.Fatal("mixed routing plan")
	}
	plan.sealed = true
	epoch, opened := Open(plan)
	if !opened {
		t.Fatal("mixed routing epoch")
	}
	if !epoch.Replace(0, []Observation{static, {Input: 0, Unit: units[1]}}) {
		t.Fatal("mixed routing dynamic route")
	}
	return epoch, source, changes, noCoverage
}

// relationEpoch builds the sealed dynamic-read portion directly. Graph and
// carry routing are deliberately outside these relation laws; the production
// assembly path invokes the same buildReadIndex from Plan.Seal.
func relationEpoch(t testing.TB, runtime *carrier.Composition, families ...Family) *Epoch {
	t.Helper()
	plan := &Plan{
		runtime:  runtime,
		families: append([]Family(nil), families...),
		selected: make([]bool, len(families)),
	}
	for group := range plan.families {
		if plan.families[group].Group != group {
			t.Fatalf("family[%d] group = %d", group, plan.families[group].Group)
		}
		plan.selected[group] = true
	}
	if !plan.buildReadIndex() {
		t.Fatal("seal relation index")
	}
	plan.sealed = true
	epoch, ok := Open(plan)
	if !ok {
		t.Fatal("open relation epoch")
	}
	return epoch
}

func hasWake(epoch *Epoch, point int, unit carrier.Unit, group int) bool {
	for _, woken := range epoch.appendCurrentUnitGroups(nil, point, unit) {
		if woken == group {
			return true
		}
	}
	return false
}

func hasDynamicWake(epoch *Epoch, point int, unit carrier.Unit, group int) bool {
	found := false
	if !epoch.visitDynamicUnitGroups(point, unit, func(woken int) bool {
		if woken == group {
			found = true
		}
		return true
	}) {
		return false
	}
	return found
}

func TestReplaceZeroReadRetractsInitialReverseSlice(t *testing.T) {
	runtime, first, _ := liveUnitFixture(t)
	epoch := relationEpoch(t, runtime, Family{Group: 0, Inputs: []int{7}, InitialReads: []Observation{{Input: 0, Unit: first}}})
	if !epoch.Replace(0, nil) {
		t.Fatal("zero-read replacement")
	}
	if got := epoch.activeObservationCount(0); got != 0 {
		t.Fatalf("active observations = %d, want zero", got)
	}
	if hasWake(epoch, 7, first, 0) {
		t.Fatal("zero-read replacement retained stale inverse subscriber")
	}
}

func TestRejectedReplacementPreservesCurrentReverseSlice(t *testing.T) {
	runtime, first, _ := liveUnitFixture(t)
	valid := Observation{Input: 0, Unit: first}
	epoch := relationEpoch(t, runtime, Family{Group: 0, Inputs: []int{7}, InitialReads: []Observation{valid}})
	if epoch.Replace(0, []Observation{{Input: 0, Unit: carrier.Unit{}}}) {
		t.Fatal("undeclared dynamic observation was admitted")
	}
	if got := epoch.activeObservationCount(0); got != 1 {
		t.Fatalf("rejected replacement changed active membership count: %d", got)
	}
	got, live := epoch.activeObservation(0, 0)
	if !live || got != valid {
		t.Fatalf("rejected replacement changed active membership: %#v, %v", got, live)
	}
	if !hasWake(epoch, 7, first, 0) {
		t.Fatal("rejected replacement changed inverse membership")
	}
}

func TestInverseSubscriptionsRemainCanonicalAndDeduplicated(t *testing.T) {
	runtime, first, _ := liveUnitFixture(t)
	families := make([]Family, 4)
	for group := range families {
		families[group] = Family{Group: group, Inputs: []int{7}, InitialReads: []Observation{{Input: 0, Unit: first}}}
	}
	epoch := relationEpoch(t, runtime, families...)
	if !epoch.Replace(1, nil) || !epoch.Replace(3, nil) {
		t.Fatal("retract selected subscribers")
	}
	groups := epoch.appendCurrentUnitGroups(nil, 7, first)
	if len(groups) != 2 || groups[0] != 0 || groups[1] != 2 {
		t.Fatalf("noncanonical exact inverse wakes: %#v", groups)
	}
}

func liveUnitFixture(t testing.TB) (*carrier.Composition, carrier.Unit, carrier.Unit) {
	t.Helper()
	runtime, units := liveUnits(t, 2)
	return runtime, units[0], units[1]
}

func liveUnits(t testing.TB, count uint64) (*carrier.Composition, []carrier.Unit) {
	t.Helper()
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	units := make([]carrier.Unit, count)
	binding, ok := demandBinding(count, func(left, _ uint64) uint64 { return left }, func(left, _ uint64) uint64 { return left }, func(binding *factbinding.Binding[uint64, uint64]) bool {
		for key := uint64(0); key < count; key++ {
			unit, declared := binding.DeclareExact(key)
			if !declared {
				return false
			}
			units[key] = unit
		}
		return true
	}, manager)
	if !ok {
		t.Fatal("binding")
	}
	prepared, ok := carrier.PrepareComposition([]carrier.FactorOperation{binding})
	if !ok {
		t.Fatal("prepare composition")
	}
	runtime, ok := prepared.Attach()
	if !ok {
		t.Fatal("attach composition")
	}
	return runtime, units
}

func liveExactAndSummary(t testing.TB) (*carrier.Composition, carrier.Unit, carrier.Unit) {
	t.Helper()
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	var exact, summary carrier.Unit
	binding, ok := demandBinding(2, func(left, _ uint64) uint64 { return left }, func(left, _ uint64) uint64 { return left }, func(binding *factbinding.Binding[uint64, uint64]) bool {
		var declared bool
		exact, declared = binding.DeclareExact(0)
		if !declared {
			return false
		}
		summary, declared = binding.DeclareSummary([]uint64{0})
		return declared
	}, manager)
	if !ok {
		t.Fatal("summary binding")
	}
	prepared, ok := carrier.PrepareComposition([]carrier.FactorOperation{binding})
	if !ok {
		t.Fatal("summary prepare")
	}
	runtime, ok := prepared.Attach()
	if !ok {
		t.Fatal("summary attach")
	}
	return runtime, exact, summary
}

func liveTwoFactorUnits(t testing.TB) (*carrier.Composition, carrier.Unit, carrier.Unit) {
	t.Helper()
	manager, err := guard.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	var first, second carrier.Unit
	makeBinding := func(target *carrier.Unit, key uint64) *factbinding.Binding[uint64, uint64] {
		binding, ok := demandBinding(2, func(left, _ uint64) uint64 { return left }, func(left, _ uint64) uint64 { return left }, func(binding *factbinding.Binding[uint64, uint64]) bool {
			unit, declared := binding.DeclareExact(key)
			if declared {
				*target = unit
			}
			return declared
		}, manager)
		if !ok {
			t.Fatal("two-factor binding")
		}
		return binding
	}
	left := makeBinding(&first, 0)
	right := makeBinding(&second, 1)
	prepared, ok := carrier.PrepareComposition([]carrier.FactorOperation{left, right})
	if !ok {
		t.Fatal("two-factor prepare")
	}
	runtime, ok := prepared.Attach()
	if !ok {
		t.Fatal("two-factor attach")
	}
	return runtime, first, second
}

func TestReplaceAtomicallyReplacesLiveDynamicReverseSlice(t *testing.T) {
	runtime, first, second := liveUnitFixture(t)
	slot, ok := first.Slot()
	if !ok {
		t.Fatal("dynamic slot")
	}
	static := Observation{Input: 0, Unit: first}
	dynamic := Observation{Input: 0, Unit: second}
	family := Family{Group: 0, Inputs: []int{5}, InitialReads: []Observation{static}, DynamicReads: []DynamicRead{{Input: 0, Slot: slot}}}
	epoch := relationEpoch(t, runtime, family)
	if !epoch.Replace(0, []Observation{static, dynamic, dynamic}) {
		t.Fatal("live replacement")
	}
	if !hasWake(epoch, 5, first, 0) {
		t.Fatal("required static observation was removed by dynamic replacement")
	}
	if !hasDynamicWake(epoch, 5, second, 0) {
		t.Fatal("new inverse subscriber missing")
	}
	if got := epoch.activeObservationCount(0); got != 2 {
		t.Fatalf("current active observations = %d, want two", got)
	}
	if !epoch.Replace(0, []Observation{static}) {
		t.Fatal("remove dynamic route")
	}
	if hasDynamicWake(epoch, 5, second, 0) {
		t.Fatal("dynamic inverse subscriber survived replacement")
	}
}

func TestReplaceDeduplicatesDynamicReportWithoutSorting(t *testing.T) {
	runtime, first, second := liveUnitFixture(t)
	slot, ok := first.Slot()
	if !ok {
		t.Fatal("dynamic slot")
	}
	family := Family{Group: 0, Inputs: []int{5}, DynamicReads: []DynamicRead{{Input: 0, Slot: slot}}}
	epoch := relationEpoch(t, runtime, family)
	if !epoch.Replace(0, []Observation{{Input: 0, Unit: second}, {Input: 0, Unit: first}, {Input: 0, Unit: second}}) {
		t.Fatal("replacement")
	}
	if got := epoch.activeObservationCount(0); got != 2 {
		t.Fatalf("deduplicated active observations = %d, want two", got)
	}
	if !hasDynamicWake(epoch, 5, first, 0) || !hasDynamicWake(epoch, 5, second, 0) {
		t.Fatal("deduplicated report lost exact source membership")
	}
}

func TestReplaceDeduplicatesAliasedInputReverseSubscriber(t *testing.T) {
	runtime, _, second := liveUnitFixture(t)
	slot, ok := second.Slot()
	if !ok {
		t.Fatal("dynamic slot")
	}
	family := Family{Group: 0, Inputs: []int{5, 5}, DynamicReads: []DynamicRead{{Input: 0, Slot: slot}, {Input: 1, Slot: slot}}}
	epoch := relationEpoch(t, runtime, family)
	if !epoch.Replace(0, []Observation{{Input: 0, Unit: second}, {Input: 1, Unit: second}}) {
		t.Fatal("aliased-input replacement")
	}
	groups := make([]int, 0, 1)
	if !epoch.visitDynamicUnitGroups(5, second, func(group int) bool {
		groups = append(groups, group)
		return true
	}) || len(groups) != 1 || groups[0] != 0 {
		t.Fatalf("aliased dynamic read duplicated inverse subscriber: %#v", groups)
	}
}

func TestRouteTypedSliceHasNoStaleSubscriberAfterReplacement(t *testing.T) {
	runtime, first, second := liveUnitFixture(t)
	slot, ok := first.Slot()
	if !ok {
		t.Fatal("dynamic slot")
	}
	static := Observation{Input: 0, Unit: first}
	family := Family{Group: 0, Inputs: []int{5}, InitialReads: []Observation{static}, DynamicReads: []DynamicRead{{Input: 0, Slot: slot}}}
	epoch := relationEpoch(t, runtime, family)
	if !epoch.Replace(0, []Observation{static, Observation{Input: 0, Unit: second}}) {
		t.Fatal("replacement")
	}
	if !epoch.Replace(0, []Observation{static}) {
		t.Fatal("remove dynamic route")
	}
	if groups := epoch.appendCurrentUnitGroups(nil, 5, first); len(groups) != 1 || groups[0] != 0 {
		t.Fatalf("required static reader did not wake after replacement: %#v", groups)
	}
	if hasDynamicWake(epoch, 5, second, 0) {
		t.Fatal("stale dynamic reader woke after replacement")
	}
}

func TestDynamicReadRejectsSummaryUnit(t *testing.T) {
	runtime, exact, summary := liveExactAndSummary(t)
	if summary.Kind() != carrier.SummaryUnit {
		t.Fatalf("summary kind = %v", summary.Kind())
	}
	slot, ok := exact.Slot()
	if !ok {
		t.Fatal("exact slot")
	}
	epoch := relationEpoch(t, runtime, Family{Group: 0, Inputs: []int{5}, DynamicReads: []DynamicRead{{Input: 0, Slot: slot}}})
	if epoch.Replace(0, []Observation{{Input: 0, Unit: summary}}) {
		t.Fatal("summary Unit was admitted as a staged exact route")
	}
	if got := epoch.activeObservationCount(0); got != 0 {
		t.Fatalf("summary rejection changed active observations: %d", got)
	}
	if hasDynamicWake(epoch, 5, summary, 0) {
		t.Fatal("summary rejection installed an inverse route")
	}
}

func TestDynamicReadRejectsForeignSlotAndInputWithoutMutation(t *testing.T) {
	runtime, first, second := liveTwoFactorUnits(t)
	firstSlot, firstOK := first.Slot()
	secondSlot, secondOK := second.Slot()
	if !firstOK || !secondOK || firstSlot == secondSlot {
		t.Fatal("two-factor slots")
	}
	family := Family{Group: 0, Inputs: []int{5, 6}, DynamicReads: []DynamicRead{{Input: 0, Slot: firstSlot}}}
	epoch := relationEpoch(t, runtime, family)
	valid := Observation{Input: 0, Unit: first}
	if !epoch.Replace(0, []Observation{valid}) {
		t.Fatal("install valid exact dynamic route")
	}
	foreignRuntime, foreign, _ := liveUnitFixture(t)
	if foreignRuntime == runtime {
		t.Fatal("foreign runtime fixture")
	}
	for name, invalid := range map[string]Observation{
		"foreign":     {Input: 0, Unit: foreign},
		"wrong-slot":  {Input: 0, Unit: second},
		"wrong-input": {Input: 1, Unit: first},
	} {
		if epoch.Replace(0, []Observation{invalid}) {
			t.Fatalf("%s dynamic route was admitted", name)
		}
		if got := epoch.activeObservationCount(0); got != 1 {
			t.Fatalf("%s rejection changed active count: %d", name, got)
		}
		got, live := epoch.activeObservation(0, 0)
		if !live || !sameObservation(got, valid) {
			t.Fatalf("%s rejection changed active route: %#v, %t", name, got, live)
		}
		if !hasDynamicWake(epoch, 5, first, 0) || hasDynamicWake(epoch, 5, second, 0) {
			t.Fatalf("%s rejection changed sparse inverse", name)
		}
	}
}

func TestReplaceNilClearsStaticAndDynamicRoutes(t *testing.T) {
	runtime, first, second := liveUnitFixture(t)
	slot, ok := first.Slot()
	if !ok {
		t.Fatal("dynamic slot")
	}
	static := Observation{Input: 0, Unit: first}
	dynamic := Observation{Input: 0, Unit: second}
	epoch := relationEpoch(t, runtime, Family{Group: 0, Inputs: []int{5}, InitialReads: []Observation{static}, DynamicReads: []DynamicRead{{Input: 0, Slot: slot}}})
	if !epoch.Replace(0, []Observation{static, dynamic}) {
		t.Fatal("install mixed route")
	}
	if !epoch.Replace(0, nil) {
		t.Fatal("nil reset")
	}
	if got := epoch.activeObservationCount(0); got != 0 {
		t.Fatalf("nil reset retained %d active observations", got)
	}
	if hasWake(epoch, 5, first, 0) || hasDynamicWake(epoch, 5, second, 0) {
		t.Fatal("nil reset retained an inverse route")
	}
}

func TestDynamicReplaceSwitchesAtoBAndReportsCanonicalActiveRoute(t *testing.T) {
	runtime, first, second := liveUnitFixture(t)
	slot, ok := first.Slot()
	if !ok {
		t.Fatal("dynamic slot")
	}
	epoch := relationEpoch(t, runtime, Family{Group: 0, Inputs: []int{5}, DynamicReads: []DynamicRead{{Input: 0, Slot: slot}}})
	if !epoch.Replace(0, []Observation{{Input: 0, Unit: first}}) {
		t.Fatal("install A")
	}
	if !epoch.Replace(0, []Observation{{Input: 0, Unit: second}}) {
		t.Fatal("switch A to B")
	}
	if hasDynamicWake(epoch, 5, first, 0) || !hasDynamicWake(epoch, 5, second, 0) {
		t.Fatal("A-to-B replacement retained stale or lost current wake")
	}
	if got := epoch.activeObservationCount(0); got != 1 {
		t.Fatalf("active count = %d, want one", got)
	}
	got, live := epoch.activeObservation(0, 0)
	if !live || !sameObservation(got, Observation{Input: 0, Unit: second}) {
		t.Fatalf("active route = %#v, live:%t; want B", got, live)
	}
}

func TestRouteCoalescesWideChangedUnitsToOneWakePerGroup(t *testing.T) {
	const width = 16
	epoch, source, changes, supportChanges, _, noCoverage := wideRoutingEpoch(t, width)
	supportWakes, ok := epoch.Route(source, supportChanges, noCoverage)
	if !ok || len(supportWakes) != width {
		t.Fatalf("support route = ok:%t wakes:%d, want %d", ok, len(supportWakes), width)
	}
	for _, wake := range supportWakes {
		want := change.Set{Reasons: change.SupportAdded, Direction: change.Known | change.Ascends}
		if wake.Reasons != want {
			t.Fatalf("support evidence = %#v, want %#v", wake.Reasons, want)
		}
		if !wake.Reasons.Admits() {
			t.Fatalf("a proven ascent is not admissible: %#v", wake)
		}
	}
	wakes, ok := epoch.Route(source, changes, noCoverage)
	if !ok || len(wakes) != width {
		t.Fatalf("wide route = ok:%t wakes:%d, want %d", ok, len(wakes), width)
	}
	seen := make([]bool, width)
	for _, wake := range wakes {
		if wake.Group < 0 || wake.Group >= width || seen[wake.Group] {
			t.Fatalf("duplicate or foreign wake: %#v", wake)
		}
		// The fixture's Groups read the changed Units and carry the changed
		// Factor Slot. Both witnesses survive; the route retains the later
		// channel instead of stopping at the first.
		if wake.Reasons != (change.Set{Reasons: change.ChangedUnit | change.ChangedFactor}) {
			t.Fatalf("typed evidence = %#v", wake.Reasons)
		}
		if !wake.Reasons.Unknown() || wake.Reasons.Admits() {
			t.Fatalf("an unclassified typed change was admitted: %#v", wake)
		}
		seen[wake.Group] = true
	}
	for group, present := range seen {
		if !present {
			t.Fatalf("missing subscribed Group %d", group)
		}
	}
}

func TestRouteCoalescesStaticAndDynamicExactRoutes(t *testing.T) {
	epoch, source, changes, noCoverage := mixedStaticDynamicRoutingEpoch(t)
	wakes, ok := epoch.Route(source, changes, noCoverage)
	if !ok || len(wakes) != 1 {
		t.Fatalf("mixed route = ok:%t wakes:%#v", ok, wakes)
	}
	if wakes[0].Group != 0 || wakes[0].Reasons != (change.Set{Reasons: change.ChangedUnit}) {
		t.Fatalf("mixed route wake = %#v", wakes[0])
	}
}

// One route accumulates every channel that reached a Group. A Group woken by
// both a typed change and an authorship change is one Wake carrying both
// reasons, so no consumer has a second channel left to de-duplicate against.
func TestRouteAccumulatesSemanticAndCoverageEvidenceIntoOneWakePerGroup(t *testing.T) {
	const width = 4
	epoch, source, changes, _, coverageChanges, noCoverage := wideRoutingEpoch(t, width)
	semanticOnly, ok := epoch.Route(source, changes, noCoverage)
	if !ok || len(semanticOnly) != width {
		t.Fatalf("semantic route = ok:%t wakes:%d", ok, len(semanticOnly))
	}
	positions := make([]int, 0, width)
	for _, wake := range semanticOnly {
		positions = append(positions, wake.Group)
	}
	wakes, ok := epoch.Route(source, changes, coverageChanges)
	if !ok || len(wakes) != width {
		t.Fatalf("accumulating route = ok:%t wakes:%d, want %d", ok, len(wakes), width)
	}
	slot, slotOK := wideRouteCarrySlot(t, epoch)
	if !slotOK {
		t.Fatal("routing carry slot")
	}
	for index, wake := range wakes {
		if wake.Group != positions[index] {
			t.Fatalf("wake[%d] group = %d, want the semantic first-mark position %d", index, wake.Group, positions[index])
		}
		if !wake.Reasons.Has(change.ChangedUnit) || !wake.Reasons.Has(change.AuthorshipChanged) {
			t.Fatalf("wake[%d] lost a channel: %#v", index, wake.Reasons)
		}
		if !wake.Slots.Test(int(slot)) || wake.Slots.Count() != 1 {
			t.Fatalf("wake[%d] carried slots %d, want exactly the authored carry", index, wake.Slots.Count())
		}
	}
}

// wideRouteCarrySlot names the one Slot the wide routing fixture carries.
func wideRouteCarrySlot(t testing.TB, epoch *Epoch) (shape.Slot, bool) {
	t.Helper()
	if len(epoch.plan.families) == 0 || len(epoch.plan.families[0].Carries) != 1 {
		return 0, false
	}
	return epoch.plan.families[0].Carries[0].Slot, true
}

func BenchmarkRouteWideChanges(b *testing.B) {
	for _, width := range []int{16, 64, 256} {
		b.Run("width="+strconv.Itoa(width), func(b *testing.B) {
			epoch, source, changes, _, _, noCoverage := wideRoutingEpoch(b, width)
			b.ReportAllocs()
			b.ResetTimer()
			for index := 0; index < b.N; index++ {
				wakes, ok := epoch.Route(source, changes, noCoverage)
				if !ok || len(wakes) != width {
					b.Fatal("wide route")
				}
			}
			b.ReportMetric(float64(width), "wakes/op")
		})
	}
}

func TestReplaceSteadyStateAllocatesNothingAcrossLargeDynamicSurface(t *testing.T) {
	const width = 512
	runtime, units := liveUnits(t, width)
	left := make([]Observation, 0, width/2)
	for index, unit := range units {
		observation := Observation{Input: 0, Unit: unit}
		if index&1 == 0 {
			left = append(left, observation)
		}
	}
	slot, ok := units[0].Slot()
	if !ok {
		t.Fatal("dynamic slot")
	}
	epoch := relationEpoch(t, runtime, Family{Group: 0, Inputs: []int{5}, DynamicReads: []DynamicRead{{Input: 0, Slot: slot}}})
	if !epoch.Replace(0, left) || !epoch.Replace(0, left) {
		t.Fatal("prime stable dynamic replacement")
	}
	allocations := testing.AllocsPerRun(128, func() {
		if !epoch.Replace(0, left) {
			panic("replace stable dynamic route")
		}
	})
	if allocations != 0 {
		t.Fatalf("Replace allocated %f objects/op after sealing", allocations)
	}
	if got := epoch.activeObservationCount(0); got != width/2 {
		t.Fatalf("active dynamic surface = %d, want %d", got, width/2)
	}
}
