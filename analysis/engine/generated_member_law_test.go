package engine

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/execution"
	"github.com/wippyai/go-lua/analysis/engine/generated"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/demand"
	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
	"github.com/wippyai/go-lua/analysis/engine/internal/executioncatalog"
)

func generatedMemberTestMember(t *testing.T) equation.RuleMember {
	t.Helper()
	matrix := newReceiptQueryMatrixFixture(t, 1, nil, nil)
	group, ok := matrix.graph.graph.HyperedgeAt(0)
	if !ok {
		t.Fatal("generated member test group")
	}
	member, ok := group.MemberAt(0)
	if !ok || !member.Key().Available() {
		t.Fatal("generated member test member")
	}
	return member
}

func generatedMemberTestSpec(t *testing.T, fixture generatedFactorAdapterFixture, rule uint32, candidate uint32) generatedMemberSpec {
	t.Helper()
	return generatedMemberSpec{
		member:        generatedMemberTestMember(t),
		outputSlot:    fixture.slot,
		hasSlot:       true,
		factor:        compositionKeyOf(coldKey(999_101)),
		factorOrdinal: 0,
		initial:       []demand.Observation{{Input: 0, Unit: fixture.unit}},
		targets:       []carrier.Target{fixture.target},
		writes:        true,
		unit:          fixture.unit,
		target:        fixture.target,
		readInput:     0,
		rule:          rule,
		candidate:     candidate,
		inputCount:    1,
		outputCount:   1,
	}
}

func generatedMemberRuntime(t *testing.T, fixture generatedFactorAdapterFixture, rule uint32, candidate uint32) (*generatedMember, *executorEpoch) {
	t.Helper()
	member, ok := newGeneratedMember(generatedMemberTestSpec(t, fixture, rule, candidate))
	if !ok || member == nil {
		t.Fatal("seal generated member")
	}
	descriptor, descriptorOK := newExactIdentityDescriptor(rule, 1, 0, 0, 0, 1, 1)
	if !descriptorOK {
		t.Fatal("seal generated descriptor")
	}
	program := &runtimeProgram{
		generatedPrograms: make([]generated.CompiledRule, int(rule)+1),
		generatedPresent:  true,
		factorOwners:      []runtimeFactor{fixture.factor},
		programSealed:     true,
	}
	program.generatedPrograms[rule] = descriptor
	exactRow, rowOK := execution.NewExactRow(fixture.binding, fixture.unit, 0, fixture.target, 0)
	family, familyOK := execution.NewExactFamily([]execution.ExactRow[uint32, uint64]{exactRow})
	catalog, catalogOK := executioncatalog.Seal([]executioncatalog.Draft{{Family: 0, Local: 0, Rule: rule, Member: 0, Candidate: candidate, InputCount: 1, OutputCount: 1}})
	if !rowOK || !familyOK || !catalogOK {
		t.Fatal("seal generated exact family")
	}
	if !member.sealInvocationRow(catalog, 0, 0, fixture.unit, fixture.target) {
		t.Fatal("seal generated invocation row")
	}
	run := execution.NewRun(1, 1)
	executor := family.NewExecutor(run)
	if run == nil || executor == nil {
		t.Fatal("generated family worker")
	}
	program.generatedExecution = &generatedExecutionProgram{catalog: catalog, families: []execution.Family{family}}
	runtime := &solverRuntime{program: program}
	epoch := &executorEpoch{
		runtime:          runtime,
		work:             fixture.work,
		generatedCatalog: catalog,
		generatedWorkers: []generatedExecutionWorker{{run: run, executor: executor}},
		relationRevision: 1,
		generation:       1,
	}
	return member, epoch
}

func beginGeneratedMemberBase(t *testing.T, fixture generatedFactorAdapterFixture) carrier.RuleContributionBase {
	t.Helper()
	base, ok := fixture.work.BeginRuleContribution(fixture.plan, fixture.composition.Scope(), []carrier.PointState{fixture.source}, fixture.whole)
	if !ok {
		t.Fatal("begin generated member contribution")
	}
	return base
}

func TestGeneratedMembersResolveOneSchemaOwnedDescriptor(t *testing.T) {
	fixture := newGeneratedFactorAdapterFixture(t)
	first, epoch := generatedMemberRuntime(t, fixture, 17, 23)
	second, ok := newGeneratedMember(generatedMemberTestSpec(t, fixture, 17, 24))
	if !ok {
		t.Fatal("seal second generated member")
	}
	if first.rule != second.rule {
		t.Fatal("members did not share rule ordinal")
	}
	left, leftOK := epoch.runtime.program.generatedProgramAt(first.rule)
	right, rightOK := epoch.runtime.program.generatedProgramAt(second.rule)
	if !leftOK || !rightOK || !reflect.DeepEqual(left, right) {
		t.Fatal("members did not resolve one descriptor")
	}
	// The row retains only sealed geometry. The epoch owns one Run and one
	// composition Executor for its invocation lifetime.
}

func TestRuntimeProgramsBorrowSchemaGeneratedDescriptors(t *testing.T) {
	descriptor, ok := newExactIdentityDescriptor(0, 1, 0, 0, 0, 1, 1)
	if !ok {
		t.Fatal("seal generated descriptor")
	}
	schema := &Schema{available: true, generatedPrograms: []generated.CompiledRule{descriptor}, generatedPresent: true}
	first, firstPresent, firstOK := sealedGeneratedPrograms(schema)
	second, secondPresent, secondOK := sealedGeneratedPrograms(schema)
	if !firstOK || !secondOK || !firstPresent || !secondPresent || len(first) != 1 || len(second) != 1 {
		t.Fatal("borrow generated descriptor table")
	}
	if &first[0] != &schema.generatedPrograms[0] || &second[0] != &schema.generatedPrograms[0] {
		t.Fatal("runtime copied the schema-owned descriptor table")
	}
}

func TestGeneratedMemberRowDispatchRefusesForeignOrStaleHandles(t *testing.T) {
	fixture := newGeneratedFactorAdapterFixture(t)
	member, epoch := generatedMemberRuntime(t, fixture, 17, 23)
	base := beginGeneratedMemberBase(t, fixture)
	foreign := newGeneratedFactorAdapterFixture(t)
	member.unit = foreign.unit
	result := epoch.executeMemberRow(memberRow{generated: member}, base, []carrier.State{fixture.source.State()}, fixture.whole)
	if result.valid || result.boundary == boundaryNone {
		t.Fatal("foreign Unit was accepted")
	}
	_ = fixture.work.AbortRuleContribution(base, nil)

	member, epoch = generatedMemberRuntime(t, fixture, 17, 23)
	base = beginGeneratedMemberBase(t, fixture)
	member.target = foreign.target
	result = epoch.executeMemberRow(memberRow{generated: member}, base, []carrier.State{fixture.source.State()}, fixture.whole)
	if result.valid || result.boundary == boundaryNone {
		t.Fatal("foreign Target was accepted")
	}
	_ = fixture.work.AbortRuleContribution(base, nil)
}

// generatedMemberDispatchRefusal runs one dispatch that must not authenticate
// and answers the boundary that refused it by name.
func generatedMemberDispatchRefusal(t *testing.T, fixture generatedFactorAdapterFixture, epoch *executorEpoch, member *generatedMember) string {
	t.Helper()
	base := beginGeneratedMemberBase(t, fixture)
	result := epoch.executeMemberRow(memberRow{generated: member}, base, []carrier.State{fixture.source.State()}, fixture.whole)
	_ = fixture.work.AbortRuleContribution(base, nil)
	if result.valid || result.wrote || !result.boundary.available() {
		t.Fatal("dispatch admitted an unauthenticated row")
	}
	return result.boundary.site
}

// TestGeneratedMemberRowDispatchRefusesAnUnsealedAddress states that a member
// row reaches execution only through the seal the execution program installed.
// A row that was never given an address has none to borrow from the epoch.
func TestGeneratedMemberRowDispatchRefusesAnUnsealedAddress(t *testing.T) {
	fixture := newGeneratedFactorAdapterFixture(t)
	_, epoch := generatedMemberRuntime(t, fixture, 17, 23)
	unsealed, ok := newGeneratedMember(generatedMemberTestSpec(t, fixture, 17, 23))
	if !ok {
		t.Fatal("seal generated member")
	}
	if site := generatedMemberDispatchRefusal(t, fixture, epoch, unsealed); site != "unsealed-row" {
		t.Fatalf("unsealed row refused at %q", site)
	}
}

// TestGeneratedMemberRowDispatchRefusesAForeignCatalog states the owner fence
// on the address itself. A Ref is meaningful only against the catalog that
// minted it, so an epoch carrying any other catalog - a superseded execution
// program, or another Program's - refuses before it indexes a row.
func TestGeneratedMemberRowDispatchRefusesAForeignCatalog(t *testing.T) {
	fixture := newGeneratedFactorAdapterFixture(t)
	member, epoch := generatedMemberRuntime(t, fixture, 17, 23)
	foreign, ok := executioncatalog.Seal([]executioncatalog.Draft{{Family: 0, Local: 0, Rule: 17, Member: 0, Candidate: 23, InputCount: 1, OutputCount: 1}})
	if !ok || foreign == epoch.generatedCatalog {
		t.Fatal("seal foreign catalog")
	}
	epoch.generatedCatalog = foreign
	if site := generatedMemberDispatchRefusal(t, fixture, epoch, member); site != "row-owner" {
		t.Fatalf("foreign catalog refused at %q", site)
	}
}

// TestGeneratedMemberRowDispatchRefusesAWrongRowOrdinal states that the sealed
// address must still name this row's own occurrence. The catalog row carries
// the member, rule and candidate ordinals it was drafted under; a row whose
// live occurrence has moved off them is dispatching against another row.
func TestGeneratedMemberRowDispatchRefusesAWrongRowOrdinal(t *testing.T) {
	fixture := newGeneratedFactorAdapterFixture(t)
	member, epoch := generatedMemberRuntime(t, fixture, 17, 23)
	member.candidate = 24
	if site := generatedMemberDispatchRefusal(t, fixture, epoch, member); site != "row-ordinal" {
		t.Fatalf("wrong candidate ordinal refused at %q", site)
	}

	member, epoch = generatedMemberRuntime(t, fixture, 17, 23)
	member.rule = 18
	if site := generatedMemberDispatchRefusal(t, fixture, epoch, member); site != "row-ordinal" {
		t.Fatalf("wrong rule ordinal refused at %q", site)
	}
}

// TestGeneratedMemberRowDispatchRefusesAStaleGeneration states the one fence
// that cannot be sealed with the address: the invocation fences name this
// solve generation, so an epoch that no longer carries a live relation
// revision and generation refuses before a Ticket is issued against them.
func TestGeneratedMemberRowDispatchRefusesAStaleGeneration(t *testing.T) {
	fixture := newGeneratedFactorAdapterFixture(t)
	member, epoch := generatedMemberRuntime(t, fixture, 17, 23)
	epoch.generation = 0
	if site := generatedMemberDispatchRefusal(t, fixture, epoch, member); site != "generation" {
		t.Fatalf("stale generation refused at %q", site)
	}

	member, epoch = generatedMemberRuntime(t, fixture, 17, 23)
	epoch.relationRevision = 0
	if site := generatedMemberDispatchRefusal(t, fixture, epoch, member); site != "generation" {
		t.Fatalf("stale relation revision refused at %q", site)
	}
}

// TestGeneratedMemberRowSealsOneAddressOnce states that the install-time proof
// is established exactly once and only against a catalog row that already
// names this member. A second address, a row that names another occurrence,
// and handles other than the ones the typed family row was built from are all
// refused at install rather than at dispatch.
func TestGeneratedMemberRowSealsOneAddressOnce(t *testing.T) {
	fixture := newGeneratedFactorAdapterFixture(t)
	member, epoch := generatedMemberRuntime(t, fixture, 17, 23)
	if member.sealInvocationRow(epoch.generatedCatalog, 0, 0, fixture.unit, fixture.target) {
		t.Fatal("a sealed row took a second address")
	}
	foreign := newGeneratedFactorAdapterFixture(t)
	fresh, ok := newGeneratedMember(generatedMemberTestSpec(t, fixture, 17, 23))
	if !ok {
		t.Fatal("seal generated member")
	}
	if fresh.sealInvocationRow(epoch.generatedCatalog, 0, 0, foreign.unit, fixture.target) {
		t.Fatal("a row was sealed to a foreign Unit")
	}
	if fresh.sealInvocationRow(epoch.generatedCatalog, 0, 1, fixture.unit, fixture.target) {
		t.Fatal("a row was sealed to another member ordinal")
	}
	if fresh.invocation.installed() {
		t.Fatal("a refused seal installed an address")
	}
	if !fresh.sealInvocationRow(epoch.generatedCatalog, 0, 0, fixture.unit, fixture.target) {
		t.Fatal("seal generated invocation row")
	}
}

func TestGeneratedMemberRowIsExclusive(t *testing.T) {
	fixture := newGeneratedFactorAdapterFixture(t)
	member, epoch := generatedMemberRuntime(t, fixture, 17, 23)
	row := memberRow{generated: member}
	geometry, ok := row.geometry()
	if !ok || geometry != member || !row.valid() || geometry.member().Key() != member.member().Key() {
		t.Fatal("generated row geometry")
	}
	if _, ok := (memberRow{}).geometry(); ok || (memberRow{}).valid() {
		t.Fatal("empty member row admitted")
	}
	if _, ok := (memberRow{legacy: (*boundRuleMember[uint64, ruleUnit])(nil), generated: member}).geometry(); ok {
		t.Fatal("dual member row admitted")
	}
	if result := epoch.executeMemberRow(memberRow{}, carrier.RuleContributionBase{}, nil, fixture.whole); result.valid || result.boundary == boundaryNone {
		t.Fatal("empty member row dispatched")
	}
}

// generatedRoutedMemberSpec is the routed shape: no exact target and no static
// output vector, the output Factor's route universe claimed instead.
func generatedRoutedMemberSpec(t *testing.T, fixture generatedFactorAdapterFixture) generatedMemberSpec {
	t.Helper()
	spec := generatedMemberTestSpec(t, fixture, 0, 0)
	spec.target, spec.targets = carrier.Target{}, nil
	spec.routed, spec.route = true, fixture.factor
	return spec
}

// TestARoutedMemberMayCarryWhatItsRoutesDidNotSelect states the shape a
// routed rule usually has, and the one the generated arm used to refuse.
//
// A routed publication writes at the coordinates its derived relation
// answered. Every other coordinate of that Factor is one the row must
// PRESERVE, and preserving them is what a carry is. Refusing the pair would
// have made every routed rule that keeps a world - which is every freeze - an
// erasure of everything it did not route to.
//
// The two halves stay distinct: the carry closure is what the row preserves,
// and it is never a destination, because a routed row has no static
// destination at all.
func TestARoutedMemberMayCarryWhatItsRoutesDidNotSelect(t *testing.T) {
	fixture := newGeneratedFactorAdapterFixture(t)
	spec := generatedRoutedMemberSpec(t, fixture)
	spec.carries = []int{0}
	spec.carryTargets = []carrier.Target{fixture.target}
	member, sealed := newGeneratedMember(spec)
	if !sealed || member == nil {
		t.Fatal("a routed member that carries was refused")
	}
	if len(member.targets()) != 0 {
		t.Fatalf("a routed member published %d static destinations", len(member.targets()))
	}
	if len(member.carryTargets()) != 1 || member.carryTargets()[0] != fixture.target {
		t.Fatal("the routed member does not preserve its carried coordinates")
	}
	if member.routeScope() != fixture.factor {
		t.Fatal("the routed member claims no route universe")
	}
	if carries := member.carries(); len(carries) != 1 || carries[0] != 0 {
		t.Fatalf("carried inputs = %v, want the one the declaration named", carries)
	}
}

// TestARoutedMemberHasNoStaticDestination fences the other direction. A
// carried coordinate is not a destination, and a routed row that named one
// would be publishing somewhere its relation never answered.
func TestARoutedMemberHasNoStaticDestination(t *testing.T) {
	fixture := newGeneratedFactorAdapterFixture(t)
	for _, test := range []struct {
		name  string
		amend func(*generatedMemberSpec)
	}{
		{name: "exact-target", amend: func(spec *generatedMemberSpec) { spec.target = fixture.target }},
		{name: "static-output-vector", amend: func(spec *generatedMemberSpec) {
			spec.targets = []carrier.Target{fixture.target}
		}},
		{name: "carried-with-no-carry", amend: func(spec *generatedMemberSpec) {
			spec.carryTargets = []carrier.Target{fixture.target}
		}},
		{name: "no-route-universe", amend: func(spec *generatedMemberSpec) { spec.route = nil }},
	} {
		t.Run(test.name, func(t *testing.T) {
			spec := generatedRoutedMemberSpec(t, fixture)
			test.amend(&spec)
			if _, sealed := newGeneratedMember(spec); sealed {
				t.Fatal("a routed member was sealed with a destination it cannot have")
			}
		})
	}
}
